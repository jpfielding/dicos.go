package dicos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"slices"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

const (
	// DefaultMaxElementBytes is the per-element length limit used when
	// Reader.MaxElementBytes is zero (1 GiB).
	DefaultMaxElementBytes = 1 << 30

	// MaxNestingDepth bounds sequence/item nesting so a crafted file cannot
	// recurse without limit.
	MaxNestingDepth = 64

	// undefinedLength is the DICOM sentinel value length.
	undefinedLength = 0xFFFFFFFF

	// readChunk is the largest single allocation made for a value before its
	// bytes have actually been read. Larger values grow chunk by chunk, so
	// memory tracks the bytes present in the stream, not the declared length.
	readChunk = 1 << 20

	// botReserveCap caps the speculative capacity reserved for Basic Offset
	// Table entries; the declared BOT length is attacker controlled.
	botReserveCap = 4096
)

// Reader reads DICOS/DICOM Part-10 files.
//
// The zero value is not usable; create one with NewReader. Configure the
// exported fields before calling ReadDataset.
type Reader struct {
	// MaxElementBytes is the largest declared value length (element body,
	// fragment, or fixed-length item) the reader accepts. Zero means
	// DefaultMaxElementBytes. Longer values fail with a *LengthError
	// wrapping ErrLengthExceedsLimit before anything is allocated.
	MaxElementBytes int64

	r              io.Reader
	offset         int64  // bytes consumed from r
	transferSyntax string // (0002,0010) as recorded, or the inferred default
	tsKnown        bool   // transferSyntax was recorded or inferred
	explicitVR     bool
	inMeta         bool // still inside group 0002
	warnings       []ParseWarning
}

// NewReader creates a new DICOS reader over r.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:          r,
		explicitVR: true,
		inMeta:     true,
	}
}

// Parse reads a complete DICOS file using the default limits. Parse
// warnings are logged at debug level; use a Reader to inspect them.
func Parse(r io.Reader) (*Dataset, error) {
	rd := NewReader(r)
	ds, err := rd.ReadDataset()
	for _, w := range rd.warnings {
		slog.Debug("dicos parse warning", "warning", w.String())
	}
	return ds, err
}

// Warnings returns the non-fatal issues found by the last ReadDataset.
func (r *Reader) Warnings() []ParseWarning {
	return slices.Clone(r.warnings)
}

func (r *Reader) maxElementBytes() int64 {
	if r.MaxElementBytes > 0 {
		return r.MaxElementBytes
	}
	return DefaultMaxElementBytes
}

// fill reads up to len(buf) bytes and returns how many were read. A short
// count with a nil error means the stream ended; any other read error is
// returned.
func (r *Reader) fill(buf []byte) (int, error) {
	n, err := io.ReadFull(r.r, buf)
	r.offset += int64(n)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	return n, err
}

// readExact reads exactly len(buf) bytes; a short read is a *TruncatedError.
func (r *Reader) readExact(buf []byte, context string) error {
	offset := r.offset
	n, err := r.fill(buf)
	if err != nil {
		return fmt.Errorf("reading %s: %w", context, err)
	}
	if n < len(buf) {
		return &TruncatedError{Offset: offset, Context: context}
	}
	return nil
}

func (r *Reader) readU16(context string) (uint16, error) {
	var b [2]byte
	if err := r.readExact(b[:], context); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b[:]), nil
}

func (r *Reader) readU32(context string) (uint32, error) {
	var b [4]byte
	if err := r.readExact(b[:], context); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

// readBoundaryTag reads a tag at an element boundary. ok is false on a clean
// end of stream (zero bytes); one to three bytes is a *TruncatedError.
func (r *Reader) readBoundaryTag(context string) (t Tag, ok bool, err error) {
	offset := r.offset
	var b [4]byte
	n, err := r.fill(b[:])
	if err != nil {
		return Tag{}, false, fmt.Errorf("reading %s: %w", context, err)
	}
	switch {
	case n == 0:
		return Tag{}, false, nil
	case n < 4:
		return Tag{}, false, &TruncatedError{Offset: offset, Context: context}
	}
	return Tag{Group: binary.LittleEndian.Uint16(b[:2]), Element: binary.LittleEndian.Uint16(b[2:])}, true, nil
}

// readTagRequired reads a tag that must be present; a clean end of stream
// here is a truncation.
func (r *Reader) readTagRequired(context string) (Tag, error) {
	t, ok, err := r.readBoundaryTag(context)
	if err != nil {
		return Tag{}, err
	}
	if !ok {
		return Tag{}, &TruncatedError{Offset: r.offset, Context: context}
	}
	return t, nil
}

// readBody reads a value of declared length n. The length is checked against
// MaxElementBytes first; the buffer then grows only as bytes actually arrive,
// so a large declared length with a short stream fails with a truncation
// after allocating at most about twice the bytes present.
func (r *Reader) readBody(n uint32, context string) ([]byte, error) {
	if limit := r.maxElementBytes(); int64(n) > limit {
		return nil, &LengthError{Length: int64(n), Max: limit, Err: ErrLengthExceedsLimit}
	}
	if n <= readChunk {
		buf := make([]byte, n)
		if err := r.readExact(buf, context); err != nil {
			return nil, err
		}
		return buf, nil
	}
	start := r.offset
	buf := make([]byte, 0, readChunk)
	for remaining := int(n); remaining > 0; {
		k := min(remaining, readChunk)
		buf = slices.Grow(buf, k)
		tail := buf[len(buf) : len(buf)+k]
		got, err := r.fill(tail)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", context, err)
		}
		if got < k {
			return nil, &TruncatedError{Offset: start, Context: context}
		}
		buf = buf[:len(buf)+k]
		remaining -= k
	}
	return buf, nil
}

// skipBytes discards n bytes, verifying the full count was consumed.
func (r *Reader) skipBytes(n uint32, context string) error {
	offset := r.offset
	copied, err := io.CopyN(io.Discard, r.r, int64(n))
	r.offset += copied
	if copied == int64(n) {
		return nil
	}
	if err != nil && err != io.EOF {
		return fmt.Errorf("skipping %s: %w", context, err)
	}
	return &TruncatedError{Offset: offset, Context: context}
}

// ReadDataset reads the complete dataset: preamble, magic, File Meta
// Information, then the dataset proper in its transfer syntax.
func (r *Reader) ReadDataset() (*Dataset, error) {
	var preamble [128]byte
	if n, err := r.fill(preamble[:]); err != nil {
		return nil, fmt.Errorf("reading preamble: %w", err)
	} else if n != len(preamble) {
		return nil, fmt.Errorf("%w: preamble too short", ErrBadPreamble)
	}
	var magic [4]byte
	if n, err := r.fill(magic[:]); err != nil {
		return nil, fmt.Errorf("reading DICM magic: %w", err)
	} else if n != len(magic) || string(magic[:]) != "DICM" {
		return nil, fmt.Errorf("%w: missing DICM magic number", ErrBadPreamble)
	}

	// Group 0002 is always Explicit VR Little Endian.
	r.explicitVR = true
	r.inMeta = true

	ds := &Dataset{Elements: make(map[Tag]*Element)}
	err := r.parseElements(0, func(elem *Element) {
		if elem.Tag == tag.TransferSyntaxUID {
			if s, ok := elem.Value.(string); ok {
				// Recorded only; the VR mode switches on leaving group 0002.
				r.transferSyntax = s
				r.tsKnown = true
			}
		}
		ds.Elements[elem.Tag] = elem
	})
	if err != nil {
		return nil, err
	}

	// Materialise an inferred default so ds.TransferSyntax() agrees with the
	// VR mode the reader actually used. Only when the dataset was entered.
	if _, ok := ds.Elements[tag.TransferSyntaxUID]; !ok && r.tsKnown {
		ds.Elements[tag.TransferSyntaxUID] = &Element{Tag: tag.TransferSyntaxUID, VR: "UI", Value: r.transferSyntax}
	}
	r.normalizeNativePixelData(ds)
	return ds, nil
}

// enterDatasetIfNeeded applies the transfer syntax when the first tag outside
// group 0002 is seen. Big-endian transfer syntaxes are rejected.
func (r *Reader) enterDatasetIfNeeded(t Tag) error {
	if t.Group == 0x0002 || !r.inMeta {
		return nil
	}
	r.inMeta = false
	if !r.tsKnown {
		// PS3.5 §10.1: no Transfer Syntax UID means Implicit VR Little Endian.
		r.transferSyntax = string(transfer.ImplicitVRLittleEndian)
		r.tsKnown = true
	}
	ts := transfer.Syntax(r.transferSyntax)
	if !ts.IsLittleEndian() {
		return fmt.Errorf("%w: %s", ErrUnsupportedTransferSyntax, r.transferSyntax)
	}
	r.explicitVR = ts.IsExplicitVR()
	return nil
}

// parseElements reads elements until a clean end of stream, passing each to
// emit. Group FFFE tags in element position are structural, not data: they
// are consumed with their 4-byte length (no VR) and dropped, and parsing
// continues.
func (r *Reader) parseElements(depth int, emit func(*Element)) error {
	for {
		t, ok, err := r.readBoundaryTag("element tag")
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if t.Group == 0xFFFE {
			if err := r.skipDelimiter(t); err != nil {
				return err
			}
			continue
		}
		if err := r.enterDatasetIfNeeded(t); err != nil {
			return err
		}
		elem, err := r.readElementWithTag(t, depth)
		if err != nil {
			return fmt.Errorf("element %v: %w", t, err)
		}
		emit(elem)
	}
}

// skipDelimiter consumes a stray group FFFE tag's 4-byte length and any
// fixed-length body.
func (r *Reader) skipDelimiter(t Tag) error {
	n, err := r.readU32("delimiter length")
	if err != nil {
		return err
	}
	if n == 0 || n == undefinedLength {
		return nil
	}
	if limit := r.maxElementBytes(); int64(n) > limit {
		return &LengthError{Length: int64(n), Max: limit, Err: ErrLengthExceedsLimit}
	}
	return r.skipBytes(n, fmt.Sprintf("delimiter %v body", t))
}

// readHeader reads the VR and value length following a tag.
func (r *Reader) readHeader(t Tag) (vr string, vl uint32, err error) {
	if !r.explicitVR {
		vl, err = r.readU32("implicit VR length")
		return getImplicitVR(t), vl, err
	}
	var vrb [2]byte
	if err := r.readExact(vrb[:], "explicit VR header"); err != nil {
		return "", 0, err
	}
	vr = string(vrb[:])
	if !knownVR(vr) {
		vr = "UN" // unknown VR bytes: treat as UN (long VR form)
	}
	if isLongVR(vr) {
		var reserved [2]byte
		if err := r.readExact(reserved[:], "explicit VR header"); err != nil {
			return "", 0, err
		}
		vl, err = r.readU32("explicit VR length")
		return vr, vl, err
	}
	vl16, err := r.readU16("explicit VR length")
	return vr, uint32(vl16), err
}

// readElementWithTag reads a DICOM element after the tag has been read.
func (r *Reader) readElementWithTag(t Tag, depth int) (*Element, error) {
	vr, vl, err := r.readHeader(t)
	if err != nil {
		return nil, err
	}
	value, err := r.readValue(t, vr, vl, depth)
	if err != nil {
		return nil, err
	}
	return &Element{Tag: t, VR: vr, Value: value}, nil
}

// readValue reads the value based on VR and VL.
func (r *Reader) readValue(t Tag, vr string, vl uint32, depth int) (interface{}, error) {
	if vl == undefinedLength {
		return r.readUndefinedLengthValue(t, vr, depth)
	}
	data, err := r.readBody(vl, "element value")
	if err != nil {
		return nil, err
	}
	if vr == "SQ" {
		return r.parseSequenceItems(data, depth)
	}
	return parseValue(vr, data), nil
}

// readUndefinedLengthValue handles encapsulated pixel data and sequences
// with undefined length. Any other undefined-length value is skipped and
// stored as nil.
func (r *Reader) readUndefinedLengthValue(t Tag, vr string, depth int) (interface{}, error) {
	if t == tag.PixelData {
		return r.readEncapsulatedPixelData()
	}
	if vr == "SQ" {
		return r.readSequenceItems(depth)
	}
	return nil, r.skipUndefinedLength(depth)
}

func nestingError() error { return fmt.Errorf("%w (%d)", ErrNestingTooDeep, MaxNestingDepth) }

// readSequenceItems reads items of an undefined-length sequence up to its
// Sequence Delimitation Item.
func (r *Reader) readSequenceItems(depth int) ([]*Dataset, error) {
	if depth >= MaxNestingDepth {
		return nil, nestingError()
	}
	items := []*Dataset{}
	for {
		t, err := r.readTagRequired("sequence item")
		if err != nil {
			return nil, err
		}
		n, err := r.readU32("sequence item length")
		if err != nil {
			return nil, err
		}
		if t == tag.SequenceDelimitationItem {
			return items, nil
		}
		if t != tag.Item {
			return nil, &UnexpectedTagError{Expected: tag.Item, Got: t, Context: "sequence item"}
		}
		var item *Dataset
		if n == undefinedLength {
			item, err = r.readItemUndefinedLength(depth)
		} else {
			item, err = r.readItemFixedLength(n, depth)
		}
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
}

// readItemUndefinedLength reads an item delimited by an Item Delimitation
// Item.
func (r *Reader) readItemUndefinedLength(depth int) (*Dataset, error) {
	ds := &Dataset{Elements: make(map[Tag]*Element)}
	for {
		t, err := r.readTagRequired("sequence item element")
		if err != nil {
			return nil, err
		}
		if t == tag.ItemDelimitationItem {
			if _, err := r.readU32("item delimiter length"); err != nil {
				return nil, err
			}
			return ds, nil
		}
		if t.Group == 0xFFFE {
			if err := r.skipDelimiter(t); err != nil {
				return nil, err
			}
			continue
		}
		elem, err := r.readElementWithTag(t, depth+1)
		if err != nil {
			return nil, fmt.Errorf("element %v: %w", t, err)
		}
		ds.Elements[elem.Tag] = elem
	}
}

// readItemFixedLength reads an item of known length and parses it as a
// nested dataset.
func (r *Reader) readItemFixedLength(n uint32, depth int) (*Dataset, error) {
	data, err := r.readBody(n, "fixed-length item body")
	if err != nil {
		return nil, err
	}
	return r.parseItem(data, depth)
}

// parseItem parses item bytes as a dataset with this reader's VR mode and
// limits.
func (r *Reader) parseItem(data []byte, depth int) (*Dataset, error) {
	sub := &Reader{
		MaxElementBytes: r.maxElementBytes(),
		r:               bytes.NewReader(data),
		transferSyntax:  r.transferSyntax,
		tsKnown:         r.tsKnown,
		explicitVR:      r.explicitVR,
	}
	ds := &Dataset{Elements: make(map[Tag]*Element)}
	err := sub.parseElements(depth+1, func(elem *Element) { ds.Elements[elem.Tag] = elem })
	if err != nil {
		return nil, err
	}
	return ds, nil
}

// parseSequenceItems parses the items of a fixed-length sequence value.
func (r *Reader) parseSequenceItems(data []byte, depth int) ([]*Dataset, error) {
	if depth >= MaxNestingDepth {
		return nil, nestingError()
	}
	items := []*Dataset{}
	pos := 0
	for pos < len(data) {
		if len(data)-pos < 8 {
			return nil, &TruncatedError{Offset: int64(pos), Context: "fixed-length SQ item"}
		}
		t := Tag{
			Group:   binary.LittleEndian.Uint16(data[pos:]),
			Element: binary.LittleEndian.Uint16(data[pos+2:]),
		}
		n := binary.LittleEndian.Uint32(data[pos+4:])
		pos += 8
		if t == tag.SequenceDelimitationItem {
			break
		}
		if t != tag.Item {
			return nil, &UnexpectedTagError{Expected: tag.Item, Got: t, Context: "fixed-length SQ item"}
		}
		if n == undefinedLength {
			return nil, ErrUndefinedLengthInFixedSQ
		}
		if remaining := len(data) - pos; int64(n) > int64(remaining) {
			return nil, &LengthError{Length: int64(n), Max: int64(remaining), Err: ErrLengthExceedsBuffer}
		}
		item, err := r.parseItem(data[pos:pos+int(n)], depth)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		pos += int(n)
	}
	return items, nil
}

// skipUndefinedLength skips an undefined-length value up to and including
// its Sequence Delimitation Item.
func (r *Reader) skipUndefinedLength(depth int) error {
	if depth >= MaxNestingDepth {
		return nestingError()
	}
	for {
		t, err := r.readTagRequired("skipped sequence")
		if err != nil {
			return err
		}
		if t.Group == 0xFFFE {
			n, err := r.readU32("skipped item length")
			if err != nil {
				return err
			}
			switch {
			case t == tag.SequenceDelimitationItem:
				return nil
			case t == tag.ItemDelimitationItem:
				continue
			case n == undefinedLength:
				if err := r.skipUndefinedLength(depth + 1); err != nil {
					return err
				}
			case n > 0:
				if err := r.skipBytes(n, "skipped item body"); err != nil {
					return err
				}
			}
			continue
		}
		_, vl, err := r.readHeader(t)
		if err != nil {
			return err
		}
		switch {
		case vl == undefinedLength:
			if err := r.skipUndefinedLength(depth + 1); err != nil {
				return err
			}
		case vl > 0:
			if err := r.skipBytes(vl, "skipped element body"); err != nil {
				return err
			}
		}
	}
}

// readEncapsulatedPixelData reads encapsulated (compressed) pixel data: the
// Basic Offset Table item, then fragment items until the Sequence
// Delimitation Item.
func (r *Reader) readEncapsulatedPixelData() (*PixelData, error) {
	pd := &PixelData{Frames: []Frame{}}

	botTag, err := r.readTagRequired("basic offset table")
	if err != nil {
		return nil, err
	}
	if botTag != tag.Item {
		return nil, &UnexpectedTagError{Expected: tag.Item, Got: botTag, Context: "basic offset table"}
	}
	botLen, err := r.readU32("basic offset table length")
	if err != nil {
		return nil, err
	}
	if botLen > 0 {
		// Never reserve from the declared length; each entry is read, so a
		// bogus count fails on the first missing entry.
		num := botLen / 4
		pd.Offsets = make([]uint32, 0, min(num, botReserveCap))
		for range num {
			off, err := r.readU32("basic offset table entry")
			if err != nil {
				return nil, err
			}
			pd.Offsets = append(pd.Offsets, off)
		}
	}

	// Fragment offsets are relative to the first fragment's Item tag, which
	// is what BOT entries reference (PS3.5 A.4).
	var fragments [][]byte
	var fragOffsets []uint32
	var running uint32
	for {
		t, err := r.readTagRequired("encapsulated pixel data")
		if err != nil {
			return nil, err
		}
		if t == tag.SequenceDelimitationItem {
			if _, err := r.readU32("pixel data delimiter length"); err != nil {
				return nil, err
			}
			break
		}
		if t != tag.Item {
			return nil, &UnexpectedTagError{Expected: tag.Item, Got: t, Context: "encapsulated pixel data"}
		}
		n, err := r.readU32("encapsulated frame length")
		if err != nil {
			return nil, err
		}
		data, err := r.readBody(n, "encapsulated frame")
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, data)
		fragOffsets = append(fragOffsets, running)
		running = satAddU32(satAddU32(running, 8), n)
	}

	frames, ok := assembleEncapsulatedFrames(fragments, fragOffsets, pd.Offsets)
	if !ok {
		r.warnings = append(r.warnings, ParseWarning{Kind: WarnInvalidOffsetTable})
	}
	pd.Frames = make([]Frame, len(frames))
	for i, f := range frames {
		pd.Frames[i] = Frame{CompressedData: f}
	}
	return pd, nil
}

func satAddU32(a, b uint32) uint32 {
	if a > math.MaxUint32-b {
		return math.MaxUint32
	}
	return a + b
}

// satMul multiplies non-negative ints, saturating at math.MaxInt.
func satMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	if a > math.MaxInt/b {
		return math.MaxInt
	}
	return a * b
}

// assembleEncapsulatedFrames groups fragments into frames. An empty BOT means
// one fragment per frame. A non-empty BOT groups the fragments whose offsets
// fall in [bot[i], bot[i+1]) into frame i. The BOT must start at 0, be
// strictly increasing and give every frame at least one fragment; otherwise
// it is ignored (ok = false) and each fragment becomes a frame. Unlike
// dicos.rs, overlapping windows are never honoured, so no fragment is copied
// more than once.
func assembleEncapsulatedFrames(fragments [][]byte, offsets, bot []uint32) (frames [][]byte, ok bool) {
	if len(bot) == 0 {
		return fragments, true
	}
	if bot[0] != 0 || len(bot) > len(fragments) {
		return fragments, false
	}
	starts := make([]int, len(bot)) // first fragment index of each frame
	j := 0
	for i, start := range bot {
		if i > 0 && start <= bot[i-1] {
			return fragments, false
		}
		for j < len(offsets) && offsets[j] < start {
			j++
		}
		if j == len(offsets) || offsets[j] != start {
			return fragments, false // no fragment begins at this offset
		}
		starts[i] = j
	}
	frames = make([][]byte, len(bot))
	for i, first := range starts {
		end := len(fragments)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if end-first == 1 {
			frames[i] = fragments[first]
			continue
		}
		size := 0
		for _, f := range fragments[first:end] {
			size += len(f)
		}
		frame := make([]byte, 0, size)
		for _, f := range fragments[first:end] {
			frame = append(frame, f...)
		}
		frames[i] = frame
	}
	return frames, true
}

// normalizeNativePixelData converts a parsed native (7FE0,0010) []byte value
// into *PixelData, following dicos.rs: 8-bit data is one sample per byte,
// anything else 16-bit little-endian pairs. Frames are derived from the
// bytes present, never from the declared geometry alone. Without Rows and
// Columns the raw bytes are left in place.
func (r *Reader) normalizeNativePixelData(ds *Dataset) {
	elem, ok := ds.Elements[tag.PixelData]
	if !ok {
		return
	}
	raw, ok := elem.Value.([]byte)
	if !ok {
		return
	}
	rows, cols := ds.Rows(), ds.Columns()
	if rows <= 0 || cols <= 0 {
		return
	}
	numFrames := max(ds.NumberOfFrames(), 1)
	bits, bitsKnown := bitsAllocated(ds)
	eightBit := bitsKnown && bits == 8

	var samples []uint16
	if eightBit {
		samples = make([]uint16, len(raw))
		for i, b := range raw {
			samples[i] = uint16(b)
		}
	} else {
		if !bitsKnown {
			r.warnings = append(r.warnings, ParseWarning{Kind: WarnBitsAllocatedMissing})
		}
		if len(raw)%2 != 0 {
			r.warnings = append(r.warnings, ParseWarning{Kind: WarnOddPixelDataLength, Length: len(raw)})
		}
		samples = make([]uint16, len(raw)/2)
		for i := range samples {
			samples[i] = binary.LittleEndian.Uint16(raw[2*i:])
		}
	}

	frameSize := satMul(rows, cols)
	expected := satMul(frameSize, numFrames)
	// Value lengths are even: one trailing pad byte on an odd-sized 8-bit
	// image is not a sample.
	if eightBit && expected%2 == 1 && len(samples) == expected+1 {
		samples = samples[:expected]
	}

	pd := &PixelData{}
	switch {
	case numFrames > 1 && len(samples) == expected:
		pd.Frames = make([]Frame, numFrames)
		for i := range pd.Frames {
			lo, hi := i*frameSize, (i+1)*frameSize
			pd.Frames[i] = Frame{Data: samples[lo:hi:hi]}
		}
	default:
		if len(samples) != expected {
			r.warnings = append(r.warnings, ParseWarning{Kind: WarnPixelCountMismatch, Actual: len(samples), Expected: expected})
		}
		pd.Frames = []Frame{{Data: samples}}
	}
	elem.Value = pd
}

// bitsAllocated returns BitsAllocated (0028,0100) and whether it is present.
func bitsAllocated(ds *Dataset) (int, bool) {
	elem, ok := ds.Elements[tag.BitsAllocated]
	if !ok {
		return 0, false
	}
	return elem.GetInt()
}

// isLongVR returns true if VR uses 4-byte VL (OB, OD, OF, OL, OW, SQ, UC, UR, UT, UN)
func isLongVR(vr string) bool {
	switch vr {
	case "OB", "OD", "OF", "OL", "OW", "SQ", "UC", "UR", "UT", "UN":
		return true
	}
	return false
}

// getImplicitVR returns VR for a tag when using Implicit VR transfer syntax
func getImplicitVR(tag Tag) string {
	// For now, return a default - in production, use a tag dictionary
	switch {
	case tag.Group == 0x0002: // File Meta Information
		return "UL"
	case tag.Group == 0x7FE0 && tag.Element == 0x0010:
		return "OW" // Pixel Data
	case tag.Group == 0x0028: // Image Pixel Module
		switch tag.Element {
		case 0x0010, 0x0011, 0x0100, 0x0101, 0x0102, 0x0103, 0x0002:
			return "US"
		case 0x0008:
			return "IS" // Number of Frames
		case 0x0030, 0x1050, 0x1051, 0x1052, 0x1053, 0x1054:
			return "DS" // Spacing, Windowing, Rescale
		case 0x0004:
			return "CS" // Photometric Interpretation
		}
	case tag.Group == 0x0008: // General Information
		switch tag.Element {
		case 0x0016, 0x0018:
			return "UI"
		case 0x0060, 0x0008, 0x0080:
			return "CS"
		}
	}
	return "UN" // Unknown
}

// knownVR reports whether vr is a DICOM Value Representation.
func knownVR(vr string) bool {
	switch vr {
	case "AE", "AS", "AT", "CS", "DA", "DS", "DT", "FD", "FL", "IS", "LO", "LT",
		"OB", "OD", "OF", "OL", "OW", "PN", "SH", "SL", "SQ", "SS", "ST", "TM",
		"UC", "UI", "UL", "UN", "UR", "US", "UT":
		return true
	}
	return false
}

// parseValue converts raw bytes to a typed value based on VR. Numeric
// values whose length does not fit the VR are returned as raw bytes.
// Multi-valued strings stay one backslash-separated string.
func parseValue(vr string, data []byte) interface{} {
	switch vr {
	case "AE", "AS", "CS", "DA", "DS", "DT", "IS", "LO", "LT", "PN", "SH", "ST",
		"TM", "UC", "UI", "UR", "UT":
		return strings.TrimRight(string(data), "\x00 ")
	case "US":
		switch {
		case len(data) == 2:
			return binary.LittleEndian.Uint16(data)
		case len(data) >= 4 && len(data)%2 == 0:
			values := make([]uint16, len(data)/2)
			for i := range values {
				values[i] = binary.LittleEndian.Uint16(data[i*2:])
			}
			return values
		}
	case "UL":
		switch {
		case len(data) == 4:
			return binary.LittleEndian.Uint32(data)
		case len(data) >= 8 && len(data)%4 == 0:
			values := make([]uint32, len(data)/4)
			for i := range values {
				values[i] = binary.LittleEndian.Uint32(data[i*4:])
			}
			return values
		}
	case "SS":
		if len(data) == 2 {
			return int16(binary.LittleEndian.Uint16(data))
		}
	case "SL":
		if len(data) == 4 {
			return int32(binary.LittleEndian.Uint32(data))
		}
	case "FL":
		switch {
		case len(data) == 4:
			return math.Float32frombits(binary.LittleEndian.Uint32(data))
		case len(data) >= 8 && len(data)%4 == 0:
			values := make([]float32, len(data)/4)
			for i := range values {
				values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
			}
			return values
		}
	case "FD":
		switch {
		case len(data) == 8:
			return math.Float64frombits(binary.LittleEndian.Uint64(data))
		case len(data) >= 16 && len(data)%8 == 0:
			values := make([]float64, len(data)/8)
			for i := range values {
				values[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[i*8:]))
			}
			return values
		}
	}
	return data
}

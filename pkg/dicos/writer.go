package dicos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// WriteFile writes a dataset to a DICOS file
func WriteFile(path string, ds *Dataset) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return Write(f, ds)
}

// Write writes a dataset as a DICOM Part-10 stream and returns the number of
// bytes written.
//
// The body is always Explicit VR Little Endian. File Meta Information (group
// 0002) is normalised: (0002,0000) is recomputed, a missing Transfer Syntax
// UID is set to Explicit VR Little Endian, and an Implicit VR Little Endian
// UID is rewritten to Explicit VR Little Endian so the file matches what is
// actually emitted. Compressed transfer syntaxes are kept.
func Write(w io.Writer, ds *Dataset) (int64, error) {
	cw := &CountingWriter{Writer: w}

	if _, err := cw.Write(make([]byte, 128)); err != nil {
		return cw.Count.Load(), err
	}
	if _, err := cw.Write([]byte("DICM")); err != nil {
		return cw.Count.Load(), err
	}

	var meta, body []*Element
	for _, elem := range ds.Elements {
		switch {
		case elem.Tag == tag.FileMetaInformationGroupLength:
		case elem.Tag.Group == 0x0002:
			meta = append(meta, elem)
		default:
			body = append(body, elem)
		}
	}
	meta = normalizeMeta(meta)
	sortElements(meta)
	sortElements(body)

	var metaBuf bytes.Buffer
	for _, elem := range meta {
		if err := writeElement(&metaBuf, elem, 0); err != nil {
			return cw.Count.Load(), fmt.Errorf("writing element %v: %w", elem.Tag, err)
		}
	}
	if int64(metaBuf.Len()) > math.MaxUint32 {
		return cw.Count.Load(), fmt.Errorf("%w: group 0002 is too large", ErrInvalidValue)
	}
	groupLength := &Element{Tag: tag.FileMetaInformationGroupLength, VR: "UL", Value: uint32(metaBuf.Len())}
	if err := writeElement(cw, groupLength, 0); err != nil {
		return cw.Count.Load(), err
	}
	if _, err := cw.Write(metaBuf.Bytes()); err != nil {
		return cw.Count.Load(), err
	}

	bits, _ := bitsAllocated(ds)
	for _, elem := range body {
		b := 0
		if elem.Tag == tag.PixelData {
			b = bits
		}
		if err := writeElement(cw, elem, b); err != nil {
			return cw.Count.Load(), fmt.Errorf("writing element %v: %w", elem.Tag, err)
		}
	}
	return cw.Count.Load(), nil
}

// normalizeMeta injects or rewrites the Transfer Syntax UID (see Write).
func normalizeMeta(meta []*Element) []*Element {
	explicit := &Element{Tag: tag.TransferSyntaxUID, VR: "UI", Value: string(transfer.ExplicitVRLittleEndian)}
	for i, elem := range meta {
		if elem.Tag != tag.TransferSyntaxUID {
			continue
		}
		if s, ok := elem.Value.(string); ok && strings.Trim(s, "\x00 \t\r\n") == string(transfer.ImplicitVRLittleEndian) {
			meta[i] = explicit
		}
		return meta
	}
	return append(meta, explicit)
}

func sortElements(elements []*Element) {
	sort.Slice(elements, func(i, j int) bool {
		a, b := elements[i].Tag, elements[j].Tag
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Element < b.Element
	})
}

// writeDataSetBody writes all elements of ds in tag order, without File
// Meta normalisation (used for sequence items).
func writeDataSetBody(w io.Writer, ds *Dataset) (int64, error) {
	elements := make([]*Element, 0, len(ds.Elements))
	for _, elem := range ds.Elements {
		elements = append(elements, elem)
	}
	sortElements(elements)

	cw := &CountingWriter{Writer: w}
	for _, elem := range elements {
		if err := writeElement(cw, elem, 0); err != nil {
			return cw.Count.Load(), fmt.Errorf("writing element %v: %w", elem.Tag, err)
		}
	}
	return cw.Count.Load(), nil
}

// writeElement writes one element in Explicit VR Little Endian. bits is
// BitsAllocated for native pixel data (0 when unknown or not pixel data).
func writeElement(w io.Writer, elem *Element, bits int) error {
	vr := elem.VR
	if len(vr) != 2 {
		slog.Warn("Invalid VR length, defaulting to UN", "vr", vr, "tag", elem.Tag)
		vr = "UN"
	}
	valBytes, undefined, err := encodeValue(elem.Value, vr, bits)
	if err != nil {
		return err
	}

	hdr := make([]byte, 0, 12)
	hdr = binary.LittleEndian.AppendUint16(hdr, elem.Tag.Group)
	hdr = binary.LittleEndian.AppendUint16(hdr, elem.Tag.Element)
	hdr = append(hdr, vr...)
	if isLongVR(vr) {
		length := uint32(undefinedLength)
		if !undefined {
			if int64(len(valBytes)) >= undefinedLength {
				return fmt.Errorf("%w: value too large for element %v (%d bytes)", ErrInvalidValue, elem.Tag, len(valBytes))
			}
			length = uint32(len(valBytes))
		}
		hdr = append(hdr, 0, 0)
		hdr = binary.LittleEndian.AppendUint32(hdr, length)
	} else {
		if undefined {
			return fmt.Errorf("%w: undefined length not supported for short VR %s", ErrInvalidValue, vr)
		}
		if len(valBytes) > math.MaxUint16 {
			return fmt.Errorf("%w: value too large for short-VR element %v (%d bytes, max 65535)",
				ErrInvalidValue, elem.Tag, len(valBytes))
		}
		hdr = binary.LittleEndian.AppendUint16(hdr, uint16(len(valBytes)))
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err = w.Write(valBytes)
	return err
}

// padEven pads b to even length: NUL for UI, space for other string VRs.
func padEven(b []byte, vr string) []byte {
	if len(b)%2 == 0 {
		return b
	}
	if vr == "UI" {
		return append(b, 0)
	}
	return append(b, ' ')
}

func appendU16s(vs []uint16) []byte {
	b := make([]byte, 0, 2*len(vs))
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint16(b, v)
	}
	return b
}

func appendF32s(vs []float32) []byte {
	b := make([]byte, 0, 4*len(vs))
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	return b
}

func formatDS(vs []float64) []byte {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return []byte(strings.Join(parts, `\`))
}

// encodeValue returns the encoded bytes and whether the element uses
// undefined length (sequences and encapsulated pixel data).
func encodeValue(v interface{}, vr string, bits int) ([]byte, bool, error) {
	switch val := v.(type) {
	case nil:
		return []byte{}, false, nil
	case *PixelData:
		if val.IsEncapsulated() {
			b, err := encodeEncapsulatedPixelData(val)
			return b, true, err
		}
		b, err := encodeNativePixelData(val, bits)
		return b, false, err
	case []*Dataset:
		if vr != "SQ" {
			return nil, false, fmt.Errorf("unexpected []*Dataset for VR %s", vr)
		}
		b, err := encodeSequence(val)
		return b, true, err
	case string:
		return padEven([]byte(val), vr), false, nil
	case []string:
		return padEven([]byte(strings.Join(val, `\`)), vr), false, nil
	case uint16:
		return binary.LittleEndian.AppendUint16(nil, val), false, nil
	case []uint16:
		return appendU16s(val), false, nil
	case uint32:
		return binary.LittleEndian.AppendUint32(nil, val), false, nil
	case []uint32:
		b := make([]byte, 0, 4*len(val))
		for _, u := range val {
			b = binary.LittleEndian.AppendUint32(b, u)
		}
		return b, false, nil
	case int16:
		return binary.LittleEndian.AppendUint16(nil, uint16(val)), false, nil
	case int32:
		return binary.LittleEndian.AppendUint32(nil, uint32(val)), false, nil
	case int:
		// Map int to VR size: 4 bytes for UL/SL, otherwise 2.
		switch vr {
		case "UL", "SL":
			return binary.LittleEndian.AppendUint32(nil, uint32(val)), false, nil
		}
		return binary.LittleEndian.AppendUint16(nil, uint16(val)), false, nil
	case float32:
		switch vr {
		case "DS":
			return padEven(formatDS([]float64{float64(val)}), vr), false, nil
		case "FD":
			return binary.LittleEndian.AppendUint64(nil, math.Float64bits(float64(val))), false, nil
		}
		return binary.LittleEndian.AppendUint32(nil, math.Float32bits(val)), false, nil
	case float64:
		switch vr {
		case "DS":
			s := fmt.Sprintf("%v", val)
			return padEven([]byte(s), vr), false, nil
		case "FD":
			return binary.LittleEndian.AppendUint64(nil, math.Float64bits(val)), false, nil
		case "FL":
			return binary.LittleEndian.AppendUint32(nil, math.Float32bits(float32(val))), false, nil
		}
		return nil, false, fmt.Errorf("float64 for VR %s not implemented", vr)
	case []float32:
		return appendF32s(val), false, nil
	case []float64:
		switch vr {
		case "DS":
			return padEven(formatDS(val), vr), false, nil
		case "FL":
			f := make([]float32, len(val))
			for i, x := range val {
				f[i] = float32(x)
			}
			return appendF32s(f), false, nil
		}
		b := make([]byte, 0, 8*len(val))
		for _, x := range val {
			b = binary.LittleEndian.AppendUint64(b, math.Float64bits(x))
		}
		return b, false, nil
	case []byte:
		return val, false, nil
	}
	return nil, false, fmt.Errorf("unsupported value type %T for VR %s", v, vr)
}

func encodeSequence(datasets []*Dataset) ([]byte, error) {
	var buf bytes.Buffer
	for _, ds := range datasets {
		var item bytes.Buffer
		if _, err := writeDataSetBody(&item, ds); err != nil {
			return nil, fmt.Errorf("encoding sequence item: %w", err)
		}
		if int64(item.Len()) >= undefinedLength {
			return nil, fmt.Errorf("%w: sequence item too large (%d bytes)", ErrInvalidValue, item.Len())
		}
		buf.Write([]byte{0xFE, 0xFF, 0x00, 0xE0})
		buf.Write(binary.LittleEndian.AppendUint32(nil, uint32(item.Len())))
		buf.Write(item.Bytes())
	}
	buf.Write([]byte{0xFE, 0xFF, 0xDD, 0xE0, 0, 0, 0, 0})
	return buf.Bytes(), nil
}

// encodeNativePixelData writes native samples per BitsAllocated: 8-bit as
// one byte each (samples must fit, padded to even length), anything else as
// 16-bit little-endian, mirroring how the reader unpacks it.
func encodeNativePixelData(pd *PixelData, bits int) ([]byte, error) {
	total := pd.TotalPixels()
	if bits != 8 {
		b := make([]byte, 0, 2*total)
		for _, f := range pd.Frames {
			for _, s := range f.Data {
				b = binary.LittleEndian.AppendUint16(b, s)
			}
		}
		return b, nil
	}
	b := make([]byte, 0, total+1)
	for _, f := range pd.Frames {
		for _, s := range f.Data {
			if s > math.MaxUint8 {
				return nil, fmt.Errorf("%w: native sample %d does not fit in 8-bit pixel data", ErrInvalidValue, s)
			}
			b = append(b, byte(s))
		}
	}
	if len(b)%2 == 1 {
		b = append(b, 0)
	}
	return b, nil
}

// encodeEncapsulatedPixelData writes one item per frame, padding odd frames
// with 0x00. A non-empty Basic Offset Table is recomputed from the items
// written; an empty one (frame boundaries unknown) stays empty.
func encodeEncapsulatedPixelData(pd *PixelData) ([]byte, error) {
	var items bytes.Buffer
	offsets := make([]uint32, 0, len(pd.Frames))
	var offset uint64
	for _, frame := range pd.Frames {
		if offset > math.MaxUint32 {
			return nil, fmt.Errorf("%w: encapsulated pixel data offsets overflow 32 bits", ErrInvalidValue)
		}
		offsets = append(offsets, uint32(offset))
		body := len(frame.CompressedData) + len(frame.CompressedData)%2
		if int64(body) >= undefinedLength {
			return nil, fmt.Errorf("%w: compressed frame too large (%d bytes)", ErrInvalidValue, body)
		}
		items.Write([]byte{0xFE, 0xFF, 0x00, 0xE0})
		items.Write(binary.LittleEndian.AppendUint32(nil, uint32(body)))
		items.Write(frame.CompressedData)
		if len(frame.CompressedData)%2 == 1 {
			items.WriteByte(0)
		}
		offset += uint64(body) + 8
	}
	if len(pd.Offsets) == 0 {
		offsets = offsets[:0]
	}

	var buf bytes.Buffer
	buf.Grow(16 + 4*len(offsets) + items.Len())
	buf.Write([]byte{0xFE, 0xFF, 0x00, 0xE0})
	buf.Write(binary.LittleEndian.AppendUint32(nil, uint32(4*len(offsets))))
	for _, o := range offsets {
		buf.Write(binary.LittleEndian.AppendUint32(nil, o))
	}
	buf.Write(items.Bytes())
	buf.Write([]byte{0xFE, 0xFF, 0xDD, 0xE0, 0, 0, 0, 0})
	return buf.Bytes(), nil
}

type CountingWriter struct {
	Count  atomic.Int64
	Writer io.Writer
}

func (c *CountingWriter) Write(p []byte) (int, error) {
	n, err := c.Writer.Write(p)
	c.Count.Add(int64(n))
	return n, err
}

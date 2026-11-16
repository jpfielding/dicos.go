package dicos

import "fmt"

// ParseWarningKind identifies a non-fatal issue found while parsing.
type ParseWarningKind int

const (
	// WarnOddPixelDataLength: native pixel data had an odd byte length; the
	// trailing byte was ignored. Length holds the byte length.
	WarnOddPixelDataLength ParseWarningKind = iota + 1
	// WarnPixelCountMismatch: the native pixel count did not match
	// rows*columns*frames; the samples were stored as a single frame.
	// Actual and Expected hold the counts.
	WarnPixelCountMismatch
	// WarnBitsAllocatedMissing: BitsAllocated (0028,0100) was absent, so
	// native pixel data was decoded as 16-bit little-endian samples.
	WarnBitsAllocatedMissing
	// WarnInvalidOffsetTable: the Basic Offset Table did not start at 0, was
	// not strictly increasing, or had an entry with no fragment, so it was
	// ignored and each fragment became one frame. Go-only; the BOT is kept
	// in PixelData.Offsets.
	WarnInvalidOffsetTable
)

// ParseWarning is a non-fatal issue detected while parsing. Retrieve them
// with Reader.Warnings after Reader.ReadDataset.
type ParseWarning struct {
	Kind     ParseWarningKind
	Length   int // WarnOddPixelDataLength
	Actual   int // WarnPixelCountMismatch
	Expected int // WarnPixelCountMismatch
}

func (w ParseWarning) String() string {
	switch w.Kind {
	case WarnOddPixelDataLength:
		return fmt.Sprintf("native pixel data has an odd byte length (%d); trailing byte ignored", w.Length)
	case WarnPixelCountMismatch:
		return fmt.Sprintf("native pixel count mismatch: got %d, expected %d", w.Actual, w.Expected)
	case WarnBitsAllocatedMissing:
		return "BitsAllocated (0028,0100) is absent; native pixel data decoded as 16-bit little-endian samples"
	case WarnInvalidOffsetTable:
		return "basic offset table is not usable; one fragment per frame assumed"
	}
	return fmt.Sprintf("parse warning %d", int(w.Kind))
}

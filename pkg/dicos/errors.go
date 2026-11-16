package dicos

import (
	"errors"
	"fmt"
)

// Sentinel errors returned (usually wrapped) by the reader and writer.
// Test for them with errors.Is; the struct types below carry detail and
// are reachable with errors.As.
var (
	// ErrBadPreamble: the stream is not a DICOM Part-10 file (short
	// preamble or missing "DICM" magic).
	ErrBadPreamble = errors.New("dicos: not a DICOM/DICOS Part-10 file")
	// ErrNestingTooDeep: sequence/item nesting exceeded MaxNestingDepth.
	ErrNestingTooDeep = errors.New("dicos: sequence nesting exceeds maximum depth")
	// ErrUnexpectedTag: a structural tag did not match what the parser
	// required. See UnexpectedTagError.
	ErrUnexpectedTag = errors.New("dicos: unexpected tag")
	// ErrLengthExceedsLimit: a declared length exceeded Reader.MaxElementBytes.
	ErrLengthExceedsLimit = errors.New("dicos: element length exceeds limit")
	// ErrLengthExceedsBuffer: a declared length exceeded the bytes remaining
	// in its enclosing buffer.
	ErrLengthExceedsBuffer = errors.New("dicos: length exceeds remaining buffer")
	// ErrUndefinedLengthInFixedSQ: an undefined-length item appeared inside a
	// fixed-length sequence.
	ErrUndefinedLengthInFixedSQ = errors.New("dicos: undefined-length item inside fixed-length SQ")
	// ErrTruncated: the stream ended inside an element or header. See
	// TruncatedError.
	ErrTruncated = errors.New("dicos: truncated")
	// ErrUnsupportedTransferSyntax: the transfer syntax cannot be read or
	// decoded (big endian, or no codec registered for it).
	ErrUnsupportedTransferSyntax = errors.New("dicos: unsupported transfer syntax")
	// ErrInvalidValue: an attribute value is inconsistent with the data
	// actually present (for example pixel geometry larger than the pixel
	// data element).
	ErrInvalidValue = errors.New("dicos: invalid value")
)

// UnexpectedTagError reports a structural tag that did not match what the
// parser expected. It matches ErrUnexpectedTag with errors.Is.
type UnexpectedTagError struct {
	Expected Tag
	Got      Tag
	Context  string
}

func (e *UnexpectedTagError) Error() string {
	return fmt.Sprintf("dicos: %s: expected tag %v, got %v", e.Context, e.Expected, e.Got)
}

// Is reports whether target is ErrUnexpectedTag.
func (e *UnexpectedTagError) Is(target error) bool { return target == ErrUnexpectedTag }

// LengthError reports a declared length that exceeds either the configured
// limit (Err = ErrLengthExceedsLimit) or the bytes remaining in an enclosing
// buffer (Err = ErrLengthExceedsBuffer).
type LengthError struct {
	Length int64 // declared length
	Max    int64 // limit, or bytes remaining
	Err    error // ErrLengthExceedsLimit or ErrLengthExceedsBuffer
}

func (e *LengthError) Error() string {
	if e.Err == ErrLengthExceedsBuffer {
		return fmt.Sprintf("dicos: length %d exceeds remaining buffer (%d)", e.Length, e.Max)
	}
	return fmt.Sprintf("dicos: element length %d exceeds limit (%d)", e.Length, e.Max)
}

// Unwrap returns the sentinel describing which bound was exceeded.
func (e *LengthError) Unwrap() error { return e.Err }

// TruncatedError reports that the stream ended in the middle of an element
// or header. Offset is the byte offset (from the start of the stream being
// parsed) at which the short read began. It matches ErrTruncated.
type TruncatedError struct {
	Offset  int64
	Context string
}

func (e *TruncatedError) Error() string {
	return fmt.Sprintf("dicos: truncated at byte %d (%s)", e.Offset, e.Context)
}

// Is reports whether target is ErrTruncated.
func (e *TruncatedError) Is(target error) bool { return target == ErrTruncated }

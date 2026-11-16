package dicos

import (
	"encoding/binary"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// Wire-format builders for reader and writer tests. They assemble byte
// streams by hand so tests control the exact on-disk layout, including
// deliberately malformed inputs. Ported from dicos.rs reader/test_util.rs.

// wireElem is one explicit-VR element: tag, VR and raw value bytes.
type wireElem struct {
	Tag   Tag
	VR    string
	Value []byte
}

func u16le(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }
func u32le(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

// padStr returns s padded to even length with a space.
func padStr(s string) []byte {
	b := []byte(s)
	if len(b)%2 != 0 {
		b = append(b, ' ')
	}
	return b
}

func appendTag(buf []byte, t Tag) []byte {
	buf = binary.LittleEndian.AppendUint16(buf, t.Group)
	return binary.LittleEndian.AppendUint16(buf, t.Element)
}

// appendExplicit encodes one element as tag | VR | len | value.
func appendExplicit(buf []byte, e wireElem) []byte {
	buf = appendTag(buf, e.Tag)
	buf = append(buf, e.VR...)
	if isLongVR(e.VR) {
		buf = append(buf, 0, 0)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(e.Value)))
	} else {
		buf = binary.LittleEndian.AppendUint16(buf, uint16(len(e.Value)))
	}
	return append(buf, e.Value...)
}

// appendImplicit encodes one element as tag | 4-byte len | value.
func appendImplicit(buf []byte, t Tag, value []byte) []byte {
	buf = appendTag(buf, t)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(value)))
	return append(buf, value...)
}

// preambleAndMagic returns 128 zero bytes followed by "DICM".
func preambleAndMagic() []byte {
	return append(make([]byte, 128), "DICM"...)
}

// buildWithTS builds a Part-10 stream: group length, the given transfer
// syntax UID, then the dataset elements encoded explicit VR LE.
func buildWithTS(ts string, elems ...wireElem) []byte {
	meta := appendExplicit(nil, wireElem{tag.TransferSyntaxUID, "UI", padStr(ts)})
	buf := preambleAndMagic()
	buf = appendExplicit(buf, wireElem{tag.FileMetaInformationGroupLength, "UL", u32le(uint32(len(meta)))})
	buf = append(buf, meta...)
	for _, e := range elems {
		buf = appendExplicit(buf, e)
	}
	return buf
}

// buildMinimalExplicitVRLE is build_minimal_explicit_vr_le.
func buildMinimalExplicitVRLE(elems ...wireElem) []byte {
	return buildWithTS(string(transfer.ExplicitVRLittleEndian), elems...)
}

// buildMinimalDicosWithTS is build_minimal_dicos_with_ts: one trivial dataset
// element so the reader crosses the meta boundary.
func buildMinimalDicosWithTS(ts string) []byte {
	return buildWithTS(ts, wireElem{tag.Modality, "CS", []byte("CT")})
}

// buildDicosWithoutTransferSyntax is build_dicos_without_transfer_syntax:
// group length 0, then (0028,0010) Rows = 64 in implicit VR LE.
func buildDicosWithoutTransferSyntax() []byte {
	buf := preambleAndMagic()
	buf = appendExplicit(buf, wireElem{tag.FileMetaInformationGroupLength, "UL", u32le(0)})
	return appendImplicit(buf, tag.Rows, u16le(64))
}

// buildSQItem wraps content in a fixed-length item.
func buildSQItem(content []byte) []byte {
	buf := appendTag(nil, tag.Item)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(content)))
	return append(buf, content...)
}

// buildFixedSQ wraps item bytes in a fixed-length SQ element.
func buildFixedSQ(t Tag, items []byte) []byte {
	return appendExplicit(nil, wireElem{t, "SQ", items})
}

// wrapUndefinedLengthSQ wraps inner in one undefined-length SQ holding one
// undefined-length item.
func wrapUndefinedLengthSQ(inner []byte) []byte {
	buf := appendTag(nil, tag.ReferencedImageSequence)
	buf = append(buf, "SQ"...)
	buf = append(buf, 0, 0)
	buf = binary.LittleEndian.AppendUint32(buf, undefinedLength)
	buf = appendTag(buf, tag.Item)
	buf = binary.LittleEndian.AppendUint32(buf, undefinedLength)
	buf = append(buf, inner...)
	buf = appendTag(buf, tag.ItemDelimitationItem)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = appendTag(buf, tag.SequenceDelimitationItem)
	return binary.LittleEndian.AppendUint32(buf, 0)
}

// appendEncapsulatedHeader emits (7FE0,0010) OB with undefined length.
func appendEncapsulatedHeader(buf []byte) []byte {
	buf = appendTag(buf, tag.PixelData)
	buf = append(buf, "OB"...)
	buf = append(buf, 0, 0)
	return binary.LittleEndian.AppendUint32(buf, undefinedLength)
}

// appendBOT emits a Basic Offset Table item.
func appendBOT(buf []byte, offsets ...uint32) []byte {
	buf = appendTag(buf, tag.Item)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(4*len(offsets)))
	for _, o := range offsets {
		buf = binary.LittleEndian.AppendUint32(buf, o)
	}
	return buf
}

// appendFragment emits an encapsulated fragment item.
func appendFragment(buf, data []byte) []byte {
	buf = appendTag(buf, tag.Item)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(data)))
	return append(buf, data...)
}

// appendSeqDelim emits the Sequence Delimitation Item.
func appendSeqDelim(buf []byte) []byte {
	buf = appendTag(buf, tag.SequenceDelimitationItem)
	return binary.LittleEndian.AppendUint32(buf, 0)
}

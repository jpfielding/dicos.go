package dicos

import (
	"bytes"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Writer tests ported from dicos.rs writer.rs.

func el(t Tag, vr string, v interface{}) *Element { return &Element{Tag: t, VR: vr, Value: v} }

func newDS(elems ...*Element) *Dataset {
	ds := &Dataset{Elements: make(map[Tag]*Element)}
	for _, e := range elems {
		ds.Elements[e.Tag] = e
	}
	return ds
}

func writeBytes(t *testing.T, ds *Dataset) []byte {
	t.Helper()
	var buf bytes.Buffer
	n, err := Write(&buf, ds)
	require.NoError(t, err)
	require.Equal(t, int64(buf.Len()), n, "Write must report every byte written")
	return buf.Bytes()
}

func roundtrip(t *testing.T, ds *Dataset) *Dataset {
	t.Helper()
	rt, err := parseBytes(writeBytes(t, ds))
	require.NoError(t, err)
	return rt
}

func str(t *testing.T, ds *Dataset, tg Tag) string {
	t.Helper()
	e, ok := ds.Elements[tg]
	require.True(t, ok, "missing %v", tg)
	s, ok := e.GetString()
	require.True(t, ok, "%v is %T", tg, e.Value)
	return s
}

func pixelDataOf(t *testing.T, ds *Dataset) *PixelData {
	t.Helper()
	e, ok := ds.Elements[tag.PixelData]
	require.True(t, ok)
	pd, ok := e.Value.(*PixelData)
	require.True(t, ok, "pixel data is %T", e.Value)
	return pd
}

func encapsulated(offsets []uint32, frames ...[]byte) *PixelData {
	pd := &PixelData{Offsets: offsets}
	for _, f := range frames {
		pd.Frames = append(pd.Frames, Frame{CompressedData: f})
	}
	return pd
}

func native(frames ...[]uint16) *PixelData {
	pd := &PixelData{}
	for _, f := range frames {
		pd.Frames = append(pd.Frames, Frame{Data: f})
	}
	return pd
}

var (
	tsExplicit = el(tag.TransferSyntaxUID, "UI", string(transfer.ExplicitVRLittleEndian))
	tsJPEGLS   = el(tag.TransferSyntaxUID, "UI", string(transfer.JPEGLSLossless))
)

func TestWrite_InjectsTransferSyntaxWhenMissing(t *testing.T) {
	rt := roundtrip(t, newDS(el(tag.PatientName, "PN", "DOE^ALICE")))
	assert.Equal(t, string(transfer.ExplicitVRLittleEndian), str(t, rt, tag.TransferSyntaxUID))
}

func TestWrite_RecomputesMetaGroupLength(t *testing.T) {
	ds := newDS(tsExplicit,
		el(tag.FileMetaInformationGroupLength, "UL", uint32(9999)), // stale
		el(tag.MediaStorageSOPClassUID, "UI", "1.2.3.4"),
		el(tag.PatientName, "PN", "META^TEST"))
	data := writeBytes(t, ds)
	rt, err := parseBytes(data)
	require.NoError(t, err)
	gl, ok := rt.Elements[tag.FileMetaInformationGroupLength].Value.(uint32)
	require.True(t, ok)
	// Group 0002 after the 12-byte group length element: (0002,0002) UI
	// "1.2.3.4\0" = 8+8, (0002,0010) UI 20 bytes = 8+20.
	assert.Equal(t, uint32(16+28), gl)
	assert.Equal(t, "META^TEST", str(t, rt, tag.PatientName))
}

func TestWrite_RoundtripStringElements(t *testing.T) {
	rt := roundtrip(t, newDS(tsExplicit,
		el(tag.PatientName, "PN", "SMITH^ALICE"),
		el(tag.PatientID, "LO", "12345"),
		el(tag.Modality, "CS", "DX"),
		el(tag.StudyDescription, "LO", "Baggage Scan")))
	assert.Equal(t, "SMITH^ALICE", str(t, rt, tag.PatientName))
	assert.Equal(t, "12345", str(t, rt, tag.PatientID))
	assert.Equal(t, "DX", rt.Modality())
	assert.Equal(t, "Baggage Scan", str(t, rt, tag.StudyDescription))
}

func TestWrite_RoundtripNumericElements(t *testing.T) {
	rt := roundtrip(t, newDS(tsExplicit,
		el(tag.Rows, "US", uint16(512)),
		el(tag.Columns, "US", uint16(256)),
		el(tag.BitsAllocated, "US", uint16(16)),
		el(tag.BitsStored, "US", uint16(12)),
		el(tag.HighBit, "US", uint16(11)),
		el(tag.PixelRepresentation, "US", uint16(0))))
	assert.Equal(t, 512, rt.Rows())
	assert.Equal(t, 256, rt.Columns())
	assert.Equal(t, 16, rt.BitsAllocated())
	assert.Equal(t, uint16(12), rt.Elements[tag.BitsStored].Value)
	assert.Equal(t, 0, rt.PixelRepresentation())
}

// Row 22: every value type the reader produces must be writable.
func TestWrite_ParsedNumericTypesAreWritable(t *testing.T) {
	ds := newDS(tsExplicit,
		el(Tag{Group: 0x0009, Element: 0x1001}, "UL", uint32(7)),
		el(Tag{Group: 0x0009, Element: 0x1002}, "UL", []uint32{1, 2}),
		el(Tag{Group: 0x0009, Element: 0x1003}, "SS", int16(-3)),
		el(Tag{Group: 0x0009, Element: 0x1004}, "SL", int32(-4)),
		el(Tag{Group: 0x0009, Element: 0x1005}, "FL", float32(1.5)),
		el(Tag{Group: 0x0009, Element: 0x1006}, "FD", []float64{1, 2}),
		el(Tag{Group: 0x0009, Element: 0x1007}, "FL", []float32{3, 4}),
		el(Tag{Group: 0x0009, Element: 0x1008}, "FD", 2.5),
	)
	rt := roundtrip(t, ds)
	for tg, e := range ds.Elements {
		if tg.Group == 0x0009 {
			assert.Equal(t, e, rt.Elements[tg], "%v", tg)
		}
	}
}

func TestWrite_RoundtripNativePixelData(t *testing.T) {
	rt := roundtrip(t, newDS(tsExplicit,
		el(tag.Rows, "US", uint16(2)),
		el(tag.Columns, "US", uint16(2)),
		el(tag.BitsAllocated, "US", uint16(16)),
		el(tag.PixelData, "OW", []byte{100, 0, 200, 0, 0x2C, 1, 0x90, 1})))
	pd := pixelDataOf(t, rt)
	require.Len(t, pd.Frames, 1)
	assert.Equal(t, []uint16{100, 200, 300, 400}, pd.Frames[0].Data)
}

func TestWrite_RoundtripEncapsulatedPixelData(t *testing.T) {
	frame := []byte{0xFF, 0xD8, 0x00, 0x01, 0x02, 0x03}
	rt := roundtrip(t, newDS(tsExplicit,
		el(tag.Rows, "US", uint16(4)),
		el(tag.Columns, "US", uint16(4)),
		el(tag.PixelData, "OB", encapsulated([]uint32{0}, frame))))
	pd := pixelDataOf(t, rt)
	assert.True(t, pd.IsEncapsulated())
	require.Len(t, pd.Frames, 1)
	assert.Equal(t, frame, pd.Frames[0].CompressedData)
	assert.Equal(t, []uint32{0}, pd.Offsets)
}

func TestWrite_RoundtripSignedValues(t *testing.T) {
	rt := roundtrip(t, newDS(tsExplicit, el(tag.RescaleIntercept, "DS", "-1024")))
	intercept, _ := GetRescaleExplicit(rt)
	assert.Equal(t, -1024.0, intercept)
}

func TestWrite_ProducesValidPreambleAndMagic(t *testing.T) {
	data := writeBytes(t, newDS(tsExplicit))
	assert.Equal(t, make([]byte, 128), data[:128])
	assert.Equal(t, []byte("DICM"), data[128:132])
}

func TestWrite_RoundtripMultipleFrames(t *testing.T) {
	rt := roundtrip(t, newDS(tsExplicit,
		el(tag.PixelData, "OB", encapsulated([]uint32{0, 12}, []byte{1, 2, 3, 4}, []byte{5, 6, 7, 8}))))
	pd := pixelDataOf(t, rt)
	require.Len(t, pd.Frames, 2)
	assert.Equal(t, []byte{1, 2, 3, 4}, pd.Frames[0].CompressedData)
	assert.Equal(t, []byte{5, 6, 7, 8}, pd.Frames[1].CompressedData)
}

func TestWrite_Roundtrip8BitNativePixels(t *testing.T) {
	ds := newDS(
		el(tag.Rows, "US", uint16(1)),
		el(tag.Columns, "US", uint16(2)),
		el(tag.BitsAllocated, "US", uint16(8)),
		el(tag.PixelData, "OB", []byte{10, 20}))
	first := roundtrip(t, ds)
	assert.Equal(t, native([]uint16{10, 20}), pixelDataOf(t, first))
	assert.Equal(t, 8, first.BitsAllocated())

	second := roundtrip(t, first)
	assert.Equal(t, pixelDataOf(t, first), pixelDataOf(t, second))
	assert.Equal(t, 8, second.BitsAllocated())
}

func TestWrite_Roundtrip16BitNativePixelData(t *testing.T) {
	rt := roundtrip(t, newDS(
		el(tag.Rows, "US", uint16(2)),
		el(tag.Columns, "US", uint16(2)),
		el(tag.BitsAllocated, "US", uint16(16)),
		el(tag.PixelData, "OW", native([]uint16{100, 200, 300, 400}))))
	assert.Equal(t, native([]uint16{100, 200, 300, 400}), pixelDataOf(t, rt))
}

func TestWrite_Roundtrip8BitOddLengthStripsPadding(t *testing.T) {
	ds := newDS(
		el(tag.Rows, "US", uint16(1)),
		el(tag.Columns, "US", uint16(1)),
		el(tag.BitsAllocated, "US", uint16(8)),
		el(tag.PixelData, "OB", []byte{10, 0}))
	first := roundtrip(t, ds)
	assert.Equal(t, native([]uint16{10}), pixelDataOf(t, first))
	second := roundtrip(t, first)
	assert.Equal(t, pixelDataOf(t, first), pixelDataOf(t, second))
}

func TestWrite_8BitRejectsSamplesThatDoNotFit(t *testing.T) {
	ds := newDS(
		el(tag.Rows, "US", uint16(1)),
		el(tag.Columns, "US", uint16(1)),
		el(tag.BitsAllocated, "US", uint16(8)),
		el(tag.PixelData, "OW", native([]uint16{256})))
	_, err := Write(&bytes.Buffer{}, ds)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidValue)
	assert.Contains(t, err.Error(), "256")
	assert.Contains(t, err.Error(), "8-bit")
}

func TestWrite_ImplicitVRNormalizesToExplicitAndRoundtrips(t *testing.T) {
	rt := roundtrip(t, newDS(
		el(tag.TransferSyntaxUID, "UI", string(transfer.ImplicitVRLittleEndian)),
		el(tag.Rows, "US", uint16(1))))
	assert.Equal(t, string(transfer.ExplicitVRLittleEndian), str(t, rt, tag.TransferSyntaxUID))
	assert.Equal(t, 1, rt.Rows())
}

func TestWrite_CompressedTransferSyntaxIsPreserved(t *testing.T) {
	rt := roundtrip(t, newDS(tsJPEGLS, el(tag.Rows, "US", uint16(1))))
	assert.Equal(t, string(transfer.JPEGLSLossless), str(t, rt, tag.TransferSyntaxUID))
	assert.Equal(t, 1, rt.Rows())
}

func TestWrite_MultiFragmentRoundtripRecomputesBOT(t *testing.T) {
	rt := roundtrip(t, newDS(tsJPEGLS,
		el(tag.PixelData, "OB", encapsulated([]uint32{0, 20}, []byte{1, 2, 3, 4}, []byte{5, 6}))))
	pd := pixelDataOf(t, rt)
	assert.Equal(t, encapsulated([]uint32{0, 12}, []byte{1, 2, 3, 4}, []byte{5, 6}), pd)

	again := roundtrip(t, rt)
	assert.Equal(t, pd, pixelDataOf(t, again))
}

func TestWrite_EncapsulatedOddLengthFramePadsAndKeepsBoundaries(t *testing.T) {
	rt := roundtrip(t, newDS(tsJPEGLS,
		el(tag.PixelData, "OB", encapsulated([]uint32{0, 99}, []byte{1, 2, 3}, []byte{4, 5}))))
	assert.Equal(t, encapsulated([]uint32{0, 12}, []byte{1, 2, 3, 0}, []byte{4, 5}), pixelDataOf(t, rt))
}

func TestWrite_EmptyBOTIsPreservedOnRewrite(t *testing.T) {
	rt := roundtrip(t, newDS(tsJPEGLS,
		el(tag.NumberOfFrames, "IS", "1"),
		el(tag.PixelData, "OB", encapsulated(nil, []byte{1, 2}, []byte{3, 4}))))
	pd := pixelDataOf(t, rt)
	assert.Equal(t, [][]byte{{1, 2}, {3, 4}}, [][]byte{pd.Frames[0].CompressedData, pd.Frames[1].CompressedData})
	assert.Empty(t, pd.Offsets, "BOT must stay empty")
	again := roundtrip(t, rt)
	assert.Equal(t, pd, pixelDataOf(t, again))
}

func TestWrite_RoundtripNativePixelsWithNon8BitAllocation(t *testing.T) {
	for _, bits := range []uint16{1, 32} {
		ds := newDS(
			el(tag.Rows, "US", uint16(1)),
			el(tag.Columns, "US", uint16(1)),
			el(tag.BitsAllocated, "US", bits),
			el(tag.PixelData, "OW", native([]uint16{513, 1027})))
		first := writeBytes(t, ds)
		rt, err := parseBytes(first)
		require.NoError(t, err)
		assert.Equal(t, native([]uint16{513, 1027}), pixelDataOf(t, rt), "BitsAllocated %d", bits)
		assert.Equal(t, first, writeBytes(t, rt), "BitsAllocated %d: bytes changed on rewrite", bits)
	}
}

func TestWrite_ImplicitVRUIDWithNULPaddingIsNormalized(t *testing.T) {
	rt := roundtrip(t, newDS(
		el(tag.TransferSyntaxUID, "UI", string(transfer.ImplicitVRLittleEndian)+"\x00"),
		el(tag.Rows, "US", uint16(1))))
	assert.Equal(t, string(transfer.ExplicitVRLittleEndian), str(t, rt, tag.TransferSyntaxUID))
	assert.Equal(t, 1, rt.Rows())
}

// Row 28: UI values are padded with NUL, other strings with a space.
func TestWrite_UIPaddingIsNUL(t *testing.T) {
	data := writeBytes(t, newDS(tsExplicit,
		el(tag.SOPClassUID, "UI", "1.2.3"),
		el(tag.Modality, "CS", "CTX")))
	assert.True(t, bytes.Contains(data, []byte("1.2.3\x00")))
	assert.True(t, bytes.Contains(data, []byte("CTX ")))
}

// Row 25: lengths that do not fit the header are errors, not truncations.
func TestWrite_ShortVRLengthOverflowIsError(t *testing.T) {
	ds := newDS(tsExplicit, el(tag.PatientName, "PN", string(make([]byte, 70000))))
	_, err := Write(&bytes.Buffer{}, ds)
	assert.ErrorIs(t, err, ErrInvalidValue)
}

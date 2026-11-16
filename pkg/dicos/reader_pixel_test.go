package dicos

import (
	"bytes"
	"image"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pixel data tests ported from dicos.rs reader/tests.rs, plus allocation
// regressions for the declared-geometry bombs.

func parseWithWarnings(t *testing.T, data []byte) (*Dataset, []ParseWarning) {
	t.Helper()
	r := NewReader(bytes.NewReader(data))
	r.MaxElementBytes = 1 << 20
	ds, err := r.ReadDataset()
	require.NoError(t, err)
	return ds, r.Warnings()
}

func parsedPixelData(t *testing.T, ds *Dataset) *PixelData {
	t.Helper()
	pd, ok := ds.Elements[tag.PixelData].Value.(*PixelData)
	require.True(t, ok, "parsed pixel data must be *PixelData, got %T", ds.Elements[tag.PixelData].Value)
	return pd
}

func TestParse_WithNativePixelData(t *testing.T) {
	ds, err := parseBytes(buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
		wireElem{tag.PixelData, "OW", []byte{100, 0, 200, 0, 0x2C, 1, 0x90, 1}},
	))
	require.NoError(t, err)
	pd := parsedPixelData(t, ds)
	require.Len(t, pd.Frames, 1)
	assert.Equal(t, []uint16{100, 200, 300, 400}, pd.Frames[0].Data)
	assert.False(t, pd.IsEncapsulated())
}

func TestParse_WarningsReportOddPixelLength(t *testing.T) {
	ds, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0, 3}},
	))
	parsedPixelData(t, ds)
	assert.Contains(t, warnings, ParseWarning{Kind: WarnOddPixelDataLength, Length: 5})
}

func TestParse_WarningsReportPixelCountMismatch(t *testing.T) {
	_, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0}},
	))
	assert.Contains(t, warnings, ParseWarning{Kind: WarnPixelCountMismatch, Actual: 2, Expected: 4})
}

func TestParse_CleanFileHasNoWarnings(t *testing.T) {
	ds, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0, 3, 0, 4, 0}},
	))
	assert.Empty(t, warnings)
	assert.Equal(t, []uint16{1, 2, 3, 4}, parsedPixelData(t, ds).Frames[0].Data)
}

func TestParse_Native8BitIsOneSamplePerByte(t *testing.T) {
	ds, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.BitsAllocated, "US", u16le(8)},
		wireElem{tag.PixelData, "OB", []byte{10, 20, 30, 40}},
	))
	assert.Empty(t, warnings)
	assert.Equal(t, []uint16{10, 20, 30, 40}, parsedPixelData(t, ds).Frames[0].Data)
}

func TestParse_Native8BitOddSizeDropsPadByte(t *testing.T) {
	ds, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(1)},
		wireElem{tag.Columns, "US", u16le(3)},
		wireElem{tag.BitsAllocated, "US", u16le(8)},
		wireElem{tag.PixelData, "OB", []byte{7, 8, 9, 0}},
	))
	assert.Empty(t, warnings)
	assert.Equal(t, []uint16{7, 8, 9}, parsedPixelData(t, ds).Frames[0].Data)
}

func TestParse_NativeMissingBitsAllocatedWarns(t *testing.T) {
	ds, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0, 3, 0, 4, 0}},
	))
	assert.Contains(t, warnings, ParseWarning{Kind: WarnBitsAllocatedMissing})
	assert.Equal(t, []uint16{1, 2, 3, 4}, parsedPixelData(t, ds).Frames[0].Data)
}

func TestParse_NativeMultiFrameIsSplit(t *testing.T) {
	ds, warnings := parseWithWarnings(t, buildMinimalExplicitVRLE(
		wireElem{tag.NumberOfFrames, "IS", padStr("2")},
		wireElem{tag.Rows, "US", u16le(1)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0, 3, 0, 4, 0}},
	))
	assert.Empty(t, warnings)
	pd := parsedPixelData(t, ds)
	require.Len(t, pd.Frames, 2)
	assert.Equal(t, []uint16{1, 2}, pd.Frames[0].Data)
	assert.Equal(t, []uint16{3, 4}, pd.Frames[1].Data)
}

func TestParse_EncapsulatedMultiFragmentFrameUsesOffsetTable(t *testing.T) {
	frag0 := []byte{0xAA, 0xBB}
	frag1 := []byte{0xCC, 0xDD, 0xEE, 0xFF}
	frag2 := []byte{0x11, 0x22}
	frame1Offset := uint32(8+len(frag0)) + uint32(8+len(frag1))

	data := buildMinimalExplicitVRLE(wireElem{tag.Rows, "US", u16le(2)}, wireElem{tag.Columns, "US", u16le(2)})
	data = appendEncapsulatedHeader(data)
	data = appendBOT(data, 0, frame1Offset)
	data = appendFragment(data, frag0)
	data = appendFragment(data, frag1)
	data = appendFragment(data, frag2)
	data = appendSeqDelim(data)

	ds, warnings := parseWithWarnings(t, data)
	assert.Empty(t, warnings)
	pd := parsedPixelData(t, ds)
	require.Len(t, pd.Frames, 2)
	assert.Equal(t, append(append([]byte{}, frag0...), frag1...), pd.Frames[0].CompressedData)
	assert.Equal(t, frag2, pd.Frames[1].CompressedData)
	assert.Equal(t, []uint32{0, frame1Offset}, pd.Offsets)
}

func TestParse_EncapsulatedEmptyBOTIsOneFragmentPerFrame(t *testing.T) {
	data := buildMinimalExplicitVRLE(wireElem{tag.Rows, "US", u16le(2)}, wireElem{tag.Columns, "US", u16le(2)})
	data = appendEncapsulatedHeader(data)
	data = appendBOT(data)
	data = appendFragment(data, []byte{1, 2})
	data = appendFragment(data, []byte{3, 4})
	data = appendSeqDelim(data)

	ds, _ := parseWithWarnings(t, data)
	pd := parsedPixelData(t, ds)
	require.Len(t, pd.Frames, 2)
	assert.Equal(t, []byte{1, 2}, pd.Frames[0].CompressedData)
	assert.Equal(t, []byte{3, 4}, pd.Frames[1].CompressedData)
}

// Go-only: dicos.rs honours overlapping BOT windows, so a BOT that repeats
// [0, max) copies every fragment once per entry. Such a BOT is ignored here.
func TestParse_EncapsulatedNonIncreasingBOTIsIgnored(t *testing.T) {
	data := buildMinimalExplicitVRLE(wireElem{tag.Rows, "US", u16le(2)}, wireElem{tag.Columns, "US", u16le(2)})
	data = appendEncapsulatedHeader(data)
	data = appendBOT(data, 0, 0xFFFFFFFF, 0, 0xFFFFFFFF)
	data = appendFragment(data, []byte{1, 2})
	data = appendFragment(data, []byte{3, 4})
	data = appendSeqDelim(data)

	ds, warnings := parseWithWarnings(t, data)
	assert.Contains(t, warnings, ParseWarning{Kind: WarnInvalidOffsetTable})
	pd := parsedPixelData(t, ds)
	require.Len(t, pd.Frames, 2)
	assert.Equal(t, []byte{1, 2}, pd.Frames[0].CompressedData)
	assert.Equal(t, []byte{3, 4}, pd.Frames[1].CompressedData)
}

// Hostile geometry with a 4-byte pixel element: parsing succeeds with a
// count warning, and every volume or lazy path errors before allocating
// anything sized by the declared geometry. Runs under the memory cap.
func TestPixelData_DeclaredGeometryBombsAreRejected(t *testing.T) {
	tests := []struct {
		name   string
		rows   uint16
		cols   uint16
		frames string
	}{
		{"negative frames", 2, 2, "-1"},
		{"huge frames", 2, 2, "2000000000"},
		{"huge frame", 65535, 65535, "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elems := []wireElem{
				{tag.NumberOfFrames, "IS", padStr(tt.frames)},
				{tag.Rows, "US", u16le(tt.rows)},
				{tag.Columns, "US", u16le(tt.cols)},
				{tag.BitsAllocated, "US", u16le(16)},
			}
			data := buildMinimalExplicitVRLE(append(elems, wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0}})...)

			ds, warnings := parseWithWarnings(t, data)
			assert.NotEmpty(t, warnings)

			_, err := DecodeVolume(ds)
			assert.ErrorIs(t, err, ErrInvalidValue)
			_, err = VolumeFromDataset(ds)
			assert.ErrorIs(t, err, ErrInvalidValue)

			// Lazy path: the same geometry on a hand-built dataset.
			raw, err := parseBytes(buildMinimalExplicitVRLE(elems...))
			require.NoError(t, err)
			raw.Elements[tag.PixelData] = &Element{Tag: tag.PixelData, VR: "OW", Value: []byte{1, 0, 2, 0}}
			_, err = raw.GetPixelData()
			assert.ErrorIs(t, err, ErrInvalidValue)
			raw.Elements[tag.PixelData].Value = []uint16{1, 2}
			_, err = raw.GetPixelData()
			assert.ErrorIs(t, err, ErrInvalidValue)
		})
	}
}

func TestDecodeVolume_EncapsulatedDimensionMismatchIsError(t *testing.T) {
	img := image.NewGray16(image.Rect(0, 0, 2, 2))
	var enc bytes.Buffer
	require.NoError(t, CodecJPEGLS.Encode(&enc, img))
	frame := enc.Bytes()
	if len(frame)%2 != 0 {
		frame = append(frame, 0)
	}

	data := buildWithTS(CodecJPEGLS.TransferSyntaxUID(),
		wireElem{tag.Rows, "US", u16le(4)},
		wireElem{tag.Columns, "US", u16le(4)},
	)
	data = appendEncapsulatedHeader(data)
	data = appendBOT(data)
	data = appendFragment(data, frame)
	data = appendSeqDelim(data)

	ds, err := parseBytes(data)
	require.NoError(t, err)
	_, err = DecodeVolume(ds)
	assert.ErrorIs(t, err, ErrDimensionMismatch)
}

func TestDecodeVolume_NativeMultiFrame(t *testing.T) {
	ds, err := parseBytes(buildMinimalExplicitVRLE(
		wireElem{tag.NumberOfFrames, "IS", padStr("2")},
		wireElem{tag.Rows, "US", u16le(1)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0, 3, 0, 4, 0}},
	))
	require.NoError(t, err)
	vol, err := DecodeVolume(ds)
	require.NoError(t, err)
	assert.Equal(t, 2, vol.Depth)
	assert.Equal(t, []uint16{1, 2, 3, 4}, vol.Data)
}

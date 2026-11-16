package dicos

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sequence and value-table tests ported from dicos.rs reader/tests.rs.

func TestParse_NumericTypes(t *testing.T) {
	data := buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(1024)},
		wireElem{tag.RescaleIntercept, "DS", padStr("-1024")},
		wireElem{tag.RescaleSlope, "DS", padStr("1.0")},
	)
	ds, err := parseBytes(data)
	require.NoError(t, err)
	assert.Equal(t, 1024, ds.Rows())
	intercept, slope := GetRescaleExplicit(ds)
	assert.Equal(t, -1024.0, intercept)
	assert.Equal(t, 1.0, slope)
}

// Q5: multi-valued strings stay a single backslash-separated string.
func TestParse_MultiValueStringStaysJoined(t *testing.T) {
	data := buildMinimalExplicitVRLE(wireElem{tag.ImageType, "CS", padStr(`ORIGINAL\PRIMARY`)})
	ds, err := parseBytes(data)
	require.NoError(t, err)
	s, ok := ds.Elements[tag.ImageType].GetString()
	require.True(t, ok)
	assert.Equal(t, `ORIGINAL\PRIMARY`, s)
}

func TestParseValue_Table(t *testing.T) {
	f32 := func(vs ...float32) []byte {
		var b []byte
		for _, v := range vs {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
		}
		return b
	}
	f64 := func(vs ...float64) []byte {
		var b []byte
		for _, v := range vs {
			b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
		}
		return b
	}
	tests := []struct {
		name string
		vr   string
		data []byte
		want interface{}
	}{
		{"AE string", "AE", []byte("NODE"), "NODE"},
		{"UC string", "UC", []byte("abc "), "abc"},
		{"UR string", "UR", []byte("http://x\x00"), "http://x"},
		{"UI nul pad", "UI", []byte("1.2.3\x00"), "1.2.3"},
		{"US single", "US", u16le(7), uint16(7)},
		{"US multi", "US", append(u16le(1), u16le(2)...), []uint16{1, 2}},
		{"US odd length is bytes", "US", []byte{1, 0, 2}, []byte{1, 0, 2}},
		{"UL single", "UL", u32le(9), uint32(9)},
		{"UL multi", "UL", append(u32le(1), u32le(2)...), []uint32{1, 2}},
		{"UL wrong size is bytes", "UL", []byte{1, 2, 3}, []byte{1, 2, 3}},
		{"SS", "SS", u16le(0xFFFF), int16(-1)},
		{"SL", "SL", u32le(0xFFFFFFFE), int32(-2)},
		{"SS wrong size is bytes", "SS", []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4}},
		{"FL single", "FL", f32(1.5), float32(1.5)},
		{"FL multi", "FL", f32(1, 2, 3), []float32{1, 2, 3}},
		{"FD single", "FD", f64(2.5), 2.5},
		{"FD multi", "FD", f64(1, 2), []float64{1, 2}},
		{"FD wrong size is bytes", "FD", []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4}},
		{"AT bytes", "AT", []byte{0x10, 0, 0x10, 0}, []byte{0x10, 0, 0x10, 0}},
		{"OB bytes", "OB", []byte{1, 2}, []byte{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseValue(tt.vr, tt.data))
		})
	}
}

func TestParse_UnknownVRIsReadAsUN(t *testing.T) {
	data := buildMinimalExplicitVRLE()
	data = appendTag(data, Tag{Group: 0x0009, Element: 0x1001})
	data = append(data, "ZZ"...)
	data = append(data, 0, 0)
	data = append(data, u32le(2)...)
	data = append(data, 0xAB, 0xCD)
	data = appendExplicit(data, wireElem{tag.Rows, "US", u16le(5)})

	ds, err := parseBytes(data)
	require.NoError(t, err)
	el := ds.Elements[Tag{Group: 0x0009, Element: 0x1001}]
	require.NotNil(t, el)
	assert.Equal(t, "UN", el.VR)
	assert.Equal(t, []byte{0xAB, 0xCD}, el.Value)
	assert.Equal(t, 5, ds.Rows())
}

func sequenceItems(t *testing.T, ds *Dataset, sq Tag) []*Dataset {
	t.Helper()
	el, ok := ds.Elements[sq]
	require.True(t, ok, "SQ element must be present")
	items, ok := el.Value.([]*Dataset)
	require.True(t, ok, "expected []*Dataset, got %T", el.Value)
	return items
}

func TestParse_FixedLengthSQValidItem(t *testing.T) {
	inner := appendExplicit(nil, wireElem{tag.Rows, "US", u16le(128)})
	file := append(buildMinimalExplicitVRLE(), buildFixedSQ(tag.ReferencedImageSequence, buildSQItem(inner))...)

	ds, err := parseBytes(file)
	require.NoError(t, err)
	items := sequenceItems(t, ds, tag.ReferencedImageSequence)
	require.Len(t, items, 1)
	assert.Equal(t, 128, items[0].Rows())
}

func TestParse_FixedLengthSQItemLengthExceedsBufferReturnsError(t *testing.T) {
	items := appendTag(nil, tag.Item)
	items = append(items, u32le(64)...)
	items = append(items, 0x00, 0x08, 0x60, 0x00) // only 4 bytes of 64
	file := append(buildMinimalExplicitVRLE(), buildFixedSQ(tag.ReferencedImageSequence, items)...)

	_, err := parseBytes(file)
	var le *LengthError
	require.ErrorAs(t, err, &le)
	assert.ErrorIs(t, err, ErrLengthExceedsBuffer)
	assert.Equal(t, int64(64), le.Length)
	assert.Equal(t, int64(4), le.Max)
}

func TestParse_FixedLengthSQEmptyItemIsValid(t *testing.T) {
	file := append(buildMinimalExplicitVRLE(), buildFixedSQ(tag.ReferencedImageSequence, buildSQItem(nil))...)
	ds, err := parseBytes(file)
	require.NoError(t, err)
	items := sequenceItems(t, ds, tag.ReferencedImageSequence)
	require.Len(t, items, 1)
	assert.Empty(t, items[0].Elements)
}

func TestParse_FixedLengthSQRejectsUndefinedLengthItem(t *testing.T) {
	items := appendTag(nil, tag.Item)
	items = append(items, u32le(undefinedLength)...)
	file := append(buildMinimalExplicitVRLE(), buildFixedSQ(tag.ReferencedImageSequence, items)...)
	_, err := parseBytes(file)
	assert.ErrorIs(t, err, ErrUndefinedLengthInFixedSQ)
}

func TestParse_FixedLengthSQRejectsNonItemTag(t *testing.T) {
	items := appendTag(nil, tag.Rows)
	items = append(items, u32le(0)...)
	file := append(buildMinimalExplicitVRLE(), buildFixedSQ(tag.ReferencedImageSequence, items)...)
	_, err := parseBytes(file)
	assert.ErrorIs(t, err, ErrUnexpectedTag)
}

func TestParse_UndefinedLengthSQWithItems(t *testing.T) {
	inner := appendExplicit(nil, wireElem{tag.Columns, "US", u16le(33)})
	file := append(buildMinimalExplicitVRLE(), wrapUndefinedLengthSQ(inner)...)
	file = appendExplicit(file, wireElem{tag.Rows, "US", u16le(9)})

	ds, err := parseBytes(file)
	require.NoError(t, err)
	items := sequenceItems(t, ds, tag.ReferencedImageSequence)
	require.Len(t, items, 1)
	assert.Equal(t, 33, items[0].Columns())
	assert.Equal(t, 9, ds.Rows(), "parsing resumes after the sequence delimiter")
}

func TestParse_RejectsExcessiveSequenceNesting(t *testing.T) {
	var payload []byte
	for range MaxNestingDepth + 16 {
		payload = wrapUndefinedLengthSQ(payload)
	}
	_, err := parseBytes(append(buildMinimalExplicitVRLE(), payload...))
	assert.ErrorIs(t, err, ErrNestingTooDeep)
}

func TestParse_AcceptsNestingWithinLimit(t *testing.T) {
	var payload []byte
	for range MaxNestingDepth - 1 {
		payload = wrapUndefinedLengthSQ(payload)
	}
	ds, err := parseBytes(append(buildMinimalExplicitVRLE(), payload...))
	require.NoError(t, err)
	assert.Contains(t, ds.Elements, tag.ReferencedImageSequence)
}

// A TDR written by this library (undefined-length SQ) must read back with
// its sequences intact.
func TestParse_TDRSequencesRoundTrip(t *testing.T) {
	tdr := NewThreatDetectionReport()
	tdr.Patient.PatientID = "BAG-001"
	tdr.AlarmDecision = "ALARM"
	tdr.ReferencedSOPClassUID = DICOSCTImageStorageUID
	tdr.ReferencedSOPInstanceUID = "1.2.3.4.5"
	tdr.PTOs = []PotentialThreatObject{
		{ID: 1, Label: "KNIFE", OOIType: "KNIFE", BoundingBox: &BoundingBox{
			TopLeft: [3]float32{1, 2, 3}, BottomRight: [3]float32{4, 5, 6}}},
		{ID: 2, Label: "LIQUID"},
	}
	var buf bytes.Buffer
	_, err := tdr.WriteTo(&buf)
	require.NoError(t, err)

	ds, err := Parse(&buf)
	require.NoError(t, err)

	// GetVR has no dictionary entry for several TDR tags, so the builder
	// writes them as UN; they read back as their raw (padded) bytes.
	refs := sequenceItems(t, ds, tag.ReferencedImageSequence)
	require.Len(t, refs, 1)
	assert.Equal(t, padStr("1.2.3.4.5"), refs[0].Elements[tag.ReferencedSOPInstanceUID].Value)

	ptos := sequenceItems(t, ds, tag.PTOSequence)
	require.Len(t, ptos, 2)
	assert.Equal(t, padStr("KNIFE"), ptos[0].Elements[tag.ThreatCategoryDescription].Value)
	assert.Equal(t, padStr("LIQUID"), ptos[1].Elements[tag.ThreatCategoryDescription].Value)
	assert.NotContains(t, ptos[1].Elements, tag.PTORepresentationSequence)

	reps := sequenceItems(t, ptos[0], tag.PTORepresentationSequence)
	require.Len(t, reps, 1)
	tl, ok := reps[0].Elements[tag.BoundingBoxTopLeft].Value.([]byte)
	require.True(t, ok)
	assert.Equal(t, []float32{1, 2, 3}, []float32{
		math.Float32frombits(binary.LittleEndian.Uint32(tl[0:])),
		math.Float32frombits(binary.LittleEndian.Uint32(tl[4:])),
		math.Float32frombits(binary.LittleEndian.Uint32(tl[8:])),
	})
}

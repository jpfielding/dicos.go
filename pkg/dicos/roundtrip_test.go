package dicos

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Round-trip properties ported from dicos.rs tests/roundtrip_prop.rs as
// seeded table loops. Datasets are generated in already-normalised form
// (no trailing padding; multi-valued strings stay joined, Q5), so the
// properties are:
//
//  1. every element survives one write+parse unchanged;
//  2. parse(write(.)) is a fixed point;
//  3. rewriting a normalised dataset is byte stable;
//  4. multi-frame native pixel values are preserved.

const roundtripCases = 100

func randString(r *rand.Rand, alphabet string, lo, hi int) string {
	n := lo + r.IntN(hi-lo+1)
	var b strings.Builder
	for range n {
		b.WriteByte(alphabet[r.IntN(len(alphabet))])
	}
	return b.String()
}

func randText(r *rand.Rand) string {
	return randString(r, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789^._-", 1, 8)
}

func randUID(r *rand.Rand) string {
	parts := make([]string, 2+r.IntN(4))
	for i := range parts {
		parts[i] = strconv.Itoa(r.IntN(1000))
	}
	return strings.Join(parts, ".")
}

func randDecimal(r *rand.Rand) string {
	s := strconv.Itoa(r.IntN(10000))
	if r.IntN(2) == 0 {
		s = "-" + s
	}
	if r.IntN(2) == 0 {
		s += "." + strconv.Itoa(r.IntN(1000))
	}
	return s
}

// randScalars returns independently present scalar elements on distinct tags.
func randScalars(r *rand.Rand) []*Element {
	var out []*Element
	maybe := func(e func() *Element) {
		if r.IntN(2) == 0 {
			out = append(out, e())
		}
	}
	maybe(func() *Element { return el(tag.PatientName, "PN", randText(r)) })
	maybe(func() *Element { return el(tag.PatientID, "LO", randText(r)) })
	maybe(func() *Element { return el(tag.StudyDescription, "LO", randText(r)) })
	maybe(func() *Element { return el(tag.SOPClassUID, "UI", randUID(r)) })
	maybe(func() *Element { return el(tag.RescaleIntercept, "DS", randDecimal(r)) })
	maybe(func() *Element { return el(tag.RescaleSlope, "DS", randDecimal(r)) })
	maybe(func() *Element { return el(tag.BitsAllocated, "US", uint16(r.Uint32())) })
	maybe(func() *Element { return el(tag.NumberOfAlarmObjects, "UL", r.Uint32()) })
	maybe(func() *Element {
		vals := make([]string, 2+r.IntN(2))
		for i := range vals {
			vals[i] = randText(r)
		}
		return el(tag.ImageType, "CS", strings.Join(vals, `\`))
	})
	return out
}

// randPixels returns self-consistent geometry and samples.
func randPixels(r *rand.Rand) (rows, cols, frames int, frameData [][]uint16) {
	rows, cols, frames = 1+r.IntN(4), 1+r.IntN(4), 1+r.IntN(2)
	for range frames {
		f := make([]uint16, rows*cols)
		for i := range f {
			f[i] = uint16(r.Uint32())
		}
		frameData = append(frameData, f)
	}
	return rows, cols, frames, frameData
}

func randDataset(r *rand.Rand) *Dataset {
	ds := newDS(randScalars(r)...)
	if r.IntN(2) == 0 {
		rows, cols, frames, fd := randPixels(r)
		ds.Elements[tag.Rows] = el(tag.Rows, "US", uint16(rows))
		ds.Elements[tag.Columns] = el(tag.Columns, "US", uint16(cols))
		if frames > 1 {
			ds.Elements[tag.NumberOfFrames] = el(tag.NumberOfFrames, "IS", strconv.Itoa(frames))
		}
		// Pin 16 so a random BitsAllocated cannot disagree with the samples.
		ds.Elements[tag.BitsAllocated] = el(tag.BitsAllocated, "US", uint16(16))
		ds.Elements[tag.PixelData] = el(tag.PixelData, "OW", native(fd...))
	}
	if r.IntN(2) == 0 {
		items := make([]*Dataset, 1+r.IntN(2))
		for i := range items {
			items[i] = newDS(randScalars(r)...)
		}
		ds.Elements[tag.ReferencedImageSequence] = el(tag.ReferencedImageSequence, "SQ", items)
	}
	return ds
}

func forEachCase(t *testing.T, seed uint64, fn func(t *testing.T, r *rand.Rand)) {
	r := rand.New(rand.NewPCG(seed, 0x9E3779B97F4A7C15))
	for i := range roundtripCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) { fn(t, r) })
	}
}

func TestRoundtrip_PreservesEveryElement(t *testing.T) {
	forEachCase(t, 1, func(t *testing.T, r *rand.Rand) {
		ds := randDataset(r)
		rt := roundtrip(t, ds)
		for tg, e := range ds.Elements {
			assert.Equal(t, e, rt.Elements[tg], "element %v did not survive the round trip", tg)
		}
	})
}

func TestRoundtrip_ParsedDatasetIsAFixedPoint(t *testing.T) {
	forEachCase(t, 2, func(t *testing.T, r *rand.Rand) {
		once := roundtrip(t, randDataset(r))
		twice := roundtrip(t, once)
		assert.Equal(t, once, twice)
	})
}

func TestRoundtrip_RewriteIsByteStable(t *testing.T) {
	forEachCase(t, 3, func(t *testing.T, r *rand.Rand) {
		normalized := roundtrip(t, randDataset(r))
		a := writeBytes(t, normalized)
		reparsed, err := parseBytes(a)
		require.NoError(t, err)
		assert.Equal(t, a, writeBytes(t, reparsed))
	})
}

func TestRoundtrip_NativePixelValuesArePreserved(t *testing.T) {
	forEachCase(t, 4, func(t *testing.T, r *rand.Rand) {
		rows, cols, frames, fd := randPixels(r)
		ds := newDS(
			el(tag.Rows, "US", uint16(rows)),
			el(tag.Columns, "US", uint16(cols)),
			el(tag.PixelData, "OW", native(fd...)))
		if frames > 1 {
			ds.Elements[tag.NumberOfFrames] = el(tag.NumberOfFrames, "IS", strconv.Itoa(frames))
		}
		assert.Equal(t, native(fd...), pixelDataOf(t, roundtrip(t, ds)))
	})
}

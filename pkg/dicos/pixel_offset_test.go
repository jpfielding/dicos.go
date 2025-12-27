package dicos

import (
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCTImage_LuggageFishtank verifies parsing of a well-formed synthetic CT DICOS file.
// The file uses native (uncompressed) pixel data with explicit rescale values.
func TestCTImage_LuggageFishtank(t *testing.T) {
	ds, err := ReadFile("testdata/luggage_fishtank_ct.dcs")
	require.NoError(t, err, "should parse luggage_fishtank_ct.dcs")

	// Modality
	assert.True(t, IsCT(ds), "should be identified as CT")

	// Dimensions
	assert.Equal(t, 256, GetRows(ds))
	assert.Equal(t, 256, GetColumns(ds))

	// BitsStored
	el, ok := ds.FindElement(tag.BitsStored.Group, tag.BitsStored.Element)
	require.True(t, ok, "BitsStored should be present")
	bitsStored, ok := el.GetInt()
	require.True(t, ok)
	assert.Equal(t, 16, bitsStored)

	// Rescale — explicit values in file, no heuristic needed
	intercept, slope := GetRescale(ds)
	assert.Equal(t, 0.0, intercept, "RescaleIntercept should be 0")
	assert.Equal(t, 1.0, slope, "RescaleSlope should be 1")

	// Native (uncompressed) pixel data
	pd, err := ds.GetPixelData()
	require.NoError(t, err)
	assert.False(t, pd.IsEncapsulated, "pixel data should be native (not encapsulated)")

	// 160 frames, each 256×256 uint16 pixels
	assert.Equal(t, 160, len(pd.Frames), "should have 160 frames")
	assert.Equal(t, 256*256, len(pd.Frames[0].Data), "each frame should have 256×256 pixels")
}

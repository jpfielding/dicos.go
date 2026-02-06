package dicos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendedCodec(t *testing.T) {
	tests := []struct {
		modality string
		wantNil  bool
	}{
		{"CT", false},
		{"DX", false},
		{"AIT2D", false},
		{"AIT3D", false},
		{"TDR", true},
		{"SR", true},
		{"UNKNOWN", false}, // Defaults to JPEG-LS
	}

	for _, tt := range tests {
		t.Run(tt.modality, func(t *testing.T) {
			codec := RecommendedCodec(tt.modality)
			if tt.wantNil {
				assert.Nil(t, codec)
			} else {
				assert.NotNil(t, codec)
				// Should recommend JPEG-LS for imaging modalities
				assert.Equal(t, "jpeg-ls", codec.Name())
			}
		})
	}
}

func TestCompareCompressionRatio(t *testing.T) {
	// Generate test data - gradient pattern
	rows, cols := 64, 64
	data := make([]uint16, rows*cols)
	for i := range data {
		data[i] = uint16(i % 65536)
	}

	// Compare multiple codecs
	ratios, err := CompareCompressionRatio(rows, cols, data,
		CodecJPEGLS, CodecRLE, nil)
	require.NoError(t, err)

	// Check results
	assert.Contains(t, ratios, "jpeg-ls")
	assert.Contains(t, ratios, "rle")
	assert.Contains(t, ratios, "uncompressed")

	// Uncompressed should have ratio of 1.0
	assert.Equal(t, 1.0, ratios["uncompressed"])

	// Compressed ratios should be > 1.0 (some compression achieved)
	assert.Greater(t, ratios["jpeg-ls"], 1.0)
	assert.Greater(t, ratios["rle"], 1.0)

	t.Logf("JPEG-LS ratio: %.2fx", ratios["jpeg-ls"])
	t.Logf("RLE ratio: %.2fx", ratios["rle"])
}

func TestCompareCompressionRatio_Errors(t *testing.T) {
	// Empty data
	_, err := CompareCompressionRatio(64, 64, []uint16{}, CodecJPEGLS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	// Data too small
	_, err = CompareCompressionRatio(64, 64, []uint16{1, 2, 3}, CodecJPEGLS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too small")
}

func TestEstimateCompressedSize(t *testing.T) {
	// Generate test data
	rows, cols := 64, 64
	data := make([]uint16, rows*cols)
	for i := range data {
		data[i] = uint16(i)
	}

	// Test with codec
	size, err := EstimateCompressedSize(rows, cols, data, CodecJPEGLS)
	require.NoError(t, err)
	assert.Greater(t, size, 0)

	// Should be smaller than uncompressed
	uncompressedSize := len(data) * 2
	assert.Less(t, size, uncompressedSize)

	t.Logf("Uncompressed: %d bytes, Compressed: %d bytes", uncompressedSize, size)

	// Test without codec (uncompressed)
	sizeUncomp, err := EstimateCompressedSize(rows, cols, data, nil)
	require.NoError(t, err)
	assert.Equal(t, uncompressedSize, sizeUncomp)
}

func TestEstimateCompressedSize_Errors(t *testing.T) {
	// Data too small
	_, err := EstimateCompressedSize(64, 64, []uint16{1, 2, 3}, CodecJPEGLS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too small")
}

func TestCompareCodecs(t *testing.T) {
	// Generate test data - gradient pattern for better compression
	rows, cols := 64, 64
	data := make([]uint16, rows*cols)
	for i := range data {
		// Create repeating pattern for better RLE compression
		data[i] = uint16((i / 16) * 256)
	}

	// Compare codecs
	comparisons, err := CompareCodecs(rows, cols, data,
		nil, CodecJPEGLS, CodecRLE)
	require.NoError(t, err)

	assert.Len(t, comparisons, 3)

	// Check uncompressed
	uncomp := comparisons[0]
	assert.Equal(t, "uncompressed", uncomp.Name)
	assert.Equal(t, 1.0, uncomp.Ratio)
	assert.Equal(t, 0, uncomp.SpaceSaved)
	assert.Equal(t, 0.0, uncomp.SpaceSavedPercent)

	// Check compressed codecs
	for _, comp := range comparisons[1:] {
		assert.NotEmpty(t, comp.Name)
		assert.Greater(t, comp.Ratio, 1.0, "Codec %s should have compression ratio > 1.0", comp.Name)
		assert.Greater(t, comp.SpaceSaved, 0, "Codec %s should save space", comp.Name)
		assert.Greater(t, comp.SpaceSavedPercent, 0.0, "Codec %s should save percentage > 0", comp.Name)

		// Log results
		t.Logf("%s", comp.String())
	}
}

func TestCompareCodecs_Errors(t *testing.T) {
	// Empty data
	_, err := CompareCodecs(64, 64, []uint16{}, CodecJPEGLS)
	assert.Error(t, err)

	// Data too small
	_, err = CompareCodecs(64, 64, []uint16{1, 2, 3}, CodecJPEGLS)
	assert.Error(t, err)
}

func TestCodecComparison_String(t *testing.T) {
	comp := CodecComparison{
		Name:               "jpeg-ls",
		UncompressedSize:   8192,
		CompressedSize:     2048,
		Ratio:              4.0,
		SpaceSaved:         6144,
		SpaceSavedPercent:  75.0,
	}

	str := comp.String()
	assert.Contains(t, str, "jpeg-ls")
	assert.Contains(t, str, "4.00x")
	assert.Contains(t, str, "8192")
	assert.Contains(t, str, "2048")
	assert.Contains(t, str, "75.0%")

	t.Logf("String: %s", str)
}

func TestCodecComparisonRealistic(t *testing.T) {
	// Generate realistic CT-like data
	rows, cols := 512, 512
	data := make([]uint16, rows*cols)

	// Simulate CT data: air (-1000 HU), soft tissue (0-100 HU), bone (1000+ HU)
	// With typical offset: add 32768 to make unsigned
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			i := y*cols + x

			// Center: soft tissue (offset ~32768)
			hu := 50.0

			// Edges: air (offset ~31768)
			if x < 100 || x > 412 || y < 100 || y > 412 {
				hu = -1000.0
			}

			// Some bone spots
			if (x > 200 && x < 300) && (y > 200 && y < 300) {
				hu = 1500.0
			}

			data[i] = uint16(hu + 32768)
		}
	}

	// Compare codecs
	comparisons, err := CompareCodecs(rows, cols, data,
		nil, CodecJPEGLS, CodecRLE, CodecJPEG2000)
	require.NoError(t, err)

	t.Log("CT-like data compression comparison:")
	for _, comp := range comparisons {
		t.Logf("  %s", comp.String())
	}

	// JPEG-LS should perform well on medical images
	jpegls := findComparison(comparisons, "jpeg-ls")
	require.NotNil(t, jpegls)
	assert.Greater(t, jpegls.Ratio, 1.5, "JPEG-LS should achieve at least 1.5x compression on CT data")
}

// Helper to find comparison by name
func findComparison(comparisons []CodecComparison, name string) *CodecComparison {
	for i := range comparisons {
		if comparisons[i].Name == name {
			return &comparisons[i]
		}
	}
	return nil
}

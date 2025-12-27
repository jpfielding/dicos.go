package dicos_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos"
	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCTImage_Write(t *testing.T) {
	ct := dicos.NewCTImage()
	ct.Patient.SetPatientName("Test", "Person", "", "", "")
	ct.Series.Modality = "CT"
	ct.Series.SeriesDescription = "Test Series"

	// Set dummy pixel data
	rows, cols := 10, 10
	data := make([]uint16, rows*cols)
	for i := range data {
		data[i] = uint16(i)
	}

	ct.SetPixelData(rows, cols, data)
	ct.Codec = nil // uncompressed

	var buf bytes.Buffer
	_, err := ct.WriteTo(&buf)
	require.NoError(t, err, "Failed to write CT Image")
	assert.Greater(t, buf.Len(), 0, "Should have written bytes")
}

func TestCTImage_WriteCompressed(t *testing.T) {
	ct := dicos.NewCTImage()
	ct.Patient.SetPatientName("Compressed", "Test", "", "", "")

	// Create a pattern that compresses well
	rows, cols := 512, 512
	data := make([]uint16, rows*cols)
	for i := range data {
		data[i] = uint16(i % 512)
	}

	ct.Rows = rows
	ct.Columns = cols
	ct.SetPixelData(rows, cols, data)
	ct.ContentDate = module.NewDate(time.Now())

	// Write uncompressed
	ct.Codec = nil
	var uncompressedBuf bytes.Buffer
	_, err := ct.WriteTo(&uncompressedBuf)
	require.NoError(t, err, "Failed to write uncompressed CT")

	// Write compressed
	ct.Codec = dicos.CodecJPEGLS
	var compressedBuf bytes.Buffer
	_, err = ct.WriteTo(&compressedBuf)
	require.NoError(t, err, "Failed to write compressed CT")

	t.Logf("Uncompressed size: %d, Compressed size: %d", uncompressedBuf.Len(), compressedBuf.Len())

	assert.Less(t, compressedBuf.Len(), uncompressedBuf.Len(),
		"Compressed (%d) should be smaller than uncompressed (%d)",
		compressedBuf.Len(), uncompressedBuf.Len())

	// Verify Transfer Syntax by reading back from buffer
	ds, err := dicos.ReadBuffer(compressedBuf.Bytes())
	require.NoError(t, err, "Failed to read back compressed data")

	syntax := dicos.GetTransferSyntax(ds)
	assert.Equal(t, dicos.JPEGLSLossless, syntax, "Expected JPEG-LS Lossless transfer syntax")
}

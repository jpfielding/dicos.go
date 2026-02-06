package dicos

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
)

// RecommendedCodec returns the recommended compression codec for the given modality.
//
// Recommendations based on NEMA DICOS standards and typical use cases:
//   - CT, DX, AIT2D, AIT3D: JPEG-LS Lossless (best compression for medical imaging)
//   - TDR: No compression (structured report, small size)
//   - Default: JPEG-LS Lossless
//
// Example:
//
//	codec := dicos.RecommendedCodec("CT")
//	ct.Codec = codec
//	ct.Write("scan.dcs")
func RecommendedCodec(modality string) Codec {
	switch modality {
	case "CT", "DX":
		return CodecJPEGLS // JPEG-LS recommended for DICOS per NEMA
	case "AIT2D", "AIT3D":
		return CodecJPEGLS // Millimeter wave imaging benefits from lossless compression
	case "TDR", "SR":
		return nil // Structured reports don't have pixel data
	default:
		return CodecJPEGLS // Safe default for medical imaging
	}
}

// CompareCompressionRatio tests multiple codecs and returns their compression ratios.
//
// The compression ratio is calculated as: uncompressed size / compressed size.
// Higher ratios mean better compression.
//
// This function compresses sample data using each codec to estimate performance.
// Actual compression ratios may vary with different image content.
//
// Parameters:
//   - rows, cols: Image dimensions
//   - data: Sample pixel data (single frame)
//   - codecs: Codecs to compare
//
// Returns a map of codec name to compression ratio, or error if encoding fails.
//
// Example:
//
//	ratios, err := dicos.CompareCompressionRatio(512, 512, pixelData,
//		dicos.CodecJPEGLS, dicos.CodecJPEG2000, dicos.CodecRLE)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for name, ratio := range ratios {
//		fmt.Printf("%s: %.2fx compression\n", name, ratio)
//	}
func CompareCompressionRatio(rows, cols int, data []uint16, codecs ...Codec) (map[string]float64, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}

	pixelsPerFrame := rows * cols
	if len(data) < pixelsPerFrame {
		return nil, fmt.Errorf("data too small: need %d pixels, got %d", pixelsPerFrame, len(data))
	}

	// Calculate uncompressed size (2 bytes per uint16 pixel)
	uncompressedSize := pixelsPerFrame * 2

	// Build grayscale image for encoding
	img := image.NewGray16(image.Rect(0, 0, cols, rows))
	for i := 0; i < pixelsPerFrame && i < len(data); i++ {
		x := i % cols
		y := i / cols
		img.SetGray16(x, y, color.Gray16{Y: data[i]})
	}

	ratios := make(map[string]float64)

	for _, codec := range codecs {
		if codec == nil {
			ratios["uncompressed"] = 1.0
			continue
		}

		var buf bytes.Buffer
		if err := codec.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encoding with %s failed: %w", codec.Name(), err)
		}

		compressedSize := buf.Len()
		ratio := float64(uncompressedSize) / float64(compressedSize)
		ratios[codec.Name()] = ratio
	}

	return ratios, nil
}

// EstimateCompressedSize estimates the compressed size for the given data and codec.
//
// Returns the estimated compressed size in bytes, or 0 if compression is not used.
//
// Note: This performs actual compression of the data, so it may be slow for large images.
// Use this for planning storage requirements or choosing codecs.
//
// Example:
//
//	size, err := dicos.EstimateCompressedSize(512, 512, pixelData, dicos.CodecJPEGLS)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Estimated compressed size: %d bytes (%.1f KB)\n", size, float64(size)/1024)
func EstimateCompressedSize(rows, cols int, data []uint16, codec Codec) (int, error) {
	if codec == nil {
		// Uncompressed size
		return len(data) * 2, nil
	}

	pixelsPerFrame := rows * cols
	if len(data) < pixelsPerFrame {
		return 0, fmt.Errorf("data too small: need %d pixels, got %d", pixelsPerFrame, len(data))
	}

	// Build grayscale image
	img := image.NewGray16(image.Rect(0, 0, cols, rows))
	for i := 0; i < pixelsPerFrame && i < len(data); i++ {
		x := i % cols
		y := i / cols
		img.SetGray16(x, y, color.Gray16{Y: data[i]})
	}

	var buf bytes.Buffer
	if err := codec.Encode(&buf, img); err != nil {
		return 0, fmt.Errorf("encoding with %s failed: %w", codec.Name(), err)
	}

	return buf.Len(), nil
}

// CompareCodecs provides a detailed comparison of multiple codecs.
//
// Returns a CodecComparison for each codec with compression metrics.
//
// Example:
//
//	comparisons, err := dicos.CompareCodecs(512, 512, pixelData,
//		dicos.CodecJPEGLS, dicos.CodecJPEG2000, dicos.CodecRLE)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, comp := range comparisons {
//		fmt.Printf("%s: %.2fx ratio, %d bytes\n",
//			comp.Name, comp.Ratio, comp.CompressedSize)
//	}
func CompareCodecs(rows, cols int, data []uint16, codecs ...Codec) ([]CodecComparison, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}

	pixelsPerFrame := rows * cols
	if len(data) < pixelsPerFrame {
		return nil, fmt.Errorf("data too small: need %d pixels, got %d", pixelsPerFrame, len(data))
	}

	uncompressedSize := pixelsPerFrame * 2

	// Build image once
	img := image.NewGray16(image.Rect(0, 0, cols, rows))
	for i := 0; i < pixelsPerFrame && i < len(data); i++ {
		x := i % cols
		y := i / cols
		img.SetGray16(x, y, color.Gray16{Y: data[i]})
	}

	comparisons := make([]CodecComparison, 0, len(codecs))

	for _, codec := range codecs {
		comp := CodecComparison{
			Codec:            codec,
			UncompressedSize: uncompressedSize,
		}

		if codec == nil {
			comp.Name = "uncompressed"
			comp.CompressedSize = uncompressedSize
			comp.Ratio = 1.0
			comp.SpaceSaved = 0
			comp.SpaceSavedPercent = 0.0
			comparisons = append(comparisons, comp)
			continue
		}

		comp.Name = codec.Name()

		var buf bytes.Buffer
		if err := codec.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encoding with %s failed: %w", codec.Name(), err)
		}

		comp.CompressedSize = buf.Len()
		comp.Ratio = float64(uncompressedSize) / float64(comp.CompressedSize)
		comp.SpaceSaved = uncompressedSize - comp.CompressedSize
		comp.SpaceSavedPercent = (float64(comp.SpaceSaved) / float64(uncompressedSize)) * 100

		comparisons = append(comparisons, comp)
	}

	return comparisons, nil
}

// CodecComparison contains compression metrics for a single codec.
type CodecComparison struct {
	Codec              Codec   // The codec being compared (nil for uncompressed)
	Name               string  // Codec name
	UncompressedSize   int     // Original size in bytes
	CompressedSize     int     // Compressed size in bytes
	Ratio              float64 // Compression ratio (uncompressed/compressed)
	SpaceSaved         int     // Bytes saved (uncompressed - compressed)
	SpaceSavedPercent  float64 // Percentage of space saved
}

// String returns a formatted string describing the codec comparison.
func (c CodecComparison) String() string {
	return fmt.Sprintf("%s: %.2fx ratio, %d → %d bytes, %.1f%% saved",
		c.Name, c.Ratio, c.UncompressedSize, c.CompressedSize, c.SpaceSavedPercent)
}

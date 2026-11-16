package dicos

import (
	"fmt"
	"image"
	"log/slog"
	"slices"
)

// DecodeVolume decodes all frames from a Dataset into a Volume.
// Handles both native (uncompressed) and encapsulated pixel data.
//
// The volume is sized from the data actually present: native samples must
// fill a whole number of Rows x Columns frames, and every decoded frame must
// have exactly Rows x Columns pixels. Memory grows frame by frame as frames
// decode, never from the declared geometry alone.
func DecodeVolume(ds *Dataset) (*Volume, error) {
	rows := ds.Rows()
	cols := ds.Columns()
	if rows <= 0 || cols <= 0 {
		return nil, fmt.Errorf("%w: dimensions %dx%d", ErrInvalidValue, cols, rows)
	}

	pd, err := ds.GetPixelData()
	if err != nil {
		return nil, err
	}
	if !pd.IsEncapsulated() {
		return nativeVolume(rows, cols, pd)
	}

	frameSize := satMul(rows, cols)
	vol := NewVolume(cols, rows, 0)
	ts := ds.TransferSyntax()
	for z, frame := range pd.Frames {
		img, err := decodeCompressedFrame(frame.CompressedData, rows, cols, ts)
		if err != nil {
			return nil, fmt.Errorf("decoding frame %d: %w", z, err)
		}
		b := img.Bounds()
		if b.Dx() != cols || b.Dy() != rows {
			return nil, fmt.Errorf("%w: frame %d decoded to %dx%d, expected %dx%d",
				ErrInvalidValue, z, b.Dx(), b.Dy(), cols, rows)
		}
		base := len(vol.Data)
		vol.Data = slices.Grow(vol.Data, frameSize)[:base+frameSize]
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				r, _, _, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				vol.Data[base+y*cols+x] = uint16(r)
			}
		}
		vol.Depth++
	}
	return vol, nil
}

// nativeVolume builds a Volume from native frames. The samples present must
// fill a whole, non-zero number of rows x cols frames.
func nativeVolume(rows, cols int, pd *PixelData) (*Volume, error) {
	total := pd.TotalPixels()
	frameSize := satMul(rows, cols)
	if total == 0 || frameSize > total || total%frameSize != 0 {
		return nil, fmt.Errorf("%w: %d native samples do not fill whole %dx%d frames",
			ErrInvalidValue, total, cols, rows)
	}
	vol := NewVolume(cols, rows, 0)
	vol.Depth = total / frameSize
	vol.Data = pd.GetFlatData()
	return vol, nil
}

// decodeCompressedFrame detects compression type and decodes
func decodeCompressedFrame(data []byte, rows, cols int, ts TransferSyntax) (image.Image, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("compressed data too short: %d bytes", len(data))
	}

	// 1. Use Transfer Syntax if available via codec registry
	tsUID := string(ts)
	if codec := CodecByTransferSyntax(tsUID); codec != nil {
		return codec.Decode(data, cols, rows)
	}

	// 2. Fallback to sniffing if TS is unknown or generic
	var sniffedCodec Codec

	// Strict check for JPEG SOI (FF D8) or J2K SOC (FF 4F) at start
	if len(data) > 2 {
		// Scan only if starts with FF D8 (JPEG)
		if data[0] == 0xFF && data[1] == 0xD8 {
			// Scan for SOF marker to distinguish JPEG-LS vs JPEG Lossless
			for i := 0; i < len(data)-1; i++ {
				if data[i] == 0xFF {
					switch data[i+1] {
					case 0xF7: // SOF55 - JPEG-LS
						sniffedCodec = CodecJPEGLS
					case 0xC3: // SOF3 - JPEG Lossless
						sniffedCodec = CodecJPEGLi
					}
					if sniffedCodec != nil {
						break
					}
				}
			}
		} else if data[0] == 0xFF && data[1] == 0x4F {
			// J2K SOC marker
			sniffedCodec = CodecJPEG2000
		}
	}

	if sniffedCodec != nil {
		return sniffedCodec.Decode(data, cols, rows)
	}

	// Check for RLE (header is 64 bytes)
	if len(data) >= 64 {
		img, err := CodecRLE.Decode(data, cols, rows)
		if err == nil {
			return img, nil
		}

		slog.Debug("RLE decode attempt failed",
			slog.Any("error", err),
			slog.Int("dataLen", len(data)),
			slog.Int("cols", cols),
			slog.Int("rows", rows))
	}

	// Fallback: Try JPEG Lossless first (more common in DICOM), then JPEG-LS
	img, err := CodecJPEGLi.Decode(data, cols, rows)
	if err == nil {
		return img, nil
	}

	return CodecJPEGLS.Decode(data, cols, rows)
}

// DecodeFrameData decodes a single frame from pixel data and returns its
// rows*cols uint16 samples. A decoded frame must have exactly rows x cols
// pixels; a native frame must hold at least rows*cols samples.
func DecodeFrameData(pd *PixelData, frameIndex int, rows, cols int, ts TransferSyntax) ([]uint16, error) {
	if frameIndex < 0 || frameIndex >= len(pd.Frames) {
		return nil, fmt.Errorf("frame index %d out of range (0-%d)", frameIndex, len(pd.Frames)-1)
	}
	if rows <= 0 || cols <= 0 {
		return nil, fmt.Errorf("%w: dimensions %dx%d", ErrInvalidValue, cols, rows)
	}
	frame := pd.Frames[frameIndex]
	pixelCount := satMul(rows, cols)

	if !pd.IsEncapsulated() {
		if pixelCount > len(frame.Data) {
			return nil, fmt.Errorf("%w: frame %d has %d samples, need %d",
				ErrInvalidValue, frameIndex, len(frame.Data), pixelCount)
		}
		return slices.Clone(frame.Data[:pixelCount]), nil
	}

	decoded, err := decodeCompressedFrame(frame.CompressedData, rows, cols, ts)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}
	b := decoded.Bounds()
	if b.Dx() != cols || b.Dy() != rows {
		return nil, fmt.Errorf("%w: frame %d decoded to %dx%d, expected %dx%d",
			ErrInvalidValue, frameIndex, b.Dx(), b.Dy(), cols, rows)
	}
	data := make([]uint16, pixelCount)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r, _, _, _ := decoded.At(b.Min.X+x, b.Min.Y+y).RGBA()
			data[y*cols+x] = uint16(r)
		}
	}
	return data, nil
}

// GetWindowLevel returns the window center and width from the dataset
func GetWindowLevel(ds *Dataset) (center, width int) {
	center, width = 40, 400 // CT soft tissue defaults

	if elem, ok := ds.FindElement(0x0028, 0x1050); ok { // Window Center
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%d", &center)
		} else if v, ok := elem.GetInt(); ok {
			center = v
		}
	}

	if elem, ok := ds.FindElement(0x0028, 0x1051); ok { // Window Width
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%d", &width)
		} else if v, ok := elem.GetInt(); ok {
			width = v
		}
	}

	return
}

// GetPixelSpacing returns the pixel spacing in mm
func GetPixelSpacing(ds *Dataset) (row, col float64) {
	row, col = 1.0, 1.0 // Defaults

	if elem, ok := ds.FindElement(0x0028, 0x0030); ok { // Pixel Spacing
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f\\%f", &row, &col)
		}
	}

	return
}

// GetSliceThickness returns the slice thickness in mm
func GetSliceThickness(ds *Dataset) float64 {
	if elem, ok := ds.FindElement(0x0018, 0x0050); ok {
		if s, ok := elem.GetString(); ok {
			var thickness float64
			fmt.Sscanf(s, "%f", &thickness)
			return thickness
		}
	}
	return 1.0 // Default
}

// GetImagePositionPatient returns the position of the image origin
func GetImagePositionPatient(ds *Dataset) []float64 {
	if elem, ok := ds.FindElement(0x0020, 0x0032); ok {
		if s, ok := elem.GetString(); ok {
			var x, y, z float64
			if _, err := fmt.Sscanf(s, "%f\\%f\\%f", &x, &y, &z); err == nil {
				return []float64{x, y, z}
			}
		}
	}
	// Default to 0,0,0
	return []float64{0.0, 0.0, 0.0}
}

// GetImageOrientationPatient returns the orientation cosines
func GetImageOrientationPatient(ds *Dataset) []float64 {
	if elem, ok := ds.FindElement(0x0020, 0x0037); ok {
		if s, ok := elem.GetString(); ok {
			var r1, r2, r3, c1, c2, c3 float64
			if _, err := fmt.Sscanf(s, "%f\\%f\\%f\\%f\\%f\\%f", &r1, &r2, &r3, &c1, &c2, &c3); err == nil {
				return []float64{r1, r2, r3, c1, c2, c3}
			}
		}
	}
	// Default to Identity
	return []float64{1.0, 0.0, 0.0, 0.0, 1.0, 0.0}
}

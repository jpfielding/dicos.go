package dicos

import (
	"fmt"
	"image"
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
				ErrDimensionMismatch, z, b.Dx(), b.Dy(), cols, rows)
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

// decodeCompressedFrame decodes one frame with the codec registered for the
// transfer syntax. An unknown or uncompressed transfer syntax is an error
// wrapping ErrUnsupportedTransferSyntax: there is no content-sniffing
// fallback. Use DecodeFrameSniffed when the transfer syntax is unavailable.
func decodeCompressedFrame(data []byte, rows, cols int, ts TransferSyntax) (image.Image, error) {
	codec := CodecByTransferSyntax(string(ts))
	if codec == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedTransferSyntax, string(ts))
	}
	return codec.Decode(data, cols, rows)
}

// SniffCodec identifies the codec of a compressed frame from its leading
// bytes, or returns nil.
//
//   - JPEG 2000: the codestream SOC marker FF 4F (the JP2 box format is not
//     recognised; the decoder is codestream only).
//   - JPEG-LS / JPEG Lossless: a structured marker walk from the FF D8 SOI to
//     the first start-of-frame marker (FF F7 is JPEG-LS, FF C3 is JPEG
//     Lossless), skipping length-prefixed segments and stopping at SOS or on
//     malformed structure. Entropy-coded data is never scanned.
//
// RLE has no reliable signature and is never sniffed.
func SniffCodec(data []byte) Codec {
	if len(data) < 2 || data[0] != 0xFF {
		return nil
	}
	switch data[1] {
	case 0x4F:
		return CodecJPEG2000
	case 0xD8:
		return sniffJPEGSOF(data)
	}
	return nil
}

// sniffJPEGSOF walks JPEG marker segments from the SOI to the first SOF.
func sniffJPEGSOF(data []byte) Codec {
	i := 2
	for {
		// Each segment starts with 0xFF, possibly repeated as fill bytes.
		if i >= len(data) || data[i] != 0xFF {
			return nil
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			return nil
		}
		marker := data[i]
		i++
		switch {
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9):
			continue // standalone: TEM, RSTn, SOI, EOI
		case marker == 0xDA:
			return nil // SOS before any SOF
		case marker == 0xC3:
			return CodecJPEGLi
		case marker == 0xF7:
			return CodecJPEGLS
		}
		if i+1 >= len(data) {
			return nil
		}
		segLen := int(data[i])<<8 | int(data[i+1])
		if segLen < 2 || i+segLen > len(data) {
			return nil
		}
		i += segLen
	}
}

// DecodeFrameSniffed decodes a compressed frame whose transfer syntax is
// unknown, choosing the codec with SniffCodec. Prefer the transfer syntax
// path (DecodeVolume, DecodeFrameData) whenever the syntax is known. An
// unidentifiable frame is an error wrapping ErrUnsupportedTransferSyntax.
func DecodeFrameSniffed(data []byte, width, height int) (image.Image, error) {
	codec := SniffCodec(data)
	if codec == nil {
		return nil, fmt.Errorf("%w: unable to identify codec from data signature", ErrUnsupportedTransferSyntax)
	}
	return codec.Decode(data, width, height)
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
			ErrDimensionMismatch, frameIndex, b.Dx(), b.Dy(), cols, rows)
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

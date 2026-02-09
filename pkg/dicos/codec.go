package dicos

import (
	"bytes"
	"image"
	"io"

	"github.com/jpfielding/jpegs/pkg/compress/jpeg2k"
	"github.com/jpfielding/jpegs/pkg/compress/jpegli"
	"github.com/jpfielding/jpegs/pkg/compress/jpegls"
	"github.com/jpfielding/jpegs/pkg/compress/rle"
)

// Codec defines the interface for DICOS pixel data compression and decompression.
//
// DICOS uses various compression codecs to reduce file size while maintaining image
// quality for security screening applications. Each codec has different characteristics
// for compression ratio, speed, and quality.
//
// Supported Codecs:
//
//   - JPEG-LS (Recommended for DICOS):
//     Lossless/near-lossless compression with excellent ratio for medical imaging.
//     Specified by NEMA DICOS standard. Use CodecJPEGLS.
//
//   - JPEG Lossless (Process 14):
//     Older lossless JPEG variant with predictive coding. Use CodecJPEGLi.
//
//   - RLE (Run-Length Encoding):
//     Simple lossless compression, fast but lower ratios. Use CodecRLE.
//
//   - JPEG 2000:
//     Wavelet-based compression with lossless/lossy modes. Use CodecJPEG2000.
//
// Example - Using a codec:
//
//	ct := dicos.NewCTImage()
//	ct.SetPixelData(512, 512, pixelData)
//	ct.Codec = dicos.CodecJPEGLS // Compress using JPEG-LS
//	ct.Write("output.dcs")
//
// Example - Decoding compressed pixel data:
//
//	ds, _ := dicos.ReadFile("scan.dcs")
//	pd, _ := ds.GetPixelData()
//	if pd.IsEncapsulated {
//		ts := dicos.GetTransferSyntax(ds)
//		codec := dicos.CodecByTransferSyntax(ts.UID())
//		for i, frame := range pd.Frames {
//			img, err := codec.Decode(frame.CompressedData, 512, 512)
//			// Process decompressed image...
//		}
//	}
type Codec interface {
	// Encode compresses an image to the writer
	Encode(w io.Writer, img image.Image) error
	// Decode decompresses data to an image
	// width/height provided for codecs that need them (RLE)
	Decode(data []byte, width, height int) (image.Image, error)
	// Name returns the codec identifier (e.g., "jpeg-ls")
	Name() string
	// TransferSyntaxUID returns the DICOM transfer syntax for this codec
	TransferSyntaxUID() string
}

// jpegLSCodec implements Codec for JPEG-LS
type jpegLSCodec struct{}

func (c *jpegLSCodec) Encode(w io.Writer, img image.Image) error {
	return jpegls.Encode(w, img, nil)
}

func (c *jpegLSCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return jpegls.Decode(bytes.NewReader(data))
}

func (c *jpegLSCodec) Name() string {
	return "jpeg-ls"
}

func (c *jpegLSCodec) TransferSyntaxUID() string {
	return "1.2.840.10008.1.2.4.80" // JPEG-LS Lossless
}

// jpegLiCodec implements Codec for JPEG Lossless (Process 14)
type jpegLiCodec struct{}

func (c *jpegLiCodec) Encode(w io.Writer, img image.Image) error {
	return jpegli.Encode(w, img, nil)
}

func (c *jpegLiCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return jpegli.Decode(bytes.NewReader(data))
}

func (c *jpegLiCodec) Name() string {
	return "jpeg-li"
}

func (c *jpegLiCodec) TransferSyntaxUID() string {
	return "1.2.840.10008.1.2.4.70" // JPEG Lossless First-Order (Process 14, SV1)
}

// rleCodec implements Codec for RLE Lossless
type rleCodec struct{}

func (c *rleCodec) Encode(w io.Writer, img image.Image) error {
	return rle.Encode(w, img)
}

func (c *rleCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return rle.Decode(data, width, height)
}

func (c *rleCodec) Name() string {
	return "rle"
}

func (c *rleCodec) TransferSyntaxUID() string {
	return "1.2.840.10008.1.2.5" // RLE Lossless
}

// jpeg2kCodec implements Codec for JPEG 2000
type jpeg2kCodec struct{}

func (c *jpeg2kCodec) Encode(w io.Writer, img image.Image) error {
	return jpeg2k.Encode(w, img, nil)
}

func (c *jpeg2kCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return jpeg2k.Decode(bytes.NewReader(data))
}

func (c *jpeg2kCodec) Name() string {
	return "jpeg-2000"
}

func (c *jpeg2kCodec) TransferSyntaxUID() string {
	return "1.2.840.10008.1.2.4.90" // JPEG 2000 Lossless Only
}

// codecsByName maps codec names to implementations
var codecsByName = map[string]Codec{
	"jpeg-ls":   &jpegLSCodec{},
	"jpeg-li":   &jpegLiCodec{},
	"rle":       &rleCodec{},
	"jpeg-2000": &jpeg2kCodec{},
	"jpeg2000":  &jpeg2kCodec{}, // alias
}

// codecsByTS maps transfer syntax UIDs to implementations
var codecsByTS = map[string]Codec{
	"1.2.840.10008.1.2.4.80": &jpegLSCodec{}, // JPEG-LS Lossless
	"1.2.840.10008.1.2.4.81": &jpegLSCodec{}, // JPEG-LS Near-Lossless
	"1.2.840.10008.1.2.4.70": &jpegLiCodec{}, // JPEG Lossless First-Order
	"1.2.840.10008.1.2.5":    &rleCodec{},    // RLE Lossless
	"1.2.840.10008.1.2.4.90": &jpeg2kCodec{}, // JPEG 2000 Lossless
}

// Predefined codec instances for convenience.
//
// Use these when configuring compression for DICOS images:
//
//	ct.Codec = dicos.CodecJPEGLS // Recommended for DICOS
//	dx.Codec = dicos.CodecRLE    // Fast lossless compression
//
// CodecJPEGLS is the recommended choice for DICOS per NEMA standards, providing
// excellent compression ratios with lossless quality.
var (
	CodecJPEGLS   Codec = codecsByName["jpeg-ls"]   // JPEG-LS Lossless (recommended)
	CodecJPEGLi   Codec = codecsByName["jpeg-li"]   // JPEG Lossless Process 14
	CodecRLE      Codec = codecsByName["rle"]       // RLE Lossless
	CodecJPEG2000 Codec = codecsByName["jpeg-2000"] // JPEG 2000 Lossless
)

// CodecByName returns a codec by its name identifier.
//
// Supported names:
//   - "jpeg-ls" - JPEG-LS Lossless (recommended for DICOS)
//   - "jpeg-li" - JPEG Lossless First-Order (Process 14)
//   - "rle" - RLE Lossless
//   - "jpeg-2000", "jpeg2000" - JPEG 2000 Lossless
//
// Returns nil if the codec name is not recognized.
//
// Example:
//
//	codec := dicos.CodecByName("jpeg-ls")
//	if codec == nil {
//		log.Fatal("Unknown codec")
//	}
func CodecByName(name string) Codec {
	return codecsByName[name]
}

// CodecByTransferSyntax returns a codec for the given DICOM Transfer Syntax UID.
//
// Supported transfer syntaxes:
//   - "1.2.840.10008.1.2.4.80" - JPEG-LS Lossless
//   - "1.2.840.10008.1.2.4.81" - JPEG-LS Near-Lossless
//   - "1.2.840.10008.1.2.4.70" - JPEG Lossless First-Order (Process 14)
//   - "1.2.840.10008.1.2.5" - RLE Lossless
//   - "1.2.840.10008.1.2.4.90" - JPEG 2000 Lossless
//
// Returns nil if the transfer syntax is not supported or is uncompressed
// (Explicit/Implicit VR Little Endian).
//
// Example:
//
//	ds, _ := dicos.ReadFile("scan.dcs")
//	ts := dicos.GetTransferSyntax(ds)
//	codec := dicos.CodecByTransferSyntax(ts.UID())
//	if codec != nil {
//		// Compressed pixel data, use codec to decompress
//	}
func CodecByTransferSyntax(ts string) Codec {
	return codecsByTS[ts]
}

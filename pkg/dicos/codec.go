package dicos

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"

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
//     Lossless reversible-wavelet (ITU-T T.800) compression, 16-bit grayscale
//     only (as dicos.rs). Use CodecJPEG2000. Streams written by dicos.go
//     before v0.3.0 (non-standard layout) still decode.
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
//	if pd.IsEncapsulated() {
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

// codecAdapter bridges a codec package to the Codec interface. Backend
// failures are wrapped in *CodecError carrying the codec name, and when the
// caller supplies dimensions the decoded image must match them.
type codecAdapter struct {
	name   string
	ts     string
	encode func(w io.Writer, img image.Image) error
	decode func(data []byte, width, height int) (image.Image, error)
}

func (c *codecAdapter) Encode(w io.Writer, img image.Image) error {
	if err := c.encode(w, img); err != nil {
		return &CodecError{Codec: c.name, Err: err}
	}
	return nil
}

// Decode decodes one frame. When width and height are both positive the
// decoded image must have exactly those dimensions, else the error wraps
// ErrDimensionMismatch.
func (c *codecAdapter) Decode(data []byte, width, height int) (image.Image, error) {
	img, err := c.decode(data, width, height)
	if err != nil {
		return nil, &CodecError{Codec: c.name, Err: err}
	}
	if width > 0 && height > 0 {
		if b := img.Bounds(); b.Dx() != width || b.Dy() != height {
			return nil, &CodecError{Codec: c.name, Err: fmt.Errorf("%w: expected %dx%d, got %dx%d",
				ErrDimensionMismatch, width, height, b.Dx(), b.Dy())}
		}
	}
	return img, nil
}

func (c *codecAdapter) Name() string              { return c.name }
func (c *codecAdapter) TransferSyntaxUID() string { return c.ts }

// Registry encodes always use each codec's defaults so the advertised
// transfer syntax stays truthful for the bytes produced.
var (
	jpegLSCodec = &codecAdapter{
		name:   "jpeg-ls",
		ts:     string(transfer.JPEGLSLossless),
		encode: func(w io.Writer, img image.Image) error { return jpegls.Encode(w, img, nil) },
		decode: func(data []byte, _, _ int) (image.Image, error) { return jpegls.Decode(bytes.NewReader(data)) },
	}
	jpegLiCodec = &codecAdapter{
		name:   "jpeg-li",
		ts:     string(transfer.JPEGLosslessFirstOrder),
		encode: func(w io.Writer, img image.Image) error { return jpegli.Encode(w, img, nil) },
		decode: func(data []byte, _, _ int) (image.Image, error) { return jpegli.Decode(bytes.NewReader(data)) },
	}
	rleCodec = &codecAdapter{
		name:   "rle",
		ts:     string(transfer.RLELossless),
		encode: rle.Encode,
		decode: rle.Decode,
	}
	jpeg2kCodec = &codecAdapter{
		name:   "jpeg-2000",
		ts:     string(transfer.JPEG2000Lossless),
		encode: func(w io.Writer, img image.Image) error { return jpeg2k.Encode(w, img, nil) },
		decode: func(data []byte, _, _ int) (image.Image, error) { return jpeg2k.Decode(bytes.NewReader(data)) },
	}
)

// codecsByName maps lower-case codec names and aliases to implementations.
var codecsByName = map[string]Codec{
	"rle":           rleCodec,
	"jpeg-ls":       jpegLSCodec,
	"jpegls":        jpegLSCodec,
	"jpeg-li":       jpegLiCodec,
	"jpegli":        jpegLiCodec,
	"jpeg-lossless": jpegLiCodec,
	"jpeg-2000":     jpeg2kCodec,
	"jpeg2000":      jpeg2kCodec,
	"j2k":           jpeg2kCodec,
}

// codecsByTS maps transfer syntax UIDs to implementations.
var codecsByTS = map[string]Codec{
	string(transfer.JPEGLSLossless):         jpegLSCodec,
	string(transfer.JPEGLSNearLossless):     jpegLSCodec,
	string(transfer.JPEGLossless):           jpegLiCodec,
	string(transfer.JPEGLosslessFirstOrder): jpegLiCodec,
	string(transfer.RLELossless):            rleCodec,
	string(transfer.JPEG2000Lossless):       jpeg2kCodec,
	// Lossy J2K maps to the lossless-only decoder (as dicos.rs), so lossy
	// streams fail as unsupported by the codec rather than as an unknown
	// transfer syntax.
	string(transfer.JPEG2000): jpeg2kCodec,
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
	CodecJPEGLS   Codec = jpegLSCodec // JPEG-LS Lossless (recommended)
	CodecJPEGLi   Codec = jpegLiCodec // JPEG Lossless Process 14
	CodecRLE      Codec = rleCodec    // RLE Lossless
	CodecJPEG2000 Codec = jpeg2kCodec // JPEG 2000 Lossless
)

// CodecByName returns a codec by its name identifier (case-insensitive).
//
// Supported names:
//   - "jpeg-ls", "jpegls" - JPEG-LS Lossless (recommended for DICOS)
//   - "jpeg-li", "jpegli", "jpeg-lossless" - JPEG Lossless (Process 14)
//   - "rle" - RLE Lossless
//   - "jpeg-2000", "jpeg2000", "j2k" - JPEG 2000 Lossless
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
	return codecsByName[strings.ToLower(name)]
}

// CodecByTransferSyntax returns a codec for the given DICOM Transfer Syntax UID.
//
// Supported transfer syntaxes:
//   - "1.2.840.10008.1.2.4.80" - JPEG-LS Lossless
//   - "1.2.840.10008.1.2.4.81" - JPEG-LS Near-Lossless
//   - "1.2.840.10008.1.2.4.57" - JPEG Lossless (Process 14)
//   - "1.2.840.10008.1.2.4.70" - JPEG Lossless First-Order (Process 14)
//   - "1.2.840.10008.1.2.5" - RLE Lossless
//   - "1.2.840.10008.1.2.4.90" - JPEG 2000 Lossless
//   - "1.2.840.10008.1.2.4.91" - JPEG 2000 (decoded by the lossless-only codec)
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

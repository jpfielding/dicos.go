package dicos

import (
	"bytes"
	"image"
	"io"

	"github.com/jpfielding/dicos.go/pkg/compress/jpeg2k"
	"github.com/jpfielding/dicos.go/pkg/compress/jpegli"
	"github.com/jpfielding/dicos.go/pkg/compress/jpegls"
	"github.com/jpfielding/dicos.go/pkg/compress/rle"
)

// Codec defines the interface for DICOS pixel data compression
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
	"1.2.840.10008.1.2.4.80": &jpegLSCodec{},  // JPEG-LS Lossless
	"1.2.840.10008.1.2.4.81": &jpegLSCodec{},  // JPEG-LS Near-Lossless
	"1.2.840.10008.1.2.4.70": &jpegLiCodec{},  // JPEG Lossless First-Order
	"1.2.840.10008.1.2.5":    &rleCodec{},     // RLE Lossless
	"1.2.840.10008.1.2.4.90": &jpeg2kCodec{},  // JPEG 2000 Lossless
}

// Predefined codec instances for convenience
var (
	CodecJPEGLS   Codec = codecsByName["jpeg-ls"]
	CodecJPEGLi   Codec = codecsByName["jpeg-li"]
	CodecRLE      Codec = codecsByName["rle"]
	CodecJPEG2000 Codec = codecsByName["jpeg-2000"]
)

// CodecByName returns a codec by name, or nil if not found
func CodecByName(name string) Codec {
	return codecsByName[name]
}

// CodecByTransferSyntax returns a codec for a transfer syntax, or nil if not found
func CodecByTransferSyntax(ts string) Codec {
	return codecsByTS[ts]
}

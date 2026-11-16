package dicos

import (
	"bytes"
	"errors"
	"image"
	"io"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codec registry tests ported from dicos.rs codec_registry.rs.

func TestCodecByName_UnknownReturnsNil(t *testing.T) {
	assert.Nil(t, CodecByName("unknown-codec"))
	assert.Nil(t, CodecByName(""))
}

func TestCodecByName_AliasesAreCaseInsensitive(t *testing.T) {
	tests := map[string]Codec{
		"rle": CodecRLE, "RLE": CodecRLE,
		"jpeg-ls": CodecJPEGLS, "jpegls": CodecJPEGLS, "JPEG-LS": CodecJPEGLS,
		"jpeg-li": CodecJPEGLi, "jpegli": CodecJPEGLi, "jpeg-lossless": CodecJPEGLi,
		"jpeg-2000": CodecJPEG2000, "jpeg2000": CodecJPEG2000, "j2k": CodecJPEG2000, "J2K": CodecJPEG2000,
	}
	for name, want := range tests {
		assert.Same(t, want, CodecByName(name), name)
	}
}

func TestCodecByTransferSyntax_UnknownReturnsNil(t *testing.T) {
	assert.Nil(t, CodecByTransferSyntax("1.2.3.4.5.999"))
	assert.Nil(t, CodecByTransferSyntax(""))
}

func TestCodecByTransferSyntax_UncompressedReturnsNil(t *testing.T) {
	assert.Nil(t, CodecByTransferSyntax(string(transfer.ImplicitVRLittleEndian)))
	assert.Nil(t, CodecByTransferSyntax(string(transfer.ExplicitVRLittleEndian)))
}

func TestCodecByTransferSyntax_Coverage(t *testing.T) {
	tests := map[transfer.Syntax]Codec{
		transfer.RLELossless:            CodecRLE,
		transfer.JPEGLSLossless:         CodecJPEGLS,
		transfer.JPEGLSNearLossless:     CodecJPEGLS,
		transfer.JPEGLossless:           CodecJPEGLi,
		transfer.JPEGLosslessFirstOrder: CodecJPEGLi,
		transfer.JPEG2000Lossless:       CodecJPEG2000,
		transfer.JPEG2000:               CodecJPEG2000,
	}
	for ts, want := range tests {
		assert.Same(t, want, CodecByTransferSyntax(string(ts)), string(ts))
	}
	assert.Equal(t, string(transfer.RLELossless), CodecRLE.TransferSyntaxUID())
	assert.Equal(t, string(transfer.JPEGLSLossless), CodecJPEGLS.TransferSyntaxUID())
	assert.Equal(t, string(transfer.JPEGLosslessFirstOrder), CodecJPEGLi.TransferSyntaxUID())
	assert.Equal(t, string(transfer.JPEG2000Lossless), CodecJPEG2000.TransferSyntaxUID())
}

func TestSniffCodec_EmptyAndRandomDataReturnNil(t *testing.T) {
	assert.Nil(t, SniffCodec(nil))
	assert.Nil(t, SniffCodec([]byte{0xFF}))
	assert.Nil(t, SniffCodec([]byte{0x00, 0x00}))
}

func TestSniffCodec_RLEShapedDataIsNotSniffed(t *testing.T) {
	assert.Nil(t, SniffCodec([]byte{0x01, 0x00, 0x00, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}))
}

func TestSniffCodec_WalksPastAPPnSegmentToFindSOF55(t *testing.T) {
	data := []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xE0, 0x00, 0x04, 0xAA, 0xBB, // APP0, length 4
		0xFF, 0xF7, 0x00, 0x0B, // SOF55
	}
	assert.Same(t, CodecJPEGLS, SniffCodec(data))
}

func TestSniffCodec_SOF3IsJPEGLossless(t *testing.T) {
	assert.Same(t, CodecJPEGLi, SniffCodec([]byte{0xFF, 0xD8, 0xFF, 0xC3, 0x00, 0x0B}))
}

// A SOF55 byte pair inside a segment body or after SOS must not be matched;
// the old byte scan matched it anywhere in the frame.
func TestSniffCodec_DoesNotMatchMarkersInsideSegments(t *testing.T) {
	inApp := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0xFF, 0xF7}
	assert.Nil(t, SniffCodec(inApp), "segment truncated before any SOF")
	afterSOS := []byte{0xFF, 0xD8, 0xFF, 0xDA, 0x00, 0x02, 0xFF, 0xF7}
	assert.Nil(t, SniffCodec(afterSOS))
	badLen := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x01, 0xFF, 0xF7}
	assert.Nil(t, SniffCodec(badLen))
}

func TestSniffCodec_JPEG2000Codestream(t *testing.T) {
	assert.Same(t, CodecJPEG2000, SniffCodec([]byte{0xFF, 0x4F, 0xFF, 0x51, 0x00, 0x00}))
}

func TestDecodeFrame_UnknownTSIsUnsupportedEvenForRLEShapedData(t *testing.T) {
	rleShaped := []byte{0x01, 0x00, 0x00, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}
	_, err := decodeCompressedFrame(rleShaped, 2, 2, TransferSyntax("1.2.3.4.5.unknown"))
	assert.ErrorIs(t, err, ErrUnsupportedTransferSyntax)

	pd := &PixelData{Frames: []Frame{{CompressedData: rleShaped}}}
	_, err = DecodeFrameData(pd, 0, 2, 2, TransferSyntax("1.2.3.4.5.unknown"))
	assert.ErrorIs(t, err, ErrUnsupportedTransferSyntax)
}

func TestDecodeFrameSniffed_DecodesJPEGLSFrame(t *testing.T) {
	img := image.NewGray16(image.Rect(0, 0, 3, 2))
	for i, v := range []uint16{10, 20, 30, 40, 50, 60} {
		img.Pix[2*i], img.Pix[2*i+1] = byte(v>>8), byte(v)
	}
	var enc bytes.Buffer
	require.NoError(t, CodecJPEGLS.Encode(&enc, img))

	got, err := DecodeFrameSniffed(enc.Bytes(), 3, 2)
	require.NoError(t, err)
	var px []uint16
	for y := range 2 {
		for x := range 3 {
			r, _, _, _ := got.At(x, y).RGBA()
			px = append(px, uint16(r))
		}
	}
	assert.Equal(t, []uint16{10, 20, 30, 40, 50, 60}, px)
}

func TestDecodeFrameSniffed_UnidentifiedIsUnsupported(t *testing.T) {
	_, err := DecodeFrameSniffed([]byte{0, 0, 0, 0}, 1, 1)
	assert.ErrorIs(t, err, ErrUnsupportedTransferSyntax)
}

// Mock backend that declares a 4x4 frame but decodes 3x3.
func TestCodecAdapter_DimensionMismatchIsError(t *testing.T) {
	mock := &codecAdapter{
		name:   "mock",
		ts:     "1.2.3.mock",
		encode: func(io.Writer, image.Image) error { return nil },
		decode: func([]byte, int, int) (image.Image, error) {
			return image.NewGray16(image.Rect(0, 0, 3, 3)), nil
		},
	}
	_, err := mock.Decode([]byte{0, 0, 0, 0}, 4, 4)
	require.ErrorIs(t, err, ErrDimensionMismatch)
	var ce *CodecError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "mock", ce.Codec)
	assert.Contains(t, err.Error(), "4x4")
	assert.Contains(t, err.Error(), "3x3")
}

func TestCodecAdapter_BackendErrorIsWrappedWithCodecName(t *testing.T) {
	backend := errors.New("boom")
	mock := &codecAdapter{
		name:   "mock",
		encode: func(io.Writer, image.Image) error { return backend },
		decode: func([]byte, int, int) (image.Image, error) { return nil, backend },
	}
	_, err := mock.Decode(nil, 1, 1)
	assert.ErrorIs(t, err, backend)
	err = mock.Encode(io.Discard, nil)
	assert.ErrorIs(t, err, backend)
	var ce *CodecError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "mock", ce.Codec)
}

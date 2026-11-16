package dicos

import (
	"bytes"
	_ "embed"
	"errors"
	"image"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
	"github.com/jpfielding/jpegs/pkg/compress/jpeg2k"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Written by the pre-v0.3.0 jpegs encoder (raw-coefficient layout), i.e. what
// dicos.go v0.2.0 and earlier stored under the J2K transfer syntax.
//
//go:embed testdata/legacy-go-jpeg2k-gradient-16x16.j2c
var legacyJ2K []byte

func TestJPEG2000Codec_Gray16RoundTrip(t *testing.T) {
	codec := CodecByTransferSyntax(string(transfer.JPEG2000Lossless))
	require.NotNil(t, codec)

	img := image.NewGray16(image.Rect(0, 0, 24, 17))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 37)
	}
	var buf bytes.Buffer
	require.NoError(t, codec.Encode(&buf, img))

	out, err := codec.Decode(buf.Bytes(), 24, 17)
	require.NoError(t, err)
	got, ok := out.(*image.Gray16)
	require.True(t, ok, "got %T", out)
	assert.Equal(t, img.Pix, got.Pix)
}

// The conformant J2K profile (as dicos.rs) encodes 16-bit gray only.
func TestJPEG2000Codec_Gray8EncodeUnsupported(t *testing.T) {
	codec := CodecByTransferSyntax(string(transfer.JPEG2000Lossless))
	err := codec.Encode(&bytes.Buffer{}, image.NewGray(image.Rect(0, 0, 8, 8)))
	require.Error(t, err)
	assert.True(t, errors.Is(err, jpeg2k.ErrUnsupported), "got %v", err)
}

// Files written by earlier dicos.go releases must still decode through the
// registry (jpeg2k.Decode auto-detects the legacy layout by fingerprint).
func TestJPEG2000Codec_DecodesLegacyStreams(t *testing.T) {
	codec := CodecByTransferSyntax(string(transfer.JPEG2000Lossless))
	out, err := codec.Decode(legacyJ2K, 16, 16)
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 16, 16), out.Bounds())
}

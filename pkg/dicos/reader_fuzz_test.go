package dicos

import (
	"bytes"
	"image"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/jpegs/pkg/compress/jpeg2k"
	"github.com/jpfielding/jpegs/pkg/compress/jpegli"
	"github.com/jpfielding/jpegs/pkg/compress/jpegls"
	"github.com/jpfielding/jpegs/pkg/compress/rle"
)

// FuzzParse feeds arbitrary bytes to the reader with a small element limit.
// Run only under a memory cap, for example:
//
//	capped -mem 1024 -timeout 60s -- go test -run '^$' -fuzz FuzzParse -fuzztime 30s ./pkg/dicos
func FuzzParse(f *testing.F) {
	f.Add(buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
		wireElem{tag.PixelData, "OW", []byte{1, 0, 2, 0, 3, 0, 4, 0}},
	))
	f.Add(buildDicosWithoutTransferSyntax())
	enc := appendEncapsulatedHeader(buildMinimalExplicitVRLE(wireElem{tag.Rows, "US", u16le(2)}))
	enc = appendBOT(enc, 0)
	enc = appendFragment(enc, []byte{1, 2})
	f.Add(appendSeqDelim(enc))
	f.Add(append(buildMinimalExplicitVRLE(), wrapUndefinedLengthSQ(wrapUndefinedLengthSQ(nil))...))
	for _, codec := range []Codec{CodecRLE, CodecJPEGLS, CodecJPEGLi} {
		var frame bytes.Buffer
		if err := codec.Encode(&frame, image.NewGray16(image.Rect(0, 0, 2, 2))); err != nil {
			f.Fatal(err)
		}
		seed := buildWithTS(codec.TransferSyntaxUID(),
			wireElem{tag.Rows, "US", u16le(2)},
			wireElem{tag.Columns, "US", u16le(2)},
			wireElem{tag.BitsAllocated, "US", u16le(16)})
		seed = appendBOT(appendEncapsulatedHeader(seed))
		f.Add(appendSeqDelim(appendFragment(seed, frame.Bytes())))
	}

	// Small decode caps: the defaults are sized for real files.
	caps := []*int64{&jpegls.MaxDecodedBytes, &jpegli.MaxDecodedBytes, &rle.MaxDecodedBytes, &jpeg2k.MaxDecodedBytes}
	saved := make([]int64, len(caps))
	for i, c := range caps {
		saved[i], *c = *c, 4<<20
	}
	f.Cleanup(func() {
		for i, c := range caps {
			*c = saved[i]
		}
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(bytes.NewReader(data))
		r.MaxElementBytes = 1 << 20
		ds, err := r.ReadDataset()
		if err != nil {
			return
		}
		_, _ = VolumeFromDataset(ds)
		// Decoded volumes grow with every frame that decodes; bound the
		// declared total so a run of decompression bombs stays small.
		if pd, err := ds.GetPixelData(); err == nil && satMul(satMul(ds.Rows(), ds.Columns()), len(pd.Frames)) <= 1<<20 {
			_, _ = DecodeVolume(ds)
		}
	})
}

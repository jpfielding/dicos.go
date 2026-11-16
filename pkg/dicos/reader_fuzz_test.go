package dicos

import (
	"bytes"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
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

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(bytes.NewReader(data))
		r.MaxElementBytes = 1 << 20
		_, _ = r.ReadDataset()
	})
}

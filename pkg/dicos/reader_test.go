package dicos

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reader tests ported from dicos.rs reader/tests.rs, plus Go regressions.

func parseBytes(data []byte) (*Dataset, error) { return Parse(bytes.NewReader(data)) }

func TestParse_MinimalExplicitVRLE(t *testing.T) {
	data := buildMinimalExplicitVRLE(
		wireElem{tag.PatientName, "PN", padStr("DOE^JOHN")},
		wireElem{tag.Rows, "US", u16le(512)},
		wireElem{tag.Columns, "US", u16le(256)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
	)
	ds, err := parseBytes(data)
	require.NoError(t, err)

	name, _ := ds.Elements[tag.PatientName].GetString()
	assert.Equal(t, "DOE^JOHN", name)
	assert.Equal(t, 512, ds.Rows())
	assert.Equal(t, 256, ds.Columns())
	assert.Equal(t, 16, ds.BitsAllocated())
	assert.Equal(t, transfer.ExplicitVRLittleEndian, ds.TransferSyntax())
}

func TestParse_DetectsMissingMagic(t *testing.T) {
	data := append(make([]byte, 128), "XXXX"...)
	_, err := parseBytes(data)
	assert.ErrorIs(t, err, ErrBadPreamble)
}

func TestParse_TooShort(t *testing.T) {
	_, err := parseBytes(make([]byte, 50))
	assert.ErrorIs(t, err, ErrBadPreamble)
}

func TestParse_RejectsBigEndianTransferSyntax(t *testing.T) {
	_, err := parseBytes(buildMinimalDicosWithTS(string(transfer.ExplicitVRBigEndian)))
	assert.ErrorIs(t, err, ErrUnsupportedTransferSyntax)
}

func TestParse_WithoutTransferSyntaxDefaultsToImplicitVRLE(t *testing.T) {
	ds, err := parseBytes(buildDicosWithoutTransferSyntax())
	require.NoError(t, err)
	assert.Equal(t, transfer.ImplicitVRLittleEndian, ds.TransferSyntax(),
		"the inferred transfer syntax must be materialised in the dataset")
	assert.Equal(t, 64, ds.Rows())
}

// Row 6: the VR mode must switch on leaving group 0002, not when (0002,0010)
// is read. Group 0002 is explicit even for an Implicit VR file.
func TestParse_ImplicitVRMetaElementsAfterTransferSyntaxStayExplicit(t *testing.T) {
	buf := preambleAndMagic()
	buf = appendExplicit(buf, wireElem{tag.TransferSyntaxUID, "UI", padStr(string(transfer.ImplicitVRLittleEndian))})
	buf = appendExplicit(buf, wireElem{tag.ImplementationClassUID, "UI", padStr("1.2.3.4.5")})
	buf = appendExplicit(buf, wireElem{tag.ImplementationVersionName, "SH", padStr("GOTEST")})
	buf = appendImplicit(buf, tag.Rows, u16le(7))

	ds, err := parseBytes(buf)
	require.NoError(t, err)
	uid, ok := ds.Elements[tag.ImplementationClassUID].GetString()
	require.True(t, ok)
	assert.Equal(t, "1.2.3.4.5", uid)
	name, _ := ds.Elements[tag.ImplementationVersionName].GetString()
	assert.Equal(t, "GOTEST", name)
	assert.Equal(t, 7, ds.Rows())
}

func TestParse_CleanEOFAtBoundaryVsPartialHeader(t *testing.T) {
	data := buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
	)
	ds, err := parseBytes(data[:132])
	require.NoError(t, err, "clean EOF after magic parses")
	assert.Empty(t, ds.Elements)

	_, err = parseBytes(data[:133])
	var te *TruncatedError
	require.ErrorAs(t, err, &te, "a partial tag must be truncated")
	assert.Equal(t, int64(132), te.Offset)
}

func TestParse_TruncationAtEveryPrefixNeverPanicsOrSilentlyMatches(t *testing.T) {
	data := buildMinimalExplicitVRLE(
		wireElem{tag.PatientName, "PN", padStr("DOE^JOHN")},
		wireElem{tag.Rows, "US", u16le(4)},
		wireElem{tag.Columns, "US", u16le(4)},
		wireElem{tag.BitsAllocated, "US", u16le(16)},
	)
	full, err := parseBytes(data)
	require.NoError(t, err)
	fullLen := len(full.Elements)

	for i := range data {
		ds, err := parseBytes(data[:i])
		if err == nil {
			assert.Less(t, len(ds.Elements), fullLen, "prefix %d silently matched the full parse", i)
			continue
		}
		assert.True(t, errors.Is(err, ErrTruncated) || errors.Is(err, ErrBadPreamble),
			"prefix %d produced an unexpected error: %v", i, err)
	}
}

func TestParse_EncapsulatedBOTLengthBombRejectedWithoutHugeAlloc(t *testing.T) {
	data := buildMinimalExplicitVRLE(
		wireElem{tag.Rows, "US", u16le(2)},
		wireElem{tag.Columns, "US", u16le(2)},
	)
	data = appendEncapsulatedHeader(data)
	data = appendTag(data, tag.Item)
	data = append(data, u32le(0xFFFFFFFC)...) // ~4 GiB BOT, no body

	r := NewReader(bytes.NewReader(data))
	r.MaxElementBytes = 1 << 20
	_, err := r.ReadDataset()
	assert.ErrorIs(t, err, ErrTruncated)
}

func TestParse_ElementLengthOverLimitIsRejected(t *testing.T) {
	data := buildMinimalExplicitVRLE()
	data = appendTag(data, tag.PatientName)
	data = append(data, "UT"...) // long VR, 4-byte length
	data = append(data, 0, 0)
	data = append(data, u32le(2<<20)...)

	r := NewReader(bytes.NewReader(data))
	r.MaxElementBytes = 1 << 20
	_, err := r.ReadDataset()
	var le *LengthError
	require.ErrorAs(t, err, &le)
	assert.ErrorIs(t, err, ErrLengthExceedsLimit)
	assert.Equal(t, int64(2<<20), le.Length)
	assert.Equal(t, int64(1<<20), le.Max)
}

// A declared length under the limit but far beyond the bytes present must
// fail as truncated after allocating only what arrived, not the declared size.
func TestParse_LargeDeclaredLengthReadsIncrementally(t *testing.T) {
	data := buildMinimalExplicitVRLE()
	data = appendTag(data, tag.PatientName)
	data = append(data, "OB"...)
	data = append(data, 0, 0)
	data = append(data, u32le(DefaultMaxElementBytes-16)...)
	data = append(data, make([]byte, 3*readChunk/2)...) // 1.5 chunks present

	_, err := parseBytes(data)
	assert.ErrorIs(t, err, ErrTruncated)
}

func TestParse_FragmentLengthOverLimitIsRejected(t *testing.T) {
	data := buildMinimalExplicitVRLE()
	data = appendEncapsulatedHeader(data)
	data = appendBOT(data)
	data = appendTag(data, tag.Item)
	data = append(data, u32le(0xFFFFFFF0)...)

	r := NewReader(bytes.NewReader(data))
	r.MaxElementBytes = 1 << 20
	_, err := r.ReadDataset()
	assert.ErrorIs(t, err, ErrLengthExceedsLimit)
}

func TestParse_EncapsulatedUnexpectedTag(t *testing.T) {
	data := buildMinimalExplicitVRLE()
	data = appendEncapsulatedHeader(data)
	data = appendTag(data, tag.Rows) // not an Item
	data = append(data, u32le(0)...)
	_, err := parseBytes(data)
	var ute *UnexpectedTagError
	require.ErrorAs(t, err, &ute)
	assert.Equal(t, tag.Item, ute.Expected)
	assert.Equal(t, tag.Rows, ute.Got)
	assert.ErrorIs(t, err, ErrUnexpectedTag)
}

// Row 5: a truncated stream inside a skipped undefined-length value must be
// reported, not treated as a clean end.
func TestParse_TruncatedInsideSkippedSequence(t *testing.T) {
	data := buildMinimalExplicitVRLE()
	data = appendTag(data, Tag{Group: 0x0009, Element: 0x0010}) // private, UN
	data = append(data, "UN"...)
	data = append(data, 0, 0)
	data = append(data, u32le(undefinedLength)...)
	data = appendTag(data, tag.Item)
	data = append(data, u32le(undefinedLength)...)
	data = append(data, 0x08, 0x00) // half a tag

	_, err := parseBytes(data)
	assert.ErrorIs(t, err, ErrTruncated)
}

// Row 16: a stray group FFFE tag at the top level is read as a 4-byte length
// with no VR and parsing continues past it.
func TestParse_StrayDelimiterAtTopLevelDoesNotDesync(t *testing.T) {
	data := buildMinimalExplicitVRLE(wireElem{tag.Rows, "US", u16le(2)})
	data = appendSeqDelim(data)
	data = appendExplicit(data, wireElem{tag.Columns, "US", u16le(3)})

	ds, err := parseBytes(data)
	require.NoError(t, err)
	assert.Equal(t, 2, ds.Rows())
	assert.Equal(t, 3, ds.Columns(), "parsing must continue after a stray delimiter")
}

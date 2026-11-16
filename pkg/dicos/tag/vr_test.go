package tag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVRCoversAllTags fails when a tag is added to tag.go without a VR.
// Several names alias one tag value, so coverage is by distinct value.
func TestVRCoversAllTags(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "tag.go", nil, 0)
	require.NoError(t, err)
	distinct := map[string]bool{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for _, v := range vs.Values {
				lit, ok := v.(*ast.CompositeLit)
				if !ok || len(lit.Elts) != 2 {
					continue
				}
				g := lit.Elts[0].(*ast.BasicLit).Value
				if strings.EqualFold(g, "0xFFFE") {
					continue // item/delimitation tags carry no VR
				}
				distinct[g+","+lit.Elts[1].(*ast.BasicLit).Value] = true
			}
		}
	}
	assert.Equal(t, len(distinct), len(vrByTag), "every non-delimiter tag in tag.go needs an entry in vrByTag")
}

func TestVR_KnownTags(t *testing.T) {
	cases := map[Tag]string{
		Manufacturer:              "LO",
		InstitutionName:           "LO",
		ImplementationVersionName: "SH",
		ReferencedSOPInstanceUID:  "UI",
		ThreatCategoryDescription: "UT",
		BoundingBoxTopLeft:        "FL",
		Rows:                      "US",
		PixelData:                 "OW",
	}
	for tg, want := range cases {
		got, ok := VR(tg)
		assert.True(t, ok, tg)
		assert.Equal(t, want, got, tg)
	}
	_, ok := VR(Item)
	assert.False(t, ok)
}

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos"
	"github.com/spf13/cobra"
)

// NewDumpCmd dumps all metadata as JSON in the same shape as dicos.rs
// `dicosctl dump`.
func NewDumpCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "dump <file>",
		Short:        "Dump all metadata as JSON",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			compact, _ := cmd.Flags().GetBool("compact")
			ds, err := openDataset(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetEscapeHTML(false)
			if !compact {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(datasetJSON(ds))
		},
	}
	cmd.Flags().BoolP("compact", "c", false, "Emit compact (single-line) JSON instead of pretty-printed")
	return cmd
}

// openDataset parses the file at path, returning the parse error.
func openDataset(path string) (*dicos.Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return dicos.Parse(bufio.NewReader(f))
}

// datasetJSON keys each element by its tag name, or "(GGGG,EEEE)" when the
// tag has no name. Map keys marshal sorted, as serde_json's default map.
func datasetJSON(ds *dicos.Dataset) map[string]any {
	out := make(map[string]any, len(ds.Elements))
	for _, e := range ds.Elements {
		key := e.Tag.LookupName()
		if key == "" {
			key = fmt.Sprintf("(%04X,%04X)", e.Tag.Group, e.Tag.Element)
		}
		out[key] = elementJSON(e)
	}
	return out
}

// rsFloat marshals a float the way dicos.rs (serde_json) prints it:
// shortest round-trip digits, integers with a trailing ".0", decimal
// notation for decimal exponents in [-5, 16), scientific ("1e+16",
// "2.5e-7") outside it. float32 values are widened to float64 first.
type rsFloat float64

func (f rsFloat) MarshalJSON() ([]byte, error) {
	v := float64(f)
	sign := ""
	if math.Signbit(v) {
		sign, v = "-", -v
	}
	if v == 0 {
		return []byte(sign + "0.0"), nil
	}
	// "d.ddde±XX": shortest digits and the exponent of the first digit.
	sci := strconv.FormatFloat(v, 'e', -1, 64)
	mant, expStr, _ := strings.Cut(sci, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, err := strconv.Atoi(expStr)
	if err != nil {
		return nil, err
	}
	n := len(digits)
	kk := exp + 1 // position of the decimal point relative to the digits
	var out string
	switch {
	case kk >= n && kk <= 16:
		out = digits + strings.Repeat("0", kk-n) + ".0"
	case kk > 0 && kk <= 16:
		out = digits[:kk] + "." + digits[kk:]
	case kk > -5 && kk <= 0:
		out = "0." + strings.Repeat("0", -kk) + digits
	default:
		m := digits[:1]
		if n > 1 {
			m += "." + digits[1:]
		}
		e := strconv.Itoa(kk - 1)
		if kk-1 >= 0 {
			e = "+" + e
		}
		out = m + "e" + e
	}
	return []byte(sign + out), nil
}

// jsonFloat renders non-finite floats as null, as serde_json does.
func jsonFloat(f float64) any {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return rsFloat(f)
}

func jsonFloat32(f float32) any { return jsonFloat(float64(f)) }

func elementJSON(e *dicos.Element) map[string]any {
	vr := e.VR
	value := func(v any) map[string]any { return map[string]any{"vr": vr, "Value": v} }
	length := func(n int) map[string]any { return map[string]any{"vr": vr, "Length": n} }

	switch v := e.Value.(type) {
	case string:
		s := strings.ToValidUTF8(v, "�")
		// dicos.rs splits multi-valued strings into an array; the Go
		// value stays joined (Q5), so split for output only.
		if strings.Contains(s, `\`) {
			return value(strings.Split(s, `\`))
		}
		return value(s)
	case uint16, uint32, int16, int32, int:
		return value(v)
	case []uint16:
		return value(v)
	case float32:
		return value(jsonFloat32(v))
	case float64:
		return value(jsonFloat(v))
	case []float32:
		vs := make([]any, len(v))
		for i, f := range v {
			vs[i] = jsonFloat32(f)
		}
		return value(vs)
	case []float64:
		vs := make([]any, len(v))
		for i, f := range v {
			vs[i] = jsonFloat(f)
		}
		return value(vs)
	case []uint32:
		// dicos.rs keeps multi-valued UL as raw bytes.
		return length(4 * len(v))
	case []byte:
		return length(len(v))
	case nil:
		return length(0)
	case []*dicos.Dataset:
		items := make([]any, len(v))
		for i, item := range v {
			items[i] = datasetJSON(item)
		}
		return map[string]any{"vr": "SQ", "Value": items}
	case *dicos.PixelData:
		return map[string]any{"vr": "OW", "Encapsulated": v.IsEncapsulated(), "Frames": v.NumFrames()}
	}
	return map[string]any{"vr": vr}
}

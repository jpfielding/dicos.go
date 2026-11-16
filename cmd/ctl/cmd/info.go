package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/spf13/cobra"
)

// NewInfoCmd prints a human-readable summary in the same format as dicos.rs
// `dicosctl info`.
func NewInfoCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:          "info <file>",
		Short:        "Print a human-readable summary of the file",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			ds, err := openDataset(path)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "File: %s\n", path)
			fmt.Fprintf(w, "Elements: %d\n", len(ds.Elements))
			if s, ok := firstString(ds, tag.Modality); ok {
				fmt.Fprintf(w, "Modality: %s\n", s)
			}
			rows, rowsOK := uint16Value(ds, tag.Rows)
			cols, colsOK := uint16Value(ds, tag.Columns)
			if rowsOK && colsOK && rows > 0 && cols > 0 {
				fmt.Fprintf(w, "Dimensions: %dx%d\n", cols, rows)
			}
			if frames := ds.NumberOfFrames(); frames > 1 {
				fmt.Fprintf(w, "Frames: %d\n", frames)
			}
			if bits, ok := uint16Value(ds, tag.BitsAllocated); ok {
				fmt.Fprintf(w, "Bits Allocated: %d\n", bits)
			}
			ts := ds.TransferSyntax()
			fmt.Fprintf(w, "Transfer Syntax: %s (%s)\n", ts.Name(), string(ts))
			for _, f := range []struct {
				label string
				tag   tag.Tag
			}{
				{"SOP Class", tag.SOPClassUID},
				{"SOP Instance", tag.SOPInstanceUID},
				{"Patient Name", tag.PatientName},
				{"Series", tag.SeriesDescription},
				{"Manufacturer", tag.Manufacturer},
				{"Model", tag.ManufacturerModelName},
			} {
				if s, ok := firstString(ds, f.tag); ok {
					fmt.Fprintf(w, "%s: %s\n", f.label, s)
				}
			}
			return nil
		},
	}
}

// firstString returns a string element's first value (dicos.rs get_string).
func firstString(ds *dicos.Dataset, t tag.Tag) (string, bool) {
	e, ok := ds.Elements[t]
	if !ok {
		return "", false
	}
	s, ok := e.GetString()
	if !ok {
		return "", false
	}
	first, _, _ := strings.Cut(s, `\`)
	return first, true
}

// uint16Value returns a US element's single value (dicos.rs get_u16).
func uint16Value(ds *dicos.Dataset, t tag.Tag) (uint16, bool) {
	e, ok := ds.Elements[t]
	if !ok {
		return 0, false
	}
	return e.GetUint16()
}

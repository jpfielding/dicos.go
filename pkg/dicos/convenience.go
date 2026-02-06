package dicos

import (
	"fmt"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
)

// QuickValidate performs basic structural validation of a DICOM dataset.
//
// This is a lightweight check for common issues, not a full DICOM compliance check.
// For comprehensive validation, use ValidateCT(), ValidateDX(), or ValidateTDR().
//
// Checks:
//   - SOP Class UID and SOP Instance UID present
//   - Transfer Syntax UID present
//   - If pixel data exists, Rows and Columns are specified
//
// Returns an empty slice if valid, or a slice of errors describing issues.
//
// Example:
//
//	ds, _ := dicos.ReadFile("scan.dcs")
//	if errs := dicos.QuickValidate(ds); len(errs) > 0 {
//		for _, err := range errs {
//			log.Printf("Validation error: %v", err)
//		}
//	}
func QuickValidate(ds *Dataset) []error {
	var errs []error

	// Check SOP Class UID
	if _, ok := ds.FindElement(tag.SOPClassUID.Group, tag.SOPClassUID.Element); !ok {
		errs = append(errs, fmt.Errorf("missing required element: SOP Class UID (0008,0016)"))
	}

	// Check SOP Instance UID
	if _, ok := ds.FindElement(tag.SOPInstanceUID.Group, tag.SOPInstanceUID.Element); !ok {
		errs = append(errs, fmt.Errorf("missing required element: SOP Instance UID (0008,0018)"))
	}

	// Check Transfer Syntax UID
	if _, ok := ds.FindElement(tag.TransferSyntaxUID.Group, tag.TransferSyntaxUID.Element); !ok {
		errs = append(errs, fmt.Errorf("missing required element: Transfer Syntax UID (0002,0010)"))
	}

	// If pixel data is present, check image dimensions
	if pixelElem, ok := ds.FindElement(tag.PixelData.Group, tag.PixelData.Element); ok {
		if pixelElem.Value != nil {
			rows := GetRows(ds)
			cols := GetColumns(ds)
			if rows == 0 {
				errs = append(errs, fmt.Errorf("pixel data present but Rows (0028,0010) is missing or zero"))
			}
			if cols == 0 {
				errs = append(errs, fmt.Errorf("pixel data present but Columns (0028,0011) is missing or zero"))
			}
		}
	}

	return errs
}

// AddSequenceItem appends a dataset item to an existing sequence element.
//
// If the sequence doesn't exist, it creates a new one. This simplifies
// building sequences incrementally.
//
// Example:
//
//	// Create TDR with multiple PTO items
//	tdr, _ := dicos.NewDataset(...)
//
//	pto1, _ := dicos.NewDataset(
//		dicos.WithElement(tag.ReferencedSOPClassUID, sopClass),
//		dicos.WithElement(tag.ReferencedSOPInstanceUID, instance1),
//	)
//	dicos.AddSequenceItem(tdr, tag.ReferencedImageSequence, pto1)
//
//	pto2, _ := dicos.NewDataset(...)
//	dicos.AddSequenceItem(tdr, tag.ReferencedImageSequence, pto2)
func AddSequenceItem(ds *Dataset, t Tag, item *Dataset) error {
	if item == nil {
		return fmt.Errorf("cannot add nil dataset to sequence")
	}

	elem, exists := ds.FindElement(t.Group, t.Element)
	if !exists {
		// Create new sequence
		ds.Elements[t] = &Element{
			Tag:   t,
			VR:    "SQ",
			Value: []*Dataset{item},
		}
		return nil
	}

	// Append to existing sequence
	seq, ok := elem.Value.([]*Dataset)
	if !ok {
		return fmt.Errorf("element %v exists but is not a sequence (VR=%s)", t, elem.VR)
	}

	elem.Value = append(seq, item)
	return nil
}

// GetSequenceItems returns all items from a sequence element.
//
// Returns nil if the element doesn't exist or isn't a sequence.
//
// Example:
//
//	items := dicos.GetSequenceItems(ds, tag.ReferencedImageSequence)
//	for i, item := range items {
//		sopClass, _ := item.FindElement(tag.ReferencedSOPClassUID.Group, tag.ReferencedSOPClassUID.Element)
//		fmt.Printf("Item %d: %v\n", i, sopClass)
//	}
func GetSequenceItems(ds *Dataset, t Tag) []*Dataset {
	elem, ok := ds.FindElement(t.Group, t.Element)
	if !ok {
		return nil
	}

	seq, ok := elem.Value.([]*Dataset)
	if !ok {
		return nil
	}

	return seq
}

// HasElement returns true if the dataset contains the specified element.
//
// Example:
//
//	if dicos.HasElement(ds, tag.PatientName) {
//		name, _ := ds.FindElement(tag.PatientName.Group, tag.PatientName.Element)
//		fmt.Printf("Patient: %v\n", name.Value)
//	}
func HasElement(ds *Dataset, t Tag) bool {
	_, ok := ds.FindElement(t.Group, t.Element)
	return ok
}

// DeleteElement removes an element from the dataset.
//
// Example:
//
//	// Remove patient identifying information
//	dicos.DeleteElement(ds, tag.PatientName)
//	dicos.DeleteElement(ds, tag.PatientID)
func DeleteElement(ds *Dataset, t Tag) {
	delete(ds.Elements, t)
}

// CloneDataset creates a deep copy of a dataset.
//
// This is useful when you need to modify a dataset without affecting the original.
//
// Note: Pixel data is not deep copied for performance reasons. Both datasets
// will reference the same underlying pixel data.
//
// Example:
//
//	original, _ := dicos.ReadFile("scan.dcs")
//	modified := dicos.CloneDataset(original)
//	modified.Elements[tag.SeriesNumber] = &dicos.Element{
//		Tag:   tag.SeriesNumber,
//		VR:    "IS",
//		Value: uint16(2),
//	}
//	// original is unchanged
func CloneDataset(ds *Dataset) *Dataset {
	clone := &Dataset{
		Elements: make(map[Tag]*Element, len(ds.Elements)),
	}

	for t, elem := range ds.Elements {
		// Create new element with same properties
		clonedElem := &Element{
			Tag: elem.Tag,
			VR:  elem.VR,
		}

		// Clone value based on type
		switch v := elem.Value.(type) {
		case string:
			clonedElem.Value = v
		case []byte:
			copied := make([]byte, len(v))
			copy(copied, v)
			clonedElem.Value = copied
		case []*Dataset:
			// Clone sequence items
			clonedSeq := make([]*Dataset, len(v))
			for i, item := range v {
				clonedSeq[i] = CloneDataset(item)
			}
			clonedElem.Value = clonedSeq
		case *PixelData:
			// Pixel data is NOT deep copied (performance)
			clonedElem.Value = v
		default:
			// For other types (primitives, slices), just copy reference
			clonedElem.Value = v
		}

		clone.Elements[t] = clonedElem
	}

	return clone
}

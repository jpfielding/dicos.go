package dicos

import (
	"fmt"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
)

// AttributeType represents DICOM attribute type requirements
type AttributeType int

const (
	// Type1 - Required, must have value
	Type1 AttributeType = 1
	// Type1C - Conditionally required, must have value if present
	Type1C AttributeType = 2
	// Type2 - Required, may be empty
	Type2 AttributeType = 3
	// Type2C - Conditionally required, may be empty if present
	Type2C AttributeType = 4
	// Type3 - Optional
	Type3 AttributeType = 5
)

// ValidationError represents a single validation failure
type ValidationError struct {
	Tag        tag.Tag
	Type       AttributeType
	Message    string
	IsCritical bool // Type 1 and 1C violations are critical
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("(%04X,%04X) %s: %s", e.Tag.Group, e.Tag.Element, e.typeName(), e.Message)
}

func (e ValidationError) typeName() string {
	switch e.Type {
	case Type1:
		return "Type 1"
	case Type1C:
		return "Type 1C"
	case Type2:
		return "Type 2"
	case Type2C:
		return "Type 2C"
	case Type3:
		return "Type 3"
	default:
		return "Unknown"
	}
}

// ValidationResult contains all validation errors for a dataset
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
}

// IsValid returns true if there are no critical errors
func (r ValidationResult) IsValid() bool {
	for _, err := range r.Errors {
		if err.IsCritical {
			return false
		}
	}
	return true
}

// HasErrors returns true if there are any errors
func (r ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any warnings
func (r ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// CriticalErrors returns only the critical validation errors.
//
// Critical errors are Type 1 and Type 1C violations that prevent
// the dataset from being DICOM-compliant.
//
// Example:
//
//	result := dicos.ValidateCT(ds)
//	critical := result.CriticalErrors()
//	if len(critical) > 0 {
//		log.Fatal("Cannot write non-compliant DICOM file")
//	}
func (r ValidationResult) CriticalErrors() []ValidationError {
	var critical []ValidationError
	for _, err := range r.Errors {
		if err.IsCritical {
			critical = append(critical, err)
		}
	}
	return critical
}

// AllMessages returns all error and warning messages as strings.
//
// This is useful for logging or displaying validation results.
//
// Example:
//
//	result := dicos.ValidateCT(ds)
//	for _, msg := range result.AllMessages() {
//		log.Println(msg)
//	}
func (r ValidationResult) AllMessages() []string {
	messages := make([]string, 0, len(r.Errors)+len(r.Warnings))
	for _, err := range r.Errors {
		messages = append(messages, "ERROR: "+err.Error())
	}
	for _, warn := range r.Warnings {
		messages = append(messages, "WARNING: "+warn.Error())
	}
	return messages
}

// Summary returns a formatted summary string of validation results.
//
// Example:
//
//	result := dicos.ValidateCT(ds)
//	fmt.Println(result.Summary())
//	// Output: "Valid: true, Errors: 0, Warnings: 0"
func (r ValidationResult) Summary() string {
	return fmt.Sprintf("Valid: %v, Errors: %d, Warnings: %d",
		r.IsValid(), len(r.Errors), len(r.Warnings))
}

// String returns a detailed string representation of all validation results.
//
// Example:
//
//	result := dicos.ValidateCT(ds)
//	fmt.Println(result.String())
func (r ValidationResult) String() string {
	if len(r.Errors) == 0 && len(r.Warnings) == 0 {
		return "Validation passed with no errors or warnings"
	}

	s := fmt.Sprintf("Validation Result: %s\n", r.Summary())

	if len(r.Errors) > 0 {
		s += "\nErrors:\n"
		for _, err := range r.Errors {
			critical := ""
			if err.IsCritical {
				critical = " [CRITICAL]"
			}
			s += fmt.Sprintf("  - %s%s\n", err.Error(), critical)
		}
	}

	if len(r.Warnings) > 0 {
		s += "\nWarnings:\n"
		for _, warn := range r.Warnings {
			s += fmt.Sprintf("  - %s\n", warn.Error())
		}
	}

	return s
}

// IODRequirement defines a required attribute for an IOD
type IODRequirement struct {
	Tag       tag.Tag
	Type      AttributeType
	Condition func(*Dataset) bool // For Type 1C/2C, returns true if attribute is required
}

// ValidateDataset validates a dataset against a set of requirements
func ValidateDataset(ds *Dataset, requirements []IODRequirement) ValidationResult {
	result := ValidationResult{}

	for _, req := range requirements {
		elem, exists := ds.FindElement(req.Tag.Group, req.Tag.Element)

		switch req.Type {
		case Type1:
			if !exists {
				result.Errors = append(result.Errors, ValidationError{
					Tag:        req.Tag,
					Type:       Type1,
					Message:    "Required attribute missing",
					IsCritical: true,
				})
			} else if isEmpty(elem) {
				result.Errors = append(result.Errors, ValidationError{
					Tag:        req.Tag,
					Type:       Type1,
					Message:    "Required attribute is empty",
					IsCritical: true,
				})
			}

		case Type1C:
			if req.Condition != nil && req.Condition(ds) {
				if !exists {
					result.Errors = append(result.Errors, ValidationError{
						Tag:        req.Tag,
						Type:       Type1C,
						Message:    "Conditionally required attribute missing",
						IsCritical: true,
					})
				} else if isEmpty(elem) {
					result.Errors = append(result.Errors, ValidationError{
						Tag:        req.Tag,
						Type:       Type1C,
						Message:    "Conditionally required attribute is empty",
						IsCritical: true,
					})
				}
			}

		case Type2:
			if !exists {
				result.Warnings = append(result.Warnings, ValidationError{
					Tag:        req.Tag,
					Type:       Type2,
					Message:    "Required attribute missing (may be empty)",
					IsCritical: false,
				})
			}

		case Type2C:
			if req.Condition != nil && req.Condition(ds) && !exists {
				result.Warnings = append(result.Warnings, ValidationError{
					Tag:        req.Tag,
					Type:       Type2C,
					Message:    "Conditionally required attribute missing (may be empty)",
					IsCritical: false,
				})
			}

		case Type3:
			// Optional - no validation needed
		}
	}

	return result
}

// isEmpty checks if an element has no value
func isEmpty(elem *Element) bool {
	if elem == nil {
		return true
	}
	if elem.Value == nil {
		return true
	}
	switch v := elem.Value.(type) {
	case string:
		return v == ""
	case []byte:
		return len(v) == 0
	case []uint16:
		return len(v) == 0
	default:
		return false
	}
}

// Common IOD Requirements

// PatientModuleRequirements defines required attributes for Patient Module
var PatientModuleRequirements = []IODRequirement{
	{Tag: tag.PatientName, Type: Type2},
	{Tag: tag.PatientID, Type: Type2},
}

// GeneralStudyModuleRequirements defines required attributes for General Study Module
var GeneralStudyModuleRequirements = []IODRequirement{
	{Tag: tag.StudyInstanceUID, Type: Type1},
	{Tag: tag.StudyDate, Type: Type2},
	{Tag: tag.StudyTime, Type: Type2},
}

// GeneralSeriesModuleRequirements defines required attributes for General Series Module
var GeneralSeriesModuleRequirements = []IODRequirement{
	{Tag: tag.Modality, Type: Type1},
	{Tag: tag.SeriesInstanceUID, Type: Type1},
}

// ImagePixelModuleRequirements defines required attributes for Image Pixel Module
var ImagePixelModuleRequirements = []IODRequirement{
	{Tag: tag.SamplesPerPixel, Type: Type1},
	{Tag: tag.PhotometricInterpretation, Type: Type1},
	{Tag: tag.Rows, Type: Type1},
	{Tag: tag.Columns, Type: Type1},
	{Tag: tag.BitsAllocated, Type: Type1},
	{Tag: tag.BitsStored, Type: Type1},
	{Tag: tag.HighBit, Type: Type1},
	{Tag: tag.PixelRepresentation, Type: Type1},
	{Tag: tag.PixelData, Type: Type1},
}

// SOPCommonModuleRequirements defines required attributes for SOP Common Module
var SOPCommonModuleRequirements = []IODRequirement{
	{Tag: tag.SOPClassUID, Type: Type1},
	{Tag: tag.SOPInstanceUID, Type: Type1},
}

// CTImageRequirements combines all requirements for CT Image IOD
var CTImageRequirements = append(append(append(append(append(
	PatientModuleRequirements,
	GeneralStudyModuleRequirements...),
	GeneralSeriesModuleRequirements...),
	ImagePixelModuleRequirements...),
	SOPCommonModuleRequirements...),
	// CT-specific
	IODRequirement{Tag: tag.RescaleIntercept, Type: Type1},
	IODRequirement{Tag: tag.RescaleSlope, Type: Type1},
)

// DXImageRequirements combines all requirements for DX Image IOD
var DXImageRequirements = append(append(append(append(
	PatientModuleRequirements,
	GeneralStudyModuleRequirements...),
	GeneralSeriesModuleRequirements...),
	ImagePixelModuleRequirements...),
	SOPCommonModuleRequirements...)

// TDRRequirements combines all requirements for TDR IOD
var TDRRequirements = append(append(append(
	PatientModuleRequirements,
	GeneralStudyModuleRequirements...),
	GeneralSeriesModuleRequirements...),
	SOPCommonModuleRequirements...)

// ValidateCT validates a CT Image dataset
func ValidateCT(ds *Dataset) ValidationResult {
	return ValidateDataset(ds, CTImageRequirements)
}

// ValidateDX validates a DX Image dataset
func ValidateDX(ds *Dataset) ValidationResult {
	return ValidateDataset(ds, DXImageRequirements)
}

// ValidateTDR validates a TDR dataset
func ValidateTDR(ds *Dataset) ValidationResult {
	return ValidateDataset(ds, TDRRequirements)
}

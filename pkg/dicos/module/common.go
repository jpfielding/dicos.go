package module

import (
	"fmt"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
)

// Vector3D represents a 3D vector (x, y, z)
type Vector3D struct {
	X, Y, Z float64
}

// Date represents a DICOS Date (DA VR)
type Date struct {
	Year  int
	Month int
	Day   int
}

func (d Date) String() string {
	return fmt.Sprintf("%04d%02d%02d", d.Year, d.Month, d.Day)
}

func NewDate(t time.Time) Date {
	return Date{
		Year:  t.Year(),
		Month: int(t.Month()),
		Day:   t.Day(),
	}
}

// Time represents a DICOS Time (TM VR)
type Time struct {
	Hour   int
	Minute int
	Second int
	Nano   int
}

func (t Time) String() string {
	// Format as HHMMSS.FFFFFF
	return fmt.Sprintf("%02d%02d%02d.%06d", t.Hour, t.Minute, t.Second, t.Nano/1000)
}

func NewTime(t time.Time) Time {
	return Time{
		Hour:   t.Hour(),
		Minute: t.Minute(),
		Second: t.Second(),
		Nano:   t.Nanosecond(),
	}
}

// PersonName represents a DICOS Person Name (PN VR)
type PersonName struct {
	FamilyName string
	GivenName  string
	MiddleName string
	Prefix     string
	Suffix     string
}

func (p PersonName) String() string {
	// DICOM format: Family^Given^Middle^Prefix^Suffix
	return fmt.Sprintf("%s^%s^%s^%s^%s", p.FamilyName, p.GivenName, p.MiddleName, p.Prefix, p.Suffix)
}

// IODModule defines the interface for DICOM Information Object Definition (IOD) modules.
//
// An IOD module is a collection of related DICOM attributes that describe a specific
// aspect of medical imaging data. Modules are the building blocks of DICOM IODs,
// providing a standardized way to organize metadata.
//
// Common DICOS modules include:
//   - Patient Module: Patient identification and demographic information
//   - Study Module: Study-level metadata (accession number, study date/time)
//   - Series Module: Series-level organization (modality, series number)
//   - Equipment Module: Scanner/detector information
//   - CT Image Module: CT-specific imaging parameters
//   - DX Detector Module: Digital X-ray detector characteristics
//   - VOI LUT Module: Windowing parameters for display
//   - SOP Common Module: Instance identification (SOP Class/Instance UIDs)
//
// Modules are composed into complete IODs. For example, a DICOS CT Image IOD contains:
//   Patient + Study + Series + Equipment + FrameOfReference + CTImage + VOILUT + SOPCommon
//
// Implementations must provide ToTags() which returns a slice of tag/value pairs
// to be added to a Dataset.
//
// Example:
//
//	patient := module.NewPatientModule("DOE^JOHN", "PAT-12345", "19700101", "M")
//	tags := patient.ToTags() // Convert module to DICOM elements
//	ds, _ := dicos.NewDataset(dicos.WithModule(tags))
type IODModule interface {
	ToTags() []IODElement
}

// IODElement represents a single DICOM element as a tag/value pair.
//
// This is used by IODModule implementations to return their constituent elements.
// The Tag identifies the DICOM attribute (e.g., tag.PatientName), and Value contains
// the element's data in the appropriate Go type.
//
// Example:
//
//	func (p *PatientModule) ToTags() []IODElement {
//		return []IODElement{
//			{Tag: tag.PatientName, Value: p.PatientName},
//			{Tag: tag.PatientID, Value: p.PatientID},
//			{Tag: tag.PatientBirthDate, Value: p.PatientBirthDate},
//			{Tag: tag.PatientSex, Value: p.PatientSex},
//		}
//	}
type IODElement struct {
	Tag   tag.Tag
	Value interface{}
}

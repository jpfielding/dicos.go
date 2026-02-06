package module

import "github.com/jpfielding/dicos.go/pkg/dicos/tag"

// PatientModule represents the DICOS Patient Module
// Stratovan: SDICOS::PatientModule
type PatientModule struct {
	PatientName      PersonName
	PatientID        string
	PatientBirthDate Date
	PatientSex       string // M, F, O
	PatientAge       string
	PatientComments  string
	OccupationalFlow string // DICOS specific
	Magistrate       string // DICOS specific
}

// ToTags converts the Patient Module to DICOM elements.
//
// Returns a slice of IODElement containing patient identification and demographic
// information including PatientName (0010,0010), PatientID (0010,0020),
// PatientBirthDate (0010,0030), PatientSex (0010,0040), PatientAge (0010,1010),
// and PatientComments (0010,4000).
func (m *PatientModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.PatientName, Value: m.PatientName.String()},
		{Tag: tag.PatientID, Value: m.PatientID},
		{Tag: tag.PatientBirthDate, Value: m.PatientBirthDate.String()},
		{Tag: tag.PatientSex, Value: m.PatientSex},
		{Tag: tag.PatientAge, Value: m.PatientAge},
		{Tag: tag.PatientComments, Value: m.PatientComments},
		// DICOS specific tags would go here, need to define custom tags if not in standard
	}
}

// SetPatientName sets the patient's name
func (m *PatientModule) SetPatientName(first, last, middle, prefix, suffix string) {
	m.PatientName = PersonName{
		GivenName:  first,
		FamilyName: last,
		MiddleName: middle,
		Prefix:     prefix,
		Suffix:     suffix,
	}
}

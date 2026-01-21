package dicos

import (
	"io"
	"os"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// ThreatDetectionReport represents a DICOS TDR IOD
// Stratovan: SDICOS::ThreatDetectionReport
// SOP Class UID: 1.2.840.10008.5.1.4.1.1.501.3
type ThreatDetectionReport struct {
	// Modules
	Patient   module.PatientModule
	Series    module.GeneralSeriesModule // Specializes to TDRSeries
	Equipment module.GeneralEquipmentModule
	SOPCommon module.SOPCommonModule

	// TDR Specifics
	ContentDate   module.Date
	ContentTime   module.Time
	AlarmDecision string // "ALARM", "NO_ALARM", "UNKNOWN"

	// Referenced Images (source CT/DX that spawned this TDR)
	ReferencedSOPClassUID    string
	ReferencedSOPInstanceUID string

	// PTOs
	PTOs []PotentialThreatObject

	// Configuration
	Codec Codec // nil = uncompressed
}

// PotentialThreatObject represents a detected threat
type PotentialThreatObject struct {
	ID          int
	Label       string // Mapped to ThreatCategoryDescription
	Description string

	// Assessment
	Probability float32 // ATDAssessmentProbability (0.0-1.0)
	Confidence  float32 // ThreatConfidenceScore (0.0-1.0)

	// Material Classification
	OOIType string  // Object of Interest type: FIREARM, KNIFE, EXPLOSIVE, etc.
	Mass    float32 // Estimated mass (grams)
	Volume  float32 // Estimated volume (mm³)

	// Spatial
	BoundingBox *BoundingBox // Optional 3D bounding box
}

type BoundingBox struct {
	TopLeft     [3]float32
	BottomRight [3]float32
}

func NewThreatDetectionReport() *ThreatDetectionReport {
	t := time.Now()
	return &ThreatDetectionReport{
		ContentDate: module.NewDate(t),
		ContentTime: module.NewTime(t),
		PTOs:        make([]PotentialThreatObject, 0),
	}
}

// GetDataset builds and returns the DICOS Dataset
func (tdr *ThreatDetectionReport) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	sopInstanceUID := tdr.SOPCommon.SOPInstanceUID
	if sopInstanceUID == "" {
		sopInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
		tdr.SOPCommon.SOPInstanceUID = sopInstanceUID
	}
	tdr.SOPCommon.SOPClassUID = DICOSTDRStorageUID

	// TDR Storage - transfer syntax
	ts := string(transfer.ExplicitVRLittleEndian)
	if tdr.Codec != nil {
		ts = tdr.Codec.TransferSyntaxUID()
	}

	// File Meta
	opts = append(opts, WithFileMeta(DICOSTDRStorageUID, sopInstanceUID, ts))

	// Modules
	opts = append(opts,
		WithModule(tdr.Patient.ToTags()),
		WithModule(tdr.Series.ToTags()),
		WithModule(tdr.Equipment.ToTags()),
		WithModule(tdr.SOPCommon.ToTags()),
	)

	// Content Date/Time
	opts = append(opts,
		WithElement(tag.ContentDate, tdr.ContentDate.String()),
		WithElement(tag.ContentTime, tdr.ContentTime.String()),
	)

	// Alarm Decision
	if tdr.AlarmDecision != "" {
		opts = append(opts, WithElement(tag.AlarmDecision, tdr.AlarmDecision))
	}

	// Referenced Image Sequence (link to source CT/DX)
	if tdr.ReferencedSOPInstanceUID != "" {
		refOpts := make([]Option, 0, 2)
		if tdr.ReferencedSOPClassUID != "" {
			refOpts = append(refOpts, WithElement(tag.ReferencedSOPClassUID, tdr.ReferencedSOPClassUID))
		}
		refOpts = append(refOpts, WithElement(tag.ReferencedSOPInstanceUID, tdr.ReferencedSOPInstanceUID))
		if refDS, err := NewDataset(refOpts...); err == nil {
			opts = append(opts, WithSequence(tag.ReferencedImageSequence, refDS))
		}
	}

	// PTO Sequence
	if len(tdr.PTOs) > 0 {
		var ptoItems []*Dataset
		for i, pto := range tdr.PTOs {
			id := pto.ID
			if id == 0 {
				id = i + 1
			}

			itemOpts := []Option{WithElement(tag.PotentialThreatObjectID, id)}
			if pto.Label != "" {
				itemOpts = append(itemOpts, WithElement(tag.ThreatCategoryDescription, pto.Label))
			}
			if pto.OOIType != "" {
				itemOpts = append(itemOpts, WithElement(tag.OOIType, pto.OOIType))
			}
			if pto.Probability > 0 {
				itemOpts = append(itemOpts, WithElement(tag.ATDAssessmentProbability, pto.Probability))
			}
			if pto.Confidence > 0 {
				itemOpts = append(itemOpts, WithElement(tag.ThreatConfidenceScore, pto.Confidence))
			}

			// PTO Representation Sequence (bounding box, mass, volume)
			if pto.BoundingBox != nil || pto.Mass > 0 || pto.Volume > 0 {
				repOpts := make([]Option, 0, 4)
				if pto.BoundingBox != nil {
					repOpts = append(repOpts,
						WithElement(tag.BoundingBoxTopLeft, []float32{
							pto.BoundingBox.TopLeft[0],
							pto.BoundingBox.TopLeft[1],
							pto.BoundingBox.TopLeft[2]}),
						WithElement(tag.BoundingBoxBottomRight, []float32{
							pto.BoundingBox.BottomRight[0],
							pto.BoundingBox.BottomRight[1],
							pto.BoundingBox.BottomRight[2]}),
					)
				}
				if pto.Mass > 0 {
					repOpts = append(repOpts, WithElement(tag.OOISize, pto.Mass))
				}
				if repDS, err := NewDataset(repOpts...); err == nil {
					itemOpts = append(itemOpts, WithSequence(tag.PTORepresentationSequence, repDS))
				}
			}

			if ptoDS, err := NewDataset(itemOpts...); err == nil {
				ptoItems = append(ptoItems, ptoDS)
			}
		}
		opts = append(opts, WithSequence(tag.PTOSequence, ptoItems...))
	}

	return NewDataset(opts...)
}

// WriteTo writes the TDR to any io.Writer
func (tdr *ThreatDetectionReport) WriteTo(w io.Writer) (int64, error) {
	dataset, err := tdr.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, dataset)
}

// Write saves the TDR to a DICOS file (convenience wrapper)
func (tdr *ThreatDetectionReport) Write(path string) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return tdr.WriteTo(f)
}

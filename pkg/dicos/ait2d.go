package dicos

import (
	"io"
	"os"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// AIT2DImage represents a DICOS AIT 2D Image IOD (body scanner 2D)
// SOP Class UID: 1.2.840.10008.5.1.4.1.1.501.4
type AIT2DImage struct {
	// Standard Modules
	Patient   module.PatientModule
	Study     module.GeneralStudyModule
	Series    module.GeneralSeriesModule
	Equipment module.GeneralEquipmentModule
	SOPCommon module.SOPCommonModule
	VOILUT    *module.VOILUTModule

	// Image Attributes
	ContentDate       module.Date
	ContentTime       module.Time
	SamplesPerPixel   int
	PhotometricInterp string // MONOCHROME2
	Rows              int
	Columns           int
	BitsAllocated     int
	BitsStored        int
	HighBit           int
	PixelRepresent    int

	// AIT 2D Specific
	BodyRegion    string  // FRONT, BACK, LEFT_SIDE, RIGHT_SIDE
	PrivacyMask   bool    // Privacy mask applied
	ScanViewAngle float64 // Degrees
	ScannerType   string  // MILLIMETER_WAVE, BACKSCATTER

	// Pixel Data
	PixelData []uint16
	Codec     Codec // nil = uncompressed
}

// NewAIT2DImage creates a new AIT 2D Image with defaults
func NewAIT2DImage() *AIT2DImage {
	t := time.Now()
	return &AIT2DImage{
		SamplesPerPixel:   1,
		PhotometricInterp: "MONOCHROME2",
		BitsAllocated:     16,
		BitsStored:        16,
		HighBit:           15,
		PixelRepresent:    0,
		ContentDate:       module.NewDate(t),
		ContentTime:       module.NewTime(t),
		Study:             module.NewGeneralStudyModule(),
		SOPCommon:         module.NewSOPCommonModule(),
		VOILUT:            module.NewVOILUTModule(),
		ScannerType:       "MILLIMETER_WAVE",
	}
}

// SetPixelData sets the raw pixel data
func (ait *AIT2DImage) SetPixelData(rows, cols int, data []uint16) {
	ait.Rows = rows
	ait.Columns = cols
	ait.PixelData = data
}

// GetDataset builds and returns the DICOS Dataset
func (ait *AIT2DImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	sopInstanceUID := ait.SOPCommon.SOPInstanceUID
	if sopInstanceUID == "" {
		sopInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
		ait.SOPCommon.SOPInstanceUID = sopInstanceUID
	}
	ait.SOPCommon.SOPClassUID = DICOSAIT2DImageStorageUID

	// Transfer syntax
	ts := string(transfer.ExplicitVRLittleEndian)
	if ait.Codec != nil {
		ts = ait.Codec.TransferSyntaxUID()
	}

	// File Meta
	opts = append(opts, WithFileMeta(DICOSAIT2DImageStorageUID, sopInstanceUID, ts))

	// Modules
	opts = append(opts,
		WithModule(ait.Patient.ToTags()),
		WithModule(ait.Study.ToTags()),
		WithModule(ait.Series.ToTags()),
		WithModule(ait.Equipment.ToTags()),
		WithModule(ait.SOPCommon.ToTags()),
	)
	if ait.VOILUT != nil {
		opts = append(opts, WithModule(ait.VOILUT.ToTags()))
	}

	// Content Date/Time
	opts = append(opts,
		WithElement(tag.ContentDate, ait.ContentDate.String()),
		WithElement(tag.ContentTime, ait.ContentTime.String()),
	)

	// Image Pixel Module
	opts = append(opts,
		WithElement(tag.SamplesPerPixel, ait.SamplesPerPixel),
		WithElement(tag.PhotometricInterpretation, ait.PhotometricInterp),
		WithElement(tag.Rows, ait.Rows),
		WithElement(tag.Columns, ait.Columns),
		WithElement(tag.BitsAllocated, ait.BitsAllocated),
		WithElement(tag.BitsStored, ait.BitsStored),
		WithElement(tag.HighBit, ait.HighBit),
		WithElement(tag.PixelRepresentation, ait.PixelRepresent),
	)

	// TODO: Add AIT-specific tags when defined in tag package
	// BodyRegion, PrivacyMask, ScanViewAngle, ScannerType

	// Pixel Data
	if len(ait.PixelData) > 0 {
		opts = append(opts, WithPixelData(ait.Rows, ait.Columns, ait.BitsAllocated, ait.PixelData, ait.Codec))
	}

	return NewDataset(opts...)
}

// WriteTo writes the AIT 2D Image to any io.Writer
func (ait *AIT2DImage) WriteTo(w io.Writer) (int64, error) {
	dataset, err := ait.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, dataset)
}

// Write saves the AIT 2D Image to a DICOS file (convenience wrapper)
func (ait *AIT2DImage) Write(path string) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return ait.WriteTo(f)
}

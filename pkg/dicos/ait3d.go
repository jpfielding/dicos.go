package dicos

import (
	"io"
	"os"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// AIT3DImage represents a DICOS AIT 3D Image IOD (body scanner 3D)
// SOP Class UID: 1.2.840.10008.5.1.4.1.1.501.5
type AIT3DImage struct {
	// Standard Modules
	Patient          module.PatientModule
	Study            module.GeneralStudyModule
	Series           module.GeneralSeriesModule
	Equipment        module.GeneralEquipmentModule
	SOPCommon        module.SOPCommonModule
	FrameOfReference *module.FrameOfReferenceModule
	ImagePlane       *module.ImagePlaneModule
	VOILUT           *module.VOILUTModule

	// Image Attributes
	ContentDate       module.Date
	ContentTime       module.Time
	SamplesPerPixel   int
	PhotometricInterp string // MONOCHROME2
	Rows              int
	Columns           int
	NumberOfFrames    int
	BitsAllocated     int
	BitsStored        int
	HighBit           int
	PixelRepresent    int

	// AIT 3D Specific
	SurfaceType      string // POINT_CLOUD, MESH, VOXEL
	CoordinateSystem string // DICOS_BODY_COORDINATE
	ScannerType      string // MILLIMETER_WAVE, BACKSCATTER

	// Volumetric Data
	PixelData        *PixelData
	UseCompression   bool
	CompressionCodec string
}

// NewAIT3DImage creates a new AIT 3D Image with defaults
func NewAIT3DImage() *AIT3DImage {
	t := time.Now()
	return &AIT3DImage{
		SamplesPerPixel:   1,
		PhotometricInterp: "MONOCHROME2",
		BitsAllocated:     16,
		BitsStored:        16,
		HighBit:           15,
		PixelRepresent:    0,
		NumberOfFrames:    1,
		ContentDate:       module.NewDate(t),
		ContentTime:       module.NewTime(t),
		Study:             module.NewGeneralStudyModule(),
		SOPCommon:         module.NewSOPCommonModule(),
		FrameOfReference:  &module.FrameOfReferenceModule{},
		ImagePlane:        module.NewImagePlaneModule(),
		VOILUT:            module.NewVOILUTModule(),
		SurfaceType:       "VOXEL",
		CoordinateSystem:  "DICOS_BODY_COORDINATE",
		ScannerType:       "MILLIMETER_WAVE",
	}
}

// SetPixelData sets native pixel data for 3D volume
func (ait *AIT3DImage) SetPixelData(rows, cols, frames int, data []uint16) {
	ait.Rows = rows
	ait.Columns = cols
	ait.NumberOfFrames = frames

	pixelsPerFrame := rows * cols
	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, frames),
	}

	for i := 0; i < frames; i++ {
		start := i * pixelsPerFrame
		end := start + pixelsPerFrame
		if end > len(data) {
			end = len(data)
		}
		frameData := make([]uint16, end-start)
		copy(frameData, data[start:end])
		pd.Frames[i] = Frame{Data: frameData}
	}
	ait.PixelData = pd
}

// GetDataset builds and returns the DICOS Dataset
func (ait *AIT3DImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	sopInstanceUID := ait.SOPCommon.SOPInstanceUID
	if sopInstanceUID == "" {
		sopInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
		ait.SOPCommon.SOPInstanceUID = sopInstanceUID
	}
	ait.SOPCommon.SOPClassUID = DICOSAIT3DImageStorageUID

	// Transfer syntax
	ts := transfer.ExplicitVRLittleEndian
	if ait.UseCompression {
		switch ait.CompressionCodec {
		case "jpeg-li":
			ts = "1.2.840.10008.1.2.4.70"
		case "rle":
			ts = "1.2.840.10008.1.2.5"
		default:
			ts = transfer.JPEGLSLossless
		}
	}

	// File Meta
	opts = append(opts, WithFileMeta(DICOSAIT3DImageStorageUID, sopInstanceUID, string(ts)))

	// Modules
	opts = append(opts,
		WithModule(ait.Patient.ToTags()),
		WithModule(ait.Study.ToTags()),
		WithModule(ait.Series.ToTags()),
		WithModule(ait.Equipment.ToTags()),
		WithModule(ait.SOPCommon.ToTags()),
	)
	if ait.FrameOfReference != nil {
		opts = append(opts, WithModule(ait.FrameOfReference.ToTags()))
	}
	if ait.ImagePlane != nil {
		opts = append(opts, WithModule(ait.ImagePlane.ToTags()))
	}
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
		WithElement(tag.NumberOfFrames, ait.NumberOfFrames),
		WithElement(tag.BitsAllocated, ait.BitsAllocated),
		WithElement(tag.BitsStored, ait.BitsStored),
		WithElement(tag.HighBit, ait.HighBit),
		WithElement(tag.PixelRepresentation, ait.PixelRepresent),
	)

	// TODO: Add AIT-specific tags when defined in tag package
	// SurfaceType, CoordinateSystem, ScannerType

	// Pixel Data
	if ait.PixelData != nil {
		opts = append(opts, WithRawPixelData(ait.PixelData))
	}

	return NewDataset(opts...)
}

// WriteTo writes the AIT 3D Image to any io.Writer
func (ait *AIT3DImage) WriteTo(w io.Writer) (int64, error) {
	dataset, err := ait.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, dataset)
}

// Write saves the AIT 3D Image to a DICOS file (convenience wrapper)
func (ait *AIT3DImage) Write(path string) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return ait.WriteTo(f)
}

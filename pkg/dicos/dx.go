package dicos

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// DXImage represents a DICOS Digital X-Ray Image IOD
// Stratovan: SDICOS::DXImage
type DXImage struct {
	// Modules
	Patient     module.PatientModule
	Study       module.GeneralStudyModule
	Series      module.GeneralSeriesModule // Specializes to DXSeries
	Equipment   module.GeneralEquipmentModule
	SOPCommon   module.SOPCommonModule
	VOILUT      *module.VOILUTModule        // Window/level presets
	Detector    *module.DXDetectorModule    // Detector parameters
	Acquisition *module.DXAcquisitionModule // X-ray acquisition parameters

	// Image Attributes
	InstanceNumber    int
	ContentDate       module.Date
	ContentTime       module.Time
	ImageType         string // ORIGINAL\PRIMARY
	SamplesPerPixel   int
	PhotometricInterp string // MONOCHROME2
	Rows              int
	Columns           int
	BitsAllocated     int
	BitsStored        int
	HighBit           int
	PixelRepresent    int // 0 unsigned, 1 signed

	// Windowing (legacy - prefer VOILUT module)
	WindowCenter float64
	WindowWidth  float64

	// DX Specifics
	PresentationIntentType string // PRESENTATION or PROCESSING

	// Pixel Data
	PixelData *PixelData
	Codec     Codec // nil = uncompressed

	// Additional Tags (Generic support for tags not explicitly defined)
	AdditionalTags map[tag.Tag]interface{}
}

// NewDXImage creates a new DX Image with default values
func NewDXImage() *DXImage {
	t := time.Now()
	return &DXImage{
		SamplesPerPixel:        1,
		PhotometricInterp:      "MONOCHROME2",
		BitsAllocated:          16,
		BitsStored:             16,
		HighBit:                15,
		PixelRepresent:         0,
		PresentationIntentType: "PRESENTATION",
		WindowCenter:           32768, // Center for 16-bit
		WindowWidth:            65535, // Full width
		ContentDate:            module.NewDate(t),
		ContentTime:            module.NewTime(t),
		Study:                  module.NewGeneralStudyModule(),
		SOPCommon:              module.NewSOPCommonModule(),
		VOILUT:                 module.NewVOILUTModuleForDX(),
		Detector:               module.NewDXDetectorModule(),
		Acquisition:            module.NewDXAcquisitionModule(),
		AdditionalTags:         make(map[tag.Tag]interface{}),
	}
}

// SetPixelData sets native pixel data for the DX image.
//
// This method handles both single-frame and multi-frame images automatically.
// If len(data) > rows*cols, it splits the data into multiple frames.
//
// Parameters:
//   - rows: Image height in pixels
//   - cols: Image width in pixels
//   - data: Pixel values in row-major order (left-to-right, top-to-bottom)
//
// To compress the pixel data, set dx.Codec before calling GetDataset():
//
//	dx.SetPixelData(512, 512, pixelData)
//	dx.Codec = dicos.CodecJPEGLS
//	dx.Write("output.dcs")
func (dx *DXImage) SetPixelData(rows, cols int, data []uint16) {
	dx.Rows = rows
	dx.Columns = cols

	pixelsPerFrame := rows * cols
	numFrames := len(data) / pixelsPerFrame
	if numFrames < 1 {
		numFrames = 1
	}

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, numFrames),
	}

	for i := 0; i < numFrames; i++ {
		start := i * pixelsPerFrame
		end := start + pixelsPerFrame
		if end > len(data) {
			end = len(data)
		}

		frameData := make([]uint16, end-start)
		copy(frameData, data[start:end])
		pd.Frames[i] = Frame{Data: frameData}
	}
	dx.PixelData = pd
}

// GetDataset builds and returns the DICOS Dataset
func (dx *DXImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	// 1. File Meta Information
	tsUID := string(transfer.ExplicitVRLittleEndian)
	if dx.Codec != nil {
		tsUID = dx.Codec.TransferSyntaxUID()
	}

	sopInstanceUID := dx.SOPCommon.SOPInstanceUID
	if sopInstanceUID == "" {
		sopInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
		dx.SOPCommon.SOPInstanceUID = sopInstanceUID
	}
	dx.SOPCommon.SOPClassUID = "1.2.840.10008.5.1.4.1.1.501.2.1"
	if dx.Study.StudyInstanceUID == "" {
		dx.Study.StudyInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	}

	// DX Storage
	opts = append(opts, WithFileMeta("1.2.840.10008.5.1.4.1.1.501.1", sopInstanceUID, tsUID))

	// 2. Modules
	opts = append(opts,
		WithModule(dx.Patient.ToTags()),
		WithModule(dx.Study.ToTags()),
		WithModule(dx.Series.ToTags()),
		WithModule(dx.Equipment.ToTags()),
		WithModule(dx.SOPCommon.ToTags()),
	)
	if dx.Detector != nil {
		opts = append(opts, WithModule(dx.Detector.ToTags()))
	}
	if dx.Acquisition != nil {
		opts = append(opts, WithModule(dx.Acquisition.ToTags()))
	}

	// 3. Image Pixel Module & Common
	opts = append(opts,
		WithElement(tag.Rows, dx.Rows),
		WithElement(tag.Columns, dx.Columns),
		WithElement(tag.BitsAllocated, dx.BitsAllocated),
		WithElement(tag.BitsStored, dx.BitsStored),
		WithElement(tag.HighBit, dx.HighBit),
		WithElement(tag.PixelRepresentation, dx.PixelRepresent),
		WithElement(tag.SamplesPerPixel, dx.SamplesPerPixel),
		WithElement(tag.PhotometricInterpretation, dx.PhotometricInterp),
		WithElement(tag.ImageType, dx.ImageType),
		WithElement(tag.ContentDate, dx.ContentDate.String()),
		WithElement(tag.ContentTime, dx.ContentTime.String()),
		WithElement(tag.InstanceNumber, fmt.Sprintf("%d", dx.InstanceNumber)),
		WithElement(tag.PresentationIntentType, dx.PresentationIntentType),
		WithElement(tag.WindowCenter, fmt.Sprintf("%v", dx.WindowCenter)),
		WithElement(tag.WindowWidth, fmt.Sprintf("%v", dx.WindowWidth)),
	)

	// Additional Tags
	for t, v := range dx.AdditionalTags {
		opts = append(opts, WithElement(t, v))
	}

	// 4. Pixel Data
	if dx.Codec != nil && dx.PixelData != nil && !dx.PixelData.IsEncapsulated {
		flatData := dx.PixelData.GetFlatData()
		opts = append(opts, WithPixelData(dx.Rows, dx.Columns, dx.BitsAllocated, flatData, dx.Codec))
	} else if dx.PixelData != nil {
		opts = append(opts, WithRawPixelData(dx.PixelData))
	}

	return NewDataset(opts...)
}

// WriteTo writes the DX Image to any io.Writer
func (dx *DXImage) WriteTo(w io.Writer) (int64, error) {
	dataset, err := dx.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, dataset)
}

// Write saves the DX Image to a DICOS file (convenience wrapper)
func (dx *DXImage) Write(path string) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return dx.WriteTo(f)
}

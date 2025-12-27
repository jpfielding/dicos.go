package dicos

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// CTImage represents a DICOM CT (Computed Tomography) Image IOD (Information Object Definition).
//
// This high-level structure composes multiple DICOM modules to create a complete CT dataset:
//   - Patient Module: Patient demographics
//   - Study Module: Study-level information
//   - Series Module: Series-level organization
//   - Equipment Module: Scanner information
//   - FrameOfReference Module: Spatial coordinate system
//   - ImagePlane Module: Image position and orientation
//   - CTImageMod: CT-specific imaging parameters (KVP, slice thickness, etc.)
//   - VOILUT Module: Window/level display presets
//   - SOPCommon Module: Instance identification
//
// Guaranteed Non-nil Modules After NewCTImage():
//   - Patient, Study, Series, Equipment, SOPCommon
//   - FrameOfReference, ImagePlane, CTImageMod, VOILUT
//
// These modules are automatically initialized with defaults and can be customized before
// calling GetDataset() to generate the final DICOS file.
//
// Example:
//
//	ct := dicos.NewCTImage()
//	ct.Patient.PatientID = "PAT-12345"
//	ct.Patient.SetPatientName("John", "Doe", "", "", "")
//	ct.Rows = 512
//	ct.Columns = 512
//	ct.SetPixelData(512, 512, pixelData)
//	ct.Codec = dicos.CodecJPEGLS
//	ct.Write("/path/to/output.dcs")
//
// Note: The legacy Image field (CTImageModule with KV map) is deprecated. Use CTImageMod
// (module.CTImageModule) for new code, which provides type-safe field access.
type CTImage struct {
	Patient   *module.PatientModule
	Study     *module.GeneralStudyModule
	Series    *module.GeneralSeriesModule
	Equipment *module.GeneralEquipmentModule
	SOPCommon *module.SOPCommonModule

	// New NEMA-compliant modules
	FrameOfReference *module.FrameOfReferenceModule
	ImagePlane       *module.ImagePlaneModule
	CTImageMod       *module.CTImageModule // Renamed to avoid conflict
	VOILUT           *module.VOILUTModule  // Window/level presets

	ContentDate module.Date
	ContentTime module.Time

	// Deprecated: Legacy custom CT items. Use CTImageMod instead for type-safe field access.
	//
	// Migration Guide:
	//
	// Old approach (deprecated):
	//   ct.Image.KV[tag.KVP] = 120.0
	//   ct.Image.KV[tag.DataCollectionDiameter] = 500.0
	//   ct.Image.KV[tag.ConvolutionKernel] = "STANDARD"
	//
	// New approach (recommended):
	//   ct.CTImageMod.KVP = 120.0
	//   ct.CTImageMod.DataCollectionDiameter = 500.0
	//   ct.CTImageMod.ConvolutionKernel = "STANDARD"
	//
	// The CTImageMod field provides type-safe, documented fields for all CT-specific
	// attributes and follows NEMA DICOS standards. The Image field with its KV map
	// remains for backward compatibility but is not recommended for new code.
	Image     *CTImageModule
	PixelData *PixelData

	// Convenience fields (mapped to tags in Write)
	SamplesPerPixel   uint16
	PhotometricInterp string
	BitsAllocated     uint16
	BitsStored        uint16
	HighBit           uint16
	PixelRepresent    uint16
	Rows              int
	Columns           int
	RescaleIntercept interface{} // float64 or string (DS)
	RescaleSlope     interface{} // float64 or string (DS)
	RescaleType      string
	Codec            Codec // nil = uncompressed
}

// CTImageModule is a legacy simple container for CT Image module attributes.
//
// Deprecated: Use module.CTImageModule (accessed via CTImage.CTImageMod) instead,
// which provides type-safe fields for CT-specific attributes.
//
// Migration Example:
//
//	// Old (deprecated):
//	ct.Image.KV[tag.KVP] = 120.0
//	ct.Image.KV[tag.DataCollectionDiameter] = 500.0
//	ct.Image.KV[tag.ConvolutionKernel] = "STANDARD"
//	ct.Image.KV[tag.ExposureTime] = 1000
//
//	// New (recommended):
//	ct.CTImageMod.KVP = 120.0
//	ct.CTImageMod.DataCollectionDiameter = 500.0
//	ct.CTImageMod.ConvolutionKernel = "STANDARD"
//	ct.CTImageMod.ExposureTime = 1000
//
// The CTImageMod field provides:
//   - Type safety: Fields are strongly typed and documented
//   - Discoverability: IDE autocomplete shows available attributes
//   - NEMA compliance: Follows DICOS Part 6 CT Image IOD specification
//   - Validation: Easier to validate required vs optional attributes
//
// This legacy KV map pattern remains for backward compatibility only. All new code
// should use CTImage.CTImageMod for CT-specific imaging parameters.
type CTImageModule struct {
	KV map[tag.Tag]interface{}
}

// NewCTImage creates a new CT Image IOD with initialized modules and sensible defaults.
//
// Guaranteed Initialized Modules:
//   - Patient, Study, Series, Equipment, SOPCommon (always non-nil)
//   - FrameOfReference, ImagePlane, CTImageMod, VOILUT (always non-nil)
//   - Image (legacy KV map, deprecated but initialized for compatibility)
//
// Generated Defaults:
//   - Unique UIDs: StudyInstanceUID, SeriesInstanceUID, SOPInstanceUID
//   - Current timestamps: StudyDate/Time, SeriesDate/Time, InstanceCreationDate/Time
//   - SOP Class UID: "1.2.840.10008.5.1.4.1.1.2" (CT Image Storage)
//   - Image attributes: 16-bit grayscale (BitsAllocated=16, SamplesPerPixel=1, MONOCHROME2)
//   - Rescale: Intercept=0.0, Slope=1.0, Type="HU" (Hounsfield Units)
//
// After construction, customize the fields as needed:
//
//	ct := dicos.NewCTImage()
//	ct.Patient.PatientID = "PAT-001"
//	ct.Patient.SetPatientName("Jane", "Smith", "", "Dr.", "")
//	ct.Study.AccessionNumber = "ACC-12345"
//	ct.CTImageMod.KVP = "120"
//	ct.CTImageMod.SliceThickness = "2.5"
//	ct.Rows = 512
//	ct.Columns = 512
//	ct.SetPixelData(512, 512, pixelValues)
func NewCTImage() *CTImage {
	ct := &CTImage{
		Patient:          &module.PatientModule{},
		Study:            &module.GeneralStudyModule{},
		Series:           &module.GeneralSeriesModule{},
		Equipment:        &module.GeneralEquipmentModule{},
		SOPCommon:        &module.SOPCommonModule{},
		FrameOfReference: &module.FrameOfReferenceModule{},
		ImagePlane:       module.NewImagePlaneModule(),
		CTImageMod:       module.NewCTImageModule(),
		VOILUT:           module.NewVOILUTModuleForCT(),                     // CT presets
		Image:            &CTImageModule{KV: make(map[tag.Tag]interface{})}, // Legacy
	}

	// Set defaults
	ct.SamplesPerPixel = 1
	ct.PhotometricInterp = "MONOCHROME2"
	ct.BitsAllocated = 16
	ct.BitsStored = 16
	ct.HighBit = 15
	ct.PixelRepresent = 0
	ct.RescaleIntercept = 0.0
	ct.RescaleSlope = 1.0
	ct.RescaleType = "HU"

	now := time.Now()

	// Generate UIDs
	ct.Study.StudyInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	ct.Series.SeriesInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	ct.SOPCommon.SOPInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	ct.SOPCommon.SOPClassUID = "1.2.840.10008.5.1.4.1.1.2" // CT Image Storage

	ct.Study.StudyDate = module.NewDate(now)
	ct.Study.StudyTime = module.NewTime(now)
	ct.Series.SeriesDate = module.NewDate(now)
	ct.Series.SeriesTime = module.NewTime(now)
	ct.SOPCommon.InstanceCreationDate = module.NewDate(now)
	ct.SOPCommon.InstanceCreationTime = module.NewTime(now)

	ct.ContentDate = module.NewDate(now)
	ct.ContentTime = module.NewTime(now)

	// Set default transfer syntax to Explicit VR Little Endian
	// User can change this by setting pixel data with compression
	return ct
}

// GetDataset builds and returns the complete DICOS Dataset from the CTImage.
//
// This method:
//  1. Determines transfer syntax based on ct.Codec or encapsulated pixel data
//  2. Adds File Meta Information (Group 0002) elements
//  3. Composes all modules into the dataset
//  4. Adds convenience field values (Rows, Columns, Rescale, etc.)
//  5. Adds pixel data (compressed if Codec is set, native otherwise)
//
// Transfer Syntax Selection:
//   - If Codec != nil: Uses codec's transfer syntax UID (e.g., JPEG-LS)
//   - If PixelData.IsEncapsulated: Defaults to JPEG-LS Lossless
//   - Otherwise: Explicit VR Little Endian (uncompressed)
//
// The returned Dataset can be written to disk using Write() or passed to other
// DICOS processing functions.
//
// Example:
//
//	ct := dicos.NewCTImage()
//	// ... configure ct fields ...
//	ds, err := ct.GetDataset()
//	if err != nil {
//		log.Fatal(err)
//	}
//	dicos.Write(file, ds)
func (ct *CTImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	// 1. Determine transfer syntax based on compression
	ts := string(transfer.ExplicitVRLittleEndian)
	if ct.Codec != nil {
		ts = ct.Codec.TransferSyntaxUID()
	} else if ct.PixelData != nil && ct.PixelData.IsEncapsulated {
		// Already encapsulated data - use JPEG-LS as default
		ts = string(transfer.JPEGLSLossless)
	}

	// 2. File Meta Information
	opts = append(opts, WithFileMeta(ct.SOPCommon.SOPClassUID, ct.SOPCommon.SOPInstanceUID, ts))

	// 3. Add Modules
	opts = append(opts,
		WithModule(ct.Patient.ToTags()),
		WithModule(ct.Study.ToTags()),
		WithModule(ct.Series.ToTags()),
		WithModule(ct.Equipment.ToTags()),
		WithModule(ct.SOPCommon.ToTags()),
	)

	// 3b. Add optional NEMA-compliant modules
	if ct.FrameOfReference != nil {
		opts = append(opts, WithModule(ct.FrameOfReference.ToTags()))
	}
	if ct.ImagePlane != nil {
		opts = append(opts, WithModule(ct.ImagePlane.ToTags()))
	}
	if ct.CTImageMod != nil {
		opts = append(opts, WithModule(ct.CTImageMod.ToTags()))
	}
	if ct.VOILUT != nil {
		opts = append(opts, WithModule(ct.VOILUT.ToTags()))
	}

	// 4. Content Date/Time
	opts = append(opts,
		WithElement(tag.ContentDate, ct.ContentDate.String()),
		WithElement(tag.ContentTime, ct.ContentTime.String()),
	)

	// 5. Image Attributes
	opts = append(opts,
		WithElement(tag.SamplesPerPixel, ct.SamplesPerPixel),
		WithElement(tag.PhotometricInterpretation, ct.PhotometricInterp),
		WithElement(tag.BitsAllocated, ct.BitsAllocated),
		WithElement(tag.BitsStored, ct.BitsStored),
		WithElement(tag.HighBit, ct.HighBit),
		WithElement(tag.PixelRepresentation, ct.PixelRepresent),
		WithElement(tag.Rows, uint16(ct.Rows)),
		WithElement(tag.Columns, uint16(ct.Columns)),
		WithElement(tag.RescaleIntercept, ct.RescaleIntercept),
		WithElement(tag.RescaleSlope, ct.RescaleSlope),
		WithElement(tag.RescaleType, ct.RescaleType),
	)

	// 6. Legacy image KV pairs
	for t, v := range ct.Image.KV {
		if t == tag.Rows || t == tag.Columns {
			continue
		}
		opts = append(opts, WithElement(t, v))
	}

	// 7. Pixel Data
	if ct.Codec != nil && ct.PixelData != nil && !ct.PixelData.IsEncapsulated {
		flatData := ct.PixelData.GetFlatData()
		opts = append(opts, WithPixelData(ct.Rows, ct.Columns, int(ct.BitsAllocated), flatData, ct.Codec))
	} else if ct.PixelData != nil {
		opts = append(opts, WithRawPixelData(ct.PixelData))
	}

	return NewDataset(opts...)
}

// WriteTo writes the CT Image to any io.Writer
func (ct *CTImage) WriteTo(w io.Writer) (int64, error) {
	ds, err := ct.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, ds)
}

// Write writes the CT Image to a file (convenience wrapper)
func (ct *CTImage) Write(path string) (int64, error) {
	slog.Debug("Writing DICOS file", "path", path, "sop_instance_uid", ct.SOPCommon.SOPInstanceUID, "compressed", ct.PixelData != nil && ct.PixelData.IsEncapsulated)
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return ct.WriteTo(f)
}

// SetPixelData sets native (uncompressed) pixel data for the CT image.
//
// Parameters:
//   - rows: Image height in pixels
//   - cols: Image width in pixels
//   - data: Pixel values in row-major order (left-to-right, top-to-bottom)
//
// Multi-Frame Handling:
// If len(data) > rows*cols, the data is automatically split into multiple frames.
// For example, data with 512*512*3 = 786,432 pixels creates 3 frames.
//
// This method:
//   - Updates ct.PixelData with uncompressed Frame structs
//   - Sets image attributes (Rows, Columns, BitsAllocated, etc.) in legacy Image.KV
//   - Configures 16-bit grayscale MONOCHROME2 format
//
// To compress the pixel data, set ct.Codec before calling GetDataset():
//
//	ct.SetPixelData(512, 512, pixelValues)
//	ct.Codec = dicos.CodecJPEGLS // Compress using JPEG-LS
//	ct.Write("output.dcs")
//
// For already-compressed data, populate ct.PixelData directly with encapsulated frames.
func (ct *CTImage) SetPixelData(rows, cols int, data []uint16) {
	// Update image module tags
	ct.Image.KV[tag.Rows] = uint16(rows)
	ct.Image.KV[tag.Columns] = uint16(cols)
	ct.Image.KV[tag.SamplesPerPixel] = uint16(1)
	ct.Image.KV[tag.PhotometricInterpretation] = "MONOCHROME2"
	ct.Image.KV[tag.BitsAllocated] = uint16(16)
	ct.Image.KV[tag.BitsStored] = uint16(16)
	ct.Image.KV[tag.HighBit] = uint16(15)
	ct.Image.KV[tag.PixelRepresentation] = uint16(0)

	// Create PixelData struct
	// For native, we create one frame with all data?
	// Or multiple frames if 3D?
	// Assuming single frame or multi-frame flattened.
	// If data length > rows*cols, it's multi-frame.
	pixelsPerFrame := rows * cols
	numFrames := len(data) / pixelsPerFrame
	ct.Image.KV[tag.NumberOfFrames] = fmt.Sprintf("%d", numFrames) // IS VR

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, numFrames),
	}

	for i := range numFrames {
		start := i * pixelsPerFrame
		end := start + pixelsPerFrame
		if end > len(data) {
			end = len(data)
		}

		frameData := make([]uint16, end-start)
		copy(frameData, data[start:end])
		pd.Frames[i] = Frame{Data: frameData}
	}
	ct.PixelData = pd
}

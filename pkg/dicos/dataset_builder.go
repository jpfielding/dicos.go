package dicos

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log/slog"

	"github.com/jpfielding/dicos.go/pkg/dicos/module"
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
)

// Option configures a Dataset during construction using the functional options pattern.
// Options are applied sequentially to build up a complete dataset.
type Option func(*Dataset) error

// NewDataset creates a Dataset with the given functional options.
//
// This is the primary way to construct DICOS datasets programmatically. Options are
// applied in order, allowing you to compose datasets from modules, elements, and pixel data.
//
// Common options:
//   - WithFileMeta() - Add File Meta Information (Group 0002) elements
//   - WithModule() - Add all elements from an IOD module
//   - WithElement() - Add individual DICOM elements
//   - WithPixelData() - Add pixel data (compressed or uncompressed)
//   - WithSequence() - Add sequence elements
//
// Example:
//
//	ds, err := dicos.NewDataset(
//		dicos.WithFileMeta(sopClassUID, sopInstanceUID, transferSyntaxUID),
//		dicos.WithModule(patient.ToTags()),
//		dicos.WithModule(study.ToTags()),
//		dicos.WithPixelData(512, 512, 16, pixelData, dicos.CodecJPEGLS),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
func NewDataset(opts ...Option) (*Dataset, error) {
	ds := &Dataset{Elements: make(map[Tag]*Element)}
	for _, opt := range opts {
		if err := opt(ds); err != nil {
			return nil, err
		}
	}
	return ds, nil
}

// WithElement adds a single DICOM element to the dataset with automatic VR detection.
//
// Supported value types:
//   - string - For text VRs (CS, LO, SH, PN, UI, etc.)
//   - uint16 - For US (Unsigned Short) VR
//   - uint32 - For UL (Unsigned Long) VR
//   - int - Converted to appropriate VR based on tag
//   - []uint16 - For multi-value US elements
//   - []int - For multi-value integer elements
//   - []float32, []float64 - For DS (Decimal String) and FL/FD VRs
//   - []*Dataset - For SQ (Sequence) VR
//
// The Value Representation (VR) is automatically determined from the tag using
// GetVR(). For custom tags not in the standard dictionary, VR defaults to "UN" (Unknown).
//
// Example:
//
//	ds, err := dicos.NewDataset(
//		dicos.WithElement(tag.PatientID, "PAT-12345"),
//		dicos.WithElement(tag.Rows, uint16(512)),
//		dicos.WithElement(tag.WindowCenter, []float64{40.0, 400.0}),
//	)
func WithElement(t tag.Tag, value interface{}) Option {
	return func(ds *Dataset) error {
		internalTag := Tag{Group: t.Group, Element: t.Element}
		vr := GetVR(t)
		ds.Elements[internalTag] = &Element{
			Tag:   internalTag,
			VR:    vr,
			Value: value,
		}
		return nil
	}
}

// WithSequence adds a sequence element to the dataset.
//
// Sequences (VR=SQ) contain zero or more items, where each item is itself a Dataset.
// This allows hierarchical nesting of DICOM data structures, commonly used for:
//   - Referenced Image Sequence
//   - PTO (Potential Threat Object) Sequence in TDR
//   - Source Image Sequence
//   - Request Attributes Sequence
//
// Example:
//
//	// Create sequence items
//	item1, _ := dicos.NewDataset(
//		dicos.WithElement(tag.ReferencedSOPClassUID, "1.2.840.10008.5.1.4.1.1.2"),
//		dicos.WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.4.5.6"),
//	)
//	item2, _ := dicos.NewDataset(
//		dicos.WithElement(tag.ReferencedSOPClassUID, "1.2.840.10008.5.1.4.1.1.2"),
//		dicos.WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.4.5.7"),
//	)
//
//	// Add sequence to dataset
//	ds, _ := dicos.NewDataset(
//		dicos.WithSequence(tag.ReferencedImageSequence, item1, item2),
//	)
func WithSequence(t tag.Tag, items ...*Dataset) Option {
	return func(ds *Dataset) error {
		internalTag := Tag{Group: t.Group, Element: t.Element}
		ds.Elements[internalTag] = &Element{
			Tag:   internalTag,
			VR:    "SQ",
			Value: items,
		}
		return nil
	}
}

// WithFileMeta adds standard DICOM File Meta Information (Group 0002) elements.
//
// These elements are required at the beginning of every DICOM/DICOS file and are
// always encoded in Explicit VR Little Endian, regardless of the transfer syntax
// specified for the rest of the file.
//
// Parameters:
//   - sopClassUID: SOP Class UID identifying the type of object (e.g., "1.2.840.10008.5.1.4.1.1.501.1" for DICOS CT)
//   - sopInstanceUID: Unique identifier for this specific instance
//   - transferSyntax: Transfer Syntax UID for the dataset encoding (e.g., "1.2.840.10008.1.2.4.80" for JPEG-LS)
//
// Automatically adds:
//   - (0002,0002) Media Storage SOP Class UID
//   - (0002,0003) Media Storage SOP Instance UID
//   - (0002,0010) Transfer Syntax UID
//   - (0002,0012) Implementation Class UID
//   - (0002,0013) Implementation Version Name
//
// Example:
//
//	ds, _ := dicos.NewDataset(
//		dicos.WithFileMeta(
//			dicos.DICOSCTImageStorageUID,
//			"1.2.826.0.1.3680043.8.498."+uuid.New().String(),
//			dicos.JPEGLSLossless,
//		),
//	)
func WithFileMeta(sopClassUID, sopInstanceUID, transferSyntax string) Option {
	return func(ds *Dataset) error {
		opts := []Option{
			WithElement(tag.MediaStorageSOPClassUID, sopClassUID),
			WithElement(tag.MediaStorageSOPInstanceUID, sopInstanceUID),
			WithElement(tag.TransferSyntaxUID, transferSyntax),
			WithElement(tag.ImplementationClassUID, "1.2.826.0.1.3680043.8.498.1"),
			WithElement(tag.ImplementationVersionName, "GO_DICOS"),
		}
		for _, opt := range opts {
			if err := opt(ds); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithModule adds all elements from a DICOM IOD (Information Object Definition) module.
//
// Modules are reusable collections of related DICOM elements that implement the
// IODModule interface. Common modules include:
//   - Patient Module (PatientName, PatientID, PatientBirthDate, PatientSex)
//   - Study Module (StudyInstanceUID, StudyDate, StudyTime, AccessionNumber)
//   - Series Module (SeriesInstanceUID, Modality, SeriesNumber)
//   - CT Image Module (ImageType, KVP, SliceThickness)
//   - Equipment Module (Manufacturer, ManufacturerModelName)
//
// Each module's ToTags() method returns []IODElement with tag/value pairs that
// are added to the dataset.
//
// Example:
//
//	patient := module.NewPatientModule("DOE^JOHN", "PAT-12345", "19700101", "M")
//	study := module.NewStudyModule("1.2.3", "20240101", "120000", "ACC-001")
//
//	ds, _ := dicos.NewDataset(
//		dicos.WithModule(patient.ToTags()),
//		dicos.WithModule(study.ToTags()),
//	)
func WithModule(tags []module.IODElement) Option {
	return func(ds *Dataset) error {
		for _, el := range tags {
			if err := WithElement(el.Tag, el.Value)(ds); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithPixelData adds pixel data to the dataset, either uncompressed (native) or compressed (encapsulated).
//
// Parameters:
//   - rows: Image height in pixels
//   - cols: Image width in pixels
//   - bitsAllocated: Bits per pixel (8 or 16)
//   - data: Pixel values in row-major order (left-to-right, top-to-bottom)
//   - codec: Compression codec (nil for uncompressed, or CodecJPEGLS, CodecJPEG2K, etc.)
//
// Native (Uncompressed) Format - codec=nil:
//   - Data stored directly as OW (Other Word) or OB (Other Byte)
//   - Multi-frame images concatenate frames sequentially
//   - No compression overhead, larger file size
//   - Use when lossless accuracy is critical or for debugging
//
// Encapsulated (Compressed) Format - codec != nil:
//   - Each frame compressed independently using specified codec
//   - Stored as OB with Basic Offset Table
//   - Compressed data padded to even length per DICOM spec
//   - Smaller file size, recommended for DICOS (use JPEG-LS per NEMA)
//
// Multi-Frame Handling:
//
// The length of the data slice determines the number of frames:
//
//	numFrames = len(data) / (rows * cols)
//
// For a 512x512 image with 3 frames, data should contain 512*512*3 = 786,432 pixels.
//
// Pixel Ordering:
//
// Pixels must be in row-major order. For a 3x3 image:
//
//	data[0] = pixel at (x=0, y=0)  // top-left
//	data[1] = pixel at (x=1, y=0)
//	data[2] = pixel at (x=2, y=0)  // top-right
//	data[3] = pixel at (x=0, y=1)  // second row, left
//	...
//	data[8] = pixel at (x=2, y=2)  // bottom-right
//
// Example - Uncompressed:
//
//	pixelData := make([]uint16, 512*512)
//	// Fill with pixel values...
//	ds, _ := dicos.NewDataset(
//		dicos.WithPixelData(512, 512, 16, pixelData, nil),
//	)
//
// Example - Compressed with JPEG-LS:
//
//	ds, _ := dicos.NewDataset(
//		dicos.WithPixelData(512, 512, 16, pixelData, dicos.CodecJPEGLS),
//	)
func WithPixelData(rows, cols, bitsAllocated int, data []uint16, codec Codec) Option {
	return func(ds *Dataset) error {
		if len(data) == 0 {
			return nil
		}

		pixelsPerFrame := rows * cols
		numFrames := len(data) / pixelsPerFrame
		compress := codec != nil

		pd := &PixelData{
			IsEncapsulated: compress,
			Frames:         make([]Frame, numFrames),
		}

		if compress {
			offsets := make([]uint32, numFrames)
			currentOffset := uint32(0)

			for i := 0; i < numFrames; i++ {
				offsets[i] = currentOffset
				start := i * pixelsPerFrame
				end := start + pixelsPerFrame
				sliceData := data[start:end]

				var buf bytes.Buffer
				var img image.Image

				if bitsAllocated > 8 {
					gray16 := image.NewGray16(image.Rect(0, 0, cols, rows))

					if i == 0 && len(sliceData) > 10 {
						slog.Debug("ENCODE Frame 0", "first_pixels_subset", sliceData[:10])
					}

					for j, val := range sliceData {
						x := j % cols
						y := j / cols
						gray16.SetGray16(x, y, color.Gray16{Y: val})
					}
					img = gray16
				} else {
					gray8 := image.NewGray(image.Rect(0, 0, cols, rows))
					for j, val := range sliceData {
						x := j % cols
						y := j / cols
						gray8.SetGray(x, y, color.Gray{Y: uint8(val)})
					}
					img = gray8
				}

				if err := codec.Encode(&buf, img); err != nil {
					return fmt.Errorf("%s encode error: %w", codec.Name(), err)
				}

				compressedData := buf.Bytes()
				if len(compressedData)%2 != 0 {
					compressedData = append(compressedData, 0x00)
				}

				pd.Frames[i] = Frame{
					CompressedData: compressedData,
				}

				frameSize := uint32(len(compressedData)) + 8
				currentOffset += frameSize
			}
			pd.Offsets = offsets

			t := Tag{Group: 0x7FE0, Element: 0x0010}
			ds.Elements[t] = &Element{
				Tag:   t,
				VR:    "OB",
				Value: pd,
			}
		} else {
			for i := 0; i < numFrames; i++ {
				start := i * pixelsPerFrame
				end := start + pixelsPerFrame

				fData := make([]uint16, len(data[start:end]))
				copy(fData, data[start:end])

				pd.Frames[i] = Frame{
					Data: fData,
				}
			}

			vr := "OB"
			if bitsAllocated > 8 {
				vr = "OW"
			}

			t := Tag{Group: 0x7FE0, Element: 0x0010}
			ds.Elements[t] = &Element{
				Tag:   t,
				VR:    vr,
				Value: pd,
			}
		}
		return nil
	}
}

// WithRawPixelData adds pre-constructed PixelData to the dataset
func WithRawPixelData(pd *PixelData) Option {
	return func(ds *Dataset) error {
		if pd == nil {
			return nil
		}
		vr := "OB"
		if !pd.IsEncapsulated && len(pd.Frames) > 0 && len(pd.Frames[0].Data) > 0 {
			vr = "OW"
		}
		t := Tag{Group: 0x7FE0, Element: 0x0010}
		ds.Elements[t] = &Element{
			Tag:   t,
			VR:    vr,
			Value: pd,
		}
		return nil
	}
}

// GetVR returns the Value Representation (VR) for a standard tag
func GetVR(t tag.Tag) string {
	if t.Group == 0x0002 {
		if t.Element == 0x0000 {
			return "UL"
		}
		if t.Element == 0x0001 {
			return "OB"
		}
		if t == tag.TransferSyntaxUID {
			return "UI"
		}
		return "UI"
	}

	switch t {
	case tag.PatientName:
		return "PN"
	case tag.PatientID:
		return "LO"
	case tag.PatientBirthDate:
		return "DA"
	case tag.PatientSex:
		return "CS"

	case tag.StudyDate:
		return "DA"
	case tag.StudyTime:
		return "TM"
	case tag.AccessionNumber:
		return "SH"
	case tag.StudyDescription:
		return "LO"
	case tag.StudyInstanceUID:
		return "UI"
	case tag.StudyID:
		return "SH"

	case tag.Modality:
		return "CS"
	case tag.SeriesInstanceUID:
		return "UI"
	case tag.SeriesNumber:
		return "IS"
	case tag.SeriesDescription:
		return "LO"

	case tag.SamplesPerPixel:
		return "US"
	case tag.PhotometricInterpretation:
		return "CS"
	case tag.Rows:
		return "US"
	case tag.Columns:
		return "US"
	case tag.BitsAllocated:
		return "US"
	case tag.BitsStored:
		return "US"
	case tag.HighBit:
		return "US"
	case tag.PixelRepresentation:
		return "US"
	case tag.NumberOfFrames:
		return "IS"

	case tag.RescaleIntercept:
		return "DS"
	case tag.RescaleSlope:
		return "DS"
	case tag.RescaleType:
		return "LO"
	case tag.WindowCenter:
		return "DS"
	case tag.WindowWidth:
		return "DS"

	case tag.PixelSpacing:
		return "DS"
	case tag.SliceThickness:
		return "DS"
	case tag.SpacingBetweenSlices:
		return "DS"
	case tag.ImagePositionPatient:
		return "DS"
	case tag.ImageOrientationPatient:
		return "DS"
	case tag.SliceLocation:
		return "DS"

	case tag.ContentDate:
		return "DA"
	case tag.ContentTime:
		return "TM"
	case tag.InstanceNumber:
		return "IS"
	case tag.ImageType:
		return "CS"

	case tag.SOPClassUID:
		return "UI"
	case tag.SOPInstanceUID:
		return "UI"

	case tag.PixelData:
		return "OW"
	}

	return "UN"
}

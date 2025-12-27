// Package dicos provides a native Go implementation for reading and writing DICOS (DICOM for Security) files.
//
// This package is modeled after the Stratovan SDICOS library and provides:
//   - Low-level DICOM parsing and writing
//   - High-level IOD (CT, DX, TDR) access
//   - JPEG-LS compression support
//   - Full NEMA DICOS compliance
//
// Basic usage:
//
//	// Read a DICOS file
//	ds, err := dicos.ReadFile("/path/to/file.dcs")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Access pixel data
//	pd, err := ds.GetPixelData()
//
//	// Determine modality
//	if dicos.IsCT(ds) {
//		// Process CT data
//	}
package dicos

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// Re-export commonly used types from subpackages
type (
	// TransferSyntax represents a DICOM transfer syntax
	TransferSyntax = transfer.Syntax
)

// Transfer syntax constants
const (
	ExplicitVRLittleEndian = transfer.ExplicitVRLittleEndian
	ImplicitVRLittleEndian = transfer.ImplicitVRLittleEndian
	JPEGLSLossless         = transfer.JPEGLSLossless
	JPEGLosslessFirstOrder = transfer.JPEGLosslessFirstOrder
)

// SOP Class UIDs for DICOS modalities
const (
	CTImageStorageUID = "1.2.840.10008.5.1.4.1.1.2"
	DXImageStorageUID = "1.2.840.10008.5.1.4.1.1.1.1"
	TDRStorageUID     = "1.2.840.10008.5.1.4.1.1.88.67" // Comprehensive SR

	// DICOS-specific
	DICOSCTImageStorageUID    = "1.2.840.10008.5.1.4.1.1.501.1"
	DICOSDXImageStorageUID    = "1.2.840.10008.5.1.4.1.1.501.2"
	DICOSTDRStorageUID        = "1.2.840.10008.5.1.4.1.1.501.3"
	DICOSAIT2DImageStorageUID = "1.2.840.10008.5.1.4.1.1.501.4"
	DICOSAIT3DImageStorageUID = "1.2.840.10008.5.1.4.1.1.501.5"
)

// ReadFile reads a DICOM/DICOS file from disk and returns a parsed Dataset.
//
// The file must follow DICOM Part 10 format with:
//   - 128-byte preamble
//   - "DICM" magic bytes
//   - File Meta Information (Group 0002) in Explicit VR Little Endian
//   - Dataset encoded per Transfer Syntax UID
//
// Returns an error if the file cannot be opened, read, or parsed.
//
// Example:
//
//	ds, err := dicos.ReadFile("/path/to/scan.dcs")
//	if err != nil {
//		log.Fatalf("Failed to read DICOS file: %v", err)
//	}
//	modality := dicos.GetModality(ds)
//	fmt.Printf("Modality: %s\n", modality)
func ReadFile(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return Parse(bytes.NewReader(data))
}

// ReadBuffer reads a DICOM/DICOS file from a byte slice and returns a parsed Dataset.
//
// This is equivalent to ReadFile but operates on in-memory data. Useful for
// processing DICOS data from network streams, archives, or embedded resources.
//
// The data must follow DICOM Part 10 format (preamble, DICM, File Meta, dataset).
//
// Example:
//
//	data, _ := os.ReadFile("scan.dcs")
//	ds, err := dicos.ReadBuffer(data)
//	if err != nil {
//		log.Fatal(err)
//	}
func ReadBuffer(data []byte) (*Dataset, error) {
	return Parse(bytes.NewReader(data))
}

// GetExtension returns the standard DICOS file extension ".dcs".
//
// DICOS files conventionally use the .dcs extension, though .dcm is also common
// for generic DICOM files.
func GetExtension() string {
	return ".dcs"
}

// IsCT returns true if the dataset represents a CT (Computed Tomography) image.
//
// Checks the SOP Class UID (0008,0016) for either:
//   - Standard DICOM CT: "1.2.840.10008.5.1.4.1.1.2"
//   - DICOS CT: "1.2.840.10008.5.1.4.1.1.501.1"
//
// CT images contain cross-sectional X-ray data with Hounsfield Units representing
// tissue density.
func IsCT(ds *Dataset) bool {
	return checkSOPClass(ds, CTImageStorageUID, DICOSCTImageStorageUID)
}

// IsDX returns true if the dataset represents a DX (Digital X-ray) image.
//
// Checks the SOP Class UID (0008,0016) for either:
//   - Standard DICOM DX: "1.2.840.10008.5.1.4.1.1.1.1"
//   - DICOS DX: "1.2.840.10008.5.1.4.1.1.501.2"
//
// DX images are 2D projection X-ray images captured by digital detectors, commonly
// used in baggage screening for transmission imaging.
func IsDX(ds *Dataset) bool {
	return checkSOPClass(ds, DXImageStorageUID, DICOSDXImageStorageUID)
}

// IsTDR returns true if the dataset represents a TDR (Threat Detection Report).
//
// Checks the SOP Class UID (0008,0016) for either:
//   - Comprehensive SR: "1.2.840.10008.5.1.4.1.1.88.67"
//   - DICOS TDR: "1.2.840.10008.5.1.4.1.1.501.3"
//
// TDR objects contain structured reporting data about detected potential threats
// in baggage/cargo scans, including PTO (Potential Threat Object) sequences with
// spatial coordinates, confidence scores, and threat classifications.
func IsTDR(ds *Dataset) bool {
	return checkSOPClass(ds, TDRStorageUID, DICOSTDRStorageUID)
}

// IsAIT2D returns true if the dataset represents an AIT (Automated Inspection Terminal) 2D image.
//
// Checks the SOP Class UID (0008,0016) for:
//   - DICOS AIT 2D: "1.2.840.10008.5.1.4.1.1.501.4"
//
// AIT 2D images are typically millimeter-wave or backscatter X-ray images used for
// personnel screening, producing 2D projection views.
func IsAIT2D(ds *Dataset) bool {
	return checkSOPClass(ds, DICOSAIT2DImageStorageUID)
}

// IsAIT3D returns true if the dataset represents an AIT 3D image.
//
// Checks the SOP Class UID (0008,0016) for:
//   - DICOS AIT 3D: "1.2.840.10008.5.1.4.1.1.501.5"
//
// AIT 3D images provide volumetric data from automated personnel screening systems,
// typically from millimeter-wave technology producing 3D surface reconstructions.
func IsAIT3D(ds *Dataset) bool {
	return checkSOPClass(ds, DICOSAIT3DImageStorageUID)
}

// GetModality returns the Modality (0008,0060) value from the dataset.
//
// Common DICOS modality values:
//   - "CT" - Computed Tomography
//   - "DX" - Digital Radiography
//   - "SR" - Structured Report (for TDR)
//   - "OT" - Other (sometimes used for AIT)
//
// Returns an empty string if the Modality element is not present.
//
// Deprecated: Use ds.Modality() method instead for better discoverability.
func GetModality(ds *Dataset) string {
	return ds.Modality()
}

// GetTransferSyntax returns the Transfer Syntax UID (0002,0010) from the dataset.
//
// Transfer syntax specifies the encoding rules for the dataset, including:
//   - Value Representation (explicit vs implicit)
//   - Byte ordering (little endian vs big endian)
//   - Compression (uncompressed vs JPEG-LS, JPEG 2000, RLE, etc.)
//
// Common transfer syntaxes:
//   - "1.2.840.10008.1.2.1" - Explicit VR Little Endian (uncompressed)
//   - "1.2.840.10008.1.2" - Implicit VR Little Endian (uncompressed)
//   - "1.2.840.10008.1.2.4.80" - JPEG-LS Lossless (compressed)
//
// Returns Explicit VR Little Endian as default if not specified.
//
// Note: File Meta Information (Group 0002) is always Explicit VR Little Endian
// regardless of the transfer syntax specified for the rest of the dataset.
//
// Deprecated: Use ds.TransferSyntax() method instead for better discoverability.
func GetTransferSyntax(ds *Dataset) TransferSyntax {
	return ds.TransferSyntax()
}

// IsEncapsulated returns true if the dataset's pixel data is encapsulated (compressed).
//
// Determines if compression is used by checking the Transfer Syntax UID.
// Encapsulated transfer syntaxes include:
//   - JPEG-LS Lossless/Near-Lossless
//   - JPEG 2000 Lossless/Lossy
//   - RLE (Run-Length Encoding)
//   - JPEG Lossless (Process 14)
//
// Returns false for native (uncompressed) transfer syntaxes:
//   - Explicit VR Little Endian
//   - Implicit VR Little Endian
//
// Encapsulated pixel data must be decompressed using DecompressPixelData() before
// accessing raw pixel values.
//
// Deprecated: Use ds.IsEncapsulated() method instead for better discoverability.
func IsEncapsulated(ds *Dataset) bool {
	return ds.IsEncapsulated()
}

// GetRows returns the number of rows (image height) from Rows (0028,0010).
//
// Returns 0 if the element is not present.
//
// Deprecated: Use ds.Rows() method instead for better discoverability.
func GetRows(ds *Dataset) int {
	return ds.Rows()
}

// GetColumns returns the number of columns (image width) from Columns (0028,0011).
//
// Returns 0 if the element is not present.
//
// Deprecated: Use ds.Columns() method instead for better discoverability.
func GetColumns(ds *Dataset) int {
	return ds.Columns()
}

// GetNumberOfFrames returns the number of frames from NumberOfFrames (0028,0008).
//
// For multi-frame images (e.g., CT series), this indicates how many frames are
// concatenated in the pixel data. Returns 1 if not specified (single-frame image).
//
// Deprecated: Use ds.NumberOfFrames() method instead for better discoverability.
func GetNumberOfFrames(ds *Dataset) int {
	return ds.NumberOfFrames()
}

// GetBitsAllocated returns the bits allocated per sample from BitsAllocated (0028,0100).
//
// Common values:
//   - 8 - For 8-bit grayscale images
//   - 16 - For 16-bit CT/DX images
//
// Returns 16 as default if not specified.
//
// Deprecated: Use ds.BitsAllocated() method instead for better discoverability.
func GetBitsAllocated(ds *Dataset) int {
	return ds.BitsAllocated()
}

// GetPixelRepresentation returns the pixel representation from PixelRepresentation (0028,0103).
//
// Values:
//   - 0 - Unsigned integer
//   - 1 - Signed integer (two's complement)
//
// Returns 0 (unsigned) as default if not specified.
//
// Note: Some non-compliant CT files use unsigned representation with values offset
// by +32768. Use GetRescale() to handle this correctly.
//
// Deprecated: Use ds.PixelRepresentation() method instead for better discoverability.
func GetPixelRepresentation(ds *Dataset) int {
	return ds.PixelRepresentation()
}

// GetInstanceNumber returns the Instance Number (0020,0013) identifying the image
// within a series.
//
// For multi-slice CT acquisitions, this typically represents the slice number.
// Returns 0 if not present.
func GetInstanceNumber(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.InstanceNumber.Group, tag.InstanceNumber.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
		if s, ok := elem.GetString(); ok {
			var n int
			fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
			return n
		}
	}
	return 0
}

// GetKVP returns the peak kilovoltage (KVP) from KVP (0018,0060).
//
// This indicates the X-ray tube voltage in kilovolts. Common values:
//   - 80-90 kV - Low energy in dual-energy CT
//   - 120-140 kV - Standard CT or high energy in dual-energy CT
//
// Returns 0 if not present.
func GetKVP(ds *Dataset) float64 {
	if elem, ok := ds.FindElement(tag.KVP.Group, tag.KVP.Element); ok {
		if s, ok := elem.GetString(); ok {
			var kvp float64
			fmt.Sscanf(strings.TrimSpace(s), "%f", &kvp)
			return kvp
		}
	}
	return 0
}

// GetImageComments returns user-defined comments from ImageComments (0020,4000).
//
// Some DICOS vendors encode energy level hints here (e.g., "high_energy", "low_energy").
// Returns empty string if not present.
func GetImageComments(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.ImageComments.Group, tag.ImageComments.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetSeriesDescription returns the user-provided series description from
// SeriesDescription (0008,103E).
//
// Example values: "Axial CT", "Luggage_Scan_HE", "Density1".
// Returns empty string if not present.
func GetSeriesDescription(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.SeriesDescription.Group, tag.SeriesDescription.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetSeriesEnergy returns the DICOS-specific energy level from SeriesEnergy (6100,0030).
//
// Values:
//   - 1 - Low Energy
//   - 2 - High Energy
//   - 0 - Not set or not applicable
//
// This is the most authoritative source for energy level in DICOS files.
func GetSeriesEnergy(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.SeriesEnergy.Group, tag.SeriesEnergy.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// GetSeriesEnergyDescription returns the DICOS energy description from
// SeriesEnergyDescription (6100,0031).
//
// Example values: "High Energy", "Low Energy", "HE", "LE".
// Returns empty string if not present.
func GetSeriesEnergyDescription(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.SeriesEnergyDescription.Group, tag.SeriesEnergyDescription.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetEnergyLevel determines the X-ray energy level for dual-energy DICOS scans.
//
// Returns:
//   - "he" for high energy
//   - "le" for low energy
//   - "" if energy level cannot be determined
//
// Detection uses a priority cascade strategy, checking multiple tags in order:
//
//  1. SeriesEnergy (6100,0030) - DICOS-specific energy tag (1=LE, 2=HE)
//  2. SeriesEnergyDescription (6100,0031) - Text containing "high"/"low"
//  3. ImageComments (0020,4000) - Text containing "high_energy"/"low_energy"
//  4. KVP (0018,0060) - Peak kilovoltage (>=110kV is HE, <110kV is LE)
//  5. SeriesDescription (0008,103E) - Text hints like "_he", "density2", etc.
//
// This heuristic approach handles various vendor implementations where energy level
// encoding differs. The KVP threshold (110kV) is based on typical dual-energy CT
// protocols (80kV low, 140kV high).
//
// Example:
//
//	energy := dicos.GetEnergyLevel(ds)
//	switch energy {
//	case "he":
//		fmt.Println("High energy scan")
//	case "le":
//		fmt.Println("Low energy scan")
//	default:
//		fmt.Println("Single energy or unknown")
//	}
func GetEnergyLevel(ds *Dataset) string {
	// 1. Check DICOS SeriesEnergy tag first
	seriesEnergy := GetSeriesEnergy(ds)
	if seriesEnergy == 2 {
		return "he"
	}
	if seriesEnergy == 1 {
		return "le"
	}

	// 2. Check SeriesEnergyDescription
	energyDesc := strings.ToLower(GetSeriesEnergyDescription(ds))
	if strings.Contains(energyDesc, "high") || strings.Contains(energyDesc, "he") {
		return "he"
	}
	if strings.Contains(energyDesc, "low") || strings.Contains(energyDesc, "le") {
		return "le"
	}

	// 3. Check ImageComments
	comments := strings.ToLower(GetImageComments(ds))
	if strings.Contains(comments, "high_energy") || strings.Contains(comments, "he") {
		return "he"
	}
	if strings.Contains(comments, "low_energy") || strings.Contains(comments, "le") {
		return "le"
	}

	// 4. Check KVP value (typical dual-energy: HE=140kV, LE=80kV)
	kvp := GetKVP(ds)
	if kvp >= 110 {
		return "he"
	}
	if kvp > 0 && kvp < 110 {
		return "le"
	}

	// 5. Check SeriesDescription for hints
	desc := strings.ToLower(GetSeriesDescription(ds))
	if strings.Contains(desc, "_he") || strings.Contains(desc, "density2") || strings.Contains(desc, "high") {
		return "he"
	}
	if strings.Contains(desc, "_le") || strings.Contains(desc, "density1") || strings.Contains(desc, "low") {
		return "le"
	}

	return ""
}

// GetPixelData extracts pixel data from the PixelData (7FE0,0010) element.
//
// Returns *PixelData in either native (uncompressed) or encapsulated (compressed) format:
//
// Native Format (IsEncapsulated=false):
//   - Frame.Data contains []uint16 pixel values in row-major order
//   - Pixels ordered left-to-right, top-to-bottom within each frame
//   - Multi-frame images have frames stored sequentially
//
// Encapsulated Format (IsEncapsulated=true):
//   - Frame.CompressedData contains compressed bitstream bytes
//   - Each frame compressed independently per DICOM encapsulation rules
//   - Must decompress using DecompressPixelData() with appropriate codec
//
// The method automatically handles:
//   - Byte to uint16 conversion for native data
//   - Multi-frame image splitting based on Rows, Columns, NumberOfFrames
//   - Both OW (Other Word) and OB (Other Byte) Value Representations
//
// Example:
//
//	pd, err := ds.GetPixelData()
//	if err != nil {
//		log.Fatal(err)
//	}
//	if pd.IsEncapsulated {
//		// Compressed - need to decompress
//		pd, err = dicos.DecompressPixelData(ds, pd)
//	}
//	// Access pixel values
//	frame0 := pd.Frames[0].Data // First frame pixels
func (ds *Dataset) GetPixelData() (*PixelData, error) {
	elem, ok := ds.FindElement(tag.PixelData.Group, tag.PixelData.Element)
	if !ok {
		return nil, fmt.Errorf("no pixel data element found")
	}

	// Case 1: Already converted to *PixelData (encapsulated)
	if pd, ok := elem.GetPixelData(); ok {
		return pd, nil
	}

	// Case 2: Uncompressed data
	var u16Raw []uint16
	var byteRaw []byte

	switch v := elem.Value.(type) {
	case []byte:
		byteRaw = v
	case []uint16:
		u16Raw = v
	default:
		return nil, fmt.Errorf("pixel data element has unexpected type: %T", elem.Value)
	}

	// Get dimensions for conversion
	rows := GetRows(ds)
	cols := GetColumns(ds)
	numFrames := GetNumberOfFrames(ds)
	bitsAllocated := GetBitsAllocated(ds)

	slog.Debug("Converting uncompressed pixel data",
		slog.Int("rows", rows),
		slog.Int("cols", cols),
		slog.Int("numFrames", numFrames),
		slog.Int("bitsAllocated", bitsAllocated),
		slog.String("type", fmt.Sprintf("%T", elem.Value)))

	if rows == 0 || cols == 0 {
		return nil, fmt.Errorf("invalid dimensions for pixel data conversion: %dx%d", rows, cols)
	}

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, numFrames),
	}

	bytesPerPixel := (bitsAllocated + 7) / 8
	pixelsPerFrame := rows * cols
	frameSizeInBytes := pixelsPerFrame * bytesPerPixel

	slog.Debug("Calculated frame metrics",
		slog.Int("bytesPerPixel", bytesPerPixel),
		slog.Int("frameSizeInBytes", frameSizeInBytes),
		slog.Int("pixelsPerFrame", pixelsPerFrame))

	for i := 0; i < numFrames; i++ {
		u16Data := make([]uint16, pixelsPerFrame)

		if len(u16Raw) > 0 {
			start := i * pixelsPerFrame
			end := start + pixelsPerFrame
			if end > len(u16Raw) {
				return nil, fmt.Errorf("pixel data truncated: expected %d pixels for %d frames, got %d", numFrames*pixelsPerFrame, numFrames, len(u16Raw))
			}
			copy(u16Data, u16Raw[start:end])
		} else if len(byteRaw) > 0 {
			start := i * frameSizeInBytes
			end := start + frameSizeInBytes
			if end > len(byteRaw) {
				return nil, fmt.Errorf("pixel data truncated: expected %d bytes for %d frames, got %d", numFrames*frameSizeInBytes, numFrames, len(byteRaw))
			}

			frameData := byteRaw[start:end]
			if bytesPerPixel == 2 {
				for j := 0; j < pixelsPerFrame; j++ {
					if j*2+1 < len(frameData) {
						u16Data[j] = uint16(frameData[j*2]) | (uint16(frameData[j*2+1]) << 8)
					}
				}
			} else {
				for j := 0; j < pixelsPerFrame; j++ {
					if j < len(frameData) {
						u16Data[j] = uint16(frameData[j])
					}
				}
			}
		}

		pd.Frames[i] = Frame{
			Data: u16Data,
		}
	}

	return pd, nil
}

// GetRescale returns the Rescale Intercept and Slope for converting raw pixel values
// to modality-specific units (e.g., Hounsfield Units for CT).
//
// This function applies heuristics for unsigned CT images. For explicit control without
// heuristics, use GetRescaleExplicit().
//
// The conversion formula is:
//
//	OutputValue = (RawPixelValue * RescaleSlope) + RescaleIntercept
//
// For CT images, this produces Hounsfield Units (HU):
//   - Air: -1000 HU
//   - Water: 0 HU
//   - Bone: +1000 to +3000 HU
//
// Special Handling for Unsigned CT Images (Heuristic):
//
// Some non-compliant CT files mark PixelRepresentation as 0 (unsigned) but contain
// values offset by +32768 to represent signed data. This function applies a heuristic
// for CT datasets: if PixelRepresentation=0 and RescaleIntercept is missing, it returns
// intercept=-32768 to correct this offset.
//
// To disable this heuristic, use GetRescaleExplicit(ds) instead.
//
// Default Values:
//   - If RescaleIntercept (0028,1052) is absent: 0.0 (or -32768.0 for unsigned CT via heuristic)
//   - If RescaleSlope (0028,1053) is absent: 1.0
//
// Example:
//
//	intercept, slope := dicos.GetRescale(ds)
//	pd, _ := ds.GetPixelData()
//	for _, pixel := range pd.Frames[0].Data {
//		hu := float64(pixel)*slope + intercept
//		fmt.Printf("Pixel: %d -> HU: %.1f\n", pixel, hu)
//	}
func GetRescale(ds *Dataset) (intercept, slope float64) {
	intercept, slope = 0, 1 // Default values

	var foundIntercept bool
	if elem, ok := ds.FindElement(tag.RescaleIntercept.Group, tag.RescaleIntercept.Element); ok {
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f", &intercept)
			foundIntercept = true
		}
	}
	// If tag is absent, check for implicit intercept via heuristic
	// CT images typically have specific defaults or are signed.
	// Some non-compliant files might be marked as Unsigned (0)
	// but contain values offset by +32768. In this case, we need -32768 intercept.
	if !foundIntercept && IsCT(ds) {
		pixelRep := GetPixelRepresentation(ds) // 0=unsigned, 1=signed
		if pixelRep == 0 {
			// Heuristic: Unsigned CT likely implies shifted values
			intercept = -32768.0
		}
	}

	if elem, ok := ds.FindElement(tag.RescaleSlope.Group, tag.RescaleSlope.Element); ok {
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f", &slope)
		}
	}

	return
}

// GetRescaleExplicit returns the Rescale Intercept and Slope without applying heuristics.
//
// Unlike GetRescale(), this function returns exactly what's in the DICOM tags
// without any automatic correction for unsigned CT images.
//
// Use this when you want explicit control or are working with non-CT modalities
// where the unsigned CT heuristic shouldn't apply.
//
// Default Values:
//   - If RescaleIntercept (0028,1052) is absent: 0.0
//   - If RescaleSlope (0028,1053) is absent: 1.0
//
// Example:
//
//	intercept, slope := dicos.GetRescaleExplicit(ds)
//	// No automatic -32768 offset applied for unsigned CT
func GetRescaleExplicit(ds *Dataset) (intercept, slope float64) {
	intercept, slope = 0, 1 // Default values

	if elem, ok := ds.FindElement(tag.RescaleIntercept.Group, tag.RescaleIntercept.Element); ok {
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f", &intercept)
		}
	}

	if elem, ok := ds.FindElement(tag.RescaleSlope.Group, tag.RescaleSlope.Element); ok {
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f", &slope)
		}
	}

	return
}

// SetEnergyLevel explicitly sets the DICOS energy level tags in the dataset.
//
// This provides direct control over energy level encoding, bypassing the heuristic
// detection used by GetEnergyLevel().
//
// Parameters:
//   - ds: The dataset to modify
//   - level: Energy level ("he" for high energy, "le" for low energy, "" to clear)
//
// Sets the following tags:
//   - SeriesEnergy (6100,0030): 2 for HE, 1 for LE
//   - SeriesEnergyDescription (6100,0031): "High Energy" or "Low Energy"
//
// Example:
//
//	ds, _ := dicos.ReadFile("scan.dcs")
//	dicos.SetEnergyLevel(ds, "he") // Mark as high energy
//	dicos.Write(file, ds)
func SetEnergyLevel(ds *Dataset, level string) error {
	level = strings.ToLower(strings.TrimSpace(level))

	switch level {
	case "he", "high":
		ds.Elements[Tag{Group: 0x6100, Element: 0x0030}] = &Element{
			Tag:   Tag{Group: 0x6100, Element: 0x0030},
			VR:    "US",
			Value: uint16(2),
		}
		ds.Elements[Tag{Group: 0x6100, Element: 0x0031}] = &Element{
			Tag:   Tag{Group: 0x6100, Element: 0x0031},
			VR:    "LO",
			Value: "High Energy",
		}
	case "le", "low":
		ds.Elements[Tag{Group: 0x6100, Element: 0x0030}] = &Element{
			Tag:   Tag{Group: 0x6100, Element: 0x0030},
			VR:    "US",
			Value: uint16(1),
		}
		ds.Elements[Tag{Group: 0x6100, Element: 0x0031}] = &Element{
			Tag:   Tag{Group: 0x6100, Element: 0x0031},
			VR:    "LO",
			Value: "Low Energy",
		}
	case "":
		// Clear energy tags
		delete(ds.Elements, Tag{Group: 0x6100, Element: 0x0030})
		delete(ds.Elements, Tag{Group: 0x6100, Element: 0x0031})
	default:
		return fmt.Errorf("invalid energy level: %q (must be 'he', 'le', or empty)", level)
	}

	return nil
}

// Helper function to check SOP Class UID
func checkSOPClass(ds *Dataset, uids ...string) bool {
	if elem, ok := ds.FindElement(tag.SOPClassUID.Group, tag.SOPClassUID.Element); ok {
		if s, ok := elem.GetString(); ok {
			s = strings.TrimSpace(s)
			for _, uid := range uids {
				if s == uid {
					return true
				}
			}
		}
	}
	return false
}

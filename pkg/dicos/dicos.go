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

// ReadFile reads a DICOM/DICOS file from disk
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

// ReadBuffer reads a DICOM/DICOS file from a byte slice
func ReadBuffer(data []byte) (*Dataset, error) {
	return Parse(bytes.NewReader(data))
}

// GetExtension returns the standard DICOS file extension
func GetExtension() string {
	return ".dcs"
}

// IsCT returns true if the dataset is a CT image
func IsCT(ds *Dataset) bool {
	return checkSOPClass(ds, CTImageStorageUID, DICOSCTImageStorageUID)
}

// IsDX returns true if the dataset is a DX image
func IsDX(ds *Dataset) bool {
	return checkSOPClass(ds, DXImageStorageUID, DICOSDXImageStorageUID)
}

// IsTDR returns true if the dataset is a Threat Detection Report
func IsTDR(ds *Dataset) bool {
	return checkSOPClass(ds, TDRStorageUID, DICOSTDRStorageUID)
}

// IsAIT2D returns true if the dataset is an AIT 2D image
func IsAIT2D(ds *Dataset) bool {
	return checkSOPClass(ds, DICOSAIT2DImageStorageUID)
}

// IsAIT3D returns true if the dataset is an AIT 3D image
func IsAIT3D(ds *Dataset) bool {
	return checkSOPClass(ds, DICOSAIT3DImageStorageUID)
}

// GetModality returns the modality string from the dataset
func GetModality(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.Modality.Group, tag.Modality.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetTransferSyntax returns the transfer syntax from the dataset
func GetTransferSyntax(ds *Dataset) TransferSyntax {
	if elem, ok := ds.FindElement(tag.TransferSyntaxUID.Group, tag.TransferSyntaxUID.Element); ok {
		if s, ok := elem.GetString(); ok {
			return transfer.FromUID(strings.TrimSpace(s))
		}
	}
	return ExplicitVRLittleEndian // Default
}

// IsEncapsulated returns true if the pixel data is encapsulated (compressed)
func IsEncapsulated(ds *Dataset) bool {
	syntax := GetTransferSyntax(ds)
	return syntax.IsEncapsulated()
}

// GetRows returns the number of rows in the image
func GetRows(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.Rows.Group, tag.Rows.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// GetColumns returns the number of columns in the image
func GetColumns(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.Columns.Group, tag.Columns.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// GetNumberOfFrames returns the number of frames in the image
func GetNumberOfFrames(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.NumberOfFrames.Group, tag.NumberOfFrames.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
		// Number of Frames can be a string (IS VR)
		if s, ok := elem.GetString(); ok {
			var n int
			fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
			return n
		}
	}
	return 1 // Default to 1 if not specified
}

// GetBitsAllocated returns the bits allocated per sample
func GetBitsAllocated(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.BitsAllocated.Group, tag.BitsAllocated.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 16 // Default
}

// GetPixelRepresentation returns 0 for unsigned, 1 for signed
func GetPixelRepresentation(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.PixelRepresentation.Group, tag.PixelRepresentation.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0 // Default to unsigned
}

// GetInstanceNumber returns the instance number (0020,0013)
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

// GetKVP returns the peak kilovoltage (0018,0060) for X-ray energy level
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

// GetImageComments returns the image comments (0020,4000)
func GetImageComments(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.ImageComments.Group, tag.ImageComments.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetSeriesDescription returns the series description (0008,103E)
func GetSeriesDescription(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.SeriesDescription.Group, tag.SeriesDescription.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetSeriesEnergy returns the DICOS series energy value (6100,0030)
// 1 = Low Energy, 2 = High Energy, 0 = not set
func GetSeriesEnergy(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.SeriesEnergy.Group, tag.SeriesEnergy.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// GetSeriesEnergyDescription returns the DICOS energy description (6100,0031)
func GetSeriesEnergyDescription(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.SeriesEnergyDescription.Group, tag.SeriesEnergyDescription.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetEnergyLevel returns the energy level ("he", "le", "") based on DICOM/DICOS tags.
// Priority: SeriesEnergy (6100,0030) > SeriesEnergyDescription > ImageComments > KVP > SeriesDescription
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

// GetPixelData extracts and returns pixel data from the dataset
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

// GetRescale returns the rescale intercept and slope from the dataset.
// If Rescale Intercept is missing, defaults to 0.
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

package dicos

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

// Dataset represents a complete DICOM/DICOS dataset as a map of Tags to Elements.
// A Dataset contains all DICOM data elements from a file or constructed programmatically.
// The Elements map uses Tag (group, element) as the key for efficient lookup.
//
// Example:
//
//	ds, err := dicos.ReadFile("scan.dcs")
//	if err != nil {
//		log.Fatal(err)
//	}
//	elem, ok := ds.FindElement(tag.PatientID.Group, tag.PatientID.Element)
type Dataset struct {
	Elements map[Tag]*Element
}

// Element represents a single DICOM data element with its tag, Value Representation (VR),
// and typed value.
//
// The VR field indicates the data type (e.g., "US" for unsigned short, "CS" for code string,
// "UI" for unique identifier). See the DICOM standard Part 5 for complete VR definitions.
//
// The Value field contains the parsed element value with Go types:
//   - String values: string
//   - Numeric values: uint16, uint32, int, []uint16, []int
//   - Floating point: []float32, []float64
//   - Sequences: []*Dataset
//   - Pixel data: *PixelData
//
// Use the typed getter methods (GetString, GetInt, GetInts, etc.) to safely extract values.
type Element struct {
	Tag   Tag
	VR    string      // Value Representation
	Value interface{} // Parsed value
}

// Tag alias to avoid duplication
type Tag = tag.Tag

// PixelData represents pixel data in either native (uncompressed) or encapsulated (compressed) format.
//
// Native Format (IsEncapsulated=false):
//   - Each Frame has Data populated as []uint16 pixel values
//   - Pixels are stored in row-major order (left-to-right, top-to-bottom)
//   - Multi-frame images store each frame sequentially
//   - Use GetFlatData() to concatenate all frames into a single slice
//
// Encapsulated Format (IsEncapsulated=true):
//   - Each Frame has CompressedData populated as compressed bitstream bytes
//   - Offsets contains the Basic Offset Table per DICOM Part 5 Section 8.2
//   - Each frame is compressed independently (intra-frame only)
//   - Must be decompressed using appropriate codec (see decode.go)
//
// The format is determined by the Transfer Syntax UID in the dataset:
//   - Explicit/Implicit VR Little Endian → Native
//   - JPEG-LS, JPEG 2000, RLE, etc. → Encapsulated
type PixelData struct {
	IsEncapsulated bool
	Frames         []Frame
	Offsets        []uint32 // Basic Offset Table for encapsulated data
}

// Frame represents a single frame (image slice) of pixel data.
//
// For uncompressed data (native pixel data):
//   - Data contains uint16 pixel values in row-major order
//   - CompressedData is nil
//
// For compressed data (encapsulated pixel data):
//   - CompressedData contains the compressed bitstream bytes
//   - Data is nil (must decompress first)
//
// Use Dataset.GetPixelData() to obtain decoded frames, or PixelData.GetFlatData()
// for native data concatenation.
type Frame struct {
	// For native (uncompressed) data
	Data []uint16

	// For encapsulated (compressed) data
	CompressedData []byte
}

// GetFlatData returns all frames concatenated into a single slice of uint16 pixel values.
//
// This method only works with native (uncompressed) pixel data. For encapsulated
// (compressed) pixel data, it returns nil. You must decompress encapsulated data first
// using the appropriate codec before calling GetFlatData.
//
// The returned slice contains pixels in row-major order (left-to-right, top-to-bottom)
// with frames concatenated sequentially.
//
// Example:
//
//	pd, err := ds.GetPixelData()
//	if err != nil {
//		log.Fatal(err)
//	}
//	if pd.IsEncapsulated {
//		// Must decompress first
//		pd, err = DecompressPixelData(ds, pd)
//	}
//	flatData := pd.GetFlatData() // All frames as single slice
func (pd *PixelData) GetFlatData() []uint16 {
	if pd.IsEncapsulated {
		return nil
	}
	var totalPixels int
	for _, f := range pd.Frames {
		totalPixels += len(f.Data)
	}
	res := make([]uint16, totalPixels)
	offset := 0
	for _, f := range pd.Frames {
		copy(res[offset:], f.Data)
		offset += len(f.Data)
	}
	return res
}

// GetFrame returns the frame at the specified index.
//
// Returns an error if the index is out of bounds.
//
// Example:
//
//	pd, _ := ds.GetPixelData()
//	frame, err := pd.GetFrame(0) // First frame
//	if err != nil {
//		log.Fatal(err)
//	}
//	pixels := frame.Data
func (pd *PixelData) GetFrame(index int) (*Frame, error) {
	if index < 0 || index >= len(pd.Frames) {
		return nil, fmt.Errorf("frame index %d out of bounds (0-%d)", index, len(pd.Frames)-1)
	}
	return &pd.Frames[index], nil
}

// NumFrames returns the number of frames in the pixel data.
//
// Example:
//
//	pd, _ := ds.GetPixelData()
//	for i := 0; i < pd.NumFrames(); i++ {
//		frame, _ := pd.GetFrame(i)
//		// Process frame...
//	}
func (pd *PixelData) NumFrames() int {
	return len(pd.Frames)
}

// IsCompressed returns true if the pixel data is encapsulated (compressed).
//
// This is an alias for checking pd.IsEncapsulated for better readability.
//
// Example:
//
//	pd, _ := ds.GetPixelData()
//	if pd.IsCompressed() {
//		// Need to decompress
//	}
func (pd *PixelData) IsCompressed() bool {
	return pd.IsEncapsulated
}

// HasFrames returns true if the pixel data contains at least one frame.
func (pd *PixelData) HasFrames() bool {
	return len(pd.Frames) > 0
}

// FrameSize returns the number of pixels in the first frame, or 0 if no frames exist.
//
// For uncompressed data, this is len(frame.Data).
// For compressed data, this returns 0 (size unknown until decompression).
//
// Example:
//
//	pd, _ := ds.GetPixelData()
//	pixelCount := pd.FrameSize()
//	rows := int(math.Sqrt(float64(pixelCount))) // Assumes square image
func (pd *PixelData) FrameSize() int {
	if len(pd.Frames) == 0 {
		return 0
	}
	if pd.IsEncapsulated {
		return 0 // Unknown until decompression
	}
	return len(pd.Frames[0].Data)
}

// TotalPixels returns the total number of pixels across all frames.
//
// Only valid for uncompressed data. Returns 0 for compressed data.
//
// Example:
//
//	pd, _ := ds.GetPixelData()
//	totalPixels := pd.TotalPixels()
//	avgValue := sum / float64(totalPixels)
func (pd *PixelData) TotalPixels() int {
	if pd.IsEncapsulated {
		return 0
	}
	total := 0
	for _, frame := range pd.Frames {
		total += len(frame.Data)
	}
	return total
}

// FindElement returns an element by tag
func (ds *Dataset) FindElement(group, element uint16) (*Element, bool) {
	elem, ok := ds.Elements[Tag{Group: group, Element: element}]
	return elem, ok
}

// Rows returns the number of rows (image height) from Rows (0028,0010).
// Returns 0 if the element is not present.
func (ds *Dataset) Rows() int {
	if elem, ok := ds.FindElement(0x0028, 0x0010); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// Columns returns the number of columns (image width) from Columns (0028,0011).
// Returns 0 if the element is not present.
func (ds *Dataset) Columns() int {
	if elem, ok := ds.FindElement(0x0028, 0x0011); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// NumberOfFrames returns the number of frames from NumberOfFrames (0028,0008).
// Returns 1 if not specified (single-frame image).
func (ds *Dataset) NumberOfFrames() int {
	if elem, ok := ds.FindElement(0x0028, 0x0008); ok {
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
	return 1
}

// BitsAllocated returns the bits allocated per sample from BitsAllocated (0028,0100).
// Returns 16 as default if not specified.
func (ds *Dataset) BitsAllocated() int {
	if elem, ok := ds.FindElement(0x0028, 0x0100); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 16
}

// PixelRepresentation returns the pixel representation from PixelRepresentation (0028,0103).
// Returns 0 (unsigned) as default if not specified.
func (ds *Dataset) PixelRepresentation() int {
	if elem, ok := ds.FindElement(0x0028, 0x0103); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// Modality returns the Modality (0008,0060) value from the dataset.
// Returns an empty string if the Modality element is not present.
func (ds *Dataset) Modality() string {
	if elem, ok := ds.FindElement(0x0008, 0x0060); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// TransferSyntax returns the Transfer Syntax UID (0002,0010) from the dataset.
// Returns Explicit VR Little Endian as default if not specified.
func (ds *Dataset) TransferSyntax() transfer.Syntax {
	if elem, ok := ds.FindElement(0x0002, 0x0010); ok {
		if s, ok := elem.GetString(); ok {
			return transfer.FromUID(strings.TrimSpace(s))
		}
	}
	return transfer.ExplicitVRLittleEndian
}

// IsEncapsulated returns true if the dataset's pixel data is encapsulated (compressed).
func (ds *Dataset) IsEncapsulated() bool {
	syntax := ds.TransferSyntax()
	return syntax.IsEncapsulated()
}

// GetString returns a string value from an element
func (elem *Element) GetString() (string, bool) {
	if s, ok := elem.Value.(string); ok {
		return s, true
	}
	return "", false
}

// GetUint16 returns a uint16 value from an element
func (elem *Element) GetUint16() (uint16, bool) {
	if u, ok := elem.Value.(uint16); ok {
		return u, true
	}
	return 0, false
}

// GetUint32 returns a uint32 value from an element
func (elem *Element) GetUint32() (uint32, bool) {
	if u, ok := elem.Value.(uint32); ok {
		return u, true
	}
	return 0, false
}

// GetInt returns an int value from an element
func (elem *Element) GetInt() (int, bool) {
	switch v := elem.Value.(type) {
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case int:
		return v, true
	case int32:
		return int(v), true
	case string:
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i, true
		}
	case []byte:
		if len(v) == 2 {
			return int(binary.LittleEndian.Uint16(v)), true
		}
		if len(v) == 4 {
			return int(binary.LittleEndian.Uint32(v)), true
		}
	}
	return 0, false
}

// GetInts returns a slice of ints from an element
func (elem *Element) GetInts() ([]int, bool) {
	switch v := elem.Value.(type) {
	case []uint16:
		res := make([]int, len(v))
		for i, val := range v {
			res[i] = int(val)
		}
		return res, true
	case []uint32:
		res := make([]int, len(v))
		for i, val := range v {
			res[i] = int(val)
		}
		return res, true
	case []int:
		return v, true
	case []byte:
		if len(v)%2 == 0 {
			res := make([]int, len(v)/2)
			for i := 0; i < len(res); i++ {
				res[i] = int(binary.LittleEndian.Uint16(v[i*2:]))
			}
			return res, true
		}
	}
	return nil, false
}

// GetFloats returns a slice of float64s from an element
func (elem *Element) GetFloats() ([]float64, bool) {
	switch v := elem.Value.(type) {
	case []float32:
		res := make([]float64, len(v))
		for i, val := range v {
			res[i] = float64(val)
		}
		return res, true
	case []float64:
		return v, true
	case float32:
		return []float64{float64(v)}, true
	case float64:
		return []float64{v}, true
	}
	return nil, false
}

// GetPixelData returns pixel data from an element if the element value is *PixelData.
// Returns (pixelData, true) if successful, (nil, false) otherwise.
//
// This method is typically used internally when extracting pixel data from the
// PixelData (7FE0,0010) element. Most users should call Dataset.GetPixelData() instead,
// which handles both encapsulated and native formats automatically.
func (elem *Element) GetPixelData() (*PixelData, bool) {
	if pd, ok := elem.Value.(*PixelData); ok {
		return pd, true
	}
	return nil, false
}


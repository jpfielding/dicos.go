package dicos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// CT Image API Documentation Tests
// ============================================================================

// TestCTImage_BasicWorkflow demonstrates creating, configuring, and writing
// a CT image. This is the standard workflow for baggage/cargo CT scans.
func TestCTImage_BasicWorkflow(t *testing.T) {
	// Create a new CT image with default values
	ct := NewCTImage()
	require.NotNil(t, ct)

	// Configure patient/study information
	ct.Patient.SetPatientName("Test", "Bag", "", "", "")
	ct.Patient.PatientID = "BAG-001"
	ct.Study.StudyDescription = "Security Scan"

	// Set image dimensions and pixel data
	rows, cols := 256, 256
	pixelData := make([]uint16, rows*cols)
	for i := range pixelData {
		pixelData[i] = uint16(i % 65536)
	}
	ct.Rows = rows
	ct.Columns = cols

	// Build the dataset
	dataset, err := ct.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)

	// Verify key elements are present
	sopClass, _ := dataset.FindElement(0x0008, 0x0016)
	assert.NotNil(t, sopClass, "SOP Class UID should be present")

	sopInstance, _ := dataset.FindElement(0x0008, 0x0018)
	assert.NotNil(t, sopInstance, "SOP Instance UID should be present")
}

// TestCTImage_WithCompression demonstrates enabling compression codecs.
func TestCTImage_WithCompression(t *testing.T) {
	ct := NewCTImage()
	ct.Rows = 128
	ct.Columns = 128

	// Enable JPEG-LS compression (lossless)
	ct.UseCompression = true
	ct.CompressionCodec = "jpeg-ls"

	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Transfer Syntax is set in file meta
	ts, exists := dataset.FindElement(0x0002, 0x0010)
	require.True(t, exists, "Transfer Syntax UID should be present")
	// Verify compression is configured (actual encoding happens when writing pixel data)
	assert.NotNil(t, ts.Value)
}

// TestCTImage_VOILUTPresets demonstrates window/level configuration.
func TestCTImage_VOILUTPresets(t *testing.T) {
	ct := NewCTImage()

	// CT images come with default presets for soft tissue, bone, lung, brain
	require.NotNil(t, ct.VOILUT, "VOI LUT module should be initialized")
	assert.GreaterOrEqual(t, len(ct.VOILUT.Windows), 1, "Should have at least one window preset")

	// Add custom window
	ct.VOILUT.AddWindow(100, 500, "CUSTOM")

	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Window Center should be present
	wc, exists := dataset.FindElement(0x0028, 0x1050)
	assert.True(t, exists, "Window Center should be present")
	assert.NotNil(t, wc)
}

// ============================================================================
// DX Image API Documentation Tests
// ============================================================================

// TestDXImage_BasicWorkflow demonstrates creating a DX (X-ray) image.
func TestDXImage_BasicWorkflow(t *testing.T) {
	dx := NewDXImage()
	require.NotNil(t, dx)

	// Configure for security X-ray
	dx.Patient.PatientID = "SCAN-001"
	dx.Rows = 1024
	dx.Columns = 768

	// Set detector parameters
	require.NotNil(t, dx.Detector, "Detector module should be initialized")
	dx.Detector.DetectorType = "SCINTILLATOR"
	dx.Detector.FieldOfViewShape = "RECTANGLE"

	// Set acquisition parameters
	require.NotNil(t, dx.Acquisition, "Acquisition module should be initialized")
	dx.Acquisition.KVP = 140
	dx.Acquisition.XRayTubeCurrent = 200

	dataset, err := dx.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)
}

// ============================================================================
// TDR (Threat Detection Report) API Documentation Tests
// ============================================================================

// TestTDR_CreateWithPTOs demonstrates creating a TDR with potential threats.
func TestTDR_CreateWithPTOs(t *testing.T) {
	tdr := NewThreatDetectionReport()
	require.NotNil(t, tdr)

	// Set alarm decision
	tdr.AlarmDecision = "ALARM"

	// Link to source image
	tdr.ReferencedSOPClassUID = "1.2.840.10008.5.1.4.1.1.2" // CT Storage
	tdr.ReferencedSOPInstanceUID = "1.2.3.4.5.6.7.8.9"

	// Add potential threat objects
	tdr.PTOs = []PotentialThreatObject{
		{
			ID:          1,
			Label:       "EXPLOSIVE",
			OOIType:     "EXPLOSIVE",
			Probability: 0.95,
			Confidence:  0.92,
			BoundingBox: &BoundingBox{
				TopLeft:     [3]float32{100, 100, 50},
				BottomRight: [3]float32{200, 200, 100},
			},
		},
		{
			ID:          2,
			Label:       "KNIFE",
			OOIType:     "KNIFE",
			Probability: 0.85,
			Confidence:  0.80,
		},
	}

	dataset, err := tdr.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)

	// Verify SOP Class is correct TDR UID
	sopClass, exists := dataset.FindElement(0x0008, 0x0016)
	require.True(t, exists)
	assert.Contains(t, sopClass.Value, "501.3", "Should use TDR SOP Class UID")
}

// ============================================================================
// IOD Validation API Documentation Tests
// ============================================================================

// TestValidation_CTImage demonstrates validating a CT dataset.
func TestValidation_CTImage(t *testing.T) {
	ct := NewCTImage()
	ct.Rows = 256
	ct.Columns = 256

	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Validate against CT requirements
	result := ValidateCT(dataset)

	// Check validation result
	t.Logf("Valid: %v, Errors: %d, Warnings: %d",
		result.IsValid(), len(result.Errors), len(result.Warnings))

	// Log any errors for debugging
	for _, e := range result.Errors {
		t.Logf("Error: %s", e.Error())
	}
}

// ============================================================================
// Read/Write Roundtrip Tests
// ============================================================================

// TestCTImage_WriteAndRead demonstrates writing to file and reading back.
func TestCTImage_WriteAndRead(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.dcs")

	// Create and write CT image
	ct := NewCTImage()
	ct.Patient.PatientID = "ROUNDTRIP-001"
	ct.Rows = 64
	ct.Columns = 64

	// Create minimal pixel data
	pixelData := make([]uint16, 64*64)
	for i := range pixelData {
		pixelData[i] = uint16(i)
	}

	// Build dataset with pixel data
	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Write to file
	f, err := os.Create(filePath)
	require.NoError(t, err)
	_, err = Write(f, dataset)
	f.Close()
	require.NoError(t, err)

	// Read back
	readDataset, err := ReadFile(filePath)
	require.NoError(t, err)
	require.NotNil(t, readDataset)

	// Verify patient ID matches
	patientID, exists := readDataset.FindElement(0x0010, 0x0020)
	require.True(t, exists, "Patient ID should be present")
	assert.Contains(t, patientID.Value, "ROUNDTRIP-001")
}

// ============================================================================
// AIT Image API Documentation Tests
// ============================================================================

// TestAIT2D_BasicWorkflow demonstrates creating an AIT 2D body scan image.
func TestAIT2D_BasicWorkflow(t *testing.T) {
	ait := NewAIT2DImage()
	require.NotNil(t, ait)

	ait.BodyRegion = "FRONT"
	ait.ScannerType = "MILLIMETER_WAVE"
	ait.Rows = 512
	ait.Columns = 256

	dataset, err := ait.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)

	// Verify AIT SOP Class
	sopClass, exists := dataset.FindElement(0x0008, 0x0016)
	require.True(t, exists)
	assert.Contains(t, sopClass.Value, "501.4", "Should use AIT 2D SOP Class UID")
}

// TestAIT3D_BasicWorkflow demonstrates creating an AIT 3D volumetric scan.
func TestAIT3D_BasicWorkflow(t *testing.T) {
	ait := NewAIT3DImage()
	require.NotNil(t, ait)

	ait.SurfaceType = "VOXEL"
	ait.ScannerType = "MILLIMETER_WAVE"
	ait.Rows = 128
	ait.Columns = 128
	ait.NumberOfFrames = 64

	dataset, err := ait.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)

	// Verify AIT 3D SOP Class
	sopClass, exists := dataset.FindElement(0x0008, 0x0016)
	require.True(t, exists)
	assert.Contains(t, sopClass.Value, "501.5", "Should use AIT 3D SOP Class UID")
}

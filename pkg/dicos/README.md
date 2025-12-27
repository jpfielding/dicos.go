# DICOS Package

A native Go implementation for reading and writing DICOS (Digital Imaging and Communications in Security) files. This package provides full compliance with the NEMA DICOS standard (NEMA IIC 1 v04-2023) for security imaging applications.

## What is DICOS?

DICOS (Digital Imaging and Communications in Security) is an extension of the DICOM (Digital Imaging and Communications in Medicine) standard, specifically designed for security screening applications. It was developed by NEMA (National Electrical Manufacturers Association) to standardize imaging data formats used in:

- **Airport Security**: CT scanners, X-ray systems, and body scanners
- **Cargo Inspection**: Container and vehicle screening systems
- **Critical Infrastructure**: Security checkpoints and threat detection systems

DICOS inherits DICOM's robust data model while adding security-specific extensions for threat detection, automatic target recognition (ATR), and specialized imaging modalities.

## File Structure

A DICOS file (.dcs) follows the DICOM Part 10 file format:

```
┌─────────────────────────────────────────┐
│  Preamble (128 bytes, typically zeros)  │
├─────────────────────────────────────────┤
│  DICM Magic (4 bytes: "DICM")           │
├─────────────────────────────────────────┤
│  File Meta Information (Group 0002)     │
│  - Transfer Syntax UID                  │
│  - SOP Class UID                        │
│  - SOP Instance UID                     │
├─────────────────────────────────────────┤
│  Dataset Elements (sorted by tag)       │
│  - Patient Module                       │
│  - Study Module                         │
│  - Series Module                        │
│  - Equipment Module                     │
│  - Image Pixel Module                   │
│  - Modality-Specific Attributes         │
│  - Pixel Data (7FE0,0010)              │
└─────────────────────────────────────────┘
```

### Data Elements

Each DICOS data element consists of:

| Component | Size | Description |
|-----------|------|-------------|
| Tag | 4 bytes | Group (2 bytes) + Element (2 bytes) |
| VR | 2 bytes | Value Representation (Explicit VR only) |
| Length | 2-4 bytes | Depends on VR type |
| Value | Variable | Actual data |

### Tags

Tags are identified by a (Group, Element) pair in hexadecimal:

- **Group 0002**: File Meta Information
- **Group 0008**: General Information (dates, UIDs, modality)
- **Group 0010**: Patient Information
- **Group 0018**: Acquisition Parameters
- **Group 0020**: Relationship Information (series, study UIDs)
- **Group 0028**: Image Pixel Module
- **Group 4010**: DICOS-Specific (ATD, threat detection)
- **Group 6100**: DICOS Energy Parameters
- **Group 7FE0**: Pixel Data

### Value Representations (VR)

The package supports all standard DICOM VRs:

| VR | Description | Max Length |
|----|-------------|------------|
| AE | Application Entity | 16 |
| AS | Age String | 4 |
| CS | Code String | 16 |
| DA | Date (YYYYMMDD) | 8 |
| DS | Decimal String | 16 |
| IS | Integer String | 12 |
| LO | Long String | 64 |
| OB | Other Byte | Unlimited |
| OW | Other Word | Unlimited |
| PN | Person Name | 64 per component |
| SH | Short String | 16 |
| SQ | Sequence | Unlimited |
| TM | Time | 16 |
| UI | Unique Identifier | 64 |
| UL | Unsigned Long | 4 |
| US | Unsigned Short | 2 |

## Supported Modalities

### CT (Computed Tomography)

3D volumetric imaging for baggage and cargo scanning.

```go
ct := dicos.NewCTImage()
ct.Patient.PatientID = "BAG-001"
ct.SetPixelData(512, 512, volumeData)
ct.UseCompression = true
ct.CompressionCodec = "jpeg-ls"
ct.Write("output.dcs")
```

**SOP Class UIDs:**
- Standard CT: `1.2.840.10008.5.1.4.1.1.2`
- DICOS CT: `1.2.840.10008.5.1.4.1.1.501.1`

### DX (Digital X-Ray)

2D projection imaging for checkpoint screening.

```go
dx := dicos.NewDXImage()
dx.SetPixelData(rows, cols, imageData)
dx.PresentationIntentType = "PRESENTATION"
dx.Write("xray.dcs")
```

**SOP Class UIDs:**
- Standard DX: `1.2.840.10008.5.1.4.1.1.1.1`
- DICOS DX: `1.2.840.10008.5.1.4.1.1.501.2`

### TDR (Threat Detection Report)

Structured reports for automated threat detection results.

```go
tdr := dicos.NewThreatDetectionReport()
tdr.PTOs = []dicos.PotentialThreatObject{
    {
        ID:    1,
        Label: "Prohibited Item",
        BoundingBox: &dicos.BoundingBox{
            TopLeft:     [3]float32{10, 20, 5},
            BottomRight: [3]float32{50, 80, 25},
        },
    },
}
tdr.Write("threat_report.dcs")
```

**SOP Class UID:** `1.2.840.10008.5.1.4.1.1.501.3`

### AIT (Advanced Imaging Technology)

Body scanner imaging (2D and 3D modes).

**SOP Class UIDs:**
- DICOS AIT 2D: `1.2.840.10008.5.1.4.1.1.501.4`
- DICOS AIT 3D: `1.2.840.10008.5.1.4.1.1.501.5`

## Transfer Syntaxes

The package supports multiple transfer syntaxes for pixel data encoding:

| Transfer Syntax | UID | Description |
|-----------------|-----|-------------|
| Implicit VR Little Endian | 1.2.840.10008.1.2 | Legacy, uncompressed |
| Explicit VR Little Endian | 1.2.840.10008.1.2.1 | Default, uncompressed |
| JPEG-LS Lossless | 1.2.840.10008.1.2.4.80 | Recommended for DICOS |
| JPEG Lossless (Process 14 SV1) | 1.2.840.10008.1.2.4.70 | Alternative lossless |
| JPEG 2000 Lossless | 1.2.840.10008.1.2.4.90 | High compression ratio |
| RLE Lossless | 1.2.840.10008.1.2.5 | Simple, fast |

## Library Features

### Reading DICOS Files

```go
// Read from file
ds, err := dicos.ReadFile("scan.dcs")
if err != nil {
    log.Fatal(err)
}

// Read from byte slice
ds, err := dicos.ReadBuffer(data)

// Check modality
if dicos.IsCT(ds) {
    fmt.Println("CT Image")
} else if dicos.IsDX(ds) {
    fmt.Println("DX Image")
} else if dicos.IsTDR(ds) {
    fmt.Println("Threat Detection Report")
}
```

### Accessing Dataset Elements

```go
// Get image dimensions
rows := dicos.GetRows(ds)
cols := dicos.GetColumns(ds)
frames := dicos.GetNumberOfFrames(ds)

// Get pixel properties
bitsAllocated := dicos.GetBitsAllocated(ds)
pixelRep := dicos.GetPixelRepresentation(ds)

// Get rescale values (for Hounsfield units)
intercept, slope := dicos.GetRescale(ds)

// Get modality string
modality := dicos.GetModality(ds)

// Get transfer syntax
ts := dicos.GetTransferSyntax(ds)

// Find any element by tag
if elem, ok := ds.FindElement(0x0010, 0x0020); ok {
    patientID, _ := elem.GetString()
}
```

### Working with Pixel Data

```go
// Get pixel data (handles both native and encapsulated)
pd, err := ds.GetPixelData()
if err != nil {
    log.Fatal(err)
}

// Check if compressed
if pd.IsEncapsulated {
    // Access compressed frames
    for i, frame := range pd.Frames {
        compressedBytes := frame.CompressedData
    }
} else {
    // Access raw pixel values
    flatData := pd.GetFlatData() // Returns []uint16
}
```

### Decoding Volumes

```go
// Decode all frames to a 3D volume (handles decompression)
vol, err := dicos.DecodeVolume(ds)
if err != nil {
    log.Fatal(err)
}

// Access voxels
value := vol.Get(x, y, z)
vol.Set(x, y, z, newValue)

// Get 2D slices
axialSlice := vol.Slice(0, zIndex)    // XY plane
coronalSlice := vol.Slice(1, yIndex)  // XZ plane
sagittalSlice := vol.Slice(2, xIndex) // YZ plane

// Get statistics
min, max := vol.MinMax()
```

### Writing DICOS Files

```go
// Using high-level IOD types
ct := dicos.NewCTImage()
ct.Patient.PatientID = "SCAN-001"
ct.Study.StudyDescription = "Baggage Scan"
ct.Series.SeriesDescription = "High Energy"
ct.SetPixelData(512, 512, volumeData)
ct.UseCompression = true
ct.CompressionCodec = "jpeg-ls" // or "jpeg-li", "rle", "jpeg-2000"
ct.Write("output.dcs")

// Using functional options for custom datasets
ds, err := dicos.NewDataset(
    dicos.WithFileMeta(sopClassUID, sopInstanceUID, transferSyntax),
    dicos.WithElement(tag.PatientID, "PATIENT-001"),
    dicos.WithElement(tag.Rows, 512),
    dicos.WithElement(tag.Columns, 512),
    dicos.WithPixelData(512, 512, 16, pixelData, true, "jpeg-ls"),
)
dicos.WriteFile("custom.dcs", ds)
```

### Energy Level Detection

DICOS supports dual-energy imaging. The library provides utilities to detect energy levels:

```go
// Returns "he" (high energy), "le" (low energy), or ""
energy := dicos.GetEnergyLevel(ds)

// Access specific energy tags
seriesEnergy := dicos.GetSeriesEnergy(ds)        // 1=LE, 2=HE
energyDesc := dicos.GetSeriesEnergyDescription(ds)
kvp := dicos.GetKVP(ds) // Peak kilovoltage
```

## Package Structure

```
pkg/dicos/
├── dicos.go           # Main API: ReadFile, IsCT, GetRows, etc.
├── types.go           # Core types: Dataset, Element, PixelData, Frame
├── reader.go          # DICOM parser implementation
├── writer.go          # DICOM writer implementation
├── decode.go          # Pixel data decompression (JPEG-LS, JPEG, RLE, J2K)
├── volume.go          # 3D volume representation
├── dataset_builder.go # Functional options for building datasets
├── ct.go              # CT Image IOD
├── dx.go              # DX Image IOD
├── tdr.go             # Threat Detection Report IOD
├── util.go            # UID generation utilities
├── compat.go          # Compatibility utilities
├── tag/
│   └── tag.go         # Standard DICOM/DICOS tag definitions
├── vr/
│   └── vr.go          # Value Representation definitions
├── transfer/
│   └── syntax.go      # Transfer Syntax definitions
└── module/
    ├── common.go      # Common types (Date, Time, PersonName)
    ├── patient.go     # Patient Module
    ├── study.go       # General Study Module
    ├── series.go      # General Series Module
    ├── equipment.go   # General Equipment Module
    └── sop_common.go  # SOP Common Module
```

## DICOS-Specific Tags

### ATD (Automatic Threat Detection) Tags - Group 4010

| Tag | Name | Description |
|-----|------|-------------|
| (4010,1006) | PotentialThreatObjectID | Unique ID for detected threat |
| (4010,1010) | PTOSequence | Sequence of threat objects |
| (4010,1011) | PTORepresentationSequence | Threat representation details |
| (4010,1012) | OOIType | Object of Interest type |
| (4010,1017) | ATDAssessmentProbability | Confidence score |
| (4010,1023) | BoundingBoxTopLeft | 3D corner coordinates |
| (4010,1024) | BoundingBoxBottomRight | 3D corner coordinates |
| (4010,1028) | ThreatCategoryDescription | Human-readable threat label |

### Energy Tags - Group 6100

| Tag | Name | Description |
|-----|------|-------------|
| (6100,0030) | SeriesEnergy | Energy level (1=LE, 2=HE) |
| (6100,0031) | SeriesEnergyDescription | Energy description string |

## Compression Support

The library integrates with compression codecs for lossless pixel data handling:

| Codec | Package | Use Case |
|-------|---------|----------|
| JPEG-LS | `pkg/compress/jpegls` | Default, excellent compression |
| JPEG Lossless | `pkg/compress/jpegli` | Wide compatibility |
| JPEG 2000 | `pkg/compress/jpeg2k` | High compression ratio |
| RLE | `pkg/compress/rle` | Simple, fast |

## References

- [NEMA DICOS Standard (IIC 1)](https://www.nema.org/standards/view/digital-imaging-and-communications-in-security)
- [DICOM Standard](https://www.dicomstandard.org/)
- [Stratovan SDICOS Library](https://www.stratovan.com/) (architectural inspiration)

## Example Usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/jpfielding/dicos.go/dicos.go/pkg/dicos"
)

func main() {
    // Read a DICOS file
    ds, err := dicos.ReadFile("baggage_scan.dcs")
    if err != nil {
        log.Fatal(err)
    }

    // Print basic info
    fmt.Printf("Modality: %s\n", dicos.GetModality(ds))
    fmt.Printf("Dimensions: %dx%dx%d\n",
        dicos.GetColumns(ds),
        dicos.GetRows(ds),
        dicos.GetNumberOfFrames(ds))
    fmt.Printf("Transfer Syntax: %s\n", dicos.GetTransferSyntax(ds).Name())
    fmt.Printf("Energy Level: %s\n", dicos.GetEnergyLevel(ds))

    // Decode volume for processing
    vol, err := dicos.DecodeVolume(ds)
    if err != nil {
        log.Fatal(err)
    }

    min, max := vol.MinMax()
    fmt.Printf("Value Range: %d - %d\n", min, max)
}
```

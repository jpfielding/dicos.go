# dicos.go

A pure Go library for reading and writing DICOS (DICOM for Security) files used in baggage and cargo scanning systems.

## Overview

DICOS is a specialized variant of the DICOM standard designed for security screening applications. This library provides full NEMA DICOS compliance with support for:

- Multiple image modalities: CT, DX, AIT2D, AIT3D, TDR
- Compression codecs: JPEG-LS, JPEG 2000, RLE, JPEG Lossless
- Dual-energy scanning systems
- Threat detection reports (TDR)

## Features

- Pure Go implementation (no CGO dependencies)
- Idiomatic API using `io.Reader`/`io.Writer`
- Functional options pattern for dataset construction
- Automatic compression/decompression of pixel data
- Modality-specific builders with sensible defaults
- Command-line tool for DICOS file analysis
- Full support for DICOM transfer syntaxes

## Installation

```bash
go get github.com/jpfielding/dicos.go
```

## Quick Start

### Reading a DICOS File

```go
package main

import (
    "log"
    "github.com/jpfielding/dicos.go/pkg/dicos"
)

func main() {
    // Read a DICOS file
    ds, err := dicos.ReadFile("scan.dcs")
    if err != nil {
        log.Fatal(err)
    }

    // Check modality
    if dicos.IsCT(ds) {
        rows := dicos.GetRows(ds)
        cols := dicos.GetColumns(ds)
        log.Printf("CT Image: %dx%d", rows, cols)
    }

    // Extract pixel data
    pixelData, err := ds.GetPixelData()
    if err != nil {
        log.Fatal(err)
    }

    // Check if data is compressed
    if pixelData.IsEncapsulated {
        log.Println("Pixel data is compressed (encapsulated)")
        // Decompress if needed
        pixelData, err = dicos.DecompressPixelData(ds, pixelData)
        if err != nil {
            log.Fatal(err)
        }
    }

    // Get rescale parameters for Hounsfield Units
    intercept, slope := dicos.GetRescale(ds)
    log.Printf("Rescale: intercept=%.2f, slope=%.2f", intercept, slope)
}
```

### Creating a CT Image

```go
package main

import (
    "log"
    "os"
    "github.com/jpfielding/dicos.go/pkg/dicos"
)

func main() {
    // Create a new CT image with builder
    ct := dicos.NewCTImage()

    // Set patient information
    ct.Patient.PatientID = "BAG-001"
    ct.Patient.SetPatientName("Anonymous", "Baggage", "", "", "")

    // Set image dimensions and parameters
    ct.Rows = 512
    ct.Columns = 512
    ct.CTImageMod.SliceThickness = "1.0"
    ct.CTImageMod.KVP = "120"

    // Generate pixel data (example: gradient)
    pixelData := make([]uint16, 512*512)
    for i := range pixelData {
        pixelData[i] = uint16(i % 65536)
    }

    // Set pixel data and enable JPEG-LS compression
    ct.SetPixelData(512, 512, pixelData)
    ct.Codec = dicos.CodecJPEGLS // Compress using JPEG-LS (recommended for DICOS)

    // Write directly to file (calls GetDataset internally)
    _, err := ct.Write("output.dcs")
    if err != nil {
        log.Fatal(err)
    }

    log.Println("DICOS file written successfully")
}
```

### Using Functional Options

```go
package main

import (
    "github.com/jpfielding/dicos.go/pkg/dicos"
    "github.com/jpfielding/dicos.go/pkg/dicos/module"
    "github.com/jpfielding/dicos.go/pkg/dicos/transfer"
)

func main() {
    // Create modules
    patient := &module.PatientModule{}
    patient.PatientID = "BAG-001"
    patient.SetPatientName("Anonymous", "Baggage", "", "", "")

    study := &module.GeneralStudyModule{}
    study.StudyInstanceUID = dicos.GenerateUID("1.2.826.0.1.3680043.8.498.")
    study.StudyDate = module.NewDate(time.Now())

    // Generate pixel data
    pixelData := make([]uint16, 512*512)
    for i := range pixelData {
        pixelData[i] = uint16(i % 65536)
    }

    // Build dataset with functional options
    ds, err := dicos.NewDataset(
        dicos.WithFileMeta(
            dicos.DICOSCTImageStorageUID,
            dicos.GenerateUID("1.2.826.0.1.3680043.8.498."),
            dicos.JPEGLSLossless, // Transfer syntax for JPEG-LS
        ),
        dicos.WithModule(patient.ToTags()),
        dicos.WithModule(study.ToTags()),
        dicos.WithPixelData(512, 512, 16, pixelData, dicos.CodecJPEGLS), // Codec as last param
    )
    if err != nil {
        log.Fatal(err)
    }

    // Write to file
    f, _ := os.Create("output.dcs")
    defer f.Close()
    dicos.Write(f, ds)
}
```

## Error Handling

The library follows Go conventions for error handling. Most functions return errors that should be checked:

```go
// Reading files
ds, err := dicos.ReadFile("scan.dcs")
if err != nil {
    // Common errors:
    // - File not found
    // - Invalid DICOM format (missing preamble or DICM magic)
    // - Corrupt or truncated data
    log.Fatalf("Failed to read DICOS file: %v", err)
}

// Extracting pixel data
pd, err := ds.GetPixelData()
if err != nil {
    // Common errors:
    // - PixelData element not found
    // - Invalid pixel data type
    // - Dimension mismatch (truncated frames)
    log.Fatalf("Failed to extract pixel data: %v", err)
}

// Decompressing pixel data
if pd.IsEncapsulated {
    pd, err = dicos.DecompressPixelData(ds, pd)
    if err != nil {
        // Common errors:
        // - Unsupported transfer syntax
        // - Corrupt compressed data
        // - Codec decoding failure
        log.Fatalf("Failed to decompress pixel data: %v", err)
    }
}

// Building datasets
ds, err := dicos.NewDataset(
    dicos.WithFileMeta(sopClass, sopInstance, transferSyntax),
    // ... more options
)
if err != nil {
    // Common errors:
    // - Invalid option values
    // - Compression codec errors
    log.Fatalf("Failed to build dataset: %v", err)
}

// Writing files
_, err = ct.Write("output.dcs")
if err != nil {
    // Common errors:
    // - Permission denied
    // - Disk full
    // - Invalid dataset (missing required elements)
    log.Fatalf("Failed to write DICOS file: %v", err)
}
```

### Error Types

- **File I/O Errors**: Standard Go file errors (os.ErrNotExist, os.ErrPermission, etc.)
- **Parse Errors**: `fmt.Errorf` with context about what failed (e.g., "invalid DICM magic")
- **Validation Errors**: Missing or invalid required DICOM elements
- **Codec Errors**: Compression/decompression failures with codec-specific details

All errors include contextual information to help diagnose issues. Use `fmt.Errorf` wrapping to preserve error chains.

## Command-Line Tool

The `ctl` tool provides DICOS file analysis capabilities:

```bash
# Build the tool
go build -o ctl ./cmd/ctl

# Analyze a DICOS file
./ctl analyze scan.dcs
```

## Building and Testing

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./pkg/dicos
go test ./pkg/compress/jpegls

# Build with version information
go build -ldflags "-X main.GitSHA=$(git rev-parse HEAD)" -o ctl ./cmd/ctl

# Install dependencies (uses vendored dependencies)
go mod download
go mod vendor
```

## Architecture

The library is organized into several key packages:

- **`pkg/dicos/`** - Core DICOS library with reader, writer, and high-level API
- **`pkg/dicos/module/`** - DICOM Information Object Definition (IOD) modules
- **`pkg/dicos/tag/`** - DICOM tag definitions
- **`pkg/dicos/vr/`** - Value Representation definitions
- **`pkg/dicos/transfer/`** - Transfer syntax definitions
- **`pkg/compress/jpegls/`** - JPEG-LS codec implementation
- **`pkg/compress/jpeg2k/`** - JPEG 2000 codec implementation
- **`pkg/compress/jpegli/`** - JPEG Lossless codec implementation
- **`pkg/compress/rle/`** - RLE codec implementation
- **`cmd/ctl/`** - Command-line utility

## Supported Modalities

- **CT** (Computed Tomography) - High-resolution 3D scans
- **DX** (Digital X-ray) - 2D projection images
- **AIT2D** (Advanced Imaging Technology 2D) - Millimeter wave imaging
- **AIT3D** (Advanced Imaging Technology 3D) - 3D body scanners
- **TDR** (Threat Detection Report) - Automated threat detection results

## Compression Support

The library includes native Go implementations of all DICOS-relevant codecs:

- JPEG-LS (lossless and near-lossless)
- JPEG 2000 (lossy and lossless)
- RLE (Run-Length Encoding)
- JPEG Lossless (Process 14)

## Development

See [CLAUDE.md](CLAUDE.md) for detailed development guidelines, coding conventions, and architectural documentation.

### Development Environment

The project includes a devcontainer configuration for consistent development:

```bash
# Open in VSCode with devcontainer support
code .
```

## Contributing

Contributions are welcome. Please follow the coding conventions outlined in [CLAUDE.md](CLAUDE.md):

- Write idiomatic Go code
- Use table-driven tests
- Prefer `io.Reader`/`io.Writer` over file paths
- Avoid CGO dependencies
- Follow the Go Proverbs

## License

[Add your license here]

## References

- [NEMA PS3.1 DICOM Standard](https://www.dicomstandard.org/)
- [NEMA PS3.17 DICOS Explanatory Information](https://www.dicomstandard.org/)
- [JPEG-LS ITU-T T.87](https://www.itu.int/rec/T-REC-T.87/)
- [JPEG 2000 ITU-T T.800](https://www.itu.int/rec/T-REC-T.800/)

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
    pixelData, err := dicos.GetPixelData(ds)
    if err != nil {
        log.Fatal(err)
    }

    // Get rescale parameters for Hounsfield Units
    slope, intercept := dicos.GetRescale(ds)
    log.Printf("Rescale: slope=%.2f, intercept=%.2f", slope, intercept)
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
    ct.Patient.PatientID = "BAG-001"
    ct.Patient.PatientName = "Anonymous"
    ct.Rows = 512
    ct.Columns = 512
    ct.SliceThickness = 1.0
    ct.PixelData = make([]uint16, 512*512) // Your pixel data here
    ct.UseCompression = true
    ct.CompressionCodec = "jpeg-ls"

    // Build the dataset
    ds, err := ct.GetDataset()
    if err != nil {
        log.Fatal(err)
    }

    // Write to file
    f, err := os.Create("output.dcs")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()

    if err := dicos.Write(f, ds); err != nil {
        log.Fatal(err)
    }
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
    patient := module.Patient{
        PatientID:   "BAG-001",
        PatientName: "Anonymous",
    }

    ds, err := dicos.NewDataset(
        dicos.WithFileMeta(
            "1.2.840.10008.5.1.4.1.1.2", // CT Image SOP Class UID
            "1.2.3.4.5.6.7.8.9",          // SOP Instance UID
            transfer.ExplicitVRLittleEndian,
        ),
        dicos.WithModule(patient.ToTags()),
        dicos.WithPixelData(512, 512, 16, pixelData, true, "jpeg-ls"),
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

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

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a Go library for reading and writing DICOS (DICOM for Security) files, which are used in baggage/cargo scanning systems. The library provides full NEMA DICOS compliance with support for multiple image modalities (CT, DX, AIT2D, AIT3D, TDR) and compression codecs.

## Coding Conventions

### General Principles
- **Idiomatic Go is primary** - follow Go conventions first, see Go Proverbs
- **Prefer io.Reader/Writer** over file paths for APIs
- **No CGO unless necessary** - pure Go implementations preferred
- **Build artifacts go to `bin/`**, never root directory
- **Tagged switch > if/else** chains
- **Avoid init() functions** - prefer explicit registration
- **Exported over unexported types** - avoid `internal/` packages

### Testing Philosophy
- **TDD for design iteration** - write tests to explore APIs
- **Unit tests document expected usage** - they are living examples
- **Self-contained tests** - no external data dependencies
- **Use testify** for assertions and test suites
- **No tests in cmd/** - command-line tools tested via integration
- **Prefer in-memory over disk I/O** in tests
- **Minimal logging** - use DEBUG level, disabled by default

### Error Handling
- **Return early** - avoid nested error checks
- **No labeled breaks** - use immediately-invoked functions that return
- **Context in errors** - log what operation failed, not just "error occurred"

### Concurrency
- **Avoid goroutines in APIs** - use callbacks to let callers control concurrency
- **Prefer channels** over waitgroups and mutexes
- **context.Context for cancellation** over custom channels
- **Mutexes only for caches** accessed from multiple goroutines

## Build and Test Commands

### Testing
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./pkg/dicos
go test ./pkg/compress/jpegls

# Run a single test
go test -run TestCTImage_BasicWorkflow ./pkg/dicos

# Run tests with verbose output
go test -v ./pkg/dicos
```

### Building
```bash
# Build the ctl command-line tool
go build -o ctl ./cmd/ctl

# Build with version information
go build -ldflags "-X main.GitSHA=$(git rev-parse HEAD)" -o ctl ./cmd/ctl

# Install dependencies (uses vendored dependencies)
go mod download
go mod vendor
```

### Running the CLI Tool
```bash
# The ctl tool provides DICOS analysis capabilities
./ctl analyze <file.dcs>
```

## Architecture

### Core Package Structure

**`pkg/dicos/`** - Main DICOS library
- `reader.go`: Low-level DICOM/DICOS file parser. Handles preamble, DICM magic, File Meta Information (always Explicit VR Little Endian), and dataset elements. Supports both Explicit and Implicit VR, handles encapsulated pixel data (compressed) and native pixel data (uncompressed).
- `writer.go`: DICOM/DICOS file writer. Writes preamble, DICM magic, sorts elements by tag, and handles both native and encapsulated pixel data encoding.
- `dicos.go`: High-level API with convenience functions for reading files (`ReadFile`, `ReadBuffer`), checking modality types (`IsCT`, `IsDX`, `IsTDR`, `IsAIT2D`, `IsAIT3D`), extracting metadata (`GetRows`, `GetColumns`, `GetTransferSyntax`, `GetEnergyLevel`), and accessing pixel data (`GetPixelData`, `GetRescale`).
- `types.go`: Core data structures (`Dataset`, `Element`, `PixelData`, `Frame`). Dataset is a map of Tags to Elements. Elements contain Tag, VR (Value Representation), and typed Value.
- `dataset_builder.go`: Functional options pattern for creating DICOS datasets. Provides `WithElement`, `WithPixelData`, `WithModule`, `WithFileMeta`, and `WithSequence` options.
- `decode.go`: Decompression logic for encapsulated pixel data. Routes to appropriate codec (JPEG-LS, JPEG 2000, RLE, JPEG Lossless) based on transfer syntax.
- IOD-specific files (`ct.go`, `dx.go`, `tdr.go`, `ait2d.go`, `ait3d.go`): High-level constructors (e.g., `NewCTImage()`, `NewDXImage()`) that initialize datasets with required modules and sensible defaults for each modality.

**`pkg/dicos/module/`** - DICOM Information Object Definition (IOD) modules
- Implements standard DICOM modules: Patient, Study, Series, Equipment, SOPCommon, FrameOfReference, CTImage, DXDetector, VOILUT, OOI (Object of Interest)
- Each module implements `IODModule` interface with `ToTags() []IODElement` method
- Modules are composable and reused across different modalities (e.g., Patient module used in CT, DX, TDR)

**`pkg/dicos/tag/`** - DICOM tag definitions
- `tag.go`: Defines all DICOM/DICOS tags as constants (e.g., `tag.SOPClassUID`, `tag.PixelData`)
- Tags are structs with Group and Element uint16 fields

**`pkg/dicos/transfer/`** - Transfer syntax definitions
- Defines transfer syntax UIDs and properties (encoding, byte order)
- Supports: Explicit VR Little Endian, Implicit VR Little Endian, JPEG-LS, JPEG 2000, RLE, JPEG Lossless

**`pkg/dicos/vr/`** - Value Representation (VR) definitions
- Maps tags to their VRs (data types like US=Unsigned Short, CS=Code String, UI=UID, etc.)

### Compression Codecs

**`pkg/compress/jpegls/`** - JPEG-LS codec (lossless/near-lossless)
- `decoder.go`: Reads JPEG-LS bitstream, parses markers (SOF55, LSE, SOS), decodes using run mode and regular mode
- `encoder.go`: Writes JPEG-LS bitstream with proper markers and parameters
- `context.go`: Context modeling for prediction
- `predictor.go`: JPEG-LS prediction algorithm (uses neighbor pixels)
- `run_mode.go`: Run-length encoding for flat regions
- `bitstream.go`: Bit-level I/O operations

**`pkg/compress/jpeg2k/`** - JPEG 2000 codec
- `jpeg2k.go`: Main entry point for JPEG 2000 decoding
- `codestream.go`: Parses JPEG 2000 codestream structure
- `markers.go`: JPEG 2000 marker definitions
- `dwt.go`: Discrete Wavelet Transform (forward/inverse)
- `ebcot.go`: Embedded Block Coding with Optimized Truncation (EBCOT) arithmetic coding
- `mq.go`: MQ arithmetic coder
- `rct.go`: Reversible Color Transform
- `tile.go`: Tile-based image processing

**`pkg/compress/jpegli/`** - JPEG Lossless (Process 14) codec
- `decode.go`: JPEG lossless decoder
- `encode.go`: JPEG lossless encoder
- Handles first-order predictor (Huffman-coded)

**`pkg/compress/rle/`** - RLE (Run-Length Encoding) codec
- `decode.go`: RLE decoder for DICOM RLE format
- `encode.go`: RLE encoder
- `packbits.go`: PackBits variant used in some DICOM files

### Command-Line Tool

**`cmd/ctl/`** - DICOS command-line utility
- `main.go`: Entry point with signal handling and logging setup
- `cmd/analyze.go`: Analyzes DICOS files (dumps metadata, validates structure)
- Uses Cobra for CLI framework

## Key Design Patterns

### Dataset Construction
The library uses functional options pattern for building datasets:
```go
ds, err := dicos.NewDataset(
    dicos.WithFileMeta(sopClassUID, sopInstanceUID, transferSyntaxUID),
    dicos.WithModule(patient.ToTags()),
    dicos.WithPixelData(rows, cols, bitsAllocated, pixelData, compress, codec),
)
```

### Modality-Specific Builders
High-level constructors provide sensible defaults for each modality:
```go
ct := dicos.NewCTImage()
ct.Patient.PatientID = "BAG-001"
ct.Rows = 512
ct.Columns = 512
ct.PixelData = pixelData
ct.UseCompression = true
ct.CompressionCodec = "jpeg-ls"
dataset, err := ct.GetDataset()
```

### Tag Access
Tags are strongly typed and accessed via helper methods:
```go
elem, ok := ds.FindElement(tag.SOPClassUID.Group, tag.SOPClassUID.Element)
if ok {
    sopClass, _ := elem.GetString()
}
```

### Pixel Data Handling
Pixel data can be native (uncompressed) or encapsulated (compressed):
- Native: `[]uint16` arrays stored directly, multiple frames concatenated
- Encapsulated: Each frame stored as separate compressed blob with Basic Offset Table
- The library automatically detects and handles both formats during read
- During write, `UseCompression` flag determines which format to use

### Energy Level Detection
DICOS files from dual-energy scanners encode energy level via multiple fallback strategies:
1. SeriesEnergy tag (6100,0030): 1=LE, 2=HE
2. SeriesEnergyDescription tag (6100,0031): text contains "high"/"low"
3. ImageComments tag (0020,4000): text contains "high_energy"/"low_energy"
4. KVP tag (0018,0060): >=110 is HE, <110 is LE
5. SeriesDescription tag (0008,103E): text contains hints like "_he", "density2"

## Important Conventions

### Transfer Syntax Handling
- File Meta Information (Group 0002) is ALWAYS Explicit VR Little Endian (per DICOM standard)
- After File Meta, the transfer syntax specified in (0002,0010) determines encoding for rest of file
- Default transfer syntax if missing: Implicit VR Little Endian

### Pixel Representation
- CT images: Often marked as Unsigned (PixelRepresentation=0) but contain signed values offset by +32768
- The `GetRescale()` function applies heuristics to detect this and return appropriate intercept (-32768)
- Hounsfield Units (HU) are computed as: HU = (RawValue * RescaleSlope) + RescaleIntercept

### VR and Value Encoding
- Long VRs (OB, OW, SQ, UT, etc.) use 4-byte value length with 2 reserved bytes
- Short VRs (US, CS, DA, etc.) use 2-byte value length
- Strings are padded to even length with space character
- Multi-valued strings are backslash-separated

### Module Composition
DICOS modalities are composed of required and optional modules:
- CT Image: Patient + Study + Series + Equipment + FrameOfReference + CTImage + VOILUT + SOPCommon
- DX Image: Patient + Study + Series + Equipment + DXDetector + VOILUT + SOPCommon
- TDR (Threat Detection Report): Patient + Study + Series + Equipment + OOI + SOPCommon

## Testing Strategy

Tests use table-driven patterns and roundtrip validation:
- Unit tests for each codec (encode/decode roundtrip)
- API tests demonstrating usage patterns (`api_test.go`)
- Integration tests with real DICOS files (if available)
- Tests use `testify/assert` and `testify/require` for assertions

## Development Environment

The project uses devcontainers for consistent development:
- Based on Rocky Linux 9
- Go toolchain installed
- VSCode extensions: golang.go, ms-vscode.hexeditor, Anthropic.claude-code
- Mounts for git credentials, SSH keys, and history

## External Dependencies

Key external imports (not vendored):
- `github.com/spf13/cobra` - CLI framework
- `github.com/stretchr/testify` - Testing assertions
- `fyne.io/fyne/v2` - GUI framework (vendored)

Note: The project vendors most dependencies (vendor/ directory) for reproducible builds.
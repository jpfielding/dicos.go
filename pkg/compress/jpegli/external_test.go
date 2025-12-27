package jpegli

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"os"
	"testing"
)

// TestDecodeExternalFrame tests decoding a JPEG Lossless frame from DICOS files.
// This is a regression test for the EOF error encountered with external JPEG Lossless files.
func TestDecodeExternalFrame(t *testing.T) {
	// Path to test data
	testFile := "../../dicos/testdata/example.dcs"

	// Extract first JPEG frame from DICOS file
	frame, err := extractFirstFrame(testFile)
	if err != nil {
		t.Fatalf("Failed to extract frame: %v", err)
	}

	t.Logf("Extracted frame: %d bytes", len(frame))
	t.Logf("First 32 bytes: % X", frame[:min(32, len(frame))])
	t.Logf("Last 32 bytes: % X", frame[len(frame)-32:])

	// Find SOS and show scan data start
	for i := 0; i < len(frame)-1; i++ {
		if frame[i] == 0xFF && frame[i+1] == 0xDA {
			sosLen := int(frame[i+2])<<8 | int(frame[i+3])
			scanStart := i + 2 + sosLen
			t.Logf("SOS at 0x%04X, scan starts at 0x%04X", i, scanStart)
			t.Logf("First 32 scan bytes: % X", frame[scanStart:min(scanStart+32, len(frame))])
			break
		}
	}

	// Analyze JPEG markers
	analyzeMarkers(t, frame)

	// Attempt to decode
	img, err := Decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	bounds := img.Bounds()
	t.Logf("Decoded image: %dx%d", bounds.Dx(), bounds.Dy())

	// Expected dimensions from the DICOS file (312x312)
	if bounds.Dx() != 312 || bounds.Dy() != 312 {
		t.Errorf("Dimension mismatch: got %dx%d, want 312x312", bounds.Dx(), bounds.Dy())
	}
}

// extractFirstFrame extracts the first JPEG frame from a DICOS file with encapsulated pixel data.
func extractFirstFrame(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r io.Reader = f

	// Handle gzip
	if len(path) > 3 && path[len(path)-3:] == ".gz" {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		r = gr
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Skip preamble (128 bytes) + DICM magic (4 bytes)
	pos := 132

	// Find Pixel Data tag (7FE0,0010) - little endian
	for pos < len(data)-8 {
		if data[pos] == 0xE0 && data[pos+1] == 0x7F &&
			data[pos+2] == 0x10 && data[pos+3] == 0x00 {
			// Found Pixel Data tag
			// Skip Tag(4) + VR(2) + Reserved(2) + Length(4) = 12 bytes
			pos += 12

			// Read BOT (Basic Offset Table) item tag (FFFE,E000)
			if data[pos] == 0xFE && data[pos+1] == 0xFF &&
				data[pos+2] == 0x00 && data[pos+3] == 0xE0 {
				botLen := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
				pos += 8 + int(botLen)
			}

			// Read first frame item tag (FFFE,E000)
			if data[pos] == 0xFE && data[pos+1] == 0xFF &&
				data[pos+2] == 0x00 && data[pos+3] == 0xE0 {
				itemLen := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
				return data[pos+8 : pos+8+int(itemLen)], nil
			}
		}
		pos++
	}

	return nil, io.EOF
}

// analyzeMarkers logs the JPEG markers found in the frame data.
func analyzeMarkers(t *testing.T, data []byte) {
	t.Helper()
	pos := 0
	for pos < len(data)-1 {
		if data[pos] != 0xFF {
			pos++
			continue
		}
		marker := data[pos+1]
		if marker == 0x00 || (0xD0 <= marker && marker <= 0xD7) {
			pos++
			continue
		}

		markerPos := pos
		pos += 2

		switch marker {
		case 0xD8: // SOI
			t.Logf("  SOI at 0x%04X", markerPos)
		case 0xD9: // EOI
			t.Logf("  EOI at 0x%04X", markerPos)
			return
		case 0xDA: // SOS
			if pos+2 > len(data) {
				return
			}
			sosLen := int(data[pos])<<8 | int(data[pos+1])
			ns := data[pos+2]
			ss := data[pos+2+1+int(ns)*2] // Predictor
			al := data[pos+2+1+int(ns)*2+2] & 0x0F
			t.Logf("  SOS at 0x%04X: Ns=%d, Predictor=%d, Al=%d, len=%d", markerPos, ns, ss, al, sosLen)
			return
		case 0xC3: // SOF3
			if pos+2 > len(data) {
				return
			}
			length := int(data[pos])<<8 | int(data[pos+1])
			precision := data[pos+2]
			height := int(data[pos+3])<<8 | int(data[pos+4])
			width := int(data[pos+5])<<8 | int(data[pos+6])
			nf := data[pos+7]
			t.Logf("  SOF3 at 0x%04X: %dx%d, %dbit, %d components", markerPos, width, height, precision, nf)
			pos += length
		case 0xC4: // DHT
			if pos+2 > len(data) {
				return
			}
			length := int(data[pos])<<8 | int(data[pos+1])
			tcTh := data[pos+2]
			tc := tcTh >> 4
			th := tcTh & 0x0F
			var totalCodes int
			bits := make([]int, 16)
			for i := 0; i < 16; i++ {
				bits[i] = int(data[pos+3+i])
				totalCodes += bits[i]
			}
			values := data[pos+19 : pos+19+totalCodes]
			t.Logf("  DHT at 0x%04X: Tc=%d, Th=%d, %d codes", markerPos, tc, th, totalCodes)
			t.Logf("    BITS (codes per length 1-16): %v", bits)
			t.Logf("    VALUES: %v", values)
			pos += length
		case 0xE0: // APP0
			if pos+2 > len(data) {
				return
			}
			length := int(data[pos])<<8 | int(data[pos+1])
			t.Logf("  APP0 at 0x%04X, len=%d", markerPos, length)
			pos += length
		default:
			if pos+2 > len(data) {
				return
			}
			length := int(data[pos])<<8 | int(data[pos+1])
			t.Logf("  0x%02X at 0x%04X, len=%d", marker, markerPos, length)
			pos += length
		}
	}
}

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"

	"github.com/spf13/cobra"
	jpegli "github.com/jpfielding/dicos.go/pkg/compress/jpegli"
	jpegls "github.com/jpfielding/dicos.go/pkg/compress/jpegls"
	dicos "github.com/jpfielding/dicos.go/pkg/dicos"
)

// NewAnalyzeCmd creates the analyze cobra command
func NewAnalyzeCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze DICOS/DICOM file structure",
		Long:  "Parses and displays detailed information about a DICOS/DICOM file including metadata and pixel data frames.",
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			dumpFrame, _ := cmd.Flags().GetInt("dump-frame")
			out, _ := cmd.Flags().GetString("out")

			if filePath == "" && len(args) > 0 {
				filePath = args[0]
			}

			if filePath == "" {
				return fmt.Errorf("file path is required. Use --file flag or provide as argument")
			}

			return runAnalyze(filePath, dumpFrame, out)
		},
	}

	pf := cmd.PersistentFlags()
	pf.StringP("file", "f", "", "DICOS/DICOM file path to analyze")
	pf.Int("dump-frame", -1, "Index of frame to dump to disk")
	pf.String("out", "", "Output path for dumped frame")

	return cmd
}

// runAnalyze performs the DICOS file analysis using pkg/dicos
func runAnalyze(filePath string, dumpFrame int, outPath string) error {
	// Use the new pkg/dicos API
	ds, err := dicos.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	fmt.Printf("Total elements: %d\n\n", len(ds.Elements))

	// Print key metadata
	fmt.Println("=== Key Metadata ===")

	modality := dicos.GetModality(ds)
	fmt.Printf("Modality: %s\n", modality)

	rows := dicos.GetRows(ds)
	cols := dicos.GetColumns(ds)
	fmt.Printf("Rows: %d\n", rows)
	fmt.Printf("Columns: %d\n", cols)

	bitsAllocated := dicos.GetBitsAllocated(ds)
	fmt.Printf("BitsAllocated: %d\n", bitsAllocated)

	pixelRep := dicos.GetPixelRepresentation(ds)
	fmt.Printf("PixelRepresentation: %d (0=unsigned, 1=signed)\n", pixelRep)

	numFrames := dicos.GetNumberOfFrames(ds)
	fmt.Printf("NumberOfFrames: %d\n", numFrames)

	syntax := dicos.GetTransferSyntax(ds)
	fmt.Printf("TransferSyntax: %s (%s)\n", syntax, syntax.Name())

	isEncapsulated := syntax.IsEncapsulated()
	fmt.Printf("Encapsulated: %v\n", isEncapsulated)

	fmt.Println()

	// Analyze pixel data
	pd, err := ds.GetPixelData()
	if err != nil {
		fmt.Printf("No pixel data: %v\n", err)
		return nil
	}

	fmt.Println("=== Pixel Data ===")
	fmt.Printf("IsEncapsulated: %v\n", pd.IsEncapsulated)
	fmt.Printf("Frames: %d\n", len(pd.Frames))

	if len(pd.Offsets) > 0 {
		fmt.Printf("BOT Offsets: %v\n", pd.Offsets)
	}

	if dumpFrame >= 0 {
		if dumpFrame >= len(pd.Frames) {
			return fmt.Errorf("frame index %d out of bounds (0-%d)", dumpFrame, len(pd.Frames)-1)
		}
		fr := pd.Frames[dumpFrame]
		var data []byte

		if pd.IsEncapsulated {
			data = fr.CompressedData
		} else {
			// Convert []uint16 to []byte (Little Endian)
			data = make([]byte, len(fr.Data)*2)
			for i, v := range fr.Data {
				data[i*2] = byte(v)
				data[i*2+1] = byte(v >> 8)
			}
		}

		if outPath == "" {
			outPath = fmt.Sprintf("frame_%d.bin", dumpFrame)
		}

		fmt.Printf("Dumping frame %d (%d bytes) to %s\n", dumpFrame, len(data), outPath)
		return os.WriteFile(outPath, data, 0644)
	}

	// Analyze first few frames
	maxFramesToShow := 3
	if len(pd.Frames) < maxFramesToShow {
		maxFramesToShow = len(pd.Frames)
	}

	for i := 0; i < maxFramesToShow; i++ {
		fr := pd.Frames[i]
		fmt.Printf("\n--- Frame %d ---\n", i)

		if pd.IsEncapsulated {
			fmt.Printf("Compressed size: %d bytes\n", len(fr.CompressedData))
			if len(fr.CompressedData) > 20 {
				fmt.Printf("First 20 bytes: % X\n", fr.CompressedData[:20])
			}

			// Try decoding the JPEG frame
			decoded, err := analyzeDecodeJPEGFrame(fr.CompressedData)
			if err != nil {
				fmt.Printf("Decode error: %v\n", err)
				continue
			}
			fmt.Printf("Decoded pixels: %d\n", len(decoded))
			if len(decoded) > 0 {
				minVal, maxVal := decoded[0], decoded[0]
				for _, v := range decoded {
					if v < minVal {
						minVal = v
					}
					if v > maxVal {
						maxVal = v
					}
				}
				fmt.Printf("Pixel range: min=%d, max=%d\n", minVal, maxVal)
			}
		} else {
			fmt.Printf("Native pixels: %d\n", len(fr.Data))
			if len(fr.Data) > 0 {
				minVal, maxVal := fr.Data[0], fr.Data[0]
				for _, v := range fr.Data {
					if v < minVal {
						minVal = v
					}
					if v > maxVal {
						maxVal = v
					}
				}
				fmt.Printf("Pixel range: min=%d, max=%d\n", minVal, maxVal)
			}
		}
	}

	// Try decoding entire volume
	fmt.Println("\n=== Volume Decode Test ===")
	vol, err := dicos.DecodeVolume(ds)
	if err != nil {
		fmt.Printf("Volume decode error: %v\n", err)
	} else {
		minVal, maxVal := vol.MinMax()
		fmt.Printf("Volume: %dx%dx%d\n", vol.Width, vol.Height, vol.Depth)
		fmt.Printf("Voxel range: min=%d, max=%d\n", minVal, maxVal)
	}

	return nil
}

// analyzeDecodeJPEGFrame decodes a JPEG frame, handling JPEG-LS and standard JPEG
func analyzeDecodeJPEGFrame(data []byte) ([]int, error) {
	// Check for JPEG-LS (FF F7) or JPEG Lossless (FF C3)
	isJPEGLS := false
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF {
			if data[i+1] == 0xF7 {
				isJPEGLS = true
				break
			}
		}
	}

	if isJPEGLS {
		decoded, err := jpegls.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}

		// Convert image.Image to []int
		bounds := decoded.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		pixels := make([]int, width*height)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, _, _, _ := decoded.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
				pixels[y*width+x] = int(r)
			}
		}
		return pixels, nil
	}

	// Check for JPEG Lossless (Process 14) - SOF3 (FF C3)
	// Or standard JPEG if generic
	isLossless := false
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF && data[i+1] == 0xC3 {
			isLossless = true
			break
		}
	}

	if isLossless {
		decoded, err := jpegli.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("jpegli decode error: %w", err)
		}

		bounds := decoded.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		pixels := make([]int, width*height)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, _, _, _ := decoded.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
				pixels[y*width+x] = int(r)
			}
		}
		return pixels, nil
	}

	// Use standard Go jpeg decoder for baseline/lossy JPEG
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]int, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			gray := (r + g + b) / 3 >> 8
			pixels[y*width+x] = int(gray)
		}
	}
	return pixels, nil
}

// Ensure image/jpeg is registered
var _ image.Image

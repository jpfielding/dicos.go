package dicos

import (
	"fmt"
)

func pixelDataOption(rows, cols, bitsAllocated int, pd *PixelData, codec Codec) (Option, error) {
	if pd == nil {
		return nil, nil
	}

	if pd.IsEncapsulated() {
		if codec == nil {
			return nil, fmt.Errorf("encapsulated pixel data requires Codec to be set")
		}
		return WithRawPixelData(pd), nil
	}

	if codec != nil {
		return WithPixelData(rows, cols, bitsAllocated, pd.GetFlatData(), codec), nil
	}

	return WithRawPixelData(pd), nil
}

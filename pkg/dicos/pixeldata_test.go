package dicos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPixelData_GetFrame(t *testing.T) {
	pd := &PixelData{
		IsEncapsulated: false,
		Frames: []Frame{
			{Data: []uint16{1, 2, 3, 4}},
			{Data: []uint16{5, 6, 7, 8}},
			{Data: []uint16{9, 10, 11, 12}},
		},
	}

	// Valid indices
	frame, err := pd.GetFrame(0)
	require.NoError(t, err)
	assert.Equal(t, []uint16{1, 2, 3, 4}, frame.Data)

	frame, err = pd.GetFrame(2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{9, 10, 11, 12}, frame.Data)

	// Out of bounds
	_, err = pd.GetFrame(-1)
	assert.Error(t, err)

	_, err = pd.GetFrame(3)
	assert.Error(t, err)
}

func TestPixelData_NumFrames(t *testing.T) {
	pd := &PixelData{
		Frames: []Frame{
			{Data: []uint16{1, 2}},
			{Data: []uint16{3, 4}},
		},
	}

	assert.Equal(t, 2, pd.NumFrames())

	emptyPD := &PixelData{}
	assert.Equal(t, 0, emptyPD.NumFrames())
}

func TestPixelData_IsCompressed(t *testing.T) {
	uncompressed := &PixelData{IsEncapsulated: false}
	assert.False(t, uncompressed.IsCompressed())

	compressed := &PixelData{IsEncapsulated: true}
	assert.True(t, compressed.IsCompressed())
}

func TestPixelData_HasFrames(t *testing.T) {
	withFrames := &PixelData{
		Frames: []Frame{{Data: []uint16{1, 2}}},
	}
	assert.True(t, withFrames.HasFrames())

	noFrames := &PixelData{}
	assert.False(t, noFrames.HasFrames())
}

func TestPixelData_FrameSize(t *testing.T) {
	// Uncompressed
	pd := &PixelData{
		IsEncapsulated: false,
		Frames: []Frame{
			{Data: []uint16{1, 2, 3, 4, 5, 6}},
		},
	}
	assert.Equal(t, 6, pd.FrameSize())

	// Compressed - size unknown
	compressed := &PixelData{
		IsEncapsulated: true,
		Frames: []Frame{
			{CompressedData: []byte{1, 2, 3, 4}},
		},
	}
	assert.Equal(t, 0, compressed.FrameSize())

	// No frames
	empty := &PixelData{}
	assert.Equal(t, 0, empty.FrameSize())
}

func TestPixelData_TotalPixels(t *testing.T) {
	// Uncompressed with multiple frames
	pd := &PixelData{
		IsEncapsulated: false,
		Frames: []Frame{
			{Data: []uint16{1, 2, 3}},
			{Data: []uint16{4, 5}},
			{Data: []uint16{6, 7, 8, 9}},
		},
	}
	assert.Equal(t, 9, pd.TotalPixels())

	// Compressed - unknown until decompression
	compressed := &PixelData{
		IsEncapsulated: true,
		Frames:         []Frame{{CompressedData: []byte{1, 2, 3}}},
	}
	assert.Equal(t, 0, compressed.TotalPixels())

	// Empty
	empty := &PixelData{}
	assert.Equal(t, 0, empty.TotalPixels())
}

func TestPixelData_GetFlatData(t *testing.T) {
	// Uncompressed multi-frame
	pd := &PixelData{
		IsEncapsulated: false,
		Frames: []Frame{
			{Data: []uint16{1, 2, 3}},
			{Data: []uint16{4, 5, 6}},
		},
	}

	flat := pd.GetFlatData()
	assert.Equal(t, []uint16{1, 2, 3, 4, 5, 6}, flat)

	// Compressed - returns nil
	compressed := &PixelData{
		IsEncapsulated: true,
		Frames:         []Frame{{CompressedData: []byte{1, 2, 3}}},
	}
	assert.Nil(t, compressed.GetFlatData())
}

func TestPixelData_Integration(t *testing.T) {
	// Create a multi-frame pixel data
	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, 3),
	}

	// Fill with test data
	for i := 0; i < 3; i++ {
		data := make([]uint16, 4)
		for j := 0; j < 4; j++ {
			data[j] = uint16(i*4 + j)
		}
		pd.Frames[i] = Frame{Data: data}
	}

	// Test methods
	assert.Equal(t, 3, pd.NumFrames())
	assert.False(t, pd.IsCompressed())
	assert.True(t, pd.HasFrames())
	assert.Equal(t, 4, pd.FrameSize())
	assert.Equal(t, 12, pd.TotalPixels())

	// Get individual frames
	for i := 0; i < 3; i++ {
		frame, err := pd.GetFrame(i)
		require.NoError(t, err)
		assert.Len(t, frame.Data, 4)
		assert.Equal(t, uint16(i*4), frame.Data[0])
	}

	// Get flat data
	flat := pd.GetFlatData()
	require.Len(t, flat, 12)
	assert.Equal(t, uint16(0), flat[0])
	assert.Equal(t, uint16(11), flat[11])
}

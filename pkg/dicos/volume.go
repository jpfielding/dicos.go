package dicos

import "fmt"

// Volume represents a 3D volume of pixel data
type Volume struct {
	// Dimensions
	Width  int // X
	Height int // Y
	Depth  int // Z (number of slices)

	// Voxel spacing in mm
	SpacingX float64
	SpacingY float64
	SpacingZ float64

	// Image position of first slice
	OriginX float64
	OriginY float64
	OriginZ float64

	// Pixel data (row-major order, slice-by-slice)
	Data []uint16
}

// NewVolume creates a new Volume with the specified dimensions
func NewVolume(width, height, depth int) *Volume {
	return &Volume{
		Width:    width,
		Height:   height,
		Depth:    depth,
		SpacingX: 1.0,
		SpacingY: 1.0,
		SpacingZ: 1.0,
		Data:     make([]uint16, width*height*depth),
	}
}

// Get returns the voxel value at (x, y, z)
func (v *Volume) Get(x, y, z int) uint16 {
	if x < 0 || x >= v.Width || y < 0 || y >= v.Height || z < 0 || z >= v.Depth {
		return 0
	}
	idx := z*v.Width*v.Height + y*v.Width + x
	return v.Data[idx]
}

// Set sets the voxel value at (x, y, z)
func (v *Volume) Set(x, y, z int, val uint16) {
	if x < 0 || x >= v.Width || y < 0 || y >= v.Height || z < 0 || z >= v.Depth {
		return
	}
	idx := z*v.Width*v.Height + y*v.Width + x
	v.Data[idx] = val
}

// Slice returns a 2D slice from the volume
// Orientation: 0=Axial (XY at Z), 1=Coronal (XZ at Y), 2=Sagittal (YZ at X)
func (v *Volume) Slice(orientation int, index int) []uint16 {
	switch orientation {
	case 0: // Axial (XY plane at Z=index)
		if index < 0 || index >= v.Depth {
			return nil
		}
		slice := make([]uint16, v.Width*v.Height)
		start := index * v.Width * v.Height
		copy(slice, v.Data[start:start+v.Width*v.Height])
		return slice

	case 1: // Coronal (XZ plane at Y=index)
		if index < 0 || index >= v.Height {
			return nil
		}
		slice := make([]uint16, v.Width*v.Depth)
		for z := 0; z < v.Depth; z++ {
			for x := 0; x < v.Width; x++ {
				slice[z*v.Width+x] = v.Get(x, index, z)
			}
		}
		return slice

	case 2: // Sagittal (YZ plane at X=index)
		if index < 0 || index >= v.Width {
			return nil
		}
		slice := make([]uint16, v.Height*v.Depth)
		for z := 0; z < v.Depth; z++ {
			for y := 0; y < v.Height; y++ {
				slice[z*v.Height+y] = v.Get(index, y, z)
			}
		}
		return slice
	}
	return nil
}

// MinMax returns the minimum and maximum voxel values
func (v *Volume) MinMax() (min, max uint16) {
	if len(v.Data) == 0 {
		return 0, 0
	}
	min, max = v.Data[0], v.Data[0]
	for _, val := range v.Data {
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}
	return
}

// VolumeFromDataset creates a Volume from a Dataset's native pixel data.
// The depth is derived from the samples present, which must fill a whole
// number of Rows x Columns frames. Encapsulated data needs DecodeVolume.
func VolumeFromDataset(ds *Dataset) (*Volume, error) {
	rows := ds.Rows()
	cols := ds.Columns()
	if rows <= 0 || cols <= 0 {
		return nil, fmt.Errorf("%w: dimensions %dx%d", ErrInvalidValue, cols, rows)
	}
	pd, err := ds.GetPixelData()
	if err != nil {
		return nil, err
	}
	if pd.IsEncapsulated() {
		return nil, fmt.Errorf("encapsulated pixel data requires decoding - use DecodeVolume")
	}
	return nativeVolume(rows, cols, pd)
}

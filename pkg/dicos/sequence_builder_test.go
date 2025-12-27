package dicos

import (
	"testing"

	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSequenceBuilder_Basic(t *testing.T) {
	// Create a sequence builder
	builder := NewSequenceBuilder(tag.ReferencedImageSequence)

	// Add items
	builder.AddItem(
		WithElement(tag.ReferencedSOPClassUID, "1.2.840.10008.5.1.4.1.1.2"),
		WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.4.5"),
	).AddItem(
		WithElement(tag.ReferencedSOPClassUID, "1.2.840.10008.5.1.4.1.1.2"),
		WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.4.6"),
	)

	assert.Equal(t, 2, builder.Count())
	assert.False(t, builder.HasErrors())

	// Build into a dataset
	opt, err := builder.Build()
	require.NoError(t, err)

	ds, err := NewDataset(
		WithElement(tag.PatientID, "PAT-001"),
		opt,
	)
	require.NoError(t, err)

	// Verify sequence exists
	items := GetSequenceItems(ds, tag.ReferencedImageSequence)
	require.NotNil(t, items)
	assert.Len(t, items, 2)

	// Check first item
	elem, ok := items[0].FindElement(tag.ReferencedSOPInstanceUID.Group, tag.ReferencedSOPInstanceUID.Element)
	require.True(t, ok)
	uid, _ := elem.GetString()
	assert.Equal(t, "1.2.3.4.5", uid)
}

func TestSequenceBuilder_TDRExample(t *testing.T) {
	// Build a sequence with multiple items (simulating TDR PTOs)
	// Using standard DICOM tags since PTO-specific tags may not be defined yet
	ptoTag := Tag{Group: 0x0040, Element: 0x0100} // Referenced SOP Sequence

	builder := NewSequenceBuilder(ptoTag)

	// Add multiple items
	for i := 1; i <= 3; i++ {
		builder.AddItem(
			WithElement(tag.SeriesNumber, uint16(i)),
			WithElement(tag.SeriesDescription, "Item description"),
		)
	}

	assert.Equal(t, 3, builder.Count())
	assert.False(t, builder.HasErrors())

	// Build dataset
	ds, err := builder.BuildDataset()
	require.NoError(t, err)

	// Verify
	items := GetSequenceItems(ds, ptoTag)
	assert.Len(t, items, 3)
}

func TestSequenceBuilder_Manipulation(t *testing.T) {
	builder := NewSequenceBuilder(tag.ReferencedImageSequence)

	// Add items
	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.1"))
	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.2"))
	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.3"))

	assert.Equal(t, 3, builder.Count())

	// Remove middle item
	builder.RemoveItem(1)
	assert.Equal(t, 2, builder.Count())

	// Get item
	item := builder.GetItem(0)
	require.NotNil(t, item)

	// Modify item directly
	item.Elements[tag.ReferencedSOPClassUID] = &Element{
		Tag:   tag.ReferencedSOPClassUID,
		VR:    "UI",
		Value: "1.2.840.10008.5.1.4.1.1.2",
	}

	// Replace item
	newItem, _ := NewDataset(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.9"))
	builder.ReplaceItem(1, newItem)

	// Verify
	items := builder.GetItems()
	assert.Len(t, items, 2)
}

func TestSequenceBuilder_Clear(t *testing.T) {
	builder := NewSequenceBuilder(tag.ReferencedImageSequence)

	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.1"))
	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.2"))
	assert.Equal(t, 2, builder.Count())

	builder.Clear()
	assert.Equal(t, 0, builder.Count())

	// Can still add after clear
	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.3"))
	assert.Equal(t, 1, builder.Count())
}

func TestSequenceBuilder_AddDataset(t *testing.T) {
	builder := NewSequenceBuilder(tag.ReferencedImageSequence)

	// Add pre-built datasets
	ds1, _ := NewDataset(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.1"))
	ds2, _ := NewDataset(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.2"))

	builder.AddDataset(ds1).AddDataset(ds2)

	assert.Equal(t, 2, builder.Count())
	assert.False(t, builder.HasErrors())

	// Build and verify
	result, err := builder.BuildDataset()
	require.NoError(t, err)

	items := GetSequenceItems(result, tag.ReferencedImageSequence)
	assert.Len(t, items, 2)
}

func TestSequenceBuilder_ErrorHandling(t *testing.T) {
	builder := NewSequenceBuilder(tag.ReferencedImageSequence)

	// Add a valid item
	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.1"))

	// Note: It's hard to force an error in NewDataset with simple options
	// This test verifies the error accumulation mechanism exists
	assert.False(t, builder.HasErrors())
	assert.Empty(t, builder.Errors())

	// Build should succeed
	_, err := builder.Build()
	assert.NoError(t, err)
}

func TestSequenceBuilder_OutOfBounds(t *testing.T) {
	builder := NewSequenceBuilder(tag.ReferencedImageSequence)

	builder.AddItem(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.1"))

	// Operations on out-of-bounds indices should be no-ops
	builder.RemoveItem(10)
	assert.Equal(t, 1, builder.Count())

	builder.RemoveItem(-1)
	assert.Equal(t, 1, builder.Count())

	assert.Nil(t, builder.GetItem(10))
	assert.Nil(t, builder.GetItem(-1))

	newItem, _ := NewDataset(WithElement(tag.ReferencedSOPInstanceUID, "1.2.3.9"))
	builder.ReplaceItem(10, newItem)
	assert.Equal(t, 1, builder.Count())
}

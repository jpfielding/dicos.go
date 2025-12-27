package dicos

import "fmt"

// SequenceBuilder provides a fluent API for constructing DICOM sequences.
//
// This builder simplifies creating complex sequences like TDR PTOs (Potential Threat Objects),
// Referenced Image Sequences, and other multi-item sequences.
//
// Error Handling:
//
// The builder accumulates errors from AddItem() calls. These errors are returned
// when you call Build() or BuildDataset(). This allows fluent chaining while
// maintaining explicit error handling.
//
// Example - Building a Referenced Image Sequence:
//
//	builder := dicos.NewSequenceBuilder(tag.ReferencedImageSequence)
//	for _, instance := range imageInstances {
//		builder.AddItem(
//			dicos.WithElement(tag.ReferencedSOPClassUID, sopClass),
//			dicos.WithElement(tag.ReferencedSOPInstanceUID, instance),
//		)
//	}
//
//	// Check for errors before using
//	opt, err := builder.Build()
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	ds, err := dicos.NewDataset(opt)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Example - Pre-build datasets for better error handling:
//
//	builder := dicos.NewSequenceBuilder(tag.ReferencedImageSequence)
//	for _, instance := range imageInstances {
//		item, err := dicos.NewDataset(
//			dicos.WithElement(tag.ReferencedSOPClassUID, sopClass),
//			dicos.WithElement(tag.ReferencedSOPInstanceUID, instance),
//		)
//		if err != nil {
//			return err  // Handle error immediately
//		}
//		builder.AddDataset(item)
//	}
//	ds, _ := builder.BuildDataset()
type SequenceBuilder struct {
	tag   Tag
	items []*Dataset
	errs  []error
}

// NewSequenceBuilder creates a new sequence builder for the specified tag.
//
// The tag should be a sequence-type DICOM element (VR=SQ).
//
// Example:
//
//	builder := dicos.NewSequenceBuilder(tag.ReferencedImageSequence)
func NewSequenceBuilder(t Tag) *SequenceBuilder {
	return &SequenceBuilder{
		tag:   t,
		items: make([]*Dataset, 0),
		errs:  make([]error, 0),
	}
}

// AddItem adds a sequence item constructed from the given options.
//
// Each call to AddItem creates a new dataset item in the sequence.
// If an error occurs, it is accumulated and will be returned from Build().
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.AddItem(
//		dicos.WithElement(tag.ReferencedSOPClassUID, sopClass),
//		dicos.WithElement(tag.ReferencedSOPInstanceUID, instance1),
//	).AddItem(
//		dicos.WithElement(tag.ReferencedSOPClassUID, sopClass),
//		dicos.WithElement(tag.ReferencedSOPInstanceUID, instance2),
//	)
//
//	opt, err := builder.Build()  // Check for accumulated errors
//	if err != nil {
//		log.Fatal(err)
//	}
func (sb *SequenceBuilder) AddItem(opts ...Option) *SequenceBuilder {
	item, err := NewDataset(opts...)
	if err != nil {
		sb.errs = append(sb.errs, fmt.Errorf("item %d: %w", len(sb.items), err))
		return sb
	}
	sb.items = append(sb.items, item)
	return sb
}

// AddDataset adds an already-constructed dataset as a sequence item.
//
// Use this when you have a pre-built dataset to add to the sequence.
//
// Returns the builder for method chaining.
//
// Example:
//
//	item, _ := dicos.NewDataset(...)
//	builder.AddDataset(item)
func (sb *SequenceBuilder) AddDataset(ds *Dataset) *SequenceBuilder {
	if ds != nil {
		sb.items = append(sb.items, ds)
	}
	return sb
}

// Count returns the number of items currently in the sequence.
func (sb *SequenceBuilder) Count() int {
	return len(sb.items)
}

// Clear removes all items and errors from the sequence.
//
// Returns the builder for method chaining.
func (sb *SequenceBuilder) Clear() *SequenceBuilder {
	sb.items = sb.items[:0]
	sb.errs = sb.errs[:0]
	return sb
}

// HasErrors returns true if any errors were accumulated during building.
func (sb *SequenceBuilder) HasErrors() bool {
	return len(sb.errs) > 0
}

// Errors returns all accumulated errors.
func (sb *SequenceBuilder) Errors() []error {
	return sb.errs
}

// Build returns an Option that adds the sequence to a dataset.
//
// Returns an error if any AddItem() calls failed during building.
//
// Example:
//
//	opt, err := builder.Build()
//	if err != nil {
//		return fmt.Errorf("building sequence: %w", err)
//	}
//
//	ds, err := dicos.NewDataset(
//		dicos.WithElement(tag.PatientID, "PAT-001"),
//		opt,
//	)
func (sb *SequenceBuilder) Build() (Option, error) {
	if len(sb.errs) > 0 {
		return nil, fmt.Errorf("sequence builder has %d error(s): %v", len(sb.errs), sb.errs)
	}
	return WithSequence(sb.tag, sb.items...), nil
}

// BuildDataset creates a standalone dataset containing only this sequence.
//
// Returns an error if any AddItem() calls failed during building.
//
// Example:
//
//	sequenceDS, err := builder.BuildDataset()
//	if err != nil {
//		return err
//	}
//	items := dicos.GetSequenceItems(sequenceDS, tag.ReferencedImageSequence)
func (sb *SequenceBuilder) BuildDataset() (*Dataset, error) {
	opt, err := sb.Build()
	if err != nil {
		return nil, err
	}
	return NewDataset(opt)
}

// GetItems returns a copy of the current sequence items.
//
// This allows inspection of the sequence before building.
func (sb *SequenceBuilder) GetItems() []*Dataset {
	// Return a copy to prevent external modification
	items := make([]*Dataset, len(sb.items))
	copy(items, sb.items)
	return items
}

// RemoveItem removes the item at the specified index.
//
// Returns the builder for method chaining.
// Does nothing if index is out of bounds.
//
// Example:
//
//	builder.RemoveItem(0) // Remove first item
func (sb *SequenceBuilder) RemoveItem(index int) *SequenceBuilder {
	if index >= 0 && index < len(sb.items) {
		sb.items = append(sb.items[:index], sb.items[index+1:]...)
	}
	return sb
}

// GetItem returns the item at the specified index, or nil if out of bounds.
//
// This allows inspection and modification of individual items before building.
//
// Example:
//
//	item := builder.GetItem(0)
//	if item != nil {
//		item.Elements[tag.ThreatProbability] = &dicos.Element{
//			Tag:   tag.ThreatProbability,
//			VR:    "FL",
//			Value: float32(0.99),
//		}
//	}
func (sb *SequenceBuilder) GetItem(index int) *Dataset {
	if index >= 0 && index < len(sb.items) {
		return sb.items[index]
	}
	return nil
}

// ReplaceItem replaces the item at the specified index with a new item.
//
// Returns the builder for method chaining.
// Does nothing if index is out of bounds or ds is nil.
//
// Example:
//
//	newItem, _ := dicos.NewDataset(...)
//	builder.ReplaceItem(0, newItem)
func (sb *SequenceBuilder) ReplaceItem(index int, ds *Dataset) *SequenceBuilder {
	if ds != nil && index >= 0 && index < len(sb.items) {
		sb.items[index] = ds
	}
	return sb
}

package pisa

import (
	"github.com/pkg/errors"

	ctypes "github.com/sarvalabs/moichain/jug/types"
)

type (
	StorageLayout  = FieldTable
	StorageBuilder = Routine
)

// NewStorageLayout generates a new StorageLayout from manifest.Map object.
// Each map value string must be parseable with ParseTypeField.
func NewStorageLayout(schema map[uint8]string) (StorageLayout, error) {
	// Create a new FieldSet from the map set
	symbols, err := NewFieldTable(schema)
	if err != nil {
		return StorageLayout{}, errors.Wrap(err, "invalid storage layout")
	}

	return symbols, nil
}

// StorageTable represents the collection of storage drivers and layout for the Logic.
// This is only required if the logic is stateful.
type StorageTable struct {
	layout  *StorageLayout
	builder *StorageBuilder

	caller ctypes.Storage
	callee ctypes.Storage
}

package runtime

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/register"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

type (
	StorageLayout  = register.FieldTable
	StorageBuilder = Routine
)

// StorageTable represents the collection of storage drivers and layout for the Logic.
// This is only required if the logic is stateful.
type StorageTable struct {
	Layout  *StorageLayout
	Builder *StorageBuilder

	Caller ctypes.Storage
	Callee ctypes.Storage
}

func (storage StorageTable) EjectElements() []*types.LogicElement {
	// Polorize the storage layout
	layout, _ := polo.Polorize(storage.Layout)
	// Create a LogicElement for the storage layout and append it
	layoutElement := &types.LogicElement{Kind: ElementCodeStorage, Index: 0, Data: layout}

	// Polorize the storage builder
	builder, _ := polo.Polorize(storage.Builder)
	// Create a LogicElement for the storage builder and append it
	builderElement := &types.LogicElement{Kind: ElementCodeStorage, Index: 1, Data: builder}

	return []*types.LogicElement{layoutElement, builderElement}
}

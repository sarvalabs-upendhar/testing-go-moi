package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/types"
)

// LogicObject is a generic container for representing an executable logic.
// It contains fields for representing the kind and metadata of the logic
// along with references to callable endpoints and all its logic elements.
// Implements the Logic interface defined in jug/types
type LogicObject struct {
	// Represents the Logic ID
	ID types.LogicID
	// Represents the Logic Engine
	Engine types.LogicEngine
	// Represents the CID of the Logic Manifest
	Manifest types.Hash

	Sealed   bool
	Stateful bool

	// Represents the collection of all LogicElement objects
	Elements types.LogicElementSet
	// Represents mapping of string names to LogicCallsite pointers
	Callsites map[string]types.LogicCallsite
}

func (lo *LogicObject) Bytes() ([]byte, error) {
	data, err := polo.Polorize(lo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize logic object")
	}

	return data, err
}

func (lo *LogicObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(lo, bytes); err != nil {
		return errors.New("failed to depolorize logic object")
	}

	return nil
}

// NewLogicObject generates a new LogicObject for a given LogicID and LogicDescriptor
func NewLogicObject(id types.LogicID, descriptor types.LogicDescriptor) *LogicObject {
	elements := make(types.LogicElementSet)
	elements.Insert(descriptor.Elements...)

	return &LogicObject{
		ID:       id,
		Engine:   descriptor.Engine,
		Manifest: descriptor.Manifest,

		Sealed:   false,
		Stateful: descriptor.Stateful,

		Elements:  elements,
		Callsites: descriptor.Callsites,
	}
}

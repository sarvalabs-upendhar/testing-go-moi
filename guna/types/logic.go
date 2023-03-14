package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

// LogicObject is a generic container for representing an executable logic. It contains fields
// for representing the kind and metadata of the logic along with references to callable endpoints
// and all its logic elements. Implements the Logic interface defined in jug/types.
// Its fields are exported for to make it serializable.
type LogicObject struct {
	// Represents the Logic ID
	ID types.LogicID
	// Represents the Logic Engine
	EngineKind types.LogicEngine
	// Represents the CID of the Logic Manifest
	ManifestHash types.Hash

	Sealed bool

	PersistentStateful  bool
	EphemeralStateful   bool
	InteractionsAllowed bool

	// Represents the collection of all LogicElement objects
	Elements types.LogicElementSet
	// Represents mapping of string names to LogicCallsite pointers
	Callsites map[string]types.LogicCallsite
}

// NewLogicObject generates a new LogicObject for a given LogicID, LogicDescriptor and Storage Namespace key
func NewLogicObject(id types.LogicID, descriptor *types.LogicDescriptor) *LogicObject {
	elements := make(types.LogicElementSet)
	elements.Insert(descriptor.Elements...)

	return &LogicObject{
		ID:           id,
		EngineKind:   descriptor.Engine,
		ManifestHash: descriptor.Manifest,

		Sealed: false,

		PersistentStateful:  descriptor.PersistentState,
		EphemeralStateful:   descriptor.EphemeralState,
		InteractionsAllowed: descriptor.AllowsInteractions,

		Elements:  elements,
		Callsites: descriptor.Callsites,
	}
}

func (logic LogicObject) IsSealed() bool           { return logic.Sealed }
func (logic LogicObject) HasPersistentState() bool { return logic.PersistentStateful }
func (logic LogicObject) HasEphemeralState() bool  { return logic.EphemeralStateful }
func (logic LogicObject) AllowsInteractions() bool { return logic.InteractionsAllowed }

func (logic LogicObject) LogicID() types.LogicID    { return logic.ID }
func (logic LogicObject) Engine() types.LogicEngine { return logic.EngineKind }
func (logic LogicObject) Manifest() types.Hash      { return logic.ManifestHash }

func (logic LogicObject) GetCallsite(name string) (types.LogicCallsite, bool) {
	callsite, ok := logic.Callsites[name]
	// Return the callsite and if it was found in the LogicObject
	return callsite, ok
}

func (logic LogicObject) GetLogicElement(kind string, index uint64) (*types.LogicElement, bool) {
	return logic.Elements.Fetch(kind, index)
}

func (logic *LogicObject) Bytes() ([]byte, error) {
	data, err := polo.Polorize(logic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize logic object")
	}

	return data, err
}

func (logic *LogicObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(logic, bytes); err != nil {
		return errors.New("failed to depolorize logic object")
	}

	return nil
}

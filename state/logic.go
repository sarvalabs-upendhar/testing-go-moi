package state

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

// LogicObject is a generic container for representing an executable logic. It contains fields
// for representing the kind and metadata of the logic along with references to callable endpoints
// and all its logic elements. Implements the Logic interface defined in jug/types.
// Its fields are exported for to make it serializable.
type LogicObject struct {
	// Represents the Logic ID
	ID common.LogicID
	// Represents the Logic Engine
	EngineKind engineio.EngineKind
	// Represents the CID of the Logic Manifest
	ManifestHash common.Hash

	Sealed bool

	// Represents the usage of different type of context states by the logic
	StateMatrix engineio.ContextStateMatrix
	// Represents the dependency graph between logic elements
	Dependencies *engineio.DependencyGraph
	// Represents the collection of all LogicElement objects
	Elements map[engineio.ElementPtr]*engineio.LogicElement
	// Represents mapping of string names to LogicCallsite pointers
	Callsites map[string]*engineio.Callsite
	Classdefs map[string]*engineio.Classdef
}

// NewLogicObject generates a new LogicObject for a given LogicID, LogicDescriptor and Storage Namespace key
func NewLogicObject(address common.Address, descriptor *engineio.LogicDescriptor) *LogicObject {
	// Generate the LogicID from the payload
	logicID := common.NewLogicIDv0(
		descriptor.StateMatrix.Persistent(),
		descriptor.StateMatrix.Ephemeral(),
		descriptor.Interactive,
		false, 0, address,
	)

	return &LogicObject{
		ID:           logicID,
		EngineKind:   descriptor.Engine,
		ManifestHash: descriptor.ManifestHash,

		Sealed: false,

		StateMatrix:  descriptor.StateMatrix,
		Dependencies: descriptor.DepGraph,
		Elements:     descriptor.Elements,

		Callsites: descriptor.Callsites,
		Classdefs: descriptor.Classdefs,
	}
}

func (logic LogicObject) LogicID() common.LogicID { return logic.ID }

func (logic LogicObject) Engine() engineio.EngineKind { return logic.EngineKind }

func (logic LogicObject) Manifest() common.Hash { return logic.ManifestHash }

func (logic LogicObject) IsSealed() bool { return logic.Sealed }

func (logic LogicObject) IsAssetLogic() bool {
	logicIdentifier, err := logic.LogicID().Identifier()
	if err != nil {
		panic("failed to fetch logic identifier")
	}

	return logicIdentifier.AssetLogic()
}

func (logic LogicObject) AllowsInteractions() bool {
	logicIdentifier, err := logic.LogicID().Identifier()
	if err != nil {
		panic("failed to fetch logic identifier")
	}

	return logicIdentifier.Interactive()
}

func (logic LogicObject) PersistentState() (engineio.ElementPtr, bool) {
	ptr, exists := logic.StateMatrix[engineio.PersistentState]

	return ptr, exists
}

func (logic LogicObject) EphemeralState() (engineio.ElementPtr, bool) {
	ptr, exists := logic.StateMatrix[engineio.EphemeralState]

	return ptr, exists
}

func (logic LogicObject) GetCallsite(name string) (*engineio.Callsite, bool) {
	callsite, ok := logic.Callsites[name]

	return callsite, ok
}

func (logic LogicObject) GetClassdef(name string) (*engineio.Classdef, bool) {
	classdef, ok := logic.Classdefs[name]

	return classdef, ok
}

func (logic LogicObject) GetElementDeps(ptr engineio.ElementPtr) []engineio.ElementPtr {
	return logic.Dependencies.AllDependencies(ptr)
}

func (logic LogicObject) GetElement(index engineio.ElementPtr) (*engineio.LogicElement, bool) {
	element, ok := logic.Elements[index]

	return element, ok
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

func GetManifestHashFromRawLogicObject(raw []byte) (common.Hash, error) {
	depolorizer, err := polo.NewDepolorizer(raw)
	if err != nil {
		return common.NilHash, err
	}

	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return common.NilHash, nil
	} else if err != nil {
		return common.NilHash, err
	}

	// Skip the first field
	if _, err = depolorizer.DepolorizeAny(); err != nil {
		return common.NilHash, err
	}

	// Skip the second field
	if _, err = depolorizer.DepolorizeAny(); err != nil {
		return common.NilHash, err
	}

	var ManifestHash common.Hash
	if err = depolorizer.Depolorize(&ManifestHash); err != nil {
		return common.NilHash, err
	}

	return ManifestHash, nil
}

type LogicContextObject struct {
	state *Object
	logic common.LogicID
}

func NewLogicContextObject(logic common.LogicID, state *Object) *LogicContextObject {
	return &LogicContextObject{state: state, logic: logic}
}

func (ctx LogicContextObject) Address() common.Address {
	return ctx.state.Address()
}

func (ctx LogicContextObject) LogicID() common.LogicID {
	return ctx.logic
}

func (ctx LogicContextObject) GetStorageEntry(key []byte) ([]byte, bool) {
	data, err := ctx.state.GetStorageEntry(ctx.logic, key)

	return data, err == nil
}

func (ctx LogicContextObject) SetStorageEntry(key, val []byte) bool {
	err := ctx.state.SetStorageEntry(ctx.logic, key, val)

	return err == nil
}

type LogicStorageObject map[string][]byte

func (storage LogicStorageObject) Copy() LogicStorageObject {
	clone := make(LogicStorageObject)

	for key, value := range storage {
		v := make([]byte, len(value))

		copy(v, value)
		clone[key] = v
	}

	return clone
}

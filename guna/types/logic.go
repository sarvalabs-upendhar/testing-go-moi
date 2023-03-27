package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
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
	EngineKind engineio.EngineKind
	// Represents the CID of the Logic Manifest
	ManifestHash types.Hash

	Sealed     bool
	AssetLogic bool

	PersistentStateful  *uint64
	EphemeralStateful   *uint64
	InteractionsAllowed bool

	Dependencies *engineio.DependencyGraph
	// Represents the collection of all LogicElement objects
	Elements map[uint64]*engineio.LogicElement
	// Represents mapping of string names to LogicCallsite pointers
	Callsites map[string]*engineio.Callsite
}

// NewLogicObject generates a new LogicObject for a given LogicID, LogicDescriptor and Storage Namespace key
func NewLogicObject(address types.Address, descriptor *engineio.LogicDescriptor) *LogicObject {
	// Generate the LogicID from the payload
	logicID, _ := types.NewLogicIDv0(
		descriptor.PersistentState != nil,
		descriptor.EphemeralState != nil,
		descriptor.AllowsInteractions,
		false, 0, address,
	)

	return &LogicObject{
		ID:           logicID,
		EngineKind:   descriptor.Engine,
		ManifestHash: descriptor.Manifest,

		Sealed:     false,
		AssetLogic: false,

		PersistentStateful:  descriptor.PersistentState,
		EphemeralStateful:   descriptor.EphemeralState,
		InteractionsAllowed: descriptor.AllowsInteractions,

		Dependencies: descriptor.DepGraph,
		Elements:     descriptor.Elements,
		Callsites:    descriptor.Callsites,
	}
}

func (logic LogicObject) LogicID() types.LogicID { return logic.ID }

func (logic LogicObject) Engine() engineio.EngineKind { return logic.EngineKind }

func (logic LogicObject) Manifest() types.Hash { return logic.ManifestHash }

func (logic LogicObject) IsSealed() bool { return logic.Sealed }

func (logic LogicObject) IsAssetLogic() bool { return logic.AssetLogic }

func (logic LogicObject) AllowsInteractions() bool { return logic.InteractionsAllowed }

func (logic LogicObject) PersistentState() (uint64, bool) {
	return *logic.PersistentStateful, logic.PersistentStateful != nil
}

func (logic LogicObject) EphemeralState() (uint64, bool) {
	return *logic.EphemeralStateful, logic.EphemeralStateful != nil
}

func (logic LogicObject) GetCallsite(name string) (*engineio.Callsite, bool) {
	callsite, ok := logic.Callsites[name]

	return callsite, ok
}

func (logic LogicObject) GetElementDeps(ptr uint64) []uint64 {
	return logic.Dependencies.AllDependencies(ptr)
}

func (logic LogicObject) GetElement(index uint64) (*engineio.LogicElement, bool) {
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

type logicStateObject interface {
	Address() types.Address
	GetStorageEntry(types.LogicID, []byte) ([]byte, error)
	SetStorageEntry(types.LogicID, []byte, []byte) error
}

type LogicContextObject struct {
	state logicStateObject
	logic types.LogicID
}

func NewLogicContextObject(logic types.LogicID, state logicStateObject) *LogicContextObject {
	return &LogicContextObject{state: state, logic: logic}
}

func (ctx LogicContextObject) Address() types.Address {
	return ctx.state.Address()
}

func (ctx LogicContextObject) LogicID() types.LogicID {
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

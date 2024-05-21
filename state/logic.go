package state

import (
	"github.com/manishmeganathan/depgraph"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
)

// LogicObject is a generic container for representing an executable logic. It contains fields
// for representing the kind and metadata of the logic along with references to callable endpoints
// and all its logic elements. Implements the Logic interface defined in jug/types.
// Its fields are exported for to make it serializable.
type LogicObject struct {
	// Represents the Logic ID
	ID identifiers.LogicID
	// Represents the Logic Engine
	EngineKind engineio.EngineKind
	// Represents the hash of the Logic Manifest
	Manifest common.Hash

	Sealed     bool
	Persistent *uint64
	Ephemeral  *uint64

	// Represents the dependency driver for managing logic elements relationships
	Dependencies *depgraph.DependencyGraph
	// Represents the collection of all engineio.LogicElement objects
	Elements map[engineio.ElementPtr]*engineio.LogicElement
	// Represents mapping of string names to engineio.Callsite
	Callsites map[string]engineio.Callsite
	// Represents mapping of string names to engineio.Classdef
	Classdefs map[string]engineio.Classdef

	// Represents mapping of string names to engineio.Eventdef
	Eventdefs map[string]engineio.Eventdef
}

// NewLogicObject generates a new LogicObject for a given LogicID, LogicDescriptor and Storage Namespace key
func NewLogicObject(address identifiers.Address, descriptor engineio.LogicDescriptor) *LogicObject {
	// Generate the LogicID from the payload
	logicID := identifiers.NewLogicIDv0(
		descriptor.Persistent != nil,
		descriptor.Ephemeral != nil,
		descriptor.Interactable, false,
		0, address,
	)

	return &LogicObject{
		ID:         logicID,
		EngineKind: descriptor.Engine,
		Manifest:   descriptor.ManifestHash,

		Sealed:     false,
		Persistent: descriptor.Persistent,
		Ephemeral:  descriptor.Ephemeral,

		Dependencies: descriptor.Depgraph,
		Elements:     descriptor.Elements,

		Callsites: descriptor.Callsites,
		Classdefs: descriptor.Classdefs,

		Eventdefs: descriptor.Eventdefs,
	}
}

func (logic LogicObject) LogicID() identifiers.LogicID { return logic.ID }
func (logic LogicObject) Engine() engineio.EngineKind  { return logic.EngineKind }
func (logic LogicObject) ManifestHash() [32]byte       { return logic.Manifest }
func (logic LogicObject) IsSealed() bool               { return logic.Sealed }

func (logic LogicObject) IsInteractable() bool {
	logicIdentifier, err := logic.ID.Identifier()
	if err != nil {
		panic("failed to fetch logic identifier")
	}

	return logicIdentifier.HasInteractableSites()
}

func (logic LogicObject) PersistentState() (engineio.ElementPtr, bool) {
	if logic.Persistent == nil {
		return 0, false
	}

	return *logic.Persistent, true
}

func (logic LogicObject) EphemeralState() (engineio.ElementPtr, bool) {
	if logic.Ephemeral == nil {
		return 0, false
	}

	return *logic.Ephemeral, true
}

func (logic LogicObject) GetCallsite(name string) (engineio.Callsite, bool) {
	callsite, ok := logic.Callsites[name]

	return callsite, ok
}

func (logic LogicObject) GetClassdef(name string) (engineio.Classdef, bool) {
	classdef, ok := logic.Classdefs[name]

	return classdef, ok
}

func (logic LogicObject) GetEventdef(name string) (engineio.Eventdef, bool) {
	eventdef, ok := logic.Eventdefs[name]

	return eventdef, ok
}

func (logic LogicObject) GetElementDeps(ptr engineio.ElementPtr) []engineio.ElementPtr {
	return logic.Dependencies.Dependencies(ptr)
}

func (logic LogicObject) GetElement(ptr engineio.ElementPtr) (*engineio.LogicElement, bool) {
	element, ok := logic.Elements[ptr]

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
		return errors.Wrap(err, "failed to depolorize logic object")
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
	logic identifiers.LogicID
}

func NewLogicContextObject(logic identifiers.LogicID, state *Object) *LogicContextObject {
	return &LogicContextObject{state: state, logic: logic}
}

func (ctx LogicContextObject) Address() identifiers.Address {
	return ctx.state.Address()
}

func (ctx LogicContextObject) LogicID() identifiers.LogicID {
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

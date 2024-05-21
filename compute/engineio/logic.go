package engineio

import (
	"testing"

	"github.com/manishmeganathan/depgraph"

	"github.com/sarvalabs/go-moi-identifiers"
)

// LogicDescriptor is a container type returned by the CompileManifest method of EngineRuntime.
// It allows different engine runtime to have a unified output standard when compiling manifests.
//
// It serves as a source of information from which an object that implements the LogicDriver
// interface can be generated. It contains within it the manifest's runtime engine, raw contents
// and hash apart from entries for the callsites and classdefs.
type LogicDescriptor struct {
	Engine EngineKind

	ManifestData []byte
	ManifestHash [32]byte
	Interactable bool

	Persistent *uint64
	Ephemeral  *uint64

	Elements map[ElementPtr]*LogicElement
	Depgraph *depgraph.DependencyGraph

	Callsites map[string]Callsite
	Classdefs map[string]Classdef

	Eventdefs map[string]Eventdef
}

// LogicDriver is an interface for logic that can be executed within an Engine.
// Every logic is uniquely identified with a LogicID and serves as a source of code, elements and metadata
// that the Engine and its EngineRuntime can use during execution of a specific callsite within the LogicDriver.
//
// A LogicDriver can usually be constructed with the information available within a LogicDescriptor.
// For example, go-moi uses the LogicDescriptor as the source for generating a state.LogicObject
// which implements the LogicDriver interface and is the canonical object for all logic content.
//
// The LogicDriver contains within it one or more Callsite entries that can be called using
// the Call method on Engine. It also contains descriptions for various logical elements,
// addressed by their ElementPtr identifiers and Classdef entries for custom class definitions.
type LogicDriver interface {
	// LogicID returns the unique Logic ID of the LogicDriver
	LogicID() identifiers.LogicID
	// Engine returns the EngineKind of the LogicDriver
	Engine() EngineKind
	// ManifestHash returns the 256-bit digest hash of the logic's Manifest
	ManifestHash() [32]byte

	// IsSealed returns whether the state of the LogicDriver has been sealed
	IsSealed() bool
	// IsInteractable returns whether the LogicDriver supports Interactable Callsites
	IsInteractable() bool

	// PersistentState returns the pointer to the persistent state element
	// with a confirmation that the LogicDriver defines a PersistentState
	PersistentState() (ElementPtr, bool)
	// EphemeralState returns the pointer to the ephemeral state element
	// with a confirmation that the LogicDriver defines a EphemeralState
	EphemeralState() (ElementPtr, bool)

	// GetCallsite returns Callsite for a given string name with confirmation of its existence.
	GetCallsite(string) (Callsite, bool)
	// GetClassdef returns class Datatype for a given string name with confirmation of its existence.
	GetClassdef(string) (Classdef, bool)
	// GetEventdef returns event Datatype for a given string name with confirmation of its existence.
	GetEventdef(string) (Eventdef, bool)

	// GetElement returns the LogicElement for a given element pointer with confirmation of its existence.
	GetElement(ElementPtr) (*LogicElement, bool)
	// GetElementDeps returns the aggregated dependencies of an element pointer.
	// The aggregation includes all sub-dependencies recursively.
	GetElementDeps(ElementPtr) []ElementPtr
}

func NewDebugLogicDriver(t *testing.T, address identifiers.Address, descriptor LogicDescriptor) LogicDriver {
	t.Helper()

	// Generate the LogicID from the payload
	logicID := identifiers.NewLogicIDv0(
		descriptor.Persistent != nil,
		descriptor.Ephemeral != nil,
		descriptor.Interactable, false,
		0, address,
	)

	return debugLogicDriver{
		id:       logicID,
		kind:     descriptor.Engine,
		manifest: descriptor.ManifestHash,

		sealed:     false,
		persistent: descriptor.Persistent,
		ephemeral:  descriptor.Ephemeral,

		dependencies: descriptor.Depgraph,
		elements:     descriptor.Elements,

		callsites: descriptor.Callsites,
		classdefs: descriptor.Classdefs,

		eventdefs: descriptor.Eventdefs,
	}
}

type debugLogicDriver struct {
	id       identifiers.LogicID
	kind     EngineKind
	manifest [32]byte

	sealed     bool
	persistent *uint64
	ephemeral  *uint64

	elements     map[ElementPtr]*LogicElement
	dependencies *depgraph.DependencyGraph

	callsites map[string]Callsite
	classdefs map[string]Classdef

	eventdefs map[string]Eventdef
}

func (logic debugLogicDriver) LogicID() identifiers.LogicID { return logic.id }
func (logic debugLogicDriver) Engine() EngineKind           { return logic.kind }
func (logic debugLogicDriver) ManifestHash() [32]byte       { return logic.manifest }
func (logic debugLogicDriver) IsSealed() bool               { return logic.sealed }

func (logic debugLogicDriver) IsInteractable() bool {
	identifier, err := logic.id.Identifier()
	if err != nil {
		panic("failed to fetch logic identifier")
	}

	return identifier.HasInteractableSites()
}

func (logic debugLogicDriver) PersistentState() (ElementPtr, bool) {
	if logic.persistent == nil {
		return 0, false
	}

	return *logic.persistent, true
}

func (logic debugLogicDriver) EphemeralState() (ElementPtr, bool) {
	if logic.ephemeral == nil {
		return 0, false
	}

	return *logic.ephemeral, true
}

func (logic debugLogicDriver) GetCallsite(name string) (Callsite, bool) {
	callsite, ok := logic.callsites[name]

	return callsite, ok
}

func (logic debugLogicDriver) GetClassdef(name string) (Classdef, bool) {
	classdef, ok := logic.classdefs[name]

	return classdef, ok
}

func (logic debugLogicDriver) GetEventdef(name string) (Eventdef, bool) {
	eventdef, ok := logic.eventdefs[name]

	return eventdef, ok
}

func (logic debugLogicDriver) GetElement(ptr ElementPtr) (*LogicElement, bool) {
	element, ok := logic.elements[ptr]

	return element, ok
}

func (logic debugLogicDriver) GetElementDeps(ptr ElementPtr) []ElementPtr {
	return logic.dependencies.Dependencies(ptr)
}

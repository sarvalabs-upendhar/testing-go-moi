package engineio

import (
	"github.com/manishmeganathan/depgraph"

	"github.com/sarvalabs/go-moi-identifiers"
)

type (
	// ElementKind is a type alias for an element kind string
	ElementKind = string
	// ElementPtr is a type alias for an element pointer
	ElementPtr = uint64
)

// Classdef represents a class definition in a Logic.
// It can be resolved from a string by looking it up on the Logic
type Classdef struct {
	Ptr  ElementPtr `json:"ptr" yaml:"ptr"`
	Name string     `json:"name" yaml:"name"`
}

// Eventdef represents an event definition in a Logic.
// It can be resolved from a string by looking it up on the Logic
type Eventdef struct {
	Ptr  ElementPtr `json:"ptr" yaml:"ptr"`
	Name string     `json:"name" yaml:"name"`
}

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

// LogicElement represents a generic container for a logic Element.
// It is uniquely identified with a group name and an index pointer.
// Engine implementations are responsible for handling
// namespacing and index conflicts within a group.
type LogicElement struct {
	// Kind represents some type identifier for the element
	Kind ElementKind
	// Deps represents the relational neighbours of the element
	Deps []ElementPtr
	// Data represents the data container for the element
	Data []byte
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

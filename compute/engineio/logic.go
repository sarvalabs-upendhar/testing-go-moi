package engineio

import (
	"github.com/sarvalabs/go-moi/common"
)

// LogicDriver is an interface that defines a logic driver for an ExecutionEngine
type LogicDriver interface {
	// LogicID returns the unique Logic ID of the LogicDriver
	LogicID() common.LogicID
	// Engine returns the EngineKind of the LogicDriver
	Engine() EngineKind
	// Manifest returns the hash of the logic Manifest
	Manifest() common.Hash

	// IsSealed returns whether the state of the Logic has been sealed
	IsSealed() bool
	// IsAssetLogic returns whether the Logic is used for regulating an Asset
	IsAssetLogic() bool
	// AllowsInteractions returns whether the Logic supports Interactable Callsites
	AllowsInteractions() bool

	// PersistentState returns the pointer to the persistent state element
	// with a confirmation that the Logic defines a PersistentState
	PersistentState() (ElementPtr, bool)
	// EphemeralState returns the pointer to the ephemeral state element
	// with a confirmation that the Logic defines a EphemeralState
	EphemeralState() (ElementPtr, bool)

	// GetElementDeps returns the aggregated dependencies of an element pointer.
	// The aggregation includes all sub-dependencies recursively.
	GetElementDeps(ElementPtr) []ElementPtr
	// GetElement returns the LogicElement for a given element pointer with confirmation of its existence.
	GetElement(ElementPtr) (*LogicElement, bool)
	// GetCallsite returns Callsite for a given string name with confirmation of its existence.
	GetCallsite(string) (*Callsite, bool)
	// GetClassdef returns class Datatype for a given string name with confirmation of its existence.
	GetClassdef(string) (*Classdef, bool)
}

// LogicDescriptor represents an object that describes the internal
// components and characteristics of a Logic implementation
type LogicDescriptor struct {
	Engine EngineKind

	ManifestRaw  []byte
	ManifestHash common.Hash

	Interactive bool
	StateMatrix ContextStateMatrix

	DepGraph *DependencyGraph
	Elements map[ElementPtr]*LogicElement

	Callsites map[string]*Callsite
	Classdefs map[string]*Classdef
}

type (
	// ElementKind is a type alias for an element kind string
	ElementKind string
	// ElementPtr is a type alias for an element pointer
	ElementPtr uint64
)

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

// LogicImplSchema represents a schematic for verifying that a
// LogicDriver implements some specific interfaces and callpoints.
type LogicImplSchema interface{}

package engineio

type (
	// ElementKind is a type alias for an element kind string
	ElementKind = string
	// ElementPtr is a type alias for an element pointer
	ElementPtr = uint64
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

// Classdef represents a class definition in a Logic.
// It can be resolved from a string by looking it up on the Logic
type Classdef struct {
	Ptr  ElementPtr `json:"ptr" yaml:"ptr"`
	Name string     `json:"name" yaml:"name"`
}

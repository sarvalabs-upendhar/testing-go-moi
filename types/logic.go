package types

// LogicEngine is an enum with its variants
// representing the execution engine
type LogicEngine string

const (
	PISA LogicEngine = "PISA"
	MERU LogicEngine = "MERU"
)

// LogicCallsite represents a callable point in a Logic
// It can be resolved from a string by looking it up on the Logic
// TODO: this is a simple version. could it also include type information for the callsite?
type LogicCallsite uint64

// LogicDescriptor represents an object that describes the internal
// components and characteristics of a Logic implementation
type LogicDescriptor struct {
	Manifest Hash
	Engine   LogicEngine

	PersistentState    bool
	EphemeralState     bool
	AllowsInteractions bool

	Elements  []*LogicElement
	Callsites map[string]LogicCallsite
}

// LogicElement represents a generic container for a logic Element.
// It is uniquely identified with a group name and an index pointer.
// Engine implementations are responsible for handling
// namespacing and index conflicts within a group.
type LogicElement struct {
	// Kind represents some type identifier for the element
	Kind string
	// Index represents some numeric identifier for the elements of specific kind
	Index uint64
	// Data represents the data container for the element
	Data []byte
	// Binds represents the relational neighbours of the element
	Binds []struct {
		Kind  string
		Index uint64
	}
}

// LogicElementSet represents a collection of logic
// elements organized as double map to the element data.
type LogicElementSet map[string]map[uint64]*LogicElement

// Exists returns whether an LogicElement for the given ID exists in the LogicElementSet
func (set LogicElementSet) Exists(kind string, idx uint64) bool {
	group, exists := set[kind]
	if !exists {
		return false
	}

	_, exists = group[idx]

	return exists
}

// Insert inserts a variadic number of LogicElements to the set.
// If a LogicElement of the same kind and index already exist, it is overwritten.
func (set LogicElementSet) Insert(elements ...*LogicElement) {
	for _, element := range elements {
		if _, ok := set[element.Kind]; !ok {
			set[element.Kind] = make(map[uint64]*LogicElement)
		}

		set[element.Kind][element.Index] = element
	}
}

// Fetch fetches an LogicElement for a given kind and index from the LogicElementSet.
// Returns nil if no LogicElement exists for the given kind or index.
func (set LogicElementSet) Fetch(kind string, idx uint64) (*LogicElement, bool) {
	group, exists := set[kind]
	if !exists {
		return nil, false
	}

	element := group[idx]
	if element == nil {
		return nil, false
	}

	return element, true
}

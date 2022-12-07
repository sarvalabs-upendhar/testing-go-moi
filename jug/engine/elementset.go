package engine

import "github.com/pkg/errors"

var ErrElementNotFound = errors.New("element not found")

// Element represents a generic container for a logic Element.
// It is uniquely identified with a group name and an index pointer.
// Engine implementations are responsible for handling
// namespacing and index conflicts within a group.
type Element struct {
	// Kind represents some type identifier for the element
	Kind string
	// Index represents some numeric identifier for the elements of specific kind
	Index uint64
	// Represents the data container for the element
	Data []byte
}

// ElementSet represents a collection of logic elements
// organized as double map to the element data.
type ElementSet map[string]map[uint64][]byte

// Exists returns whether an Element for the given ID exists in the set
func (set ElementSet) Exists(kind string, idx uint64) bool {
	group, exists := set[kind]
	if !exists {
		return false
	}

	_, exists = group[idx]

	return exists
}

// Insert inserts a variadic number of Elements to the set.
// If an Element of the same kind and index already exist, it is overwritten.
func (set ElementSet) Insert(elements ...*Element) {
	for _, element := range elements {
		if _, ok := set[element.Kind]; !ok {
			set[element.Kind] = make(map[uint64][]byte)
		}

		set[element.Kind][element.Index] = element.Data
	}
}

// Fetch fetches an Element for a given kind and index from the ElementSet.
// Returns nil if no Element exists for the given kind or index.
func (set ElementSet) Fetch(kind string, idx uint64) (*Element, error) {
	group, exists := set[kind]
	if !exists {
		return nil, ErrElementNotFound
	}

	data := group[idx]
	if data == nil {
		return nil, ErrElementNotFound
	}

	return &Element{Kind: kind, Index: idx, Data: data}, nil
}

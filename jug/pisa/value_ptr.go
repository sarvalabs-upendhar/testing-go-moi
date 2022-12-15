package pisa

import "github.com/sarvalabs/go-polo"

// PtrValue represents a Value that operates like uint64 pointer address.
// It is purely a representational Value and has no operations and methods.
type PtrValue uint64

// NewPointer generates a new PtrValue for a given uint64 value
func NewPointer(ptr uint64) (PtrValue, error) {
	return PtrValue(ptr), nil
}

// Type returns the Datatype of PtrValue, which is TypePtr.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Type() *Datatype { return TypePtr }

// Copy returns a copy of PtrValue as a Value.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Copy() Value { return ptr }

// Data returns the POLO encoded bytes of PtrValue.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Data() []byte {
	data, _ := polo.Polorize(ptr)

	return data
}

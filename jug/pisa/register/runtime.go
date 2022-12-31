package register

import "github.com/sarvalabs/go-polo"

// NullValue represents the default empty value for a register
type NullValue struct{}

// Type returns the Typedef of NullValue, which is TypeNull.
// Implements the Value interface for NullValue.
func (null NullValue) Type() *Typedef { return TypeNull }

// Copy returns a copy of NullValue as a Value.
// Implements the Value interface for NullValue.
func (null NullValue) Copy() Value { return NullValue{} }

// Data returns the POLO encoded bytes of NullValue.
// Implements the Value interface for NullValue.
func (null NullValue) Data() []byte { return []byte{0} }

// PtrValue represents a Value that operates like uint64 pointer address.
// It is purely a representational Value and has no operations and methods.
type PtrValue uint64

// Type returns the Typedef of PtrValue, which is TypePtr.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Type() *Typedef { return TypePtr }

// Copy returns a copy of PtrValue as a Value.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Copy() Value { return ptr }

// Data returns the POLO encoded bytes of PtrValue.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Data() []byte {
	data, _ := polo.Polorize(ptr)

	return data
}

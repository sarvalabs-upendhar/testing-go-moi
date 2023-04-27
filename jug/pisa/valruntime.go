package pisa

import "github.com/sarvalabs/go-polo"

// Constant represents a constant value declaration.
// It consists of the type information of the constant (primitive)
// and some POLO encoded bytes that describe the constant value.
type Constant struct {
	Type Primitive
	Data []byte
}

// Value generate a new RegisterValue object from a Constant
// Returns an error if the constant data is not interpretable for its type.
func (constant *Constant) Value() (RegisterValue, error) {
	return NewRegisterValue(constant.Type.Datatype(), constant.Data)
}

// PtrValue represents a Value that operates like uint64 pointer address.
// It is purely a representational Value and has no operations and methods.
type PtrValue uint64

// Type returns the Datatype of PtrValue, which is TypePtr.
// Implements the RegisterValue interface for PtrValue.
func (ptr PtrValue) Type() *Datatype { return TypePtr }

// Copy returns a copy of PtrValue as a RegisterValue.
// Implements the Value interface for PtrValue.
func (ptr PtrValue) Copy() RegisterValue { return ptr }

// Norm returns the normalized value of PtrValue as an uint64.
// Implements the RegisterValue interface for PtrValue.
func (ptr PtrValue) Norm() any { return uint64(ptr) }

// Data returns the POLO encoded bytes of PtrValue.
// Implements the RegisterValue interface for PtrValue.
func (ptr PtrValue) Data() []byte {
	data, _ := polo.Polorize(ptr)

	return data
}

// NullValue represents the default empty value for a register
type NullValue struct{}

// Type returns the Datatype of NullValue, which is TypeNull.
// Implements the RegisterValue interface for NullValue.
func (null NullValue) Type() *Datatype { return TypeNull }

// Copy returns a copy of NullValue as a Value.
// Implements the RegisterValue interface for NullValue.
func (null NullValue) Copy() RegisterValue { return NullValue{} }

// Norm returns the normalized value of NullValue as a nil.
// Implements the RegisterValue interface for NullValue.
func (null NullValue) Norm() any { return nil }

// Data returns the POLO encoded bytes of NullValue.
// Implements the RegisterValue interface for NullValue.
func (null NullValue) Data() []byte { return []byte{0} }

// IsNullValue returns if the given RegisterValue is null.
// Is true if the register is empty or its datatype is TypeNull.
func IsNullValue(value RegisterValue) bool {
	if value == nil {
		return true
	}

	if value.Type().Equals(TypeNull) {
		return true
	}

	return false
}

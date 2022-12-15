package pisa

import "github.com/sarvalabs/go-polo"

// BoolValue represents a Value that operates like a boolean
type BoolValue bool

// NewBoolValue generates a new BoolValue for a given bool value.
func NewBoolValue(boolean bool) BoolValue { return BoolValue(boolean) }

// DefaultBoolValue generates a new BoolValue with a false value.
func DefaultBoolValue() BoolValue { return false }

// Type returns the Datatype of BoolValue, which is TypeBool.
// Implements the Value interface for BoolValue.
func (boolean BoolValue) Type() *Datatype { return TypeBool }

// Copy returns a copy of BoolValue as a Value.
// Implements the Value interface for BoolValue.
func (boolean BoolValue) Copy() Value { return boolean }

// Data returns the POLO encoded bytes of BoolValue.
// Implements the Value interface for BoolValue.
func (boolean BoolValue) Data() []byte {
	data, _ := polo.Polorize(boolean)

	return data
}

// And returns the value of and(x,y) as a BoolValue.
func (boolean BoolValue) And(other BoolValue) BoolValue { return boolean && other }

// Or returns the value of or(x,y) as a BoolValue.
func (boolean BoolValue) Or(other BoolValue) BoolValue { return boolean || other }

// Xor returns the value of xor(x,y) as a BoolValue.
func (boolean BoolValue) Xor(other BoolValue) BoolValue { return boolean != other }

// Not returns the value of not(x) as a BoolValue.
func (boolean BoolValue) Not() BoolValue { return !boolean }

package register

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

// BoolValue represents a Value that operates like a boolean
type BoolValue bool

// Type returns the Typedef of BoolValue, which is TypeBool.
// Implements the Value interface for BoolValue.
func (boolean BoolValue) Type() *Typedef { return TypeBool }

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

func BoolMethods() MethodTable {
	return MethodTable{
		// bool.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: PrimitiveBool,
			fields: CallFields{
				Inputs:  makefields([]*TypeField{{"self", TypeBool}}),
				Outputs: makefields([]*TypeField{{"result", TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Return a copy of the bool value
				return ValueTable{0: inputs[0].Copy()}, nil
			},
		},

		// bool.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: PrimitiveBool,
			fields: CallFields{
				Inputs:  makefields([]*TypeField{{"self", TypeBool}}),
				Outputs: makefields([]*TypeField{{"result", TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Convert bool to its string form
				var result StringValue
				if inputs[0].(BoolValue) { //nolint:forcetypeassert
					result = "true"
				} else {
					result = "false"
				}

				// Set the result into the outputs
				return ValueTable{0: result}, nil
			},
		},
	}
}

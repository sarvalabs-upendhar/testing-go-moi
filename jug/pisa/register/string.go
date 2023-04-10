package register

import (
	"strings"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

// StringValue represents a Value that operates like a string.
type StringValue string

// Type returns the Typedef of StringValue, which is TypeString.
// Implements the Value interface for StringValue.
func (str StringValue) Type() *engineio.Datatype { return engineio.TypeString }

// Copy returns a copy of StringValue as a Value.
// Implements the Value interface for StringValue.
func (str StringValue) Copy() Value { return StringValue(strings.Clone(string(str))) }

// Norm returns the normalized value of StringValue as a string.
// Implements the Value interface for StringValue.
func (str StringValue) Norm() any { return string(str) }

// Data returns the POLO encoded bytes of StringValue.
// Implements the Value interface for StringValue.
func (str StringValue) Data() []byte {
	data, _ := polo.Polorize(str)

	return data
}

func (str StringValue) Concat(other StringValue) StringValue {
	return str + other
}

func (str StringValue) HasPrefix(prefix StringValue) BoolValue {
	return BoolValue(strings.HasPrefix(string(str), string(prefix)))
}

//nolint:lll
func StringMethods() MethodTable {
	return MethodTable{
		// string.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: engineio.PrimitiveString,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeString}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// True for all values except empty string
				result := inputs[0].(StringValue) != "" //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// string.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: engineio.PrimitiveString,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeString}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Return a copy of the string value
				return ValueTable{0: inputs[0].Copy()}, nil
			},
		},

		// string.HasPrefix(string) -> bool
		0x10: &BuiltinMethod{
			datatype: engineio.PrimitiveString,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeString}, {Name: "prefix", Type: engineio.TypeString}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "ok", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				self, prefix := inputs[0], inputs[1]
				ok := self.(StringValue).HasPrefix(prefix.(StringValue)) //nolint:forcetypeassert

				return ValueTable{0: ok}, nil
			},
		},
	}
}

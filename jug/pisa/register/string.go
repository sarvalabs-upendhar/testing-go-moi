package register

import (
	"strings"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/exceptions"
)

// StringValue represents a Value that operates like a string.
type StringValue string

// Type returns the Typedef of StringValue, which is TypeString.
// Implements the Value interface for StringValue.
func (str StringValue) Type() *Typedef { return TypeString }

// Copy returns a copy of StringValue as a Value.
// Implements the Value interface for StringValue.
func (str StringValue) Copy() Value { return StringValue(strings.Clone(string(str))) }

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

func StringMethods() MethodTable {
	return MethodTable{
		// string.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: PrimitiveString,
			fields: CallFields{
				Inputs:  fields([]*TypeField{{"self", TypeString}}),
				Outputs: fields([]*TypeField{{"result", TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exceptions.ExceptionObject) {
				// True for all values except empty string
				result := inputs[0].(StringValue) != "" //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// string.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: PrimitiveString,
			fields: CallFields{
				Inputs:  fields([]*TypeField{{"self", TypeString}}),
				Outputs: fields([]*TypeField{{"result", TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exceptions.ExceptionObject) {
				// Return a copy of the string value
				return ValueTable{0: inputs[0].Copy()}, nil
			},
		},

		// string.HasPrefix(string) -> bool
		0x10: &BuiltinMethod{
			datatype: PrimitiveString,
			fields: CallFields{
				Inputs:  fields([]*TypeField{{"self", TypeString}, {"prefix", TypeString}}),
				Outputs: fields([]*TypeField{{"ok", TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exceptions.ExceptionObject) {
				self, prefix := inputs[0], inputs[1]
				ok := self.(StringValue).HasPrefix(prefix.(StringValue)) //nolint:forcetypeassert

				return ValueTable{0: ok}, nil
			},
		},
	}
}

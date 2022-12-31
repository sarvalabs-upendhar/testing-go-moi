package register

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/exceptions"
)

// BytesValue represents Value that operates like a bytes
type BytesValue []byte

// Type returns the Typedef of BytesValue, which is TypeBytes.
// Implements the Value interface for BytesValue.
func (bytes BytesValue) Type() *Typedef { return TypeBytes }

// Copy returns a copy of BytesValue as a Value.
// Implements the Value interface for BytesValue.
func (bytes BytesValue) Copy() Value {
	copied := BytesValue{}
	copy(bytes, copied)

	return copied
}

// Data returns the POLO encoded bytes of BytesValue.
// Implements the Value interface for BytesValue.
func (bytes BytesValue) Data() []byte {
	data, _ := polo.Polorize(bytes)

	return data
}

func BytesMethods() MethodTable {
	return MethodTable{
		// bytes.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: PrimitiveBytes,
			fields: CallFields{
				Inputs:  fields([]*TypeField{{"self", TypeBytes}}),
				Outputs: fields([]*TypeField{{"result", TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exceptions.ExceptionObject) {
				// True for all values except empty bytes
				result := len(inputs[0].(BytesValue)) != 0 //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// bytes.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: PrimitiveBytes,
			fields: CallFields{
				Inputs:  fields([]*TypeField{{"self", TypeBytes}}),
				Outputs: fields([]*TypeField{{"result", TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exceptions.ExceptionObject) {
				// Return bytes converted into a string
				return ValueTable{0: StringValue(inputs[0].(BytesValue))}, nil //nolint:forcetypeassert
			},
		},
	}
}

package register

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

// BytesValue represents Value that operates like a bytes
type BytesValue []byte

// Type returns the Typedef of BytesValue, which is TypeBytes.
// Implements the Value interface for BytesValue.
func (bytes BytesValue) Type() *engineio.Datatype { return engineio.TypeBytes }

// Copy returns a copy of BytesValue as a Value.
// Implements the Value interface for BytesValue.
func (bytes BytesValue) Copy() Value {
	clone := make(BytesValue, len(bytes))
	copy(clone, bytes)

	return clone
}

// Norm returns the normalized value of BytesValue as a []byte.
// Implements the Value interface for BytesValue.
func (bytes BytesValue) Norm() any { return []byte(bytes) }

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
			datatype: engineio.PrimitiveBytes,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeBytes}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// True for all values except empty bytes
				result := len(inputs[0].(BytesValue)) != 0 //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// bytes.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: engineio.PrimitiveBytes,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeBytes}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Return bytes converted into a string
				return ValueTable{0: StringValue(inputs[0].(BytesValue))}, nil //nolint:forcetypeassert
			},
		},
	}
}

package register

import (
	"encoding/hex"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/types"
)

// AddressValue represents a Value that operates like a types.Address
type AddressValue [32]byte

// Type returns the Typedef of AddressValue, which is TypeAddress.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Type() *Typedef { return TypeAddress }

// Copy returns a copy of AddressValue as a Value.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Copy() Value { return addr }

// Data returns the POLO encoded bytes of AddressValue.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Data() []byte {
	data, _ := polo.Polorize(addr)

	return data
}

func (addr AddressValue) ToHex() StringValue {
	return StringValue(hex.EncodeToString(addr[:]))
}

func AddressMethods() MethodTable {
	return MethodTable{
		// address.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: PrimitiveAddress,
			fields: CallFields{
				Inputs:  makefields([]*TypeField{{"self", TypeAddress}}),
				Outputs: makefields([]*TypeField{{"result", TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// True for all values except nil address
				result := inputs[0].(AddressValue) != AddressValue(types.NilAddress) //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// address.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: PrimitiveAddress,
			fields: CallFields{
				Inputs:  makefields([]*TypeField{{"self", TypeAddress}}),
				Outputs: makefields([]*TypeField{{"result", TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Hex encode the address
				result := inputs[0].(AddressValue).ToHex() //nolint:forcetypeassert
				// Set the result into the outputs
				return ValueTable{0: result}, nil
			},
		},
	}
}

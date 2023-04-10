package register

import (
	"encoding/hex"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/types"
)

// AddressValue represents a Value that operates like a types.Address
type AddressValue [32]byte

// Type returns the Typedef of AddressValue, which is TypeAddress.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Type() *engineio.Datatype { return engineio.TypeAddress }

// Copy returns a copy of AddressValue as a Value.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Copy() Value { return addr }

// Norm returns the normalized value of AddressValue as a [32]byte.
// Implements the Value interface for AddressValue.
func (addr AddressValue) Norm() any { return [32]byte(addr) }

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
			datatype: engineio.PrimitiveAddress,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeAddress}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
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
			datatype: engineio.PrimitiveAddress,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeAddress}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeString}}),
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

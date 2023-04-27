package pisa

import (
	"encoding/hex"
	"strings"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

/*
BoolValue Implementation
*/

// BoolValue represents a RegisterValue that operates like a boolean
type BoolValue bool

// Type returns the Datatype of BoolValue, which is TypeBool.
// Implements the RegisterValue interface for BoolValue.
func (boolean BoolValue) Type() *Datatype { return TypeBool }

// Copy returns a copy of BoolValue as a RegisterValue.
// Implements the RegisterValue interface for BoolValue.
func (boolean BoolValue) Copy() RegisterValue { return boolean }

// Norm returns the normalized value of BoolValue as a bool.
// Implements the RegisterValue interface for BoolValue.
func (boolean BoolValue) Norm() any { return bool(boolean) }

// Data returns the POLO encoded bytes of BoolValue.
// Implements the RegisterValue interface for BoolValue.
func (boolean BoolValue) Data() []byte {
	data, _ := polo.Polorize(boolean)

	return data
}

// And returns the value of and(x,y) as a BoolValue.
func (boolean BoolValue) And(other BoolValue) BoolValue { return boolean && other }

// Or returns the value of or(x,y) as a BoolValue.
func (boolean BoolValue) Or(other BoolValue) BoolValue { return boolean || other }

// Not returns the value of not(x) as a BoolValue.
func (boolean BoolValue) Not() BoolValue { return !boolean }

//nolint:forcetypeassert
func methodsBool() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// bool.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveBool, MethodBool,
			makefields([]*TypeField{{"self", TypeBool}}),
			makefields([]*TypeField{{"result", TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the bool value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// bool.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveBool, MethodStr,
			makefields([]*TypeField{{"self", TypeBool}}),
			makefields([]*TypeField{{"result", TypeString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Convert bool to its string form
				if inputs[0].(BoolValue) {
					return RegisterSet{0: StringValue("true")}, nil
				}

				return RegisterSet{0: StringValue("true")}, nil
			},
		),
	}
}

/*
StringValue Implementation
*/

// StringValue represents a RegisterValue that operates like a string.
type StringValue string

// Type returns the Datatype of StringValue, which is TypeString.
// Implements the RegisterValue interface for StringValue.
func (str StringValue) Type() *Datatype { return TypeString }

// Copy returns a copy of StringValue as a RegisterValue.
// Implements the RegisterValue interface for StringValue.
func (str StringValue) Copy() RegisterValue { return StringValue(strings.Clone(string(str))) }

// Norm returns the normalized value of StringValue as a string.
// Implements the RegisterValue interface for StringValue.
func (str StringValue) Norm() any { return string(str) }

// Data returns the POLO encoded bytes of StringValue.
// Implements the RegisterValue interface for StringValue.
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

//nolint:forcetypeassert
func methodsString() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// string.__throw__() -> string
		MethodThrow: makeBuiltinMethod(
			MethodThrow.String(),
			PrimitiveString, MethodThrow,
			makefields([]*TypeField{{"self", TypeString}}),
			makefields([]*TypeField{{"except", TypeString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the string value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// string.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveString, MethodBool,
			makefields([]*TypeField{{"self", TypeString}}),
			makefields([]*TypeField{{"result", TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except empty string
				result := inputs[0].(StringValue) != ""
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// string.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveString, MethodStr,
			makefields([]*TypeField{{"self", TypeString}}),
			makefields([]*TypeField{{"result", TypeString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the string value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// string.HasPrefix(string) -> bool
		0x10: makeBuiltinMethod(
			"HasPrefix",
			PrimitiveString, 0x10,
			makefields([]*TypeField{{"self", TypeString}, {"prefix", TypeString}}),
			makefields([]*TypeField{{"ok", TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, prefix := inputs[0], inputs[1]
				ok := self.(StringValue).HasPrefix(prefix.(StringValue))

				return RegisterSet{0: ok}, nil
			},
		),
	}
}

/*
BytesValue Implementation
*/

// BytesValue represents RegisterValue that operates like a bytes
type BytesValue []byte

// Type returns the Datatype of BytesValue, which is TypeBytes.
// Implements the RegisterValue interface for BytesValue.
func (bytes BytesValue) Type() *Datatype { return TypeBytes }

// Copy returns a copy of BytesValue as a RegisterValue.
// Implements the RegisterValue interface for BytesValue.
func (bytes BytesValue) Copy() RegisterValue {
	clone := make(BytesValue, len(bytes))
	copy(clone, bytes)

	return clone
}

// Norm returns the normalized value of BytesValue as a []byte.
// Implements the RegisterValue interface for BytesValue.
func (bytes BytesValue) Norm() any { return []byte(bytes) }

// Data returns the POLO encoded bytes of BytesValue.
// Implements the RegisterValue interface for BytesValue.
func (bytes BytesValue) Data() []byte {
	data, _ := polo.Polorize(bytes)

	return data
}

//nolint:forcetypeassert
func methodsBytes() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// bytes.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveBytes, MethodBool,
			makefields([]*TypeField{{"self", TypeBytes}}),
			makefields([]*TypeField{{"result", TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except empty bytes
				result := len(inputs[0].(BytesValue)) != 0
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// bytes.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveBytes, MethodStr,
			makefields([]*TypeField{{"self", TypeBytes}}),
			makefields([]*TypeField{{"result", TypeString}}),

			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return bytes converted into a string
				return RegisterSet{0: StringValue(inputs[0].(BytesValue))}, nil
			},
		),
	}
}

/*
AddressValue Implementation
*/

// AddressValue represents a RegisterValue that operates like a types.Address
type AddressValue [32]byte

// Type returns the Datatype of AddressValue, which is TypeAddress.
// Implements the RegisterValue interface for AddressValue.
func (addr AddressValue) Type() *Datatype { return TypeAddress }

// Copy returns a copy of AddressValue as a RegisterValue.
// Implements the RegisterValue interface for AddressValue.
func (addr AddressValue) Copy() RegisterValue { return addr }

// Norm returns the normalized value of AddressValue as a [32]byte.
// Implements the RegisterValue interface for AddressValue.
func (addr AddressValue) Norm() any { return [32]byte(addr) }

// Data returns the POLO encoded bytes of AddressValue.
// Implements the RegisterValue interface for AddressValue.
func (addr AddressValue) Data() []byte {
	data, _ := polo.Polorize(addr)

	return data
}

func (addr AddressValue) ToHex() StringValue {
	return StringValue(hex.EncodeToString(addr[:]))
}

//nolint:forcetypeassert
func methodsAddress() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// address.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveAddress, MethodBool,
			makefields([]*TypeField{{"self", TypeAddress}}),
			makefields([]*TypeField{{"result", TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except nil address
				result := inputs[0].(AddressValue) != AddressValue(types.NilAddress)
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// address.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveAddress, MethodStr,
			makefields([]*TypeField{{"self", TypeAddress}}),
			makefields([]*TypeField{{"result", TypeString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Hex encode the address
				result := inputs[0].(AddressValue).ToHex()
				// Set the result into the outputs
				return RegisterSet{0: result}, nil
			},
		),
	}
}

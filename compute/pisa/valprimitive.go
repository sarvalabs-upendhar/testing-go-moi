package pisa

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"
	"unicode"

	"github.com/holiman/uint256"

	"github.com/sarvalabs/go-polo"
)

/*
BoolValue Implementation
*/

// BoolValue represents a RegisterValue that operates like a boolean
type BoolValue bool

// Type returns the Datatype of BoolValue, which is PrimitiveBool.
// Implements the RegisterValue interface for BoolValue.
func (boolean BoolValue) Type() Datatype { return PrimitiveBool }

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
func (boolean BoolValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// bool.__join__(bool) -> bool
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveBool, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveBool}, {"other", PrimitiveBool}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform boolean AND on the operands
				result := inputs[0].(BoolValue).And(inputs[1].(BoolValue))
				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// bool.__eq__(bool) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveBool, MethodEq, 10,
			makefields([]*TypeField{{"self", PrimitiveBool}, {"other", PrimitiveBool}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Check if both boolean values are equal
				equals := inputs[0].(BoolValue) == inputs[1].(BoolValue)
				// Return the result
				return RegisterSet{0: BoolValue(equals)}, nil
			},
		),

		// bool.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveBool, MethodBool, 10,
			makefields([]*TypeField{{"self", PrimitiveBool}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the bool value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// bool.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveBool, MethodStr, 10,
			makefields([]*TypeField{{"self", PrimitiveBool}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Convert bool to its string form
				if inputs[0].(BoolValue) {
					return RegisterSet{0: StringValue("true")}, nil
				}

				return RegisterSet{0: StringValue("false")}, nil
			},
		),
	}
}

/*
StringValue Implementation
*/

// StringValue represents a RegisterValue that operates like a string.
type StringValue string

// Type returns the Datatype of StringValue, which is PrimitiveString.
// Implements the RegisterValue interface for StringValue.
func (str StringValue) Type() Datatype { return PrimitiveString }

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

func (str StringValue) Get(idx U64Value) (StringValue, *Exception) {
	if idx >= U64Value(len(str)) {
		return "", exceptionOutOfBounds()
	}

	char := str[idx : idx+1]

	return char, nil
}

func (str StringValue) Set(idx U64Value, substr StringValue) (StringValue, *Exception) {
	if idx >= U64Value(len(str)) {
		return "", exceptionOutOfBounds()
	}

	res := str[:idx] + substr + str[idx+1:]

	return res, nil
}

func (str StringValue) Slice(start U64Value, stop U64Value) (StringValue, *Exception) {
	if stop > U64Value(len(str)) || start > stop {
		return "", exceptionOutOfBounds()
	}

	res := str[start:stop]

	return res, nil
}

//nolint:forcetypeassert
func (str StringValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// string.__throw__() -> string
		MethodThrow: makeBuiltinMethod(
			MethodThrow.String(),
			PrimitiveString, MethodThrow, 20,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"except", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the string value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// string.__join__(string) -> string
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveString, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveString}, {"other", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform string concatenation on the operands
				result := inputs[0].(StringValue).Concat(inputs[1].(StringValue))
				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// string.__eq__(string) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveString, MethodEq, 10,
			makefields([]*TypeField{{"self", PrimitiveString}, {"other", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Check if both string values are equal
				equals := inputs[0].(StringValue) == inputs[1].(StringValue)
				// Return the result
				return RegisterSet{0: BoolValue(equals)}, nil
			},
		),

		// string.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveString, MethodBool, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
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
			PrimitiveString, MethodStr, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the string value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// string.__addr__() -> address
		MethodAddr: makeBuiltinMethod(
			MethodAddr.String(),
			PrimitiveString, MethodAddr, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveAddress}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Remove the 0x prefix on the string if it exists
				trimmed := strings.TrimPrefix(string(inputs[0].(StringValue)), "0x")
				// Decode the string as hex encoded data
				decoded, err := hex.DecodeString(trimmed)
				if err != nil {
					return nil, exception(ValueError, "string hex decode failed")
				}

				// Check size of bytes value (must be less than 32)
				if len(decoded) > 32 {
					return RegisterSet{}, exception(ValueError, "data too long for address")
				}

				// Create a new AddressValue and set the bytes
				addr := new(AddressValue)
				addr.SetBytes(decoded)

				// Return address
				return RegisterSet{0: *addr}, nil
			},
		),

		// string.__len__() -> u64
		MethodLen: makeBuiltinMethod(
			MethodLen.String(),
			PrimitiveString, MethodLen, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"length", PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Get length of bytes
				length := len(inputs[0].(StringValue))
				// Return the length as u64
				return RegisterSet{0: U64Value(length)}, nil
			},
		),

		// string.Get(string, position) -> string
		0x10: makeBuiltinMethod("Get",
			PrimitiveString, 0x10, 10,
			makefields([]*TypeField{{"self", PrimitiveString}, {"position", PrimitiveU64}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, pos := inputs[0], inputs[1]
				char, err := self.(StringValue).Get(pos.(U64Value))
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: char}, nil
			},
		),

		// string.Set(string, position, update_string) -> string
		0x11: makeBuiltinMethod("Set",
			PrimitiveString, 0x11, 15,
			makefields([]*TypeField{
				{Name: "self", Type: PrimitiveString},
				{Name: "position", Type: PrimitiveU64},
				{Name: "update_string", Type: PrimitiveString},
			}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, pos, updateChar := inputs[0], inputs[1], inputs[2]
				res, err := self.(StringValue).Set(pos.(U64Value), updateChar.(StringValue))
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: res}, nil
			},
		),

		// string.IsAlpha(string) -> bool
		0x12: makeBuiltinMethod("IsAlpha",
			PrimitiveString, 0x12, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0].(StringValue)
				b := true

				if len(self) == 0 {
					b = false
				}

				for _, r := range self {
					if !unicode.IsLetter(r) {
						b = false

						break
					}
				}

				return RegisterSet{0: BoolValue(b)}, nil
			},
		),

		// string.IsNumeric(string) -> bool
		0x13: makeBuiltinMethod("IsNumeric",
			PrimitiveString, 0x13, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0].(StringValue)
				b := true

				if len(self) == 0 {
					b = false
				}

				for _, r := range self {
					if !unicode.IsNumber(r) {
						b = false

						break
					}
				}

				return RegisterSet{0: BoolValue(b)}, nil
			},
		),

		// string.IsLower(string) -> bool
		0x14: makeBuiltinMethod("IsLower",
			PrimitiveString, 0x14, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0].(StringValue)
				b := true

				for _, r := range self {
					if !unicode.IsLower(r) && unicode.IsLetter(r) {
						b = false

						break
					}
				}

				return RegisterSet{0: BoolValue(b)}, nil
			},
		),

		// string.IsUpper(string) -> bool
		0x15: makeBuiltinMethod("IsUpper",
			PrimitiveString, 0x15, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0].(StringValue)
				b := true

				for _, r := range self {
					if !unicode.IsUpper(r) && unicode.IsLetter(r) {
						b = false

						break
					}
				}

				return RegisterSet{0: BoolValue(b)}, nil
			},
		),

		// string.HasPrefix(string) -> bool
		0x16: makeBuiltinMethod(
			"HasPrefix",
			PrimitiveString, 0x16, 10,
			makefields([]*TypeField{{"self", PrimitiveString}, {"prefix", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, prefix := inputs[0], inputs[1]
				ok := strings.HasPrefix(string(self.(StringValue)), string(prefix.(StringValue)))

				return RegisterSet{0: BoolValue(ok)}, nil
			},
		),

		// string.HasSuffix(string) -> bool
		0x17: makeBuiltinMethod(
			"HasSuffix",
			PrimitiveString, 0x17, 10,
			makefields([]*TypeField{{"self", PrimitiveString}, {"suffix", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, suffix := inputs[0], inputs[1]
				ok := strings.HasSuffix(string(self.(StringValue)), string(suffix.(StringValue)))

				return RegisterSet{0: BoolValue(ok)}, nil
			},
		),

		// string.Contains(string) -> bool
		0x18: makeBuiltinMethod(
			"Contains",
			PrimitiveString, 0x18, 30,
			makefields([]*TypeField{{"self", PrimitiveString}, {"contains", PrimitiveString}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, substr := inputs[0], inputs[1]
				ok := strings.Contains(string(self.(StringValue)), string(substr.(StringValue)))

				return RegisterSet{0: BoolValue(ok)}, nil
			},
		),

		// string.Split(string, delim) -> []string
		0x19: makeBuiltinMethod(
			"Split",
			PrimitiveString, 0x19, 30,
			makefields([]*TypeField{{"self", PrimitiveString}, {"delim", PrimitiveString}}),
			makefields([]*TypeField{{"result", VarrayDatatype{PrimitiveString}}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, delim := inputs[0], inputs[1]
				res := strings.Split(string(self.(StringValue)), string(delim.(StringValue)))
				var resvalarr []RegisterValue
				for _, i := range res {
					resval := StringValue(i)
					resvalarr = append(resvalarr, resval)
				}
				resOp, _ := newVarrayFromValues(VarrayDatatype{PrimitiveString}, resvalarr...)

				return RegisterSet{0: resOp}, nil
			},
		),

		// string.Slice(string) -> string
		0x1A: makeBuiltinMethod(
			"Slice",
			PrimitiveString, 0x1A, 30,
			makefields([]*TypeField{{"self", PrimitiveString}, {"idx1", PrimitiveU64}, {"idx2", PrimitiveU64}}),
			makefields([]*TypeField{{"ok", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, idx1, idx2 := inputs[0], inputs[1], inputs[2]
				res, err := self.(StringValue).Slice(idx1.(U64Value), idx2.(U64Value))
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: res}, nil
			},
		),

		// string.ToLower() -> string
		0x1B: makeBuiltinMethod(
			"ToLower",
			PrimitiveString, 0x1B, 20,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"res", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				res := strings.ToLower(string(self.(StringValue)))

				return RegisterSet{0: StringValue(res)}, nil
			},
		),

		// string.ToUpper() -> string
		0x1C: makeBuiltinMethod(
			"ToUpper",
			PrimitiveString, 0x1C, 20,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"res", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				res := strings.ToUpper(string(self.(StringValue)))

				return RegisterSet{0: StringValue(res)}, nil
			},
		),

		// string.ToBytes() -> bytes
		0x1D: makeBuiltinMethod(
			"ToBytes",
			PrimitiveString, 0x1D, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveBytes}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				result := BytesValue(self.(StringValue))

				return RegisterSet{0: result}, nil
			},
		),

		// string.ToU64(string) -> U64
		0x1E: makeBuiltinMethod("ToU64",
			PrimitiveString, 0x1E, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]

				res, err := strconv.ParseUint(string(self.(StringValue)), 0, 64)
				if err != nil {
					return nil, exception(ValueError, err.Error())
				}

				return RegisterSet{0: U64Value(res)}, nil
			},
		),

		// string.ToI64(string) -> I64
		0x1F: makeBuiltinMethod("ToI64",
			PrimitiveString, 0x1F, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]

				res, err := strconv.ParseUint(string(self.(StringValue)), 0, 64)
				if err != nil {
					return nil, exception(ValueError, err.Error())
				}

				return RegisterSet{0: I64Value(res)}, nil
			},
		),

		// string.ToU256(string) -> U256
		0x20: makeBuiltinMethod("ToU256",
			PrimitiveString, 0x20, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]

				res, err := uint256.FromHex(string(self.(StringValue)))
				if err != nil {
					return nil, exception(ValueError, err.Error())
				}

				return RegisterSet{0: &U256Value{value: res}}, nil
			},
		),

		// string.ToI256(string) -> I256
		0x21: makeBuiltinMethod("ToI256",
			PrimitiveString, 0x21, 10,
			makefields([]*TypeField{{"self", PrimitiveString}}),
			makefields([]*TypeField{{"result", PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]

				res, err := uint256.FromHex(string(self.(StringValue)))
				if err != nil {
					return nil, exception(ValueError, err.Error())
				}

				return RegisterSet{0: &I256Value{value: res}}, nil
			},
		),
	}
}

/*
BytesValue Implementation
*/

// BytesValue represents RegisterValue that operates like a bytes
type BytesValue []byte

// Type returns the Datatype of BytesValue, which is PrimitiveBytes.
// Implements the RegisterValue interface for BytesValue.
func (bytesval BytesValue) Type() Datatype { return PrimitiveBytes }

// Copy returns a copy of BytesValue as a RegisterValue.
// Implements the RegisterValue interface for BytesValue.
func (bytesval BytesValue) Copy() RegisterValue {
	clone := make(BytesValue, len(bytesval))
	copy(clone, bytesval)

	return clone
}

// Norm returns the normalized value of BytesValue as a []byte.
// Implements the RegisterValue interface for BytesValue.
func (bytesval BytesValue) Norm() any { return []byte(bytesval) }

// Data returns the POLO encoded bytes of BytesValue.
// Implements the RegisterValue interface for BytesValue.
func (bytesval BytesValue) Data() []byte {
	data, _ := polo.Polorize(bytesval)

	return data
}

func (bytesval BytesValue) Concat(other BytesValue) BytesValue {
	return bytes.Join([][]byte{bytesval, other}, []byte{})
}

func (bytesval BytesValue) Get(idx U64Value) (BytesValue, *Exception) {
	if idx >= U64Value(len(bytesval)) {
		return []byte{}, exceptionOutOfBounds()
	}

	res := bytesval[idx : idx+1]

	return res, nil
}

func (bytesval BytesValue) Set(idx U64Value, updatebyte BytesValue) (BytesValue, *Exception) {
	if idx >= U64Value(len(bytesval)) {
		return []byte{}, exceptionOutOfBounds()
	}

	bytesval[idx] = updatebyte[0]

	return bytesval, nil
}

func (bytesval BytesValue) Slice(start U64Value, stop U64Value) (BytesValue, *Exception) {
	if stop > U64Value(len(bytesval)) || start > stop {
		return []byte{}, exceptionOutOfBounds()
	}

	res := bytesval[start:stop]

	return res, nil
}

//nolint:forcetypeassert
func (bytesval BytesValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// bytes.__join__(bytes) -> bytes
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveBytes, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"other", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveBytes}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform bytes concatenation on the operands
				result := inputs[0].(BytesValue).Concat(inputs[1].(BytesValue))
				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// bytes.__eq__(bytes) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveBytes, MethodEq, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"other", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Check if both bytes values are equal
				equals := bytes.Equal(inputs[0].(BytesValue), inputs[1].(BytesValue))
				// Return the result
				return RegisterSet{0: BoolValue(equals)}, nil
			},
		),

		// bytes.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveBytes, MethodBool, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
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
			PrimitiveBytes, MethodStr, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),

			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return bytes converted into a string
				return RegisterSet{0: StringValue(inputs[0].(BytesValue))}, nil
			},
		),

		// bytes.__addr__() -> addr
		MethodAddr: makeBuiltinMethod(
			MethodAddr.String(),
			PrimitiveBytes, MethodAddr, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"address", PrimitiveAddress}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Check size of bytes value (must be less than 32)
				if len(inputs[0].(BytesValue)) > 32 {
					return RegisterSet{}, exception(ValueError, "data too long for address")
				}

				// Create a new AddressValue and set the bytes
				addr := new(AddressValue)
				addr.SetBytes(inputs[0].(BytesValue))

				// Return address
				return RegisterSet{0: *addr}, nil
			},
		),

		// bytes.__len__() -> u64
		MethodLen: makeBuiltinMethod(
			MethodLen.String(),
			PrimitiveBytes, MethodLen, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"length", PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Get length of bytes
				length := len(inputs[0].(BytesValue))
				// Return the length as u64
				return RegisterSet{0: U64Value(length)}, nil
			},
		),

		// byte.Get(byte, position) -> byte
		0x10: makeBuiltinMethod("Get",
			PrimitiveBytes, 0x10, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"position", PrimitiveU64}}),
			makefields([]*TypeField{{"result", PrimitiveBytes}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, pos := inputs[0], inputs[1]
				byteres, err := self.(BytesValue).Get(pos.(U64Value))
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: byteres}, nil
			},
		),

		// byte.Set(byte, position, updateByte) -> byte
		0x11: makeBuiltinMethod("Set",
			PrimitiveBytes, 0x11, 15,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"position", PrimitiveU64}, {"update_byte", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveBytes}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, pos, updateByte := inputs[0], inputs[1], inputs[2]
				res, err := self.(BytesValue).Set(pos.(U64Value), updateByte.(BytesValue))
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: res}, nil
			},
		),

		// byte.HasPrefix(byte) -> bool
		0x12: makeBuiltinMethod("HasPrefix",
			PrimitiveBytes, 0x12, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"prefix", PrimitiveBytes}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, prefix := inputs[0], inputs[1]
				ok := bytes.HasPrefix(self.(BytesValue), prefix.(BytesValue))

				return RegisterSet{0: BoolValue(ok)}, nil
			},
		),

		// byte.HasSuffix(byte) -> bool
		0x13: makeBuiltinMethod("HasSuffix",
			PrimitiveBytes, 0x13, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"suffix", PrimitiveBytes}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, prefix := inputs[0], inputs[1]
				ok := bytes.HasSuffix(self.(BytesValue), prefix.(BytesValue))

				return RegisterSet{0: BoolValue(ok)}, nil
			},
		),

		// byte.Contains(byte) -> bool
		0x14: makeBuiltinMethod("Contains",
			PrimitiveBytes, 0x14, 30,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"subbyte", PrimitiveBytes}}),
			makefields([]*TypeField{{"ok", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, subbyte := inputs[0], inputs[1]
				ok := bytes.Contains(self.(BytesValue), subbyte.(BytesValue))

				return RegisterSet{0: BoolValue(ok)}, nil
			},
		),

		// byte.Split(byte, delim) -> []byte
		0x15: makeBuiltinMethod("Split",
			PrimitiveBytes, 0x15, 30,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"delim", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", VarrayDatatype{PrimitiveBytes}}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, delim := inputs[0], inputs[1]
				res := bytes.Split(self.(BytesValue), delim.(BytesValue))
				var resvalarr []RegisterValue
				for _, i := range res {
					resval := BytesValue(i)
					resvalarr = append(resvalarr, resval)
				}
				resOp, _ := newVarrayFromValues(VarrayDatatype{PrimitiveBytes}, resvalarr...)

				return RegisterSet{0: resOp}, nil
			},
		),

		// byte.Slice(byte, idx1, idx2) -> byte
		0x16: makeBuiltinMethod("Slice",
			PrimitiveBytes, 0x16, 30,
			makefields([]*TypeField{{"self", PrimitiveBytes}, {"idx1", PrimitiveU64}, {"idx2", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", VarrayDatatype{PrimitiveBytes}}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self, idx1, idx2 := inputs[0], inputs[1], inputs[2]
				res, err := self.(BytesValue).Slice(idx1.(U64Value), idx2.(U64Value))
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: res}, nil
			},
		),

		// byte.ToU64(byte) -> U64
		0x17: makeBuiltinMethod("ToU64",
			PrimitiveBytes, 0x17, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				if len(self.Data()) > 8 {
					return nil, exception(OverflowError, "U64 size overflow")
				}
				res := int(big.NewInt(0).SetBytes(self.(BytesValue)).Uint64())

				return RegisterSet{0: U64Value(res)}, nil
			},
		),

		// byte.ToI64(byte) -> I64
		0x18: makeBuiltinMethod("ToI64",
			PrimitiveBytes, 0x18, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				if len(self.(BytesValue)) > 8 {
					return nil, exception(OverflowError, "I64 size overflow")
				}
				res := int(big.NewInt(0).SetBytes(self.(BytesValue)).Int64())

				return RegisterSet{0: I64Value(res)}, nil
			},
		),

		// byte.ToU256(byte) -> U256
		0x19: makeBuiltinMethod("ToU256",
			PrimitiveBytes, 0x19, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]

				res, err := uint256.FromBig(big.NewInt(0).SetBytes(self.(BytesValue)))
				if err {
					return nil, exception(OverflowError, "U256 size overflow")
				}

				return RegisterSet{0: &U256Value{value: res}}, nil
			},
		),

		// byte.ToI256(byte) -> I256
		0x20: makeBuiltinMethod("ToI256",
			PrimitiveBytes, 0x20, 10,
			makefields([]*TypeField{{"self", PrimitiveBytes}}),
			makefields([]*TypeField{{"result", PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				bigint := big.NewInt(0).SetBytes(self.(BytesValue))
				sign := bigint.Sign()
				res, err := uint256.FromBig(new(big.Int).Abs(bigint))
				if err || res.Gt(MaxI256.value) {
					return nil, exception(OverflowError, "I256 size overflow")
				}
				if sign == -1 {
					res = new(uint256.Int).Neg(res)
				}

				return RegisterSet{0: &I256Value{value: res}}, nil
			},
		),
	}
}

/*
AddressValue Implementation
*/

// AddressValue represents a RegisterValue that operates like a types.Address
type AddressValue [32]byte

// ZeroAddress represents the zero value of Address
var ZeroAddress AddressValue

// Type returns the Datatype of AddressValue, which is PrimitiveAddress.
// Implements the RegisterValue interface for AddressValue.
func (addr AddressValue) Type() Datatype { return PrimitiveAddress }

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

func (addr *AddressValue) SetBytes(b []byte) {
	if len(b) > 32 {
		b = b[len(b)-32:]
	}

	copy(addr[32-len(b):], b)
}

//nolint:forcetypeassert
func (addr AddressValue) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// address.__eq__(address) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveAddress, MethodEq, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}, {"other", PrimitiveAddress}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Check if both address values are equal
				equals := inputs[0].(AddressValue) == inputs[1].(AddressValue)
				// Return the result
				return RegisterSet{0: BoolValue(equals)}, nil
			},
		),

		// address.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveAddress, MethodBool, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}}),
			makefields([]*TypeField{{"result", PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except zero address
				result := inputs[0].(AddressValue) != ZeroAddress
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// address.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveAddress, MethodStr, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}}),
			makefields([]*TypeField{{"result", PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Hex encode the address
				result := inputs[0].(AddressValue).ToHex()
				// Set the result into the outputs
				return RegisterSet{0: result}, nil
			},
		),

		// address.__addr__() -> address
		MethodAddr: makeBuiltinMethod(
			MethodAddr.String(),
			PrimitiveAddress, MethodAddr, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}}),
			makefields([]*TypeField{{"result", PrimitiveAddress}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Return a copy of the address value
				return RegisterSet{0: inputs[0].Copy()}, nil
			},
		),

		// address.__len__() -> u64 (always 32)
		MethodLen: makeBuiltinMethod(
			MethodLen.String(),
			PrimitiveAddress, MethodLen, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}}),
			makefields([]*TypeField{{"length", PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				return RegisterSet{0: U64Value(32)}, nil
			},
		),

		// address.ToBytes() -> bytes
		0x10: makeBuiltinMethod(
			"ToBytes",
			PrimitiveAddress, 0x10, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}}),
			makefields([]*TypeField{{"result", PrimitiveBytes}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0]
				b := [32]byte(self.(AddressValue))

				return RegisterSet{0: BytesValue(b[:])}, nil
			},
		),

		// address.ToU256(address) -> U256
		0x11: makeBuiltinMethod("ToU256",
			PrimitiveAddress, 0x11, 10,
			makefields([]*TypeField{{"self", PrimitiveAddress}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				self := inputs[0].(AddressValue)
				byteval := self[:]
				res, err := uint256.FromBig(big.NewInt(0).SetBytes(byteval))
				if err {
					return nil, exception(OverflowError, "U256 size overflow")
				}

				return RegisterSet{0: &U256Value{value: res}}, nil
			},
		),
	}
}

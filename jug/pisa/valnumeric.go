package pisa

import (
	"encoding/binary"
	"math"
	"math/big"
	"strconv"

	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// U64Value represents a RegisterValue that operates like an uint64
type U64Value uint64

// Type returns the Datatype of U64Value, which is PrimitiveU64.
// Implements the RegisterValue interface for U64Value.
func (x U64Value) Type() Datatype { return PrimitiveU64 }

// Copy returns a copy of U64Value as a RegisterValue.
// Implements the RegisterValue interface for U64Value.
func (x U64Value) Copy() RegisterValue { return x }

// Norm returns the normalized value of U64Value as an uint64.
// Implements the RegisterValue interface for U64Value.
func (x U64Value) Norm() any { return uint64(x) }

// Data returns the POLO encoded bytes of U64Value.
// Implements the RegisterValue interface for U64Value.
func (x U64Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// I64 returns x as an I64Value.
// Returns an OverflowError if x overflows for 64-bit signed integer.
func (x U64Value) I64() (I64Value, *Exception) {
	if x > math.MaxInt64 {
		return 0, exception(OverflowError, "conversion overflow")
	}

	return I64Value(int64(x)), nil
}

// Add returns the value of x + y as a U64Value.
// Returns an OverflowError if the addition overflows.
func (x U64Value) Add(y U64Value) (U64Value, *Exception) {
	if z := x + y; z >= x {
		return z, nil
	}

	return 0, exception(OverflowError, "addition overflow")
}

// Sub returns the value of x - y as a U64Value.
// Returns an OverflowError if the subtraction overflows.
func (x U64Value) Sub(y U64Value) (U64Value, *Exception) {
	if z := x - y; z <= x {
		return z, nil
	}

	return 0, exception(OverflowError, "subtraction overflow")
}

// Mul returns the value of x * y as a U64Value.
// Returns an OverflowError if the multiplication overflows.
func (x U64Value) Mul(y U64Value) (U64Value, *Exception) {
	if x == 0 || y == 0 {
		return 0, nil
	}

	if z := x * y; z >= x {
		return z, nil
	}

	return 0, exception(OverflowError, "multiplication overflow")
}

// Div returns the value of x / y as a U64Value.
// Returns an DivideByZeroError if y is zero.
func (x U64Value) Div(y U64Value) (U64Value, *Exception) {
	if y == 0 {
		return 0, exception(DivideByZeroError, "division by zero")
	}

	return x / y, nil
}

// Mod returns the value of x % y as a U64Value.
// Returns an DivideByZeroError if y is zero.
func (x U64Value) Mod(y U64Value) (U64Value, *Exception) {
	if y == 0 {
		return 0, exception(DivideByZeroError, "modulo division by zero")
	}

	return x % y, nil
}

// Bxor returns  the value of x ^ y  as a U64Value
func (x U64Value) Bxor(y U64Value) U64Value {
	return x ^ y
}

// Band returns  the value of x & y  as a U64Value
func (x U64Value) Band(y U64Value) U64Value {
	return x & y
}

// Bor returns  the value of x | y  as a U64Value
func (x U64Value) Bor(y U64Value) U64Value {
	return x | y
}

// Bnot returns  the value of ^x as a U64Value
func (x U64Value) Bnot() U64Value {
	return ^x
}

func (x U64Value) Incr() (U64Value, *Exception) {
	if y := x + 1; y > x {
		return y, nil
	}

	return 0, exception(OverflowError, "increment overflow")
}

func (x U64Value) Decr() (U64Value, *Exception) {
	if y := x - 1; y < x {
		return y, nil
	}

	return 0, exception(OverflowError, "decrement overflow")
}

// Gt returns the value of x > y as a BoolValue
func (x U64Value) Gt(y U64Value) BoolValue {
	return x > y
}

// Lt returns the value of x < y as a BoolValue
func (x U64Value) Lt(y U64Value) BoolValue {
	return x < y
}

// Eq returns the value of x == y as a BoolValue
func (x U64Value) Eq(y U64Value) BoolValue {
	return x == y
}

//nolint:forcetypeassert
func (x U64Value) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// uint.__join__(uint) -> uint64
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveU64, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveU64}, {"other", PrimitiveU64}}),
			makefields([]*TypeField{{"result", PrimitiveU64}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform unsigned addition on the operands
				result, except := inputs[0].(U64Value).Add(inputs[1].(U64Value))
				// Check for overflow and raise exception
				if except != nil {
					return nil, except.traced(engine.callstack.trace())
				}

				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// uint.__lt__(uint) -> bool
		MethodLt: makeBuiltinMethod(
			MethodLt.String(),
			PrimitiveU64, MethodLt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}, {Name: "other", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Lt(y.(U64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__gt__(uint) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveU64, MethodGt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}, {Name: "other", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Gt(y.(U64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__eq__(uint) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveU64, MethodEq, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}, {Name: "other", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Eq(y.(U64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveU64, MethodBool, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := inputs[0].(U64Value) != 0
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// uint.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveU64, MethodStr, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				result := strconv.FormatUint(uint64(inputs[0].(U64Value)), 10)
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),

		// uint64.abs() -> uint64
		0x10: makeBuiltinMethod(
			"Abs",
			PrimitiveU64, 0x10, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]

				return RegisterSet{0: x}, nil
			},
		),

		// uint64.ToBytes() -> bytes
		0x11: makeBuiltinMethod(
			"ToBytes",
			PrimitiveU64, 0x11, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBytes}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res := make([]byte, 8)

				binary.BigEndian.PutUint64(res, uint64(x.(U64Value)))

				return RegisterSet{0: BytesValue(trimBytes(res, uint64(x.(U64Value))))}, nil
			},
		),

		// uint64.ToI64() -> I64
		0x12: makeBuiltinMethod(
			"ToI64",
			PrimitiveU64, 0x12, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]

				res, err := x.(U64Value).I64()
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: res}, nil
			},
		),

		// uint64.ToU256() -> U256
		0x13: makeBuiltinMethod(
			"ToU256",
			PrimitiveU64, 0x13, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]

				return RegisterSet{0: &U256Value{value: uint256.NewInt(uint64(x.(U64Value)))}}, nil
			},
		),

		// uint64.ToI256() -> I256
		0x14: makeBuiltinMethod(
			"ToI256",
			PrimitiveU64, 0x14, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]

				return RegisterSet{0: &I256Value{value: uint256.NewInt(uint64(x.(U64Value)))}}, nil
			},
		),
	}
}

/*
I64Value Implementation
*/

// I64Value represents RegisterValue that operates like an int64
type I64Value int64

// Type returns the Datatype of I64Value, which is PrimitiveI64.
// Implements the RegisterValue interface for I64Value.
func (x I64Value) Type() Datatype { return PrimitiveI64 }

// Copy returns a copy of I64Value as a RegisterValue.
// Implements the RegisterValue interface for I64Value.
func (x I64Value) Copy() RegisterValue { return x }

// Norm returns the normalized value of I64Value as an int64.
// Implements the RegisterValue interface for I64Value.
func (x I64Value) Norm() any { return int64(x) }

// Data returns the POLO encoded bytes of I64Value.
// Implements the RegisterValue interface for I64Value.
func (x I64Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// U64 returns x as an U64Value.
// Returns an OverflowError if x is less than 0
func (x I64Value) U64() (U64Value, *Exception) {
	if x < 0 {
		return 0, exception(OverflowError, "conversion overflow")
	}

	return U64Value(uint64(x)), nil
}

// Add returns the value of x + y as a I64Value.
// Returns an OverflowError if the addition overflows.
func (x I64Value) Add(y I64Value) (I64Value, *Exception) {
	if z := x + y; z >= x {
		return z, nil
	}

	return 0, exception(OverflowError, "addition overflow")
}

// Sub returns the value of x - y as a I64Value.
// Returns an OverflowError if the subtraction overflows.
func (x I64Value) Sub(y I64Value) (I64Value, *Exception) {
	if z := x - y; (z < x) == (y > 0) {
		return z, nil
	}

	return 0, exception(OverflowError, "subtraction overflow")
}

// Mul returns the value of x * y as a I64Value.
// Returns an OverflowError if the multiplication overflows.
func (x I64Value) Mul(y I64Value) (I64Value, *Exception) {
	if x == 0 || y == 0 {
		return 0, nil
	}

	if z := x * y; (z < 0) == ((x < 0) != (y < 0)) {
		if z/y == x {
			return z, nil
		}
	}

	return 0, exception(OverflowError, "multiplication overflow")
}

// Div returns the value of x / y as a I64Value.
// Returns an DivideByZeroError if y is zero or
// OverflowError if x is -1<<63 AND y is -1.
func (x I64Value) Div(y I64Value) (I64Value, *Exception) {
	if y == 0 {
		return 0, exception(DivideByZeroError, "division by zero")
	}

	if (x == math.MinInt64) && (y == -1) {
		return 0, exception(OverflowError, "division overflow")
	}

	return x / y, nil
}

// Mod returns the value of x % y as a I64Value.
// Returns an DivideByZeroError if y is zero or
// OverflowError if x is -1<<63 AND y is -1.
func (x I64Value) Mod(y I64Value) (I64Value, *Exception) {
	if y == 0 {
		return 0, exception(DivideByZeroError, "modulo division by zero")
	}

	if (x == math.MinInt64) && (y == -1) {
		return 0, exception(OverflowError, "modulo division overflow")
	}

	return x % y, nil
}

// Bxor returns  the value of x ^ y  as a I64Value
func (x I64Value) Bxor(y I64Value) I64Value {
	return x ^ y
}

// Band returns  the value of x ^ y  as a I64Value
func (x I64Value) Band(y I64Value) I64Value {
	return x & y
}

// Bor returns  the value of x | y  as a I64Value
func (x I64Value) Bor(y I64Value) I64Value {
	return x | y
}

// Bnot returns  the value of ^x  as a I64Value
func (x I64Value) Bnot() I64Value {
	return ^x
}

func (x I64Value) Incr() (I64Value, *Exception) {
	if y := x + 1; y > x {
		return y, nil
	}

	return 0, exception(OverflowError, "increment overflow")
}

func (x I64Value) Decr() (I64Value, *Exception) {
	if y := x - 1; y < x {
		return y, nil
	}

	return 0, exception(OverflowError, "decrement overflow")
}

// Gt returns the value of x > y as a BoolValue
func (x I64Value) Gt(y I64Value) BoolValue {
	return x > y
}

// Lt returns the value of x < y as a BoolValue
func (x I64Value) Lt(y I64Value) BoolValue {
	return x < y
}

// Eq returns the value of x == y as a BoolValue
func (x I64Value) Eq(y I64Value) BoolValue {
	return x == y
}

//nolint:forcetypeassert
func (x I64Value) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// int.__join__(int) -> int64
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveI64, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveI64}, {"other", PrimitiveI64}}),
			makefields([]*TypeField{{"result", PrimitiveI64}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform signed addition on the operands
				result, except := inputs[0].(I64Value).Add(inputs[1].(I64Value))
				// Check for overflow and raise exception
				if except != nil {
					return nil, except.traced(engine.callstack.trace())
				}

				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// int.__lt__(int) -> bool
		MethodLt: makeBuiltinMethod(
			MethodLt.String(),
			PrimitiveI64, MethodLt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}, {Name: "other", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Lt(y.(I64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__gt__(int) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveI64, MethodGt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}, {Name: "other", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Gt(y.(I64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__eq__(int) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveI64, MethodEq, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}, {Name: "other", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Eq(y.(I64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveI64, MethodBool, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := inputs[0].(I64Value) != 0
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// int.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveI64, MethodStr, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				result := strconv.FormatInt(int64(inputs[0].(I64Value)), 10)
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),

		// int64.abs() -> int64
		0x10: makeBuiltinMethod(
			"Abs",
			PrimitiveI64, 0x10, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				var result I64Value
				if x.(I64Value) < 0 {
					result = x.(I64Value) * -1
				} else {
					result = x.(I64Value)
				}

				return RegisterSet{0: result}, nil
			},
		),

		// int64.ToBytes() -> bytes
		0x11: makeBuiltinMethod(
			"ToBytes",
			PrimitiveI64, 0x11, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBytes}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res := make([]byte, 8)

				binary.BigEndian.PutUint64(res, uint64(x.(I64Value)))

				return RegisterSet{0: BytesValue(trimBytes(res, uint64(x.(I64Value))))}, nil
			},
		),

		// int64.ToU64() -> U64
		0x12: makeBuiltinMethod(
			"ToU64",
			PrimitiveI64, 0x12, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res, err := x.(I64Value).U64()
				if err != nil {
					return nil, err
				}

				return RegisterSet{0: res}, nil
			},
		),

		// int64.ToU256() -> U256
		0x13: makeBuiltinMethod(
			"ToU256",
			PrimitiveI64, 0x13, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				if x.(I64Value) < 0 {
					return nil, exception(OverflowError, "negative conversion to U256")
				}

				return RegisterSet{0: &U256Value{value: uint256.NewInt(uint64(x.(I64Value)))}}, nil
			},
		),

		// int64.ToI256() -> I256
		0x14: makeBuiltinMethod(
			"ToI256",
			PrimitiveI64, 0x14, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				sign := 0
				if x.(I64Value) < 0 {
					sign = -1
					x = x.(I64Value) * -1
				}

				res := uint256.NewInt(uint64(x.(I64Value)))
				if sign == -1 {
					res = new(uint256.Int).Neg(res)
				}

				return RegisterSet{0: &I256Value{value: res}}, nil
			},
		),
	}
}

/*
U256Value Implementation
*/

// U256Value represents a RegisterValue that operates like an uint256
type U256Value struct {
	value *uint256.Int
}

// Type returns the Datatype of U256Value, which is PrimitiveU256.
// Implements the RegisterValue interface for U256Value.
func (x *U256Value) Type() Datatype { return PrimitiveU256 }

// Copy returns a copy of U256Value as a RegisterValue.
// Implements the RegisterValue interface for U256Value.
func (x *U256Value) Copy() RegisterValue { return x }

// Norm returns the normalized value of U256Value as a big.Int.
// Implements the RegisterValue interface for U256Value.
func (x *U256Value) Norm() any {
	return x.value.ToBig()
}

// Data returns the POLO encoded bytes of U256Value.
// Implements the RegisterValue interface for U256Value.
func (x *U256Value) Data() []byte {
	// Polorize the U256 (will call the custom polorizer)
	data, _ := polo.Polorize(x)

	return data
}

// Polorize implements the Polorizable interface for U256Value.
// Serializes the array of 64-bit integers as a POLO BigInt instead of as a pack encoded wire.
func (x *U256Value) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeBigInt(x.value.ToBig())

	return polorizer, nil
}

// Depolorize implements the Depolorizable interface for U256Value.
// Deserialized the array of 64-bit integers from a POLO BigInt instead of as a pack encoded wire.
func (x *U256Value) Depolorize(depolorizer *polo.Depolorizer) error {
	bigint, err := depolorizer.DepolorizeBigInt()
	if err != nil {
		return err
	}

	u, overflow := uint256.FromBig(bigint)
	if overflow {
		return errors.New("overflow for 256-bit numeric")
	}

	*x = U256Value{u}

	return nil
}

// I256 returns an I256Value for a U256Value input
func (x *U256Value) I256() (*I256Value, *Exception) {
	if x.value.Lt(MaxI256.value) {
		return &I256Value{x.value}, nil
	}

	return nil, exception(OverflowError, "conversion overflow")
}

// Add returns the value of x + y as a U256Value.
// Returns an OverflowError if the addition overflows.
func (x *U256Value) Add(y *U256Value) (*U256Value, *Exception) {
	if result, overflow := new(uint256.Int).AddOverflow(x.value, y.value); !overflow {
		return &U256Value{result}, nil
	}

	return nil, exception(OverflowError, "addition overflow")
}

// Sub returns the value of x - y as a U256Value.
// Returns an OverflowError if the subtraction overflows.
func (x *U256Value) Sub(y *U256Value) (*U256Value, *Exception) {
	if result, overflow := new(uint256.Int).SubOverflow(x.value, y.value); !overflow {
		return &U256Value{result}, nil
	}

	return nil, exception(OverflowError, "subtraction overflow")
}

// Mul returns the value of x * y as a U256Value.
// Returns an OverflowError if the multiplication overflows.
func (x *U256Value) Mul(y *U256Value) (*U256Value, *Exception) {
	if result, overflow := new(uint256.Int).MulOverflow(x.value, y.value); !overflow {
		return &U256Value{result}, nil
	}

	return nil, exception(OverflowError, "multiplication overflow")
}

// Div returns the value of x / y as a U256Value.
// Returns an DivideByZeroError if y is zero.
func (x *U256Value) Div(y *U256Value) (*U256Value, *Exception) {
	if y.value.Eq(Zero256) {
		return nil, exception(DivideByZeroError, "division by zero")
	}

	result := new(uint256.Int).Div(x.value, y.value)

	return &U256Value{result}, nil
}

// Mod returns the value of x % y as a U256Value.
// Returns an DivideByZeroError if y is zero.
func (x *U256Value) Mod(y *U256Value) (*U256Value, *Exception) {
	if y.value.Eq(Zero256) {
		return nil, exception(DivideByZeroError, "modulo division by zero")
	}

	result := new(uint256.Int).Mod(x.value, y.value)

	return &U256Value{result}, nil
}

// Bxor returns  the value of x ^ y  as a U256Value
func (x *U256Value) Bxor(y *U256Value) *U256Value {
	result := new(uint256.Int).Xor(x.value, y.value)

	return &U256Value{result}
}

// Band returns  the value of x ^ y  as a U256Value
func (x *U256Value) Band(y *U256Value) *U256Value {
	result := new(uint256.Int).And(x.value, y.value)

	return &U256Value{result}
}

// Bor returns  the value of x | y  as a U256Value
func (x *U256Value) Bor(y *U256Value) *U256Value {
	result := new(uint256.Int).Or(x.value, y.value)

	return &U256Value{result}
}

// Bnot returns  the value of ^x  as a U256Value
func (x *U256Value) Bnot() *U256Value {
	result := new(uint256.Int).Not(x.value)

	return &U256Value{result}
}

func (x *U256Value) Incr() (*U256Value, *Exception) {
	if result, overflow := new(uint256.Int).AddOverflow(x.value, uint256.NewInt(1)); !overflow {
		return &U256Value{result}, nil
	}

	return nil, exception(OverflowError, "increment overflow")
}

func (x *U256Value) Decr() (*U256Value, *Exception) {
	if result, overflow := new(uint256.Int).SubOverflow(x.value, uint256.NewInt(1)); !overflow {
		return &U256Value{result}, nil
	}

	return nil, exception(OverflowError, "decrement overflow")
}

// Gt returns the value of x > y as a BoolValue
func (x *U256Value) Gt(y *U256Value) BoolValue {
	return BoolValue(x.value.Gt(y.value))
}

// Lt returns the value of x < y as a BoolValue
func (x *U256Value) Lt(y *U256Value) BoolValue {
	return BoolValue(x.value.Lt(y.value))
}

// Eq returns the value of x == y as a BoolValue
func (x *U256Value) Eq(y *U256Value) BoolValue {
	return BoolValue(x.value.Eq(y.value))
}

//nolint:forcetypeassert
func (x *U256Value) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// uint.__join__(uint) -> int256
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveU256, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveU256}, {"other", PrimitiveU256}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform unsigned addition on the operands
				result, except := inputs[0].(*U256Value).Add(inputs[1].(*U256Value))
				// Check for overflow and raise exception
				if except != nil {
					return nil, except.traced(engine.callstack.trace())
				}

				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// uint.__lt__(uint) -> bool
		MethodLt: makeBuiltinMethod(
			MethodLt.String(),
			PrimitiveU256, MethodLt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}, {Name: "other", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a, b := inputs[0], inputs[1]
				result := a.(*U256Value).Lt(b.(*U256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__gt__(uint) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveU256, MethodGt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}, {Name: "other", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a, b := inputs[0], inputs[1]
				result := a.(*U256Value).Gt(b.(*U256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__eq__(uint) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveU256, MethodEq, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}, {Name: "other", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a, b := inputs[0], inputs[1]
				result := a.(*U256Value).Eq(b.(*U256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveU256, MethodBool, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				isZero := inputs[0].(*U256Value).Eq(&U256Value{Zero256})
				// True for all values except 0
				return RegisterSet{0: !isZero}, nil
			},
		),

		// int.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveU256, MethodStr, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				result := inputs[0].(*U256Value).value.Dec()
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),

		// uint256.__addr__() -> address
		MethodAddr: makeBuiltinMethod(
			MethodAddr.String(),
			PrimitiveU256, MethodAddr, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveAddress}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res := x.(*U256Value).value.Bytes32()

				return RegisterSet{0: AddressValue(res)}, nil
			},
		),

		// uint256.abs() -> uint256
		0x10: makeBuiltinMethod(
			"Abs",
			PrimitiveU256, 0x10, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				return RegisterSet{0: inputs[0]}, nil
			},
		),

		// uint256.ToBytes() -> bytes
		0x11: makeBuiltinMethod(
			"ToBytes",
			PrimitiveU256, 0x11, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBytes}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res := x.(*U256Value).value.Bytes()

				return RegisterSet{0: BytesValue(res)}, nil
			},
		),

		// uint256.ToU64() -> U64
		0x12: makeBuiltinMethod(
			"ToU64",
			PrimitiveU256, 0x12, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res, overflow := x.(*U256Value).value.Uint64WithOverflow()
				if overflow {
					return nil, exception(OverflowError, "U64 overflow")
				}

				return RegisterSet{0: U64Value(res)}, nil
			},
		),

		// uint256.ToI64() -> I64
		0x13: makeBuiltinMethod(
			"ToI64",
			PrimitiveU256, 0x13, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]
				res, overflow := x.(*U256Value).value.Uint64WithOverflow()
				if overflow || res > math.MaxInt64 {
					return nil, exception(OverflowError, "I64 overflow")
				}

				return RegisterSet{0: I64Value(res)}, nil
			},
		),

		// uint256.ToI256() -> I256
		0x14: makeBuiltinMethod(
			"ToI256",
			PrimitiveU256, 0x14, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0].(*U256Value).value
				if x.Gt(MaxI256.value) {
					return nil, exception(OverflowError, "I256 overflow")
				}

				return RegisterSet{0: &I256Value{value: x}}, nil
			},
		),
	}
}

/*
I256Value Implementation
*/

var (
	Zero256 = uint256.NewInt(0)
	MinU256 = &U256Value{Zero256}
	MaxU256 = &U256Value{uint256.MustFromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")}
	MinI256 = &I256Value{uint256.MustFromHex("0x8000000000000000000000000000000000000000000000000000000000000000")}
	MaxI256 = &I256Value{uint256.MustFromHex("0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")}
)

// I256Value represents a RegisterValue that operates like an int256
type I256Value struct {
	value *uint256.Int
}

// Type returns the Datatype of I256Value, which is PrimitiveI256.
// Implements the RegisterValue interface for I256Value.
func (x *I256Value) Type() Datatype { return PrimitiveI256 }

// Copy returns a copy of I256Value as a RegisterValue.
// Implements the RegisterValue interface for I256Value.
func (x *I256Value) Copy() RegisterValue { return x }

// Norm returns the normalized value of I256Value as a big.Int.
// Implements the RegisterValue interface for I256Value.
func (x *I256Value) Norm() any {
	// Convert to big
	absX := uint256.NewInt(0).Abs(x.value)
	norm := absX.ToBig()
	// Flip sign if negative
	if x.value.Sign() == -1 {
		norm = new(big.Int).Neg(norm)
	}

	return norm
}

// Data returns the POLO encoded bytes of I256Value.
// Implements the RegisterValue interface for I256Value.
func (x *I256Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// Polorize implements the Polorizable interface for I256Value.
// Serializes the array of 64-bit integers as a POLO BigInt instead of as a pack encoded wire.
func (x *I256Value) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()

	value := big.NewInt(0)

	switch x.value.Sign() {
	case 1:
		value = x.value.ToBig()
	case -1:
		value = new(big.Int).Neg(new(uint256.Int).Abs(x.value).ToBig())
	}

	polorizer.PolorizeBigInt(value)

	return polorizer, nil
}

// Depolorize implements the Depolorizable interface for I256Value.
// Deserialized the array of 64-bit integers from a POLO BigInt instead of as a pack encoded wire.
func (x *I256Value) Depolorize(depolorizer *polo.Depolorizer) error {
	bigint, err := depolorizer.DepolorizeBigInt()
	if err != nil {
		return err
	}

	u, overflow := uint256.FromBig(bigint)
	if overflow {
		return errors.New("overflow for 256-bit numeric")
	}

	*x = I256Value{u}

	return nil
}

// U256 returns an U256Value for a I256Value input
func (x *I256Value) U256() (*U256Value, *Exception) {
	if x.value.Slt(Zero256) {
		return nil, exception(OverflowError, "conversion overflow")
	}

	return &U256Value{x.value}, nil
}

// Add returns the value of x + y as a I256Value.
// Returns an OverflowError if the addition overflows.
func (x *I256Value) Add(y *I256Value) (*I256Value, *Exception) {
	signX := x.value.Sign()
	signY := y.value.Sign()

	absX := new(uint256.Int).Abs(x.value)
	absY := new(uint256.Int).Abs(y.value)

	switch {
	// Checks if x is -ve and y is +ve
	case signX == -1 && signY >= 0:
		// If in y-x, x>y then it does -(x-y) else simply y-x
		if absX.Gt(absY) {
			if z, overflow := new(uint256.Int).SubOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{new(uint256.Int).Neg(z)}, nil
			}
		} else {
			if z, overflow := new(uint256.Int).SubOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{z}, nil
			}
		}

		return nil, exception(OverflowError, "addition overflow")

	// Checks if x is +ve and y is -ve
	case signX >= 0 && signY == -1:
		// If in x-y, y>x then it does -(y-x) else simply x-y
		if absY.Gt(absX) {
			if z, overflow := new(uint256.Int).SubOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{new(uint256.Int).Neg(z)}, nil
			}
		} else {
			if z, overflow := new(uint256.Int).SubOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{z}, nil
			}
		}

		return nil, exception(OverflowError, "addition overflow")

	// If both are neg => -x-y = -(x+y)
	case signX == -1 && signY == -1:
		if z, overflow := new(uint256.Int).AddOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{new(uint256.Int).Neg(z)}, nil
		}

		return nil, exception(OverflowError, "addition overflow")

	// If both are +ve => x+y
	default:
		if z, overflow := new(uint256.Int).AddOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{z}, nil
		}

		return nil, exception(OverflowError, "addition overflow")
	}
}

// Sub returns the value of x - y as a I256Value.
// Returns an OverflowError if the subtraction overflows.
func (x *I256Value) Sub(y *I256Value) (*I256Value, *Exception) {
	signX := x.value.Sign()
	signY := y.value.Sign()

	absX := new(uint256.Int).Abs(x.value)
	absY := new(uint256.Int).Abs(y.value)

	switch {
	// Checks if x is -ve and y is +ve
	case signX == -1 && signY >= 0:
		// Checks if x is -ve and y is +ve => -x-(+y) = -(x+y)
		if z, overflow := new(uint256.Int).AddOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{new(uint256.Int).Neg(z)}, nil
		}

		return nil, exception(OverflowError, "subtraction overflow")

	// Checks if x is +ve and y is -ve => x-(-y) = x+y
	case signX >= 0 && signY == -1:
		if z, overflow := new(uint256.Int).AddOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{z}, nil
		}

		return nil, exception(OverflowError, "subtraction overflow")

	// Checks if x is -ve and y is -ve => -x-(-y) = y-x
	case signX == -1 && signY == -1:
		// If in y-x, x>y then it does -(x-y) else simply y-x
		if absX.Gt(absY) {
			if z, overflow := new(uint256.Int).SubOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{new(uint256.Int).Neg(z)}, nil
			}
		} else {
			if z, overflow := new(uint256.Int).SubOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{z}, nil
			}
		}

		return nil, exception(OverflowError, "subtraction overflow")

	// If both are +ve => x-y
	default:
		// If in x-y, y>x then it does -(y-x) else simply x-y
		if absY.Gt(absX) {
			if z, overflow := new(uint256.Int).SubOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{new(uint256.Int).Neg(z)}, nil
			}
		} else {
			if z, overflow := new(uint256.Int).SubOverflow(absX, absY); !overflow && z.Lt(MaxI256.value) {
				return &I256Value{z}, nil
			}
		}

		return nil, exception(OverflowError, "subtraction overflow")
	}
}

// Mul returns the value of x * y as a I256Value.
// Returns an OverflowError if the multiplication overflows.
func (x *I256Value) Mul(y *I256Value) (*I256Value, *Exception) {
	signX := x.value.Sign()
	signY := y.value.Sign()

	absX := new(uint256.Int).Abs(x.value)
	absY := new(uint256.Int).Abs(y.value)

	switch {
	// Checks if any of the values are zero
	case signX == 0 || signY == 0:
		return &I256Value{uint256.NewInt(0)}, nil

	// Checks if x is -ve and y is +ve => -x-(+y) = -(x+y)
	case (signX == -1 && signY == 1) || (signX == 1 && signY == -1):
		if z, overflow := new(uint256.Int).MulOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{new(uint256.Int).Neg(z)}, nil
		}

		return nil, exception(OverflowError, "multiplication overflow")

	// Checks if x & y are +ve or x & y are -ve in both cases result is +ve
	default:
		if z, overflow := new(uint256.Int).MulOverflow(absY, absX); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{z}, nil
		}

		return nil, exception(OverflowError, "multiplication overflow")
	}
}

// Div returns the value of x / y as a I256Value.
// Returns an DivideByZeroError if y is zero.
func (x *I256Value) Div(y *I256Value) (*I256Value, *Exception) {
	if y.value.Sign() == 0 { // Checks if any of the values are 0
		return nil, exception(DivideByZeroError, "division by zero")
	}

	z := new(uint256.Int).SDiv(x.value, y.value)

	return &I256Value{z}, nil
}

// Mod returns the value of x % y as a I256Value.
// Returns an DivideByZeroError if y is zero.
func (x *I256Value) Mod(y *I256Value) (*I256Value, *Exception) {
	if y.value.Sign() == 0 { // Checks if any of the values are 0
		return nil, exception(DivideByZeroError, "modulo division by zero")
	}

	z := new(uint256.Int).SMod(x.value, y.value)

	return &I256Value{z}, nil
}

// Bxor returns  the value of x ^ y  as a I256Value
func (x *I256Value) Bxor(y *I256Value) *I256Value {
	result := new(uint256.Int).Xor(x.value, y.value)

	return &I256Value{result}
}

// Band returns  the value of x ^ y  as a I256Value
func (x *I256Value) Band(y *I256Value) *I256Value {
	result := new(uint256.Int).And(x.value, y.value)

	return &I256Value{result}
}

// Bor returns  the value of x | y  as a I256Value
func (x *I256Value) Bor(y *I256Value) *I256Value {
	result := new(uint256.Int).Or(x.value, y.value)

	return &I256Value{result}
}

// Bnot returns  the value of ^x  as a i256Value
func (x *I256Value) Bnot() *I256Value {
	result := new(uint256.Int).Not(x.value)

	return &I256Value{result}
}

func (x *I256Value) Incr() (*I256Value, *Exception) {
	absX := new(uint256.Int).Abs(x.value)

	if x.value.Sign() == -1 {
		if z, overflow := new(uint256.Int).SubOverflow(absX, uint256.NewInt(1)); !overflow && absX.Lt(MaxI256.value) {
			return &I256Value{new(uint256.Int).Neg(z)}, nil
		}

		return nil, exception(OverflowError, "increment overflow")
	}

	if z, overflow := new(uint256.Int).AddOverflow(absX, uint256.NewInt(1)); !overflow && absX.Lt(MaxI256.value) {
		return &I256Value{z}, nil
	}

	return nil, exception(OverflowError, "increment overflow")
}

func (x *I256Value) Decr() (*I256Value, *Exception) {
	signX := x.value.Sign()
	absX := new(uint256.Int).Abs(x.value)

	if signX == -1 || signX == 0 {
		if z, overflow := new(uint256.Int).AddOverflow(absX, uint256.NewInt(1)); !overflow && z.Lt(MaxI256.value) {
			return &I256Value{new(uint256.Int).Neg(z)}, nil
		}

		return nil, exception(OverflowError, "decrement overflow")
	}

	if z, overflow := new(uint256.Int).SubOverflow(absX, uint256.NewInt(1)); !overflow && z.Lt(MaxI256.value) {
		return &I256Value{z}, nil
	}

	return nil, exception(OverflowError, "decrement overflow")
}

// Gt returns the value of x > y as a BoolValue
func (x *I256Value) Gt(y *I256Value) BoolValue {
	return BoolValue(x.value.Sgt(y.value))
}

// Lt returns the value of x < y as a BoolValue
func (x *I256Value) Lt(y *I256Value) BoolValue {
	return BoolValue(x.value.Slt(y.value))
}

// Eq returns the value of x == y as a BoolValue
func (x *I256Value) Eq(y *I256Value) BoolValue {
	return BoolValue(x.value.Eq(y.value))
}

//nolint:forcetypeassert
func (x I256Value) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// int.__join__(int) -> int256
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveI256, MethodJoin, 20,
			makefields([]*TypeField{{"self", PrimitiveI256}, {"other", PrimitiveI256}}),
			makefields([]*TypeField{{"result", PrimitiveI256}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform signed addition on the operands
				result, except := inputs[0].(*I256Value).Add(inputs[1].(*I256Value))
				// Check for overflow and raise exception
				if except != nil {
					return nil, except.traced(engine.callstack.trace())
				}

				// Return the result
				return RegisterSet{0: result}, nil
			},
		),

		// int.__lt__(int) -> bool
		MethodLt: makeBuiltinMethod(
			MethodLt.String(),
			PrimitiveI256, MethodLt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}, {Name: "other", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a, b := inputs[0], inputs[1]
				result := a.(*I256Value).Lt(b.(*I256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__gt__(int) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveI256, MethodGt, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}, {Name: "other", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a, b := inputs[0], inputs[1]
				result := a.(*I256Value).Gt(b.(*I256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__eq__(int) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveI256, MethodEq, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}, {Name: "other", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a, b := inputs[0], inputs[1]
				result := a.(*I256Value).Eq(b.(*I256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveI256, MethodBool, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := !inputs[0].(*I256Value).Eq(&I256Value{Zero256})

				// Set value into outputs
				return RegisterSet{0: result}, nil
			},
		),

		// int.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveI256, MethodStr, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a := inputs[0].(*I256Value)
				// Obtain absolute value
				abs := new(uint256.Int).Abs(a.value)

				// Format into base10 string
				result := abs.Dec()
				// Prepend negative sign if negative
				if a.value.Sign() == -1 {
					result = string('-') + result
				}

				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),

		// int256.abs() -> int256
		0x10: makeBuiltinMethod(
			"Abs",
			PrimitiveI256, 0x10, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a := inputs[0].(*I256Value)
				// Obtain absolute value
				result := new(uint256.Int).Abs(a.value)

				return RegisterSet{0: &I256Value{result}}, nil
			},
		),

		// int256.ToBytes() -> bytes
		0x11: makeBuiltinMethod(
			"ToBytes",
			PrimitiveI256, 0x11, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBytes}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a := inputs[0].(*I256Value)
				// Obtain absolute value
				result := a.value.Bytes()

				return RegisterSet{0: BytesValue(result)}, nil
			},
		),

		// int256.ToU64() -> U64
		0x12: makeBuiltinMethod(
			"ToU64",
			PrimitiveI256, 0x12, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a := inputs[0].(*I256Value)
				sign := a.value.Sign()

				if sign == -1 || a.value.Gt(uint256.NewInt(math.MaxUint64)) {
					return nil, exception(OverflowError, "U64 overflow error")
				}

				result := a.value.Uint64()

				return RegisterSet{0: U64Value(result)}, nil
			},
		),

		// int256.ToI64() -> I64
		0x13: makeBuiltinMethod(
			"ToI64",
			PrimitiveI256, 0x13, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a := inputs[0].(*I256Value)
				sign := a.value.Sign()

				if new(uint256.Int).Abs(a.value).Gt(uint256.NewInt(math.MaxInt64)) {
					return nil, exception(OverflowError, "I64 overflow error")
				}

				result := int64(new(uint256.Int).Abs(a.value).Uint64())
				if sign == -1 {
					result *= -1
				}

				return RegisterSet{0: I64Value(result)}, nil
			},
		),

		// int256.ToU256() -> U256
		0x14: makeBuiltinMethod(
			"ToU256",
			PrimitiveI256, 0x14, 10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				a := inputs[0].(*I256Value)
				sign := a.value.Sign()

				if sign == -1 {
					return nil, exception(OverflowError, "U256 overflow error")
				}

				return RegisterSet{0: &U256Value{value: a.value}}, nil
			},
		),
	}
}

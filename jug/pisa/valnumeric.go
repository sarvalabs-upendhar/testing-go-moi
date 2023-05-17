package pisa

import (
	"math"
	"strconv"

	"github.com/holiman/uint256"
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
			PrimitiveU64, MethodJoin,
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
			PrimitiveU64, MethodLt,
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
			PrimitiveU64, MethodGt,
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
			PrimitiveU64, MethodEq,
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
			PrimitiveU64, MethodBool,
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
			PrimitiveU64, MethodStr,
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
			PrimitiveU64, 0x10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU64}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU64}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]

				return RegisterSet{0: x}, nil
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
			PrimitiveI64, MethodJoin,
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
			PrimitiveI64, MethodLt,
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
			PrimitiveI64, MethodGt,
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
			PrimitiveI64, MethodEq,
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
			PrimitiveI64, MethodBool,
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
			PrimitiveI64, MethodStr,
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
			PrimitiveI64, 0x10,
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
	}
}

/*
U256Value Implementation
*/

// U256Value represents a RegisterValue that operates like an uint256
type U256Value uint256.Int

// Type returns the Datatype of U256Value, which is PrimitiveU256.
// Implements the RegisterValue interface for U256Value.
func (x U256Value) Type() Datatype { return PrimitiveU256 }

// Copy returns a copy of U256Value as a RegisterValue.
// Implements the RegisterValue interface for U256Value.
func (x U256Value) Copy() RegisterValue { return x }

// Norm returns the normalized value of U256Value as an uint256.
// Implements the RegisterValue interface for U256Value.
func (x U256Value) Norm() any { return uint256.Int(x) }

// Data returns the POLO encoded bytes of U256Value.
// Implements the RegisterValue interface for U256Value.
func (x U256Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// I256 returns an I256Value for a U256Value input
func (x U256Value) I256() (I256Value, *Exception) {
	if (*uint256.Int)(&x).Lt(MAXI256) {
		return I256Value(x), nil
	}

	return I256Value(*uint256.NewInt(0)), exception(OverflowError, "conversion overflow")
}

// Add returns the value of x + y as a U256Value.
// Returns an OverflowError if the addition overflows.
func (x U256Value) Add(y U256Value) (U256Value, *Exception) {
	if res, overflow := (*uint256.Int)(&x).AddOverflow((*uint256.Int)(&x), (*uint256.Int)(&y)); !overflow {
		return U256Value(*res), nil
	}

	return U256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")
}

// Sub returns the value of x - y as a U256Value.
// Returns an OverflowError if the subtraction overflows.
func (x U256Value) Sub(y U256Value) (U256Value, *Exception) {
	if res, overflow := (*uint256.Int)(&x).SubOverflow((*uint256.Int)(&x), (*uint256.Int)(&y)); !overflow {
		return U256Value(*res), nil
	}

	return U256Value(*uint256.NewInt(0)), exception(OverflowError, "subtraction overflow")
}

// Mul returns the value of x * y as a U256Value.
// Returns an OverflowError if the multiplication overflows.
func (x U256Value) Mul(y U256Value) (U256Value, *Exception) {
	if res, overflow := (*uint256.Int)(&x).MulOverflow((*uint256.Int)(&x), (*uint256.Int)(&y)); !overflow {
		return U256Value(*res), nil
	}

	return U256Value(*uint256.NewInt(0)), exception(OverflowError, "multiplication overflow")
}

// Div returns the value of x / y as a U256Value.
// Returns an DivideByZeroError if y is zero.
func (x U256Value) Div(y U256Value) (U256Value, *Exception) {
	zcheck := uint256.Int(y)
	if zcheck.Eq(uint256.NewInt(0)) {
		return U256Value(*uint256.NewInt(0)), exception(DivideByZeroError, "division by zero")
	}

	res := (*uint256.Int)(&x).Div((*uint256.Int)(&x), (*uint256.Int)(&y))

	return U256Value(*res), nil
}

// Mod returns the value of x % y as a U256Value.
// Returns an DivideByZeroError if y is zero.
func (x U256Value) Mod(y U256Value) (U256Value, *Exception) {
	zcheck := uint256.Int(y)
	if zcheck.Eq(uint256.NewInt(0)) {
		return U256Value(*uint256.NewInt(0)), exception(DivideByZeroError, "modulo division by zero")
	}

	res := (*uint256.Int)(&x).Mod((*uint256.Int)(&x), (*uint256.Int)(&y))

	return U256Value(*res), nil
}

// Bxor returns  the value of x ^ y  as a U256Value
func (x U256Value) Bxor(y U256Value) U256Value {
	res := (*uint256.Int)(&x).Xor((*uint256.Int)(&x), (*uint256.Int)(&y))

	return U256Value(*res)
}

// Band returns  the value of x ^ y  as a U256Value
func (x U256Value) Band(y U256Value) U256Value {
	res := (*uint256.Int)(&x).And((*uint256.Int)(&x), (*uint256.Int)(&y))

	return U256Value(*res)
}

// Bor returns  the value of x | y  as a U256Value
func (x U256Value) Bor(y U256Value) U256Value {
	res := (*uint256.Int)(&x).Or((*uint256.Int)(&x), (*uint256.Int)(&y))

	return U256Value(*res)
}

// Bnot returns  the value of ^x  as a U256Value
func (x U256Value) Bnot() U256Value {
	res := (*uint256.Int)(&x).Not((*uint256.Int)(&x))

	return U256Value(*res)
}

func (x U256Value) Incr() (U256Value, *Exception) {
	if res, overflow := (*uint256.Int)(&x).AddOverflow((*uint256.Int)(&x), uint256.NewInt(1)); !overflow {
		return U256Value(*res), nil
	}

	return U256Value(*uint256.NewInt(0)), exception(OverflowError, "increment overflow")
}

func (x U256Value) Decr() (U256Value, *Exception) {
	if res, overflow := (*uint256.Int)(&x).SubOverflow((*uint256.Int)(&x), uint256.NewInt(1)); !overflow {
		return U256Value(*res), nil
	}

	return U256Value(*uint256.NewInt(0)), exception(OverflowError, "decrement overflow")
}

// Gt returns the value of x > y as a BoolValue
func (x U256Value) Gt(y U256Value) BoolValue {
	return BoolValue((*uint256.Int)(&x).Gt((*uint256.Int)(&y)))
}

// Lt returns the value of x < y as a BoolValue
func (x U256Value) Lt(y U256Value) BoolValue {
	return BoolValue((*uint256.Int)(&x).Lt((*uint256.Int)(&y)))
}

// Eq returns the value of x == y as a BoolValue
func (x U256Value) Eq(y U256Value) BoolValue {
	return BoolValue((*uint256.Int)(&x).Eq((*uint256.Int)(&y)))
}

//nolint:forcetypeassert
func (x U256Value) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// uint.__join__(uint) -> int256
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveU256, MethodJoin,
			makefields([]*TypeField{{"self", PrimitiveU256}, {"other", PrimitiveU256}}),
			makefields([]*TypeField{{"result", PrimitiveU256}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform unsigned addition on the operands
				result, except := inputs[0].(U256Value).Add(inputs[1].(U256Value))
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
			PrimitiveU256, MethodLt,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}, {Name: "other", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U256Value).Lt(y.(U256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__gt__(uint) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveU256, MethodGt,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}, {Name: "other", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U256Value).Gt(y.(U256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__eq__(uint) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveU256, MethodEq,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}, {Name: "other", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U256Value).Eq(y.(U256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveU256, MethodBool,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := !inputs[0].(U256Value).Eq(U256Value(*uint256.NewInt(0)))
				// Set value into outputs
				return RegisterSet{0: result}, nil
			},
		),

		// int.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveU256, MethodStr,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				u := (uint256.Int)(inputs[0].(U256Value))
				result := u.Dec()
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),

		// uint256.abs() -> uint256
		0x10: makeBuiltinMethod(
			"Abs",
			PrimitiveU256, 0x10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveU256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveU256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0]

				return RegisterSet{0: x}, nil
			},
		),
	}
}

/*
I256Value Implementation
*/

var MAXI256, _ = uint256.FromHex("0x8000000000000000000000000000000000000000000000000000000000000000")

// I256Value represents a RegisterValue that operates like an int256
type I256Value uint256.Int

// Type returns the Datatype of I256Value, which is PrimitiveI256.
// Implements the RegisterValue interface for I256Value.
func (x I256Value) Type() Datatype { return PrimitiveI256 }

// Copy returns a copy of I256Value as a RegisterValue.
// Implements the RegisterValue interface for I256Value.
func (x I256Value) Copy() RegisterValue { return x }

// Norm returns the normalized value of I256Value as an int256.
// Implements the RegisterValue interface for I256Value.
func (x I256Value) Norm() any { return uint256.Int(x) }

// Data returns the POLO encoded bytes of I256Value.
// Implements the RegisterValue interface for I256Value.
func (x I256Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// U256 returns an U256Value for a I256Value input
func (x I256Value) U256() (U256Value, *Exception) {
	if (*uint256.Int)(&x).Slt(uint256.NewInt(0)) {
		return U256Value(*uint256.NewInt(0)), exception(OverflowError, "conversion overflow")
	}

	return U256Value(x), nil
}

// Add returns the value of x + y as a I256Value.
// Returns an OverflowError if the addition overflows.
func (x I256Value) Add(y I256Value) (I256Value, *Exception) {
	signX := (*uint256.Int)(&x).Sign()
	signY := (*uint256.Int)(&y).Sign()
	xabs := (*uint256.Int)(&x).Abs((*uint256.Int)(&x))
	yabs := (*uint256.Int)(&y).Abs((*uint256.Int)(&y))

	switch {
	case signX == -1 && signY >= 0:
		{ // Checks if x is -ve and y is +ve
			// If in y-x, x>y then it does -(x-y) else simply y-x
			if xabs.Gt(yabs) {
				if z, overflow := xabs.SubOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z.Neg(z)), nil
				}
			} else {
				if z, overflow := xabs.SubOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z), nil
				}
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")
		}
	case signX >= 0 && signY == -1:
		{ // Checks if x is +ve and y is -ve
			// If in x-y, y>x then it does -(y-x) else simply x-y
			if yabs.Gt(xabs) {
				if z, overflow := xabs.SubOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z.Neg(z)), nil
				}
			} else {
				if z, overflow := xabs.SubOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z), nil
				}
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")
		}
	case signX == -1 && signY == -1:
		{
			// If both are neg => -x-y = -(x+y)
			if z, overflow := xabs.AddOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
				return I256Value(*z.Neg(z)), nil
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")
		}
	default:
		{
			// If both are +ve => x+y
			if z, overflow := xabs.AddOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
				return I256Value(*z), nil
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "addition overflow")
		}
	}
}

// Sub returns the value of x - y as a I256Value.
// Returns an OverflowError if the subtraction overflows.
func (x I256Value) Sub(y I256Value) (I256Value, *Exception) {
	signX := (*uint256.Int)(&x).Sign()
	signY := (*uint256.Int)(&y).Sign()
	xabs := (*uint256.Int)(&x).Abs((*uint256.Int)(&x))
	yabs := (*uint256.Int)(&y).Abs((*uint256.Int)(&y))

	switch {
	case signX == -1 && signY >= 0:
		{ // Checks if x is -ve and y is +ve
			// Checks if x is -ve and y is +ve => -x-(+y) = -(x+y)
			if z, overflow := xabs.AddOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
				return I256Value(*z.Neg(z)), nil
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "subtraction overflow")
		}
	case signX >= 0 && signY == -1:
		{ // Checks if x is +ve and y is -ve => x-(-y) = x+y
			if z, overflow := xabs.AddOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
				return I256Value(*z), nil
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "subtraction overflow")
		}
	case signX == -1 && signY == -1:
		{ // Checks if x is -ve and y is -ve => -x-(-y) = y-x
			// If in y-x, x>y then it does -(x-y) else simply y-x
			if xabs.Gt(yabs) {
				if z, overflow := xabs.SubOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z.Neg(z)), nil
				}
			} else {
				if z, overflow := xabs.SubOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z), nil
				}
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "subtraction overflow")
		}
	default:
		{ // If both are +ve => x-y
			// If in x-y, y>x then it does -(y-x) else simply x-y
			if yabs.Gt(xabs) {
				if z, overflow := xabs.SubOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z.Neg(z)), nil
				}
			} else {
				if z, overflow := xabs.SubOverflow(xabs, yabs); !overflow && z.Lt(MAXI256) {
					return I256Value(*z), nil
				}
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "subtraction overflow")
		}
	}
}

// Mul returns the value of x * y as a I256Value.
// Returns an OverflowError if the multiplication overflows.
func (x I256Value) Mul(y I256Value) (I256Value, *Exception) {
	signX := (*uint256.Int)(&x).Sign()
	signY := (*uint256.Int)(&y).Sign()
	xabs := (*uint256.Int)(&x).Abs((*uint256.Int)(&x))
	yabs := (*uint256.Int)(&y).Abs((*uint256.Int)(&y))

	switch {
	case signX == 0 || signY == 0:
		{ // Checks if any of the values are zero
			return I256Value(*uint256.NewInt(0)), nil
		}
	case (signX == -1 && signY == 1) || (signX == 1 && signY == -1):
		{ // Checks if x is -ve and y is +ve => -x-(+y) = -(x+y)
			if z, overflow := xabs.MulOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
				return I256Value(*z.Neg(z)), nil
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "multiplication overflow")
		}
	default:
		{ // Checks if x & y are +ve or x & y are -ve in both cases result is +ve
			if z, overflow := xabs.MulOverflow(yabs, xabs); !overflow && z.Lt(MAXI256) {
				return I256Value(*z), nil
			}

			return I256Value(*uint256.NewInt(0)), exception(OverflowError, "multiplication overflow")
		}
	}
}

// Div returns the value of x / y as a I256Value.
// Returns an DivideByZeroError if y is zero.
func (x I256Value) Div(y I256Value) (I256Value, *Exception) {
	signY := (*uint256.Int)(&y).Sign()
	if signY == 0 { // Checks if any of the values are 0
		return I256Value(*uint256.NewInt(0)), exception(DivideByZeroError, "division by zero")
	} else { // Checks if both values have the same sign
		z := (*uint256.Int)(&x).SDiv((*uint256.Int)(&x), (*uint256.Int)(&y))

		return I256Value(*z), nil
	}
}

// Mod returns the value of x % y as a I256Value.
// Returns an DivideByZeroError if y is zero.
func (x I256Value) Mod(y I256Value) (I256Value, *Exception) {
	signY := (*uint256.Int)(&y).Sign()
	if signY == 0 { // Checks if any of the values are 0
		return I256Value(*uint256.NewInt(0)), exception(DivideByZeroError, "modulo division by zero")
	} else { // Checks if both values have the same sign
		z := (*uint256.Int)(&x).SMod((*uint256.Int)(&x), (*uint256.Int)(&y))

		return I256Value(*z), nil
	}
}

// Bxor returns  the value of x ^ y  as a I256Value
func (x I256Value) Bxor(y I256Value) I256Value {
	res := (*uint256.Int)(&x).Xor((*uint256.Int)(&x), (*uint256.Int)(&y))

	return I256Value(*res)
}

// Band returns  the value of x ^ y  as a I256Value
func (x I256Value) Band(y I256Value) I256Value {
	res := (*uint256.Int)(&x).And((*uint256.Int)(&x), (*uint256.Int)(&y))

	return I256Value(*res)
}

// Bor returns  the value of x | y  as a I256Value
func (x I256Value) Bor(y I256Value) I256Value {
	res := (*uint256.Int)(&x).Or((*uint256.Int)(&x), (*uint256.Int)(&y))

	return I256Value(*res)
}

// Bnot returns  the value of ^x  as a i256Value
func (x I256Value) Bnot() I256Value {
	res := (*uint256.Int)(&x).Not((*uint256.Int)(&x))

	return I256Value(*res)
}

func (x I256Value) Incr() (I256Value, *Exception) {
	signX := (*uint256.Int)(&x).Sign()
	xabs := (*uint256.Int)(&x).Abs((*uint256.Int)(&x))

	if signX == -1 {
		if z, overflow := xabs.SubOverflow(xabs, uint256.NewInt(1)); !overflow && xabs.Lt(MAXI256) {
			return I256Value(*z.Neg(z)), nil
		}

		return I256Value(*uint256.NewInt(0)), exception(OverflowError, "increment overflow")
	} else {
		if z, overflow := xabs.AddOverflow(xabs, uint256.NewInt(1)); !overflow && xabs.Lt(MAXI256) {
			return I256Value(*z), nil
		}

		return I256Value(*uint256.NewInt(0)), exception(OverflowError, "increment overflow")
	}
}

func (x I256Value) Decr() (I256Value, *Exception) {
	signX := (*uint256.Int)(&x).Sign()
	xabs := (*uint256.Int)(&x).Abs((*uint256.Int)(&x))

	if signX == -1 || signX == 0 {
		if z, overflow := xabs.AddOverflow(xabs, uint256.NewInt(1)); !overflow && z.Lt(MAXI256) {
			return I256Value(*z.Neg(z)), nil
		}

		return I256Value(*uint256.NewInt(0)), exception(OverflowError, "decrement overflow")
	} else {
		if z, overflow := xabs.SubOverflow(xabs, uint256.NewInt(1)); !overflow && z.Lt(MAXI256) {
			return I256Value(*z), nil
		}

		return I256Value(*uint256.NewInt(0)), exception(OverflowError, "decrement overflow")
	}
}

// Gt returns the value of x > y as a BoolValue
func (x I256Value) Gt(y I256Value) BoolValue {
	return BoolValue((*uint256.Int)(&x).Sgt((*uint256.Int)(&y)))
}

// Lt returns the value of x < y as a BoolValue
func (x I256Value) Lt(y I256Value) BoolValue {
	return BoolValue((*uint256.Int)(&x).Slt((*uint256.Int)(&y)))
}

// Eq returns the value of x == y as a BoolValue
func (x I256Value) Eq(y I256Value) BoolValue {
	return BoolValue((*uint256.Int)(&x).Eq((*uint256.Int)(&y)))
}

//nolint:forcetypeassert
func (x I256Value) methods() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// int.__join__(int) -> int256
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveI256, MethodJoin,
			makefields([]*TypeField{{"self", PrimitiveI256}, {"other", PrimitiveI256}}),
			makefields([]*TypeField{{"result", PrimitiveI256}}),
			func(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Perform signed addition on the operands
				result, except := inputs[0].(I256Value).Add(inputs[1].(I256Value))
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
			PrimitiveI256, MethodLt,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}, {Name: "other", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I256Value).Lt(y.(I256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__gt__(int) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveI256, MethodGt,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}, {Name: "other", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I256Value).Gt(y.(I256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__eq__(int) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveI256, MethodEq,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}, {Name: "other", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I256Value).Eq(y.(I256Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveI256, MethodBool,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := !inputs[0].(I256Value).Eq(I256Value(*uint256.NewInt(0)))
				// Set value into outputs
				return RegisterSet{0: result}, nil
			},
		),

		// int.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveI256, MethodStr,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				x := (uint256.Int)(inputs[0].(I256Value))
				signx := x.Sign()
				uabs := x.Abs(&x)
				result := uabs.Dec()
				if signx == -1 {
					result = string('-') + result
				}
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),

		// int256.abs() -> int256
		0x10: makeBuiltinMethod(
			"Abs",
			PrimitiveI256, 0x10,
			makefields([]*TypeField{{Name: "self", Type: PrimitiveI256}}),
			makefields([]*TypeField{{Name: "result", Type: PrimitiveI256}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x := inputs[0].(I256Value)
				result := (*uint256.Int)(&x).Abs((*uint256.Int)(&x))

				return RegisterSet{0: I256Value(*result)}, nil
			},
		),
	}
}

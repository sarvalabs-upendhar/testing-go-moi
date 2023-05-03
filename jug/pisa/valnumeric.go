package pisa

import (
	"math"
	"strconv"

	"github.com/sarvalabs/go-polo"
)

// U64Value represents a RegisterValue that operates like an uint64
type U64Value uint64

// Type returns the Datatype of U64Value, which is TypeU64.
// Implements the RegisterValue interface for U64Value.
func (x U64Value) Type() *Datatype { return TypeU64 }

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

//nolint:forcetypeassert, dupl
func methodsU64() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// uint64.__join__(uint64) -> uint64
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveU64, MethodJoin,
			makefields([]*TypeField{{"self", TypeU64}, {"other", TypeU64}}),
			makefields([]*TypeField{{"result", TypeU64}}),
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

		// uint64.__lt__(int64) -> bool
		MethodLt: makeBuiltinMethod(
			MethodLt.String(),
			PrimitiveU64, MethodLt,
			makefields([]*TypeField{{Name: "x", Type: TypeU64}, {Name: "y", Type: TypeU64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Lt(y.(U64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint64.__gt__(int64) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveU64, MethodGt,
			makefields([]*TypeField{{Name: "x", Type: TypeU64}, {Name: "y", Type: TypeU64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Gt(y.(U64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint64.__eq__(int64) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveU64, MethodEq,
			makefields([]*TypeField{{Name: "x", Type: TypeU64}, {Name: "y", Type: TypeU64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Eq(y.(U64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// uint64.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveU64, MethodBool,
			makefields([]*TypeField{{Name: "self", Type: TypeU64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := inputs[0].(U64Value) != 0
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// uint64.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveU64, MethodStr,
			makefields([]*TypeField{{Name: "self", Type: TypeU64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				result := strconv.FormatUint(uint64(inputs[0].(U64Value)), 10)
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),
	}
}

/*
I64Value Implementation
*/

// I64Value represents RegisterValue that operates like an int64
type I64Value int64

// Type returns the Datatype of I64Value, which is TypeI64.
// Implements the RegisterValue interface for I64Value.
func (x I64Value) Type() *Datatype { return TypeI64 }

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

//nolint:forcetypeassert, dupl
func methodsI64() [256]*BuiltinMethod {
	return [256]*BuiltinMethod{
		// int64.__join__(int64) -> int64
		MethodJoin: makeBuiltinMethod(
			MethodJoin.String(),
			PrimitiveI64, MethodJoin,
			makefields([]*TypeField{{"self", TypeI64}, {"other", TypeI64}}),
			makefields([]*TypeField{{"result", TypeI64}}),
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

		// int64.__lt__(int64) -> bool
		MethodLt: makeBuiltinMethod(
			MethodLt.String(),
			PrimitiveI64, MethodLt,
			makefields([]*TypeField{{Name: "x", Type: TypeI64}, {Name: "y", Type: TypeI64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Lt(y.(I64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int64.__gt__(int64) -> bool
		MethodGt: makeBuiltinMethod(
			MethodGt.String(),
			PrimitiveI64, MethodGt,
			makefields([]*TypeField{{Name: "x", Type: TypeI64}, {Name: "y", Type: TypeI64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Gt(y.(I64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int64.__eq__(int64) -> bool
		MethodEq: makeBuiltinMethod(
			MethodEq.String(),
			PrimitiveI64, MethodEq,
			makefields([]*TypeField{{Name: "x", Type: TypeI64}, {Name: "y", Type: TypeI64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Eq(y.(I64Value))

				return RegisterSet{0: result}, nil
			},
		),

		// int64.__bool__() -> bool
		MethodBool: makeBuiltinMethod(
			MethodBool.String(),
			PrimitiveI64, MethodBool,
			makefields([]*TypeField{{Name: "self", Type: TypeI64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeBool}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// True for all values except 0
				result := inputs[0].(I64Value) != 0
				// Set value into outputs
				return RegisterSet{0: BoolValue(result)}, nil
			},
		),

		// int64.__str__() -> string
		MethodStr: makeBuiltinMethod(
			MethodStr.String(),
			PrimitiveI64, MethodStr,
			makefields([]*TypeField{{Name: "self", Type: TypeI64}}),
			makefields([]*TypeField{{Name: "result", Type: TypeString}}),
			func(_ *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
				// Format into a string (base 10)
				result := strconv.FormatInt(int64(inputs[0].(I64Value)), 10)
				// Set value into outputs
				return RegisterSet{0: StringValue(result)}, nil
			},
		),
	}
}

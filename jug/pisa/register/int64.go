package register

import (
	"math"
	"strconv"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

// I64Value represents Value that operates like an int64
type I64Value int64

// Type returns the Typedef of I64Value, which is TypeI64.
// Implements the Value interface for I64Value.
func (x I64Value) Type() *engineio.Datatype { return engineio.TypeI64 }

// Copy returns a copy of I64Value as a Value.
// Implements the Value interface for I64Value.
func (x I64Value) Copy() Value { return x }

// Norm returns the normalized value of I64Value as an int64.
// Implements the Value interface for I64Value.
func (x I64Value) Norm() any { return int64(x) }

// Data returns the POLO encoded bytes of I64Value.
// Implements the Value interface for I64Value.
func (x I64Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// U64 returns x as an U64Value.
// Returns an error if x is less than 0
func (x I64Value) U64() (U64Value, error) {
	if x < 0 {
		return 0, ErrIntegerOverflow
	}

	return U64Value(uint64(x)), nil
}

// Add returns the value of x + y as a I64Value.
// Returns an ErrIntegerOverflow if the addition overflows.
func (x I64Value) Add(y I64Value) (I64Value, error) {
	if z := x + y; (z > x) == (y > 0) {
		return z, nil
	}

	return 0, ErrIntegerOverflow
}

// Sub returns the value of x - y as a I64Value.
// Returns an ErrIntegerOverflow if the subtraction overflows.
func (x I64Value) Sub(y I64Value) (I64Value, error) {
	if z := x - y; (z < x) == (y > 0) {
		return z, nil
	}

	return 0, ErrIntegerOverflow
}

// Mul returns the value of x * y as a I64Value.
// Returns an ErrIntegerOverflow if the multiplication overflows.
func (x I64Value) Mul(y I64Value) (I64Value, error) {
	if x == 0 || y == 0 {
		return 0, nil
	}

	if z := x * y; (z < 0) == ((x < 0) != (y < 0)) {
		if z/y == x {
			return z, nil
		}
	}

	return 0, ErrIntegerOverflow
}

// Div returns the value of x / y as a I64Value.
// Returns an ErrDivideByZero if y is zero or
// ErrIntegerOverflow if x is -1<<63 AND y is -1.
func (x I64Value) Div(y I64Value) (I64Value, error) {
	if y == 0 {
		return 0, ErrDivideByZero
	}

	if (x == math.MinInt64) && (y == -1) {
		return 0, ErrIntegerOverflow
	}

	return x / y, nil
}

// Mod returns the value of x % y as a I64Value.
// Returns an ErrDivideByZero if y is zero or
// ErrIntegerOverflow if x is -1<<63 AND y is -1.
func (x I64Value) Mod(y I64Value) (I64Value, error) {
	if y == 0 {
		return 0, ErrDivideByZero
	}

	if (x == math.MinInt64) && (y == -1) {
		return 0, ErrIntegerOverflow
	}

	return x % y, nil
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

//nolint:dupl, lll
func I64Methods() MethodTable {
	return MethodTable{
		// int64.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: engineio.PrimitiveI64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeI64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// True for all values except 0
				result := inputs[0].(I64Value) != 0 //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// int64.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: engineio.PrimitiveI64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeI64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Format into a string (base 10)
				result := strconv.FormatInt(int64(inputs[0].(I64Value)), 10) //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: StringValue(result)}, nil
			},
		},

		// int64.__lt__(int64) -> bool
		MethodLt: &BuiltinMethod{
			datatype: engineio.PrimitiveI64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "x", Type: engineio.TypeI64}, {Name: "y", Type: engineio.TypeI64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Lt(y.(I64Value)) //nolint:forcetypeassert

				return ValueTable{0: result}, nil
			},
		},

		// int64.__gt__(int64) -> bool
		MethodGt: &BuiltinMethod{
			datatype: engineio.PrimitiveI64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "x", Type: engineio.TypeI64}, {Name: "y", Type: engineio.TypeI64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Gt(y.(I64Value)) //nolint:forcetypeassert

				return ValueTable{0: result}, nil
			},
		},

		// int64.__eq__(int64) -> bool
		MethodEq: &BuiltinMethod{
			datatype: engineio.PrimitiveI64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "x", Type: engineio.TypeI64}, {Name: "y", Type: engineio.TypeI64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				x, y := inputs[0], inputs[1]
				result := x.(I64Value).Eq(y.(I64Value)) //nolint:forcetypeassert

				return ValueTable{0: result}, nil
			},
		},
	}
}

package register

import (
	"math"
	"strconv"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

// U64Value represents a Value that operates like an uint64
type U64Value uint64

// Type returns the Typedef of U64Value, which is TypeU64.
// Implements the Value interface for U64Value.
func (x U64Value) Type() *engineio.Datatype { return engineio.TypeU64 }

// Copy returns a copy of U64Value as a Value.
// Implements the Value interface for U64Value.
func (x U64Value) Copy() Value { return x }

// Norm returns the normalized value of U64Value as an uint64.
// Implements the Value interface for U64Value.
func (x U64Value) Norm() any { return uint64(x) }

// Data returns the POLO encoded bytes of U64Value.
// Implements the Value interface for U64Value.
func (x U64Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

// I64 returns x as an I64Value.
// Returns an error if x overflows for 64-bit signed integer.
func (x U64Value) I64() (I64Value, error) {
	if x > math.MaxInt64 {
		return 0, ErrIntegerOverflow
	}

	return I64Value(int64(x)), nil
}

// Add returns the value of x + y as a U64Value.
// Returns an ErrIntegerOverflow if the addition overflows.
func (x U64Value) Add(y U64Value) (U64Value, error) {
	if z := x + y; z >= x {
		return z, nil
	}

	return 0, ErrIntegerOverflow
}

// Sub returns the value of x - y as a U64Value.
// Returns an ErrIntegerOverflow if the subtraction overflows.
func (x U64Value) Sub(y U64Value) (U64Value, error) {
	if z := x - y; z <= x {
		return z, nil
	}

	return 0, ErrIntegerOverflow
}

// Mul returns the value of x * y as a U64Value.
// Returns an ErrIntegerOverflow if the multiplication overflows.
func (x U64Value) Mul(y U64Value) (U64Value, error) {
	if x == 0 || y == 0 {
		return 0, nil
	}

	if z := x * y; z >= x {
		return z, nil
	}

	return 0, ErrIntegerOverflow
}

// Div returns the value of x / y as a U64Value.
// Returns an ErrDivideByZero if y is zero.
func (x U64Value) Div(y U64Value) (U64Value, error) {
	if y == 0 {
		return 0, ErrDivideByZero
	}

	return x / y, nil
}

// Mod returns the value of x % y as a U64Value.
// Returns an ErrDivideByZero if y is zero.
func (x U64Value) Mod(y U64Value) (U64Value, error) {
	if y == 0 {
		return 0, ErrDivideByZero
	}

	return x % y, nil
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

//nolint:dupl, lll
func U64Methods() MethodTable {
	return MethodTable{
		// uint64.__bool__() -> bool
		MethodBool: &BuiltinMethod{
			datatype: engineio.PrimitiveU64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeU64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// True for all values except 0
				result := inputs[0].(U64Value) != 0 //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: BoolValue(result)}, nil
			},
		},

		// uint64.__str__() -> string
		MethodStr: &BuiltinMethod{
			datatype: engineio.PrimitiveU64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "self", Type: engineio.TypeU64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeString}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				// Format into a string (base 10)
				result := strconv.FormatUint(uint64(inputs[0].(U64Value)), 10) //nolint:forcetypeassert
				// Set value into outputs
				return ValueTable{0: StringValue(result)}, nil
			},
		},

		// uint64.__lt__(int64) -> bool
		MethodLt: &BuiltinMethod{
			datatype: engineio.PrimitiveU64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "x", Type: engineio.TypeU64}, {Name: "y", Type: engineio.TypeU64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Lt(y.(U64Value)) //nolint:forcetypeassert

				return ValueTable{0: result}, nil
			},
		},

		// uint64.__gt__(int64) -> bool
		MethodGt: &BuiltinMethod{
			datatype: engineio.PrimitiveU64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "x", Type: engineio.TypeU64}, {Name: "y", Type: engineio.TypeU64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Gt(y.(U64Value)) //nolint:forcetypeassert

				return ValueTable{0: result}, nil
			},
		},

		// uint64.__eq__(int64) -> bool
		MethodEq: &BuiltinMethod{
			datatype: engineio.PrimitiveU64,
			fields: engineio.CallFields{
				Inputs:  makefields([]*engineio.TypeField{{Name: "x", Type: engineio.TypeU64}, {Name: "y", Type: engineio.TypeU64}}),
				Outputs: makefields([]*engineio.TypeField{{Name: "result", Type: engineio.TypeBool}}),
			},
			execute: func(inputs ValueTable) (ValueTable, *exception.Object) {
				x, y := inputs[0], inputs[1]
				result := x.(U64Value).Eq(y.(U64Value)) //nolint:forcetypeassert

				return ValueTable{0: result}, nil
			},
		},
	}
}

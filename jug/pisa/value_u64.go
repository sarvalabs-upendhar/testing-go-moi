package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// U64Value represents a Value that operates like an uint64
type U64Value uint64

// NewU64Value generates a new U64Value for a given uint64 value.
func NewU64Value(val uint64) U64Value {
	return U64Value(val)
}

// DefaultU64Value generates a new U64Value with an 0.
func DefaultU64Value() U64Value { return 0 }

// Type returns the Datatype of U64Value, which is TypeU64.
// Implements the Value interface for U64Value.
func (x U64Value) Type() *Datatype { return TypeU64 }

// Copy returns a copy of U64Value as a Value.
// Implements the Value interface for U64Value.
func (x U64Value) Copy() Value { return x }

// Data returns the POLO encoded bytes of U64Value.
// Implements the Value interface for U64Value.
func (x U64Value) Data() []byte {
	data, _ := polo.Polorize(x)

	return data
}

var (
	ErrIntegerOverflow = errors.New("integer overflow")
	ErrDivideByZero    = errors.New("divide by zero")
)

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

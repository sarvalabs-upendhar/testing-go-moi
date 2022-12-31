package register

import "fmt"

// MaxTypeID represents the maximum allowed type ID.
// This takes the value of the highest value for a defined PrimitiveType variant.
const MaxTypeID = uint8(PrimitiveBigInt)

// PrimitiveType represents an enum with variations that
// represent the builtin primitive/scalar datatypes
type PrimitiveType int

const (
	PrimitivePtr PrimitiveType = iota - 1
	PrimitiveNull
	PrimitiveBool
	PrimitiveBytes
	PrimitiveString
	PrimitiveU64
	PrimitiveI64
	PrimitiveAddress
	PrimitiveBigInt
)

var primitiveToString = map[PrimitiveType]string{
	PrimitivePtr:     "ptr",
	PrimitiveNull:    "null",
	PrimitiveBool:    "bool",
	PrimitiveBytes:   "bytes",
	PrimitiveString:  "string",
	PrimitiveU64:     "uint64",
	PrimitiveI64:     "int64",
	PrimitiveAddress: "address",
	PrimitiveBigInt:  "bigint",
}

// String returns a string representation of the primitive.
// It implements the Stringer interface for primitive
func (p PrimitiveType) String() string {
	str := primitiveToString[p]
	if len(str) == 0 {
		return fmt.Sprintf("undefined primitive [%#x]", int(p))
	}

	return str
}

// Datatype returns the PrimitiveType as a Typedef
func (p PrimitiveType) Datatype() *Typedef { return &Typedef{P: p} }

// Equals returns whether a primitive has equality with another
func (p PrimitiveType) Equals(other PrimitiveType) bool { return p == other }

// Declarable returns whether a primitive has declarability.
// All primitives except runtime objects such as null and pointers and are declarable
func (p PrimitiveType) Declarable() bool { return p > 0 }

// Numeric returns whether a primitive has numericality.
func (p PrimitiveType) Numeric() bool {
	switch p {
	case PrimitiveI64, PrimitiveU64, PrimitiveBigInt:
		return true
	}

	return false
}

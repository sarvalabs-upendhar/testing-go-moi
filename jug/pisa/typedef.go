package pisa

import "fmt"

// Datatype represents the type information for a Value
type Datatype struct {
	K DatatypeKind // represents the kind of datatype

	P PrimitiveType // represents the primitive variant for either a Primitive or a Hashmap key.
	E *Datatype     // represents the element datatype for Array, Sequence or Hashmap value.

	S uint64 // represents the size for Array.
	I uint64 // represents the ptr index to the typedef for a Class or Event.

	F *FieldTable // represents the fields for a Class or Event
	// M Methods     // represents the methods for a Class
}

// newArrayType creates a new Array Datatype
func newArrayType(size uint64, element *Datatype) *Datatype {
	return &Datatype{K: Array, E: element, S: size}
}

// newSequenceType creates a new Sequence Datatype
func newSequenceType(element *Datatype) *Datatype {
	return &Datatype{K: Sequence, E: element}
}

// newHashmapType creates a new Hashmap Datatype
func newHashmapType(key PrimitiveType, val *Datatype) *Datatype {
	return &Datatype{K: Hashmap, P: key, E: val}
}

var (
	// TypePtr is a PrimitivePtr as a Datatype
	TypePtr = &Datatype{P: PrimitivePtr}
	// TypeNull is a PrimitiveNull as a Datatype
	TypeNull = &Datatype{P: PrimitiveNull}
	// TypeBool is a PrimitiveBool as a Datatype
	TypeBool = &Datatype{P: PrimitiveBool}
	// TypeBytes is a PrimitiveBytes as a Datatype
	TypeBytes = &Datatype{P: PrimitiveBytes}
	// TypeString is a PrimitiveString as a Datatype
	TypeString = &Datatype{P: PrimitiveString}
	// TypeU32 is a PrimitiveU32 as a Datatype
	TypeU32 = &Datatype{P: PrimitiveU32}
	// TypeU64 is a PrimitiveU64 as a Datatype
	TypeU64 = &Datatype{P: PrimitiveU64}
	// TypeI32 is a PrimitiveI32 as a Datatype
	TypeI32 = &Datatype{P: PrimitiveI32}
	// TypeI64 is a PrimitiveI64 as a Datatype
	TypeI64 = &Datatype{P: PrimitiveI64}
	// TypeAddress is a PrimitiveAddress as a Datatype
	TypeAddress = &Datatype{P: PrimitiveAddress}
	// TypeBigInt is a PrimitiveBigInt as a Datatype
	TypeBigInt = &Datatype{P: PrimitiveBigInt}
)

// Kind returns the DatatypeKind of the Datatype
func (dt Datatype) Kind() DatatypeKind { return dt.K }

// Equals returns whether the given Datatype is equal to another
func (dt Datatype) Equals(other *Datatype) bool {
	// If type kinds are not the same, immediately not equal
	if dt.Kind() != other.Kind() {
		return false
	}

	switch dt.Kind() {
	case Primitive:
		return dt.P.Equals(other.P)
	case Array:
		return dt.E.Equals(other.E) && dt.S == other.S
	case Sequence:
		return dt.E.Equals(other.E)
	case Hashmap:
		return dt.E.Equals(other.E) && dt.P.Equals(other.P)

	default:
		panic("cannot check type equality for unimplemented datatype kind")
	}
}

// Copy returns a deep copy of Datatype
func (dt Datatype) Copy() *Datatype {
	ndt := new(Datatype)
	ndt.K, ndt.P, ndt.E, ndt.S, ndt.I = dt.K, dt.P, dt.E, dt.S, dt.I

	return ndt
}

// String returns the Datatype expression string which is a
// valid type expression that can be parsed with ParseDatatype.
// It implements the Stringer interface for Datatype.
func (dt Datatype) String() string {
	switch dt.Kind() {
	case Primitive:
		return dt.P.String()
	case Array:
		return fmt.Sprintf("[%v]%v", dt.S, dt.E.String())
	case Sequence:
		return fmt.Sprintf("[]%v", dt.E.String())
	case Hashmap:
		return fmt.Sprintf("map[%v]%v", dt.P.String(), dt.E.String())

	default:
		panic("unsupported string conversion for Datatype")
	}
}

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
	PrimitiveU32
	PrimitiveU64
	PrimitiveI32
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
	PrimitiveU32:     "uint32",
	PrimitiveU64:     "uint64",
	PrimitiveI32:     "int32",
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

// Datatype returns the PrimitiveType as a Datatype
func (p PrimitiveType) Datatype() *Datatype { return &Datatype{P: p} }

// Equals returns whether a primitive has equality with another
func (p PrimitiveType) Equals(other PrimitiveType) bool { return p == other }

// Declarable returns whether a primitive has declarability.
// All primitives except runtime objects such as null and pointers and are declarable
func (p PrimitiveType) Declarable() bool { return p > 0 }

// Numeric returns whether a primitive has numericality.
func (p PrimitiveType) Numeric() bool {
	switch p {
	case PrimitiveI32, PrimitiveI64, PrimitiveU32, PrimitiveU64, PrimitiveBigInt:
		return true
	}

	return false
}

// DatatypeKind represents an enum with variations that indicate the kind of Datatype
type DatatypeKind int

const (
	Primitive DatatypeKind = iota
	Array
	Sequence
	Hashmap
	Event
	Class
)

var datatypeKindToString = map[DatatypeKind]string{
	Primitive: "primitive",
	Array:     "array",
	Sequence:  "sequence",
	Hashmap:   "hashmap",
	Event:     "event",
	Class:     "class",
}

// String implements the Stringer interface for DatatypeKind
func (kind DatatypeKind) String() string {
	str := datatypeKindToString[kind]
	if len(str) == 0 {
		return fmt.Sprintf("undefined datatype kind [%#x]", int(kind))
	}

	return str
}

package register

import "fmt"

// Typedef represents the type information for a Value
type Typedef struct {
	K TypeKind // represents the kind of datatype

	P PrimitiveType // represents the primitive variant for either a Primitive or a Hashmap key.
	E *Typedef      // represents the element datatype for Array, Sequence or Hashmap value.

	S uint64 // represents the size for Array.
	I uint64 // represents the ptr index to the typedef for a Class or Event.

	F *FieldTable // represents the fields for a Class or Event
	// M Methods     // represents the methods for a Class
}

// newArrayType creates a new Array Typedef
func newArrayType(size uint64, element *Typedef) *Typedef {
	return &Typedef{K: Array, E: element, S: size}
}

// newVarrayType creates a new Varray Typedef
func newVarrayType(element *Typedef) *Typedef {
	return &Typedef{K: Varray, E: element}
}

// newHashmapType creates a new Hashmap Typedef
func newHashmapType(key PrimitiveType, val *Typedef) *Typedef {
	return &Typedef{K: Hashmap, P: key, E: val}
}

var (
	// TypePtr is a PrimitivePtr as a Typedef
	TypePtr = &Typedef{P: PrimitivePtr}
	// TypeNull is a PrimitiveNull as a Typedef
	TypeNull = &Typedef{P: PrimitiveNull}
	// TypeBool is a PrimitiveBool as a Typedef
	TypeBool = &Typedef{P: PrimitiveBool}
	// TypeBytes is a PrimitiveBytes as a Typedef
	TypeBytes = &Typedef{P: PrimitiveBytes}
	// TypeString is a PrimitiveString as a Typedef
	TypeString = &Typedef{P: PrimitiveString}
	// TypeU64 is a PrimitiveU64 as a Typedef
	TypeU64 = &Typedef{P: PrimitiveU64}
	// TypeI64 is a PrimitiveI64 as a Typedef
	TypeI64 = &Typedef{P: PrimitiveI64}
	// TypeAddress is a PrimitiveAddress as a Typedef
	TypeAddress = &Typedef{P: PrimitiveAddress}
	// TypeBigInt is a PrimitiveBigInt as a Typedef
	TypeBigInt = &Typedef{P: PrimitiveBigInt}
)

// Kind returns the TypeKind of the Typedef
func (dt Typedef) Kind() TypeKind { return dt.K }

func (dt Typedef) IsCollection() bool {
	if dt.Kind() == Array || dt.Kind() == Varray || dt.Kind() == Hashmap {
		return true
	}

	return false
}

// Equals returns whether the given Typedef is equal to another
func (dt Typedef) Equals(other *Typedef) bool {
	// If type kinds are not the same, immediately not equal
	if dt.Kind() != other.Kind() {
		return false
	}

	switch dt.Kind() {
	case Primitive:
		return dt.P.Equals(other.P)
	case Array:
		return dt.E.Equals(other.E) && dt.S == other.S
	case Varray:
		return dt.E.Equals(other.E)
	case Hashmap:
		return dt.E.Equals(other.E) && dt.P.Equals(other.P)

	default:
		panic("cannot check type equality for unimplemented datatype kind")
	}
}

// Copy returns a deep copy of Typedef
func (dt Typedef) Copy() *Typedef {
	ndt := new(Typedef)
	ndt.K, ndt.P, ndt.E, ndt.S, ndt.I = dt.K, dt.P, dt.E, dt.S, dt.I

	return ndt
}

// String returns the Typedef expression string which is a
// valid type expression that can be parsed with ParseDatatype.
// It implements the Stringer interface for Typedef.
func (dt Typedef) String() string {
	switch dt.Kind() {
	case Primitive:
		return dt.P.String()
	case Array:
		return fmt.Sprintf("[%v]%v", dt.S, dt.E.String())
	case Varray:
		return fmt.Sprintf("[]%v", dt.E.String())
	case Hashmap:
		return fmt.Sprintf("map[%v]%v", dt.P.String(), dt.E.String())

	default:
		panic("unsupported string conversion for Typedef")
	}
}

// TypeKind represents an enum with variations that indicate the kind of Typedef
type TypeKind int

const (
	Primitive TypeKind = iota
	Array
	Varray
	Hashmap
	Event
	Class
)

var datatypeKindToString = map[TypeKind]string{
	Primitive: "primitive",
	Array:     "array",
	Varray:    "varray",
	Hashmap:   "hashmap",
	Event:     "event",
	Class:     "class",
}

// String implements the Stringer interface for TypeKind
func (kind TypeKind) String() string {
	str := datatypeKindToString[kind]
	if len(str) == 0 {
		return fmt.Sprintf("undefined datatype kind [%#x]", int(kind))
	}

	return str
}

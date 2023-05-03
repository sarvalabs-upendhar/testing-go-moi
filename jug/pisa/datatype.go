package pisa

import (
	"fmt"
	"strconv"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// Datatype represents a type information for engine values
type Datatype struct {
	// Kind specifies the kind of Datatype.
	Kind DatatypeKind

	// Prim is the Primitive type for a PrimitiveType.
	// It also holds the key type for MappingType.
	Prim Primitive
	// Elem is the Element type for ArrayType and VarrayType.
	// It also holds the val type of MappingType.
	Elem *Datatype
	// Size specifies the fixed size for an ArrayType.
	Size uint64

	// Ident specifies the name for a ClassType
	Ident string
	// Fields specifies the fields for a ClassType
	Fields *TypeFields
	// Methods specifies the method code to element pointer for a ClassType
	Methods map[MethodCode]engineio.ElementPtr
}

// NewArrayType creates a new Array Datatype
func NewArrayType(size uint64, element *Datatype) *Datatype {
	return &Datatype{Kind: ArrayType, Elem: element, Size: size}
}

// NewVarrayType creates a new Varray Datatype
func NewVarrayType(element *Datatype) *Datatype {
	return &Datatype{Kind: VarrayType, Elem: element}
}

// NewMappingType creates a new Mapping Datatype
func NewMappingType(key Primitive, val *Datatype) *Datatype {
	return &Datatype{Kind: MappingType, Prim: key, Elem: val}
}

// NewClassType creates a new Class Datatype
func NewClassType(name string, fields *TypeFields) *Datatype {
	return &Datatype{Kind: ClassType, Ident: name, Fields: fields}
}

// String returns the Typedef expression string which is a
// valid type expression that can be parsed with ParseDatatype.
// It implements the Stringer interface for Typedef.
func (datatype Datatype) String() string {
	switch datatype.Kind {
	case PrimitiveType:
		return datatype.Prim.String()
	case ArrayType:
		return fmt.Sprintf("[%v]%v", datatype.Size, datatype.Elem.String())
	case VarrayType:
		return fmt.Sprintf("[]%v", datatype.Elem.String())
	case MappingType:
		return fmt.Sprintf("map[%v]%v", datatype.Prim.String(), datatype.Elem.String())
	case ClassType:
		return datatype.Ident

	default:
		panic("unsupported string conversion for Datatype")
	}
}

// Copy returns a deep copy of Typedef
func (datatype Datatype) Copy() *Datatype {
	clone := &Datatype{
		Kind:  datatype.Kind,
		Prim:  datatype.Prim,
		Size:  datatype.Size,
		Ident: datatype.Ident,
	}

	if datatype.Elem != nil {
		clone.Elem = datatype.Elem.Copy()
	}

	if datatype.Fields != nil {
		clone.Fields = datatype.Fields.Copy()
	}

	return clone
}

// Equals returns whether the given Datatype is equal to another
func (datatype Datatype) Equals(other *Datatype) bool {
	// If type kinds are not the same, immediately not equal
	if datatype.Kind != other.Kind {
		return false
	}

	switch datatype.Kind {
	case PrimitiveType:
		return datatype.Prim.Equals(other.Prim)
	case ArrayType:
		return datatype.Elem.Equals(other.Elem) && datatype.Size == other.Size
	case VarrayType:
		return datatype.Elem.Equals(other.Elem)
	case MappingType:
		return datatype.Elem.Equals(other.Elem) && datatype.Prim.Equals(other.Prim)
	case ClassType:
		return datatype.Fields.Equals(other.Fields) && datatype.Ident == other.Ident

	default:
		panic("cannot check type equality for unknown datatype kind")
	}
}

// Method is extension of the Runnable interface and
// represents runnable methods on primitive and class types.
type Method interface {
	Runnable

	code() MethodCode
	datatype() *Datatype
}

// MethodCode represents a unique byte identifier for the method of a type.
// The first 16 bytes (0x00 - 0x0F) are reserved as special method codes.
type MethodCode byte

const (
	MethodBuild MethodCode = 0x0
	MethodThrow MethodCode = 0x1
	MethodEmit  MethodCode = 0x2
	MethodJoin  MethodCode = 0x3

	MethodLt MethodCode = 0x4
	MethodGt MethodCode = 0x5
	MethodEq MethodCode = 0x6

	MethodBool MethodCode = 0x7
	MethodStr  MethodCode = 0x8
	MethodAddr MethodCode = 0x9
	MethodLen  MethodCode = 0xA
)

var methodCodeToString = map[MethodCode]string{
	MethodBuild: "__build__",
	MethodThrow: "__throw__",
	MethodEmit:  "__emit__",
	MethodJoin:  "__join__",

	MethodLt: "__lt__",
	MethodGt: "__gt__",
	MethodEq: "__eq__",

	MethodBool: "__bool__",
	MethodStr:  "__str__",
	MethodAddr: "__addr__",
	MethodLen:  "__len__",
}

// String returns a string representation of the primitive.
// It implements the Stringer interface for primitive
func (method MethodCode) String() string {
	str, ok := methodCodeToString[method]
	if !ok {
		return fmt.Sprintf("method(%#x)", int(method))
	}

	return str
}

var (
	// TypePtr is a PrimitivePtr as a Datatype
	TypePtr = &Datatype{Prim: PrimitivePtr}
	// TypeNull is a PrimitiveNull as a Datatype
	TypeNull = &Datatype{Prim: PrimitiveNull}
	// TypeBool is a PrimitiveBool as a Datatype
	TypeBool = &Datatype{Prim: PrimitiveBool}
	// TypeBytes is a PrimitiveBytes as a Datatype
	TypeBytes = &Datatype{Prim: PrimitiveBytes}
	// TypeString is a PrimitiveString as a Datatype
	TypeString = &Datatype{Prim: PrimitiveString}
	// TypeU64 is a PrimitiveU64 as a Datatype
	TypeU64 = &Datatype{Prim: PrimitiveU64}
	// TypeI64 is a PrimitiveI64 as a Datatype
	TypeI64 = &Datatype{Prim: PrimitiveI64}
	// TypeAddress is a PrimitiveAddress as a Datatype
	TypeAddress = &Datatype{Prim: PrimitiveAddress}
	// TypeBigInt is a PrimitiveBigInt as a Datatype
	TypeBigInt = &Datatype{Prim: PrimitiveBigInt}
)

// Primitive represents an enum with variations that
// represent the builtin primitive/scalar datatypes
type Primitive int

// MaxPrimitive represents the maximum allowed primitive type number.
// This takes the value of the highest value for a defined PrimitiveType variant.
const MaxPrimitive = uint8(PrimitiveBigInt)

const (
	PrimitivePtr Primitive = iota - 1
	PrimitiveNull
	PrimitiveBool
	PrimitiveBytes
	PrimitiveString
	PrimitiveU64
	PrimitiveI64
	PrimitiveAddress
	PrimitiveBigInt
)

var primitiveToString = map[Primitive]string{
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
func (primitive Primitive) String() string {
	str, ok := primitiveToString[primitive]
	if !ok {
		panic("unknown Primitive variant")
	}

	return str
}

// Datatype returns the PrimitiveType as a Typedef
func (primitive Primitive) Datatype() *Datatype { return &Datatype{Prim: primitive} }

// Equals returns whether a primitive has equality with another
func (primitive Primitive) Equals(other Primitive) bool { return primitive == other }

// Declarable returns whether a primitive has declarability.
// All primitives except runtime objects such as null and pointers and are declarable
func (primitive Primitive) Declarable() bool { return primitive > 0 }

// Numeric returns whether a primitive has numericality.
func (primitive Primitive) Numeric() bool {
	switch primitive {
	case PrimitiveI64, PrimitiveU64, PrimitiveBigInt:
		return true
	}

	return false
}

// DatatypeKind represents an enum with variants
// that indicate the different kinds of Datatype
type DatatypeKind int

const (
	PrimitiveType DatatypeKind = iota
	ArrayType
	VarrayType
	MappingType
	ClassType
)

var datatypeKindToString = map[DatatypeKind]string{
	PrimitiveType: "primitive",
	ArrayType:     "array",
	VarrayType:    "varray",
	MappingType:   "mapping",
	ClassType:     "class",
}

// String implements the Stringer interface for DatatypeKind
func (kind DatatypeKind) String() string {
	str, ok := datatypeKindToString[kind]
	if !ok {
		panic("unknown DatatypeKind variant")
	}

	return str
}

func (kind DatatypeKind) IsCollection() bool {
	return kind == ArrayType || kind == VarrayType || kind == MappingType
}

// ClassProvider is an interface that provides Class Datatype definitions
type ClassProvider interface {
	GetClassDatatype(string) (*Datatype, bool)
}

// ParseDatatype attempts to parse a string input into a Typedef
// 1. Valid Primitive types include {bool, bytes, string, address, (u)int(32/64), bigint}
// 2. Valid Array types are expressed as '[{size}]{element}'. The element must in turn be any valid Typedef.
// 3. Valid Sequence types are expressed as '[]{element}'. The element must in turn be any valid Typedef
// 3. Valid Hashmap types are expressed as 'map[{element}]'. The element must in turn be any valid Typedef.
func ParseDatatype(input string, provider ClassProvider) (*Datatype, error) {
	// Create a new parser check cursor type
	parser := newTypeParser(input)

	switch parser.Cursor().Kind {
	// Primitive token
	case tokenPrimitive:
		// Check datatype literal
		switch dt := parser.Cursor().Literal; dt {
		case "bool":
			return TypeBool, nil
		case "bytes":
			return TypeBytes, nil
		case "string":
			return TypeString, nil
		case "int64":
			return TypeI64, nil
		case "uint64":
			return TypeU64, nil
		case "address":
			return TypeAddress, nil
		case "bigint":
			return TypeBigInt, nil
		default:
			panic(fmt.Sprintf("unsupported primtive literal: %v", dt))
		}

	// Array or Sequence
	case symbolizer.TokenKind('['):
		// Unwrap the key type data from within []
		unwrapped, err := parser.Unwrap(symbolizer.EnclosureSquare())
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for array")
		}

		// If data within [] is empty -> Sequence
		if unwrapped == "" {
			// Parse what's left in the parser into a Typedef
			elementType, err := ParseDatatype(parser.Unparsed(), provider)
			if err != nil {
				return nil, errors.Wrap(err, "invalid type data for sequence: invalid element type")
			}

			return NewVarrayType(elementType), nil
		}

		// Parse unwrapped data into a uint64 (array size)
		arraySize, err := strconv.ParseUint(unwrapped, 10, 64)
		if err != nil {
			return nil, errors.New("invalid type data for array: size must be a 64-bit unsigned integer")
		}

		// Parse what's left in the parser into a Typedef
		elementType, err := ParseDatatype(parser.Unparsed(), provider)
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for array: invalid element type")
		}

		return NewArrayType(arraySize, elementType), nil

	// Hashmap Token
	case tokenMapping:
		parser.Advance()

		// Unwrap the key type data from within []
		keyData, err := parser.Unwrap(symbolizer.EnclosureSquare())
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for hashmap")
		}

		// Create a new parser from the enclosed key type data
		// and check that the first token is a primitive type
		keyParser := newTypeParser(keyData)
		if !keyParser.IsCursor(tokenPrimitive) {
			return nil, errors.New("invalid type data for hashmap: invalid key type: must be a valid primitive")
		}

		// Parse the datatype token into a Typedef.
		// This is guaranteed to work because only valid primitive types are literals for TokenPrimitive
		keyType, _ := ParseDatatype(keyParser.Cursor().Literal, provider)

		// Parse what's left in the parser into a Typedef
		elementType, err := ParseDatatype(parser.Unparsed(), provider)
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for hashmap: invalid value type")
		}

		return NewMappingType(keyType.Prim, elementType), nil

	// Identifier Token (Class)
	case symbolizer.TokenIdent:
		// Get the name of the class
		className := parser.Cursor().Literal

		// Get the typedef for the class from the provider
		classDef, ok := provider.GetClassDatatype(className)
		if !ok {
			return nil, errors.Errorf("invalid class reference: '%v' not found", className)
		}

		// Check that the typedef is of kind ClassType
		if classDef.Kind != ClassType {
			return nil, errors.Errorf("invalid class reference: '%v' is not a class", className)
		}

		return classDef, nil

	default:
		// Input does not start with type or bind keyword
		return nil, errors.New("not a datatype")
	}
}

// Custom Token Classes start from -10 and
// descend as recommended by the symbolizer pkg
const (
	tokenPrimitive symbolizer.TokenKind = -(iota + 10)
	tokenBoolean
	tokenMapping
)

// newTypeParser generates a new symbol parser that ignores whitespaces and detects datatype tokens
// such as "map" and "string". It also detects boolean literals ("true" and "false") apart from all
// the supported token classes from the symbolizer package such identifiers, hex and decimal numbers.
func newTypeParser(symbol string) *symbolizer.Parser {
	return symbolizer.NewParser(
		symbol,
		symbolizer.IgnoreWhitespaces(),
		symbolizer.Keywords(map[string]symbolizer.TokenKind{
			"bool":    tokenPrimitive,
			"bytes":   tokenPrimitive,
			"string":  tokenPrimitive,
			"uint32":  tokenPrimitive,
			"uint64":  tokenPrimitive,
			"int32":   tokenPrimitive,
			"int64":   tokenPrimitive,
			"address": tokenPrimitive,
			"bigint":  tokenPrimitive,

			"true":  tokenBoolean,
			"false": tokenBoolean,
			"map":   tokenMapping,
		}),
	)
}

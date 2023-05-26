package pisa

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// Datatype represents the type information for a RegisterValue
type Datatype interface {
	Kind() DatatypeKind
	Copy() Datatype
	String() string
	Equals(Datatype) bool
}

// EncodeDatatype encodes a given Datatype into its POLO serialized bytes.
func EncodeDatatype(datatype Datatype) ([]byte, error) {
	if datatype == nil {
		return nil, errors.New("cannot encode nil datatype")
	}

	// Create a new Polorizer to encode the data
	polorizer := polo.NewPolorizer()

	// Encode the datatype kind [0]
	kind := datatype.Kind()
	polorizer.PolorizeUint(uint64(kind))

	switch kind {
	// kind:0
	case Primitive:
		// Encode the primitive value [1]
		primitive, _ := datatype.(PrimitiveDatatype)
		polorizer.PolorizeInt(int64(primitive))

	// kind:3
	case Mapping:
		// Encode the key type [1]
		mapping, _ := datatype.(MapDatatype)
		polorizer.PolorizeInt(int64(mapping.key))

		// Encode the val type
		encoded, err := EncodeDatatype(mapping.val)
		if err != nil {
			return nil, err
		}

		// Add the val type wire [2]
		// We can skip the error check, because the data
		// is guaranteed to be valid POLO encoded data.
		_ = polorizer.PolorizeAny(encoded)

	// kind:1
	case Array:
		array, _ := datatype.(ArrayDatatype)
		// Encode the array element type
		encoded, err := EncodeDatatype(array.elem)
		if err != nil {
			return nil, err
		}

		// Add the element type wire [1]
		_ = polorizer.PolorizeAny(encoded)
		// Encode the array size [2]
		polorizer.PolorizeUint(array.size)

	// kind:2
	case Varray:
		varray, _ := datatype.(VarrayDatatype)
		// Encode the varray element type
		encoded, err := EncodeDatatype(varray.elem)
		if err != nil {
			return nil, err
		}

		// Add the element type wire [1]
		_ = polorizer.PolorizeAny(encoded)

	// kind:5
	case Class:
		class, _ := datatype.(ClassDatatype)
		// Encode the class name [1]
		polorizer.PolorizeString(class.name)

		// Encode the class fields [2]
		if err := polorizer.Polorize(class.fields); err != nil {
			return nil, err
		}

		// Encode the class methods [3]
		if err := polorizer.Polorize(class.methods); err != nil {
			return nil, err
		}

	default:
		panic(fmt.Sprintf("cannot encode Datatype of unknown kind: %v", kind))
	}

	// Flatten the polorizer bytes and return it
	return polorizer.Bytes(), nil
}

// DecodeDatatype decodes some POLO serialized bytes into a Datatype
func DecodeDatatype(data []byte) (Datatype, error) {
	// Create a depolorizer to decode the data
	depolorizer, err := polo.NewDepolorizer(data)
	if err != nil {
		return nil, err
	}

	// Unwrap the depolorizer from its packed wire
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return PrimitiveNull, nil
	} else if err != nil {
		return nil, err
	}

	// Decode the datatype kind [0]
	k, err := depolorizer.DepolorizeUint()
	if err != nil {
		return nil, err
	}

	switch kind := DatatypeKind(k); kind {
	// kind: 0, 3
	case Primitive, Mapping:
		// Decode primitive type value (key type if map) [1]
		key, err := depolorizer.DepolorizeInt()
		if err != nil {
			return nil, err
		}

		// Check that primitive type value is within valid bounds
		if key > int64(MaxPrimitiveKind) || key < 0 {
			return nil, errors.Errorf("invalid primitive type value: %v", key)
		}

		// If kind is Primitive, we have all data needed.
		// Create a new PrimitiveDatatype and return it
		if kind == Primitive {
			return PrimitiveDatatype(key), nil
		}

		// Decode a wire element for the map val type [2]
		wire, err := depolorizer.DepolorizeAny()
		if err != nil {
			return nil, err
		}

		// Recursively, decode the wire into a Datatype
		datatype, err := DecodeDatatype(wire)
		if err != nil {
			return nil, err
		}

		// Create a MapDatatype and return
		return MapDatatype{key: PrimitiveDatatype(key), val: datatype}, nil

	// kind: 1, 2
	case Array, Varray:
		// Decode a wire element for the v/array element type [1]
		wire, err := depolorizer.DepolorizeAny()
		if err != nil {
			return nil, err
		}

		// Recursively, decode the wire into a Datatype
		datatype, err := DecodeDatatype(wire)
		if err != nil {
			return nil, err
		}

		// If kind is Varray, we have all data needed.
		// Create a new VarrayDatatype and return it
		if kind == Varray {
			return VarrayDatatype{elem: datatype}, nil
		}

		// Decode the array size [2]
		size, err := depolorizer.DepolorizeUint()
		if err != nil {
			return nil, err
		}

		// Create an ArrayDatatype and return
		return ArrayDatatype{elem: datatype, size: size}, nil

	case Class:
		// Decode the class name [1]
		name, err := depolorizer.DepolorizeString()
		if err != nil {
			return nil, err
		}

		// Decode the class fields [2]
		fields := new(TypeFields)
		if err = depolorizer.Depolorize(fields); err != nil {
			return nil, err
		}

		// todo: update type
		// Decode the class method [3]
		methods := make(map[MethodCode]engineio.ElementPtr)
		if err = depolorizer.Depolorize(&methods); err != nil {
			return nil, err
		}

		// Create a ClassDatatype and return
		return ClassDatatype{name, fields, methods}, nil

	default:
		panic(fmt.Sprintf("cannot decode Datatype of unknown kind: %v", kind))
	}
}

// Method is extension of the Runnable interface and
// represents runnable methods on primitive and class types.
type Method interface {
	Runnable

	code() MethodCode
	datatype() Datatype
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

// BuiltinDatatype defines the datatype for a builtin class type.
// Implements the Datatype interface
type BuiltinDatatype struct {
	name   string
	fields *TypeFields
}

func (builtin BuiltinDatatype) Kind() DatatypeKind { return BuiltinClass }

func (builtin BuiltinDatatype) Copy() Datatype {
	return ClassDatatype{
		name:   builtin.name,
		fields: builtin.fields.Copy(),
	}
}

func (builtin BuiltinDatatype) String() string {
	return fmt.Sprintf("builtin.%v", builtin.name)
}

func (builtin BuiltinDatatype) Equals(other Datatype) bool {
	if other.Kind() != builtin.Kind() {
		return false
	}

	another, _ := other.(BuiltinDatatype)

	return builtin.name == another.name && builtin.fields.Equals(another.fields)
}

// ClassDatatype defines the datatype for a class type.
// Implements the Datatype interface.
type ClassDatatype struct {
	name    string
	fields  *TypeFields
	methods map[MethodCode]engineio.ElementPtr
}

func (class ClassDatatype) Kind() DatatypeKind { return Class }

func (class ClassDatatype) Copy() Datatype {
	clone := ClassDatatype{
		name:   class.name,
		fields: class.fields.Copy(),
	}

	if class.methods == nil {
		return clone
	}

	clone.methods = make(map[MethodCode]engineio.ElementPtr)
	for code, ptr := range class.methods {
		clone.methods[code] = ptr
	}

	return clone
}

func (class ClassDatatype) String() string {
	return class.name
}

func (class ClassDatatype) Equals(other Datatype) bool {
	if other.Kind() != class.Kind() {
		return false
	}

	another, _ := other.(ClassDatatype)

	return class.name == another.name &&
		class.fields.Equals(another.fields) &&
		reflect.DeepEqual(class.methods, another.methods)
}

// MapDatatype defines the datatype for a mapping type.
// Implements the Datatype interface.
type MapDatatype struct {
	key PrimitiveDatatype
	val Datatype
}

func (mapping MapDatatype) Kind() DatatypeKind { return Mapping }

func (mapping MapDatatype) Copy() Datatype {
	return MapDatatype{
		key: mapping.key,
		val: mapping.val.Copy(),
	}
}

func (mapping MapDatatype) String() string {
	return fmt.Sprintf("map[%v]%v", mapping.key.String(), mapping.val.String())
}

func (mapping MapDatatype) Equals(other Datatype) bool {
	if other.Kind() != mapping.Kind() {
		return false
	}

	// Cast the Datatype into MapDatatype
	another, _ := other.(MapDatatype)
	// Check that val and key types are equal
	return mapping.key == another.key && mapping.val.Equals(another.val)
}

// VarrayDatatype defines the datatype for a varray type.
// Implements the Datatype interface.
type VarrayDatatype struct {
	elem Datatype
}

func (varray VarrayDatatype) Kind() DatatypeKind { return Varray }

func (varray VarrayDatatype) Copy() Datatype {
	return VarrayDatatype{
		elem: varray.elem.Copy(),
	}
}

func (varray VarrayDatatype) String() string {
	return fmt.Sprintf("[]%v", varray.elem.String())
}

func (varray VarrayDatatype) Equals(other Datatype) bool {
	if other.Kind() != varray.Kind() {
		return false
	}

	// Cast the Datatype into VarrayDatatype
	another, _ := other.(VarrayDatatype)
	// Check that element type of varray is equal
	return varray.elem.Equals(another.elem)
}

// ArrayDatatype defines the datatype for an array type.
// Implements the Datatype interface.
type ArrayDatatype struct {
	elem Datatype
	size uint64
}

func (array ArrayDatatype) Kind() DatatypeKind { return Array }

func (array ArrayDatatype) String() string {
	return fmt.Sprintf("[%v]%v", array.size, array.elem.String())
}

func (array ArrayDatatype) Copy() Datatype {
	return ArrayDatatype{
		elem: array.elem.Copy(),
		size: array.size,
	}
}

func (array ArrayDatatype) Equals(other Datatype) bool {
	if other.Kind() != array.Kind() {
		return false
	}

	// Cast the Datatype into ArrayDatatype
	another, _ := other.(ArrayDatatype)
	// Check that element type and size of arrays are equal
	return array.elem.Equals(another.elem) && array.size == another.size
}

// PrimitiveDatatype represents an enum with variations for defined primitive types.
// Implements the Datatype interface.
type PrimitiveDatatype int

// MaxPrimitiveKind represents the maximum allowed primitive type number.
// This takes the value of the highest value for a defined PrimitiveDatatype variant.
const MaxPrimitiveKind = uint8(PrimitiveI256)

const (
	PrimitiveCargs PrimitiveDatatype = iota - 2
	PrimitivePtr
	PrimitiveNull
	PrimitiveBool
	PrimitiveBytes
	PrimitiveString
	PrimitiveAddress
	PrimitiveU64
	PrimitiveI64
	PrimitiveU256
	PrimitiveI256
)

var primitiveToString = map[PrimitiveDatatype]string{
	PrimitiveCargs:   "cargs",
	PrimitivePtr:     "ptr",
	PrimitiveNull:    "null",
	PrimitiveBool:    "bool",
	PrimitiveBytes:   "bytes",
	PrimitiveString:  "string",
	PrimitiveAddress: "address",
	PrimitiveU64:     "u64",
	PrimitiveI64:     "i64",
	PrimitiveU256:    "u256",
	PrimitiveI256:    "i256",
}

func (primitive PrimitiveDatatype) Kind() DatatypeKind { return Primitive }

func (primitive PrimitiveDatatype) Copy() Datatype { return primitive }

func (primitive PrimitiveDatatype) Equals(other Datatype) bool {
	if other.Kind() != primitive.Kind() {
		return false
	}

	return other.(PrimitiveDatatype) == primitive //nolint:forcetypeassert
}

func (primitive PrimitiveDatatype) String() string {
	str, ok := primitiveToString[primitive]
	if !ok {
		panic("unknown PrimitiveDatatype variant")
	}

	return str
}

// Declarable returns whether a primitive has declarability.
// All primitives except runtime objects such as null and pointers and are declarable
func (primitive PrimitiveDatatype) Declarable() bool { return primitive > 0 }

// Numeric returns whether a primitive has numericality.
func (primitive PrimitiveDatatype) Numeric() bool {
	switch primitive {
	case PrimitiveI64, PrimitiveU64, PrimitiveU256, PrimitiveI256:
		return true
	}

	return false
}

// DatatypeKind represents an enum with variants
// that indicate the different kinds of Datatype
type DatatypeKind int

const (
	Primitive DatatypeKind = iota
	Array
	Varray
	Mapping
	BuiltinClass
	Class
)

var datatypeKindToString = map[DatatypeKind]string{
	Primitive:    "primitive",
	Array:        "array",
	Varray:       "varray",
	Mapping:      "mapping",
	BuiltinClass: "builtin",
	Class:        "class",
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
	return kind == Array || kind == Varray || kind == Mapping
}

// ClassProvider is an interface that provides ClassDatatype definitions
type ClassProvider interface {
	GetClassDatatype(string) (ClassDatatype, bool)
}

// ParseDatatype attempts to parse a string input into a Typedef
// 1. Valid Primitive types include {bool, bytes, string, address, (u)int(32/64), bigint}
// 2. Valid Array types are expressed as '[{size}]{element}'. The element must in turn be any valid Typedef.
// 3. Valid Sequence types are expressed as '[]{element}'. The element must in turn be any valid Typedef
// 3. Valid Hashmap types are expressed as 'map[{element}]'. The element must in turn be any valid Typedef.
func ParseDatatype(input string, provider ClassProvider) (Datatype, error) {
	// Create a new parser check cursor type
	parser := newTypeParser(input)

	switch parser.Cursor().Kind {
	// Primitive token
	case tokenPrimitive:
		// Check datatype literal
		switch dt := parser.Cursor().Literal; dt {
		case "bool":
			return PrimitiveBool, nil
		case "bytes":
			return PrimitiveBytes, nil
		case "string":
			return PrimitiveString, nil
		case "i64":
			return PrimitiveI64, nil
		case "u64":
			return PrimitiveU64, nil
		case "address":
			return PrimitiveAddress, nil
		case "u256":
			return PrimitiveU256, nil
		case "i256":
			return PrimitiveU256, nil
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

			return VarrayDatatype{elementType}, nil
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

		return ArrayDatatype{elementType, arraySize}, nil

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

		return MapDatatype{keyType.(PrimitiveDatatype), elementType}, nil //nolint:forcetypeassert

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
		if classDef.Kind() != Class {
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
			"address": tokenPrimitive,
			"u64":     tokenPrimitive,
			"i64":     tokenPrimitive,
			"u256":    tokenPrimitive,
			"i256":    tokenPrimitive,

			"true":  tokenBoolean,
			"false": tokenBoolean,
			"map":   tokenMapping,
		}),
	)
}

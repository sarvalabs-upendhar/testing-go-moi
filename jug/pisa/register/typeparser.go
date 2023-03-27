package register

import (
	"fmt"
	"strconv"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
)

// Custom Token Classes start from -10 and
// descend as recommended by the symbolizer pkg
const (
	TokenPrimitive symbolizer.TokenKind = -(iota + 10)
	TokenHashmap
	TokenClass
	TokenBoolean
)

// NewTypeParser generates a new symbol parser that ignores whitespaces and detects datatype tokens
// such as "map" and "string". It also detects boolean literals ("true" and "false") apart from all
// the supported token classes from the symbolizer package such identifiers, hex and decimal numbers.
func NewTypeParser(symbol string) *symbolizer.Parser {
	return symbolizer.NewParser(symbol, symbolizer.IgnoreWhitespaces(), symbolizer.Keywords(typeKeywords))
}

var typeKeywords = map[string]symbolizer.TokenKind{
	"true":  TokenBoolean,
	"false": TokenBoolean,

	"bool":    TokenPrimitive,
	"bytes":   TokenPrimitive,
	"string":  TokenPrimitive,
	"uint32":  TokenPrimitive,
	"uint64":  TokenPrimitive,
	"int32":   TokenPrimitive,
	"int64":   TokenPrimitive,
	"address": TokenPrimitive,
	"bigint":  TokenPrimitive,

	"map":     TokenHashmap,
	"classes": TokenClass,
}

// ParseDatatype attempts to parse a string input into a Typedef
// 1. Valid Primitive types include {bool, bytes, string, address, (u)int(32/64), bigint}
// 2. Valid Array types are expressed as '[{size}]{element}'. The element must in turn be any valid Typedef.
// 3. Valid Sequence types are expressed as '[]{element}'. The element must in turn be any valid Typedef
// 3. Valid Hashmap types are expressed as 'map[{element}]'. The element must in turn be any valid Typedef.
func ParseDatatype(input string) (*Typedef, error) {
	// Create a new parser check cursor type
	parser := NewTypeParser(input)
	switch parser.Cursor().Kind {
	// Primitive token
	case TokenPrimitive:
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
			elementType, err := ParseDatatype(parser.Unparsed())
			if err != nil {
				return nil, errors.Wrap(err, "invalid type data for sequence: invalid element type")
			}

			return newVarrayType(elementType), nil
		}

		// Parse unwrapped data into a uint64 (array size)
		arraySize, err := strconv.ParseUint(unwrapped, 10, 64)
		if err != nil {
			return nil, errors.New("invalid type data for array: size must be a 64-bit unsigned integer")
		}

		// Parse what's left in the parser into a Typedef
		elementType, err := ParseDatatype(parser.Unparsed())
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for array: invalid element type")
		}

		return newArrayType(arraySize, elementType), nil

	// Hashmap Token
	case TokenHashmap:
		parser.Advance()

		// Unwrap the key type data from within []
		keyData, err := parser.Unwrap(symbolizer.EnclosureSquare())
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for hashmap")
		}

		// Create a new parser from the enclosed key type data
		// and check that the first token is a primitive type
		keyParser := NewTypeParser(keyData)
		if !keyParser.IsCursor(TokenPrimitive) {
			return nil, errors.New("invalid type data for hashmap: invalid key type: must be a valid primitive")
		}

		// Parse the datatype token into a Typedef.
		// This is guaranteed to work because only valid primitive types are literals for TokenPrimitive
		keyType, _ := ParseDatatype(keyParser.Cursor().Literal)

		// Parse what's left in the parser into a Typedef
		elementType, err := ParseDatatype(parser.Unparsed())
		if err != nil {
			return nil, errors.Wrap(err, "invalid type data for hashmap: invalid value type")
		}

		return newHashmapType(keyType.P, elementType), nil

	// todo: class support
	// // Class Token
	// case TokenClass:

	default:
		// Input does not start with type or bind keyword
		return nil, errors.New("not a datatype")
	}
}

package pisa

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// Constant represents a constant value declaration.
// It consists of the type information of the constant (primitive)
// and some POLO encoded bytes that describe the constant value.
type Constant struct {
	Type PrimitiveType
	Data []byte
}

// ConstantTable represents a collection of Constant
// objects indexed by their 64-bit pointer (uint64)
type ConstantTable map[uint64]*Constant

// lookup retrieves a Constant object from the ConstantTable for
// a given pointer with a boolean indicating if it exists.
func (table ConstantTable) fetch(ptr uint64) (*Constant, bool) {
	constant, exists := table[ptr]
	// Return the Constant and if it exists in the table
	return constant, exists
}

// insert adds a Constant object into the ConstantTable at the specified pointer.
func (table ConstantTable) insert(ptr uint64, constant *Constant) {
	table[ptr] = constant
}

// NewConstantValue generates a Value object from a Constant.
// Returns an error if the constant data is not interpretable for its type.
func NewConstantValue(constant *Constant) (Value, error) {
	switch constant.Type {
	case PrimitiveString:
		str := new(string)
		if err := polo.Depolorize(str, constant.Data); err != nil {
			return nil, errors.New("malformed data for constant: not a string")
		}

		return NewStringValue(*str), nil

	case PrimitiveBool:
		boolean := new(bool)
		if err := polo.Depolorize(boolean, constant.Data); err != nil {
			return nil, errors.New("malformed data for constant: not a bool")
		}

		return NewBoolValue(*boolean), nil

	case PrimitiveU64:
		number := new(uint64)
		if err := polo.Depolorize(number, constant.Data); err != nil {
			return nil, errors.New("malformed data for constant: not a uint64")
		}

		return NewU64Value(*number), nil

	case PrimitiveAddress:
		address := new([32]byte)
		if err := polo.Depolorize(address, constant.Data); err != nil {
			return nil, errors.New("malformed data for constant: not an address")
		}

		return NewAddressValue(*address), nil

	default:
		panic("cannot generate value for unsupported constant type")
	}
}

// ParseConstant attempts to parse an input into a Constant
// Must follow a struct as follows: {datatype}({value}).
func ParseConstant(input string) (*Constant, error) {
	// Create a new parser and confirm the first token as a datatype
	parser := newTypeParser(input)
	if !parser.IsCursor(TokenPrimitive) {
		return nil, errors.New("constant does not begin with datatype")
	}

	// Parse the token literal into a Datatype
	dt, err := ParseDatatype(parser.Cursor().Literal)
	if err != nil {
		return nil, errors.Wrap(err, "invalid constant datatype")
	}

	// Confirm that the type is scalar
	if dt.Kind() != Primitive {
		return nil, errors.New("constant datatype is not scalar")
	}

	parser.Advance()
	// Unwrap the () enclosed data from the parser for the constant value
	enclosed, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, errors.Wrap(err, "constant value data malformed")
	}

	// Create a parser for the value data and switch over the constant datatype
	vParser := newTypeParser(enclosed)

	switch scalar := dt.P; scalar {
	// Bytes Constant
	case PrimitiveBytes:
		// Value token must be hexadecimal
		if !vParser.IsCursor(symbolizer.TokenHexNumber) {
			return nil, errors.New("invalid constant value for bytes: missing hexadecimal")
		}

		val, err := hex.DecodeString(vParser.Cursor().Literal)
		if err != nil {
			return nil, errors.Wrap(err, "invalid constant value for bytes: invalid hexadecimal")
		}

		data, _ := polo.Polorize(val)

		// Return a Bytes Constant
		return &Constant{Type: PrimitiveBytes, Data: data}, nil

	// Address Constant
	case PrimitiveAddress:
		if !vParser.IsCursor(symbolizer.TokenHexNumber) {
			return nil, errors.New("invalid constant value for address: missing hexadecimal")
		}

		val, err := hex.DecodeString(vParser.Cursor().Literal)
		if err != nil {
			return nil, errors.Wrap(err, "invalid constant value for address: invalid hexadecimal")
		}

		if len(val) != 32 {
			return nil, errors.New("invalid constant value for address: bad length (not 32)")
		}

		data, _ := polo.Polorize(val)

		// Return an Address Constant
		return &Constant{Type: PrimitiveAddress, Data: data}, nil

	// String Constant
	case PrimitiveString:
		if !vParser.IsCursor(symbolizer.TokenString) {
			return nil, errors.New("invalid constant value for string: missing text")
		}

		data, _ := polo.Polorize(vParser.Cursor().Literal)

		// Return a String Constant
		return &Constant{Type: PrimitiveString, Data: data}, nil

	// Bool Constant
	case PrimitiveBool:
		// Check the token kind in the parser
		var val bool

		switch vParser.Cursor().Kind {
		// Bool Token
		case TokenBoolean:
			// true if "true", false otherwise
			val = vParser.Cursor().Literal == "true"

		// Numeric Token
		case symbolizer.TokenNumber:
			// false if "0", true otherwise
			val = vParser.Cursor().Literal != "0"

		default:
			return nil, errors.New("invalid constant value for boolean: unsupported value form")
		}

		data, _ := polo.Polorize(val)

		// Return a Bool Constant
		return &Constant{Type: PrimitiveBool, Data: data}, nil

	// Signed Integer Constant
	case PrimitiveI32, PrimitiveI64:
		// Check the token kind in the parser
		var number string

		switch vParser.Cursor().Kind {
		// '-' Token -> neg sign
		case symbolizer.TokenKind('-'):
			// Concat the neg sign to the number string and fallthrough
			number += vParser.Cursor().Literal

			fallthrough

		// '+' Token -> pos sign
		case symbolizer.TokenKind('+'):
			// + signage can be ignored and does not need to be added to the number string
			// We check that the next token is number and fallthrough
			if !vParser.ExpectPeek(symbolizer.TokenNumber) {
				return nil, errors.Errorf("invalid constant value for %v: missing numbers after sign", input)
			}

			fallthrough

		// Numeric Token -> unsigned number
		case symbolizer.TokenNumber:
			// Concat the number into the number string
			number += vParser.Cursor().Literal
			// Set bitSize and kind
			bits, kind := 32, PrimitiveI32
			if scalar == PrimitiveI64 {
				bits, kind = 64, PrimitiveI64
			}

			// Parse the number string into an int64
			val, err := strconv.ParseInt(number, 10, bits)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid constant value for %v", scalar)
			}

			data, _ := polo.Polorize(val)

			// Return a Signed Integer Constant
			return &Constant{Type: kind, Data: data}, nil

		// Hex Token -> hex number
		case symbolizer.TokenHexNumber:
			// Decode the hexadecimal into a string
			hexval, err := hex.DecodeString(vParser.Cursor().Literal)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid constant value for %v: invalid hexadecimal", scalar)
			}

			// Parse into integer based on datatype
			if scalar == PrimitiveI32 {
				if len(hexval) > 4 {
					return nil, errors.New("invalid constant value for int32: hex length too long")
				}

				val := int32(binary.BigEndian.Uint32(append(make([]byte, 4-len(hexval), 4), hexval...)))
				data, _ := polo.Polorize(val)

				// Return a I32 Constant
				return &Constant{Type: PrimitiveI32, Data: data}, nil
			} else {
				if len(hexval) > 8 {
					return nil, errors.New("invalid constant value for int64: hex length too long")
				}

				val := int64(binary.BigEndian.Uint64(append(make([]byte, 8-len(hexval), 8), hexval...)))
				data, _ := polo.Polorize(val)

				// Return a I64 Constant
				return &Constant{Type: PrimitiveI64, Data: data}, nil
			}

		default:
			return nil, errors.Wrapf(err, "invalid constant value for %v: unsupported value form", scalar)
		}

	// Unsigned Integer Constant
	case PrimitiveU32, PrimitiveU64:
		// Check the token kind in the parser
		switch vParser.Cursor().Kind {
		// Numeric Token
		case symbolizer.TokenNumber:
			// Set bitSize and kind
			bits, kind := 32, PrimitiveU32
			if scalar == PrimitiveU64 {
				bits, kind = 64, PrimitiveU64
			}

			// Parse the number string into an uint64
			val, err := strconv.ParseUint(enclosed, 10, bits)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid constant value for %v", scalar)
			}

			data, _ := polo.Polorize(val)

			// Return an Unsigned Integer Constant
			return &Constant{Type: kind, Data: data}, nil

		// Hex Token
		case symbolizer.TokenHexNumber:
			// Decode the hexadecimal into a string
			hexval, err := hex.DecodeString(vParser.Cursor().Literal)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid constant value for %v: invalid hexadecimal", scalar)
			}

			if scalar == PrimitiveU32 {
				if len(hexval) > 4 {
					return nil, errors.New("invalid constant value for uint32: hex length too long")
				}

				val := binary.BigEndian.Uint32(append(make([]byte, 4-len(hexval), 4), hexval...))
				data, _ := polo.Polorize(val)

				// Return a U32 Constant
				return &Constant{Type: PrimitiveU32, Data: data}, nil
			} else {
				if len(hexval) > 8 {
					return nil, errors.New("invalid constant value for uint64: hex length too long")
				}

				val := binary.BigEndian.Uint64(append(make([]byte, 8-len(hexval), 8), hexval...))
				data, _ := polo.Polorize(val)

				// Return a U64 Constant
				return &Constant{Type: PrimitiveU64, Data: data}, nil
			}

		default:
			return nil, errors.Wrapf(err, "invalid constant value for %v: unsupported value form", scalar)
		}

	default:
		panic(fmt.Sprintf("unhandled type case for constant parsing: %v", scalar))
	}
}

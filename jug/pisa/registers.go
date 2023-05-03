package pisa

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

// RegisterValue describes a value stored in a
// register that can be manipulated by PISA
type RegisterValue interface {
	// Type returns the Datatype of the RegisterValue
	Type() *Datatype
	// Copy returns a deep copy of the RegisterValue
	Copy() RegisterValue
	// Data returns the POLO serialized bytes of the RegisterValue
	Data() []byte
	// Norm returns the normalized value of the RegisterValue
	Norm() any
}

// NewRegisterValue generates a RegisterValue object for a given Typedef and some POLO encoded bytes.
// The encoded bytes must be able to deserialize to the underlying type for Typedef.
func NewRegisterValue(dt *Datatype, data []byte) (RegisterValue, error) {
	switch dt.Kind {
	case PrimitiveType:
		switch dt.Prim {
		// StringValue
		case PrimitiveString:
			// If empty data, create the default string value and return
			if data == nil {
				return StringValue(""), nil
			}

			// Decode data into a string
			str := new(string)
			if err := polo.Depolorize(str, data); err != nil {
				return nil, errors.New("data does not decode to a string")
			}

			return StringValue(*str), nil

		// BytesValue
		case PrimitiveBytes:
			// If empty data, create the default bytes value and return
			if data == nil {
				return BytesValue([]byte{}), nil
			}

			// Decode data into a bytes
			bytes := new([]byte)
			if err := polo.Depolorize(bytes, data); err != nil {
				return nil, errors.New("data does not decode to bytes")
			}

			return BytesValue(*bytes), nil

		// BoolValue
		case PrimitiveBool:
			// If empty data, create the default bool value and return
			if data == nil {
				return BoolValue(false), nil
			}

			// Decode data into a bool
			boolean := new(bool)
			if err := polo.Depolorize(boolean, data); err != nil {
				return nil, errors.New("data does not decode to a boolean")
			}

			return BoolValue(*boolean), nil

		// U64Value
		case PrimitiveU64:
			// If empty data, create the default u64 value and return
			if data == nil {
				return U64Value(0), nil
			}

			// Decode data into a uint64
			number := new(uint64)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("data does not decode to a uint64")
			}

			return U64Value(*number), nil

		// I64Value
		case PrimitiveI64:
			// If empty data, create the default i64 value and return
			if data == nil {
				return I64Value(0), nil
			}

			// Decode data into a int64
			number := new(int64)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("data does not decode to a int64")
			}

			return I64Value(*number), nil

		// AddressValue
		case PrimitiveAddress:
			// If empty data, create the default address value and return
			if data == nil {
				return AddressValue(types.NilAddress), nil
			}

			// Decode data into a address
			address := new([32]byte)
			if err := polo.Depolorize(address, data); err != nil {
				return nil, errors.Wrap(err, "data does not decode to an address")
			}

			return AddressValue(*address), nil

		default:
			panic(fmt.Sprintf("unsupported datatype for value generation: %v", dt))
		}

	// ListValue
	case ArrayType, VarrayType:
		return newListValue(dt, data)

	// MapValue
	case MappingType:
		return newMapValue(dt, data)

	// ClassValue
	case ClassType:
		return newClassValue(dt, data)

	default:
		panic(fmt.Sprintf("unsupported datatype for value generation: DatatypeKind(%d)", dt.Kind))
	}
}

// RegisterSet is a collection of byte indexed RegisterValue objects.
type RegisterSet map[byte]RegisterValue

// NewRegisterSet generates a RegisterSet for given set of type fields and values as a polo.Document.
// Each field in the TypeFields must have some associated data in the values that can be interpreted for its type.
// A RegisterValue is generated with this data and attached to the table index specified by the TypeFields.
// Returns an error if data is missing for a field or is malformed and cannot be interpreted for a field's type.
func NewRegisterSet(fields *TypeFields, values polo.Document) (RegisterSet, error) {
	registers := make(RegisterSet, len(fields.Symbols))

	// If the value is nil, but fields are expected
	if values == nil && fields.Size() != 0 {
		return nil, errors.New("missing input values")
	}

	for label, index := range fields.Symbols {
		data := values.GetRaw(label)
		if data == nil {
			return nil, errors.Errorf("missing data for '%v'", label)
		}

		fieldVal, err := NewRegisterValue(fields.Lookup(label).Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed data for '%v'", label)
		}

		registers[index] = fieldVal
	}

	return registers, nil
}

// Get retrieves a RegisterValue for a given address.
// Returns a NullValue if there is no value for the address.
func (registers RegisterSet) Get(id byte) RegisterValue {
	if reg, ok := registers[id]; ok {
		return reg
	}

	return NullValue{}
}

// Set inserts a RegisterValue to a given address.
// Overwrites any existing RegisterValue at the address.
func (registers RegisterSet) Set(id byte, reg RegisterValue) {
	registers[id] = reg
}

// Unset clears a RegisterValue at a given address
func (registers RegisterSet) Unset(id byte) {
	delete(registers, id)
}

func (registers RegisterSet) Validate(fields *TypeFields) error {
	for idx, field := range fields.Table {
		value := registers.Get(idx)
		if value.Type() == TypeNull {
			return errors.Errorf("missing value for field &%v '%v'", idx, field.Name)
		}

		if !value.Type().Equals(field.Type) {
			return errors.Errorf(
				"type mismatch for field &%v '%v'. expected: %v. got: %v",
				idx, field.Name, field.Type, value.Type(),
			)
		}
	}

	return nil
}

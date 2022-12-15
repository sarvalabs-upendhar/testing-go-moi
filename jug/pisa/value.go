package pisa

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	ctypes "github.com/sarvalabs/moichain/jug/types"
)

// Value describes a type that can be held within
// a Register upon which operations can be applied.
type Value interface {
	// Type returns the TypeData of the Value
	Type() *Datatype
	// Data returns the POLO serialized bytes of the Value
	Data() []byte
	// Copy returns a deep copy of the Value
	Copy() Value

	//// String returns the Value as a StringValue
	// String() String
	//// Bytes returns the Value as a BytesValue
	// Bytes() Bytes
	//// Bool returns the Value as a BoolValue
	// Bool() Bool
}

// ValueTable represents a byte indexed collection of Value objects
type ValueTable map[byte]Value

// NewValueTable generates a ValueTable for given set of fields and values as engine.Values.
// Each field in the FieldSet must have some associated data in the values that can be interpreted for its type.
// A Value is generated with this data and attached to the table index specified by the FieldSet.
// Returns an error if data is missing for a field or is malformed and cannot be interpreted for a field's type.
func NewValueTable(fields FieldTable, values ctypes.ExecutionValues) (ValueTable, error) {
	table := make(ValueTable, len(fields.Symbols))

	// If there are fields expected but values is nil
	if len(fields.Symbols) != 0 && values == nil {
		return nil, errors.New("missing input values")
	}

	for label, index := range fields.Symbols {
		data := values.Get(label)
		if data == nil {
			return nil, errors.Errorf("missing data for '%v'", label)
		}

		field := fields.lookup(label)

		fieldVal, err := NewValue(field.Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed data for '%v'", label)
		}

		table[index] = fieldVal
	}

	return table, nil
}

// NewValue generates a Value object for a given Datatype and some POLO encoded bytes.
// The encoded bytes must be able to deserialize to the underlying type for Datatype.
func NewValue(datatype *Datatype, data []byte) (Value, error) {
	switch datatype.Kind() {
	case Primitive:
		switch datatype.P {
		// StringValue
		case PrimitiveString:
			// If empty data, create the default string value and return
			if data == nil {
				return DefaultStringValue(), nil
			}

			// Decode data into a string
			str := new(string)
			if err := polo.Depolorize(str, data); err != nil {
				return nil, errors.New("not string")
			}

			return NewStringValue(*str), nil

		// BoolValue
		case PrimitiveBool:
			// If empty data, create the default bool value and return
			if data == nil {
				return DefaultBoolValue(), nil
			}

			// Decode data into a bool
			boolean := new(bool)
			if err := polo.Depolorize(boolean, data); err != nil {
				return nil, errors.New("not boolean")
			}

			return NewBoolValue(*boolean), nil

		// U64Value
		case PrimitiveU64:
			// If empty data, create the default u64 value and return
			if data == nil {
				return DefaultU64Value(), nil
			}

			// Decode data into a uint64
			number := new(uint64)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("not uint64")
			}

			return NewU64Value(*number), nil

		// AddressValue
		case PrimitiveAddress:
			// If empty data, create the default address value and return
			if data == nil {
				return DefaultAddressValue(), nil
			}

			// Decode data into a address
			address := new([32]byte)
			if err := polo.Depolorize(address, data); err != nil {
				return nil, errors.Wrap(err, "not address")
			}

			return NewAddressValue(*address), nil

		default:
			panic(fmt.Sprintf("unsupported datatype for value generation: %v", datatype))
		}

	// MapValue
	case Hashmap:
		return NewMapValue(datatype, data)

	default:
		panic(fmt.Sprintf("unsupported datatype for value generation: %v", datatype))
	}
}

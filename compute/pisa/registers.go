package pisa

import (
	"fmt"

	"github.com/holiman/uint256"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common"
)

// RegisterValue describes a value stored in a register that can be manipulated by PISA.
// All value types must implement this interface.
type RegisterValue interface {
	// Type returns the Datatype of the RegisterValue
	Type() Datatype
	// Copy returns a deep copy of the RegisterValue
	Copy() RegisterValue
	// Data returns the POLO serialized bytes of the RegisterValue
	Data() []byte
	// Norm returns the normalized value of the RegisterValue
	Norm() any
}

// RegisterObject describes a RegisterValue that can express some methods
// All primitive types, builtin classes and user defined classes must implement this interface.
type RegisterObject interface {
	RegisterValue

	methods() [256]*BuiltinMethod
}

// NewRegisterValue generates a RegisterValue object for a given Typedef and some POLO encoded bytes.
// The encoded bytes must be able to deserialize to the underlying type for Typedef.
func NewRegisterValue(datatype Datatype, data []byte) (RegisterValue, error) {
	switch datatype.Kind() {
	case Primitive:
		prim, _ := datatype.(PrimitiveDatatype)

		switch prim {
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
				return nil, errors.New("data does not decode to a u64")
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
				return nil, errors.New("data does not decode to a i64")
			}

			return I64Value(*number), nil

		// U256Value
		case PrimitiveU256:
			// If empty data, create the default U256 value and return
			if data == nil {
				return &U256Value{uint256.NewInt(0)}, nil
			}

			// Decode data into a uint256
			number := new(U256Value)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("data does not decode to a u256")
			}

			return number, nil

		// I256Value
		case PrimitiveI256:
			// If empty data, create the default I256 value and return
			if data == nil {
				return &I256Value{uint256.NewInt(0)}, nil
			}

			// Decode data into a int256
			number := new(I256Value)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("data does not decode to a i256")
			}

			return number, nil

		// AddressValue
		case PrimitiveAddress:
			// If empty data, create the default address value and return
			if data == nil {
				return AddressValue(common.NilAddress), nil
			}

			// Decode data into a address
			address := new([32]byte)
			if err := polo.Depolorize(address, data); err != nil {
				return nil, errors.Wrap(err, "data does not decode to an address")
			}

			return AddressValue(*address), nil

		default:
			panic(fmt.Sprintf("unsupported datatype for value generation: %v", datatype))
		}

	// ArrayValue
	case Array:
		return newArrayValue(datatype.(ArrayDatatype), data) //nolint:forcetypeassert

	// VarrayValue
	case Varray:
		return newVarrayValue(datatype.(VarrayDatatype), data) //nolint:forcetypeassert

	// MapValue
	case Mapping:
		return newMapValue(datatype.(MapDatatype), data) //nolint:forcetypeassert

	// ClassValue
	case Class:
		return newClassValue(datatype.(ClassDatatype), data) //nolint:forcetypeassert

	default:
		panic(fmt.Sprintf("unsupported datatype for value generation: DatatypeKind(%d)", datatype.Kind()))
	}
}

type CollectionValue interface {
	RegisterValue

	Size() U64Value
	Get(RegisterValue) (RegisterValue, *Exception)
	Set(RegisterValue, RegisterValue) *Exception
}

func decodeListedValues(data []byte, elementType Datatype) ([]RegisterValue, error) {
	depolorizer, err := polo.NewDepolorizer(data)
	if err != nil {
		return nil, err
	}

	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	values := make([]RegisterValue, 0)

	var index uint64

	for !depolorizer.Done() {
		var edata []byte

		// Depolorize the element data from the wire
		if edata, err = depolorizer.DepolorizeAny(); err != nil {
			return nil, err
		}

		var element RegisterValue
		// Create new value from the data for the element
		if element, err = NewRegisterValue(elementType, edata); err != nil {
			return nil, err
		}

		values = append(values, element)

		index++
	}

	return values, nil
}

func decodeMappedValues(data []byte, keyType, valType Datatype) (map[RegisterValue]RegisterValue, error) {
	depolorizer, err := polo.NewDepolorizer(data)
	if err != nil {
		return nil, err
	}

	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	values := make(map[RegisterValue]RegisterValue)

	// Unpack each key value pair from the wire into Values based on their expected datatype.
	for !depolorizer.Done() {
		var kdata, vdata []byte

		// Unpack the key data from the wire
		if kdata, err = depolorizer.DepolorizeAny(); err != nil {
			return nil, err
		}

		// Unpack the value data from the wire
		if vdata, err = depolorizer.DepolorizeAny(); err != nil {
			return nil, err
		}

		var key, val RegisterValue

		// Create new value from the data for the key
		if key, err = NewRegisterValue(keyType, kdata); err != nil {
			return nil, err
		}

		// Create new value from the data for the value
		if val, err = NewRegisterValue(valType, vdata); err != nil {
			return nil, err
		}

		values[key] = val
	}

	return values, nil
}

type SlottedValue interface {
	RegisterValue

	Size() U64Value
	Get(uint8) (RegisterValue, *Exception)
	Set(uint8, RegisterValue) *Exception
}

func decodeSlottedValues(data []byte, fields *TypeFields) (map[byte]RegisterValue, error) {
	doc := make(polo.Document)
	if err := polo.Depolorize(&doc, data); err != nil {
		return nil, err
	}

	values := make(map[byte]RegisterValue)

	for key, raw := range doc {
		// Get the field type from the class def
		field := fields.Lookup(key)
		if field == nil {
			return nil, errors.Errorf("invalid data for field '%v': no such field", key)
		}

		// Create new value from the data for the key
		value, err := NewRegisterValue(field.Type, raw)
		if err != nil {
			return nil, err
		}

		// Get the slot for the field and insert it
		slot := fields.Symbols[field.Name]
		values[slot] = value
	}

	return values, nil
}

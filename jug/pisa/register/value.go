package register

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
)

var (
	ErrIntegerOverflow = errors.New("IntegerOverflow")
	ErrDivideByZero    = errors.New("DivideByZero")
)

// Value describes a type that can be held within
// a register upon which operations can be applied.
type Value interface {
	// Type returns the Datatype of the Value
	Type() *engineio.Datatype
	// Copy returns a deep copy of the Value
	Copy() Value
	// Data returns the POLO serialized bytes of the Value
	Data() []byte
	// Norm returns the normalized value of the Value
	Norm() any
}

type Collection interface {
	Value

	Size() U64Value
	Get(Value) (Value, error)
	Set(Value, Value) error
}

// Constant represents a constant value declaration.
// It consists of the type information of the constant (primitive)
// and some POLO encoded bytes that describe the constant value.
type Constant struct {
	Type engineio.Primitive
	Data []byte
}

// Value generate a new RegisterValue object from a Constant
// Returns an error if the constant data is not interpretable for its type.
func (constant *Constant) Value() (Value, error) {
	return NewValue(constant.Type.Datatype(), constant.Data)
}

// NewValue generates a RegisterValue object for a given Typedef and some POLO encoded bytes.
// The encoded bytes must be able to deserialize to the underlying type for Typedef.
func NewValue(datatype *engineio.Datatype, data []byte) (Value, error) {
	switch datatype.Kind {
	case engineio.PrimitiveType:
		switch datatype.Prim {
		// StringValue
		case engineio.PrimitiveString:
			// If empty data, create the default string value and return
			if data == nil {
				return StringValue(""), nil
			}

			// Decode data into a string
			str := new(string)
			if err := polo.Depolorize(str, data); err != nil {
				return nil, errors.New("not string")
			}

			return StringValue(*str), nil

		// BytesValue
		case engineio.PrimitiveBytes:
			// If empty data, create the default bytes value and return
			if data == nil {
				return BytesValue([]byte{}), nil
			}

			// Decode data into a bytes
			bytes := new([]byte)
			if err := polo.Depolorize(bytes, data); err != nil {
				return nil, errors.New("not bytes")
			}

			return BytesValue(*bytes), nil

		// BoolValue
		case engineio.PrimitiveBool:
			// If empty data, create the default bool value and return
			if data == nil {
				return BoolValue(false), nil
			}

			// Decode data into a bool
			boolean := new(bool)
			if err := polo.Depolorize(boolean, data); err != nil {
				return nil, errors.New("not boolean")
			}

			return BoolValue(*boolean), nil

		// U64Value
		case engineio.PrimitiveU64:
			// If empty data, create the default u64 value and return
			if data == nil {
				return U64Value(0), nil
			}

			// Decode data into a uint64
			number := new(uint64)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("not uint64")
			}

			return U64Value(*number), nil

		// I64Value
		case engineio.PrimitiveI64:
			// If empty data, create the default i64 value and return
			if data == nil {
				return I64Value(0), nil
			}

			// Decode data into a int64
			number := new(int64)
			if err := polo.Depolorize(number, data); err != nil {
				return nil, errors.New("not int64")
			}

			return I64Value(*number), nil

		// AddressValue
		case engineio.PrimitiveAddress:
			// If empty data, create the default address value and return
			if data == nil {
				return AddressValue(types.NilAddress), nil
			}

			// Decode data into a address
			address := new([32]byte)
			if err := polo.Depolorize(address, data); err != nil {
				return nil, errors.Wrap(err, "not address")
			}

			return AddressValue(*address), nil

		default:
			panic(fmt.Sprintf("unsupported datatype for value generation: %v", datatype))
		}

	// ListValue
	case engineio.ArrayType, engineio.VarrayType:
		return NewListValue(datatype, data)

	// MapValue
	case engineio.MappingType:
		return NewMapValue(datatype, data)

	// ClassValue
	case engineio.ClassType:
		return NewClassValue(datatype, data)

	default:
		panic(fmt.Sprintf("unsupported datatype for value generation: %v", datatype))
	}
}

func IsNullValue(value Value) bool {
	if value == nil {
		return true
	}

	if value.Type().Equals(engineio.TypeNull) {
		return true
	}

	return false
}

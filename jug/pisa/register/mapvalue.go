package register

import (
	"reflect"
	"sort"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// MapValue represents a Value that operates like a mapping.
type MapValue struct {
	values  map[Value]Value
	typedef *Typedef
}

// NewMapValue generates a new MapValue for a given Typedef and some POLO encoded bytes.
func NewMapValue(datatype *Typedef, data []byte) (*MapValue, error) {
	// Check if datatype is a map
	if datatype.Kind() != Hashmap {
		return nil, errors.New("datatype is not a hashmap")
	}

	// Initialize the MapValue with the Typedef and an empty mapping
	mapping := new(MapValue)
	mapping.typedef = datatype
	mapping.values = make(map[Value]Value)

	// If there is some data to decode into the MapValue
	// Unpack each key value pair from the wire into Values based on their expected datatype.
	if data != nil {
		unpacker, err := polo.NewUnpacker(data)
		if err != nil {
			return nil, err
		}

		for !unpacker.Done() {
			var kdata, vdata []byte

			// Unpack the key data from the wire
			if kdata, err = unpacker.UnpackWire(); err != nil {
				return nil, err
			}

			// Unpack the value data from the wire
			if vdata, err = unpacker.UnpackWire(); err != nil {
				return nil, err
			}

			var key, val Value

			// Create new value from the data for the key
			if key, err = NewValue(datatype.P.Datatype(), kdata); err != nil {
				return nil, err
			}

			// Create new value from the data for the value
			if val, err = NewValue(datatype.E, vdata); err != nil {
				return nil, err
			}

			mapping.values[key] = val
		}
	}

	return mapping, nil
}

// Type returns the Typedef of MapValue, which is some Hashmap Typedef.
// Implements the Value interface for MapValue.
func (mapping MapValue) Type() *Typedef { return mapping.typedef }

// Copy returns a copy of MapValue as a Value.
// Implements the Value interface for MapValue.
func (mapping MapValue) Copy() Value {
	mcopy := MapValue{values: make(map[Value]Value, len(mapping.values))}
	mcopy.typedef = mapping.typedef.Copy()

	for key, val := range mapping.values {
		mcopy.values[key.Copy()] = val.Copy()
	}

	return mcopy
}

// Data returns the POLO encoded bytes of MapValue.
// Implements the Value interface for MapValue.
func (mapping MapValue) Data() []byte {
	packer := polo.NewPacker()
	v := reflect.ValueOf(mapping.values)

	keys := v.MapKeys()
	sort.Slice(keys, sorter(keys))

	//nolint:forcetypeassert
	for _, key := range keys {
		_ = packer.PackWire(key.Interface().(Value).Data())
		_ = packer.PackWire(v.MapIndex(key).Interface().(Value).Data())
	}

	return packer.Bytes()
}

// Get is a safe read from the MapValue, returns an error
// if the key is not of the correct type for MapValue
func (mapping *MapValue) Get(key Value) (Value, error) {
	keyType := key.Type()
	if keyType.Kind() != Primitive {
		return nil, errors.New("cannot Get from MapValue with non-primitive key")
	}

	if !mapping.typedef.P.Equals(keyType.P) {
		return nil, errors.New("cannot Get from MapValue with incorrect key type")
	}

	value := mapping.values[key]
	// If value is nil, generate the default value for the map element type
	if value == nil {
		value, _ = NewValue(mapping.typedef.E, nil)
	}

	return value, nil
}

// Set is a safe write into the MapValue, returns an error if
// either the key or value are not the correct type for MapValue
func (mapping *MapValue) Set(key, val Value) error {
	keyType := key.Type()
	if keyType.Kind() != Primitive {
		return errors.New("cannot Set to MapValue with non-primitive key")
	}

	if !mapping.typedef.P.Equals(keyType.P) {
		return errors.New("cannot Set to MapValue with incorrect key type")
	}

	if !mapping.typedef.E.Equals(val.Type()) {
		return errors.New("cannot Set to MapValue with incorrect value type")
	}

	mapping.values[key] = val

	return nil
}

func (mapping *MapValue) Size() U64Value {
	return U64Value(len(mapping.values))
}

// sorter is used by the sort package to sort a slice of reflect.Value objects.
// Assumes that the reflect.Value objects can only be types which are comparable
// i.e, can be used as a map key. (will panic otherwise)
func sorter(keys []reflect.Value) func(int, int) bool {
	return func(i int, j int) bool {
		a, b := keys[i], keys[j]
		if a.Kind() == reflect.Interface {
			a, b = a.Elem(), b.Elem()
		}

		switch a.Kind() {
		case reflect.Bool:
			return b.Bool()

		case reflect.String:
			return a.String() < b.String()

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return a.Int() < b.Int()

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return a.Uint() < b.Uint()

		case reflect.Float32, reflect.Float64:
			return a.Float() < b.Float()

		case reflect.Array:
			if a.Len() != b.Len() {
				panic("array length must equal")
			}

			for i := 0; i < a.Len(); i++ {
				result := compare(a.Index(i), b.Index(i))
				if result == 0 {
					continue
				}

				return result < 0
			}

			return false
		}

		panic("unsupported key compare")
	}
}

// compare returns an integer representing the comparison between two reflect.Value objects.
// Assumes that a and b can only have a type that is comparable. (will panic otherwise).
// Returns 1 (a > b); 0 (a == b); -1 (a < b)
func compare(a, b reflect.Value) int {
	if a.Kind() == reflect.Interface {
		a, b = a.Elem(), b.Elem()
	}

	switch a.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		av, bv := a.Int(), b.Int()

		switch {
		case av < bv:
			return -1
		case av == bv:
			return 0
		case av > bv:
			return 1
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		av, bv := a.Uint(), b.Uint()

		switch {
		case av < bv:
			return -1
		case av == bv:
			return 0
		case av > bv:
			return 1
		}

	case reflect.Float32, reflect.Float64:
		av, bv := a.Float(), b.Float()

		switch {
		case av < bv:
			return -1
		case av == bv:
			return 0
		case av > bv:
			return 1
		}

	case reflect.String:
		av, bv := a.String(), b.String()

		switch {
		case av < bv:
			return -1
		case av == bv:
			return 0
		case av > bv:
			return 1
		}

	case reflect.Array:
		if a.Len() != b.Len() {
			panic("array length must equal")
		}

		for i := 0; i < a.Len(); i++ {
			result := compare(a.Index(i), b.Index(i))
			if result == 0 {
				continue
			}

			return result
		}

		return 0
	}

	panic("unsupported key compare")
}

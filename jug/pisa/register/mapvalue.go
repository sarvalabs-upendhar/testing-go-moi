package register

import (
	"reflect"
	"sort"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// MapValue represents a Value that operates like a mapping.
type MapValue struct {
	values   map[Value]Value
	datatype *engineio.Datatype
}

// NewMapValue generates a new MapValue for a given engineio.Datatype and some POLO encoded bytes.
func NewMapValue(datatype *engineio.Datatype, data []byte) (*MapValue, error) {
	// Check if datatype is a map
	if datatype.Kind != engineio.MappingType {
		return nil, errors.New("datatype is not a mapping")
	}

	// Initialize the MapValue with the Typedef and an empty mapping
	mapping := new(MapValue)
	mapping.datatype = datatype
	mapping.values = make(map[Value]Value)

	// If there is some data to decode into the MapValue
	// Unpack each key value pair from the wire into Values based on their expected datatype.
	if data != nil {
		depolorizer, err := polo.NewDepolorizer(data)
		if err != nil {
			return nil, err
		}

		depolorizer, err = depolorizer.DepolorizePacked()
		if errors.Is(err, polo.ErrNullPack) {
			return mapping, nil
		} else if err != nil {
			return nil, err
		}

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

			var key, val Value

			// Create new value from the data for the key
			if key, err = NewValue(datatype.Prim.Datatype(), kdata); err != nil {
				return nil, err
			}

			// Create new value from the data for the value
			if val, err = NewValue(datatype.Elem, vdata); err != nil {
				return nil, err
			}

			mapping.values[key] = val
		}
	}

	return mapping, nil
}

// Type returns the Typedef of MapValue, which is some Mapping engineio.Datatype.
// Implements the Value interface for MapValue.
func (mapping MapValue) Type() *engineio.Datatype { return mapping.datatype }

// Copy returns a copy of MapValue as a Value.
// Implements the Value interface for MapValue.
func (mapping MapValue) Copy() Value {
	mcopy := MapValue{values: make(map[Value]Value, len(mapping.values))}
	mcopy.datatype = mapping.datatype.Copy()

	for key, val := range mapping.values {
		mcopy.values[key.Copy()] = val.Copy()
	}

	return mcopy
}

// Norm returns the normalized value of MapValue as a map[any]any.
// Implements the Value interface for MapValue.
func (mapping MapValue) Norm() any {
	norm := make(map[any]any, len(mapping.values))
	for k, v := range mapping.values {
		norm[k.Norm()] = v.Norm()
	}

	return norm
}

// Data returns the POLO encoded bytes of MapValue.
// Implements the Value interface for MapValue.
func (mapping MapValue) Data() []byte {
	polorizer := polo.NewPolorizer()
	v := reflect.ValueOf(mapping.values)

	keys := v.MapKeys()
	sort.Slice(keys, engineio.MapSorter(keys))

	//nolint:forcetypeassert
	for _, key := range keys {
		_ = polorizer.PolorizeAny(key.Interface().(Value).Data())
		_ = polorizer.PolorizeAny(v.MapIndex(key).Interface().(Value).Data())
	}

	return polorizer.Bytes()
}

// Get is a safe read from the MapValue, returns an error
// if the key is not of the correct type for MapValue
func (mapping *MapValue) Get(key Value) (Value, error) {
	keyType := key.Type()
	if keyType.Kind != engineio.PrimitiveType {
		return nil, errors.New("cannot Get from MapValue with non-primitive key")
	}

	if !mapping.datatype.Prim.Equals(keyType.Prim) {
		return nil, errors.New("cannot Get from MapValue with incorrect key type")
	}

	value := mapping.values[key]
	// If value is nil, generate the default value for the map element type
	if value == nil {
		value, _ = NewValue(mapping.datatype.Elem, nil)
	}

	return value, nil
}

// Set is a safe write into the MapValue, returns an error if
// either the key or value are not the correct type for MapValue
func (mapping *MapValue) Set(key, val Value) error {
	keyType := key.Type()
	if keyType.Kind != engineio.PrimitiveType {
		return errors.New("cannot Set to MapValue with non-primitive key")
	}

	if !mapping.datatype.Prim.Equals(keyType.Prim) {
		return errors.New("cannot Set to MapValue with incorrect key type")
	}

	if !mapping.datatype.Elem.Equals(val.Type()) {
		return errors.New("cannot Set to MapValue with incorrect value type")
	}

	mapping.values[key] = val

	return nil
}

func (mapping *MapValue) Size() U64Value {
	return U64Value(len(mapping.values))
}

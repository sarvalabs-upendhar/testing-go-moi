package pisa

import (
	"reflect"
	"sort"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/compute/engineio"
)

// MapValue represents a RegisterValue that operates like a mapping.
type MapValue struct {
	values   map[RegisterValue]RegisterValue
	datatype MapDatatype
}

// newMapValue generates a new MapValue for a given MapDatatype and some POLO encoded bytes.
func newMapValue(datatype MapDatatype, data []byte) (*MapValue, error) {
	// Initialize the MapValue with the Typedef and an empty mapping
	mapping := new(MapValue)
	mapping.datatype = datatype
	mapping.values = make(map[RegisterValue]RegisterValue)

	// If there is no data to decode, return empty MapValue
	if data == nil {
		return mapping, nil
	}

	values, err := decodeMappedValues(data, mapping.datatype.key, mapping.datatype.val)
	if err != nil {
		return nil, err
	}

	mapping.values = values

	return mapping, nil
}

// newMapFromValues generates a new MapValue for a given MapDatatype and some POLO encoded bytes.
func newMapFromValues(datatype MapDatatype, maps map[RegisterValue]RegisterValue) (*MapValue, error) {
	// Initialize the MapValue with the Typedef and an empty mapping
	mapping := MapValue{values: make(map[RegisterValue]RegisterValue, len(maps))}
	mapping.datatype = datatype

	// If there is no data to decode, return empty MapValue
	if maps == nil {
		return &mapping, nil
	}

	for key, val := range maps {
		mapping.values[key.Copy()] = val.Copy()
	}

	return &mapping, nil
}

// Type returns the Datatype of MapValue, which is some Mapping Datatype.
// Implements the RegisterValue interface for MapValue.
func (mapping *MapValue) Type() Datatype { return mapping.datatype }

// Copy returns a copy of MapValue as a RegisterValue.
// Implements the RegisterValue interface for MapValue.
func (mapping *MapValue) Copy() RegisterValue {
	mcopy := &MapValue{values: make(map[RegisterValue]RegisterValue, len(mapping.values))}
	mcopy.datatype, _ = mapping.datatype.Copy().(MapDatatype)

	for key, val := range mapping.values {
		mcopy.values[key.Copy()] = val.Copy()
	}

	return mcopy
}

// Norm returns the normalized value of MapValue as a map[any]any.
// Implements the RegisterValue interface for MapValue.
func (mapping *MapValue) Norm() any {
	norm := make(map[any]any, len(mapping.values))
	for k, v := range mapping.values {
		norm[k.Norm()] = v.Norm()
	}

	return norm
}

// Data returns the POLO encoded bytes of MapValue.
// Implements the RegisterValue interface for MapValue.
func (mapping *MapValue) Data() []byte {
	polorizer := polo.NewPolorizer()
	v := reflect.ValueOf(mapping.values)

	keys := v.MapKeys()
	sort.Slice(keys, engineio.MapSorter(keys))

	//nolint:forcetypeassert
	for _, key := range keys {
		_ = polorizer.PolorizeAny(key.Interface().(RegisterValue).Data())
		_ = polorizer.PolorizeAny(v.MapIndex(key).Interface().(RegisterValue).Data())
	}

	return polorizer.Packed()
}

// Get is a safe read from the MapValue, returns an error
// if the key is not of the correct type for MapValue
func (mapping *MapValue) Get(key RegisterValue) (RegisterValue, *Exception) {
	// We can safely access Prim, because it will default to
	// PrimitiveNull if the key's datatype is not Primitive
	if !mapping.datatype.key.Equals(key.Type()) {
		return nil, exceptionf(TypeError, "invalid map key: not a %v", mapping.datatype.key)
	}

	value := mapping.values[key]
	if value == nil {
		// todo: this should not happen, we need to return NullValue{} and AccessError instead
		// If value is nil, generate the default value for the map element type
		value, _ = NewRegisterValue(mapping.datatype.val, nil)
	}

	return value, nil
}

// Set is a safe write into the MapValue, returns an error if
// either the key or value are not the correct type for MapValue
func (mapping *MapValue) Set(key, val RegisterValue) *Exception {
	// We can safely access Prim, because it will default to
	// PrimitiveNull if the key's datatype is not Primitive
	if !mapping.datatype.key.Equals(key.Type()) {
		return exceptionf(TypeError, "invalid map key: not a %v", mapping.datatype.key)
	}

	if !mapping.datatype.val.Equals(val.Type()) {
		exceptionf(TypeError, "invalid map value: not a %v", mapping.datatype.val)
	}

	mapping.values[key] = val

	return nil
}

func (mapping *MapValue) Size() U64Value {
	return U64Value(len(mapping.values))
}

func (mapping *MapValue) Has(key RegisterValue) (BoolValue, *Exception) {
	// We can safely access Prim, because it will default to
	// PrimitiveNull if the key's datatype is not Primitive
	if !mapping.datatype.key.Equals(key.Type()) {
		return false, exceptionf(TypeError, "invalid map key: not a %v", mapping.datatype.key)
	}

	_, ok := mapping.values[key]

	return BoolValue(ok), nil
}

func (mapping *MapValue) Merge(insert *MapValue) *MapValue {
	// Copy the mapping into a new mapping
	merged, _ := mapping.Copy().(*MapValue)

	// Insert each non-nil entry into the merged map
	for key, val := range insert.values {
		if val != nil {
			merged.values[key] = val
		}
	}

	return merged
}

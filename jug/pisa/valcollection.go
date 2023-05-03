package pisa

import (
	"reflect"
	"sort"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

type CollectionValue interface {
	RegisterValue

	Size() U64Value
	Get(RegisterValue) (RegisterValue, *Exception)
	Set(RegisterValue, RegisterValue) *Exception
}

type ListValue struct {
	values   []RegisterValue
	datatype *Datatype
}

func newListValue(dt *Datatype, data []byte) (*ListValue, error) {
	list := new(ListValue)
	list.datatype = dt

	switch dt.Kind {
	case ArrayType:
		list.values = make([]RegisterValue, dt.Size)
	case VarrayType:
		list.values = make([]RegisterValue, 0)
	default:
		return nil, errors.New("type is not an v/array")
	}

	if data != nil {
		depolorizer, err := polo.NewDepolorizer(data)
		if err != nil {
			return nil, err
		}

		depolorizer, err = depolorizer.DepolorizePacked()
		if errors.Is(err, polo.ErrNullPack) {
			return list, nil
		} else if err != nil {
			return nil, err
		}

		var index uint64

		for !depolorizer.Done() {
			var edata []byte
			// Depolorize the element data from the wire
			if edata, err = depolorizer.DepolorizeAny(); err != nil {
				return nil, err
			}

			var element RegisterValue
			// Create new value from the data for the element
			if element, err = NewRegisterValue(dt.Elem, edata); err != nil {
				return nil, err
			}

			switch dt.Kind {
			case ArrayType:
				if index >= dt.Size {
					return nil, errors.New("too many elements in data")
				}

				list.values[index] = element

			case VarrayType:
				list.values = append(list.values, element)
			}

			index++
		}
	}

	return list, nil
}

func newListFromValues(datatype *Datatype, values ...RegisterValue) (*ListValue, error) {
	switch datatype.Kind {
	case ArrayType:
		if uint64(len(values)) != datatype.Size {
			return nil, errors.Errorf("incorrect number of values for array of size %v", datatype.Size)
		}

		fallthrough

	case VarrayType:
		list := new(ListValue)
		list.datatype = datatype

		for _, value := range values {
			if value.Type() != datatype.Elem {
				return nil, errors.Errorf("incorrect value type for v/array with element %v", datatype.Elem)
			}

			list.values = append(list.values, value)
		}

		return list, nil

	default:
		return nil, errors.New("type is not an v/array")
	}
}

func newSizedList(datatype *Datatype, size U64Value) (*ListValue, error) {
	switch datatype.Kind {
	case ArrayType:
		if uint64(size) != datatype.Size {
			return nil, errors.Errorf("incorrect size for array")
		}

		fallthrough

	case VarrayType:
		list := new(ListValue)

		list.datatype = datatype
		list.values = make([]RegisterValue, size)

		return list, nil

	default:
		return nil, errors.New("type is not an v/array")
	}
}

func (list ListValue) Type() *Datatype { return list.datatype }

func (list ListValue) Copy() RegisterValue {
	lcopy := ListValue{values: make([]RegisterValue, len(list.values))}
	lcopy.datatype = list.datatype.Copy()

	for idx, val := range list.values {
		lcopy.values[idx] = val.Copy()
	}

	return lcopy
}

func (list ListValue) Norm() any {
	norm := make([]any, 0, len(list.values))
	for _, v := range list.values {
		norm = append(norm, v.Norm())
	}

	return norm
}

func (list ListValue) Data() []byte {
	polorizer := polo.NewPolorizer()
	for _, val := range list.values {
		_ = polorizer.PolorizeAny(val.Data())
	}

	return polorizer.Packed()
}

func (list *ListValue) Get(index RegisterValue) (RegisterValue, *Exception) {
	if !index.Type().Equals(TypeU64) {
		return nil, exceptionf(TypeError, "invalid %v index: not a uint64", list.datatype.Kind)
	}

	listIndex := index.(U64Value) //nolint:forcetypeassert
	if listIndex >= list.Size() {
		return nil, exceptionf(AccessError, "invalid %v index: out of bounds", list.datatype.Kind)
	}

	value := list.values[listIndex]
	if value == nil {
		// At this point, we know the data is supposed to be an initialized element in the list.
		// So, if the element is null, we return the zero value for the type
		value, _ = NewRegisterValue(list.datatype.Elem, nil)
	}

	return value, nil
}

func (list *ListValue) Set(index RegisterValue, element RegisterValue) *Exception {
	if !index.Type().Equals(TypeU64) {
		return exceptionf(TypeError, "invalid %v index: not a uint64", list.datatype.Kind)
	}

	listIndex := index.(U64Value) //nolint:forcetypeassert
	if listIndex >= list.Size() {
		return exceptionf(AccessError, "invalid %v index: out of bounds", list.datatype.Kind)
	}

	if !list.datatype.Elem.Equals(element.Type()) {
		exceptionf(TypeError, "invalid %v element: not a %v", list.datatype.Kind, list.datatype.Elem)
	}

	list.values[listIndex] = element

	return nil
}

func (list ListValue) Size() U64Value {
	if list.datatype.Kind == ArrayType {
		return U64Value(list.datatype.Size)
	}

	return U64Value(len(list.values))
}

func (list *ListValue) Append(value RegisterValue) error {
	if list.Type().Kind != VarrayType {
		return errors.New("not a varray")
	}

	if !list.datatype.Elem.Equals(value.Type()) {
		return errors.Errorf("invalid varray element: not a %v", list.datatype.Elem)
	}

	list.values = append(list.values, value)

	return nil
}

func (list *ListValue) Popend() (RegisterValue, error) {
	if list.Type().Kind != VarrayType {
		return nil, errors.New("not a varray")
	}

	if list.Size() == 0 {
		return nil, errors.New("varray is empty")
	}

	element := list.values[list.Size()-1]
	list.values = list.values[:list.Size()-1]

	return element, nil
}

func (list *ListValue) Grow(size U64Value) error {
	if list.Type().Kind != VarrayType {
		return errors.New("not a varray")
	}

	list.values = append(list.values, make([]RegisterValue, size)...)

	return nil
}

// MapValue represents a RegisterValue that operates like a mapping.
type MapValue struct {
	values   map[RegisterValue]RegisterValue
	datatype *Datatype
}

// newMapValue generates a new MapValue for a given Datatype and some POLO encoded bytes.
func newMapValue(dt *Datatype, data []byte) (*MapValue, error) {
	// Check if datatype is a map
	if dt.Kind != MappingType {
		return nil, errors.New("datatype is not a mapping")
	}

	// Initialize the MapValue with the Typedef and an empty mapping
	mapping := new(MapValue)
	mapping.datatype = dt
	mapping.values = make(map[RegisterValue]RegisterValue)

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

			var key, val RegisterValue

			// Create new value from the data for the key
			if key, err = NewRegisterValue(dt.Prim.Datatype(), kdata); err != nil {
				return nil, err
			}

			// Create new value from the data for the value
			if val, err = NewRegisterValue(dt.Elem, vdata); err != nil {
				return nil, err
			}

			mapping.values[key] = val
		}
	}

	return mapping, nil
}

// Type returns the Datatype of MapValue, which is some Mapping Datatype.
// Implements the RegisterValue interface for MapValue.
func (mapping MapValue) Type() *Datatype { return mapping.datatype }

// Copy returns a copy of MapValue as a RegisterValue.
// Implements the RegisterValue interface for MapValue.
func (mapping MapValue) Copy() RegisterValue {
	mcopy := MapValue{values: make(map[RegisterValue]RegisterValue, len(mapping.values))}
	mcopy.datatype = mapping.datatype.Copy()

	for key, val := range mapping.values {
		mcopy.values[key.Copy()] = val.Copy()
	}

	return mcopy
}

// Norm returns the normalized value of MapValue as a map[any]any.
// Implements the RegisterValue interface for MapValue.
func (mapping MapValue) Norm() any {
	norm := make(map[any]any, len(mapping.values))
	for k, v := range mapping.values {
		norm[k.Norm()] = v.Norm()
	}

	return norm
}

// Data returns the POLO encoded bytes of MapValue.
// Implements the RegisterValue interface for MapValue.
func (mapping MapValue) Data() []byte {
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
	if !mapping.datatype.Prim.Equals(key.Type().Prim) {
		return nil, exceptionf(TypeError, "invalid map key: not a %v", mapping.datatype.Prim)
	}

	value := mapping.values[key]
	if value == nil {
		// todo: this should not happen, we need to return NullValue{} and AccessError instead
		// If value is nil, generate the default value for the map element type
		value, _ = NewRegisterValue(mapping.datatype.Elem, nil)
	}

	return value, nil
}

// Set is a safe write into the MapValue, returns an error if
// either the key or value are not the correct type for MapValue
func (mapping *MapValue) Set(key, val RegisterValue) *Exception {
	// We can safely access Prim, because it will default to
	// PrimitiveNull if the key's datatype is not Primitive
	if !mapping.datatype.Prim.Equals(key.Type().Prim) {
		return exceptionf(TypeError, "invalid map key: not a %v", mapping.datatype.Prim)
	}

	if !mapping.datatype.Elem.Equals(val.Type()) {
		exceptionf(TypeError, "invalid map value: not a %v", mapping.datatype.Elem)
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
	if !mapping.datatype.Prim.Equals(key.Type().Prim) {
		return false, exceptionf(TypeError, "invalid map key: not a %v", mapping.datatype.Prim)
	}

	_, ok := mapping.values[key]

	return BoolValue(ok), nil
}

package register

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type ListValue struct {
	values  []Value
	typedef *Typedef
}

func NewListValue(typedef *Typedef, data []byte) (*ListValue, error) {
	list := new(ListValue)
	list.typedef = typedef

	switch typedef.Kind() {
	case Array:
		list.values = make([]Value, typedef.S)
	case Varray:
		list.values = make([]Value, 0)
	default:
		return nil, errors.New("type is not an array or varray")
	}

	if data != nil {
		unpacker, err := polo.NewUnpacker(data)
		if err != nil {
			return nil, err
		}

		var index uint64

		for !unpacker.Done() {
			var edata []byte
			// Unpack the element data from the wire
			if edata, err = unpacker.UnpackWire(); err != nil {
				return nil, err
			}

			var element Value
			// Create new value from the data for the element
			if element, err = NewValue(typedef.E, edata); err != nil {
				return nil, err
			}

			switch typedef.Kind() {
			case Array:
				if index >= typedef.S {
					return nil, errors.New("too many elements in data")
				}

				list.values[index] = element

			case Varray:
				list.values = append(list.values, element)
			}

			index++
		}
	}

	return list, nil
}

func (list ListValue) Type() *Typedef { return list.typedef }

func (list ListValue) Copy() Value {
	lcopy := ListValue{values: make([]Value, len(list.values))}
	lcopy.typedef = list.typedef.Copy()

	for idx, val := range list.values {
		lcopy.values[idx] = val.Copy()
	}

	return lcopy
}

func (list ListValue) Data() []byte {
	packer := polo.NewPacker()
	for _, val := range list.values {
		_ = packer.PackWire(val.Data())
	}

	return packer.Bytes()
}

func (list *ListValue) Get(index Value) (Value, error) {
	if !index.Type().Equals(TypeU64) {
		return nil, errors.New("cannot access list element without uint64 index")
	}

	listIndex := index.(U64Value) //nolint:forcetypeassert
	if listIndex >= list.Size() {
		return nil, errors.New("cannot access list element: index out of bounds")
	}

	value := list.values[listIndex]
	if value == nil {
		value, _ = NewValue(list.typedef.E, nil)
	}

	return value, nil
}

func (list *ListValue) Set(index Value, element Value) error {
	if !index.Type().Equals(TypeU64) {
		return errors.New("cannot access list element without uint64 index")
	}

	listIndex := index.(U64Value) //nolint:forcetypeassert
	if listIndex >= list.Size() {
		return errors.New("cannot access list element: index out of bounds")
	}

	if !list.typedef.E.Equals(element.Type()) {
		return errors.New("cannot set list element with invalid type")
	}

	list.values[listIndex] = element

	return nil
}

func (list ListValue) Size() U64Value {
	if list.typedef.Kind() == Array {
		return U64Value(list.typedef.S)
	}

	return U64Value(len(list.values))
}

package register

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

type ListValue struct {
	values   []Value
	datatype *engineio.Datatype
}

func NewListValue(datatype *engineio.Datatype, data []byte) (*ListValue, error) {
	list := new(ListValue)
	list.datatype = datatype

	switch datatype.Kind {
	case engineio.ArrayType:
		list.values = make([]Value, datatype.Size)
	case engineio.VarrayType:
		list.values = make([]Value, 0)
	default:
		return nil, errors.New("type is not an array or varray")
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

			var element Value
			// Create new value from the data for the element
			if element, err = NewValue(datatype.Elem, edata); err != nil {
				return nil, err
			}

			switch datatype.Kind {
			case engineio.ArrayType:
				if index >= datatype.Size {
					return nil, errors.New("too many elements in data")
				}

				list.values[index] = element

			case engineio.VarrayType:
				list.values = append(list.values, element)
			}

			index++
		}
	}

	return list, nil
}

func (list ListValue) Type() *engineio.Datatype { return list.datatype }

func (list ListValue) Copy() Value {
	lcopy := ListValue{values: make([]Value, len(list.values))}
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

	return polorizer.Bytes()
}

func (list *ListValue) Get(index Value) (Value, error) {
	if !index.Type().Equals(engineio.TypeU64) {
		return nil, errors.New("cannot access list element without uint64 index")
	}

	listIndex := index.(U64Value) //nolint:forcetypeassert
	if listIndex >= list.Size() {
		return nil, errors.New("cannot access list element: index out of bounds")
	}

	value := list.values[listIndex]
	if value == nil {
		value, _ = NewValue(list.datatype.Elem, nil)
	}

	return value, nil
}

func (list *ListValue) Set(index Value, element Value) error {
	if !index.Type().Equals(engineio.TypeU64) {
		return errors.New("cannot access list element without uint64 index")
	}

	listIndex := index.(U64Value) //nolint:forcetypeassert
	if listIndex >= list.Size() {
		return errors.New("cannot access list element: index out of bounds")
	}

	if !list.datatype.Elem.Equals(element.Type()) {
		return errors.New("cannot set list element with invalid type")
	}

	list.values[listIndex] = element

	return nil
}

func (list ListValue) Size() U64Value {
	if list.datatype.Kind == engineio.ArrayType {
		return U64Value(list.datatype.Size)
	}

	return U64Value(len(list.values))
}

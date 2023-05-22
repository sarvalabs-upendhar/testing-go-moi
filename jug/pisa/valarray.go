package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type ArrayValue struct {
	values   []RegisterValue
	datatype ArrayDatatype
}

func newArrayValue(datatype ArrayDatatype, data []byte) (*ArrayValue, error) {
	array := new(ArrayValue)

	array.datatype = datatype
	array.values = make([]RegisterValue, datatype.size)

	if data == nil {
		return array, nil
	}

	values, err := decodeListedValues(data, array.datatype.elem)
	if err != nil {
		return nil, err
	}

	if uint64(len(values)) != array.datatype.size {
		return nil, errors.New("")
	}

	array.values = values

	return array, nil
}

func newArrayFromValues(datatype ArrayDatatype, values ...RegisterValue) (*ArrayValue, error) {
	if uint64(len(values)) != datatype.size {
		return nil, errors.Errorf("incorrect number of values for array of size %v", datatype.size)
	}

	array := new(ArrayValue)

	array.datatype = datatype
	array.values = make([]RegisterValue, datatype.size)

	for idx, value := range values {
		if value == nil {
			continue
		}

		if value.Type() != array.datatype.elem {
			return nil, errors.Errorf("incorrect value type for v/array with element %v", datatype.elem)
		}

		array.values[idx] = value
	}

	return array, nil
}

func (array ArrayValue) Type() Datatype { return array.datatype }

func (array ArrayValue) Copy() RegisterValue {
	//nolint:forcetypeassert
	clone := &ArrayValue{datatype: array.datatype.Copy().(ArrayDatatype)}
	// Skip value cloning if values are empty
	if array.values == nil {
		return clone
	}

	// Initialize clone values
	clone.values = make([]RegisterValue, len(array.values))
	// Copy each value from the original into the clone
	for idx, val := range array.values {
		// Skip the copy if value for index is nil
		if val == nil {
			continue
		}

		clone.values[idx] = val.Copy()
	}

	return clone
}

func (array ArrayValue) Data() []byte {
	polorizer := polo.NewPolorizer()

	for _, val := range array.values {
		if val != nil {
			_ = polorizer.PolorizeAny(val.Data())
		} else {
			_ = polorizer.PolorizeAny(nil)
		}
	}

	return polorizer.Packed()
}

func (array ArrayValue) Norm() any {
	norm := make([]any, 0, len(array.values))

	for _, v := range array.values {
		norm = append(norm, v.Norm())
	}

	return norm
}

func (array ArrayValue) Get(index RegisterValue) (RegisterValue, *Exception) {
	if !index.Type().Equals(PrimitiveU64) {
		return nil, exception(TypeError, "invalid array index: not a u64")
	}

	arrayIndex := index.(U64Value) //nolint:forcetypeassert
	if arrayIndex >= array.Size() {
		return nil, exception(AccessError, "invalid array index: out of bounds")
	}

	value := array.values[arrayIndex]
	if value == nil {
		// At this point, we know the data is supposed to be an initialized element in the array.
		// So, if the element is null, we return the zero value for the type
		value, _ = NewRegisterValue(array.datatype.elem, nil)
	}

	return value, nil
}

func (array *ArrayValue) Set(index RegisterValue, element RegisterValue) *Exception {
	if !index.Type().Equals(PrimitiveU64) {
		return exceptionf(TypeError, "invalid array index: not a u64")
	}

	arrayIndex := index.(U64Value) //nolint:forcetypeassert
	if arrayIndex >= array.Size() {
		return exception(AccessError, "invalid array index: out of bounds")
	}

	if !array.datatype.elem.Equals(element.Type()) {
		exceptionf(TypeError, "invalid array element: not a %v", array.datatype.elem)
	}

	array.values[arrayIndex] = element

	return nil
}

func (array ArrayValue) Size() U64Value {
	return U64Value(array.datatype.size)
}

func (array *ArrayValue) Slice(start RegisterValue, stop RegisterValue) (*VarrayValue, *Exception) {
	if !start.Type().Equals(PrimitiveU64) {
		return nil, exception(TypeError, "invalid array index for slice start: not a u64")
	}

	if !stop.Type().Equals(PrimitiveU64) {
		return nil, exception(TypeError, "invalid array index for slice stop: not a u64")
	}

	startIdx, stopIdx := start.(U64Value), stop.(U64Value) //nolint:forcetypeassert

	// Verify slice index bounds
	if stopIdx.Gt(array.Size()) || startIdx.Gt(stopIdx) {
		return nil, exception(ValueError, "invalid array index for slice: out of range")
	}

	// Slice the values in a temporary Array
	sliced := array.values[startIdx:stopIdx]

	// Create a new Varray from the values
	varray, err := newVarrayFromValues(VarrayDatatype{array.datatype.elem}, sliced...)
	if err != nil {
		return nil, exception(ValueError, err.Error())
	}

	return varray, nil
}

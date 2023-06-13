package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type VarrayValue struct {
	values   []RegisterValue
	datatype VarrayDatatype
}

func newVarrayValue(datatype VarrayDatatype, data []byte) (*VarrayValue, error) {
	varray := new(VarrayValue)

	varray.datatype = datatype
	varray.values = make([]RegisterValue, 0)

	if data == nil {
		return varray, nil
	}

	values, err := decodeListedValues(data, varray.datatype.elem)
	if err != nil {
		return nil, err
	}

	varray.values = values

	return varray, nil
}

func newVarrayFromValues(datatype VarrayDatatype, values ...RegisterValue) (*VarrayValue, error) {
	varray := new(VarrayValue)

	varray.datatype = datatype
	varray.values = make([]RegisterValue, 0)

	for _, value := range values {
		if value == nil {
			continue
		}

		if value.Type() != varray.datatype.elem {
			return nil, errors.Errorf("incorrect value type for v/array with element %v", datatype.elem)
		}

		varray.values = append(varray.values, value)
	}

	return varray, nil
}

func newVarrayWithSize(datatype VarrayDatatype, size uint64) *VarrayValue {
	varray := new(VarrayValue)

	varray.datatype = datatype
	varray.values = make([]RegisterValue, size)

	return varray
}

func (varray *VarrayValue) Type() Datatype { return varray.datatype }

func (varray *VarrayValue) Copy() RegisterValue {
	//nolint:forcetypeassert
	clone := &VarrayValue{datatype: varray.datatype.Copy().(VarrayDatatype)}
	// Skip value cloning if values are empty
	if varray.values == nil {
		return clone
	}

	// Initialize clone values
	clone.values = make([]RegisterValue, len(varray.values))
	// Copy each value from the original into the clone
	for idx, val := range varray.values {
		// Skip the copy if value for index is nil
		if val == nil {
			continue
		}

		clone.values[idx] = val.Copy()
	}

	return clone
}

func (varray *VarrayValue) Data() []byte {
	polorizer := polo.NewPolorizer()

	for _, val := range varray.values {
		if val != nil {
			_ = polorizer.PolorizeAny(val.Data())
		} else {
			_ = polorizer.PolorizeAny(nil)
		}
	}

	return polorizer.Packed()
}

func (varray *VarrayValue) Norm() any {
	norm := make([]any, 0, len(varray.values))

	for _, v := range varray.values {
		norm = append(norm, v.Norm())
	}

	return norm
}

func (varray *VarrayValue) Get(index RegisterValue) (RegisterValue, *Exception) {
	if !index.Type().Equals(PrimitiveU64) {
		return nil, exception(TypeError, "invalid varray index: not a uint64")
	}

	varrayIndex := index.(U64Value) //nolint:forcetypeassert
	if varrayIndex >= varray.Size() {
		return nil, exception(AccessError, "invalid varray index: out of bounds")
	}

	value := varray.values[varrayIndex]
	if value == nil {
		// At this point, we know the data is supposed to be an initialized element in the varray.
		// So, if the element is null, we return the zero value for the type
		value, _ = NewRegisterValue(varray.datatype.elem, nil)
	}

	return value, nil
}

func (varray *VarrayValue) Set(index RegisterValue, element RegisterValue) *Exception {
	if !index.Type().Equals(PrimitiveU64) {
		return exceptionf(TypeError, "invalid varray index: not a uint64")
	}

	varrayIndex := index.(U64Value) //nolint:forcetypeassert
	if varrayIndex >= varray.Size() {
		return exception(AccessError, "invalid varray index: out of bounds")
	}

	if !varray.datatype.elem.Equals(element.Type()) {
		exceptionf(TypeError, "invalid varray element: not a %v", varray.datatype.elem)
	}

	varray.values[varrayIndex] = element

	return nil
}

func (varray *VarrayValue) Size() U64Value {
	return U64Value(len(varray.values))
}

func (varray *VarrayValue) Append(value RegisterValue) error {
	if !varray.datatype.elem.Equals(value.Type()) {
		return errors.Errorf("invalid varray element: not a %v", varray.datatype.elem)
	}

	varray.values = append(varray.values, value)

	return nil
}

func (varray *VarrayValue) Popend() (RegisterValue, error) {
	if varray.Size() == 0 {
		return nil, errors.New("varray is empty")
	}

	element := varray.values[varray.Size()-1]
	varray.values = varray.values[:varray.Size()-1]

	return element, nil
}

func (varray *VarrayValue) Grow(size U64Value) {
	varray.values = append(varray.values, make([]RegisterValue, size)...)
}

func (varray *VarrayValue) Merge(insert *VarrayValue) *VarrayValue {
	// Copy the varray values into a new varray
	merged, _ := varray.Copy().(*VarrayValue)
	// Append each value into the new merged varray
	merged.values = append(merged.values, insert.values...)

	return merged
}

func (varray *VarrayValue) Slice(start RegisterValue, stop RegisterValue) (*VarrayValue, *Exception) {
	if !start.Type().Equals(PrimitiveU64) {
		return nil, exception(TypeError, "invalid array index for slice start: not a u64")
	}

	if !stop.Type().Equals(PrimitiveU64) {
		return nil, exception(TypeError, "invalid array index for slice stop: not a u64")
	}

	startIdx, stopIdx := start.(U64Value), stop.(U64Value) //nolint:forcetypeassert

	// Verify slice index bounds
	if stopIdx.Gt(varray.Size()) || startIdx.Gt(stopIdx) {
		return nil, exception(ValueError, "invalid array index for slice: out of range")
	}

	// Slice the values in a temporary Array
	sliced := varray.values[startIdx:stopIdx]

	// Create a new Varray from the values
	slicedVarray, err := newVarrayFromValues(VarrayDatatype{varray.datatype.elem}, sliced...)
	if err != nil {
		return nil, exception(ValueError, err.Error())
	}

	return slicedVarray, nil
}

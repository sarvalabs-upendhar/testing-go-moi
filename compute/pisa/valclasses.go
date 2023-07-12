package pisa

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// ClassValue represents a RegisterValue that operates like a class struct.
// It implements the SlottedValue interface.
type ClassValue struct {
	values   map[byte]RegisterValue
	datatype ClassDatatype
}

// newClassValue generates a new ClassValue for a given ClassDatatype and some doc-encoded POLO encoded bytes.
func newClassValue(dt ClassDatatype, data []byte) (*ClassValue, error) {
	// Check if datatype is a class
	if dt.Kind() != Class {
		return nil, errors.New("datatype is not a class")
	}

	// Initialize the ClassValue with the Typedef and an empty value
	class := new(ClassValue)
	class.datatype = dt
	class.values = make(map[byte]RegisterValue)

	// If there is no data to decode, return empty ClassValue
	if data == nil {
		return class, nil
	}

	values, err := decodeSlottedValues(data, class.datatype.fields)
	if err != nil {
		return nil, err
	}

	class.values = values

	return class, nil
}

// Type returns the Datatype of ClassValue, which is some class Datatype.
// Implements the RegisterValue interface for ClassValue.
func (class *ClassValue) Type() Datatype { return class.datatype }

// Copy returns a copy of ClassValue as a RegisterValue.
// Implements the RegisterValue interface for ClassValue.
func (class *ClassValue) Copy() RegisterValue {
	//nolint:forcetypeassert
	clone := &ClassValue{
		datatype: class.datatype.Copy().(ClassDatatype),
		values:   make(map[byte]RegisterValue, len(class.values)),
	}

	for slot, val := range class.values {
		clone.values[slot] = val.Copy()
	}

	return clone
}

// Data returns the POLO encoded bytes of ClassValue.
// Implements the Value interface for ClassValue.
func (class *ClassValue) Data() []byte {
	doc := make(polo.Document)

	for slot, value := range class.values {
		field := class.datatype.fields.Get(slot)
		doc.SetRaw(field.Name, value.Data())
	}

	return doc.Bytes()
}

// Norm returns the normalized value of ClassValue as a map[string]any.
// Implements the Value interface for ClassValue.
func (class *ClassValue) Norm() any {
	norm := make(map[string]any, len(class.values))

	for slot, val := range class.values {
		field := class.datatype.fields.Get(slot)
		norm[field.Name] = val.Norm()
	}

	return norm
}

// Size returns the number of fields in the ClassValue
func (class *ClassValue) Size() U64Value {
	return U64Value(class.datatype.fields.Size())
}

// Get is a safe read from the ClassValue, returns an
// error if the slot is not valid for the ClassValue
func (class *ClassValue) Get(slot uint8) (RegisterValue, *Exception) {
	if slot >= uint8(class.Size()) {
		return nil, exceptionf(AccessError, "invalid class field: &%v", slot)
	}

	value := class.values[slot]
	if value == nil {
		value, _ = NewRegisterValue(class.datatype.fields.Get(slot).Type, nil)
	}

	return value, nil
}

// Set is a safe write into the ClassValue, returns an error if either
// the slot is invalid or value is not the correct type for ClassValue
func (class *ClassValue) Set(slot uint8, value RegisterValue) *Exception {
	if slot >= uint8(class.Size()) {
		return exceptionf(AccessError, "invalid field slot: &%v", slot)
	}

	field := class.datatype.fields.Get(slot)
	if !field.Type.Equals(value.Type()) {
		return exceptionf(TypeError, "invalid field value: not a %v", field.Type)
	}

	class.values[slot] = value

	return nil
}

// CargsValue represents a RegisterValue that is used for bundling call arguments.
// It implements the SlottedValue interface
type CargsValue map[byte]RegisterValue

// Type returns the Datatype of CargsValue, which is PrimitiveCargs
// Implements the RegisterValue interface for CargsValue.
func (cargs CargsValue) Type() Datatype { return PrimitiveCargs }

// Copy returns a copy of CargsValue as a RegisterValue.
// Implements the RegisterValue interface for CargsValue.
func (cargs CargsValue) Copy() RegisterValue {
	clone := make(CargsValue)

	for slot, value := range cargs {
		clone[slot] = value.Copy()
	}

	return clone
}

// Data returns the POLO encoded bytes of CargsValue.
// Implements the Value interface for CargsValue.
func (cargs CargsValue) Data() []byte {
	doc := make(polo.Document)

	for slot, value := range cargs {
		doc.SetRaw(fmt.Sprintf("%d", slot), value.Data())
	}

	return doc.Bytes()
}

// Norm returns the normalized value of CargsValue as a map[string]any.
// Implements the Value interface for CargsValue.
func (cargs CargsValue) Norm() any {
	norm := make(map[string]any, len(cargs))

	for slot, val := range cargs {
		norm[fmt.Sprintf("%d", slot)] = val.Norm()
	}

	return norm
}

// Size returns the number of used slots in the CargsValue
func (cargs CargsValue) Size() U64Value {
	return U64Value(len(cargs))
}

// Get is a safe read from the CargsValue
func (cargs CargsValue) Get(slot uint8) (RegisterValue, *Exception) {
	value := cargs[slot]
	if value == nil {
		return NullValue{}, nil
	}

	return value, nil
}

// Set is a safe write into the CargsValue
func (cargs CargsValue) Set(slot uint8, value RegisterValue) *Exception {
	cargs[slot] = value

	return nil
}

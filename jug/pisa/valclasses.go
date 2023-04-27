package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// ClassValue represents a RegisterValue that operates like a class struct.
type ClassValue struct {
	values   map[byte]RegisterValue
	datatype *Datatype
}

// NewClassValue generates a new ClassValue for a given engineio.Datatype and some POLO encoded bytes.
func NewClassValue(dt *Datatype, data []byte) (*ClassValue, error) {
	// Check if datatype is a class
	if dt.Kind != ClassType {
		return nil, errors.New("datatype is not a class")
	}

	// Initialize the ClassValue with the Typedef and an empty value
	class := new(ClassValue)
	class.datatype = dt
	class.values = make(map[byte]RegisterValue)

	// If there is some data to decode into the ClassValue
	// Unpack each element into
	if data != nil {
		doc := make(polo.Document)
		if err := polo.Depolorize(&doc, data); err != nil {
			return nil, err
		}

		for key, raw := range doc {
			// Get the field type from the class def
			field := dt.Fields.Lookup(key)
			if field == nil {
				return nil, errors.Errorf("invalid data for '%v' field '%v': no such field", dt.Ident, key)
			}

			// Create new value from the data for the key
			value, err := NewRegisterValue(field.Type, raw)
			if err != nil {
				return nil, err
			}

			// Get the slot for the field and insert it
			slot := dt.Fields.Symbols[field.Name]
			class.values[slot] = value
		}
	}

	return class, nil
}

// Type returns the Datatype of ClassValue, which is some class Datatype.
// Implements the RegisterValue interface for ClassValue.
func (class ClassValue) Type() *Datatype { return class.datatype }

// Copy returns a copy of ClassValue as a RegisterValue.
// Implements the RegisterValue interface for ClassValue.
func (class ClassValue) Copy() RegisterValue {
	clone := ClassValue{values: make(map[byte]RegisterValue, len(class.values))}
	clone.datatype = class.datatype.Copy()

	for slot, val := range class.values {
		clone.values[slot] = val.Copy()
	}

	return clone
}

// Norm returns the normalized value of ClassValue as a map[string]any.
// Implements the Value interface for ClassValue.
func (class ClassValue) Norm() any {
	norm := make(map[string]any, len(class.values))

	for slot, val := range class.values {
		field := class.datatype.Fields.Get(slot)
		norm[field.Name] = val.Norm()
	}

	return norm
}

// Data returns the POLO encoded bytes of ClassValue.
// Implements the Value interface for ClassValue.
func (class ClassValue) Data() []byte {
	doc := make(polo.Document)

	for slot, value := range class.values {
		field := class.datatype.Fields.Get(slot)
		doc.SetRaw(field.Name, value.Data())
	}

	return doc.Bytes()
}

// Get is a safe read from the ClassValue, returns an
// error if the slot is not valid for the ClassValue
func (class *ClassValue) Get(slot uint8) (RegisterValue, error) {
	if slot >= class.Size() {
		return nil, errors.Errorf("undefined class field slot '%v'", slot)
	}

	value := class.values[slot]
	if value == nil {
		value, _ = NewRegisterValue(class.datatype.Fields.Get(slot).Type, nil)
	}

	return value, nil
}

// Set is a safe write into the ClassValue, returns an error if either
// the slot is invalid or value is not the correct type for ClassValue
func (class *ClassValue) Set(slot uint8, value RegisterValue) error {
	if slot >= class.Size() {
		return errors.Errorf("undefined class field slot '%v'", slot)
	}

	if !class.datatype.Fields.Get(slot).Type.Equals(value.Type()) {
		return errors.New("cannot set field value with invalid type")
	}

	class.values[slot] = value

	return nil
}

// Size returns the number of fields in the ClassValue
func (class ClassValue) Size() uint8 {
	return uint8(len(class.values))
}

package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// RegisterSet is a collection of byte indexed RegisterValue objects.
type RegisterSet map[byte]RegisterValue

// NewRegisterSet generates a RegisterSet for given set of type fields and values as a polo.Document.
// Each field in the TypeFields must have some associated data in the values that can be interpreted for its type.
// A RegisterValue is generated with this data and attached to the table index specified by the TypeFields.
// Returns an error if data is missing for a field or is malformed and cannot be interpreted for a field's type.
func NewRegisterSet(fields *TypeFields, values polo.Document) (RegisterSet, error) {
	registers := make(RegisterSet, len(fields.Symbols))

	// If the value is nil, but fields are expected
	if values == nil && fields.Size() != 0 {
		return nil, errors.New("missing input values")
	}

	for label, index := range fields.Symbols {
		data := values.GetRaw(label)
		if data == nil {
			return nil, errors.Errorf("missing data for '%v'", label)
		}

		fieldVal, err := NewRegisterValue(fields.Lookup(label).Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed data for '%v'", label)
		}

		registers[index] = fieldVal
	}

	return registers, nil
}

// Get retrieves a RegisterValue for a given address.
// Returns a NullValue if there is no value for the address.
func (registers RegisterSet) Get(id byte) RegisterValue {
	if reg, ok := registers[id]; ok {
		return reg
	}

	return NullValue{}
}

// Set inserts a RegisterValue to a given address.
// Overwrites any existing RegisterValue at the address.
func (registers RegisterSet) Set(id byte, reg RegisterValue) {
	registers[id] = reg
}

// Unset clears a RegisterValue at a given address
func (registers RegisterSet) Unset(id byte) {
	delete(registers, id)
}

func (registers RegisterSet) Validate(fields *TypeFields, fillNulls bool) (err error) {
	for slot, field := range fields.Table {
		value := registers.Get(slot)
		if value.Type() == PrimitiveNull {
			// If fill nulls flag is not set, we assert the missing value
			if !fillNulls {
				return errors.Errorf("missing value for field &%v '%v'", slot, field.Name)
			}

			// Create a zero value for the type
			value, err = NewRegisterValue(field.Type, nil)
			if err != nil {
				return errors.Wrapf(err, "failed to fill value for &%v '%v'", slot, field.Name)
			}

			// Update the zero value to register set
			registers.Set(slot, value)
		}

		if !value.Type().Equals(field.Type) {
			return errors.Errorf(
				"type mismatch for field &%v '%v'. expected: %v. got: %v",
				slot, field.Name, field.Type, value.Type(),
			)
		}
	}

	return nil
}

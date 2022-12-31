package register

import (
	"github.com/pkg/errors"

	ctypes "github.com/sarvalabs/moichain/jug/types"
)

// ValueTable is a collection of byte indexed Value objects.
type ValueTable map[byte]Value

// NewValueTable generates a RegisterTable for given set of fields and values as engine.Values.
// Each field in the FieldTable must have some associated data in the values that can be interpreted for its type.
// A RegisterValue is generated with this data and attached to the table index specified by the FieldTable.
// Returns an error if data is missing for a field or is malformed and cannot be interpreted for a field's type.
func NewValueTable(fields FieldTable, values ctypes.ExecutionValues) (ValueTable, error) {
	table := make(ValueTable, len(fields.Symbols))

	// If there are fields expected but values is nil
	if len(fields.Symbols) != 0 && values == nil {
		return nil, errors.New("missing input values")
	}

	for label, index := range fields.Symbols {
		data := values.Get(label)
		if data == nil {
			return nil, errors.Errorf("missing data for '%v'", label)
		}

		field := fields.Lookup(label)

		fieldVal, err := NewValue(field.Type, data)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed data for '%v'", label)
		}

		table[index] = fieldVal
	}

	return table, nil
}

// Get retrieves a RegisterValue for a given address.
// Returns a NullValue if there is no value for the address.
func (registers ValueTable) Get(id byte) (Value, bool) {
	if reg, ok := registers[id]; ok {
		return reg, true
	}

	return NullValue{}, false
}

// Set inserts a RegisterValue to a given address.
// Overwrites any existing RegisterValue at the address.
func (registers ValueTable) Set(id byte, reg Value) {
	registers[id] = reg
}

// Unset clears a RegisterValue at a given address
func (registers ValueTable) Unset(id byte) {
	delete(registers, id)
}

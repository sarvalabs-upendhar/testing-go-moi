package register

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/types"
)

// StateFields represents the state symbols for a Logic
type StateFields = FieldTable

// Signature generates a signature from the RoutineFields symbols and their typedata.
// It is structured as '(input1, input2)->(output1, output2)', where the values are type data of each field
func (fields CallFields) Signature() string {
	return fmt.Sprintf("%v->%v", fields.Inputs.String(), fields.Outputs.String())
}

// SigHash generates a signature hash from the RoutineFields symbols and their typedata.
// The signature is hashed and the last 8 characters of the digest are returned as a string.
func (fields CallFields) SigHash() string {
	return types.GetHash([]byte(fields.Signature())).Hex()[:8]
}

// FieldTable represents a collection of typefield objects.
// The fields are indexed by both their position and name.
type FieldTable struct {
	Table   map[uint8]*TypeField
	Symbols map[string]uint8
}

// String returns the fields of the FieldTable as a string.
// Format of the returned string is '(type1, type2, ...)'.
// Implements the Stringer interface for FieldTable.
func (fields FieldTable) String() string {
	fieldTypes := make([]string, 0, len(fields.Symbols))
	for position := uint8(0); position < uint8(len(fields.Table)); position++ {
		fieldTypes = append(fieldTypes, fields.Table[position].Type.String())
	}

	combined := strings.Join(fieldTypes, ", ")
	combined = "(" + combined + ")"

	return combined
}

// Get retrieves a typefield from the FieldTable for a given position.
// Returns nil if there is typefield for that position
func (fields FieldTable) Get(position uint8) *TypeField {
	return fields.Table[position]
}

// Lookup retrieves a typefield from the FieldTable for a given name.
// Returns nil if there is no typefield for that name.
func (fields FieldTable) Lookup(name string) *TypeField {
	index, exists := fields.Symbols[name]
	if !exists {
		return nil
	}

	return fields.Table[index]
}

func (fields FieldTable) Validate(values ValueTable) error {
	for idx, field := range fields.Table {
		value, ok := values.Get(idx)
		if !ok {
			return errors.Errorf("missing value for field &%v '%v'", idx, field.Name)
		}

		if !value.Type().Equals(field.Type) {
			return errors.Errorf(
				"type mismatch for field &%v '%v'. expected: %v. got: %v",
				idx, field.Name, field.Type, value.Type(),
			)
		}
	}

	return nil
}

// TypeField represent a named field for composite object such
// as storage and calldata fields as well as class attributes
type TypeField struct {
	Name string
	Type *Typedef
}

func makefields(fields []*TypeField) *FieldTable {
	// Ensure that there are less than 256 field expressions
	// This is an internal call so, is alright to panic
	if len(fields) > 256 {
		panic("cannot have more than 256 fields for FieldTable")
	}

	// Create a blank field table
	table := &FieldTable{
		Table:   make(map[uint8]*TypeField, len(fields)),
		Symbols: make(map[string]uint8, len(fields)),
	}

	for position, field := range fields {
		table.Table[uint8(position)] = field
		table.Symbols[field.Name] = uint8(position)
	}

	return table
}

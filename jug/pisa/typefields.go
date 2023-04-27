package pisa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sarvalabs/moichain/types"
)

// TypeField represent a named field for composite object such
// as storage and calldata fields as well as class attributes
type TypeField struct {
	Name string
	Type *Datatype // implements a method (datatype Datatype) Equals(other *Datatype) bool
}

// TypeFields represents a collection of TypeField objects.
// The fields are indexed by both their position and name.
type TypeFields struct {
	Table   map[uint8]*TypeField
	Symbols map[string]uint8
}

// Insert inserts a TypeField into the TypeFields for a given position.
// Any existing field in that position is overwritten.
func (fields *TypeFields) Insert(position uint8, field *TypeField) {
	fields.Table[position] = field
	fields.Symbols[field.Name] = position
}

// Get retrieves a TypeField from the TypeFields for a given position.
// Returns nil if there is no typefield for that position.
func (fields TypeFields) Get(position uint8) *TypeField {
	return fields.Table[position]
}

// Lookup retrieves a TypeField from the TypeFields for a given name.
// Returns nil if there is no typefield for that name.
func (fields TypeFields) Lookup(name string) *TypeField {
	index, exists := fields.Symbols[name]
	if !exists {
		return nil
	}

	return fields.Table[index]
}

// Size returns the number of field symbols in the TypeFields
func (fields TypeFields) Size() uint8 {
	return uint8(len(fields.Table))
}

// String returns the fields of the TypeFields as a string.
// Format of the returned string is '(type1, type2, ...)'.
// Implements the Stringer interface for TypeFields.
func (fields TypeFields) String() string {
	fieldTypes := make([]string, 0, len(fields.Symbols))
	for position := uint8(0); position < uint8(len(fields.Table)); position++ {
		fieldTypes = append(fieldTypes, fields.Table[position].Type.String())
	}

	combined := strings.Join(fieldTypes, ", ")
	combined = "(" + combined + ")"

	return combined
}

// Copy generates a deep copy of TypeFields
func (fields TypeFields) Copy() *TypeFields {
	clone := &TypeFields{
		Table:   make(map[uint8]*TypeField, len(fields.Table)),
		Symbols: make(map[string]uint8, len(fields.Table)),
	}

	// For each field, clone the datatype and insert into the fields clone
	for position, field := range fields.Table {
		clone.Insert(position, &TypeField{field.Name, field.Type.Copy()})
	}

	return clone
}

// Equals returns whether the given TypeFields is equal to another
func (fields TypeFields) Equals(other *TypeFields) bool {
	return reflect.DeepEqual(fields, *other)
}

// makefields generates a TypeFields object from a slice of TypeField objects.
// Panics if more than 256 field elements are given.
func makefields(fields []*TypeField) *TypeFields {
	// Ensure that there are less than 256 field expressions
	// This is an internal call so, is alright to panic
	if len(fields) > 256 {
		panic("cannot have more than 256 fields for FieldTable")
	}

	// Create a blank field table
	table := &TypeFields{
		Table:   make(map[uint8]*TypeField, len(fields)),
		Symbols: make(map[string]uint8, len(fields)),
	}

	for position, field := range fields {
		table.Table[uint8(position)] = field
		table.Symbols[field.Name] = uint8(position)
	}

	return table
}

// StateFields represents the state symbols for a Logic
type StateFields = TypeFields

// CallFields represents the input/output symbols for a callable.
type CallFields struct {
	Inputs  *TypeFields
	Outputs *TypeFields
}

// Signature generates a signature from the CallFields symbols and their typedata.
// It is structured as '(input1, input2)->(output1, output2)', where the values are type data of each field
func (fields CallFields) Signature() string {
	return fmt.Sprintf("%v->%v", fields.Inputs.String(), fields.Outputs.String())
}

// SigHash generates a signature hash from the CallFields symbols and their typedata.
// The signature is hashed and the last 8 characters of the digest are returned as a string.
func (fields CallFields) SigHash() string {
	return types.GetHash([]byte(fields.Signature())).Hex()[:8]
}

package engineio

import (
	"reflect"
	"strings"
)

// StateFields represents the state symbols for a Logic
type StateFields = TypeFields

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

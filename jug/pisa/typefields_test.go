package pisa

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypeFields_Insert(t *testing.T) {
	// Initialize TypeFields
	fields := TypeFields{
		Table:   make(map[uint8]*TypeField),
		Symbols: make(map[string]uint8),
	}

	// Insert a new TypeField at position 0
	field := &TypeField{Name: "myField", Type: PrimitiveI64}
	fields.Insert(0, field)

	// Verify that the field was inserted at the correct position
	require.Equal(t, field, fields.Table[0], "TypeField was not inserted at the correct position")
}

func TestTypeFields_Get(t *testing.T) {
	// Initialize TypeFields
	fields := TypeFields{
		Table:   make(map[uint8]*TypeField),
		Symbols: make(map[string]uint8),
	}

	// Insert a new TypeField at position 0
	field := &TypeField{Name: "myField", Type: PrimitiveBool}
	fields.Insert(0, field)

	// Get the TypeField at position 0
	result := fields.Get(0)
	// Verify that the correct TypeField was returned
	require.Equal(t, field, result, "Incorrect TypeField was returned")

	// Get the TypeField at position 10 (non-existent)
	result = fields.Get(10)
	// Verify that nil was returned
	require.Nil(t, result, "Get should have returned nil")
}

func TestTypeFields_Lookup(t *testing.T) {
	// Initialize TypeFields
	fields := TypeFields{
		Table:   make(map[uint8]*TypeField),
		Symbols: make(map[string]uint8),
	}

	// Insert a new TypeField at position 0
	field := &TypeField{Name: "myField", Type: PrimitiveString}
	fields.Insert(0, field)

	// Lookup the TypeField by name
	result := fields.Lookup("myField")
	// Verify that the correct TypeField was returned
	require.Equal(t, field, result, "Incorrect TypeField was returned")

	// Lookup a non-existent TypeField
	result = fields.Lookup("nonExistentField")
	// Verify that nil was returned
	require.Nil(t, result, "Lookup should have returned nil")
}

func TestTypeFields_String(t *testing.T) {
	// Initialize TypeFields
	fields := TypeFields{
		Table:   make(map[uint8]*TypeField),
		Symbols: make(map[string]uint8),
	}

	// Insert two new TypeFields
	fields.Insert(0, &TypeField{Name: "field1", Type: PrimitiveString})
	fields.Insert(1, &TypeField{Name: "field2", Type: PrimitiveAddress})

	// Get the string representation of the TypeFields
	result := fields.String()

	// Verify that the string is in the correct format
	require.Equal(t, "(string, address)", result, "String representation was not in the correct format")
}

func TestTypeFields_Copy(t *testing.T) {
	// Initialize TypeFields
	fields := &TypeFields{
		Table:   make(map[uint8]*TypeField),
		Symbols: make(map[string]uint8),
	}

	// Insert a new TypeField
	field := &TypeField{Name: "myField", Type: PrimitiveI64}
	fields.Insert(0, field)

	// Make a copy of the TypeFields
	clone := fields.Copy()
	// Verify that references are not the same
	require.NotEqual(t, reflect.ValueOf(fields).Pointer(), reflect.ValueOf(clone).Pointer())

	// Modify the original TypeFields
	fields.Insert(1, &TypeField{Name: "newField", Type: PrimitiveString})
	// Verify that the copy is not affected by changes to the original
	require.NotEqual(t, fields, clone)
}

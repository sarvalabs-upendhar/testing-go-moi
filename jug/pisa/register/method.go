package register

import (
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
)

// Method is an interface that describes an executable type method
type Method interface {
	Executable

	Builtin() bool
	Datatype() *engineio.Datatype
}

// MethodCode represents a method ID for a type
// Some method IDs are reserved for special functionality
type MethodCode byte

const (
	MethodNew  MethodCode = 0x0
	MethodInit MethodCode = 0x1

	MethodBool MethodCode = 0x2
	MethodStr  MethodCode = 0x3

	MethodLt MethodCode = 0x4
	MethodGt MethodCode = 0x5
	MethodEq MethodCode = 0x6

	MethodJoin  MethodCode = 0x7
	MethodThrow MethodCode = 0x8
)

// MethodTable represents a collection of Methods
type MethodTable [256]Method

// BuiltinMethod represents a method for a Builtin type (Primitive).
// Implements the Method interface
type BuiltinMethod struct {
	datatype engineio.Primitive
	fields   engineio.CallFields
	execute  func(inputs ValueTable) (outputs ValueTable, exception *exception.Object)
}

func (method BuiltinMethod) Builtin() bool { return true }

func (method BuiltinMethod) Datatype() *engineio.Datatype { return method.datatype.Datatype() }

func (method BuiltinMethod) Interface() engineio.CallFields { return method.fields }

func (method BuiltinMethod) Execute(scope ExecutionScope, inputs ValueTable) (outputs ValueTable) {
	if err := inputs.Validate(method.Interface().Inputs); err != nil {
		scope.Throw(exception.Exception(exception.InvalidInputs, err.Error()))

		return nil
	}

	var except *exception.Object
	if outputs, except = method.execute(inputs); except != nil {
		scope.Throw(except)

		return nil
	}

	if err := outputs.Validate(method.Interface().Outputs); err != nil {
		scope.Throw(exception.Exception(exception.InvalidOutputs, err.Error()))

		return nil
	}

	return outputs
}

func makefields(fields []*engineio.TypeField) *engineio.TypeFields {
	// Ensure that there are less than 256 field expressions
	// This is an internal call so, is alright to panic
	if len(fields) > 256 {
		panic("cannot have more than 256 fields for FieldTable")
	}

	// Create a blank field table
	table := &engineio.TypeFields{
		Table:   make(map[uint8]*engineio.TypeField, len(fields)),
		Symbols: make(map[string]uint8, len(fields)),
	}

	for position, field := range fields {
		table.Table[uint8(position)] = field
		table.Symbols[field.Name] = uint8(position)
	}

	return table
}

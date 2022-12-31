package register

import "github.com/sarvalabs/moichain/jug/pisa/exceptions"

type ExecutionScope interface {
	Throw(object *exceptions.ExceptionObject)
	ExceptionThrown() bool
	GetException() *exceptions.ExceptionObject
}

type Executable interface {
	Interface() CallFields
	Execute(ExecutionScope, ValueTable) ValueTable
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

// Method is an interface that describes an executable type method
type Method interface {
	Executable

	Builtin() bool
	Datatype() *Typedef
}

// BuiltinMethod represents a method for a Builtin type (Primitive).
// Implements the Method interface
type BuiltinMethod struct {
	datatype PrimitiveType
	fields   CallFields
	execute  func(inputs ValueTable) (outputs ValueTable, exception *exceptions.ExceptionObject)
}

func (method BuiltinMethod) Builtin() bool { return true }

func (method BuiltinMethod) Datatype() *Typedef { return method.datatype.Datatype() }

func (method BuiltinMethod) Interface() CallFields { return method.fields }

func (method BuiltinMethod) Execute(scope ExecutionScope, inputs ValueTable) (outputs ValueTable) {
	if err := method.Interface().Inputs.Validate(inputs); err != nil {
		scope.Throw(exceptions.Exception(exceptions.ExceptionInputValidate, err.Error()))

		return nil
	}

	var exception *exceptions.ExceptionObject
	if outputs, exception = method.execute(inputs); exception != nil {
		scope.Throw(exception)

		return nil
	}

	if err := method.Interface().Outputs.Validate(outputs); err != nil {
		scope.Throw(exceptions.Exception(exceptions.ExceptionOutputValidate, err.Error()))

		return nil
	}

	return outputs
}

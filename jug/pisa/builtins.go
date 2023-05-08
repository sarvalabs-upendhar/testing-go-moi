package pisa

import (
	"github.com/sarvalabs/moichain/jug/engineio"
)

// BuiltinRunner is the executor function for a builtin executable
type BuiltinRunner func(*Engine, RegisterSet) (RegisterSet, *Exception)

// Builtin represents an executable of implementation code (golang)
// Implements the Runnable interface
type Builtin struct {
	// CallFields embeds the input and output
	// symbols for the Builtin calling interface.
	CallFields

	// Name represents the name of the builtin
	Name string
	// Ptr represents the pointer reference of the routine
	Ptr engineio.ElementPtr

	// Execute represents the execution function for the builtin.
	// It accepts set of inputs and returns some outputs along with an error code and message.
	runner BuiltinRunner
}

func (builtin Builtin) name() string { return builtin.Name }

func (builtin Builtin) callfields() CallFields { return builtin.CallFields }

func (builtin Builtin) ptr() engineio.ElementPtr { return builtin.Ptr }

func (builtin Builtin) run(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
	if !engine.callstack.push(&callframe{
		scope: "builtin",
		label: builtin.name(),
		point: uint64(builtin.ptr()),
	}) {
		return nil, exception(RuntimeError, "max call depth reached").traced(engine.callstack.trace())
	}

	defer engine.callstack.pop()

	return builtin.runner(engine, inputs)
}

// BuiltinMethod represents a method executable of implementation code (golang)
// Implements the Runnable & Method interfaces
type BuiltinMethod struct {
	// Builtin embeds all Builtin properties
	Builtin
	// Code represents the method code of the method
	Code MethodCode
	// Datatype represents the type that the method belongs to.
	Datatype PrimitiveDatatype
}

func (bmethod BuiltinMethod) code() MethodCode { return bmethod.Code }

func (bmethod BuiltinMethod) datatype() Datatype { return bmethod.Datatype }

func (bmethod BuiltinMethod) run(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
	if !engine.callstack.push(&callframe{
		scope: bmethod.datatype().String(),
		label: bmethod.name(),
		point: uint64(bmethod.code()),
	}) {
		return nil, exception(RuntimeError, "max call depth reached").traced(engine.callstack.trace())
	}

	defer engine.callstack.pop()

	return bmethod.runner(engine, inputs)
}

func makeBuiltinMethod(
	name string, dt PrimitiveDatatype, code MethodCode,
	inputs, outputs *TypeFields, runner BuiltinRunner,
) *BuiltinMethod {
	return &BuiltinMethod{
		Code: code, Datatype: dt, Builtin: Builtin{
			Name: name, runner: runner,
			CallFields: CallFields{Inputs: inputs, Outputs: outputs},
		},
	}
}

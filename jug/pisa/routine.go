package pisa

import (
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/jug/pisa/register"
)

// Routine represents an executable logic procedure
type Routine struct {
	// Name represents the name of the routine
	Name string
	// Kind represents the kind of routine callsite
	Kind engineio.CallsiteKind

	// CallFields contains the input/output symbols
	// injected into the Routine when invoked/called.
	register.CallFields

	// Instructs represents the set of logic instructions to
	// execute when the Routine is invoked/called.
	Instructs Instructions

	// catches represents the exception catch table specifying the
	// exceptions to catch between code points and their handling
	// Catches CatchTable
}

// Interface returns the input/output CallFields of the Routine.
// Implements the register.Executable interface for Routine.
func (routine Routine) Interface() register.CallFields { return routine.CallFields }

// Execute performs the execution of the Routine within the provided ExecutionScope with the given input values.
// Implements the register.Executable interface for Routine.
func (routine Routine) Execute(scope register.ExecutionScope, inputs register.ValueTable) register.ValueTable {
	// Perform input validation
	if err := routine.Inputs.Validate(inputs); err != nil {
		scope.Throw(exception.Exception(exception.InvalidInputs, err.Error()))

		return nil
	}

	// This assertion must never fail
	outerScope, ok := scope.(*ExecutionScope)
	if !ok {
		panic("non routine scope used for routine execution")
	}

	// Create a child context for the routine and its inputs
	innerScope := outerScope.child(routine, inputs)
	// If an exception was thrown during child context creation, return
	if outerScope.ExceptionThrown() {
		return nil
	}

	// If an exception was thrown during routine execution,
	// throw the exception in the parent context and return
	if innerScope.run(); innerScope.ExceptionThrown() {
		outerScope.Throw(innerScope.except)

		return nil
	}

	// Perform output validation
	if err := routine.Outputs.Validate(innerScope.outputs); err != nil {
		outerScope.Throw(exception.Exception(exception.InvalidOutputs, err.Error()))

		return nil
	}

	return innerScope.outputs
}

func (routine Routine) Exported() bool {
	return IsExportedName(routine.Name)
}

func (routine Routine) Mutable() bool {
	return IsMutableName(routine.Name)
}

func (routine Routine) Payable() bool {
	return IsPayableName(routine.Name)
}

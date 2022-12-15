package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	ctypes "github.com/sarvalabs/moichain/jug/types"
)

// ExecutionContext represents an isolated runtime environment for executing some logic Instructions.
// It acts as an execution space with its own Register set and load/yield value slots.
type ExecutionContext struct {
	depth  int
	engine *Engine

	routine *Routine
	program program

	inputs    ValueTable
	outputs   ValueTable
	registers RegisterSet
}

// NewExecutionContext generates a new ExecutionContext object within the context of the given PISA engine
// and for the given Routine. The generated environment is initialized for the Routine and its instructions
// with a call depth of 0. The given input values are injected as input data for the ExecutionContext.
func NewExecutionContext(engine *Engine, routine *Routine, calldata ValueTable) *ExecutionContext {
	return &ExecutionContext{
		depth:   0,
		engine:  engine,
		routine: routine,
		program: program{code: routine.Instructs},

		inputs:    calldata,
		outputs:   make(ValueTable),
		registers: make(RegisterSet),
	}
}

// Outputs returns the output values of the ExecutionContext as a polo.Document.
// The output values are taken from the returned Values in the ExecutionContext and are
// converted into a polo.Document with fields names specified by the Routine's output callfields.
func (ctx *ExecutionContext) Outputs() ctypes.ExecutionValues {
	doc := make(polo.Document)

	for label, index := range ctx.routine.Outputs.Symbols {
		if value, ok := ctx.outputs[index]; ok {
			doc.Set(label, value.Data())
		}
	}

	return doc
}

// GetPtrValue resolves a register ID into uint64 pointer address.
// The Register at the reg address must exist and be of type TypePtr.
func (ctx *ExecutionContext) GetPtrValue(reg byte) (uint64, error) {
	// Retrieve the Register object
	register, exists := ctx.registers.get(reg)
	if !exists {
		return 0, errors.New("missing register for pointer")
	}

	// Check that the register type is Ptr
	if register.Value.Type() != TypePtr {
		return 0, errors.New("register is not a pointer")
	}

	// Cast into a PtrValue
	ptr, _ := register.Value.(PtrValue)
	// Return PtrValue as a uint64
	return uint64(ptr), nil
}

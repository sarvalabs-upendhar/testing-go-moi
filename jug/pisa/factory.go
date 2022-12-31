package pisa

import (
	"github.com/sarvalabs/moichain/jug/pisa/runtime"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// Factory represents the engine generation factory for PISA.
// It holds the base instruction set and type table that is copied
// into every engine instance instead of recomputing them each time.
// Implements the engine.Factory interface.
type Factory struct {
	datatypes runtime.TypeTable
	instructs runtime.InstructionSet
}

// NewFactory generates a new Factory instance that can be
// used to generate new instances of the PISA Execution Engine
func NewFactory() Factory {
	return Factory{
		datatypes: runtime.BabylonTypeTable(),
		instructs: runtime.BabylonInstructionSet(),
	}
}

// Kind returns the kind of engine factory and implements
// the engine.Factory interface for the PISA runtime
func (factory Factory) Kind() types.LogicEngine { return types.PISA }

// NewExecutionEngine generates a new PISA ExecutionEngine instance for the given fuel capacity.
// The returned engine instance has the base instruction set and type table configured.
func (factory Factory) NewExecutionEngine(fuelCap uint64) ctypes.ExecutionEngine {
	return &Engine{
		instructs: factory.instructs,
		datatypes: factory.datatypes,
		constants: make(runtime.ConstantTable),
		routines:  runtime.NewRoutineTable(),
		fueltank:  ctypes.NewFuelTank(fuelCap),
	}
}

package pisa

import (
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// Factory represents the engine generation factory for PISA.
// It holds the base instruction set and type table that is copied
// into every engine instance instead of recomputing them each time.
// Implements the engine.Factory interface.
type Factory struct{}

// NewFactory generates a new Factory instance that can be
// used to generate new instances of the PISA Execution Engine
func NewFactory() Factory {
	return Factory{}
}

// Kind returns the kind of engine factory and implements
// the engine.Factory interface for the PISA runtime
func (f Factory) Kind() types.LogicEngine {
	return types.PISA
}

// NewExecutionEngine generates a new PISA ExecutionEngine instance for the given fuel capacity.
// The returned engine instance has the base instruction set and type table configured.
func (f Factory) NewExecutionEngine(fuelCap uint64) ctypes.ExecutionEngine {
	return &Engine{
		fueltank:  ctypes.NewFuelTank(fuelCap),
		constants: make(ConstantTable),
	}
}

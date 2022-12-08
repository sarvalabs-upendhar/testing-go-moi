package pisa

import "github.com/sarvalabs/moichain/jug/engine"

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

// Kind returns the kind of engine factory and
// implements the engine.Factory interface for PISA
func (f Factory) Kind() engine.Kind {
	return engine.PISA
}

// NewExecutionEngine generates a new PISA ExecutionEngine instance for the given fuel capacity.
// The returned engine instance has the base instruction set and type table configured.
func (f Factory) NewExecutionEngine(fuelCap uint64) engine.ExecutionEngine {
	return &PISA{
		fueltank:  engine.NewFuelTank(fuelCap),
		constants: make(ConstantTable),
	}
}

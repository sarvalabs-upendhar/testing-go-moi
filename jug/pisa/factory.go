package pisa

import (
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/jug/pisa/runtime"
)

// Factory represents the engine generation factory for PISA. It holds the base instruction set
// and type table that is copied into every engine instance instead of recomputing them each time.
// Implements the engineio.EngineFactory interface.
type Factory struct {
	instructs runtime.InstructionSet
	builtins  map[register.PrimitiveType]register.MethodTable
}

// NewFactory generates a new Factory instance that can be
// used to generate new instances of the PISA Execution Engine
func NewFactory() Factory {
	return Factory{
		instructs: runtime.BaseInstructionSet(),
		builtins: map[register.PrimitiveType]register.MethodTable{
			register.PrimitiveBool:    register.BoolMethods(),
			register.PrimitiveBytes:   register.BytesMethods(),
			register.PrimitiveString:  register.StringMethods(),
			register.PrimitiveU64:     register.U64Methods(),
			register.PrimitiveI64:     register.I64Methods(),
			register.PrimitiveAddress: register.AddressMethods(),
		},
	}
}

// Kind returns the kind of engine factory and implements
// the  interface for the PISA runtime
func (factory Factory) Kind() engineio.EngineKind { return engineio.PISA }

// NewEngine generates a new PISA Engine instance. The returned engine
// instance has the base instruction set and builtins configured.
func (factory Factory) NewEngine() engineio.Engine {
	return &Engine{
		classes:  make(map[string]uint64),
		elements: make(map[uint64]any),

		instructs: factory.instructs,
		builtins:  factory.builtins,
	}
}

package pisa

import (
	"context"

	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// Engine represents the execution engine for the PISA Virtual Machine.
// It implements the types.ExecutionEngine interface.
type Engine struct {
	// fueltank represents the fuel tank of the engine
	fueltank *ctypes.FuelTank

	// events engine.EventStream
	// extrinsics engine.ExtrinsicSet
	// instructs InstructionSet
	// datatypes TypeTable

	// constants represents a table of logic Constant objects indexed by their 64-bit pointer
	constants ConstantTable
}

// Kind returns the kind of engine and implements
// the engine.ExecutionEngine interface for PISA
func (engine Engine) Kind() types.LogicEngine { return types.PISA }

func (engine Engine) Execute(_ context.Context, _ ctypes.Logic, _ *ctypes.ExecutionScope) *ctypes.ExecutionResult {
	panic("missing implementation: pisa.PISA.Execute()")
}

func (engine Engine) Compile(_ context.Context, _ []byte) (*types.LogicDescriptor, *ctypes.ExecutionResult) {
	panic("missing implementation: pisa.PISA.Execute()")
}

func (engine Engine) Implements(_ context.Context, _ ctypes.Logic, _ types.LogicKind) (bool, *ctypes.ExecutionResult) {
	panic("missing implementation: pisa.PISA.Implements()")
}

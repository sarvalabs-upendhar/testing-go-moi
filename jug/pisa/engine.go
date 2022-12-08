package pisa

import "github.com/sarvalabs/moichain/jug/engine"

// PISA represents the execution engine for the PISA Virtual Machine.
// It implements the engine.ExecutionEngine interface.
type PISA struct {
	// logic *engine.LogicObject
	// storage engine.StorageObject

	// fueltank represents the fuel tank of the engine
	fueltank *engine.FuelTank

	// events engine.EventStream
	// extrinsics engine.ExtrinsicSet
	// instructs InstructionSet
	// datatypes TypeTable

	// constants represents a table of logic Constant objects indexed by their 64-bit pointer
	constants ConstantTable
}

// Kind returns the kind of engine and implements
// the engine.ExecutionEngine interface for PISA
func (pisa PISA) Kind() engine.Kind { return engine.PISA }

// Fuel returns the FuelTank instance in the engine.
// It implements the engine.ExecutionEngine interface for PISA
func (pisa PISA) Fuel() *engine.FuelTank { return pisa.fueltank }

func (pisa PISA) Init(_ *engine.LogicObject, _ engine.StorageObject, _ engine.Values) error {
	panic("missing implementation: pisa.PISA.Init()")
}

func (pisa PISA) Run(_ string, _ *engine.LogicObject, _ engine.StorageObject, _ engine.Values) (engine.Values, error) {
	panic("missing implementation: pisa.PISA.Run()")
}

func (pisa PISA) CompileLogic(_ []byte) (*engine.LogicObject, error) {
	panic("missing implementation: pisa.PISA.CompileLogic()")
}

func (pisa PISA) CompileManifest(_ *engine.LogicObject) ([]byte, error) {
	panic("missing implementation: pisa.PISA.CompileManifest()")
}

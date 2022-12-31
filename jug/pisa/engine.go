package pisa

import (
	"context"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/exceptions"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/jug/pisa/runtime"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// Engine represents the execution engine for the PISA Virtual Machine.
// It implements the types.ExecutionEngine interface.
type Engine struct {
	// instructs represents the engine instruction set for PISA.
	instructs runtime.InstructionSet

	// fueltank represents the fuel tank of the engine
	fueltank *ctypes.FuelTank
	// logic represents the logic driver
	logic ctypes.Logic
	// storage represents a table of storage drivers
	storage runtime.StorageTable

	// datatypes represents the type management table for PISA.
	datatypes runtime.TypeTable
	// constants represents a table of logic Constant objects indexed by their 64-bit pointer
	constants runtime.ConstantTable
	// routines represents a table of logic Routine objects indexed by their 64-bit pointers
	routines runtime.RoutineTable
}

// Kind returns the kind of engine and implements
// the engine.ExecutionEngine interface for PISA
func (engine Engine) Kind() types.LogicEngine { return types.PISA }

func (engine *Engine) Execute(
	_ context.Context,
	logic ctypes.Logic,
	order *ctypes.ExecutionOrder,
) *ctypes.ExecutionResult {
	// Set the logic driver for the engine
	engine.logic = logic
	// Set the storage drivers for the engine if logic is stateful
	if logic.IsStateful() {
		// Load the storage layout into the engine
		if _, ok := engine.GetStorageLayout(); !ok {
			return engine.Error(exceptions.ExceptionExecutionSetup, "could not load storage layout")
		}

		// Set the storage drivers for the caller and callee
		engine.storage.Callee = order.Callee
		engine.storage.Caller = order.Caller
	}

	// Set up the execution context depending on the execution order flag
	if order.Initialise {
		return engine.InitialiseStorage(order.Inputs)
	}

	// Fetch the callsite pointer for the given input string callsite
	callsite, exists := logic.GetCallsite(order.Callsite)
	if !exists {
		return engine.Errorf(exceptions.ExceptionInvalidCallsite, "callsite '%v' does not exist", order.Callsite)
	}

	// Get the routine
	routine, ok := engine.GetRoutine(uint64(callsite))
	if !ok {
		return engine.Error(exceptions.ExceptionExecutionSetup, "could not load routine")
	}

	// Exhaust some fuel for the execution setup
	if !engine.fueltank.Exhaust(50) {
		return engine.Error(exceptions.ExceptionFuelExhausted, "")
	}

	// Convert the input ExecutionValues into a RegisterTable
	inputs, err := register.NewValueTable(routine.Inputs, order.Inputs)
	if err != nil {
		return engine.Errorf(exceptions.ExceptionExecutionSetup, "invalid calldata: %v", err)
	}

	// Create a root execution context
	rootCtx := runtime.RootExecutionScope(engine)
	// Execute the routine with the root context and inputs
	outputs := routine.Execute(rootCtx, inputs)

	// If root context has a raised exception return it
	if rootCtx.ExceptionThrown() {
		return engine.ErrorException(rootCtx.GetException())
	}

	// Convert the output RegisterTable into ExecutionValues
	outputValues := make(polo.Document)
	// For each output value, set encoded data for the label
	for label, index := range routine.Outputs.Symbols {
		if value, ok := outputs[index]; ok {
			outputValues.Set(label, value.Data())
		}
	}

	return engine.Result(outputValues)
}

func (engine *Engine) Compile(_ context.Context, manifest []byte) (*types.LogicDescriptor, *ctypes.ExecutionResult) {
	// Decode into engine.ManifestHeader
	header := new(ctypes.ManifestHeader)
	if err := polo.Depolorize(header, manifest); err != nil {
		return nil, engine.Error(exceptions.ExceptionInvalidManifest, "malformed header")
	}

	// Check that the Engine is PISA
	if header.LogicEngine() != types.PISA {
		return nil, engine.Error(exceptions.ExceptionInvalidManifest, "unsupported engine")
	}

	// Check that syntax is "1". Error for all other syntax forms
	if header.Syntax != "1" {
		return nil, engine.Error(exceptions.ExceptionInvalidManifest, "unsupported syntax")
	}

	return engine.compileLogicV1(manifest)
}

func (engine *Engine) Implements(_ context.Context, _ ctypes.Logic, _ types.LogicKind) (bool, *ctypes.ExecutionResult) {
	panic("missing implementation: pisa.PISA.Implements()")
}

func (engine *Engine) InitialiseStorage(inputs ctypes.ExecutionValues) *ctypes.ExecutionResult {
	// Initialize the storage slots
	for slot, datatype := range engine.storage.Layout.Table {
		// For each typefield in the storage layout, generate the default value
		defaultValue, err := register.NewValue(datatype.Type, nil)
		if err != nil {
			return engine.Errorf(exceptions.ExceptionValueInit, "storage &%v: %v", slot, err)
		}

		// Create a storage entry for the default value
		if err = engine.WriteStorage(slot, defaultValue.Data()); err != nil {
			return engine.Errorf(exceptions.ExceptionStorageWrite, "storage &%v: %v", slot, err)
		}
	}

	// Exhaust some fuel for the storage build
	if !engine.fueltank.Exhaust(50) {
		return engine.Error(exceptions.ExceptionFuelExhausted, "")
	}

	// Get the builder
	builder, ok := engine.GetStorageBuilder()
	if !ok {
		return engine.Error(exceptions.ExceptionExecutionSetup, "could not load builder")
	}

	// Exhaust some fuel for the execution setup
	if !engine.fueltank.Exhaust(50) {
		return engine.Error(exceptions.ExceptionFuelExhausted, "")
	}

	// Convert the input ExecutionValues into a RegisterTable
	inputValues, err := register.NewValueTable(builder.Inputs, inputs)
	if err != nil {
		return engine.Errorf(exceptions.ExceptionExecutionSetup, "invalid calldata: %v", err)
	}

	// Create a root execution context
	rootCtx := runtime.RootExecutionScope(engine)
	// Execute the builder with the root context and inputs
	_ = builder.Execute(rootCtx, inputValues)

	// If root context has a raised exception return it
	if rootCtx.ExceptionThrown() {
		return engine.ErrorException(rootCtx.GetException())
	}

	return engine.Result(nil)
}

// Result returns an ExecutionResult for some given ExecutionValues.
// The result contains the consumed fuel, generated logs and the ErrorCodeOk.
func (engine Engine) Result(values ctypes.ExecutionValues) *ctypes.ExecutionResult {
	return &ctypes.ExecutionResult{
		Fuel:    engine.fueltank.Consumed,
		ErrCode: uint64(exceptions.ExceptionOk),
		Outputs: values,
	}
}

// Error returns an ExecutionResult for some ErrorCode and Error Message.
// The result contains the consumed fuel and the given ErrorCode and Message.
func (engine Engine) Error(kind exceptions.ExceptionCode, data string) *ctypes.ExecutionResult {
	return engine.ErrorException(exceptions.Exception(kind, data))
}

// Errorf returns an ExecutionResult for some ErrorCode and Error Message (with formatting).
// The result contains the consumed fuel and the given ErrorCode and Message.
func (engine Engine) Errorf(kind exceptions.ExceptionCode, format string, a ...any) *ctypes.ExecutionResult {
	return engine.ErrorException(exceptions.Exceptionf(kind, format, a...))
}

func (engine Engine) ErrorException(exception *exceptions.ExceptionObject) *ctypes.ExecutionResult {
	return &ctypes.ExecutionResult{
		Fuel:       engine.fueltank.Consumed,
		ErrCode:    uint64(exception.Code),
		ErrMessage: exception.String(),
	}
}

func (engine *Engine) GetRoutine(ptr uint64) (*runtime.Routine, bool) {
	// If Routine already exists in table, return
	if routine, ok := engine.routines.Get(ptr); ok {
		return routine, true
	}

	// Fetch the logic element for the Routine from the logic driver
	element, ok := engine.logic.GetLogicElement(runtime.ElementCodeRoutine, ptr)
	if !ok {
		return nil, false
	}

	// Attempt to depolorize the element data into a Routine
	routine := new(runtime.Routine)
	if err := polo.Depolorize(routine, element.Data); err != nil {
		return nil, false
	}

	// Set the Routine into the engine's routine table
	engine.routines.Symbols[routine.Name] = ptr
	engine.routines.Table[ptr] = routine

	return routine, true
}

func (engine *Engine) GetConstant(ptr uint64) (*runtime.Constant, bool) {
	// If the constant already exists, return
	if constant, ok := engine.constants.Get(ptr); ok {
		return constant, true
	}

	// Fetch the logic element for the Constant from the logic driver
	element, ok := engine.logic.GetLogicElement(runtime.ElementCodeConstant, ptr)
	if !ok {
		return nil, false
	}

	// Attempt to depolorize the element data into a Constant
	constant := new(runtime.Constant)
	if err := polo.Depolorize(constant, element.Data); err != nil {
		return nil, false
	}

	// Set the Constant into the engine's constant table
	engine.constants[ptr] = constant

	return constant, true
}

func (engine *Engine) GetSymbolicType(ptr uint64) (*register.Typedef, bool) {
	// If the typedef already exists, return
	if typedef, ok := engine.datatypes.GetSymbolic(ptr); ok {
		return typedef, true
	}

	// Fetch the logic element for the typedef from the logic driver
	element, ok := engine.logic.GetLogicElement(runtime.ElementCodeTypedef, ptr)
	if !ok {
		return nil, false
	}

	// Attempt to depolorize the element data into a Typedef
	datatype := new(register.Typedef)
	if err := polo.Depolorize(&datatype, element.Data); err != nil {
		return nil, false
	}

	// Set the Typedef as symbolic type into engine's type table
	engine.datatypes.SetSymbolic(ptr, datatype)

	return datatype, true
}

func (engine *Engine) GetStorageLayout() (*runtime.StorageLayout, bool) {
	// If the storage layout exists, return
	if engine.storage.Layout != nil {
		return engine.storage.Layout, true
	}

	// Fetch the logic element for the StorageLayout from the logic driver
	element, ok := engine.logic.GetLogicElement(runtime.ElementCodeStorage, 0)
	if !ok {
		return nil, false
	}

	// Attempt to depolorize the element data into a StorageLayout
	layout := new(runtime.StorageLayout)
	if err := polo.Depolorize(layout, element.Data); err != nil {
		return nil, false
	}

	// Set the StorageLayout into the engine's storage table
	engine.storage.Layout = layout

	return layout, true
}

func (engine *Engine) GetStorageBuilder() (*runtime.StorageBuilder, bool) {
	// If the storage builder exists, return
	if engine.storage.Builder != nil {
		return engine.storage.Builder, true
	}

	// Fetch the logic element for the StorageBuilder from the logic driver
	element, ok := engine.logic.GetLogicElement(runtime.ElementCodeStorage, 1)
	if !ok {
		return nil, false
	}

	// Attempt to depolorize the element data into a StorageBuilder
	builder := new(runtime.StorageBuilder)
	if err := polo.Depolorize(builder, element.Data); err != nil {
		return nil, false
	}

	// Set the StorageBuilder into the engine's storage table
	engine.storage.Builder = builder

	return builder, true
}

func (engine *Engine) ConsumedFuel() uint64 {
	return engine.fueltank.Consumed
}

func (engine *Engine) ExhaustFuel(fuel uint64) bool {
	return engine.fueltank.Exhaust(fuel)
}

func (engine *Engine) GetInstruction(opcode runtime.OpCode) *runtime.InstructOperation {
	return engine.instructs[opcode]
}

func (engine *Engine) GetTypeMethod(dt *register.Typedef, methodCode register.MethodCode) (register.Method, bool) {
	return engine.datatypes.GetMethod(dt, methodCode)
}

func (engine *Engine) WriteStorage(slot uint8, data []byte) error {
	return engine.storage.Callee.SetStorageEntry(engine.logic.LogicID(), runtime.SlotHash(slot), data)
}

func (engine *Engine) ReadStorage(slot uint8) ([]byte, error) {
	return engine.storage.Callee.GetStorageEntry(engine.logic.LogicID(), runtime.SlotHash(slot))
}

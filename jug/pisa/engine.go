package pisa

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// Engine represents the execution engine for the PISA Virtual Machine.
// It implements the types.ExecutionEngine interface.
type Engine struct {
	// fueltank represents the fuel tank of the engine
	fueltank *ctypes.FuelTank

	// instructs represents the engine instruction set for PISA.
	instructs InstructionSet
	// datatypes represents the type management table for PISA.
	datatypes TypeTable

	// logic represents the logic driver
	logic ctypes.Logic
	// storage represents a table of storage drivers
	storage StorageTable

	// constants represents a table of logic Constant objects indexed by their 64-bit pointer
	constants ConstantTable
	// routines represents a table of logic Routine objects indexed by their 64-bit pointers
	routines RoutineTable
}

// Kind returns the kind of engine and implements
// the engine.ExecutionEngine interface for PISA
func (engine Engine) Kind() types.LogicEngine { return types.PISA }

func (engine Engine) Execute(
	_ context.Context,
	logic ctypes.Logic,
	order *ctypes.ExecutionOrder,
) *ctypes.ExecutionResult {
	// Set the logic driver for the engine
	engine.logic = logic
	// Set the storage drivers for the engine if logic is stateful
	if logic.IsStateful() {
		// Load the storage layout into the engine
		if err := engine.loadStorageLayout(); err != nil {
			return engine.Error(ErrorCodeExecutionSetupFail,
				fmt.Sprintf("could not load storage layout: %v", err))
		}

		// Set the storage drivers for the caller and callee
		engine.storage.callee = order.Callee
		engine.storage.caller = order.Caller
	}

	// Set up the execution context depending on the execution order flag
	if order.Initialise {
		return engine.BuildStorage(order.Inputs)
	}

	// Fetch the callsite pointer for the given input string callsite
	callsite, exists := logic.GetCallsite(order.Callsite)
	if !exists {
		return engine.Error(ErrorCodeInvalidCallsite, "callsite does not exists")
	}

	// Load the routine into the engine
	if err := engine.loadRoutine(uint64(callsite)); err != nil {
		return engine.Error(ErrorCodeExecutionSetupFail,
			fmt.Sprintf("could not load routine (%v|%v): %v", order.Callsite, callsite, err))
	}

	// Spawn an ExecutionContext for the Routine callsite
	executionCtx, err := engine.spawnRoutineCtx(callsite, order.Inputs)
	if err != nil {
		return engine.Error(ErrorCodeExecutionSetupFail, fmt.Sprintf("could not spawn execution context: %v", err))
	}

	// Exhaust some fuel for the execution setup
	if !engine.fueltank.Exhaust(50) {
		return engine.Error(ErrorCodeRanOutOfFuel, "exhausted before routine execution")
	}

	// Run the execution and return the result
	return engine.RunExecution(executionCtx)
}

func (engine Engine) Compile(_ context.Context, manifest []byte) (*types.LogicDescriptor, *ctypes.ExecutionResult) {
	// Decode into engine.ManifestHeader
	header := new(ctypes.ManifestHeader)
	if err := polo.Depolorize(header, manifest); err != nil {
		return nil, engine.Error(ErrorCodeInvalidManifest, "malformed header")
	}

	// Check that the Engine is PISA
	if header.LogicEngine() != types.PISA {
		return nil, engine.Error(ErrorCodeInvalidManifest, "unsupported engine")
	}

	// Check that syntax is "1". Error for all other syntax forms
	if header.Syntax != "1" {
		return nil, engine.Error(ErrorCodeInvalidManifest, "unsupported syntax")
	}

	return engine.compileLogicV1(manifest)
}

func (engine Engine) Implements(_ context.Context, _ ctypes.Logic, _ types.LogicKind) (bool, *ctypes.ExecutionResult) {
	panic("missing implementation: pisa.PISA.Implements()")
}

func (engine Engine) RunExecution(ctx *ExecutionContext) *ctypes.ExecutionResult {
	// Execution Loop -> ends when instructions are completed or engine fuel is depleted
	for !ctx.program.done() {
		// Get the next instruction from the program
		instruct := ctx.program.read()
		// Get the operation for the opcode from the instruction set
		op := engine.instructs.lookup(instruct.Op)

		// Attempt to exhaust some fuel from the engine -> fails if there is not enough fuel left
		if ok := engine.fueltank.Exhaust(op.expense(ctx)); !ok {
			// Fuel Depleted - unread the instruction and break execution
			ctx.program.unread()

			return engine.Error(ErrorCodeRanOutOfFuel, "Exception: Fuel Depleted")
		}

		// Execute the instruction
		if err := op.execute(ctx, instruct.Args); err != nil {
			// If the error is a terminate signal, break from execution look
			if errors.Is(err, ErrTerminate) {
				break
			}

			ctx.program.unread()

			return engine.Error(
				ErrorCodeExecutionRuntimeFail,
				fmt.Sprintf("Execution Halted | Instruction: [%#x] | Cause: %v", ctx.program.pc, err),
			)
		}
	}

	// Generate the ExecutionResult with output values from the ExecutionContext
	return engine.Result(ctx.Outputs())
}

func (engine Engine) BuildStorage(inputs ctypes.ExecutionValues) *ctypes.ExecutionResult {
	// Initialize the storage slots
	for slot, datatype := range engine.storage.layout.Table {
		// For each typefield in the storage layout, generate the default value
		defaultValue, err := NewValue(datatype.Type, nil)
		if err != nil {
			return engine.Error(ErrorCodeStorageBuildFail, fmt.Sprintf("storage build slot [%v]: %v", slot, err))
		}

		// Create a storage entry for the default value
		if err = engine.storage.callee.SetStorageEntry(
			engine.logic.LogicID(), SlotHash(slot), defaultValue.Data(),
		); err != nil {
			return engine.Error(ErrorCodeStorageBuildFail, fmt.Sprintf("storage build slot [%v]: %v", slot, err))
		}
	}

	// Exhaust some fuel for the storage build
	if !engine.fueltank.Exhaust(50) {
		return engine.Error(ErrorCodeRanOutOfFuel, "exhausted before builder execution setup")
	}

	// Load the routine into the engine
	if err := engine.loadStorageBuilder(); err != nil {
		return engine.Error(ErrorCodeExecutionSetupFail, fmt.Sprintf("could not load builder: %v", err))
	}

	// Spawn an ExecutionContext
	executionCtx, err := engine.spawnBuilderCtx(inputs)
	if err != nil {
		return engine.Error(ErrorCodeExecutionSetupFail, fmt.Sprintf("could not spawn execution context: %v", err))
	}

	// Exhaust some fuel for the execution setup
	if !engine.fueltank.Exhaust(50) {
		return engine.Error(ErrorCodeRanOutOfFuel, "exhausted before builder execution")
	}

	// Run the execution and return the result
	return engine.RunExecution(executionCtx)
}

// Result returns an ExecutionResult for some given ExecutionValues.
// The result contains the consumed fuel, generated logs and the ErrorCodeOk.
func (engine Engine) Result(values ctypes.ExecutionValues) *ctypes.ExecutionResult {
	return &ctypes.ExecutionResult{
		Fuel:    engine.fueltank.Consumed,
		Outputs: values, Logs: nil,
		ErrCode: uint64(ErrorCodeOk), ErrMessage: "",
	}
}

// Error returns an ExecutionResult for some ErrorCode and Error Message.
// The result contains the consumed fuel and the given ErrorCode and Message.
func (engine Engine) Error(code ErrorCode, message string) *ctypes.ExecutionResult {
	return &ctypes.ExecutionResult{
		Fuel:    engine.fueltank.Consumed,
		Outputs: nil, Logs: nil,
		ErrCode:    uint64(code),
		ErrMessage: message,
	}
}

func (engine *Engine) spawnRoutineCtx(
	ptr types.LogicCallsite,
	inputs ctypes.ExecutionValues,
) (*ExecutionContext, error) {
	// Fetch the routine
	routine, exists := engine.routines.fetch(uint64(ptr))
	if !exists {
		return nil, errors.Errorf("routine '%v' unavailable", ptr)
	}

	// Create a table of values from the calldata as defined by the routine input fields
	// This implicitly performs calldata validation by ensuring that all input fields have
	// some data of the correct type in the calldata document
	inputTable, err := NewValueTable(routine.Inputs, inputs)
	if err != nil {
		return nil, errors.Wrap(err, "invalid calldata")
	}

	// Generate an execution runtime for the routine
	return NewExecutionContext(engine, routine, inputTable), nil
}

func (engine *Engine) spawnBuilderCtx(inputs ctypes.ExecutionValues) (*ExecutionContext, error) {
	// Fetch the builder
	builder := engine.storage.builder
	if builder == nil {
		return nil, errors.New("builder unavailable")
	}

	// Create a table of values from the calldata as defined by the routine input fields
	// This implicitly performs calldata validation by ensuring that all input fields have
	// some data of the correct type in the calldata document
	inputTable, err := NewValueTable(builder.Inputs, inputs)
	if err != nil {
		return nil, errors.Wrap(err, "invalid calldata")
	}

	// Generate an execution runtime for the routine
	return NewExecutionContext(engine, builder, inputTable), nil
}

func (engine *Engine) loadRoutine(ptr uint64) error {
	// If Routine already exists in table, return
	if _, ok := engine.routines.fetch(ptr); ok {
		return nil
	}

	// Fetch the logic element for the Routine from the logic driver
	element, err := engine.logic.GetLogicElement(elementRoutine, ptr)
	if err != nil {
		return errors.New("routine load: no routine for address")
	}

	// Depolorize the element data into a Routine
	routine := new(Routine)
	if err = polo.Depolorize(routine, element.Data); err != nil {
		return errors.New("routine load: could not decode")
	}

	// Set the Routine into the engine's routine table
	engine.routines.insert(routine.Name, ptr, routine)

	return nil
}

func (engine *Engine) loadConstant(ptr uint64) error {
	// If the constant already exists, return
	if _, ok := engine.constants.fetch(ptr); ok {
		return nil
	}

	// Fetch the logic element for the Constant from the logic driver
	element, err := engine.logic.GetLogicElement(elementConstant, ptr)
	if err != nil {
		return errors.New("constant load: no constant for address")
	}

	// Depolorize the element data into a Constant
	constant := new(Constant)
	if err = polo.Depolorize(constant, element.Data); err != nil {
		return errors.New("constant load: could not decode")
	}

	// Set the Constant into the engine's constant table
	engine.constants.insert(ptr, constant)

	return nil
}

func (engine *Engine) loadTypedefs(ptr uint64) error {
	// If the typedef already exists, return
	if _, ok := engine.datatypes.symbolic[ptr]; ok {
		return nil
	}

	// Fetch the logic element for the typedef from the logic driver
	element, err := engine.logic.GetLogicElement(elementTypedef, ptr)
	if err != nil {
		return errors.New("typedef load: no typedef element")
	}

	// Depolorize the element data into a Datatype
	datatype := new(Datatype)
	if err = polo.Depolorize(&datatype, element.Data); err != nil {
		return errors.New("typedef load: could not decode")
	}

	// Set the Datatype as symbolic type into engine's type table
	engine.datatypes.insertSymbolic(ptr, datatype)

	return nil
}

func (engine *Engine) loadStorageLayout() error {
	// If the storage layout exists, return
	if engine.storage.layout != nil {
		return nil
	}

	// Fetch the logic element for the StorageLayout from the logic driver
	element, err := engine.logic.GetLogicElement(elementStorage, 0)
	if err != nil {
		return errors.New("storage layout load: no such element")
	}

	// Depolorize the element data into a StorageLayout
	storage := new(StorageLayout)
	if err = polo.Depolorize(storage, element.Data); err != nil {
		return errors.New("storage layout load: could not decode")
	}

	// Set the StorageLayout into the engine's storage table
	engine.storage.layout = storage

	return nil
}

func (engine *Engine) loadStorageBuilder() error {
	// If the storage builder exists, return
	if engine.storage.builder != nil {
		return nil
	}

	// Fetch the logic element for the StorageBuilder from the logic driver
	element, err := engine.logic.GetLogicElement(elementStorage, 1)
	if err != nil {
		return errors.New("storage builder load: no such element")
	}

	// Depolorize the element data into a StorageBuilder
	builder := new(StorageBuilder)
	if err = polo.Depolorize(builder, element.Data); err != nil {
		return errors.New("storage builder load: could not decode")
	}

	// Set the StorageBuilder into the engine's storage table
	engine.storage.builder = builder

	return nil
}

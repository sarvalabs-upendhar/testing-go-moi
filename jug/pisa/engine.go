package pisa

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/jug/pisa/runtime"
	"github.com/sarvalabs/moichain/types"
)

// Engine represents the execution engine for the PISA Virtual Machine.
// It implements the engineio.EngineDriver interface.
type Engine struct {
	// fueltank represents the fuel tank of the engine
	fueltank *engineio.FuelTank

	// logic represents the logic driver
	logic engineio.LogicDriver
	// ctx drivers
	internal engineio.CtxDriver
	// env driver
	environment engineio.EnvDriver

	// instructs represents the engine instruction set for PISA.
	instructs runtime.InstructionSet
	// builtins represents the methods for builtin types
	builtins map[register.PrimitiveType]register.MethodTable

	// elements represents the decoded engineio.LogicElement cache
	elements map[uint64]any
	// classes represents a name-map for classes to their element pointers
	classes map[string]uint64
}

// Kind returns the engineio.EngineKind of the PISA
// It implements the engineio.EngineDriver interface.
func (engine Engine) Kind() engineio.EngineKind { return engineio.PISA }

// Bootstrapped returns whether the engine has been bootstrapped
// or initialized based on the null-ness of the logic driver.
// It implements the engineio.EngineDriver interface.
func (engine Engine) Bootstrapped() bool { return engine.logic != nil }

// Bootstrap initializes the engine with some fuel, an engineio.LogicDriver, its corresponding
// engineio.CtxDriver and an engineio.EnvDriver. The bootstrap will fail if the logic and
// context drivers don't match or if the logic driver is not supported by PISA.
// It implements the engineio.EngineDriver interface.
func (engine *Engine) Bootstrap(
	ctx context.Context, fuel engineio.Fuel,
	logic engineio.LogicDriver,
	internal engineio.CtxDriver,
	env engineio.EnvDriver,
) error {
	// Check logic driver engine
	if logic.Engine() != engineio.PISA {
		return errors.New("incompatible logic driver: not a PISA logic")
	}

	// Check logic driver's logic ID and its context's logic ID.
	if !bytes.Equal(logic.LogicID(), internal.LogicID()) {
		return errors.New("incompatible context driver for logic: logic ID is not equal")
	}

	// Check the logic driver and context driver's addresses
	if logic.LogicID().Address().Hex() != internal.Address().Hex() {
		return errors.New("incompatible context driver for logic: address does not match")
	}

	engine.logic = logic
	engine.internal = internal
	engine.environment = env
	engine.fueltank = engineio.NewFuelTank(fuel)

	return nil
}

// Compile generates an engineio.LogicDescriptor from an engineio.Manifest object.
// The compilation will fail if the manifest object is not compatible or has malformed elements.
// It implements the engineio.EngineDriver interface.
func (engine Engine) Compile(
	ctx context.Context, fuel engineio.Fuel, manifest *engineio.Manifest,
) (
	*engineio.LogicDescriptor, engineio.Fuel, error,
) {
	// Check that the Manifest Engine is PISA
	if manifest.Header().LogicEngine() != engineio.PISA {
		return nil, 0, errors.New("invalid manifest: manifest engine is not PISA")
	}

	// Create a ManifestCompiler instance
	compiler, err := newManifestCompiler(fuel, manifest, engine.instructs)
	if err != nil {
		return nil, 0, errors.Wrap(err, "")
	}

	// Compile the manifest
	descriptor, err := compiler.compile()
	if err != nil {
		return nil, compiler.fueltank.Consumed, errors.Wrap(err, "")
	}

	return descriptor, compiler.fueltank.Consumed, nil
}

// ValidateCall verifies the IxnObject's callsite and calldata for a given engineio.LogicDriver.
// Returns an error if the logic is supported by PISA or if the callsite does not exist or if the
// callsite is not compatible with the ixn type or if the calldata is invalid for the callsite.
func (engine Engine) ValidateCall(ctx context.Context, logic engineio.LogicDriver, ixn *engineio.IxnObject) error {
	// Check logic driver engine
	if logic.Engine() != engineio.PISA {
		return errors.New("incompatible logic driver: not a PISA logic")
	}

	// Get the callsite information from the logic and verify that it exists
	callsite, ok := engine.logic.GetCallsite(ixn.Callsite())
	if !ok {
		return errors.Errorf("invalid callsite '%v': does not exist", ixn.Callsite())
	}

	// Check that the callsite kind and ixn type of the IxnObject are compatible
	switch callsite.Kind {
	case engineio.InvokableCallsite:
		if ixn.IxType() != types.IxLogicInvoke {
			return errors.Errorf("invalid callsite '%v' for IxnLogicInvoke", ixn.Callsite())
		}

	case engineio.DeployerCallsite:
		if ixn.IxType() != types.IxLogicDeploy {
			return errors.Errorf("invalid callsite '%v' for IxnLogicDeploy", ixn.Callsite())
		}

	case engineio.InteractableCallsite, engineio.EnlisterCallsite:
		return errors.Errorf("unsupported callsite kind '%v' for callsite '%v'", callsite.Kind, ixn.Callsite())

	default:
		panic("invalid callsite kind variant detected")
	}

	// Retrieve the Routine for the Callsite from the Logic
	routine, err := engine.GetRoutine(callsite.Ptr)
	if err != nil {
		return errors.Errorf("could not fetch routine for callsite '%v'", ixn.Callsite())
	}

	// Convert the input Calldata into a ValueTable
	if _, err = register.NewValueTable(routine.Inputs, ixn.Calldata()); err != nil {
		return errors.Errorf("invalid calldata for callsite '%v': %v", ixn.Callsite(), err)
	}

	return nil
}

// Call performs the execution of a logic function from the logic driver in the Engine.
// Call will fail if the engine is not already bootstrapped. It will also fail if the callsite from the
// interaction object does not match the call kind or is not consistent with the number of provided contexts.
// It implements the engineio.EngineDriver interface.
func (engine *Engine) Call(
	ctx context.Context,
	kind engineio.CallsiteKind,
	ixn *engineio.IxnObject,
	participants ...engineio.CtxDriver,
) *engineio.CallResult {
	// Check if the engine is bootstrapped
	if !engine.Bootstrapped() {
		return engine.Error(exception.EngineNotBootstrapped, "")
	}

	// Get the callsite information from the logic and verify that it exists
	callsite, ok := engine.logic.GetCallsite(ixn.Callsite())
	if !ok {
		return engine.Errorf(exception.InvalidCallsite, "callsite '%v' does not exist", ixn.Callsite())
	}

	// Verify that the call kind and callsite match
	if kind != callsite.Kind {
		return engine.Errorf(exception.InvalidCallsite, "callsite '%v' is not a deployer", ixn.Callsite())
	}

	switch kind {
	case engineio.InvokableCallsite:
		if kind != callsite.Kind {
			return engine.Errorf(exception.InvalidCallsite, "callsite '%v' is not an invokable", ixn.Callsite())
		}

		if len(participants) != 1 {
			return engine.Errorf(exception.InvalidIxnCtx, "invokable call has malformed context drivers (must have 1)")
		}

	case engineio.DeployerCallsite:
		if kind != callsite.Kind {
			return engine.Errorf(exception.InvalidCallsite, "callsite '%v' is not an deployer", ixn.Callsite())
		}

		if len(participants) != 1 {
			return engine.Errorf(exception.InvalidIxnCtx, "deployer call has malformed context drivers (must have 1)")
		}

	default:
		return engine.Errorf(exception.InvalidCallsite, "unsupported call type '%v'", kind)
	}

	// Generate a set of pointer to load (the callsite pointer and its dependencies)
	elements := append([]uint64{callsite.Ptr}, engine.logic.GetElementDeps(callsite.Ptr)...)
	// Iterate over the element dependency pointers for the callsite
	for _, ptr := range elements {
		// Retrieve the LogicElement from the Logic
		element, ok := engine.logic.GetElement(ptr)
		if !ok {
			return engine.Errorf(exception.ElementNotFound, "element %#x not found", ptr)
		}

		// Load the LogicElement into the Engine
		if err := engine.loadLogicElement(ptr, element); err != nil {
			return engine.Errorf(exception.ElementMalformed, "element %#x not decoded: %v", ptr, err)
		}
	}

	// Exhaust fuel for execution setup
	if !engine.ConsumeFuel(50) {
		return engine.Error(exception.FuelExhausted, "fuel depleted after execution setup")
	}

	// Get the invokable Routine from the engine
	routine, err := engine.GetRoutine(callsite.Ptr)
	if err != nil {
		return engine.Errorf(exception.ElementNotFound, "routine not found for callsite: %v", err)
	}

	// Convert the input Calldata into a RegisterTable
	inputs, err := register.NewValueTable(routine.Inputs, ixn.Calldata())
	if err != nil {
		return engine.Errorf(exception.InvalidInputs, "%v", err)
	}

	// Create a runtime root and execute the routine with it
	root, err := runtime.Root(engine, ixn, engine.internal, participants...)
	if err != nil {
		return engine.Errorf(exception.InvalidIxnCtx, "%v", err)
	}

	// Perform the execution
	result := routine.Execute(root, inputs)
	// Check if an exception was thrown
	if root.ExceptionThrown() {
		return engine.ErrorException(root.GetException())
	}

	// Convert the output RegisterTable into polo.Document
	outputs := make(polo.Document)
	// For each output value, set encoded data for the label
	for label, index := range routine.Outputs.Symbols {
		if value, ok := result[index]; ok {
			outputs.SetRaw(label, value.Data())
		}
	}

	return engine.Result(outputs)
}

// Error returns an engineio.CallResult for some exception.Code and error Message.
// The result contains the consumed fuel and the given exception.Code and Message.
func (engine Engine) Error(kind exception.Code, data string) *engineio.CallResult {
	return engine.ErrorException(exception.Exception(kind, data))
}

// Errorf returns an engineio.CallResult for some exception.Code and error message (with formatting).
// The result contains the consumed fuel and the given exception.Code and Message.
func (engine Engine) Errorf(kind exception.Code, format string, a ...any) *engineio.CallResult {
	return engine.ErrorException(exception.Exceptionf(kind, format, a...))
}

// ErrorException returns an engineio.CallResult for some exception.Object.
// The result contains the consumed fuel and the given ErrorCode and Message.
func (engine Engine) ErrorException(except *exception.Object) *engineio.CallResult {
	return &engineio.CallResult{
		Fuel:       engine.fueltank.Consumed,
		ErrCode:    uint64(except.Code),
		ErrMessage: except.String(),
	}
}

// Result returns an ExecutionResult for some given ExecutionValues.
// The result contains the consumed fuel, generated logs and the ErrorCodeOk.
func (engine Engine) Result(values polo.Document) *engineio.CallResult {
	return &engineio.CallResult{
		Fuel:    engine.fueltank.Consumed,
		ErrCode: uint64(exception.Ok),
		Outputs: values,
	}
}

func (engine Engine) FuelConsumed() engineio.Fuel {
	return engine.fueltank.Consumed
}

func (engine *Engine) ConsumeFuel(fuel engineio.Fuel) bool {
	return engine.fueltank.Exhaust(fuel)
}

func (engine Engine) GetInstruction(opcode runtime.OpCode) *runtime.InstructOperation {
	return engine.instructs[opcode]
}

func (engine *Engine) GetTypeMethod(datatype *register.Typedef, mtcode register.MethodCode) (register.Method, bool) {
	if datatype.Kind() == register.Primitive {
		if methods, ok := engine.builtins[datatype.P]; ok {
			if method := methods[mtcode]; method != nil {
				return method, true
			}
		}
	}

	return nil, false
}

func (engine *Engine) GetTypedef(ptr uint64) (*register.Typedef, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*register.Typedef)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	typedef := new(register.Typedef)
	if err := polo.Depolorize(typedef, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = typedef

	return typedef, nil
}

func (engine *Engine) GetConstant(ptr uint64) (*register.Constant, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*register.Constant)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	constant := new(register.Constant)
	if err := polo.Depolorize(constant, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = constant

	return constant, nil
}

func (engine *Engine) GetRoutine(ptr uint64) (*runtime.Routine, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*runtime.Routine)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	routine := new(runtime.Routine)
	if err := polo.Depolorize(routine, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = routine

	return routine, nil
}

func (engine *Engine) GetStateFields(kind engineio.ContextStateKind) (*register.StateFields, error) {
	var (
		ptr uint64
		ok  bool
	)

	switch kind {
	case engineio.PersistentState:
		ptr, ok = engine.logic.PersistentState()
		if !ok {
			return nil, errors.New("logic does not define a persistent state")
		}

	case engineio.EphemeralState:
		ptr, ok = engine.logic.EphemeralState()
		if !ok {
			return nil, errors.New("logic does not define an ephemeral state")
		}

	default:
		panic("invalid state kind variant")
	}

	if item, cached := engine.elements[ptr]; cached {
		state, _ := item.(*register.StateFields)

		return state, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	state := new(register.StateFields)
	if err := polo.Depolorize(state, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = state

	return state, nil
}

func (engine *Engine) loadLogicElement(ptr uint64, element *engineio.LogicElement) error {
	// If the element pointer has already been loaded into the engine, skip
	if _, exists := engine.elements[ptr]; exists {
		return nil
	}

	// Check the kind of element
	switch ElementKind(element.Kind) {
	case RoutineElement:
		routine := new(runtime.Routine)
		if err := polo.Depolorize(routine, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = routine

	case StateElement:
		state := new(register.StateFields)
		if err := polo.Depolorize(state, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = state

	case TypedefElement:
		typedef := new(register.Typedef)
		if err := polo.Depolorize(typedef, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = typedef

	case ConstantElement:
		constant := new(register.Constant)
		if err := polo.Depolorize(constant, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = constant

	default:
		return errors.Errorf("cannot load element [%#x] into engine: invalid element kind: %v", ptr, element.Kind)
	}

	return nil
}

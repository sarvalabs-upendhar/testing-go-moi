package pisa

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/jug/pisa/register"
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
	instructs InstructionSet
	// builtins represents the methods for builtin types
	builtins map[engineio.Primitive]register.MethodTable

	// elements represents the decoded engineio.LogicElement cache
	elements map[engineio.ElementPtr]any
	// classes represents a name-map for classes to their element pointers
	classes map[string]engineio.ElementPtr
}

// Kind returns the engineio.EngineKind of the PISA
// It implements the engineio.EngineDriver interface.
func (engine Engine) Kind() engineio.EngineKind { return engineio.PISA }

// Call performs the execution of a logic function from the logic driver in the Engine.
// Call will fail if the engine is not already bootstrapped. It will also fail if the callsite from the
// interaction object does not match the call kind or is not consistent with the number of provided contexts.
// It implements the engineio.EngineDriver interface.
func (engine *Engine) Call(
	ctx context.Context,
	ixn *engineio.IxnObject,
	participants ...engineio.CtxDriver,
) *engineio.CallResult {
	// Get the callsite information from the logic and verify that it exists
	callsite, ok := engine.logic.GetCallsite(ixn.Callsite())
	if !ok {
		return engine.Errorf(exception.InvalidCallsite, "callsite '%v' does not exist", ixn.Callsite())
	}

	switch callsite.Kind {
	case engineio.InvokableCallsite:
		if ixn.IxType() != types.IxLogicInvoke {
			return engine.Errorf(exception.InvalidIxnCtx, "invokable callsite cannot be called with IxLogicInvoke")
		}

		if len(participants) != 1 {
			return engine.Errorf(exception.InvalidIxnCtx, "invokable call has insufficient context drivers (must have 1)")
		}

	case engineio.DeployerCallsite:
		if ixn.IxType() != types.IxLogicDeploy {
			return engine.Errorf(exception.InvalidIxnCtx, "deployer callsite cannot be called with IxLogicDeploy")
		}

		if len(participants) != 1 {
			return engine.Errorf(exception.InvalidIxnCtx, "deployer call has insufficient context drivers (must have 1)")
		}

	default:
		return engine.Errorf(exception.InvalidCallsite, "unsupported callsite kind '%v'", callsite.Kind)
	}

	// Generate a set of pointer to load (the callsite pointer and its dependencies)
	elements := append([]engineio.ElementPtr{callsite.Ptr}, engine.logic.GetElementDeps(callsite.Ptr)...)
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
	root, err := Root(engine, ixn, engine.internal, participants...)
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

func (engine Engine) GetInstruction(opcode OpCode) *InstructOperation {
	return engine.instructs[opcode]
}

func (engine *Engine) GetTypeMethod(datatype *engineio.Datatype, mtcode register.MethodCode) (register.Method, bool) {
	if datatype.Kind == engineio.PrimitiveType {
		if methods, ok := engine.builtins[datatype.Prim]; ok {
			if method := methods[mtcode]; method != nil {
				return method, true
			}
		}
	}

	return nil, false
}

func (engine *Engine) GetTypedef(ptr engineio.ElementPtr) (*engineio.Datatype, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*engineio.Datatype)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	typedef := new(engineio.Datatype)
	if err := polo.Depolorize(typedef, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = typedef

	return typedef, nil
}

func (engine *Engine) GetConstant(ptr engineio.ElementPtr) (*register.Constant, error) {
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

func (engine *Engine) GetRoutine(ptr engineio.ElementPtr) (*Routine, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*Routine)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	routine := new(Routine)
	if err := polo.Depolorize(routine, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = routine

	return routine, nil
}

func (engine *Engine) GetStateFields(kind engineio.ContextStateKind) (*engineio.StateFields, error) {
	var (
		ptr engineio.ElementPtr
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
		state, _ := item.(*engineio.StateFields)

		return state, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	state := new(engineio.StateFields)
	if err := polo.Depolorize(state, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = state

	return state, nil
}

func (engine *Engine) loadLogicElement(ptr engineio.ElementPtr, element *engineio.LogicElement) error {
	// If the element pointer has already been loaded into the engine, skip
	if _, exists := engine.elements[ptr]; exists {
		return nil
	}

	// Check the kind of element
	switch element.Kind {
	case RoutineElement:
		routine := new(Routine)
		if err := polo.Depolorize(routine, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = routine

	case StateElement:
		state := new(engineio.StateFields)
		if err := polo.Depolorize(state, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = state

	case TypedefElement:
		typedef := new(engineio.Datatype)
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

package pisa

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/types"
)

// Engine represents the execution engine for the PISA Virtual Machine.
// It implements the engineio.EngineDriver interface.
type Engine struct {
	// runtime represents the runtime definitions for PISA
	runtime *Runtime
	// callstack represents the callstack of the engine
	callstack callstack
	// fueltank represents the fuel tank of the engine
	fueltank *engineio.FuelTank

	// logic represents the logic driver
	logic engineio.LogicDriver
	// elements represents the decoded engineio.LogicElement cache
	elements map[engineio.ElementPtr]any
	// classes represents a name-map for classes to their element pointers
	classes map[string]engineio.ElementPtr

	// persistent ctx driver
	persistent engineio.CtxDriver
	// ephemeralS sender ctx driver
	ephemeralS engineio.CtxDriver
	// ephemeralR receiver ctx driver
	ephemeralR engineio.CtxDriver //nolint:unused

	// ixn driver
	interaction *engineio.IxnObject
	// env driver
	environment engineio.EnvDriver
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
	engine.callstack.push(&callframe{scope: "root", label: "start"})
	defer engine.callstack.pop()

	// Get the callsite information from the logic and verify that it exists
	callsite, ok := engine.logic.GetCallsite(ixn.Callsite())
	if !ok {
		return engine.ErrResult(exceptionf(InitError,
			"callsite '%v' does not exist", ixn.Callsite()).traced(engine.callstack.trace()),
		)
	}

	engine.interaction = ixn

	switch callsite.Kind {
	case engineio.InvokableCallsite:
		if ixn.IxType() != types.IxLogicInvoke {
			return engine.ErrResult(exception(InitError,
				"invokable callsite cannot be called without IxLogicInvoke").traced(engine.callstack.trace()),
			)
		}

		if len(participants) != 1 {
			return engine.ErrResult(exception(InitError,
				"insufficient context drivers for invokable call (needs 1)").traced(engine.callstack.trace()),
			)
		}

		engine.ephemeralS = participants[0]

	case engineio.DeployerCallsite:
		if ixn.IxType() != types.IxLogicDeploy {
			return engine.ErrResult(exception(InitError,
				"deployed callsite cannot be called without IxLogicDeploy").traced(engine.callstack.trace()),
			)
		}

		if len(participants) != 1 {
			return engine.ErrResult(exception(InitError,
				"insufficient context drivers for deployer call (needs 1)").traced(engine.callstack.trace()),
			)
		}

		engine.ephemeralS = participants[0]

	default:
		return engine.ErrResult(exceptionf(InitError,
			"unsupported callsite kind '%v'", callsite.Kind).traced(engine.callstack.trace()),
		)
	}

	// Generate a set of pointer to load (the callsite pointer and its dependencies)
	elements := append([]engineio.ElementPtr{callsite.Ptr}, engine.logic.GetElementDeps(callsite.Ptr)...)
	// Iterate over the element dependency pointers for the callsite
	for _, ptr := range elements {
		// Retrieve the LogicElement from the Logic
		element, ok := engine.logic.GetElement(ptr)
		if !ok {
			return engine.ErrResult(exceptionf(InitError,
				"element %#x not found", ptr).traced(engine.callstack.trace()),
			)
		}

		// Load the LogicElement into the Engine
		if err := engine.loadLogicElement(ptr, element); err != nil {
			return engine.ErrResult(exceptionf(InitError,
				"element %#x malformed: %v", ptr, err).traced(engine.callstack.trace()),
			)
		}
	}

	// Get the invokable Routine from the engine
	routine, err := engine.GetRoutine(callsite.Ptr)
	if err != nil {
		return engine.ErrResult(exceptionf(InitError,
			"routine not found for callsite: %v", err).traced(engine.callstack.trace()),
		)
	}

	calldata := make(polo.Document)
	// Decode the payload calldata into a polo.Document
	if ixn.Calldata() != nil {
		if err = polo.Depolorize(&calldata, ixn.Calldata()); err != nil {
			return engine.ErrResult(exceptionf(InitError,
				"could not decode calldata into polo document: %v", err).traced(engine.callstack.trace()),
			)
		}
	}

	// Convert the input Calldata into a RegisterSet
	inputs, err := NewRegisterSet(routine.Inputs, calldata)
	if err != nil {
		return engine.ErrResult(exceptionf(InitError, "invalid inputs: %v", err).traced(engine.callstack.trace()))
	}

	// Run the routine and return the error result
	result, except := engine.run(routine, inputs)
	if except != nil {
		return engine.ErrResult(except)
	}

	// Convert the output RegisterTable into polo.Document
	outputs := make(polo.Document)
	// For each output value, set encoded data for the label
	for label, index := range routine.Outputs.Symbols {
		if value, ok := result[index]; ok {
			outputs.SetRaw(label, value.Data())
		}
	}

	return engine.OkResult(outputs)
}

// ErrResult returns an engineio.CallResult for some Exception.
// The result contains the consumed fuel and the polo-encoded exception object
func (engine Engine) ErrResult(except *Exception) *engineio.CallResult {
	return &engineio.CallResult{
		Consumed: engine.fueltank.Consumed,
		Error:    except.Bytes(),
	}
}

// OkResult returns an engineio.CallResult for some given polo.Document output.
// The result contains the consumed fuel and output values as doc-encoded bytes
func (engine Engine) OkResult(values polo.Document) *engineio.CallResult {
	return &engineio.CallResult{
		Consumed: engine.fueltank.Consumed,
		Outputs:  values.Bytes(),
	}
}

func (engine *Engine) run(runnable Runnable, inputs RegisterSet) (RegisterSet, *Exception) {
	// Perform input validation
	if err := inputs.Validate(runnable.callfields().Inputs); err != nil {
		return nil, exceptionf(CallError, "invalid inputs: %v", err).traced(engine.callstack.trace())
	}

	outputs, except := runnable.run(engine, inputs)
	if except != nil {
		return nil, except
	}

	// Perform output validation
	if err := outputs.Validate(runnable.callfields().Outputs); err != nil {
		return nil, exceptionf(CallError, "invalid outputs: %v", err).traced(engine.callstack.trace())
	}

	return outputs, nil
}

func (engine *Engine) exhaustFuel(fuel engineio.Fuel) bool {
	return engine.fueltank.Exhaust(fuel)
}

func (engine Engine) lookupInstruction(opcode OpCode) InstructionFunc {
	return engine.runtime.instructs[opcode]
}

func (engine *Engine) lookupMethod(dt *Datatype, mtcode MethodCode) (Method, bool) {
	if dt.Kind == PrimitiveType {
		if methods, ok := engine.runtime.bmethods[dt.Prim]; ok {
			if method := methods[mtcode]; method != nil {
				return method, true
			}
		}
	}

	return nil, false
}

func (engine *Engine) GetTypedef(ptr engineio.ElementPtr) (*Datatype, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*Datatype)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	typedef := new(Datatype)
	if err := polo.Depolorize(typedef, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = typedef

	return typedef, nil
}

func (engine *Engine) GetConstant(ptr engineio.ElementPtr) (*Constant, error) {
	if item, cached := engine.elements[ptr]; cached {
		routine, _ := item.(*Constant)

		return routine, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	constant := new(Constant)
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

func (engine *Engine) GetStateFields(kind engineio.ContextStateKind) (*StateFields, error) {
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
		state, _ := item.(*StateFields)

		return state, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	state := new(StateFields)
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
		state := new(StateFields)
		if err := polo.Depolorize(state, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = state

	case ClassElement:
		class := new(Datatype)
		if err := polo.Depolorize(class, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = class

	case TypedefElement:
		typedef := new(Datatype)
		if err := polo.Depolorize(typedef, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = typedef

	case ConstantElement:
		constant := new(Constant)
		if err := polo.Depolorize(constant, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = constant

	default:
		return errors.Errorf("cannot load element [%#x] into engine: invalid element kind: %v", ptr, element.Kind)
	}

	return nil
}

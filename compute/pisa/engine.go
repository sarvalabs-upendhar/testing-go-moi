package pisa

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/compute/engineio"
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

	// persistent represents the logic's ctx state driver
	persistent engineio.CtxDriver
	// sephemeral represents the ixn sender's ctx state driver
	sephemeral engineio.CtxDriver
	// rephemeral represents the ixn receiver's ctx state driver
	rephemeral engineio.CtxDriver //nolint:unused

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
) (*engineio.CallResult, error) {
	engine.callstack.push(&callframe{scope: "root", label: "start"})
	defer engine.callstack.pop()

	// Get the callsite information from the logic and verify that it exists
	callsite, ok := engine.logic.GetCallsite(ixn.Callsite())
	if !ok {
		return nil, errors.Errorf("callsite '%v' does not exist", ixn.Callsite())
	}

	engine.interaction = ixn

	switch kind := callsite.Kind; kind {
	case engineio.InvokableCallsite, engineio.DeployerCallsite:
		if ixn.IxType() != callsite.Kind.IxnType() {
			return nil, errors.Errorf("callsite kind '%v' is not appropriate for %v", kind, ixn.IxType())
		}

		if len(participants) != 1 {
			return nil, errors.Errorf("insufficient context drivers for %v call (needs 1)", kind)
		}

		// Retrieve the sender context state and check if it is nil
		engine.sephemeral = participants[0]

	default:
		return nil, errors.Errorf("unsupported callsite kind '%v'", kind)
	}

	// Generate a set of pointer to load (the callsite pointer and its dependencies)
	elements := append([]engineio.ElementPtr{callsite.Ptr}, engine.logic.GetElementDeps(callsite.Ptr)...)
	// Iterate over the element dependency pointers for the callsite
	for _, ptr := range elements {
		// Retrieve the LogicElement from the Logic
		element, ok := engine.logic.GetElement(ptr)
		if !ok {
			return nil, errors.Errorf("missing element %#x", ptr)
		}

		// Load the LogicElement into the Engine
		if err := engine.loadLogicElement(ptr, element); err != nil {
			return nil, errors.Errorf("malformed element %#x: %v", ptr, err)
		}
	}

	// Get the invokable Routine from the engine
	routine, err := engine.GetRoutine(callsite.Ptr)
	if err != nil {
		return nil, errors.Errorf("routine not found for callsite: %v", err)
	}

	calldata := make(polo.Document)
	// Decode the payload calldata into a polo.Document
	if ixn.Calldata() != nil {
		if err = polo.Depolorize(&calldata, ixn.Calldata()); err != nil {
			return nil, errors.Errorf("could not decode calldata into polo document: %v", err)
		}
	}

	// Convert the input Calldata into a RegisterSet
	inputs, err := NewRegisterSet(routine.Inputs, calldata)
	if err != nil {
		return engine.ErrResult(exceptionf(
			InitError, "invalid inputs: %v", err).
			traced(engine.callstack.trace())), nil
	}

	// Run the routine and return the error result
	result, except := engine.run(routine, inputs)
	if except != nil {
		return engine.ErrResult(except), nil
	}

	// Convert the output RegisterTable into polo.Document
	outputs := make(polo.Document)
	// For each output value, set encoded data for the label
	for label, index := range routine.Outputs.Symbols {
		if value, ok := result[index]; ok {
			outputs.SetRaw(label, value.Data())
		}
	}

	return engine.OkResult(outputs), nil
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

func (engine Engine) lookupBuiltin(ptr uint64) (*Builtin, bool) {
	builtin, ok := engine.runtime.builtinLibrary[ptr]

	return builtin, ok
}

func (engine *Engine) lookupMethod(dt Datatype, mtcode MethodCode) (Method, bool) {
	switch dt.Kind() {
	case Primitive:
		primitive, _ := dt.(PrimitiveDatatype)

		if methods, ok := engine.runtime.primitiveMethods[primitive]; ok {
			if method := methods[mtcode]; method != nil {
				return method, true
			}
		}

	case BuiltinClass:
		builtin, _ := dt.(BuiltinDatatype)

		if class, ok := engine.runtime.builtinClasses[builtin.name]; ok {
			if method := class.methods[mtcode]; method != nil {
				return method, true
			}
		}

	case Class:
		class, _ := dt.(ClassDatatype)

		methodptr := class.methods[mtcode]

		element, exists := engine.logic.GetElement(methodptr)
		if !exists {
			return nil, false
		}

		method := new(RoutineMethod)
		if err := polo.Depolorize(method, element.Data); err != nil {
			return nil, false
		}

		engine.elements[methodptr] = method

		return method, true
	}

	return nil, false
}

func (engine *Engine) GetTypedef(ptr engineio.ElementPtr) (Datatype, error) {
	if item, cached := engine.elements[ptr]; cached {
		typedef, _ := item.(Datatype)

		return typedef, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	typedef, err := DecodeDatatype(element.Data)
	if err != nil {
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

func (engine *Engine) GetMethod(ptr engineio.ElementPtr) (*Method, error) {
	if item, cached := engine.elements[ptr]; cached {
		method, _ := item.(*Method)

		return method, nil
	}

	element, ok := engine.logic.GetElement(ptr)
	if !ok {
		return nil, errors.Errorf("could not find element at %#x", ptr)
	}

	method := new(Method)
	if err := polo.Depolorize(method, element.Data); err != nil {
		return nil, err
	}

	engine.elements[ptr] = method

	return method, nil
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

	case ClassElement, TypedefElement:
		datatype, err := DecodeDatatype(element.Data)
		if err != nil {
			return err
		}

		engine.elements[ptr] = datatype

	case ConstantElement:
		constant := new(Constant)
		if err := polo.Depolorize(constant, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = constant

	case MethodElement:
		method := new(RoutineMethod)
		if err := polo.Depolorize(method, element.Data); err != nil {
			return err
		}

		engine.elements[ptr] = method

	default:
		return errors.Errorf("cannot load element [%#x] into engine: invalid element kind: %v", ptr, element.Kind)
	}

	return nil
}

package pisa

import (
	"fmt"
	"strings"
)

// MaxCallDepth defines the max call depth for child Scope objects
const MaxCallDepth = 1024

type callframe struct {
	scope string
	label string
	point uint64

	instruct *struct {
		line uint64
		code string
	}
}

func (frame callframe) String() string {
	var str strings.Builder

	str.WriteString(fmt.Sprintf("%v.%v()", frame.scope, frame.label))

	if !(frame.scope == "root" && frame.label == "start") {
		str.WriteString(fmt.Sprintf(" [%#x]", frame.point))
	}

	if frame.instruct != nil {
		str.WriteString(fmt.Sprintf(" ... [%#x: %v]", frame.instruct.line, frame.instruct.code))
	}

	return str.String()
}

type callstack []*callframe

func (stack *callstack) push(frame *callframe) bool {
	if stack.depth() >= MaxCallDepth {
		return false
	}

	*stack = append(*stack, frame)

	return true
}

func (stack *callstack) pop() {
	if stack.depth() == 0 {
		return
	}

	s := *stack
	*stack = s[:len(s)-1]
}

func (stack callstack) head() *callframe {
	if stack.depth() == 0 {
		return nil
	}

	return stack[len(stack)-1]
}

func (stack callstack) inject(line uint64, instruct Instruction) {
	stack.head().instruct = &struct {
		line uint64
		code string
	}{
		line,
		instruct.String(),
	}
}

func (stack callstack) depth() uint64 {
	return uint64(len(stack))
}

func (stack callstack) trace() []string {
	trace := make([]string, 0, len(stack))

	for _, frame := range stack {
		trace = append(trace, frame.String())
	}

	return trace
}

// callscope represents an isolated environment for executing some logic Instructions.
// It acts as an execution space with its own register sets for memory, inputs and outputs.
type callscope struct {
	engine *Engine

	inputs  RegisterSet
	outputs RegisterSet
	memory  RegisterSet
}

// throw returns the given Exception object traced with the current execution trace.
func (scope *callscope) throw(except *Exception) *Exception {
	return except.traced(scope.engine.callstack.trace())
}

// raise returns a continueException object with the given Exception traced with the current execution trace.
func (scope *callscope) raise(except *Exception) continueException {
	return continueException{exception: scope.throw(except)}
}

// propagate returns a continueException object with the given Exception object preserving its execution trace.
func (scope *callscope) propagate(except *Exception) continueException {
	return continueException{exception: except}
}

// getPtrValue resolves a register ID into a PtrValue
// The Register at the reg address must exist and be of type TypePtr.
func (scope *callscope) getPtrValue(regID byte) (PtrValue, *Exception) {
	// Retrieve the Register object
	reg := scope.memory.Get(regID)
	// Check that the register type is Ptr
	if reg.Type() != PrimitivePtr {
		return 0, scope.throw(exceptionf(ReferenceError, "not a pointer: $%v", regID))
	}

	// Cast into a PtrValue
	ptr, _ := reg.(PtrValue)
	// Return PtrValue as a uint64
	return ptr, nil
}

// getSymmetricValues obtains two registers of the same Datatype from the given register IDs.
// Returns an error if the registers are not of the same type.
func (scope *callscope) getSymmetricValues(a, b byte) (regA, regB RegisterValue, except *Exception) {
	// Retrieve the register for A, B
	regA, regB = scope.memory.Get(a), scope.memory.Get(b)

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return nil, nil, scope.throw(exceptionf(ValueError, "not symmetric: [$%v, $%v]", a, b))
	}

	return regA, regB, nil
}

// callMethod calls the given method code with the given set of registers as an input.
// The first value of the input register [$0] is treated as the method register on which the method is called.
// Returns an Exception for the following:
//   - CallError: method register is missing or input/output validation failed
//   - NotImplementedError: method is not implemented by the method register
//   - Exception: any exception returned from method execution
func (scope *callscope) callMethod(method MethodCode, inputs RegisterSet) (RegisterSet, *Exception) {
	// Error if inputs is missing the method register [$0]
	if _, exists := inputs[0]; !exists {
		return nil, scope.throw(exceptionf(CallError, "missing method register"))
	}

	// Retrieve the method register from the first value
	methodRegister := inputs.Get(0)
	// Check that register implements method and retrieve it
	methodObject, ok := scope.engine.lookupMethod(methodRegister.Type(), method)
	if !ok {
		return nil, scope.throw(exceptionf(
			NotImplementedError, "%v does not implement %v", methodRegister.Type(), method,
		))
	}

	// Execute the method
	return scope.engine.run(methodObject, inputs)
}

func (scope *callscope) callMethodThrow(reg RegisterValue) (StringValue, *Exception) {
	// Call the __throw__ method on the given register
	outputs, except := scope.callMethod(MethodThrow, RegisterSet{0: reg})
	if except != nil {
		return "", except
	}

	//nolint:forcetypeassert
	return outputs[0].(StringValue), nil
}

func (scope *callscope) callMethodJoin(a, b RegisterValue) (RegisterValue, *Exception) {
	// Call the __join__ method on the given register
	outputs, except := scope.callMethod(MethodJoin, RegisterSet{0: a, 1: b})
	if except != nil {
		return nil, except
	}

	return outputs[0], nil
}

func (scope *callscope) callMethodCompare(method MethodCode, a, b RegisterValue) (BoolValue, *Exception) {
	// Panic, if this method is used for anything but __lt__, __gt__, __eq__
	if method < MethodLt || method > MethodEq {
		panic("callMethodCompare used incorrectly")
	}

	// Call the comparison method on the given register
	outputs, except := scope.callMethod(method, RegisterSet{0: a, 1: b})
	if except != nil {
		return false, except
	}

	//nolint:forcetypeassert
	return outputs[0].(BoolValue), nil
}

func (scope *callscope) callMethodBool(reg RegisterValue) (BoolValue, *Exception) {
	// Call the __bool__ method on the given register
	outputs, except := scope.callMethod(MethodBool, RegisterSet{0: reg})
	if except != nil {
		return false, except
	}

	//nolint:forcetypeassert
	return outputs[0].(BoolValue), nil
}

func (scope *callscope) callMethodStr(reg RegisterValue) (StringValue, *Exception) {
	// Call the __str__ method on the given register
	outputs, except := scope.callMethod(MethodStr, RegisterSet{0: reg})
	if except != nil {
		return "", except
	}

	//nolint:forcetypeassert
	return outputs[0].(StringValue), nil
}

func (scope *callscope) callMethodAddr(reg RegisterValue) (AddressValue, *Exception) {
	// Call the __addr__ method on the given register
	outputs, except := scope.callMethod(MethodAddr, RegisterSet{0: reg})
	if except != nil {
		return ZeroAddress, except
	}

	//nolint:forcetypeassert
	return outputs[0].(AddressValue), nil
}

func (scope *callscope) callMethodLen(reg RegisterValue) (U64Value, *Exception) {
	// Call the __len__ method on the given register
	outputs, except := scope.callMethod(MethodLen, RegisterSet{0: reg})
	if except != nil {
		return 0, except
	}

	//nolint:forcetypeassert
	return outputs[0].(U64Value), nil
}

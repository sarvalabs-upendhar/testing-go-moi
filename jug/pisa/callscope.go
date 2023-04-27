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

func (scope *callscope) exception(class ExceptionClass, data string) *Exception {
	return exception(class, scope.engine.callstack.trace(), data)
}

func (scope *callscope) exceptionf(class ExceptionClass, format string, a ...any) *Exception {
	return exceptionf(class, scope.engine.callstack.trace(), format, a...)
}

// getPtrValue resolves a register ID into a PtrValue
// The Register at the reg address must exist and be of type TypePtr.
func (scope *callscope) getPtrValue(regID byte) (PtrValue, *Exception) {
	// Retrieve the Register object
	reg := scope.memory.Get(regID)
	// Check that the register type is Ptr
	if reg.Type() != TypePtr {
		return 0, scope.exceptionf(ReferenceError, "register $%v is not a pointer", regID)
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
		return nil, nil, scope.exceptionf(ValueError, "registers $%v and $%v are not symmetric", a, b)
	}

	return regA, regB, nil
}

func (scope *callscope) callMethodBool(reg RegisterValue) (BoolValue, *Exception) {
	// Check that register implements __bool__
	method, ok := scope.engine.lookupMethod(reg.Type(), MethodBool)
	if !ok {
		return false, scope.exceptionf(NotImplementedError, "%v does not implement __bool__", reg.Type())
	}

	// Execute the __bool__ method
	outputs, except := scope.engine.run(method, RegisterSet{0: reg})
	if except != nil {
		return false, except.Wrap(scope.engine.callstack.head().String())
	}

	//nolint:forcetypeassert
	return outputs[0].(BoolValue), nil
}

func (scope *callscope) callMethodStr(reg RegisterValue) (StringValue, *Exception) {
	// Check that register implements __str__
	method, ok := scope.engine.lookupMethod(reg.Type(), MethodStr)
	if !ok {
		return "", scope.exceptionf(NotImplementedError, "%v does not implement __str__", reg.Type())
	}

	// Execute the __str__ method
	outputs, except := scope.engine.run(method, RegisterSet{0: reg})
	if except != nil {
		return "", except.Wrap(scope.engine.callstack.head().String())
	}

	//nolint:forcetypeassert
	return outputs[0].(StringValue), nil
}

func (scope *callscope) callMethodThrow(reg RegisterValue) (StringValue, *Exception) {
	// Check that register implements __throw__
	method, ok := scope.engine.lookupMethod(reg.Type(), MethodThrow)
	if !ok {
		return "", scope.exceptionf(NotImplementedError, "%v does not implement __throw__", reg.Type())
	}

	// Execute the __throw__ method
	outputs, except := scope.engine.run(method, RegisterSet{0: reg})
	if except != nil {
		return "", except.Wrap(scope.engine.callstack.head().String())
	}

	//nolint:forcetypeassert
	return outputs[0].(StringValue), nil
}

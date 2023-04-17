package pisa

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/exception"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/types"
)

// ExecutionFlow defines an enum with variations
// that describe the current nature of execution flow
type ExecutionFlow int

const (
	FlowOk ExecutionFlow = iota
	FlowJump
	FlowExcept
	FlowTerminate
)

// MaxCallDepth defines the max call depth for child Scope objects
const MaxCallDepth = 1024

// CallScope represents an isolated runtime environment for executing some logic Instructions.
// It acts as an execution space with its own register set and accept/return value slots.
// Implements the register.ExecutionScope interface
type CallScope struct {
	engine  *Engine
	routine Routine

	calldepth   uint64
	instructptr uint64

	jump   *uint64
	flow   ExecutionFlow
	except *exception.Object

	ixn      *engineio.IxnObject
	internal engineio.CtxDriver
	sender   engineio.CtxDriver
	receiver engineio.CtxDriver

	inputs    register.ValueTable
	outputs   register.ValueTable
	registers register.ValueTable
}

// Root generates a root Scope for some Runtime (pisa.Engine).
// It has a call depth of 0 and is the root of all execution calls.
// It requires the logic's engineio.CtxDriver along with a variadic
// number of them, that depends on the nature of the Interaction
func Root(
	engine *Engine,
	ixn *engineio.IxnObject,
	logic engineio.CtxDriver,
	participants ...engineio.CtxDriver,
) (
	*CallScope, error,
) {
	// Declare the base scope with the runtime, routine and ixn
	scope := &CallScope{ixn: ixn, engine: engine, internal: logic}

	// Check the number of participant contexts based on the interaction type
	switch ixn.IxType() {
	case types.IxLogicDeploy, types.IxLogicInvoke:
		if len(participants) != 1 {
			return nil, errors.Errorf("invalid no of participant contexts for %v", ixn.IxType())
		}

		scope.sender = participants[0]

	default:
		return nil, errors.Errorf("unsupported interaction type %v", ixn.IxType())
	}

	return scope, nil
}

func (scope *CallScope) child(routine Routine, inputs register.ValueTable) *CallScope {
	if calldepth := scope.calldepth + 1; calldepth > MaxCallDepth {
		// todo: throw exception
		return nil
	}

	return &CallScope{
		routine: routine,
		ixn:     scope.ixn,
		engine:  scope.engine,

		calldepth: scope.calldepth + 1,

		internal: scope.internal,
		sender:   scope.sender,
		receiver: scope.receiver,

		inputs:    inputs,
		outputs:   make(register.ValueTable),
		registers: make(register.ValueTable),
	}
}

func (scope *CallScope) run() {
	// Execution Loop
	for !scope.done() {
		switch scope.flow {
		case FlowTerminate:
			return

		case FlowExcept:
			return

		case FlowOk:
			// Get the next instruction from the program
			instruct := scope.read()
			// Get the operation for the opcode from the instruction set
			op := scope.engine.GetInstruction(instruct.Op)
			// Execute the instruction with the args
			consumed := op(scope, instruct.Args)

			// Attempt to exhaust some fuel from the engine -> fails if there is not enough fuel left
			if ok := scope.engine.ConsumeFuel(consumed); !ok {
				// Fuel Depleted - unread the instruction and throw exception
				scope.unread()
				scope.Throw(exception.Exception(exception.FuelExhausted, ""))

				continue
			}

			// Execute the instruction
			if scope.ExceptionThrown() {
				scope.unread()
			}

		case FlowJump:
			// If attempting to jump to instruction that does not exist
			if *scope.jump >= scope.routine.Instructs.Len() {
				// Throw an InvalidJump exception
				scope.Throw(exception.Exception(exception.InvalidJumpsite, "destination out of bounds"))

				continue
			}

			if instruct := scope.routine.Instructs[*scope.jump]; instruct.Op != DEST {
				// Throw an InvalidJump exception
				scope.Throw(exception.Exception(exception.InvalidJumpsite, "invalid jump destination"))

				continue
			}

			// Set the new instruction pointer and reset the flow
			scope.instructptr = *scope.jump
			scope.flow = FlowOk
			scope.jump = nil
		}
	}
}

// Throw throws an exception in the execution
// Scope and changes the flow to FlowExcept
func (scope *CallScope) Throw(except *exception.Object) {
	scope.flow = FlowExcept
	scope.except = except
}

// ExceptionThrown returns whether the execution
// Scope is currently in the FlowExcept flow
func (scope *CallScope) ExceptionThrown() bool {
	return scope.flow == FlowExcept
}

// GetException returns the current exception in the execution Scope.
// Returns nil if flow is not FlowExcept
func (scope *CallScope) GetException() *exception.Object {
	return scope.except
}

// GetPtrValue resolves a register ID into uint64 pointer address.
// The Register at the reg address must exist and be of type TypePtr.
func (scope *CallScope) GetPtrValue(regID byte) (engineio.ElementPtr, *exception.Object) {
	// Retrieve the Register object
	reg, exists := scope.registers.Get(regID)
	if !exists {
		return 0, exception.Exceptionf(exception.RegisterNotFound, "register $%v", regID)
	}

	// Check that the register type is Ptr
	if reg.Type() != engineio.TypePtr {
		return 0, exception.Exceptionf(exception.InvalidRegisterType, "register $%v is not a pointer", regID)
	}

	// Cast into a PtrValue
	ptr, _ := reg.(register.PtrValue)
	// Return PtrValue as a uint64
	return engineio.ElementPtr(ptr), nil
}

// GetSymmetricValues obtains two registers of the same Typedef for the given register IDs.
// Returns an error if either of the Registers are empty or are not of the same type.
func (scope *CallScope) GetSymmetricValues(a, b byte) (
	regA, regB register.Value,
	except *exception.Object,
) {
	var exists bool

	// Retrieve the register for A
	if regA, exists = scope.registers.Get(a); !exists {
		return nil, nil, exception.Exceptionf(exception.RegisterNotFound, "register $%v", a)
	}

	// Retrieve the register for B
	if regB, exists = scope.registers.Get(b); !exists {
		return nil, nil, exception.Exceptionf(exception.RegisterNotFound, "register $%v", b)
	}

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return nil, nil, exception.Exceptionf(exception.InvalidRegisterType, "unequal types ($%v, $%v)", a, b)
	}

	return regA, regB, nil
}

func (scope *CallScope) read() Instruction {
	if scope.done() {
		return Instruction{}
	}

	instruct := scope.routine.Instructs[scope.instructptr]
	scope.instructptr++

	return instruct
}

func (scope *CallScope) unread() {
	if scope.instructptr == 0 {
		return
	}

	scope.instructptr--
}

func (scope CallScope) done() bool {
	return scope.instructptr >= uint64(len(scope.routine.Instructs))
}

func (scope *CallScope) stop() {
	scope.flow = FlowTerminate
}

func (scope *CallScope) jumpTo(ptr uint64) {
	scope.flow = FlowJump
	scope.jump = &ptr
}

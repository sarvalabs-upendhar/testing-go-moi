package runtime

import (
	"github.com/sarvalabs/moichain/jug/pisa/exceptions"
	"github.com/sarvalabs/moichain/jug/pisa/register"
)

const MaxCallDepth = 1024

type FlowCode int

const (
	FlowOk FlowCode = iota
	FlowJump
	FlowExcept
	FlowTerminate
)

type Environment interface {
	ConsumedFuel() uint64
	ExhaustFuel(uint64) bool

	ReadStorage(uint8) ([]byte, error)
	WriteStorage(uint8, []byte) error
	GetStorageLayout() (*StorageLayout, bool)

	GetConstant(uint64) (*Constant, bool)
	GetRoutine(ptr uint64) (*Routine, bool)
	GetInstruction(OpCode) *InstructOperation
	GetSymbolicType(uint64) (*register.Typedef, bool)
	GetTypeMethod(*register.Typedef, register.MethodCode) (register.Method, bool)
}

// RoutineScope represents an isolated runtime environment for executing some logic Instructions.
// It acts as an execution space with its own Register set and load/yield value slots.
type RoutineScope struct {
	routine Routine
	environ Environment

	calldepth   uint64
	instructptr uint64

	flow   FlowCode
	jump   *uint64
	except *exceptions.ExceptionObject

	inputs    register.ValueTable
	outputs   register.ValueTable
	registers register.ValueTable
}

// RootExecutionScope generates a root RoutineScope for some Environment (pisa.Engine).
// It has a call depth of 0 and is used to capture execution halting.
func RootExecutionScope(environ Environment) register.ExecutionScope {
	return &RoutineScope{
		environ: environ,
	}
}

func (scope *RoutineScope) child(routine Routine, inputs register.ValueTable) *RoutineScope {
	if calldepth := scope.calldepth + 1; calldepth > MaxCallDepth {
		// todo: throw exception
		return nil
	}

	return &RoutineScope{
		routine:   routine,
		environ:   scope.environ,
		calldepth: scope.calldepth + 1,

		inputs:    inputs,
		outputs:   make(register.ValueTable),
		registers: make(register.ValueTable),
	}
}

func (scope *RoutineScope) run() {
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
			op := scope.environ.GetInstruction(instruct.Op)

			// Attempt to exhaust some fuel from the engine -> fails if there is not enough fuel left
			if ok := scope.environ.ExhaustFuel(op.Expense(scope)); !ok {
				// Fuel Depleted - unread the instruction and throw exception
				scope.unread()
				scope.Throw(exceptions.Exception(exceptions.ExceptionFuelExhausted, ""))

				continue
			}

			// Execute the instruction
			if op.Execute(scope, instruct.Args); scope.ExceptionThrown() {
				scope.unread()
			}

		case FlowJump:
			// If attempting to jump to instruction that does not exist
			if *scope.jump >= scope.routine.Instructs.Len() {
				// Throw an InvalidJump exception
				scope.Throw(exceptions.Exception(exceptions.ExceptionInvalidJump, "destination out of bounds"))

				continue
			}

			if instruct := scope.routine.Instructs[*scope.jump]; instruct.Op != DEST {
				// Throw an InvalidJump exception
				scope.Throw(exceptions.Exception(exceptions.ExceptionInvalidJump, "invalid jump destination"))

				continue
			}

			// Set the new instruction pointer and reset the flow
			scope.instructptr = *scope.jump
			scope.flow = FlowOk
			scope.jump = nil
		}
	}
}

// GetPtrValue resolves a register ID into uint64 pointer address.
// The Register at the reg address must exist and be of type TypePtr.
func (scope *RoutineScope) GetPtrValue(regID byte) (uint64, *exceptions.ExceptionObject) {
	// Retrieve the Register object
	reg, exists := scope.registers.Get(regID)
	if !exists {
		return 0, exceptions.Exceptionf(exceptions.ExceptionNotFound, "register $%v", regID)
	}

	// Check that the register type is Ptr
	if reg.Type() != register.TypePtr {
		return 0, exceptions.Exceptionf(exceptions.ExceptionInvalidRegisterType, "register $%v is not a pointer", regID)
	}

	// Cast into a PtrValue
	ptr, _ := reg.(register.PtrValue)
	// Return PtrValue as a uint64
	return uint64(ptr), nil
}

// GetSymmetricValues obtains two registers of the same Typedef for the given register IDs.
// Returns an error if either of the Registers are empty or are not of the same type.
func (scope *RoutineScope) GetSymmetricValues(a, b byte) (
	regA, regB register.Value,
	exception *exceptions.ExceptionObject,
) {
	var exists bool

	// Retrieve the register for A
	if regA, exists = scope.registers.Get(a); !exists {
		return nil, nil, exceptions.Exceptionf(exceptions.ExceptionNotFound, "register $%v", a)
	}

	// Retrieve the register for B
	if regB, exists = scope.registers.Get(b); !exists {
		return nil, nil, exceptions.Exceptionf(exceptions.ExceptionNotFound, "register $%v", b)
	}

	// Check that register types are equal
	if !regA.Type().Equals(regB.Type()) {
		return nil, nil, exceptions.Exceptionf(exceptions.ExceptionInvalidRegisterType, "unequal types ($%v, $%v)", a, b)
	}

	return regA, regB, nil
}

func (scope RoutineScope) done() bool {
	return scope.instructptr >= uint64(len(scope.routine.Instructs))
}

func (scope *RoutineScope) read() Instruction {
	if scope.done() {
		return Instruction{}
	}

	instruct := scope.routine.Instructs[scope.instructptr]
	scope.instructptr++

	return instruct
}

func (scope *RoutineScope) unread() {
	if scope.instructptr == 0 {
		return
	}

	scope.instructptr--
}

func (scope *RoutineScope) stop() {
	scope.flow = FlowTerminate
}

func (scope *RoutineScope) jumpTo(ptr uint64) {
	scope.flow = FlowJump
	scope.jump = &ptr
}

func (scope *RoutineScope) Throw(except *exceptions.ExceptionObject) {
	scope.flow = FlowExcept
	scope.except = except
}

func (scope *RoutineScope) ExceptionThrown() bool {
	return scope.flow == FlowExcept
}

func (scope *RoutineScope) GetException() *exceptions.ExceptionObject {
	return scope.except
}

package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/jug/engineio"
)

// Runnable represents an object that can be executed.
type Runnable interface {
	name() string
	callfields() CallFields
	ptr() engineio.ElementPtr

	run(*Engine, RegisterSet) (RegisterSet, *Exception)
}

// Routine represents an executable of PISA bytecode.
// Implements the Runnable interface.
type Routine struct {
	// CallFields embeds the input and output
	// symbols for the Routine calling interface.
	CallFields

	// Name represents the name of the routine
	Name string
	// Ptr represents the pointer reference of the routine
	Ptr engineio.ElementPtr
	// Kind represents the kind of routine callsite
	Kind engineio.CallsiteKind

	// Instructs represents the set of logic instructions to
	// execute when the Routine is invoked/called.
	Instructs Instructions
	// catches represents the exception catch table specifying the
	// exceptions to catch between code points and their handling
	// Catches CatchTable
}

func (routine Routine) exported() bool {
	return isExportedName(routine.Name)
}

func (routine Routine) mutable() bool {
	return isMutableName(routine.Name)
}

func (routine Routine) payable() bool {
	return isPayableName(routine.Name)
}

// name returns the name of the Routine.
// Implements the Runnable interface for Routine & RoutineMethod.
func (routine Routine) name() string { return routine.Name }

// ptr returns the element pointer to the Routine.
// Implements the Runnable interface for Routine & RoutineMethod.
func (routine Routine) ptr() engineio.ElementPtr { return routine.Ptr }

// callfields returns the input/output CallFields of the Routine.
// Implements the Runnable interface for Routine & RoutineMethod.
func (routine Routine) callfields() CallFields { return routine.CallFields }

// run performs the execution of the Routine for the given engine and some input registers.
// Implements the Runnable interface for Routine
func (routine Routine) run(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
	if !engine.callstack.push(&callframe{
		scope: "root",
		label: routine.name(),
		point: uint64(routine.ptr()),
	}) {
		return nil, exception(RuntimeError, "max call depth reached").traced(engine.callstack.trace())
	}

	defer engine.callstack.pop()

	return engine.executeCode(routine.Instructs, inputs)
}

// RoutineMethod represents a method executable of PISA Bytecode
// Implements the Runnable & Method interfaces.
type RoutineMethod struct {
	// CallFields embeds the input and output
	// symbols for the Routine calling interface.
	CallFields

	// Name represents the name of the routine
	Name string
	// Ptr represents the pointer reference of the routine
	Ptr engineio.ElementPtr
	// Code represents the method code of the method
	Code MethodCode
	// Datatype represents the type that the method belongs to.
	Datatype Datatype

	// Instructs represents the set of logic instructions to
	// execute when the Routine is invoked/called.
	Instructs Instructions
	// catches represents the exception catch table specifying the
	// exceptions to catch between code points and their handling
	// Catches CatchTable
}

func (rmethod RoutineMethod) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(rmethod.CallFields); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(rmethod.Name); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(rmethod.Ptr); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(rmethod.Code); err != nil {
		return nil, err
	}

	encoded, err := EncodeDatatype(rmethod.Datatype)
	if err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(rmethod.Instructs); err != nil {
		return nil, err
	}

	if err := polorizer.PolorizeAny(encoded); err != nil {
		return nil, err
	}

	return polorizer, nil
}

func (rmethod *RoutineMethod) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil
	} else if err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&rmethod.CallFields); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&rmethod.Name); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&rmethod.Ptr); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&rmethod.Code); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&rmethod.Instructs); err != nil {
		return err
	}

	wire, err := depolorizer.DepolorizeAny()
	if err != nil {
		return err
	}

	datatype, err := DecodeDatatype(wire)
	if err != nil {
		return err
	}

	rmethod.Datatype = datatype

	return nil
}

// code returns the method code of the RoutineMethod.
// Implements the Method interface for RoutineMethod.
func (rmethod RoutineMethod) code() MethodCode { return rmethod.Code }

// datatype returns the Datatype of the RoutineMethod.
// Implements the Method interface for RoutineMethod.
func (rmethod RoutineMethod) datatype() Datatype { return rmethod.Datatype }

// name returns the Name of the RoutineMethod.
// Implements the Method interface for RoutineMethod.
func (rmethod RoutineMethod) name() string { return rmethod.Name }

// ptr returns the Pointer of the RoutineMethod.
// Implements the Method interface for RoutineMethod.
func (rmethod RoutineMethod) ptr() engineio.ElementPtr { return rmethod.Ptr }

// callfields returns the CallFields of the RoutineMethod.
// Implements the Method interface for RoutineMethod.
func (rmethod RoutineMethod) callfields() CallFields {
	return rmethod.CallFields
}

// run performs the execution of the RoutineMethod for the given engine and some input registers.
// Implements the Runnable interface for RoutineMethod
func (rmethod RoutineMethod) run(engine *Engine, inputs RegisterSet) (RegisterSet, *Exception) {
	if !engine.callstack.push(&callframe{
		scope: rmethod.datatype().String(),
		label: rmethod.name(),
		point: uint64(rmethod.ptr()),
	}) {
		return nil, exception(RuntimeError, "max call depth reached").traced(engine.callstack.trace())
	}

	defer engine.callstack.pop()

	return engine.executeCode(rmethod.Instructs, inputs)
}

// executeCode runs some instructions in the context of the engine with some given inputs.
// Returns the output of executing the code and any exception that occurs.
func (engine *Engine) executeCode(instructions Instructions, inputs RegisterSet) (RegisterSet, *Exception) {
	var (
		// Declare a program counter
		pc = uint64(0)
		// Create a new callscope
		scope = &callscope{
			engine:  engine,
			inputs:  inputs,
			outputs: make(RegisterSet),
			memory:  make(RegisterSet),
		}
	)

ExecutionLoop:
	for pc < instructions.Len() {
		// Get the current instruction
		instruction := instructions[pc]
		// Update the callstack with the current instruction
		engine.callstack.inject(pc, instruction)

		// Lookup the operation function for the instruction
		operation := scope.engine.lookupInstruction(instruction.Op)
		// Execute the operation
		continuity := operation(scope, instruction.Args)
		// Exhaust fuel for operation
		if ok := scope.engine.exhaustFuel(continuity.fuel()); !ok {
			return nil, exception(FuelError, "fuel exhausted").traced(engine.callstack.trace())
		}

		switch continuity.mode() {
		case continueModeOk:
			pc++

		case continueModeTerm:
			break ExecutionLoop

		case continueModeJump:
			jump := continuity.(continueJump) //nolint:forcetypeassert

			// If attempting to jump out of bounds
			if jump.jumpdest >= instructions.Len() {
				// Throw an invalid jump exception
				return nil, exception(RuntimeError, "invalid jump: out of bounds").traced(engine.callstack.trace())
			}

			// If jump destination is invalid
			if instruction = instructions[jump.jumpdest]; instruction.Op != DEST {
				// Throw an invalid jump exception
				return nil, exception(RuntimeError, "invalid jump destination").traced(engine.callstack.trace())
			}

			// Update the program counter
			pc = jump.jumpdest

		case continueModeExcept:
			except := continuity.(continueException) //nolint:forcetypeassert

			// todo: check for exception handler with catch table

			return nil, except.exception
		}
	}

	return scope.outputs, nil
}

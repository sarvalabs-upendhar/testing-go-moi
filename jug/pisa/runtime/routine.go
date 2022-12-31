package runtime

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/pisa/exceptions"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/types"
)

// Routine represents an executable logic procedure
type Routine struct {
	// Name represents the name of the routine
	Name string

	// CallFields contains the input/output symbols
	// injected into the Routine when invoked/called.
	register.CallFields

	// Instructs represents the set of logic instructions to
	// execute when the Routine is invoked/called.
	Instructs Instructions

	// catches represents the exception catch table specifying the
	// exceptions to catch between code points and their handling
	// Catches CatchTable
}

func (routine Routine) Interface() register.CallFields { return routine.CallFields }

func (routine Routine) Execute(scope register.ExecutionScope, inputs register.ValueTable) register.ValueTable {
	// Perform input validation
	if err := routine.Inputs.Validate(inputs); err != nil {
		scope.Throw(exceptions.Exception(exceptions.ExceptionInputValidate, err.Error()))

		return nil
	}

	// This assertion must never fail
	outerScope, ok := scope.(*RoutineScope)
	if !ok {
		panic("non routine scope used for routine execution")
	}

	// Create a child context for the routine and its inputs
	innerScope := outerScope.child(routine, inputs)
	// If an exception was thrown during child context creation, return
	if outerScope.ExceptionThrown() {
		return nil
	}

	// If an exception was thrown during routine execution,
	// throw the exception in the parent context and return
	if innerScope.run(); innerScope.ExceptionThrown() {
		outerScope.Throw(innerScope.except)

		return nil
	}

	// Perform output validation
	if err := routine.Outputs.Validate(innerScope.outputs); err != nil {
		outerScope.Throw(exceptions.Exception(exceptions.ExceptionOutputValidate, err.Error()))

		return nil
	}

	return innerScope.outputs
}

// RoutineTable is a collection of Routine objects.
// The Routines are indexed by both their pointer and name.
type RoutineTable struct {
	Table   map[uint64]*Routine
	Symbols map[string]uint64
}

// NewRoutineTable generates a blank RoutineTable
func NewRoutineTable() RoutineTable {
	return RoutineTable{
		Table:   make(map[uint64]*Routine),
		Symbols: make(map[string]uint64),
	}
}

func (routines RoutineTable) Callsites() map[string]types.LogicCallsite {
	callsites := make(map[string]types.LogicCallsite)

	for name, index := range routines.Symbols {
		if exported(name) {
			callsites[name] = types.LogicCallsite(index)
		}
	}

	return callsites
}

// Get retrieves a Routine from the RoutineTable for a given position.
// Returns nil if there is Routine for that position
func (routines RoutineTable) Get(ptr uint64) (*Routine, bool) {
	routine, exists := routines.Table[ptr]
	// Return the Routine and if it exists in the table
	return routine, exists
}

// Lookup retrieves a Routine from the RoutineTable for a given name.
// Returns nil if there is no Routine for that name.
func (routines RoutineTable) Lookup(name string) *Routine {
	index, exists := routines.Symbols[name]
	if !exists {
		return nil
	}

	return routines.Table[index]
}

func (routines RoutineTable) Size() int {
	return len(routines.Table)
}

func (routines RoutineTable) EjectElements() []*types.LogicElement {
	elements := make([]*types.LogicElement, 0, routines.Size())

	for index, routine := range routines.Table {
		// Polorize the routine
		data, _ := polo.Polorize(routine)
		// Create a LogicElement for the routine and append it
		elements = append(elements, &types.LogicElement{Kind: ElementCodeRoutine, Index: index, Data: data})
	}

	return elements
}

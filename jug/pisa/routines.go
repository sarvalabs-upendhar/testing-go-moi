package pisa

import "github.com/pkg/errors"

// Routine represents an executable logic procedure
type Routine struct {
	// CallFields contains the input/output symbols
	// injected into the Routine when invoked/called.
	CallFields

	// name represents the name of the routine
	Name string
	// instructs represents the set of logic instructions to
	// execute when the Routine is invoked/called.
	Instructs instructions

	// catches represents the exception catch table specifying the
	// exceptions to catch between code points and their handling
	// Catches CatchTable
}

// NewRoutine generates a new Routine object for a given PISA func
// schema and a set of compiled logic instructions for it.
// Returns an error if the function schema contains invalid data.
func NewRoutine(schema RoutineSchema, instructs instructions) (*Routine, error) {
	// Create a new FieldSet from the schema 'loads'
	inputs, err := NewFieldTable(schema.Accepts)
	if err != nil {
		return nil, errors.Wrap(err, "invalid routine inputs")
	}

	// Create a new FieldSet from the schema 'yields'
	outputs, err := NewFieldTable(schema.Returns)
	if err != nil {
		return nil, errors.Wrap(err, "invalid routine output")
	}

	return &Routine{
		Name: schema.Name, Instructs: instructs,
		CallFields: CallFields{inputs, outputs},
	}, nil
}

// Interface returns the CallFields of the Routine representing its input/output symbols.
func (routine *Routine) Interface() CallFields { return routine.CallFields }

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

// lookup retrieves a Routine from the RoutineTable for a given name.
// Returns nil if there is no Routine for that name.
func (routines RoutineTable) lookup(name string) *Routine { //nolint:unused
	index, exists := routines.Symbols[name]
	if !exists {
		return nil
	}

	return routines.Table[index]
}

// fetch retrieves a Routine from the RoutineTable for a given position.
// Returns nil if there is Routine for that position
func (routines RoutineTable) fetch(ptr uint64) (*Routine, bool) {
	routine, exists := routines.Table[ptr]
	// Return the Routine and if it exists in the table
	return routine, exists
}

// insert adds a Routine into the RoutineTable at the given position with a given name.
func (routines *RoutineTable) insert(name string, index uint64, item *Routine) {
	routines.Symbols[name] = index
	routines.Table[index] = item
}

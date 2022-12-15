package pisa

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// compileLogicV1 compiles a manifest bytes of syntax v1 into a LogicDescriptor
func (engine *Engine) compileLogicV1(manifestData []byte) (*types.LogicDescriptor, *ctypes.ExecutionResult) {
	// Decode the manifest into the ManifestSchemaV1 object
	manifest := new(ManifestSchemaV1)
	if err := manifest.FromBytes(manifestData); err != nil {
		return nil, engine.Error(ErrorCodeInvalidManifest, "malformed: does not decode to ManifestSchemaV1")
	}

	// Create a new LogicDescriptor and set the engine ID and manifest hash
	descriptor := new(types.LogicDescriptor)
	descriptor.Engine = types.PISA
	descriptor.Manifest = types.GetHash(manifestData)

	// Compile storage elements if storage is non nil
	if manifest.Storage != nil {
		// Compile the storage layout
		layout, err := NewStorageLayout(manifest.Storage.Fields)
		if err != nil {
			return nil, engine.Error(ErrorCodeStorageCompile, fmt.Sprintf("storage layout invalid: %v", err))
		}

		// Compile the storage builder if it exists
		if err = engine.compileBuilder(manifest.Storage.Builder); err != nil {
			return nil, engine.Error(ErrorCodeStorageCompile, fmt.Sprintf("storage builder invalid: %v", err))
		}

		// Insert the compiled storage layout
		engine.storage.layout = &layout
		// Set descriptor as stateful
		descriptor.Stateful = true
	}

	// Compile the constants
	var err error
	if err = engine.compileConstants(manifest.Constants); err != nil {
		return nil, engine.Error(ErrorCodeConstantCompile, err.Error())
	}

	// Compile the typedefs
	if err = engine.compileTypedefs(manifest.Typedefs); err != nil {
		return nil, engine.Error(ErrorCodeTypedefCompile, err.Error())
	}

	var callsites map[string]types.LogicCallsite
	// Compile the routines
	if callsites, err = engine.compileRoutines(manifest.Routines); err != nil {
		return nil, engine.Error(ErrorCodeRoutineCompile, err.Error())
	}

	// todo: compile events
	// todo: compile classes

	// Set the descriptor callsites
	descriptor.Callsites = callsites
	// Compile the engine into LogicElements
	descriptor.Elements, err = engine.compile()
	if err != nil {
		return nil, engine.Error(ErrorCodeBindingCompile, err.Error())
	}

	// Exhaust some fuel for the compilation
	if !engine.fueltank.Exhaust(50) {
		return nil, engine.Error(ErrorCodeRanOutOfFuel, "")
	}

	// Return the LogicDescriptor
	return descriptor, engine.Result(nil)
}

// compileConstants compiles and inserts Constant values into the Engine
// from a given map of constant expressions indexed by their uint64 addresses.
// Returns an error if a constant expression is invalid (not parseable with ParseConstant)
func (engine *Engine) compileConstants(table map[uint64]string) error {
	for ptr, c := range table {
		constant, err := ParseConstant(c)
		if err != nil {
			return errors.Wrapf(err, "invalid constant expression [%x]", ptr)
		}

		engine.constants.insert(ptr, constant)
	}

	return nil
}

// compileTypedefs compiles and inserts symbolic typedefs into the Engine
// from a given map of type expressions indexed by their uint64 addresses.
// Returns an error if a type expression is invalid (not parseable with ParseDatatype)
func (engine *Engine) compileTypedefs(table map[uint64]string) error {
	for ptr, t := range table {
		datatype, err := ParseDatatype(t)
		if err != nil {
			return errors.Wrapf(err, "invalid type expression [%x]", ptr)
		}

		engine.datatypes.insertSymbolic(ptr, datatype)
	}

	return nil
}

// compileRoutines compiles and inserts Routine value into the Routine table
// for a given map of routine schemas indexed by their uint64 addresses.
// Returns an error if a routine schema is invalid.
func (engine *Engine) compileRoutines(table map[uint64]RoutineSchema) (map[string]types.LogicCallsite, error) {
	callsites := make(map[string]types.LogicCallsite)

	for index, schema := range table {
		name := schema.Name
		if name == "" {
			return nil, errors.Errorf("cannot compile routine [%v]: missing name", index)
		}

		// Compile the instructions for the routine logic
		instructs, err := engine.compileInstructions(schema.Executes)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot compile routine [%v]", index)
		}

		// Create a routine for the compiled instructions
		routine, err := NewRoutine(schema, instructs)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot compile routine [%v]", index)
		}

		// Insert the compiled routine into the compiler table
		engine.routines.insert(name, index, routine)
		// Mark as a callsite if the routine name is exported
		if exported(name) {
			callsites[name] = types.LogicCallsite(index)
		}
	}

	return callsites, nil
}

func (engine *Engine) compileBuilder(schema RoutineSchema) error {
	// Compile the instructions for the routine logic
	instructs, err := engine.compileInstructions(schema.Executes)
	if err != nil {
		return errors.Wrap(err, "cannot compile builder")
	}

	// Create a routine from the compiled instructions
	routine, err := NewRoutine(schema, instructs)
	if err != nil {
		return errors.Wrap(err, "cannot compile builder")
	}

	// Insert the compiled builder into the compiler table
	engine.storage.builder = routine

	return nil
}

// compileInstructions compiles a set of PISA Instructions from a given Bytecode object.
// Returns an error if the Bytecode is not properly structured or contains invalid opcodes.
func (engine *Engine) compileInstructions(bytecode Bytecode) (instructions, error) {
	if bytecode.Bytecode == nil {
		return nil, errors.New("cannot compile non bytecode instructions")
	}

	reader := bytes.NewReader(bytecode.Bytecode)
	instructs := make(instructions, 0)

	for line := 1; reader.Len() != 0; line++ {
		// Read an opcode byte
		opcode, _ := reader.ReadByte()
		// Lookup the instructions for the opcode
		op := engine.instructs.lookup(OpCode(opcode))
		if op == nil {
			return nil, errors.Errorf("cannot compile instructions: invalid opcode '%v' [line: %v]", opcode, line)
		}

		// Read the operands for the opcode
		operands, ok := op.operand(reader)
		if !ok {
			return nil, errors.Errorf("cannot compile instructions: insufficient operands for '%v' [line: %v]", opcode, line)
		}

		// Append instruction into the program code
		instructs = append(instructs, instruction{Op: OpCode(opcode), Args: operands})
	}

	return instructs, nil
}

const (
	elementStorage  = "storage"
	elementConstant = "constant"
	elementTypedef  = "typedef"
	elementRoutine  = "routine"

	elementEvent = "event" //nolint:unused
	elementClass = "class" //nolint:unused
)

// compile performs checks to ensure all LogicElement bindings are valid
// and returns a slice of LogicElements with all compiled elements.
// TODO: this does perform the bindings checks or generate bound elements yet!
func (engine *Engine) compile() ([]*types.LogicElement, error) {
	// Approximate size the number of logic elements in the engine
	size := 1 + len(engine.datatypes.symbolic) + len(engine.constants) + len(engine.routines.Table)
	// Create a slice of LogicElements with enough capacity for the expected number of elements
	elements := make([]*types.LogicElement, 0, size)

	// Polorize the storage layout
	storage, _ := polo.Polorize(engine.storage.layout)
	// Create a LogicElement for the storage layout and append it
	elements = append(elements, &types.LogicElement{Kind: elementStorage, Index: 0, Data: storage})

	// Polorize the storage builder
	builder, _ := polo.Polorize(engine.storage.builder)
	// Create a LogicElement for the storage builder and append it
	elements = append(elements, &types.LogicElement{Kind: elementStorage, Index: 1, Data: builder})

	for index, typedef := range engine.datatypes.symbolic {
		// Polorize the symbolic typedef
		data, _ := polo.Polorize(typedef)
		// Create a LogicElement for the symbolic typedef and append it
		elements = append(elements, &types.LogicElement{Kind: elementTypedef, Index: index, Data: data})
	}

	// todo: generate LogicElements for classes and events

	for index, constant := range engine.constants {
		// Polorize the constant
		data, _ := polo.Polorize(constant)
		// Create a LogicElement for the constant and append it
		elements = append(elements, &types.LogicElement{Kind: elementConstant, Index: index, Data: data})
	}

	for index, routine := range engine.routines.Table {
		// Polorize the routine
		data, _ := polo.Polorize(routine)
		// Create a LogicElement for the routine and append it
		elements = append(elements, &types.LogicElement{Kind: elementRoutine, Index: index, Data: data})
	}

	return elements, nil
}

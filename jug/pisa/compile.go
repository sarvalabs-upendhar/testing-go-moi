package pisa

import (
	"bytes"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/pisa/exceptions"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/jug/pisa/runtime"
	ctypes "github.com/sarvalabs/moichain/jug/types"
	"github.com/sarvalabs/moichain/types"
)

// compileLogicV1 compiles a manifest bytes of syntax v1 into a LogicDescriptor
func (engine *Engine) compileLogicV1(manifestData []byte) (*types.LogicDescriptor, *ctypes.ExecutionResult) {
	// Decode the manifest into the ManifestSchemaV1 object
	manifest := new(ManifestSchemaV1)
	if err := manifest.FromBytes(manifestData); err != nil {
		return nil, engine.Error(exceptions.ExceptionInvalidManifest, "malformed: does not decode to ManifestSchemaV1")
	}

	// Create a new LogicDescriptor and set the engine ID and manifest hash
	descriptor := new(types.LogicDescriptor)
	descriptor.Engine = types.PISA
	descriptor.Manifest = types.GetHash(manifestData)

	// Compile storage elements if storage is non nil
	if manifest.Storage != nil {
		// Compile the storage layout
		if err := engine.compileStorageLayout(manifest.Storage.Fields); err != nil {
			return nil, engine.Errorf(exceptions.ExceptionElementCompile, "storage layout compile: %v", err)
		}

		// Compile the storage builder if non-nil
		if manifest.Storage.Builder != nil {
			if err := engine.compileStorageBuilder(*manifest.Storage.Builder); err != nil {
				return nil, engine.Errorf(exceptions.ExceptionElementCompile, "storage builder compile: %v", err)
			}
		}

		// Set descriptor as stateful
		descriptor.PersistentState = true
	}

	var err error

	// Compile the constants
	if err = engine.compileConstants(manifest.Constants); err != nil {
		return nil, engine.Errorf(exceptions.ExceptionElementCompile, "constant compile: %v", err)
	}

	// Compile the typedefs
	if err = engine.compileTypedefs(manifest.Typedefs); err != nil {
		return nil, engine.Errorf(exceptions.ExceptionElementCompile, "typedef compile: %v", err)
	}

	// Compile the routines
	if err = engine.compileRoutines(manifest.Routines); err != nil {
		return nil, engine.Errorf(exceptions.ExceptionElementCompile, "routine compile: %v", err)
	}

	// todo: compile events
	// todo: compile classes

	// Set the descriptor callsites
	descriptor.Callsites = engine.routines.Callsites()
	// Compile the engine into LogicElements
	descriptor.Elements, err = engine.compile()
	if err != nil {
		return nil, engine.Error(exceptions.ExceptionBindingCompile, err.Error())
	}

	// Exhaust some fuel for the compilation
	if !engine.fueltank.Exhaust(50) {
		return nil, engine.Error(exceptions.ExceptionFuelExhausted, "")
	}

	// Return the LogicDescriptor
	return descriptor, engine.Result(nil)
}

// compileConstants compiles and inserts Constant values into the Engine
// from a given map of constant expressions indexed by their uint64 addresses.
// Returns an error if a constant expression is invalid (not parseable with ParseConstant)
func (engine *Engine) compileConstants(table map[uint64]string) error {
	constants := make(runtime.ConstantTable)

	for ptr, c := range table {
		constant, err := runtime.ParseConstant(c)
		if err != nil {
			return errors.Wrapf(err, "invalid constant expression [%x]", ptr)
		}

		constants[ptr] = constant
	}

	engine.constants = constants

	return nil
}

// compileTypedefs compiles and inserts symbolic typedefs into the Engine
// from a given map of type expressions indexed by their uint64 addresses.
// Returns an error if a type expression is invalid (not parseable with ParseDatatype)
func (engine *Engine) compileTypedefs(table map[uint64]string) error {
	for ptr, t := range table {
		datatype, err := register.ParseDatatype(t)
		if err != nil {
			return errors.Wrapf(err, "invalid type expression [%x]", ptr)
		}

		engine.datatypes.SetSymbolic(ptr, datatype)
	}

	return nil
}

// compileRoutines compiles and inserts Routine value into the Routine table
// for a given map of routine schemas indexed by their uint64 addresses.
// Returns an error if a routine schema is invalid.
func (engine *Engine) compileRoutines(table map[uint64]RoutineSchema) error {
	for index, schema := range table {
		name := schema.Name
		if name == "" {
			return errors.Errorf("cannot compile routine [%#v]: missing name", index)
		}

		// Compile the instructions for the routine logic
		instructs, err := engine.compileInstructions(schema.Executes)
		if err != nil {
			return errors.Wrapf(err, "cannot compile routine [%#v]: invalid instructions", index)
		}

		// Create a new FieldSet from the schema 'accepts'
		inputs, err := register.NewFieldTable(schema.Accepts)
		if err != nil {
			return errors.Wrap(err, "invalid routine inputs")
		}

		// Create a new FieldSet from the schema 'returns'
		outputs, err := register.NewFieldTable(schema.Returns)
		if err != nil {
			return errors.Wrap(err, "invalid routine output")
		}

		// Create a routine for the compiled instructions
		routine := &runtime.Routine{
			Name: name, Instructs: instructs,
			CallFields: register.CallFields{Inputs: inputs, Outputs: outputs},
		}

		engine.routines.Symbols[name] = index
		engine.routines.Table[index] = routine
	}

	return nil
}

func (engine *Engine) compileStorageLayout(table map[uint8]string) error {
	// Create a new FieldSet from the map set
	layout, err := register.NewFieldTable(table)
	if err != nil {
		return err
	}

	engine.storage.Layout = &layout

	return nil
}

func (engine *Engine) compileStorageBuilder(schema RoutineSchema) error {
	// Compile the instructions for the routine logic
	instructs, err := engine.compileInstructions(schema.Executes)
	if err != nil {
		return errors.Wrap(err, "cannot compile builder: invalid instructions")
	}

	// Create a new FieldSet from the schema 'accepts'
	inputs, err := register.NewFieldTable(schema.Accepts)
	if err != nil {
		return errors.Wrap(err, "invalid builder inputs")
	}

	// Create a new FieldSet from the schema 'returns'
	outputs, err := register.NewFieldTable(schema.Returns)
	if err != nil {
		return errors.Wrap(err, "invalid builder output")
	}

	// Create a routine from the compiled instructions
	routine := &runtime.Routine{
		Name: "", Instructs: instructs,
		CallFields: register.CallFields{Inputs: inputs, Outputs: outputs},
	}

	// Insert the compiled builder into the compiler table
	engine.storage.Builder = routine

	return nil
}

// compileInstructions compiles a set of PISA Instructions from a given Bytecode object.
// Returns an error if the Bytecode is not properly structured or contains invalid opcodes.
func (engine *Engine) compileInstructions(bytecode Bytecode) (runtime.Instructions, error) {
	if bytecode.Bytecode == nil {
		return nil, errors.New("cannot compile non bytecode instructions")
	}

	reader := bytes.NewReader(bytecode.Bytecode)
	instructs := make(runtime.Instructions, 0)

	for line := 1; reader.Len() != 0; line++ {
		// Read an opcode byte
		opcode, _ := reader.ReadByte()
		// Lookup the instructions for the opcode
		op := engine.GetInstruction(runtime.OpCode(opcode))
		if op == nil {
			return nil, errors.Errorf("invalid opcode '%#v' [line %v]", opcode, line)
		}

		// Read the operands for the opcode
		operands, ok := op.Operand(reader)
		if !ok {
			return nil, errors.Errorf("insufficient operands for '%#v' [line %v]", opcode, line)
		}

		// Append instruction into the program code
		instructs = append(instructs, runtime.Instruction{Op: runtime.OpCode(opcode), Args: operands})
	}

	return instructs, nil
}

// compile performs checks to ensure all LogicElement bindings are valid
// and returns a slice of LogicElements with all compiled elements.
// TODO: this does perform the bindings checks or generate bound elements yet!
func (engine *Engine) compile() ([]*types.LogicElement, error) {
	// Approximate size the number of logic elements in the engine
	size := 2 + engine.datatypes.Size() + engine.constants.Size() + engine.routines.Size()
	// Create a slice of LogicElements with enough capacity for the expected number of elements
	elements := make([]*types.LogicElement, 0, size)

	// Eject the storage elements (builder & layout)
	elements = append(elements, engine.storage.EjectElements()...)

	// Eject type elements (typedefs, classes, events, etc)
	elements = append(elements, engine.datatypes.EjectElements()...)

	// Eject routine elements
	elements = append(elements, engine.routines.EjectElements()...)

	// Eject constant elements
	elements = append(elements, engine.constants.EjectElements()...)

	return elements, nil
}

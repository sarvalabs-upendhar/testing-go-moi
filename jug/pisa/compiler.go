package pisa

import (
	"bytes"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/exp/constraints"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/register"
)

// ManifestCompiler is a compiler implementation for PISA that converts
// an engineio.Manifest object into an engineio.LogicDescriptor which can
// in turn be used to construct an engineio.LogicDriver implementation.
type ManifestCompiler struct {
	fueltank  *engineio.FuelTank
	instructs InstructionSet
	manifest  *engineio.Manifest

	elements map[engineio.ElementPtr]engineio.ManifestElement
	kindmap  map[engineio.ElementPtr]engineio.ElementKind
	depgraph *engineio.DependencyGraph

	callsites map[string]*engineio.Callsite
	compiled  map[engineio.ElementPtr]*engineio.LogicElement

	state engineio.ContextStateMatrix
}

// newManifestCompiler generates a new ManifestCompiler for some given
// fuel capacity, engineio.Manifest and a PISA InstructionSet object.
// The compiler is seeded with all detected elements from the Manifest.
func newManifestCompiler(
	fuel engineio.Fuel,
	manifest *engineio.Manifest,
	instructs InstructionSet,
) (
	*ManifestCompiler, error,
) {
	compiler := &ManifestCompiler{
		instructs: instructs,
		manifest:  manifest,
		fueltank:  engineio.NewFuelTank(fuel),
		depgraph:  engineio.NewDependencyGraph(),
		state:     make(engineio.ContextStateMatrix, 2),
		kindmap:   make(map[engineio.ElementPtr]engineio.ElementKind, len(manifest.Elements)),
		elements:  make(map[engineio.ElementPtr]engineio.ManifestElement, len(manifest.Elements)),
	}

	// Create a map that can track ptr duplicates and positional gaps
	ptrs := make(map[engineio.ElementPtr]struct{}, len(manifest.Elements))
	// Iterate over the manifest elements
	for _, element := range manifest.Elements {
		// If the ptr has already been encountered, return error
		if _, exists := ptrs[element.Ptr]; exists {
			return nil, errors.Errorf("invalid manifest: duplicate element pointer %x", element.Ptr)
		}

		// Insert the ptr into the ptr tracker
		ptrs[element.Ptr] = struct{}{}

		// Add the element to the element set, depgraph and kindmap
		compiler.elements[element.Ptr] = element
		compiler.depgraph.Insert(element.Ptr, element.Deps...)
		compiler.kindmap[element.Ptr] = element.Kind
	}

	// Check if there are gaps in the elements pointers
	if hasGaps(ptrs) {
		return nil, errors.New("invalid manifest: element pointer gaps detected")
	}

	return compiler, nil
}

// compile generates an engineio.LogicDescriptor from the ManifestCompiler
// by compiling each ManifestElement into a supported LogicElement and
// resolving dependencies and performing other verifications on the Logic.
func (compiler *ManifestCompiler) compile() (*engineio.LogicDescriptor, error) {
	// Resolve the dependency graph into a sequence of element pointers
	// representing the order in which the elements must be compiled
	order, ok := compiler.depgraph.Resolve()
	if !ok {
		return nil, errors.New("invalid manifest: circular/empty dependency detected")
	}

	// Exhaust some fuel for the dependency resolution
	if !compiler.fueltank.Exhaust(50) {
		return nil, errors.New("insufficient fuel for manifest compile")
	}

	// Set up the compiler accumulators
	compiler.compiled = make(map[engineio.ElementPtr]*engineio.LogicElement, len(compiler.elements))
	compiler.callsites = make(map[string]*engineio.Callsite)

	// Iterate over the element pointers in compile order
	for _, ptr := range order {
		switch kind := compiler.kindmap[ptr]; kind {
		case ConstantElement:
			// Compile the element as a ConstantElement
			if err := compiler.compileConstantElement(ptr); err != nil {
				return nil, errors.Wrapf(err, "constant element [%#v] compile failed", ptr)
			}

		case TypedefElement:
			// Compile the element as a TypedefElement
			if err := compiler.compileTypedefElement(ptr); err != nil {
				return nil, errors.Wrapf(err, "typedef element [%#v] compile failed", ptr)
			}

		case StateElement:
			// Compile the element as a StateElement
			if err := compiler.compileStateElement(ptr); err != nil {
				return nil, errors.Wrapf(err, "state element [%#v] compile failed", ptr)
			}

		case RoutineElement:
			// Compile the element as a RoutineElement
			if err := compiler.compileRoutineElement(ptr); err != nil {
				return nil, errors.Wrapf(err, "routine element [%#v] compile failed", ptr)
			}

		default:
			return nil, errors.Errorf("invalid element kind [%#v]: %v", ptr, kind)
		}
	}

	// Exhaust some fuel for the compilation resolution
	if !compiler.fueltank.Exhaust(50) {
		return nil, errors.New("insufficient fuel for manifest compile")
	}

	// Generate the hash of the manifest
	manifestHash, _ := compiler.manifest.Hash()
	// Generate a LogicDescriptor from the compiler data
	return &engineio.LogicDescriptor{
		Manifest:    manifestHash,
		Engine:      engineio.PISA,
		Interactive: false,

		StateMatrix: compiler.state,
		DepGraph:    compiler.depgraph,
		Elements:    compiler.compiled,
		Callsites:   compiler.callsites,
	}, nil
}

// compileConstantElement compiles an engineio.ManifestElement object of type ConstantElement
func (compiler *ManifestCompiler) compileConstantElement(ptr engineio.ElementPtr) error {
	// Get the element from the compiler
	element := compiler.elements[ptr]

	// Convert element into a ConstantSchema
	constantSchema, ok := element.Data.(*ConstantSchema)
	if !ok {
		return errors.New("invalid element data for 'constant' kind")
	}

	// Create a new Constant object
	constant, err := compileConstant(constantSchema)
	if err != nil {
		return errors.Wrap(err, "invalid constant element")
	}

	// Check dependencies (must not have any)
	if len(element.Deps) != 0 {
		return errors.New("invalid constant element: cannot have dependencies")
	}

	// Generate the compiled element and store it
	encoded, _ := polo.Polorize(constant)
	compiler.compiled[ptr] = &engineio.LogicElement{
		Kind: ConstantElement,
		Data: encoded,
		Deps: element.Deps,
	}

	return nil
}

// compileTypedefElement compiles an engineio.ManifestElement object of type TypedefElement
func (compiler *ManifestCompiler) compileTypedefElement(ptr engineio.ElementPtr) error {
	// Get the element from the compiler
	element := compiler.elements[ptr]

	// Convert element into a TypedefSchema
	typedefSchema, ok := element.Data.(*TypedefSchema)
	if !ok {
		return errors.New("invalid element data for 'typedef' kind")
	}

	// Parse the type expression into a Typedef
	datatype, err := engineio.ParseDatatype(string(*typedefSchema))
	if err != nil {
		return errors.Wrap(err, "invalid typedef element: invalid type expression")
	}

	// Check the dependencies for the typedef (must only be a ClassElement)
	for _, dep := range element.Deps {
		if kind := compiler.kindmap[dep]; kind != ClassElement {
			return errors.Errorf("invalid typedef element: cannot have a '%v' as dependency", kind)
		}
	}

	// Generate the compiled element and store it
	encoded, _ := polo.Polorize(datatype)
	compiler.compiled[ptr] = &engineio.LogicElement{
		Kind: TypedefElement,
		Data: encoded,
		Deps: element.Deps,
	}

	return nil
}

// compileStateElement compiles an engineio.ManifestElement object of type StateElement
func (compiler *ManifestCompiler) compileStateElement(ptr engineio.ElementPtr) error {
	// Get the element from the compiler
	element := compiler.elements[ptr]

	// Convert element into a StateSchema
	stateSchema, ok := element.Data.(*StateSchema)
	if !ok {
		return errors.New("invalid element data for 'state' kind")
	}

	// Check the dependencies for the storage class (must only be a ClassElement)
	for _, dep := range element.Deps {
		if kind := compiler.kindmap[dep]; kind != ClassElement {
			return errors.Errorf("invalid state element: cannot have '%v' as dependency", kind)
		}
	}

	// Check intended scope for StateElement
	switch stateSchema.Kind {
	case engineio.PersistentState:
		// Check if a persistent storage class has already been compiled
		if compiler.state.Persistent() {
			return errors.New("invalid state element: duplicate persistent state")
		}

		// Create a new FieldSet from the map set
		layout, err := compileTypeFields(stateSchema.Fields)
		if err != nil {
			return errors.Errorf("invalid state element: invalid fields: %v", err)
		}

		// Set the persistent state pointer in the compiler
		compiler.state[engineio.PersistentState] = ptr

		// Generate the compiled element and store it
		encoded, _ := polo.Polorize(layout)
		compiler.compiled[ptr] = &engineio.LogicElement{
			Kind: StateElement,
			Data: encoded,
			Deps: element.Deps,
		}

		return nil

	case engineio.EphemeralState:
		return errors.New("ephemeral state elements are not supported")
	default:
		return errors.Errorf("invalid kind '%v' for state element", stateSchema.Kind)
	}
}

// compileRoutineElement compiles an engineio.ManifestElement object of type RoutineElement
func (compiler *ManifestCompiler) compileRoutineElement(ptr engineio.ElementPtr) error {
	// Get the element from the compiler
	element := compiler.elements[ptr]

	// Convert element into a RoutineSchema
	routineSchema, ok := element.Data.(*RoutineSchema)
	if !ok {
		return errors.New("invalid element data for 'routine' kind")
	}

	// Check type of RoutineElement
	switch routineSchema.Kind {
	case engineio.InvokableCallsite:
		// Supported with no checks (all dependencies supported)

	case engineio.DeployerCallsite:
		if !IsExportedName(routineSchema.Name) || !IsMutableName(routineSchema.Name) {
			return errors.Errorf("invalid routine element: invalid name '%v' for deployer routine", routineSchema.Name)
		}

		// Check if the persistent state has been compiled
		if !compiler.state.Persistent() {
			return errors.New("invalid routine element: deployer routine for non-existent persistent storage")
		}

		var stateDepFound bool

		// All deps are allowed, but we check that the storage class dependency is consistent
		for _, dep := range element.Deps {
			if kind := compiler.kindmap[dep]; kind == StateElement {
				// If stateDepFound is already true, then error (cannot have dependency on multiple state types)
				if stateDepFound {
					return errors.New("invalid routine element: dependency on multiple state elements")
				}

				// Check that dependency pointer matches persistent state
				if compiler.state[engineio.PersistentState] != dep {
					return errors.New("invalid routine element: invalid state element dep")
				}

				// Set the stateDepFound to true, this prevents the same
				// deployer having a dependency on the ephemeral state
				stateDepFound = true
			}
		}

		if !stateDepFound {
			return errors.New("invalid routine element: missing state dependency for deployer routine")
		}

	case engineio.InteractableCallsite:
		return errors.New("interactable routine elements are not supported")
	case engineio.EnlisterCallsite:
		return errors.New("enlister routine elements are not supported")
	default:
		return errors.Errorf("invalid kind '%v' for routine element", routineSchema.Kind)
	}

	// Create a Routine object
	routine, err := compileRoutine(routineSchema, compiler.instructs)
	if err != nil {
		return errors.Wrapf(err, "invalid routine element")
	}

	if routine.Exported() {
		compiler.callsites[routine.Name] = &engineio.Callsite{Ptr: ptr, Kind: routine.Kind}
	}

	// Generate the compiled element
	encoded, _ := polo.Polorize(routine)
	compiler.compiled[ptr] = &engineio.LogicElement{
		Kind: RoutineElement,
		Data: encoded,
		Deps: element.Deps,
	}

	return nil
}

// compileRoutine compiles a RoutineSchema object into a runtime.Routine.
func compileRoutine(schema *RoutineSchema, instructSet InstructionSet) (*Routine, error) {
	// Create a new FieldSet from the schema 'accepts'
	inputs, err := compileTypeFields(schema.Accepts)
	if err != nil {
		return nil, errors.Wrap(err, "invalid accept fields")
	}

	// Create a new FieldSet from the schema 'returns'
	outputs, err := compileTypeFields(schema.Returns)
	if err != nil {
		return nil, errors.Wrap(err, "invalid return fields")
	}

	// Compile the instructions for the routine logic
	instructions, err := compileInstructions(schema.Executes, instructSet)
	if err != nil {
		return nil, errors.Wrap(err, "invalid instructions")
	}

	// Create a routine for the compiled instructions
	return &Routine{
		Name:      schema.Name,
		Kind:      schema.Kind,
		Instructs: instructions,
		CallFields: engineio.CallFields{
			Inputs:  inputs,
			Outputs: outputs,
		},
	}, nil
}

// compileInstructions compiles an InstructionsSchema into some runtime.Instructions.
func compileInstructions(schema InstructionsSchema, instructSet InstructionSet) (Instructions, error) {
	switch {
	case schema.Bin != nil:
		return compileBinInstructions(schema.Bin, instructSet)
	case schema.Hex != "":
		return compileHexInstructions(schema.Hex, instructSet)
	case schema.Asm != nil:
		return compileAsmInstructions(schema.Asm, instructSet)
	default:
		return nil, errors.New("no instructions found")
	}
}

// compileBinInstructions compiles an InstructionsSchema with binary instructions into runtime.Instructions.
func compileBinInstructions(instructions []byte, instructSet InstructionSet) (Instructions, error) {
	reader := bytes.NewReader(instructions)
	instructs := make(Instructions, 0)

	for line := 1; reader.Len() != 0; line++ {
		// Read an opcode byte
		opcode, _ := reader.ReadByte()
		// Lookup the instructions for the opcode
		op := instructSet[opcode]
		if op == nil {
			return nil, errors.Errorf("invalid opcode '%#v' [line %v]", opcode, line)
		}

		// Read the operands for the opcode
		operands, ok := op.Operand(reader)
		if !ok {
			return nil, errors.Errorf("insufficient operands for '%#v' [line %v]", opcode, line)
		}

		// Append instruction into the program code
		instructs = append(instructs, Instruction{Op: OpCode(opcode), Args: operands})
	}

	return instructs, nil
}

// compileHexInstructions compiles an InstructionsSchema with hexadecimal instructions into runtime.Instructions.
func compileHexInstructions(instructions string, instructSet InstructionSet) (Instructions, error) {
	// Remove the 0x prefix if it exists
	instructions = strings.TrimPrefix(instructions, "0x")

	decodedInstructs, err := hex.DecodeString(instructions)
	if err != nil {
		return nil, errors.Wrap(err, "invalid hex instructions")
	}

	return compileBinInstructions(decodedInstructs, instructSet)
}

// compileAsmInstructions compiles an InstructionsSchema with assembly instructions into runtime.Instructions.
func compileAsmInstructions(_ []string, _ InstructionSet) (Instructions, error) {
	return nil, errors.New("cannot compile assembly instructions (yet!)")
}

// compileConstant compiles a ConstantSchema object into a register.Constant.
func compileConstant(schema *ConstantSchema) (*register.Constant, error) {
	// Parse the token literal into a Typedef
	datatype, err := engineio.ParseDatatype(schema.Type)
	if err != nil {
		return nil, errors.Wrap(err, "invalid constant datatype")
	}

	// Confirm that the type is scalar
	if datatype.Kind != engineio.PrimitiveType {
		return nil, errors.New("constant datatype is not scalar")
	}

	data := schema.Value
	// Remove the 0x prefix if it exists
	data = strings.TrimPrefix(data, "0x")

	// Decode value hex string into bytes
	vdata, err := hex.DecodeString(data)
	if err != nil {
		return nil, errors.Wrap(err, "invalid constant value: invalid hexadecimal")
	}

	// Create a register value for the datatype and data
	value, err := register.NewValue(datatype, vdata)
	if err != nil {
		return nil, errors.Wrap(err, "invalid constant value: invalid data for type")
	}

	// Create a constant and return it
	return &register.Constant{Type: datatype.Prim, Data: value.Data()}, nil
}

// compileTypeFields compiles a map of TypefieldSchema objects into an engineio.TypeFields.
// Returns an error if the given map of field expressions contains positional gaps or invalid expressions.
func compileTypeFields(table []TypefieldSchema) (*engineio.TypeFields, error) {
	// Create a blank field table
	fields := &engineio.TypeFields{
		Table:   make(map[uint8]*engineio.TypeField, len(table)),
		Symbols: make(map[string]uint8, len(table)),
	}

	// Error if there are more than 2^8 slots
	if len(table) > 256 {
		return nil, errors.New("invalid field set: too many typefield schema (max 256)")
	}

	// Create a map that can track slot duplicates and positional gaps
	slots := make(map[uint8]struct{}, len(table))
	// Iterate over the type fields in the table
	for _, typefield := range table {
		// If the slot has already been encountered, return error
		if _, exists := slots[typefield.Slot]; exists {
			return nil, errors.Errorf("invalid typefield for slot '%v': duplicate type field", typefield.Slot)
		}

		// Insert the slot into the slot tracker
		slots[typefield.Slot] = struct{}{}

		// Compile the TypefieldSchema into a register.TypeField
		compiled, err := compileTypefield(typefield)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid typefield for slot '%v'", typefield.Slot)
		}

		// Insert the typefield into the FieldTable
		fields.Insert(typefield.Slot, compiled)
	}

	// Check if the slot tracker has gaps
	if hasGaps(slots) {
		return nil, errors.New("invalid field set: slot gaps detected")
	}

	return fields, nil
}

// compileTypefield compiles a TypefieldSchema into a engineio.TypeField.
func compileTypefield(schema TypefieldSchema) (*engineio.TypeField, error) {
	// Parse the enclosed data into a datatype
	dt, err := engineio.ParseDatatype(schema.Type)
	if err != nil {
		return nil, errors.Wrap(err, "invalid type field type data")
	}

	// Create a Symbol with the name and type data
	return &engineio.TypeField{Name: schema.Label, Type: dt}, nil
}

// hasGaps returns if the keys of a map of unsigned numbers has gaps.
// They must also start from 0 and go up to len-1
func hasGaps[U constraints.Unsigned](indices map[U]struct{}) bool {
	for i := U(0); i < U(len(indices)); i++ {
		if _, exists := indices[i]; !exists {
			return true
		}
	}

	return false
}

package pisa

import (
	"bytes"
	"encoding/hex"
	"strings"

	"github.com/manishmeganathan/depgraph"
	"github.com/pkg/errors"
	"golang.org/x/exp/constraints"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/datatypes"
	"github.com/sarvalabs/go-pisa/logic"
	"github.com/sarvalabs/go-pisa/opcode"
	"github.com/sarvalabs/go-pisa/state"
)

const (
	// BaseCompileCost is the base cost for compiling a Manifest (always applied)
	BaseCompileCost engineio.EngineFuel = 50
	// DepsResolveCost is the cost to resolve all the element dependencies in a Manifest (always applied)
	DepsResolveCost engineio.EngineFuel = 50

	// TypeExprParseCost is the cost to parse a type expression into a Datatype.
	// It is applied for each TypedefElement and per each field in ClassElement and StateElement
	TypeExprParseCost engineio.EngineFuel = 10
	// BaseClassCost is the base cost for compiling a ClassElement
	BaseClassCost engineio.EngineFuel = 10
	// BaseStateCost is the base cost for compiling a StateElement
	BaseStateCost engineio.EngineFuel = 20
	// BaseEventCost is the base cost for compiling an EventElement
	BaseEventCost engineio.EngineFuel = 15

	// ConstantCompileCost is the cost to compile a ConstantElement
	ConstantCompileCost engineio.EngineFuel = 20
	// TypedefCompileCost is the cost to compile a TypedefElement
	TypedefCompileCost engineio.EngineFuel = TypeExprParseCost + 5
	// ClassMethodCost is the base cost for compiling a MethodElement
	// The TypeExprParseCost is applied on top of this per field in the accepts/returns of the MethodElement
	ClassMethodCost engineio.EngineFuel = 15

	// BaseRoutineCost is the base cost for compiling a RoutineElement
	// The TypeExprParseCost is applied on top of this per field in the accepts/returns of the RoutineElement
	BaseRoutineCost engineio.EngineFuel = 10
	// InvokableRoutineSurcharge is the additional cost for compiling an Invokable RoutineElement.
	// This surcharge is applied on top of the BaseRoutineCost
	InvokableRoutineSurcharge engineio.EngineFuel = 5
	// DeployerRoutineSurcharge is the additional cost for compiling a Deployer RoutineElement.
	// This surcharge is applied on top of the BaseRoutineCost
	DeployerRoutineSurcharge engineio.EngineFuel = 10
	// EnlisterRoutineSurcharge is the additional cost for compiling a Enlister RoutineElement.
	// This surcharge is applied on top of the BaseRoutineCost
	EnlisterRoutineSurcharge engineio.EngineFuel = 10

	// BaseInstructionCost is the base cost for compiling a BIN instruction.
	// It is applied per instruction in a RoutineElement or MethodElement
	BaseInstructionCost engineio.EngineFuel = 5
	// HexInstructionSurcharge is the additional cost for compiling HEX instructions.
	// It is applied on top of the BaseInstructionCost per instruction
	HexInstructionSurcharge engineio.EngineFuel = 2
	// AsmInstructionSurcharge is the additional cost for compiling ASM instructions.
	// It is applied on top of the BaseInstructionCost per instruction
	AsmInstructionSurcharge engineio.EngineFuel = 5
)

var ErrInsufficientCompileFuel = errors.New("insufficient fuel for manifest compile")

// ManifestCompiler is a compiler implementation for PISA that converts
// an engineio.Manifest object into an engineio.LogicDescriptor which can
// in turn be used to construct an engineio.LogicDriver implementation.
//
// manifestCompiler internally uses the tools and validation rules provided by pisa.ArtifactBuilder
type ManifestCompiler struct {
	manifest   engineio.Manifest
	fueltank   engineio.EngineFuel
	dependency *depgraph.DependencyGraph

	builder *pisa.ArtifactBuilder

	classdefs map[string]engineio.Classdef
	callsites map[string]engineio.Callsite

	deployers []uint64
	enlisters []uint64

	eventdefs map[string]engineio.Eventdef
}

func NewManifestCompiler(fuel uint64, manifest engineio.Manifest) *ManifestCompiler {
	return &ManifestCompiler{
		fueltank:   fuel,
		manifest:   manifest,
		dependency: depgraph.NewDependencyGraph(),

		builder: pisa.NewArtifactBuilder(),

		classdefs: make(map[string]engineio.Classdef),
		callsites: make(map[string]engineio.Callsite),

		deployers: make([]uint64, 0),

		eventdefs: make(map[string]engineio.Eventdef),
		// enlisters: make([]uint64, 0),
	}
}

func (compiler *ManifestCompiler) CompileArtifact() (*logic.Artifact, engineio.EngineFuel, error) {
	// Insert elements to the depgraph
	for _, element := range compiler.manifest.Elements() {
		compiler.dependency.Insert(element.Ptr, element.Deps...)
	}

	// Resolve the dependency graph into a sequence of element pointers
	// representing the order in which the elements must be compiled
	order, ok := compiler.dependency.Resolve()
	if !ok {
		return nil, compiler.fueltank, errors.New("invalid manifest: circular/empty dependency detected")
	}

	// Exhaust some fuel for the base and dependency resolution
	if !compiler.exhaust(BaseCompileCost + DepsResolveCost) {
		return nil, compiler.fueltank, errors.Wrap(ErrInsufficientCompileFuel, "dependency resolution failed")
	}

	// Iterate over the element pointers in declared compile order
	for _, ptr := range order {
		element, _ := compiler.manifest.GetElement(ptr)

		switch element.Kind {
		case ConstantElement:
			// Compile the element as a ConstantElement
			if err := compiler.compileConstantElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "constant element [%#v] compile failed", ptr)
			}

		case TypedefElement:
			// Compile the element as a TypedefElement
			if err := compiler.compileTypedefElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "typedef element [%#v] compile failed", ptr)
			}

		case ClassElement:
			// Compile the element as a ClassElement
			if err := compiler.compileClassElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "class element [%#v] compile failed", ptr)
			}

		case StateElement:
			// Compile the element as a StateElement
			if err := compiler.compileStateElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "state element [%#v] compile failed", ptr)
			}

		case RoutineElement:
			// Compile the element as a RoutineElement
			if err := compiler.compileRoutineElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "routine element [%#v] compile failed", ptr)
			}

		case MethodElement:
			// Compile the element as a MethodElement
			if err := compiler.compileMethodElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "method element [%#v] compile failed", ptr)
			}

		case EventElement:
			// Compile the element as an EventElement
			if err := compiler.compileEventElement(element); err != nil {
				return nil, compiler.fueltank, errors.Wrapf(err, "event element [%#v] compile failed", ptr)
			}

		default:
			return nil, compiler.fueltank, errors.Errorf("invalid element kind [%#v]: %v", ptr, element.Kind)
		}
	}

	artifact, err := compiler.builder.Build()
	if err != nil {
		return nil, compiler.fueltank, errors.Wrap(err, "artifact build failed")
	}

	return artifact, compiler.fueltank, nil
}

func (compiler *ManifestCompiler) CompileDescriptor() (engineio.LogicDescriptor, engineio.EngineFuel, error) {
	artifact, fueltank, err := compiler.CompileArtifact()
	if err != nil {
		return engineio.LogicDescriptor{}, fueltank, err
	}

	// Generate the hash of the manifest
	manifestHash := compiler.manifest.Hash()
	manifestData, _ := compiler.manifest.Encode(common.POLO)

	// Generate a LogicDescriptor from the compiler data
	descriptor := engineio.LogicDescriptor{
		Engine: engineio.PISA,

		ManifestData: manifestData,
		ManifestHash: manifestHash,

		Callsites: compiler.callsites,
		Classdefs: compiler.classdefs,

		Depgraph: compiler.dependency,
		Elements: make(map[engineio.ElementPtr]*engineio.LogicElement),

		Eventdefs: compiler.eventdefs,
	}

	// Check if the compiled logic includes a persistent state
	if ptr := artifact.Directory.Persistent; ptr != nil {
		// Set the persistent state pointer to the descriptor
		descriptor.Persistent = ptr

		// Check that at least one deployer has been compiled
		if len(compiler.deployers) == 0 {
			return engineio.LogicDescriptor{},
				compiler.fueltank,
				errors.New("logic with persistent state must have at least one deployer")
		}
	}

	// Check if the compiled logic includes a ephemeral state
	if ptr := artifact.Directory.Ephemeral; ptr != nil {
		// Set the persistent state pointer to the descriptor
		descriptor.Ephemeral = ptr

		// Check that at least one enlister has been compiled
		if len(compiler.enlisters) == 0 {
			return engineio.LogicDescriptor{},
				compiler.fueltank,
				errors.New("logic with ephemeral state must have at least one enlister")
		}
	}

	for ptr := uint64(0); ptr < artifact.Size(); ptr++ {
		element, _ := artifact.Element(ptr)
		descriptor.Elements[ptr] = &engineio.LogicElement{
			Kind: element.Kind.String(),
			Deps: element.Deps,
			Data: element.Data,
		}
	}

	return descriptor, compiler.fueltank, nil
}

// exhaust exhausts some fuel from the compiler and return false if there isnt sufficient fuel
func (compiler *ManifestCompiler) exhaust(amount uint64) bool {
	if compiler.fueltank < amount {
		return false
	}

	compiler.fueltank -= amount

	return true
}

// compileConstantElement compiles an engineio.ManifestElement object of type ConstantElement
func (compiler *ManifestCompiler) compileConstantElement(element engineio.ManifestElement) error {
	// Exhaust fuel for constant compilation
	if !compiler.exhaust(ConstantCompileCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a ConstantSchema
	schema, ok := element.Data.(*ConstantSchema)
	if !ok {
		return errors.New("invalid element data for 'constant' kind")
	}

	// Check that there are no dependencies
	// constant elements don't allow dependencies
	if len(element.Deps) != 0 {
		return errors.New("invalid constant element: cannot have dependencies")
	}

	// Parse the token literal into a Typedef
	datatype, err := datatypes.Parse(schema.Type, compiler.builder)
	if err != nil {
		return errors.Wrap(err, "invalid constant datatype")
	}

	// Decode value hex string into bytes (remove the 0x prefix if it exists)
	datavalue, err := hex.DecodeString(strings.TrimPrefix(schema.Value, "0x"))
	if err != nil {
		return errors.Wrap(err, "invalid constant value: invalid hexadecimal")
	}

	// Create a new Constant object
	constant, err := pisa.NewConstant(datatype, datavalue)
	if err != nil {
		return errors.Wrap(err, "invalid constant element")
	}

	// Add the Constant element into the builder
	if err = compiler.builder.AddConstant(element.Ptr, constant); err != nil {
		return errors.Wrap(err, "invalid constant element")
	}

	return nil
}

// compileTypedefElement compiles an engineio.ManifestElement object of type TypedefElement
func (compiler *ManifestCompiler) compileTypedefElement(element engineio.ManifestElement) error {
	// Exhaust fuel for typedef compilation
	if !compiler.exhaust(TypedefCompileCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a TypedefSchema
	schema, ok := element.Data.(*TypedefSchema)
	if !ok {
		return errors.New("invalid element data for 'typedef' kind")
	}

	// Parse the type expression into a Typedef
	datatype, err := datatypes.Parse(string(*schema), compiler.builder)
	if err != nil {
		return errors.Wrap(err, "invalid typedef element: invalid type expression")
	}

	// Add the Typedef element into the builder
	if err = compiler.builder.AddTypedef(element.Ptr, datatype, element.Deps); err != nil {
		return errors.Wrap(err, "invalid typedef element")
	}

	return nil
}

// compileClassElement compiles an engineio.ManifestElement object into a ClassElement
func (compiler *ManifestCompiler) compileClassElement(element engineio.ManifestElement) error {
	// Exhaust fuel for class element compilation
	// We will charge cost beyond this per field in the class
	if !compiler.exhaust(BaseClassCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a ClassSchema
	schema, ok := element.Data.(*ClassSchema)
	if !ok {
		return errors.New("invalid element data for 'class' kind")
	}

	// Create a new datatypes.TypeFields from the class fields
	fields, err := compiler.compileTypeFields(schema.Fields, false)
	if err != nil {
		return errors.Errorf("invalid class element: invalid fields: %v", err)
	}

	// Create new method table from class methods
	methods := make(map[opcode.MethodCode]engineio.ElementPtr)
	for _, method := range schema.Methods {
		methods[opcode.MethodCode(method.Code)] = method.Ptr
	}

	// Create a new Class Typedef with methods
	class := datatypes.NewClass(schema.Name, fields).WithMethods(methods)
	// Add the Class element into the builder
	if err = compiler.builder.AddClass(element.Ptr, class, element.Deps); err != nil {
		return errors.Wrap(err, "invalid class element")
	}

	// Add the classdef to the compiler
	compiler.classdefs[schema.Name] = engineio.Classdef{Ptr: element.Ptr, Name: schema.Name}

	return nil
}

// compileEventElement compiles an engineio.ManifestElement object into an EventElement
func (compiler *ManifestCompiler) compileEventElement(element engineio.ManifestElement) error {
	// Exhaust fuel for event element compilation
	// We will charge cost beyond this per field in the event
	if !compiler.exhaust(BaseEventCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into an EventSchema
	schema, ok := element.Data.(*EventSchema)
	if !ok {
		return errors.New("invalid element data for 'event' kind")
	}

	// Create a new datatypes.TypeFields from the event fields
	fields, err := compiler.compileTypeFields(schema.Fields, false)
	if err != nil {
		return errors.Errorf("invalid event element: invalid fields: %v", err)
	}

	// Create a new Event Typedef
	event, err := datatypes.NewEvent(schema.Name, schema.Topics, fields)
	if err != nil {
		return errors.Errorf("invalid event element: %v", err)
	}

	// Add the Event element into the builder
	if err = compiler.builder.AddEvent(element.Ptr, event); err != nil {
		return errors.Wrap(err, "invalid event element")
	}

	// Add the eventdef to the compiler
	compiler.eventdefs[schema.Name] = engineio.Eventdef{Ptr: element.Ptr, Name: schema.Name}

	return nil
}

// compileMethodElement compiles an engineio.ManifestElement object into a MethodElement
func (compiler *ManifestCompiler) compileMethodElement(element engineio.ManifestElement) error {
	// Exhaust fuel for method compilation
	if !compiler.exhaust(ClassMethodCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a MethodSchema
	schema, ok := element.Data.(*MethodSchema)
	if !ok {
		return errors.New("invalid element data for 'routine' kind")
	}

	// Create a new TypeFields from the schema 'accepts'
	inputs, err := compiler.compileTypeFields(schema.Accepts, false)
	if err != nil {
		return errors.Wrap(err, "invalid accept fields")
	}

	// Create a new TypeFields from the schema 'returns'
	// WE ALLOW EVENTS IN THIS ONLY TO SUPPORT __event__ METHODS FOR CLASSES
	outputs, err := compiler.compileTypeFields(schema.Returns, true)
	if err != nil {
		return errors.Wrap(err, "invalid return fields")
	}

	// Compile the instructions for the routine logic
	instructions, err := compiler.compileInstructions(schema.Executes)
	if err != nil {
		return errors.Wrap(err, "invalid instructions")
	}

	// Get class associated with the method
	class, ok := compiler.builder.GetClassDatatype(schema.Class)
	if !ok {
		return errors.Errorf("invalid class '%v': does not exist", schema.Class)
	}

	// Confirm that class describes a method at the expected slot
	methodCode := func() *opcode.MethodCode {
		for code, mptr := range class.Methods() {
			// Return the method code only if the method element pointer matches
			// the defined element pointer in the class type's method table
			if mptr == element.Ptr {
				return &code
			}
		}

		return nil
	}()

	// If method code did not match or was not found, return an error
	if methodCode == nil {
		return errors.Errorf("method not declared on class '%v'", schema.Class)
	}

	// Create a routine for the compiled method
	method := &pisa.RoutineMethod{
		Name:  schema.Name,
		Point: element.Ptr,
		CallFields: logic.CallFields{
			Inputs:  inputs,
			Outputs: outputs,
		},
		Datatype:  class,
		Code:      *methodCode,
		Mutable:   schema.Mutable,
		Instructs: instructions,
	}

	// Add the Method element into the builder
	if err = compiler.builder.AddMethod(method, element.Deps); err != nil {
		return errors.Wrap(err, "invalid class element")
	}

	return nil
}

// compileStateElement compiles an engineio.ManifestElement object of type StateElement
func (compiler *ManifestCompiler) compileStateElement(element engineio.ManifestElement) error {
	// Exhaust fuel for state fields compilation
	// We charge more per field in the state
	if !compiler.exhaust(BaseStateCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a StateSchema
	schema, ok := element.Data.(*StateSchema)
	if !ok {
		return errors.New("invalid element data for 'state' kind")
	}

	// Create a new TypeFields from the map set
	fields, err := compiler.compileTypeFields(schema.Fields, false)
	if err != nil {
		return errors.Errorf("invalid state element: invalid fields: %v", err)
	}

	// Check intended scope for StateElement
	switch schema.Mode {
	case state.Persistent:
		// Add the State element into the builder
		if err = compiler.builder.AddPersistentState(element.Ptr, state.NewStateFields(fields), element.Deps); err != nil {
			return errors.Wrap(err, "invalid class element")
		}

		return nil

	case state.Ephemeral:
		// Add the State element into the builder
		if err = compiler.builder.AddEphemeralState(element.Ptr, state.NewStateFields(fields), element.Deps); err != nil {
			return errors.Wrap(err, "invalid class element")
		}

		return nil

	default:
		return errors.Errorf("invalid mode '%v' for state element", schema.Mode)
	}
}

// compileRoutineElement compiles an engineio.ManifestElement object of type RoutineElement
func (compiler *ManifestCompiler) compileRoutineElement(element engineio.ManifestElement) (err error) {
	// Exhaust fuel for base routine compilation
	// We charge more for specific callsite types and per instruction in the routine.
	if !compiler.exhaust(BaseRoutineCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a RoutineSchema
	schema, ok := element.Data.(*RoutineSchema)
	if !ok {
		return errors.New("invalid element data for 'routine' kind")
	}

	// Check type of RoutineElement
	switch schema.Kind {
	case engineio.CallsiteInternal:
		// Supported with no checks (all dependencies supported)

	case engineio.CallsiteInvoke:
		// Exhaust surcharge fuel for invokable endpoint compilation
		if !compiler.exhaust(InvokableRoutineSurcharge) {
			return ErrInsufficientCompileFuel
		}

		// Supported with no checks (all dependencies supported)

	case engineio.CallsiteDeploy:
		// Exhaust surcharge fuel for deployer endpoint compilation
		if !compiler.exhaust(DeployerRoutineSurcharge) {
			return ErrInsufficientCompileFuel
		}

		// Check that deployer has the persistent mode
		if schema.Mode != state.Persistent {
			return errors.New("invalid routine element: invalid state mode for deployer routine")
		}

		// Check if the persistent state has been compiled
		pstate, ok := compiler.builder.GetPersistentStatePtr()
		if !ok {
			return errors.New("invalid routine element: deployer routine for non-existent persistent storage")
		}

		// Check if the routine has the necessary dependency for the persistent state
		if !contains(element.Deps, pstate) {
			return errors.New("invalid routine element: missing dependency on persistent state for deployer")
		}

	case engineio.CallsiteInteract:
		return errors.New("interactable routine elements are not supported")

	case engineio.CallsiteEnlist:
		// Exhaust surcharge fuel for enlister endpoint compilation
		if !compiler.exhaust(EnlisterRoutineSurcharge) {
			return ErrInsufficientCompileFuel
		}

		// Check that enlister has the ephemeral mode
		if schema.Mode != state.Ephemeral {
			return errors.New("invalid routine element: invalid state mode for enlister routine")
		}

		// Check if the ephemeral state has been compiled
		estate, ok := compiler.builder.GetEphemeralStatePtr()
		if !ok {
			return errors.New("invalid routine element: enlister routine for non-existent ephemeral storage")
		}

		// Check if the routine has the necessary dependency for the ephemeral state
		if !contains(element.Deps, estate) {
			return errors.New("invalid routine element: missing dependency on ephemeral state for enlister")
		}

	default:
		return errors.Errorf("invalid kind '%v' for routine element", schema.Kind)
	}

	// Create a new TypeFields from the schema 'accepts'
	inputs, err := compiler.compileTypeFields(schema.Accepts, false)
	if err != nil {
		return errors.Wrap(err, "invalid routine: invalid accept fields")
	}

	// Create a new TypeFields from the schema 'returns'
	outputs, err := compiler.compileTypeFields(schema.Returns, false)
	if err != nil {
		return errors.Wrap(err, "invalid routine: invalid return fields")
	}

	// Compile the instructions for the routine logic
	instructions, err := compiler.compileInstructions(schema.Executes)
	if err != nil {
		return errors.Wrap(err, "invalid routine: invalid instructions")
	}

	// Create a routine element
	routine := &pisa.Routine{
		Point:     element.Ptr,
		Name:      schema.Name,
		Mode:      schema.Mode,
		Endpoint:  schema.Kind != engineio.CallsiteInternal,
		Instructs: instructions,
		CallFields: logic.CallFields{
			Inputs:  inputs,
			Outputs: outputs,
		},
	}

	// Add the Routine element to the builder
	if err = compiler.builder.AddRoutine(routine, element.Deps); err != nil {
		return errors.Wrap(err, "invalid routine element")
	}

	// Add the endpoint reference to the compiler metadata
	if routine.Endpoint {
		switch schema.Kind {
		case engineio.CallsiteDeploy:
			compiler.deployers = append(compiler.deployers, element.Ptr)
		case engineio.CallsiteEnlist:
			compiler.enlisters = append(compiler.enlisters, element.Ptr)
		}

		compiler.callsites[schema.Name] = engineio.Callsite{Ptr: element.Ptr, Name: schema.Name, Kind: schema.Kind}
	}

	return nil
}

// compileTypeFields compiles a list of TypefieldSchema objects into datatypes.TypeFields.
// Returns an error if the given map of field expressions contains positional gaps or invalid expressions.
func (compiler *ManifestCompiler) compileTypeFields(
	table []TypefieldSchema, allowEvents bool,
) (
	*datatypes.TypeFields, error,
) {
	// Error if there are more than 2^8 slots
	if len(table) > 256 {
		return nil, errors.New("invalid field set: too many typefield schema (max 256)")
	}

	// Exhaust fuel for typefields compilation per type expression expected to be parsed
	if !compiler.exhaust(TypeExprParseCost * engineio.EngineFuel(len(table))) {
		return nil, ErrInsufficientCompileFuel
	}

	// Create a map to collect all compiled typefields
	fields := make(map[uint8]*datatypes.TypeField)
	// Iterate over the type fields in the table
	for _, typefield := range table {
		// If the slot has already been encountered, return error
		if _, exists := fields[typefield.Slot]; exists {
			return nil, errors.Errorf("invalid typefield for slot '%v': duplicate type field", typefield.Slot)
		}

		// Parse the enclosed data into a datatype
		dt, err := datatypes.Parse(typefield.Type, compiler.builder)
		if err != nil {
			return nil, errors.Wrap(err, "invalid type field type data")
		}

		// Insert TypeField into the map
		fields[typefield.Slot] = &datatypes.TypeField{Name: typefield.Label, Type: dt}
	}

	// Gap detection is performed implicitly when using this constructor
	return datatypes.NewFieldsWithSlots(fields, allowEvents)
}

// compileInstructions compiles an InstructionsSchema into some runtime.Instructions.
func (compiler *ManifestCompiler) compileInstructions(schema InstructionsSchema) (pisa.Instructions, error) {
	switch {
	case len(schema.Bin) > 0:
		return compiler.compileBinInstructions(schema.Bin)
	case schema.Hex != "":
		return compiler.compileHexInstructions(schema.Hex)
	case schema.Asm != nil:
		return compiler.compileAsmInstructions(schema.Asm)
	case schema.Bin != nil:
		return nil, nil
	default:
		return nil, errors.New("no instructions found")
	}
}

// compileBinInstructions compiles an InstructionsSchema with binary instructions into runtime.Instructions.
func (compiler *ManifestCompiler) compileBinInstructions(instructions []byte) (pisa.Instructions, error) {
	reader := bytes.NewReader(instructions)
	instructs := make(pisa.Instructions, 0)

	for line := 0; reader.Len() != 0; line++ {
		// Fuel charge for instruction (base = BIN)
		if !compiler.exhaust(BaseInstructionCost) {
			return nil, ErrInsufficientCompileFuel
		}

		// Read an opcode byte
		code, _ := reader.ReadByte()

		// Check the number of args for the opcode
		count, ok := opcode.OpCode(code).Operands()
		if !ok {
			return nil, errors.Errorf("invalid opcode '%#v' [line %v]", code, line)
		}

		instruct := pisa.Instruction{Op: opcode.OpCode(code)}

		// If no operands are expected for the opcode, skip operand reading
		if count == 0 {
			// Append instruction into the program code
			instructs = append(instructs, instruct)

			continue
		}

		operands := make([]byte, count)
		// Read the operands
		read, err := reader.Read(operands)
		if read != count || err != nil {
			return nil, errors.Errorf("insufficient operands for '%#v' [line %v]", code, line)
		}

		// Set the operands into the instruction
		instruct.Args = operands
		// Append instruction into the program code
		instructs = append(instructs, instruct)
	}

	return instructs, nil
}

// compileHexInstructions compiles an InstructionsSchema with hexadecimal instructions into runtime.Instructions.
func (compiler *ManifestCompiler) compileHexInstructions(instructions string) (pisa.Instructions, error) {
	// Remove the 0x prefix if it exists
	instructions = strings.TrimPrefix(instructions, "0x")

	decodedInstructs, err := hex.DecodeString(instructions)
	if err != nil {
		return nil, errors.Wrap(err, "invalid hex instructions")
	}

	compiled, err := compiler.compileBinInstructions(decodedInstructs)
	if err != nil {
		return nil, err
	}

	// Calculate the surcharge PER hex instruction
	surcharge := HexInstructionSurcharge * uint64(len(compiled))
	// Exhaust fuel surcharge for HEX instruction
	// The base instruction cost is exhausted when decoding the binary instruction
	if !compiler.exhaust(surcharge) {
		return nil, ErrInsufficientCompileFuel
	}

	return compiled, nil
}

// compileAsmInstructions compiles an InstructionsSchema with assembly instructions into runtime.Instructions.
func (compiler *ManifestCompiler) compileAsmInstructions(asm []string) (pisa.Instructions, error) {
	// Calculate the surcharge PER asm instruction
	surcharge := AsmInstructionSurcharge * uint64(len(asm))
	// Exhaust fuel surcharge for ASM instruction
	// The base instruction cost is exhausted when decoding the binary instruction
	if !compiler.exhaust(surcharge) {
		return nil, ErrInsufficientCompileFuel
	}

	binary, err := opcode.Asm2Bin(asm)
	if err != nil {
		return nil, err
	}

	return compiler.compileBinInstructions(binary)
}

// hasgaps returns if the keys of a map of unsigned numbers has gaps.
// They must also start from 0 and go up to len-1
//
//nolint:unused
func hasgaps[U constraints.Unsigned](indices map[U]struct{}) bool {
	for i := U(0); i < U(len(indices)); i++ {
		if _, exists := indices[i]; !exists {
			return true
		}
	}

	return false
}

// contains returns if a numeric value is present in the array of values
func contains[T constraints.Integer](array []T, check T) bool {
	for _, ptr := range array {
		if ptr == check {
			return true
		}
	}

	return false
}

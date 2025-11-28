package pisa

import (
	"encoding/hex"
	"strings"

	"github.com/manishmeganathan/depgraph"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"golang.org/x/exp/constraints"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/datatypes"
	"github.com/sarvalabs/go-pisa/opcode"
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
	// BaseExternCost is the base cost for compiling an ExternElement
	BaseExternCost engineio.EngineFuel = 10
	// BaseStateCost is the base cost for compiling a StateElement
	BaseStateCost engineio.EngineFuel = 20
	// BaseEventCost is the base cost for compiling an EventElement
	BaseEventCost engineio.EngineFuel = 15

	// ConstantCompileCost is the cost to compile a LiteralElement
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
	manifest     engineio.Manifest
	manifestKind engineio.ManifestKind
	fuel         engineio.FuelGauge
	dependency   *depgraph.DependencyGraph
	logicID      identifiers.Identifier
	builder      *pisa.ArtifactBuilder
}

func NewManifestCompiler(
	manifestKind engineio.ManifestKind,
	logicID identifiers.Identifier,
	fuel engineio.FuelGauge,
	manifest engineio.Manifest,
) *ManifestCompiler {
	mc := &ManifestCompiler{
		fuel:         fuel,
		manifest:     manifest,
		manifestKind: manifestKind,
		dependency:   depgraph.NewDependencyGraph(),
		builder:      pisa.NewArtifactBuilder(common.Hash(manifest.Hash()).String()),
		logicID:      logicID,
	}

	if manifestKind == engineio.AssetKind {
		err := mc.builder.SetArtifactKind("asset")
		if err != nil {
			panic(err)
		}
	}

	return mc
}

func (compiler *ManifestCompiler) CompileArtifact() ([]byte, error) {
	// Insert elements to the depgraph
	for _, element := range compiler.manifest.Elements() {
		compiler.dependency.Insert(element.Ptr, element.Deps...)
	}

	// Resolve the dependency graph into a sequence of element pointers
	// representing the order in which the elements must be compiled
	order, ok := compiler.dependency.Resolve()
	if !ok {
		return nil, errors.New("invalid manifest: circular/empty dependency detected")
	}

	// Exhaust some fuel for the base and dependency resolution
	if !compiler.exhaustCompute(BaseCompileCost + DepsResolveCost) {
		return nil, errors.Wrap(ErrInsufficientCompileFuel, "dependency resolution failed")
	}

	// Iterate over the element pointers in declared compile order
	for _, ptr := range order {
		element, _ := compiler.manifest.GetElement(ptr)

		switch element.Kind {
		case LiteralElement:
			// Compile the element as a LiteralElement
			if err := compiler.compileLiteralElement(element); err != nil {
				return nil, errors.Wrapf(err, "constant element [%#v] compile failed", ptr)
			}

		case TypedefElement:
			// Compile the element as a TypedefElement
			if err := compiler.compileTypedefElement(element); err != nil {
				return nil, errors.Wrapf(err, "typedef element [%#v] compile failed", ptr)
			}

		case ClassElement:
			// Compile the element as a ClassElement
			if err := compiler.compileClassElement(element); err != nil {
				return nil, errors.Wrapf(err, "class element [%#v] compile failed", ptr)
			}

		case StateElement:
			// Compile the element as a StateElement
			if err := compiler.compileStateElement(element); err != nil {
				return nil, errors.Wrapf(err, "state element [%#v] compile failed", ptr)
			}

		case RoutineElement:
			// Compile the element as a RoutineElement
			if err := compiler.compileRoutineElement(element); err != nil {
				return nil, errors.Wrapf(err, "routine element [%#v] compile failed", ptr)
			}

		case MethodElement:
			// Compile the element as a MethodElement
			if err := compiler.compileMethodElement(element); err != nil {
				return nil, errors.Wrapf(err, "method element [%#v] compile failed", ptr)
			}

		case EventElement:
			// Compile the element as an EventElement
			if err := compiler.compileEventElement(element); err != nil {
				return nil, errors.Wrapf(err, "event element [%#v] compile failed", ptr)
			}
		case ExternElement:
			if err := compiler.compileExternElement(element); err != nil {
				return nil, errors.Wrapf(err, "extern element [%#v] compile failed", ptr)
			}

		case AssetElement:
			schema, ok := element.Data.(*AssetSchema)
			if !ok {
				return nil, errors.New("invalid element data for 'extern' kind")
			}

			if err := compiler.builder.AddAsset(ptr, pisa.AssetDef{
				AssetID: compiler.logicID,
				Engine:  schema.Engine,
			}); err != nil {
				return nil, errors.Wrapf(err, "asset element [%#v] compile failed", ptr)
			}

		default:
			return nil, errors.Errorf("invalid element kind [%#v]: %v", ptr, element.Kind)
		}
	}

	artifact, err := compiler.builder.GenerateRawArtifact()
	if err != nil {
		return nil, errors.Wrap(err, "artifact build failed")
	}

	return artifact, nil
}

// exhaustCompute exhausts some fuel from the compiler and return false if there isnt sufficient fuel
func (compiler *ManifestCompiler) exhaustCompute(amount uint64) bool {
	if compiler.fuel.Compute < amount {
		return false
	}

	compiler.fuel.Compute -= amount

	return true
}

// compileLiteralElement compiles an engineio.ManifestElement object of type LiteralElement
func (compiler *ManifestCompiler) compileLiteralElement(element engineio.ManifestElement) error {
	// Exhaust fuel for constant compilation
	if !compiler.exhaustCompute(ConstantCompileCost) {
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
	constant, err := pisa.NewLiteral(datatype, datavalue)
	if err != nil {
		return errors.Wrap(err, "invalid constant element")
	}

	// Add the Constant element into the builder
	if err = compiler.builder.AddLiteral(element.Ptr, constant); err != nil {
		return errors.Wrap(err, "invalid constant element")
	}

	return nil
}

// compileTypedefElement compiles an engineio.ManifestElement object of type TypedefElement
func (compiler *ManifestCompiler) compileTypedefElement(element engineio.ManifestElement) error {
	// Exhaust fuel for typedef compilation
	if !compiler.exhaustCompute(TypedefCompileCost) {
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
	if err = compiler.builder.AddType(element.Ptr, datatype, element.Deps); err != nil {
		return errors.Wrap(err, "invalid typedef element")
	}

	return nil
}

func (compiler *ManifestCompiler) compileExternElement(element engineio.ManifestElement) error {
	// Exhaust fuel for class element compilation
	// We will charge cost beyond this per field in the class
	if !compiler.exhaustCompute(BaseExternCost) {
		return ErrInsufficientCompileFuel
	}

	schema, ok := element.Data.(*ExternSchema)
	if !ok {
		return errors.New("invalid element data for 'extern' kind")
	}

	var (
		logicDef *pisa.StateDef
		actorDef *pisa.StateDef
		err      error
	)

	if schema.Logic != nil {
		logicDef, err = compiler.getStateDef(schema.Logic, false)
		if err != nil {
			return errors.Wrap(err, "invalid extern element: invalid logic state")
		}
	}

	if schema.Actor != nil {
		actorDef, err = compiler.getStateDef(schema.Actor, false)
		if err != nil {
			return errors.Wrap(err, "invalid extern element: invalid actor state")
		}
	}

	callables := make([]pisa.ExternalCallable, 0, len(schema.Endpoints))

	for _, routine := range schema.Endpoints {
		callable, err := compiler.compileExternEndpoint(&routine)
		if err != nil {
			return errors.Wrap(err, "invalid extern element: invalid routine")
		}

		callables = append(callables, *callable)
	}

	return compiler.builder.AddExtern(
		element.Ptr,
		&pisa.ExternDef{
			Name:      schema.Name,
			Logic:     logicDef,
			Actor:     actorDef,
			Callables: callables,
		}, element.Deps)
}

// compileClassElement compiles an engineio.ManifestElement object into a ClassElement
func (compiler *ManifestCompiler) compileClassElement(element engineio.ManifestElement) error {
	// Exhaust fuel for class element compilation
	// We will charge cost beyond this per field in the class
	if !compiler.exhaustCompute(BaseClassCost) {
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

	return nil
}

// compileEventElement compiles an engineio.ManifestElement object into an EventElement
func (compiler *ManifestCompiler) compileEventElement(element engineio.ManifestElement) error {
	// Exhaust fuel for event element compilation
	// We will charge cost beyond this per field in the event
	if !compiler.exhaustCompute(BaseEventCost) {
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

	return nil
}

// compileMethodElement compiles an engineio.ManifestElement object into a MethodElement
func (compiler *ManifestCompiler) compileMethodElement(element engineio.ManifestElement) error {
	// Exhaust fuel for method compilation
	if !compiler.exhaustCompute(ClassMethodCost) {
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
	method := &pisa.DefinedMethod{
		Name:  schema.Name,
		Point: element.Ptr,
		CallFields: datatypes.CallFields{
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

func (compiler *ManifestCompiler) getStateDef(schema *StateSchema, allowEvents bool) (*pisa.StateDef, error) {
	// Create a new TypeFields from the map set
	fields, err := compiler.compileTypeFields(schema.Fields, allowEvents)
	if err != nil {
		return nil, errors.Errorf("invalid state element: invalid fields: %v", err)
	}

	// Check the intended scope for StateElement
	stateKind := StateModeToKind(schema.Mode)
	if stateKind < 0 {
		return nil, errors.Errorf("invalid mode for state element")
	}

	return pisa.NewStateDef(stateKind, fields), nil
}

// compileStateElement compiles an engineio.ManifestElement object of type StateElement
func (compiler *ManifestCompiler) compileStateElement(element engineio.ManifestElement) error {
	// Exhaust fuel for state fields compilation
	// We charge more per field in the state
	if !compiler.exhaustCompute(BaseStateCost) {
		return ErrInsufficientCompileFuel
	}

	// Convert element into a StateSchema
	schema, ok := element.Data.(*StateSchema)
	if !ok {
		return errors.New("invalid element data for 'state' kind")
	}

	pisaStateDef, err := compiler.getStateDef(schema, false)
	if err != nil {
		return err
	}

	return compiler.builder.AddState(element.Ptr, pisaStateDef, element.Deps)
}

// compileRoutineElement compiles an engineio.ManifestElement object of type RoutineElement
func (compiler *ManifestCompiler) compileRoutineElement(element engineio.ManifestElement) (err error) {
	// Exhaust fuel for base routine compilation
	// We charge more for specific callsite types and per instruction in the routine.
	if !compiler.exhaustCompute(BaseRoutineCost) {
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
		if !compiler.exhaustCompute(InvokableRoutineSurcharge) {
			return ErrInsufficientCompileFuel
		}

		// Supported with no checks (all dependencies supported)

	case engineio.CallsiteDeploy:
		// Exhaust surcharge fuel for deployer endpoint compilation
		if !compiler.exhaustCompute(DeployerRoutineSurcharge) {
			return ErrInsufficientCompileFuel
		}

		// Check that deployer has the persistent mode
		if schema.Mode != DynamicState {
			return errors.New("invalid routine element: invalid state mode for deployer routine")
		}

		// Check if the persistent state has been compiled
		pstate, ok := compiler.builder.GetLogicStatePtr()
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
		if !compiler.exhaustCompute(EnlisterRoutineSurcharge) {
			return ErrInsufficientCompileFuel
		}

		// Check that enlister has the ephemeral mode
		if schema.Mode != DynamicState {
			return errors.New("invalid routine element: invalid state mode for enlister routine")
		}

		// Check if the ephemeral state has been compiled
		estate, ok := compiler.builder.GetActorStatePtr()
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
	routine := &pisa.DefinedCallable{
		Point:      element.Ptr,
		Name:       schema.Name,
		Access:     pisa.DynamicAccess{},
		Visibility: pisa.CallVisibility(schema.Kind),
		Instructs:  instructions,
		CallFields: datatypes.CallFields{
			Inputs:  inputs,
			Outputs: outputs,
		},
	}

	switch schema.Mode {
	case "dynamic":
		routine.Access = pisa.DynamicAccess{
			Logic:  datatypes.AccessWrite,
			Origin: datatypes.AccessWrite,
			Actors: datatypes.AccessWrite,
		}
	case "static":
		routine.Access = pisa.DynamicAccess{
			Logic:  datatypes.AccessRead,
			Origin: datatypes.AccessRead,
			Actors: datatypes.AccessRead,
		}
	case "pure":
		routine.Access = pisa.DynamicAccess{
			Logic:  datatypes.AccessNone,
			Origin: datatypes.AccessNone,
			Actors: datatypes.AccessNone,
		}
	default:
		return errors.Errorf("invalid mode for routine element")
	}

	// Add the Routine element to the builder
	if err = compiler.builder.AddCallable(routine, element.Deps); err != nil {
		return errors.Wrap(err, "failed to add routine element")
	}

	return nil
}

func (compiler *ManifestCompiler) compileExternEndpoint(
	routine *ExternalRoutineSchema,
) (*pisa.ExternalCallable, error) {
	// Exhaust fuel for external callable compilation
	if !compiler.exhaustCompute(BaseRoutineCost) {
		return nil, ErrInsufficientCompileFuel
	}

	// Create a new TypeFields from the schema 'accepts'
	inputs, err := compiler.compileTypeFields(routine.Accepts, false)
	if err != nil {
		return nil, errors.Wrap(err, "invalid callable: invalid accept fields")
	}

	// Create a new TypeFields from the schema 'returns'
	outputs, err := compiler.compileTypeFields(routine.Returns, false)
	if err != nil {
		return nil, errors.Wrap(err, "invalid callable: invalid return fields")
	}

	callable := &pisa.ExternalCallable{
		Name:       routine.Name,
		CallFields: datatypes.CallFields{Inputs: inputs, Outputs: outputs},
	}

	return callable, nil
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
	if !compiler.exhaustCompute(TypeExprParseCost * engineio.EngineFuel(len(table))) {
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
	// TODO: Exhaust fuel for BIN instructions
	return pisa.NewBinaryInstructions(instructions)
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
	if !compiler.exhaustCompute(surcharge) {
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
	if !compiler.exhaustCompute(surcharge) {
		return nil, ErrInsufficientCompileFuel
	}

	// TODO: Charge fuel for ASM instructions
	return pisa.NewAssemblyInstructions(asm)
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

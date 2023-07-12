package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

// Runtime represents the engine runtime for PISA. It holds the base instruction
// set and builtin primitives that is copied into every engine instance instead
// of recomputing them each time. Implements the engineio.EngineFactory interface.
type Runtime struct {
	instructs        InstructionSet
	primitiveMethods map[PrimitiveDatatype][256]*BuiltinMethod

	builtinLibrary map[uint64]*Builtin
	builtinClasses map[string]struct {
		datatype BuiltinDatatype
		methods  [256]*BuiltinMethod
	}
}

// NewRuntime generates a new Runtime instance that can be
// used to generate new instances of the PISA Execution Engine
func NewRuntime() Runtime {
	runtime := Runtime{
		instructs:        BaseInstructionSet(),
		primitiveMethods: make(map[PrimitiveDatatype][256]*BuiltinMethod),
		builtinClasses: make(map[string]struct {
			datatype BuiltinDatatype
			methods  [256]*BuiltinMethod
		}),
	}

	runtime.setupBuiltinLibrary()
	runtime.setupBuiltinClasses()
	runtime.setupPrimitiveMethods()

	return runtime
}

// Kind returns the kind of engine factory and implements
// the engineio.EngineRuntime interface for the PISA
func (runtime Runtime) Kind() engineio.EngineKind { return engineio.PISA }

// SpawnEngine generates a new Engine instance bootstrapped with some Fuel, a
// Logic, its associated Context and an Environment Driver. The returned Engine
// is configured with the base PISA instruction set and builtin primitives.
func (runtime Runtime) SpawnEngine(
	fuel engineio.Fuel,
	logic engineio.LogicDriver,
	state engineio.CtxDriver,
	env engineio.EnvDriver,
) (
	engineio.Engine, error,
) {
	// Check logic driver engine
	if logic.Engine() != engineio.PISA {
		return nil, errors.New("incompatible logic driver: not a PISA logic")
	}

	// Check logic driver's logic ID and its context's logic ID.
	if logic.LogicID() != state.LogicID() {
		return nil, errors.New("incompatible context driver for logic: logic ID is not equal")
	}

	// Check the logic driver and context driver's addresses
	if logic.LogicID().Address().Hex() != state.Address().Hex() {
		return nil, errors.New("incompatible context driver for logic: address does not match")
	}

	return &Engine{
		runtime:   &runtime,
		callstack: make(callstack, 0),
		fueltank:  engineio.NewFuelTank(fuel),

		classes:  make(map[string]engineio.ElementPtr),
		elements: make(map[engineio.ElementPtr]any),

		logic:       logic,
		persistent:  state,
		environment: env,
	}, nil
}

// CompileManifest generates an engineio.LogicDescriptor from an engineio.Manifest object.
// The compilation will fail if the manifest object is not compatible or has malformed
// elements. It implements the engineio.EngineDriver interface.
func (runtime Runtime) CompileManifest(
	fuel engineio.Fuel,
	manifest *engineio.Manifest,
) (
	*engineio.LogicDescriptor, engineio.Fuel, error,
) {
	// Check that the Manifest Engine is PISA
	if manifest.Header().LogicEngine() != engineio.PISA {
		return nil, nil, errors.New("invalid manifest: manifest engine is not PISA")
	}

	// Create a ManifestCompiler instance
	compiler, err := newManifestCompiler(fuel, manifest, runtime.instructs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialise manifest compiler")
	}

	// Compile the manifest
	descriptor, err := compiler.compile()
	if err != nil {
		return nil, compiler.fueltank.Consumed, errors.Wrap(err, "")
	}

	return descriptor, compiler.fueltank.Consumed, nil
}

// ValidateCalldata verifies the IxnObject's callsite and calldata for a given engineio.LogicDriver.
// Returns an error if the logic is not supported by PISA or if the callsite does not exist or if
// the callsite is not compatible with the ixn type or if the calldata is invalid for the callsite.
func (runtime Runtime) ValidateCalldata(logic engineio.LogicDriver, ixn *engineio.IxnObject) error {
	// Check logic driver engine
	if logic.Engine() != engineio.PISA {
		return errors.New("incompatible logic driver: not a PISA logic")
	}

	// Get the callsite information from the logic and verify that it exists
	callsite, ok := logic.GetCallsite(ixn.Callsite())
	if !ok {
		return errors.Errorf("invalid callsite '%v': does not exist", ixn.Callsite())
	}

	// Check that the callsite kind and ixn type of the IxnObject are compatible
	switch callsite.Kind {
	case engineio.InvokableCallsite:
		if ixn.IxType() != common.IxLogicInvoke {
			return errors.Errorf("invalid callsite '%v' for IxnLogicInvoke", ixn.Callsite())
		}

	case engineio.DeployerCallsite:
		if ixn.IxType() != common.IxLogicDeploy {
			return errors.Errorf("invalid callsite '%v' for IxnLogicDeploy", ixn.Callsite())
		}

	case engineio.InteractableCallsite, engineio.EnlisterCallsite:
		return errors.Errorf("unsupported callsite kind '%v' for callsite '%v'", callsite.Kind, ixn.Callsite())

	default:
		panic("invalid callsite kind variant detected")
	}

	element, ok := logic.GetElement(callsite.Ptr)
	if !ok {
		return errors.Errorf("could not fetch element for callsite '%v'", ixn.Callsite())
	}

	routine := new(Routine)
	if err := polo.Depolorize(routine, element.Data); err != nil {
		return errors.Wrap(err, "could not decode element into routine")
	}

	calldata := make(polo.Document)
	// Decode the payload calldata into a polo.Document
	if ixn.Calldata() != nil {
		if err := polo.Depolorize(&calldata, ixn.Calldata()); err != nil {
			return errors.Wrap(err, "could not decode calldata into polo document")
		}
	}

	// Convert the input Calldata into a RegisterSet confirming
	if _, err := NewRegisterSet(routine.Inputs, calldata); err != nil {
		return errors.Errorf("invalid calldata for callsite '%v': %v", ixn.Callsite(), err)
	}

	return nil
}

// GetElementGenerator returns a generator function for the
// schema object for an element of the given engineio.ElementKind
func (runtime Runtime) GetElementGenerator(kind engineio.ElementKind) (engineio.ManifestElementGenerator, bool) {
	generator, exists := elementGenerators[kind]

	return generator, exists
}

// GetCallEncoder generates an engineio.CallEncoder from a engineio.LogicDriver
// that can encode inputs and decode outputs for a given callsite pointer. This will fail
// if no valid callsite element exists for the given pointer in the given Manifest.
func (runtime Runtime) GetCallEncoder(
	callsite *engineio.Callsite, logic engineio.LogicDriver,
) (
	engineio.CallEncoder, error,
) {
	// Check that the logic engine matches for the runtime
	if logic.Engine() != runtime.Kind() {
		return nil, errors.New("incompatible logic for runtime: PISA")
	}

	// Get LogicElement from the LogicDriver
	element, ok := logic.GetElement(callsite.Ptr)
	if !ok {
		return nil, errors.Errorf("cannot find logic element for ptr '%v'", callsite)
	}

	// Check that the element is a RoutineElement
	if element.Kind != RoutineElement {
		return nil, errors.Errorf("cannot generate CallEncode for '%v' element", element.Kind)
	}

	// Decode the element data into a Routine object
	routine := new(Routine)
	if err := polo.Depolorize(routine, element.Data); err != nil {
		return nil, errors.Wrapf(err, "could not decode 'routine' element into Routine")
	}

	// Return the routine callfields as a CallEncoder
	return CallEncoder(routine.callfields()), nil
}

func (runtime *Runtime) setupPrimitiveMethods() {
	primitives := []RegisterObject{
		BoolValue(false),
		BytesValue{},
		StringValue(""),
		AddressValue{},
		U64Value(0),
		I64Value(0),
		&U256Value{},
		&I256Value{},
	}

	for _, primitive := range primitives {
		// Get the type of the primitive
		datatype := primitive.Type().(PrimitiveDatatype) //nolint:forcetypeassert
		// Set the primitive methods to the runtime
		runtime.primitiveMethods[datatype] = primitive.methods()
	}
}

func (runtime *Runtime) setupBuiltinClasses() {
	builtins := []RegisterObject{
		LogicContextValue{},
		ParticipantContextValue{},
	}

	for _, builtin := range builtins {
		// Get the type of the builtin
		datatype := builtin.Type().(BuiltinDatatype) //nolint:forcetypeassert
		// Set the builtin type reference and its methods to the runtime
		runtime.builtinClasses[datatype.name] = struct {
			datatype BuiltinDatatype
			methods  [256]*BuiltinMethod
		}{
			datatype: datatype,
			methods:  builtin.methods(),
		}
	}
}

func (runtime *Runtime) setupBuiltinLibrary() {
	runtime.builtinLibrary = map[uint64]*Builtin{
		0: builtinSHA256(),
		1: builtinKeccak256(),
		2: builtinBLAKE2b(),
		3: builtinSignatureVerify(),
	}
}

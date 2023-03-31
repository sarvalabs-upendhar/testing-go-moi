package pisa

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa/register"
	"github.com/sarvalabs/moichain/types"
)

// Runtime represents the engine runtime for PISA. It holds the base instruction
// set and builtin primitives that is copied into every engine instance instead
// of recomputing them each time. Implements the engineio.EngineFactory interface.
type Runtime struct {
	instructs InstructionSet
	builtins  map[register.PrimitiveType]register.MethodTable
}

// NewRuntime generates a new Runtime instance that can be
// used to generate new instances of the PISA Execution Engine
func NewRuntime() Runtime {
	return Runtime{
		instructs: BaseInstructionSet(),
		builtins: map[register.PrimitiveType]register.MethodTable{
			register.PrimitiveBool:    register.BoolMethods(),
			register.PrimitiveBytes:   register.BytesMethods(),
			register.PrimitiveString:  register.StringMethods(),
			register.PrimitiveU64:     register.U64Methods(),
			register.PrimitiveI64:     register.I64Methods(),
			register.PrimitiveAddress: register.AddressMethods(),
		},
	}
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
	ctx engineio.CtxDriver,
	env engineio.EnvDriver,
) (
	engineio.Engine, error,
) {
	// Check logic driver engine
	if logic.Engine() != engineio.PISA {
		return nil, errors.New("incompatible logic driver: not a PISA logic")
	}

	// Check logic driver's logic ID and its context's logic ID.
	if !bytes.Equal(logic.LogicID(), ctx.LogicID()) {
		return nil, errors.New("incompatible context driver for logic: logic ID is not equal")
	}

	// Check the logic driver and context driver's addresses
	if logic.LogicID().Address().Hex() != ctx.Address().Hex() {
		return nil, errors.New("incompatible context driver for logic: address does not match")
	}

	return &Engine{
		environment: env,
		logic:       logic,
		internal:    ctx,

		instructs: runtime.instructs,
		builtins:  runtime.builtins,

		classes:  make(map[string]engineio.ElementPtr),
		elements: make(map[engineio.ElementPtr]any),
		fueltank: engineio.NewFuelTank(fuel),
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
		return nil, 0, errors.New("invalid manifest: manifest engine is not PISA")
	}

	// Create a ManifestCompiler instance
	compiler, err := newManifestCompiler(fuel, manifest, runtime.instructs)
	if err != nil {
		return nil, 0, errors.Wrap(err, "")
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
		if ixn.IxType() != types.IxLogicInvoke {
			return errors.Errorf("invalid callsite '%v' for IxnLogicInvoke", ixn.Callsite())
		}

	case engineio.DeployerCallsite:
		if ixn.IxType() != types.IxLogicDeploy {
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

	// Convert the input Calldata into a ValueTable
	if _, err := register.NewValueTable(routine.Inputs, ixn.Calldata()); err != nil {
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

// GetCallEncoderFromManifest generates an engineio.CallEncoder from an engineio.Manifest
// that can encode inputs and decode outputs for a given callsite pointer. This will fail
// if no valid callsite element exists for the given pointer in the given Manifest.
func (runtime Runtime) GetCallEncoderFromManifest(
	callsite *engineio.Callsite, manifest *engineio.Manifest,
) (
	engineio.CallEncoder, error,
) {
	// Check that the manifest engine matches for the runtime
	if manifest.Header().LogicEngine() != runtime.Kind() {
		return nil, errors.New("incompatible manifest for runtime: PISA")
	}

	// Get the ManifestElement from the Manifest
	element, ok := manifest.FindElement(callsite.Ptr)
	if !ok {
		return nil, errors.Errorf("cannot find manifest element for ptr '%v'", callsite)
	}

	// Check that the element is a RoutineElement
	if element.Kind != RoutineElement {
		return nil, errors.Errorf("cannot generate CallEncode for '%v' element", element.Kind)
	}

	// Convert the element data into a RoutineSchema
	routine, ok := element.Data.(*RoutineSchema)
	if !ok {
		return nil, errors.New("could not convert 'routine' element into RoutineSchema")
	}

	// Create a new FieldTable from the schema 'accepts'
	inputs, err := compileFieldTable(routine.Accepts)
	if err != nil {
		return nil, errors.Wrap(err, "invalid accept fields")
	}

	// Create a new FieldTable from the schema 'returns'
	outputs, err := compileFieldTable(routine.Returns)
	if err != nil {
		return nil, errors.Wrap(err, "invalid return fields")
	}

	return register.CallFields{Inputs: inputs, Outputs: outputs}, nil
}

// GetCallEncoderFromLogic generates an engineio.CallEncoder from a engineio.LogicDriver
// that can encode inputs and decode outputs for a given callsite pointer. This will fail
// if no valid callsite element exists for the given pointer in the given Manifest.
func (runtime Runtime) GetCallEncoderFromLogic(
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
		return nil, errors.Errorf("could not decode 'routine' element into Routine")
	}

	// Return the routine callfields
	return routine.CallFields, nil
}

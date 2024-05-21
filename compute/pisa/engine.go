package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/exception"
	"github.com/sarvalabs/go-pisa/values"
	"github.com/sarvalabs/go-polo"
)

type Engine struct {
	library *pisa.Library
	crypto  engineio.CryptographyDriver
}

// NewEngine generates a new Engine instance that can be
// used to generate new instances of the PISA Execution Instance
func NewEngine() Engine {
	run := Engine{
		library: &pisa.Library{
			Builtins:  pisa.NewBuiltinSet(),
			Instructs: pisa.NewInstructionSet(),
		},
		crypto: Crypto(0),
	}

	return run
}

func (engine Engine) Kind() engineio.EngineKind { return engineio.PISA }
func (engine Engine) Version() string           { return pisa.VersionWithMeta }

// SpawnInstance generates a new engineio.EngineInstance bootstrapped with some engineio.EngineFuel,
// an engineio.LogicDriver, its associated engineio.StateDriver and an engineio.EnvironmentDriver.
// The returned instance is configured with the base PISA instruction set and builtin primitives.
// Implements the engineio.Engine interface for pisa.Engine
func (engine Engine) SpawnInstance(
	logic engineio.LogicDriver,
	fuel engineio.EngineFuel,
	state engineio.StateDriver,
	env engineio.EnvironmentDriver,
	event engineio.EventDriver,
) (
	engineio.EngineInstance, error,
) {
	// Check logic driver engine
	if logic.Engine() != engineio.PISA {
		return nil, errors.New("incompatible logic driver: not a PISA logic")
	}

	// Check logic driver's logic ID and its context's logic ID.
	if logic.LogicID().String() != state.LogicID().String() {
		return nil, errors.New("incompatible context driver for logic: logic ID is not equal")
	}

	// Check the logic driver and context driver's addresses
	if logic.LogicID().Address() != state.Address() {
		return nil, errors.New("incompatible context driver for logic: address does not match")
	}

	return &Instance{
		logicIO: logic,
		internal: pisa.NewInstance(
			fuel, engine.library,
			Logic{logic},
			newState(state),
			env, engine.crypto,
			EventStream{event},
		),
	}, nil
}

// CompileManifest generates an engineio.LogicDescriptor from an engineio.Manifest object.
// The compilation will fail if the manifest object is not compatible or has malformed elements.
// Implements the engineio.Engine interface for pisa.Engine
func (engine Engine) CompileManifest(
	manifest engineio.Manifest,
	fuel engineio.EngineFuel,
) (
	engineio.LogicDescriptor,
	engineio.EngineFuel, error,
) {
	// Check that the Manifest Instance is PISA
	if manifest.Engine().Kind != engineio.PISA {
		return engineio.LogicDescriptor{}, 0, errors.New("invalid manifest: manifest engine is not PISA")
	}

	// Create a new manifest compiler
	compiler := NewManifestCompiler(fuel, manifest)
	// Compile the manifest
	descriptor, leftover, err := compiler.CompileDescriptor()
	if err != nil {
		return engineio.LogicDescriptor{}, fuel - leftover, errors.Wrap(err, "compile error")
	}

	return descriptor, fuel - leftover, nil
}

func (engine Engine) DecodeErrorResult(data []byte) (engineio.ErrorResult, error) {
	// Create a new Error object
	except := new(exception.Exception)
	if err := polo.Depolorize(except, data); err != nil {
		return nil, err
	}

	return Error{except: *except}, nil
}

// ValidateCalldata verifies the given engineio.IxnDriver for an engineio.LogicDriver.
// Returns an error for one of the following:
// * If the logic is not supported by PISA
// * If the callsite does not exist
// * If the callsite is not compatible with the ixn type
// * If the calldata is invalid for the callsite.
//
// Implements the engineio.Engine interface for pisa.Engine.
func (engine Engine) ValidateCalldata(logic engineio.LogicDriver, ixn engineio.InteractionDriver) error {
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
	case engineio.CallsiteInvokable:
		if ixn.Type() != common.IxLogicInvoke {
			return errors.Errorf("invalid callsite '%v' for IxnLogicInvoke", ixn.Callsite())
		}

	case engineio.CallsiteDeployer:
		if ixn.Type() != common.IxLogicDeploy {
			return errors.Errorf("invalid callsite '%v' for IxnLogicDeploy", ixn.Callsite())
		}

	case engineio.CallsiteInteractable, engineio.CallsiteEnlister:
		return errors.Errorf("unsupported callsite kind '%v' for callsite '%v'", callsite.Kind, ixn.Callsite())

	default:
		panic("invalid callsite kind variant detected")
	}

	element, ok := logic.GetElement(callsite.Ptr)
	if !ok {
		return errors.Errorf("could not fetch element for callsite '%v'", ixn.Callsite())
	}

	routine := new(pisa.Routine)
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
	if _, err := values.NewRegisterSet(routine.Inputs, calldata); err != nil {
		return errors.Errorf("invalid calldata for callsite '%v': %v", ixn.Callsite(), err)
	}

	return nil
}

func (engine Engine) GenerateManifestElement(kind engineio.ElementKind) (any, bool) {
	element, ok := ElementMetadata[kind]
	if !ok {
		return nil, false
	}

	return element.generator(), true
}

// GetCallEncoderFromLogicDriver generates an engineio.CallEncoder from a engineio.LogicDriver
// that can encode inputs and decode outputs for a given callsite pointer. This will fail
// if no valid callsite element exists for the given pointer in the given Manifest.
// Implements the engineio.Engine interface for pisa.Engine.
func (engine Engine) GetCallEncoderFromLogicDriver(
	driver engineio.LogicDriver,
	callsite engineio.Callsite,
) (
	engineio.CallEncoder, error,
) {
	// Check that the logic engine matches for the runtime
	if driver.Engine() != engine.Kind() {
		return nil, errors.New("incompatible logic for runtime: PISA")
	}

	return NewCallEncoder(Logic{driver}, callsite.Name)
}

func (engine Engine) GetCallEncoderFromManifest(
	manifest engineio.Manifest,
	callsite engineio.Callsite,
) (
	engineio.CallEncoder, error,
) {
	// TODO implement me
	panic("implement me")
}

package engineio

import (
	"context"
	"strings"
)

// EngineFuel is a measure of execution effort
type EngineFuel = uint64

// EngineKind is an enum with its variants
// representing the set of valid engines
type EngineKind int

func (kind EngineKind) String() string {
	switch kind {
	case PISA:
		return "PISA"
	case MERU:
		return "MERU"
	default:
		panic("unknown EngineKind variant")
	}
}

const (
	// InvalidEngine is a base reserved engine variant
	InvalidEngine EngineKind = iota

	// PISA is the EngineKind for the PISA VM Runtime.
	// The canonical implementation is available at https://github.com/sarvalabs/go-pisa
	PISA

	// MERU is the EngineKind for a hypothetical engine runtime that works as a WASM
	// environment that allows custom runtime implementations to run within it
	MERU
)

// Engine is the base definition for execution engine runtime. It is
// used for runtime level behavioural capabilities rather for logic execution.
//
// This can include:
//   - Compiling Manifest objects for the runtime into a LogicDriver
//   - Spawning execution EngineInstance for a specific LogicDriver
//   - Validating input calldata for a specific callsite on a LogicDriver
//   - Obtaining a calldata encoder for a specific callsite on a LogicDriver
type Engine interface {
	// Kind returns the kind of engine that the factory can produce
	Kind() EngineKind
	// Version returns the semver version string of the engine runtime
	Version() string

	// GetCallEncoderFromLogicDriver returns a CallEncoder object
	// for a given callsite element pointer from a LogicDriver object
	GetCallEncoderFromLogicDriver(LogicDriver, Callsite) (CallEncoder, error)
	// GetCallEncoderFromManifest returns a CallEncoder object
	// for a given callsite element pointer from a Manifest object
	GetCallEncoderFromManifest(Manifest, Callsite) (CallEncoder, error)

	// GenerateManifestElement returns an object for the given ElementKind
	// The object is used as a deserialization target when decoding a Manifest
	GenerateManifestElement(ElementKind) (any, bool)
	// CompileManifest generates a LogicDescriptor from a Manifest,
	// which can then be used to generate a Logic object.
	// The fuel spent during compile is returned with any potential error.
	CompileManifest(Manifest, EngineFuel) (LogicDescriptor, EngineFuel, error)

	// DecodeErrorResult decodes the given bytes into an
	// ErrorResult that is suitable for the engine runtime
	DecodeErrorResult([]byte) (ErrorResult, error)
	// ValidateCalldata verifies the calldata and callsite in an InteractionDriver.
	// The Logic must describe a callsite which accepts the calldata.
	ValidateCalldata(LogicDriver, InteractionDriver) error

	// SpawnInstance returns a new EngineInstance instance and initializes it with some
	// EngineFuel, a LogicDriver, the StateDriver associated with the logic and an EnvironmentDriver.
	// Will return an error if the Logic and its StateDriver do not match.
	SpawnInstance(LogicDriver, EngineFuel, StateDriver, EnvironmentDriver, EventDriver) (EngineInstance, error)
	SpawnDebugInstance(LogicDriver, EngineFuel, StateDriver, EnvironmentDriver, EventDriver) (DebugEngineInstance, error)
}

// EngineInstance is an execution engine runner with a specific EngineKind.
// A new EngineRunner instance can be spawned from an EngineRuntime with its
// Spawn method and is bound to a specific Logic and EnvironmentDriver.
//
// An Engine can be used to perform calls on its Logic with an InteractionDriver
// and some optional participants with their StateDriver objects.
type EngineInstance interface {
	// Kind returns the kind of engine
	Kind() EngineKind

	// Call calls a logical callsite on the Engine's Logic.
	// The callsite and input calldata are provided in the given InteractionDriver.
	// Optionally accepts some participant StateDriver objects based on the interaction type.
	Call(context.Context, InteractionDriver, StateDriver, ...StateDriver) (CallResult, error)
}

// registry is an in-memory registry of supported EngineRuntime instances.
// Support for different engine runtimes is only available if they are first registered with this package.
var registry = map[EngineKind]Engine{}

// RegisterEngine registers an EngineRuntime with the package.
// If a runtime instance already exists for the EngineKind, it is overwritten.
func RegisterEngine(runtime Engine) {
	registry[runtime.Kind()] = runtime
}

// FetchEngine retrieves an EngineRuntime for a given EngineKind.
// If the runtime for the engine kind is not registered, returns false.
func FetchEngine(kind EngineKind) (Engine, bool) {
	runtime, exists := registry[kind]
	if !exists {
		return nil, false
	}

	return runtime, true
}

func EngineKindFromString(str string) EngineKind {
	switch strings.ToUpper(str) {
	case "PISA":
		return PISA
	default:
		return InvalidEngine
	}
}

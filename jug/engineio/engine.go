package engineio

import (
	"context"
)

// EngineKind is an enum with its variants
// representing the types of execution engine
type EngineKind string

const (
	PISA EngineKind = "PISA"
	MERU EngineKind = "MERU"
)

var runtimeRegistry = map[EngineKind]EngineRuntime{}

// RegisterEngineRuntime registers an EngineRuntime with the package.
// It is indexed by the EngineKind returned by the Kind() method of the runtime.
// If a runtime instance already exists for the EngineKind, it is overwritten.
func RegisterEngineRuntime(runtime EngineRuntime) {
	runtimeRegistry[runtime.Kind()] = runtime
}

// FetchEngineRuntime retrieves an EngineRuntime for a given EngineKind.
// If the runtime for the engine kind is not registered, returns false.
func FetchEngineRuntime(kind EngineKind) (EngineRuntime, bool) {
	runtime, exists := runtimeRegistry[kind]

	return runtime, exists
}

// EngineRuntime is an interface that defines an engine runtime.
type EngineRuntime interface {
	// Kind returns the kind of engine that the factory can produce
	Kind() EngineKind

	// SpawnEngine returns a new Engine instance and initializes it with some
	// Fuel, a LogicDriver, the CtxDriver associated with the logic and an EnvDriver.
	// Will return an error if the LogicDriver and its CtxDriver do not match.
	SpawnEngine(Fuel, LogicDriver, CtxDriver, EnvDriver) (Engine, error)

	// CompileManifest generates a LogicDescriptor from a Manifest, which can then be used to generate
	// a LogicDriver object. The fuel spent during compile is returned with any potential error.
	CompileManifest(Fuel, *Manifest) (*LogicDescriptor, Fuel, error)

	// ValidateCalldata verifies the calldata and callsite in an IxnObject.
	// The LogicDriver must describe a callsite which accepts the calldata.
	ValidateCalldata(LogicDriver, *IxnObject) error

	// GetElementGenerator returns a generator function for an element schema with the
	// given ElementKind. Returns false, if no such element is defined by the runtime
	GetElementGenerator(ElementKind) (ManifestElementGenerator, bool)

	// GetCallEncoder returns a CallEncoder object for a given
	// callsite element pointer from a LogicDriver object
	GetCallEncoder(*Callsite, LogicDriver) (CallEncoder, error)
}

// Engine is an interface that defines an execution engine
type Engine interface {
	// Kind returns the kind of engine
	Kind() EngineKind

	// Call calls a logic function of a specified callsite kind.
	// The callsite and calldata are provided within the IxnObject.
	// Requires EngineDriver to be Bootstrapped with a Logic.
	Call(context.Context, *IxnObject, ...CtxDriver) (*CallResult, error)
}

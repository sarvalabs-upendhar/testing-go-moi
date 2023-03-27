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

// EngineFactory is an interface that defines an engine generator for different runtimes.
// This allows each engine implementation to define some base generation
// logic and reduce the overhead of creating multiple engine instances.
type EngineFactory interface {
	// Kind returns the kind of engine that the factory can produce
	Kind() EngineKind
	// NewEngine returns a new Engine
	NewEngine() Engine
}

// Engine is an interface that defines an execution engine
type Engine interface {
	// Kind returns the kind of engine
	Kind() EngineKind

	// Bootstrapped returns whether the engine has been bootstrapped
	Bootstrapped() bool
	// Bootstrap initializes the engine with some fuel, the LogicDriver, its CtxDriver and an EnvDriver.
	Bootstrap(context.Context, Fuel, LogicDriver, CtxDriver, EnvDriver) error

	// Compile generates a LogicDescriptor from a Manifest, which
	// can then be used to generate a LogicDriver object. The fuel
	// spent during compile is returned with any potential error.
	Compile(context.Context, Fuel, *Manifest) (*LogicDescriptor, Fuel, error)

	// Implements verifies that the LogicDriver implements the LogicImplSchema
	// Implements(context.Context, LogicDriver, LogicImplSchema) error

	// ValidateCall verifies the IxnDriver's calldata
	// for a callsite specified in the LogicDriver
	ValidateCall(context.Context, LogicDriver, *IxnObject) error

	// Call calls a logic function of a specified callsite kind.
	// The callsite and calldata are provided within the IxnObject.
	// Requires EngineDriver to be Bootstrapped with a Logic.
	Call(context.Context, CallsiteKind, *IxnObject, ...CtxDriver) *CallResult
}

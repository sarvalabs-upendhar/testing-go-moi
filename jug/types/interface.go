package types

import (
	"context"
	"strings"

	"github.com/sarvalabs/moichain/types"
)

// EngineFactory is an interface that defines an engine generator for different runtimes.
// This allows each engine implementation to define some base generation
// logic and reduce the overhead of creating multiple engine instances.
type EngineFactory interface {
	// Kind returns the kind of engine that the factory can produce
	Kind() types.LogicEngine
	// NewExecutionEngine returns a new ExecutionEngine with a fixed fuel cap.
	NewExecutionEngine(fuel uint64) ExecutionEngine
}

// ExecutionEngine is an interface that defines an execution environment
// for compiling Logic and executing its routines.
type ExecutionEngine interface {
	// Kind returns the kind of engine
	Kind() types.LogicEngine

	// Execute performs the execution of logic function on the LogicObject.
	// The callsite to execute and input data are provided as part of the ExecutionScope.
	// The output values along with any error (and error code) that may occur and the
	// execution effort (fuel consumption) are returned with the ExecutionResult.
	Execute(context.Context, Logic, *ExecutionOrder) *ExecutionResult

	// Compile will compile a logic manifest into a LogicDescriptor.
	// This LogicDescriptor can be used to generate a LogicObject.
	// The execution effort and any compile error are returned with the ExecutionResult
	Compile(context.Context, []byte) (*types.LogicDescriptor, *ExecutionResult)
}

// Logic is an interface that defines a logic driver for an ExecutionEngine
type Logic interface {
	IsSealed() bool

	HasPersistentState() bool
	HasEphemeralState() bool
	AllowsInteractions() bool

	LogicID() types.LogicID
	Engine() types.LogicEngine
	Manifest() types.Hash

	GetCallsite(name string) (types.LogicCallsite, bool)
	GetLogicElement(kind string, index uint64) (*types.LogicElement, bool)
}

// Storage is an interface that defines a storage driver for an ExecutionEngine
type Storage interface {
	Address() types.Address

	GetStorageEntry(types.LogicID, []byte) ([]byte, error)
	SetStorageEntry(types.LogicID, []byte, []byte) error

	// GetAssetBalance(types.AssetID) (*big.Int, error)
	// AddAssetBalance(types.AssetID, *big.Int) error
	// SubAssetBalance(types.AssetID, *big.Int) error

	// GetAssetApproval(types.AssetID, types.Address) (*big.Int, error)
	// SetAssetApproval(types.AssetID, types.Address, *big.Int) error
}

// ManifestHeader represents a simple header for a Manifest and describes its syntax form
// and engine mode. Useful for determining which engine to use to handle the Manifest.
// Every engine's manifest implementation must be able to decode into this header.
type ManifestHeader struct {
	Syntax string `polo:"syntax" yaml:"syntax" json:"syntax"`
	Engine string `polo:"engine" yaml:"engine" json:"engine"`
}

// LogicEngine returns the normalized form of the logic engine value in the ManifestHeader.
// It is capitalized to uppercase letter and converted into a types.LogicEngine
func (header ManifestHeader) LogicEngine() types.LogicEngine {
	return types.LogicEngine(strings.ToUpper(header.Engine))
}

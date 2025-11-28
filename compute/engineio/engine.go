package engineio

import (
	"math/big"
	"strings"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
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

type Cryptography interface {
	// IsSignature returns whether the given signature is in a supported format.
	// This function provides no indication on the authenticity of the signature for some message.
	IsSignature(signature []byte) bool

	// AuthenticateSignature returns whether the given signature is valid for the given message and public key.
	// Errors if the signature or the public key formats are unsupported.
	AuthenticateSignature(data, signature, publicKey []byte) (bool, error)
}

// Engine is the base definition for execution engine runtime. It is
// used for runtime level behavioural capabilities rather for logic execution.
//
// This can include:
//   - Compiling Manifest objects for the runtime
//   - Creating Runtime instances that can execute logic
type Engine interface {
	// Kind returns the kind of engine that the factory can produce
	Kind() EngineKind
	// Version returns the semver version string of the engine runtime
	Version() string

	Runtime(timestamp uint64) Runtime

	CompileManifest(
		manifestKind ManifestKind,
		logicID identifiers.Identifier,
		manifest Manifest,
		fuel FuelGauge,
	) (
		[]byte,
		*FuelGauge,
		error,
	)

	GenerateManifestElement(kind ElementKind) (any, bool)
}

type Runtime interface {
	SpawnLogic(logicID [32]byte, artifact []byte, storage Storage, params map[string][]byte) error
	CreateAsset(
		ixHash common.Hash,
		assetID identifiers.AssetID,
		symbol string, decimals uint8, dimension uint8,
		manager, creator identifiers.Identifier,
		maxSupply *big.Int, staticMetadata, dynamicMetadata map[string][]byte,
		enableEvents bool, logicID identifiers.LogicID,
	) (uint64, error)
	ActorExists(logicID [32]byte) bool
	CreateActor(id [32]byte, storage Storage, params map[string][]byte) error
	Call(logicID [32]byte, action Action, transition Transition, limit *FuelGauge) *CallResult
	BindAssetEngine(ae AssetEngine)
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

type Transition interface {
	GetLogicStorageObject(logicID identifiers.Identifier) (Storage, error)
}

func EngineKindFromString(str string) EngineKind {
	switch strings.ToUpper(str) {
	case "PISA":
		return PISA
	default:
		return InvalidEngine
	}
}

type RuntimeContext struct {
	ClusterContext *common.ExecutionContext
	Runtime        Runtime
}

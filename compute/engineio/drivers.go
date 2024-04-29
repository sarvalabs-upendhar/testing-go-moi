package engineio

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
)

// CryptographyDriver represents an interface for cryptographic operations.
// It can be used to validate signature formats and verify them for a public key.
// This interfaces allows us to pass the capabilities of go-moi's crypto package to different engine runtimes.
type CryptographyDriver interface {
	ValidateSignature(sig []byte) bool
	VerifySignature(data, sig, pub []byte) (bool, error)
}

// InteractionDriver represents a driver for interaction information.
// It describes the callsite and input calldata for execution calls along with
// other information such as the Interaction's fuel parameters or transfer funds.
type InteractionDriver interface {
	Type() common.IxType
	// Supports(Callsite) bool

	FuelPrice() *big.Int
	FuelLimit() uint64

	Callsite() string
	Calldata() []byte
}

func NewDebugIxnDriver(
	t *testing.T, kind common.IxType,
	callsite string, calldata []byte,
	limit uint64, price *big.Int,
) InteractionDriver {
	t.Helper()

	return debugIxnDriver{
		kind:     kind,
		price:    price,
		limit:    limit,
		callsite: callsite,
		calldata: calldata,
	}
}

type debugIxnDriver struct {
	kind     common.IxType
	price    *big.Int
	limit    uint64
	callsite string
	calldata []byte
}

func (ixn debugIxnDriver) Type() common.IxType { return ixn.kind }
func (ixn debugIxnDriver) FuelPrice() *big.Int { return ixn.price }
func (ixn debugIxnDriver) FuelLimit() uint64   { return ixn.limit }
func (ixn debugIxnDriver) Callsite() string    { return ixn.callsite }
func (ixn debugIxnDriver) Calldata() []byte    { return ixn.calldata }

// EnvironmentDriver represents a driver for environmental information.
// It describes information about the execution context such
// as the consensus cluster ID or execution timestamp.
type EnvironmentDriver interface {
	Timestamp() uint64
	ClusterID() string
}

func NewDebugEnvDriver(t *testing.T, timestamp uint64, clusterID string) debugEnvDriver {
	t.Helper()

	return debugEnvDriver{
		timestamp: 0,
		clusterID: "",
	}
}

type debugEnvDriver struct {
	timestamp uint64
	clusterID string
}

func (env debugEnvDriver) Timestamp() uint64 { return env.timestamp }
func (env debugEnvDriver) ClusterID() string { return env.clusterID }

package engineio

import (
	"math/big"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
)

// StateDriver represents an interface for accessing and manipulating state information of an account.
// It is bounded to a particular account and can only mutate within applicable portions
// of the state within the bounds of the logic's namespace
type StateDriver interface {
	Address() identifiers.Address
	LogicID() identifiers.LogicID

	GetStorageEntry([]byte) ([]byte, error)
	SetStorageEntry([]byte, []byte) error
}

// StorageReader is an interface that allows reading storage data
type StorageReader interface {
	GetStorageEntry([]byte) ([]byte, error)
}

// IxDriver represents a driver for interaction information.
// It describes the callsite and input calldata for execution calls along with
// other information such as the Interactions fuel parameters or transfer funds.
type IxDriver interface {
	Type() common.IxOpType
	Hash() common.Hash

	FuelPrice() *big.Int
	FuelLimit() uint64

	Callsite() string
	Calldata() []byte
}

// EventDriver represents a driver for event collection.
// It describes methods to fetch events from the event stream along
// with methods to emit events to the stream and reset the stream.
type EventDriver interface {
	Logic() identifiers.LogicID
	Count() uint64
	Reset()

	Fetch(uint64) (common.Log, bool)
	Insert(common.Log)

	Collect() []common.Log
	Iterate() <-chan common.Log
}

// EnvironmentDriver represents a driver for environmental information.
// It describes information about the execution context such
// as the consensus cluster ID or execution timestamp.
type EnvironmentDriver interface {
	Timestamp() uint64
	ClusterID() string
}

// CryptographyDriver represents an interface for cryptographic operations.
// It can be used to validate signature formats and verify them for a public key.
// This interfaces allows us to pass the capabilities of go-moi's crypto package to different engine runtimes.
type CryptographyDriver interface {
	ValidateSignature(sig []byte) bool
	VerifySignature(data, sig, pub []byte) (bool, error)
}

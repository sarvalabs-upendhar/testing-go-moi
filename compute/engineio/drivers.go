package engineio

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

// IxnDriver represents an interface that exposes access to interaction
// information such as the fuel price, fuel limit and interaction type
// along with capabilities to make external logic invocations
type IxnDriver interface {
	Type() common.IxType

	FuelPrice() *big.Int
	FuelLimit() *big.Int

	Callsite() string
	Calldata() []byte
}

// EnvDriver represents an interface that exposes access to environment
// information such as the cluster data, timestamps and fuel prices
// along with capabilities to make external logic invocations
type EnvDriver interface {
	Timestamp() int64
	ClusterID() string
}

// DepDriver represents an interface for an
// engine's element dependency manager.
type DepDriver interface {
	fmt.Stringer
	json.Marshaler
	json.Unmarshaler

	polo.Polorizable
	polo.Depolorizable

	Insert(uint64, ...uint64)
	Remove(uint64)

	Size() uint64
	Iter() <-chan uint64
	Contains(uint64) bool
	Edges(uint64) []uint64
	Dependencies(uint64) []uint64
}

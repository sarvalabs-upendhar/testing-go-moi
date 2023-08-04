package engineio

import (
	"math/big"

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

package compute

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
)

type envObject struct {
	clusterID string
	timestamp int64
}

func (env envObject) Timestamp() int64  { return env.timestamp }
func (env envObject) ClusterID() string { return env.clusterID }

type ixnObject struct {
	kind     common.IxType
	price    *big.Int
	limit    *big.Int
	callsite string
	calldata []byte
}

func (ixn ixnObject) Type() common.IxType { return ixn.kind }
func (ixn ixnObject) FuelPrice() *big.Int { return ixn.price }
func (ixn ixnObject) FuelLimit() *big.Int { return ixn.limit }
func (ixn ixnObject) Callsite() string    { return ixn.callsite }
func (ixn ixnObject) Calldata() []byte    { return ixn.calldata }

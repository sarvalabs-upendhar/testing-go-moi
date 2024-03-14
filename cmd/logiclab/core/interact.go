package core

import (
	"math/big"

	"github.com/sarvalabs/go-moi-engineio"

	"github.com/sarvalabs/go-moi/common"
)

type LogicInteraction struct {
	Kind  common.IxType
	Price *big.Int
	Limit uint64
	Site  string
	Call  []byte
}

func (ixn LogicInteraction) IxnType() engineio.IxnType { return ixn.Kind }
func (ixn LogicInteraction) FuelPrice() *big.Int       { return ixn.Price }
func (ixn LogicInteraction) FuelLimit() uint64         { return ixn.Limit }
func (ixn LogicInteraction) Callsite() string          { return ixn.Site }
func (ixn LogicInteraction) Calldata() []byte          { return ixn.Call }

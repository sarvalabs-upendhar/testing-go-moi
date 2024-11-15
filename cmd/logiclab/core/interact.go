package core

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
)

type Interaction struct {
	Kind  common.IxOpType
	Nonce uint64
	Price *big.Int
	Limit uint64
	Site  string
	Call  []byte
}

func (ixn Interaction) Type() common.IxOpType { return ixn.Kind }
func (ixn Interaction) FuelPrice() *big.Int   { return ixn.Price }
func (ixn Interaction) FuelLimit() uint64     { return ixn.Limit }
func (ixn Interaction) Callsite() string      { return ixn.Site }
func (ixn Interaction) Calldata() []byte      { return ixn.Call }
func (ixn Interaction) Hash() common.Hash {
	hash, err := common.PoloHash(ixn)
	if err != nil {
		panic("ixn hash generation failed")
	}

	return hash
}

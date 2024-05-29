package core

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
)

type LogicInteraction struct {
	Kind  common.IxType
	Nonce uint64
	Price *big.Int
	Limit uint64
	Site  string
	Call  []byte
}

func (ixn LogicInteraction) Type() common.IxType { return ixn.Kind }
func (ixn LogicInteraction) FuelPrice() *big.Int { return ixn.Price }
func (ixn LogicInteraction) FuelLimit() uint64   { return ixn.Limit }
func (ixn LogicInteraction) Callsite() string    { return ixn.Site }
func (ixn LogicInteraction) Calldata() []byte    { return ixn.Call }
func (ixn LogicInteraction) Hash() (common.Hash, error) {
	hash, err := common.PoloHash(ixn)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to polorize logic interaction")
	}

	return hash, nil
}

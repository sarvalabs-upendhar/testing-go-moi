package core

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
)

type Interaction struct {
	Kind  common.IxType
	Nonce uint64
	Price *big.Int
	Limit uint64
	Site  string
	Call  []byte
}

func (ixn Interaction) Type() common.IxType { return ixn.Kind }
func (ixn Interaction) FuelPrice() *big.Int { return ixn.Price }
func (ixn Interaction) FuelLimit() uint64   { return ixn.Limit }
func (ixn Interaction) Callsite() string    { return ixn.Site }
func (ixn Interaction) Calldata() []byte    { return ixn.Call }
func (ixn Interaction) Hash() (common.Hash, error) {
	hash, err := common.PoloHash(ixn)
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to polorize logic interaction")
	}

	return hash, nil
}

package types

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

type BalanceObject struct {
	Balances types.AssetMap
	PrvHash  types.Hash
}

func (b *BalanceObject) TDU() (types.AssetMap, types.Hash) {
	return b.Balances, b.PrvHash
}

func (b *BalanceObject) Copy() *BalanceObject {
	newObject := new(BalanceObject)
	newObject.PrvHash = b.PrvHash

	newObject.Balances = make(types.AssetMap)
	for k, v := range b.Balances {
		newObject.Balances[k] = new(big.Int).SetBytes(v.Bytes())
	}

	return newObject
}

func (b *BalanceObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(b)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize balance object")
	}

	return rawData, nil
}

func (b *BalanceObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(b, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize balance object")
	}

	return nil
}

type ApprovalObject struct {
	Approvals map[types.Address]types.AssetMap
	PrvHash   types.Hash
}

func (a *ApprovalObject) Copy() *ApprovalObject {
	newObject := new(ApprovalObject)
	newObject.PrvHash = a.PrvHash
	newObject.Approvals = make(map[types.Address]types.AssetMap)

	for k, v := range a.Approvals {
		newObject.Approvals[k] = v.Copy()
	}

	return newObject
}

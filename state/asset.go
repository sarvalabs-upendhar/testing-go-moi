package state

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type BalanceObject struct {
	AssetMap common.AssetMap
	PrvHash  common.Hash
}

func (balance *BalanceObject) TDU() (common.AssetMap, common.Hash) {
	return balance.AssetMap, balance.PrvHash
}

func (balance *BalanceObject) Copy() *BalanceObject {
	newObject := new(BalanceObject)
	if !balance.PrvHash.IsNil() {
		newObject.PrvHash = balance.PrvHash
	}

	newObject.AssetMap = make(common.AssetMap)
	for k, v := range balance.AssetMap {
		newObject.AssetMap[k] = new(big.Int).SetBytes(v.Bytes())
	}

	return newObject
}

func (balance *BalanceObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(balance)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize balance object")
	}

	return rawData, nil
}

func (balance *BalanceObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(balance, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize balance object")
	}

	return nil
}

type ApprovalObject struct {
	Approvals map[common.Address]common.AssetMap
	PrvHash   common.Hash
}

func (a *ApprovalObject) Copy() *ApprovalObject {
	newObject := new(ApprovalObject)
	newObject.PrvHash = a.PrvHash
	newObject.Approvals = make(map[common.Address]common.AssetMap)

	for k, v := range a.Approvals {
		newObject.Approvals[k] = v.Copy()
	}

	return newObject
}

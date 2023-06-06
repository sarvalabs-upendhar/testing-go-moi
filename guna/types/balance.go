package types

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

type BalanceObject struct {
	AssetMap types.AssetMap
	PrvHash  types.Hash
}

type RegistryObject struct {
	Entries map[string][]byte
}

func (r *RegistryObject) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize registry object")
	}

	return rawData, nil
}

func (r *RegistryObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(r, bytes); err != nil {
		return errors.Wrap(err, "failed to polorize registry object")
	}

	return nil
}

func (r *RegistryObject) Copy() *RegistryObject {
	newObject := &RegistryObject{
		Entries: make(map[string][]byte, len(r.Entries)),
	}

	for k, v := range r.Entries {
		newObject.Entries[k] = make([]byte, len(v))
		copy(newObject.Entries[k], v)
	}

	return r
}

func (b *BalanceObject) TDU() (types.AssetMap, types.Hash) {
	return b.AssetMap, b.PrvHash
}

func (b *BalanceObject) Copy() *BalanceObject {
	newObject := new(BalanceObject)
	if !b.PrvHash.IsNil() {
		newObject.PrvHash = b.PrvHash
	}

	newObject.AssetMap = make(types.AssetMap)
	for k, v := range b.AssetMap {
		newObject.AssetMap[k] = new(big.Int).SetBytes(v.Bytes())
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

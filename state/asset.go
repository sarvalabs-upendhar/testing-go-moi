package state

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

// AssetObject represents an asset's state, including balance, deposits, mandates, and properties.
type AssetObject struct {
	Balance    *big.Int
	Deposit    map[identifiers.LogicID]*big.Int
	Mandate    map[identifiers.Address]*big.Int
	Properties *common.AssetDescriptor
}

// NewAssetObject initializes a new AssetObject with the given balance and properties.
func NewAssetObject(balance *big.Int, properties *common.AssetDescriptor) *AssetObject {
	return &AssetObject{
		Balance:    balance,
		Deposit:    make(map[identifiers.LogicID]*big.Int),
		Mandate:    make(map[identifiers.Address]*big.Int),
		Properties: properties,
	}
}

// Bytes serializes the AssetObject into bytes.
func (ao *AssetObject) Bytes() ([]byte, error) {
	data, err := polo.Polorize(ao)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset object")
	}

	return data, err
}

// FromBytes deserializes the AssetObject from bytes.
func (ao *AssetObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ao, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize asset object")
	}

	return nil
}

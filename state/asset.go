package state

import (
	"math/big"
	"strconv"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

const AssetLogicPrefix = "Asset_Logic_"

type MetaData struct {
	static  map[string][]byte
	dynamic map[string][]byte
}

// AssetObject represents an asset's state, including balance, lockups, mandates, and properties.
type AssetObject struct {
	Balance       map[common.TokenID]*big.Int
	TokenMetaData map[common.TokenID]*MetaData
	Lockup        map[identifiers.Identifier]map[common.TokenID]*common.AmountWithExpiry
	Mandate       map[identifiers.Identifier]map[common.TokenID]*common.AmountWithExpiry
	Properties    *common.AssetDescriptor
}

// NewAssetObject initializes a new AssetObject with the given balance and properties.
func NewAssetObject(properties *common.AssetDescriptor) *AssetObject {
	return &AssetObject{
		Balance:       make(map[common.TokenID]*big.Int),
		TokenMetaData: make(map[common.TokenID]*MetaData),
		Lockup:        make(map[identifiers.Identifier]map[common.TokenID]*common.AmountWithExpiry),
		Mandate:       make(map[identifiers.Identifier]map[common.TokenID]*common.AmountWithExpiry),
		Properties:    properties,
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

func (ao *AssetObject) hasTokenID(tokenID common.TokenID) bool {
	if _, ok := ao.Balance[tokenID]; ok {
		return true
	}

	for _, tokens := range ao.Lockup {
		if _, hasTokenID := tokens[tokenID]; hasTokenID {
			return true
		}
	}

	return false
}

func (ao *AssetObject) deleteTokenMetadata(tokenID common.TokenID) {
	if !ao.hasTokenID(tokenID) {
		delete(ao.TokenMetaData, tokenID)
	}
}

func (ao *AssetObject) GetBalance(tokenID common.TokenID) *big.Int {
	bal, ok := ao.Balance[tokenID]
	if !ok {
		return big.NewInt(0)
	}

	return bal
}

func (ao *AssetObject) HasBalance(tokenID common.TokenID, amount *big.Int) error {
	bal, ok := ao.Balance[tokenID]
	if !ok {
		return common.ErrTokenNotFound
	}

	// Check if sender has sufficient balance
	if bal.Cmp(amount) == -1 {
		return common.ErrInsufficientFunds
	}

	return nil
}

func (ao *AssetObject) updateStaticMetadata(key string, value []byte) error {
	if ao.Properties.StaticMetaData == nil {
		ao.Properties.StaticMetaData = make(map[string][]byte)
	}

	if _, exists := ao.Properties.StaticMetaData[key]; exists {
		return common.ErrKeyExists
	}

	ao.Properties.StaticMetaData[key] = value

	return nil
}

func (ao *AssetObject) updateDynamicMetadata(key string, value []byte) {
	if ao.Properties.DynamicMetaData == nil {
		ao.Properties.DynamicMetaData = make(map[string][]byte)
	}

	ao.Properties.DynamicMetaData[key] = value
}

func (ao *AssetObject) updateStaticTokenMetaData(tokenID common.TokenID, key string, value []byte) error {
	if _, ok := ao.TokenMetaData[tokenID].static[key]; ok {
		return common.ErrKeyExists
	}

	ao.TokenMetaData[tokenID].static[key] = value

	return nil
}

func (ao *AssetObject) updateDynamicTokenMetaData(tokenID common.TokenID, key string, value []byte) {
	ao.TokenMetaData[tokenID].dynamic[key] = value
}

func AssetLogicKey(standard common.AssetStandard) []byte {
	str := AssetLogicPrefix + strconv.FormatUint(uint64(standard), 10)

	return []byte(str)
}

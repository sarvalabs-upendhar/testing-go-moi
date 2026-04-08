package engineio

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type AssetEngine interface {
	// Asset property getters
	BalanceOf(
		address identifiers.Identifier,
		assetID identifiers.AssetID, tokenID common.TokenID, access map[[32]byte]int) (*big.Int, error)
	Symbol(assetID identifiers.AssetID, access map[[32]byte]int) (string, error)
	Creator(assetID identifiers.AssetID, access map[[32]byte]int) (identifiers.Identifier, error)
	Manager(assetID identifiers.AssetID, access map[[32]byte]int) (identifiers.Identifier, error)
	Decimals(assetID identifiers.AssetID, access map[[32]byte]int) (uint8, error)
	MaxSupply(assetID identifiers.AssetID, access map[[32]byte]int) (*big.Int, error)
	CirculatingSupply(assetID identifiers.AssetID, access map[[32]byte]int) (*big.Int, error)
	LogicID(assetID identifiers.AssetID, access map[[32]byte]int) (identifiers.LogicID, error)
	EnableEvents(assetID identifiers.AssetID, access map[[32]byte]int) (bool, error)

	// Asset metadata operations
	SetStaticMetaData(
		assetID identifiers.AssetID,
		participantID identifiers.Identifier,
		key string, val []byte,
		access map[[32]byte]int,
	) error
	SetDynamicMetaData(
		assetID identifiers.AssetID,
		participantID identifiers.Identifier,
		key string, val []byte,
		access map[[32]byte]int,
	) error

	GetStaticMetaData(
		assetID identifiers.AssetID,
		key string,
		access map[[32]byte]int,
	) ([]byte, error)
	GetDynamicMetaData(
		assetID identifiers.AssetID,
		key string,
		access map[[32]byte]int,
	) ([]byte, error)

	SetStaticTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier, tokenID common.TokenID,
		key string, val []byte, access map[[32]byte]int) error
	SetDynamicTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier, tokenID common.TokenID,
		key string, val []byte, access map[[32]byte]int) error

	GetStaticTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
		tokenID common.TokenID, key string, access map[[32]byte]int) ([]byte, error)
	GetDynamicTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
		tokenID common.TokenID, key string, access map[[32]byte]int) ([]byte, error)

	// Asset lifecycle operations
	CreateAsset(ixHash common.Hash,
		assetID identifiers.AssetID, symbol string, decimals uint8, dimension uint8,
		manager identifiers.Identifier, creator identifiers.Identifier, maxSupply *big.Int,
		staticMetadata, dynamicMetadata map[string][]byte,
		enableEvents bool, logicID identifiers.LogicID, access map[[32]byte]int) (uint64, error)
	Transfer(assetID identifiers.AssetID, tokenID common.TokenID,
		operatorID, benefactorID, beneficiaryID identifiers.Identifier,
		amount *big.Int, access map[[32]byte]int) (uint64, error)
	Mint(assetID identifiers.AssetID, tokenID common.TokenID, senderID, beneficiaryID identifiers.Identifier,
		amount *big.Int, staticMetadata map[string][]byte, access map[[32]byte]int) (uint64, error)
	Burn(assetID identifiers.AssetID, tokenID common.TokenID,
		benefactorID identifiers.Identifier, amount *big.Int, access map[[32]byte]int) (uint64, error)

	// Asset authorization operations
	Approve(assetID identifiers.AssetID, tokenID common.TokenID,
		benefactorID, beneficiaryID identifiers.Identifier,
		amount *big.Int, expiresAt uint64, access map[[32]byte]int) (uint64, error)
	Revoke(assetID identifiers.AssetID, tokenID common.TokenID,
		benefactorID, beneficiaryID identifiers.Identifier, access map[[32]byte]int) (uint64, error)

	// Asset lockup operations
	Lockup(assetID identifiers.AssetID, tokenID common.TokenID,
		benefactorID, beneficiaryID identifiers.Identifier, amount *big.Int, access map[[32]byte]int) (uint64, error)
	Release(
		assetID identifiers.AssetID,
		tokenID common.TokenID, operatorID, benefactorID, beneficiaryID identifiers.Identifier,
		amount *big.Int, access map[[32]byte]int) (uint64, error)
}

package api

import (
	"math/big"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/guna"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type IxPool interface {
	AddInteractions(ixs types.Interactions) []error
	GetNonce(addr types.Address) (uint64, error)
}

type ChainManager interface {
	GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error)
	GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
	GetReceiptByIxHash(addr types.Address, ixHash types.Hash) (*types.Receipt, error)
	GetTesseractByHeight(address types.Address, height uint64, withInteractions bool) (*types.Tesseract, error)
	GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetObject, error)
}

type StateManager interface {
	GetLatestStateObject(addr types.Address) (*guna.StateObject, error)
	GetContextByHash(address types.Address, hash types.Hash) (types.Hash, []id.KramaID, []id.KramaID, error)
	GetBalances(addrs types.Address) (*gtypes.BalanceObject, error)
	GetBalance(addr types.Address, assetID types.AssetID) (*big.Int, error)
	GetLatestNonce(addr types.Address) (uint64, error)
	GetStorageEntry(logicID types.LogicID, slot []byte) ([]byte, error)
	GetAccountState(addr types.Address, stateHash types.Hash) (*types.Account, error)
	GetLogicManifest(logicID types.LogicID) ([]byte, error)
}

// Backend is a struct that represents the API backend
type Backend struct {
	// Represents the API interaction pool
	ixpool IxPool

	// Represents the API chain manager
	chain ChainManager

	// Represents the API state manager
	sm StateManager

	// Represents the node config
	cfg *common.IxPoolConfig
}

// NewBackend is a constructor function that generates and returns a new API Backend object
func NewBackend(ixpool IxPool, chain ChainManager, sm StateManager, cfg *common.IxPoolConfig) *Backend {
	// Create a new API Backend object and return it
	return &Backend{ixpool, chain, sm, cfg}
}

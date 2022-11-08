package api

import (
	"math/big"

	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/guna"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/types"
)

type IxPool interface {
	AddInteractions(ixs types.Interactions) []error
	GetNonce(addr types.Address) (uint64, error)
}

type ChainManager interface {
	GetLatestTesseract(addr types.Address) (*types.Tesseract, error)
	GetTesseract(hash types.Hash) (*types.Tesseract, error)
	GetReceipt(addr types.Address, ixHash types.Hash) (*types.Receipt, error)
	GetTesseractByHeight(address string, height uint64) (*types.Tesseract, error)
	GetAssetDataByAssetHash(assetHash []byte) (*types.AssetData, error)
}

type StateManager interface {
	GetLatestStateObject(addr types.Address) (*guna.StateObject, error)
	GetContextByHash(address types.Address, hash types.Hash) (types.Hash, []id.KramaID, []id.KramaID, error)
	GetBalances(addrs types.Address) (*types.BalanceObject, error)
	GetBalance(addr types.Address, assetID types.AssetID) (*big.Int, error)
	GetLatestNonce(addr types.Address) (uint64, error)
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

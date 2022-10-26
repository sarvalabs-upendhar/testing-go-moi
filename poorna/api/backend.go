package api

import (
	"math/big"

	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/guna"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type IxPool interface {
	AddInteractions(ixs ktypes.Interactions) []error
	GetNonce(addr ktypes.Address) (uint64, error)
}

type ChainManager interface {
	GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error)
	GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error)
	GetReceipt(addr ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error)
	GetTesseractByHeight(address string, height uint64) (*ktypes.Tesseract, error)
	GetAssetDataByAssetHash(assetHash []byte) (*ktypes.AssetData, error)
}

type StateManager interface {
	GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error)
	GetContextByHash(address ktypes.Address, hash ktypes.Hash) (ktypes.Hash, []id.KramaID, []id.KramaID, error)
	GetBalances(addrs ktypes.Address) (*ktypes.BalanceObject, error)
	GetBalance(addr ktypes.Address, assetID ktypes.AssetID) (*big.Int, error)
	GetLatestNonce(addr ktypes.Address) (uint64, error)
	IsGenesis(addr ktypes.Address) (bool, error)
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

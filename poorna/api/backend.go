package api

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/guna"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type ChainManager interface {
	GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error)
	GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error)
	GetReceipt(addr ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error)
	GetTesseractByHeight(address string, height uint64) (*ktypes.Tesseract, error)
	GetAssetDataByAssetHash(assetHash []byte) (*ktypes.AssetData, error)
}

type StateManager interface {
	GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error)
	GetLatestContext(address ktypes.Address) (ktypes.Hash, []id.KramaID, []id.KramaID, error)
	GetBalances(addrs ktypes.Address) (*ktypes.BalanceObject, error)
	GetLatestNonce(addr ktypes.Address) (uint64, error)
}

// Backend is a struct that represents the API backend
type Backend struct {
	// Represents the API interaction pool
	ixpool *ixpool.IxPool

	// Represents the API chain manager
	chain ChainManager

	// Represents the API state manager
	sm StateManager
}

// NewBackend is a constructor function that generates and returns a new API Backend object
func NewBackend(ixpool *ixpool.IxPool, chain ChainManager, sm *guna.StateManager) *Backend {
	// Create a new API Backend object and return it
	return &Backend{ixpool, chain, sm}
}

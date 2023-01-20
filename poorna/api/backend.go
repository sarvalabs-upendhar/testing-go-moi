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
	GetIxs(addr types.Address, inclQueued bool) (promoted, enqueued []*types.Interaction)
	GetAllIxs(inclQueued bool) (allPromoted, allEnqueued map[types.Address][]*types.Interaction)
	GetAccountWaitTime(addr types.Address) (int64, error)
	GetAllAccountsWaitTime() map[types.Address]int64
}

type ChainManager interface {
	GetLatestTesseract(addr types.Address, withInteractions bool) (*types.Tesseract, error)
	GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
	GetReceiptByIxHash(ixHash types.Hash) (*types.Receipt, error)
	GetTesseractByHeight(address types.Address, height uint64, withInteractions bool) (*types.Tesseract, error)
	GetAssetDataByAssetHash(assetHash []byte) (*gtypes.AssetObject, error)
}

type StateManager interface {
	GetLatestStateObject(addr types.Address) (*guna.StateObject, error)
	GetContextByHash(address types.Address, hash types.Hash) (types.Hash, []id.KramaID, []id.KramaID, error)
	GetBalances(addrs types.Address, stateHash types.Hash) (*gtypes.BalanceObject, error)
	GetBalance(addr types.Address, assetID types.AssetID, stateHash types.Hash) (*big.Int, error)
	GetNonce(addr types.Address, stateHash types.Hash) (uint64, error)
	GetAccountState(addr types.Address, stateHash types.Hash) (*types.Account, error)
	GetLogicManifest(logicID types.LogicID, stateHash types.Hash) ([]byte, error)
	GetStorageEntry(logicID types.LogicID, slot []byte, stateHash types.Hash) ([]byte, error)
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

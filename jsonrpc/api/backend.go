package api

import (
	"math/big"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/state"
)

type IxPool interface {
	AddInteractions(ixs common.Interactions) []error
	GetNonce(addr common.Address) (uint64, error)
	GetIxs(addr common.Address, inclQueued bool) (promoted, enqueued []*common.Interaction)
	GetAllIxs(inclQueued bool) (allPromoted, allEnqueued map[common.Address][]*common.Interaction)
	GetPendingIx(ixHash common.Hash) (*common.Interaction, bool)
	GetAccountWaitTime(addr common.Address) (*big.Int, error)
	GetAllAccountsWaitTime() map[common.Address]*big.Int
}

type ChainManager interface {
	GetLatestTesseract(addr common.Address, withInteractions bool) (*common.Tesseract, error)
	GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error)
	GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error)
	GetInteractionAndPartsByIxHash(ixHash common.Hash) (*common.Interaction, *common.TesseractParts, int, error)
	GetInteractionAndPartsByTSHash(tsHash common.Hash, ixIndex int) (*common.Interaction, *common.TesseractParts, error)
	GetTesseractHeightEntry(address common.Address, height uint64) (common.Hash, error)
}

type StateManager interface {
	GetLatestStateObject(addr common.Address) (*state.Object, error)
	GetContextByHash(address common.Address, hash common.Hash) (common.Hash, []id.KramaID, []id.KramaID, error)
	GetBalances(addrs common.Address, stateHash common.Hash) (*state.BalanceObject, error)
	GetBalance(addr common.Address, assetID common.AssetID, stateHash common.Hash) (*big.Int, error)
	GetNonce(addr common.Address, stateHash common.Hash) (uint64, error)
	GetAccountState(addr common.Address, stateHash common.Hash) (*common.Account, error)
	GetLogicManifest(logicID common.LogicID, stateHash common.Hash) ([]byte, error)
	GetStorageEntry(logicID common.LogicID, slot []byte, stateHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(addr common.Address) (*common.AccountMetaInfo, error)
	GetLogicIDs(addr common.Address, stateHash common.Hash) ([]common.LogicID, error)
	GetAssetInfo(assetID common.AssetID, stateHash common.Hash) (*common.AssetDescriptor, error)
	GetRegistry(addr common.Address, stateHash common.Hash) (map[string][]byte, error)
}

type ExecutionManager interface {
	InteractionCall(ix *common.Interaction) (*common.Receipt, error)
}

type Network interface {
	GetPeers() []id.KramaID
	GetVersion() string
	GetKramaID() id.KramaID
	GetConns() []network.Conn
}

type DB interface {
	ReadEntry(key []byte) ([]byte, error)
	GetRegisteredAccounts() ([]common.Address, error)
}

// Backend is a struct that represents the API backend
type Backend struct {
	// Represents the API interaction pool
	ixpool IxPool
	// Represents the API chain manager
	chain ChainManager
	// Represents the API execution manager
	exec ExecutionManager
	// Represents the API state manager
	sm StateManager
	// Represents the API network
	net Network
	// Represents the API database
	db DB
	// Represents the node config
	cfg *config.IxPoolConfig
}

// NewBackend is a constructor function that generates and returns a new API Backend object
func NewBackend(
	ixpool IxPool,
	chain ChainManager,
	exec ExecutionManager,
	sm StateManager,
	net Network,
	db DB,
	cfg *config.IxPoolConfig,
) *Backend {
	// Create a new API Backend object and return it
	return &Backend{ixpool, chain, exec, sm, net, db, cfg}
}

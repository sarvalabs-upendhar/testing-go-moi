package backend

import (
	"context"
	"math/big"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/state"
)

type IxPool interface {
	AddLocalInteractions(ixs common.Interactions) []error
	GetSequenceID(addr identifiers.Address, keyID uint64) (uint64, error)
	GetIxs(addr identifiers.Address, inclQueued bool) (promoted, enqueued []*common.Interaction)
	GetAllIxs(inclQueued bool) (allPromoted, allEnqueued map[identifiers.Address][]*common.Interaction)
	GetPendingIx(ixHash common.Hash) (*common.Interaction, bool)
	GetAccountWaitTime(addr identifiers.Address) (*big.Int, error)
	GetAllAccountsWaitTime() map[identifiers.Address]*big.Int
}

type ChainManager interface {
	GetTesseract(hash common.Hash, withInteractions, withCommitInfo bool) (*common.Tesseract, error)
	GetReceiptByIxHash(ixHash common.Hash) (*common.Receipt, error)
	GetTesseractHeightEntry(address identifiers.Address, height uint64) (common.Hash, error)
	GetInteractionAndParticipantsByIxHash(ixHash common.Hash) (
		*common.Interaction,
		common.Hash,
		common.ParticipantsState,
		int,
		error,
	)
	GetInteractionAndParticipantsByTSHash(tsHash common.Hash, ixIndex int) (
		*common.Interaction,
		common.ParticipantsState,
		error,
	)
}

type StateManager interface {
	GetAccountKeys(addrs identifiers.Address, stateHash common.Hash) (common.AccountKeys, error)
	GetLatestStateObject(identifiers.Address) (*state.Object, error)
	CreateStateObject(identifiers.Address, common.AccountType, bool) *state.Object
	GetStateObjectByHash(addr identifiers.Address, hash common.Hash) (*state.Object, error)
	FetchIxStateObjects(common.Interactions, map[identifiers.Address]common.Hash) (*state.Transition, error)

	GetSequenceID(addr identifiers.Address, KeyID uint64, stateHash common.Hash) (uint64, error)
	GetAccountState(identifiers.Address, common.Hash) (*common.Account, error)
	GetAccountMetaInfo(identifiers.Address) (*common.AccountMetaInfo, error)
	IsAccountRegistered(identifiers.Address) (bool, error)
	GetContextByHash(identifiers.Address, common.Hash) (common.Hash, []kramaid.KramaID, []kramaid.KramaID, error)

	GetAssetInfo(identifiers.AssetID, common.Hash) (*common.AssetDescriptor, error)
	GetBalances(identifiers.Address, common.Hash) (common.AssetMap, error)
	GetBalance(identifiers.Address, identifiers.AssetID, common.Hash) (*big.Int, error)
	GetDeeds(identifiers.Address, common.Hash) (map[string]*common.AssetDescriptor, error)
	GetMandates(identifiers.Address, common.Hash) ([]common.AssetMandateOrLockup, error)
	GetLockups(identifiers.Address, common.Hash) ([]common.AssetMandateOrLockup, error)

	GetLogicIDs(identifiers.Address, common.Hash) ([]identifiers.LogicID, error)
	GetLogicManifest(identifiers.LogicID, common.Hash) ([]byte, error)
	GetPersistentStorageEntry(identifiers.LogicID, []byte, common.Hash) ([]byte, error)
	GetEphemeralStorageEntry(identifiers.Address, identifiers.LogicID, []byte, common.Hash) ([]byte, error)
}

type ExecutionManager interface {
	InteractionCall(
		ctx *common.ExecutionContext,
		ix *common.Interaction,
		transition *state.Transition,
	) (*common.Receipt, error)
}

type Syncer interface {
	GetAccountSyncStatus(addr identifiers.Address) (*args.AccSyncStatus, error)
	GetNodeSyncStatus(includePendingAccounts bool) *args.NodeSyncStatus
	GetSyncJobInfo(addr identifiers.Address) (*args.SyncJobInfo, error)
}

type Network interface {
	GetVersion() string
	GetKramaID() kramaid.KramaID
	GetConns() []network.Conn
	GetPeers() []kramaid.KramaID
	GetInboundConnCount() int64
	GetOutboundConnCount() int64
	GetSubscribedTopics() map[string]int
	GetPeersScores() map[peer.ID]*pubsub.PeerScoreSnapshot
}

type DB interface {
	ReadEntry(key []byte) ([]byte, error)
	GetRegisteredAccounts() ([]identifiers.Address, error)
	GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *common.DBEntry, error)
}

// Backend is a struct that represents the API backend
type Backend struct {
	// Represents the API interaction pool
	Ixpool IxPool
	// Represents the API chain manager
	Chain ChainManager
	// Represents the API execution manager
	Exec ExecutionManager
	// Represents the API state manager
	SM StateManager
	// Represents the API syncer
	Syncer Syncer
	// Represents the API network
	Net Network
	// Represents the API database
	DB DB
}

// NewBackend is a constructor function that generates and returns a new API Backend object
func NewBackend(
	ixpool IxPool,
	chain ChainManager,
	exec ExecutionManager,
	sm StateManager,
	syncer Syncer,
	net Network,
	db DB,
) *Backend {
	// Create a new API Backend object and return it
	return &Backend{ixpool, chain, exec, sm, syncer, net, db}
}

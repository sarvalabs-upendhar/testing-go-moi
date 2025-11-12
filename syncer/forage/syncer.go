package forage

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/sync/errgroup"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/network/rpc"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-moi/syncer"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

const (
	MaxBucketSyncAttempts = 3
	ChannelBufferSize     = 10
	MaxPeersToDial        = 8
	TesseractFetchTimeOut = 8 * time.Second
	DefaultWorkerWaitTime = 50 * time.Millisecond
	accountWorker         = "account-worker"
	tsWorker              = "ts-worker"
	workerCount           = 10
)

var DefaultMinConnectedPeers = 6

type StorageStatus int

const (
	NotStored       StorageStatus = iota // 0: No participant is stored
	PartiallyStored                      // 1: Some participants are stored, some are not
	FullyStored                          // 2: All participants are stored
)

type lattice interface {
	AddTesseractWithState(
		id identifiers.Identifier,
		dirtyStorage map[common.Hash][]byte,
		ts *common.Tesseract,
		transition *state.Transition,
		allParticipants bool,
	) error
	GetTesseract(hash common.Hash, withInteractions, withCommitInfo bool) (*common.Tesseract, error)
	GetTesseractByHeight(
		id identifiers.Identifier,
		height uint64,
		withInteractions bool,
		withCommitInfo bool,
	) (*common.Tesseract, error)
	GetTesseractHeightEntry(id identifiers.Identifier, height uint64) (common.Hash, error)
	GetInteractionsByTSHash(tsHash common.Hash) ([]*common.Interaction, error)
}

type stateManager interface {
	LoadTransitionObjects(
		ps map[identifiers.Identifier]common.ParticipantInfo,
		psState common.ParticipantsState,
	) (*state.Transition, error)
	CreateStateObject(identifiers.Identifier, bool) *state.Object
	GetLatestStateObject(id identifiers.Identifier) (*state.Object, error)
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	SyncStorageTrees(
		ctx context.Context,
		newRoot *common.RootNode,
		logicStorageTreeRoots map[string]*common.RootNode,
		so *state.Object,
	) error
	SyncAssetTree(
		newRoot *common.RootNode,
		so *state.Object,
	) error
	SyncLogicTree(
		newRoot *common.RootNode,
		so *state.Object,
	) error
	GetParticipantContextRaw(
		id identifiers.Identifier,
		hash common.Hash,
		rawContext map[string][]byte,
	) error
	HasParticipantStateAt(id identifiers.Identifier, stateHash common.Hash) bool
	IsInitialTesseract(ts *common.Tesseract, id identifiers.Identifier) (bool, error)
	IsSealValid(ts *common.Tesseract) (bool, error)
	GetConsensusNodesByHash(
		id identifiers.Identifier,
		hash common.Hash,
	) ([]identifiers.KramaID, error)
	RefreshCachedObject(id identifiers.Identifier, sysObj *state.SystemObject)
	GetSystemObject() *state.SystemObject
}

type store interface {
	NewBatchWriter() db.BatchWriter
	CreateEntry([]byte, []byte) error
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	SetAccount(id identifiers.Identifier, stateHash common.Hash, data []byte) error
	SetInteractions(tsHash common.Hash, data []byte) error
	SetReceipts(tsHash common.Hash, data []byte) error
	GetInteractions(tsHash common.Hash) ([]byte, error)
	GetCommitInfo(tsHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Identifier) (*common.AccountMetaInfo, error)
	SetAccountSyncStatus(id identifiers.Identifier, status *common.AccountSyncStatus) error
	CleanupAccountSyncStatus(id identifiers.Identifier) error
	StoreAccountSnapShot(snap *common.Snapshot) error
	GetReceipts(tsHash common.Hash) ([]byte, error)
	GetAccountsSyncStatus() ([]*common.AccountSyncStatus, error)
	DropPrefix(prefix []byte) error
	UpdatePrimarySyncStatus(id identifiers.Identifier) error
	IsAccountPrimarySyncDone(id identifiers.Identifier) bool
	HasTesseract(tsHash common.Hash) bool
	SetTesseract(tsHash common.Hash, data []byte) error
	UpdatePrincipalSyncStatus() error
	GetBucketCount(bucketNumber uint64) (uint64, error)
	StreamAccountMetaInfosRaw(ctx context.Context, bucketNumber uint64, response chan []byte) error
	GetRecentUpdatedAccMetaInfosRaw(ctx context.Context, bucketID uint64, sinceTS uint64) ([][]byte, error)
	IsPrincipalSyncDone() (bool, int64)
	StreamSnapshot(
		ctx context.Context,
		id identifiers.Identifier,
		sinceTS uint64,
		respChan chan<- common.SnapResponse,
	) (uint64, error)
	SetTesseractHeightEntry(id identifiers.Identifier, height uint64, tsHash common.Hash) error
	HasAccMetaInfoAt(id identifiers.Identifier, height uint64) bool
	GetAccount(id identifiers.Identifier, stateHash common.Hash) ([]byte, error)
	UpdateAccMetaInfo(
		id identifiers.Identifier,
		height uint64,
		tesseractHash common.Hash,
		stateHash, contextHash common.Hash,
		consensusNodesHash common.Hash,
		inheritedAccount identifiers.Identifier,
		commitHash common.Hash,
		accType common.AccountType,
		shouldUpdateContextSetPosition bool,
		positionInContextSet int,
	) (int32, bool, error)
	SetCommitInfo(tsHash common.Hash, data []byte) error
}

type ixpool interface {
	GetIxnsWithMissingIxns(ixHashes common.Hashes) ([]*common.Interaction, []common.Hash)
}

type p2pServer interface {
	GetPeers() []identifiers.KramaID
	StartNewRPCServer(protocol protocol.ID, tag string) *rpc.Client
	RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error
	GetKramaID() identifiers.KramaID
	Subscribe(
		ctx context.Context,
		topicName string,
		validator utils.WrappedVal,
		defaultValidator bool,
		handler func(msg *pubsub.Message) error,
	) error
	GetPeerSetLen() int
	Unsubscribe(topicName string) error
	AddPeerInfoPermanently(info *peer.AddrInfo)
	GetAddrsFromPeerStore(peerID peer.ID) []maddr.Multiaddr
}

type kramaEngine interface {
	ValidateTesseract(
		id identifiers.Identifier,
		ts *common.Tesseract,
		ics *ktypes.ICSCommittee,
		allParticipants bool,
	) error
	ExecuteAndValidate(
		ts *common.Tesseract,
		transition *state.Transition,
	) error
	GetICSCommittee(
		ts *common.Tesseract,
		info *common.CommitInfo,
		systemObject *state.SystemObject,
	) (*ktypes.ICSCommittee, error)
	GetICSCommitteeFromRawContext(
		ts *common.Tesseract,
		rawContext map[string][]byte,
		info *common.CommitInfo,
		systemObject *state.SystemObject,
	) (*ktypes.ICSCommittee, error)
	AddActiveAccounts(accountLocks map[identifiers.Identifier]common.LockType, clusterID common.ClusterID) bool
	ClearActiveAccounts(clusterID common.ClusterID, ids ...identifiers.Identifier)
	GetLockedTSFromDB(tsHash common.Hash) (*common.Tesseract, error)
	DeleteLockedTSInfo(ts *common.Tesseract, fromSyncer bool)
}

func createClusterID(prefix string, id common.ClusterID) common.ClusterID {
	return common.ClusterID(fmt.Sprintf("%s%s", prefix, id))
}

type participantInfo struct {
	lock           common.LockType
	isSender       bool
	newParticipant bool
}

func (pi *participantInfo) isReadOrNoLock() bool {
	return pi.lock > common.MutateLock
}

type Syncer struct {
	lock                sync.RWMutex
	cfg                 *config.SyncerConfig
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	network             p2pServer
	mux                 *utils.TypeMux
	agora               syncer.BlockSync
	db                  store
	tesseractRegistry   *common.HashRegistry
	tsJobQueue          *TesseractJobQueue
	tsWorkerSignal      chan struct{}
	tsWorkerCount       uint32
	tsWorkerLock        sync.Mutex
	accountJobQueue     *AccountJobQueue
	rpcClient           *rpc.Client
	consensus           kramaEngine
	lattice             lattice
	state               stateManager
	ixpool              ixpool
	logger              hclog.Logger
	accountWorkerLock   sync.Mutex
	accountWorkerCount  uint32
	accountWorkerSignal chan struct{}
	isPrincipalSyncDone bool
	bucketSyncDone      bool
	pendingAccounts     uint64
	consensusSlots      *ktypes.Slots
	lastActiveTimeStamp uint64
	lockedAccounts      map[identifiers.Identifier]struct{}
	metrics             *Metrics
	initialSyncDone     bool
	pendingMsgChan      chan *TesseractInfo
	pendingMsgQueue     []*TesseractInfo
	init                sync.Once
	execGrid            map[common.Hash]struct{}
	tracker             *SyncStatusTracker
	workerWaitTime      time.Duration
	syncPeersPresent    bool
	IxFetchGrid         map[common.Hash]struct{}
	IxFetchLock         sync.Mutex
	compressor          common.Compressor
}

func NewSyncer(
	cfg *config.SyncerConfig,
	logger hclog.Logger,
	node *p2p.Server,
	mux *utils.TypeMux,
	db store,
	krama kramaEngine,
	lattice lattice,
	sm stateManager,
	ixpool ixpool,
	slots *ktypes.Slots,
	lastActiveTimeStamp uint64,
	syncerMetrics *Metrics,
	blockSync syncer.BlockSync,
	compressor common.Compressor,
) (*Syncer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Syncer{
		ctx:                 ctx,
		ctxCancel:           cancel,
		network:             node,
		cfg:                 cfg,
		mux:                 mux,
		agora:               blockSync,
		db:                  db,
		consensus:           krama,
		lattice:             lattice,
		state:               sm,
		ixpool:              ixpool,
		tsJobQueue:          NewTesseractJobQueue(syncerMetrics),
		tsWorkerCount:       workerCount,
		tsWorkerSignal:      make(chan struct{}),
		accountWorkerCount:  workerCount,
		workerWaitTime:      DefaultWorkerWaitTime,
		accountJobQueue:     NewAccountJobQueue(mux, krama),
		logger:              logger.Named("Syncer"),
		accountWorkerSignal: make(chan struct{}),
		tesseractRegistry:   common.NewHashRegistry(60),
		consensusSlots:      slots,
		lastActiveTimeStamp: lastActiveTimeStamp,
		lockedAccounts:      make(map[identifiers.Identifier]struct{}),
		metrics:             syncerMetrics,
		pendingMsgQueue:     make([]*TesseractInfo, 0),
		pendingMsgChan:      make(chan *TesseractInfo, 10),
		execGrid:            make(map[common.Hash]struct{}),
		tracker:             NewSyncStatusTracker(0),
		syncPeersPresent:    len(cfg.SyncPeers) > 0,
		IxFetchGrid:         make(map[common.Hash]struct{}),
		compressor:          compressor,
	}

	return s, nil
}

func (s *Syncer) RPCClient() *rpc.Client {
	return s.rpcClient
}

func (s *Syncer) addSyncPeersToPeerstore() error {
	for i := 0; i < len(s.cfg.SyncPeers); i++ {
		bestPeer := s.cfg.SyncPeers[i]

		peerID, err := bestPeer.ID.DecodedPeerID()
		if err != nil {
			s.logger.Error("failed to get peer ID from consensus ID", "consensus-accountID", bestPeer.ID, "err", err)

			return common.ErrInvalidKramaID
		}

		s.network.AddPeerInfoPermanently(
			&peer.AddrInfo{
				ID:    peerID,
				Addrs: []maddr.Multiaddr{bestPeer.Address},
			},
		)
	}

	return nil
}

func (s *Syncer) NewSyncRequest(
	id identifiers.Identifier,
	expectedHeight uint64,
	syncMode common.SyncMode,
	bestPeers []identifiers.KramaID,
	snapDownloaded bool,
	tsWorkerSignal chan struct{},
	tesseracts ...*TesseractInfo,
) (err error) {
	job, ok := s.accountJobQueue.getJob(id)
	if job == nil {
		clusterID, err := common.GenerateClusterID()
		if err != nil {
			return err
		}

		job = &AccountSyncJob{
			db:              s.db,
			logger:          s.logger,
			id:              id,
			creationTime:    time.Now(),
			mode:            syncMode,
			tesseractQueue:  NewTesseractQueue(),
			jobState:        Pending,
			snapDownloaded:  snapDownloaded,
			tesseractSignal: make(chan struct{}, 1),
			bestPeers:       make(map[identifiers.KramaID]struct{}),
			tsWorkerSignal:  tsWorkerSignal,
			clusterID:       createClusterID(accountWorker, clusterID),
		}

		job.updateBestPeers(bestPeers)

		metaInfo, err := s.db.GetAccountMetaInfo(id)
		if err == nil {
			if metaInfo.Height >= expectedHeight {
				_, err = s.postAdditionHook(job, metaInfo.Height)

				return err
			}

			if job.getCurrentHeight() < metaInfo.Height {
				job.updateCurrentHeight(metaInfo.Height)
			}
		}
	}

	for _, v := range tesseracts {
		if job.tesseractQueue.Has(v.height()) || v.height() < job.getCurrentHeight() {
			continue
		}

		job.tesseractQueue.Push(v)
	}

	if job.getExpectedHeight() < expectedHeight {
		if err = job.updateExpectedHeight(expectedHeight); err != nil {
			return err
		}
	}

	tsHash, _ := s.lattice.GetTesseractHeightEntry(job.id, job.getCurrentHeight())
	if job.getCurrentHeight() == job.getExpectedHeight() && tsHash != common.NilHash {
		s.logger.Debug("ts found, avoiding new sync request")

		return nil
	}

	if !s.hasSyncPeers() && syncMode == common.FullSync {
		var height uint64

		height, bestPeers, err = s.findLatestHeightAndBestPeers(id)
		if err != nil {
			return errors.Wrap(err, "failed to find best peers for sync")
		}

		// if system account best peers doesn't have the latest height then it will be updated here
		if job.expectedHeight < height {
			if err := job.updateExpectedHeight(height); err != nil {
				return err
			}
		}
	}

	job.updateBestPeers(bestPeers)

	if job.tesseractQueue.Len() > 0 && job.getJobState() == Done {
		job.updateJobState(Pending)
	}

	if !ok {
		if err = s.accountJobQueue.AddJob(job); err != nil {
			return err
		}

		s.metrics.captureTotalJobs(float64(len(s.accountJobQueue.jobs)))

		if err = job.commitJob(); err != nil {
			return errors.Wrap(err, "failed to commit job")
		}

		s.signalNewAccountJob()
	}

	return nil
}

func (s *Syncer) addTSThroughExecution(msg *TesseractInfo) error {
	err := s.consensus.ValidateTesseract(
		identifiers.Nil,
		msg.tesseract,
		msg.committee,
		true,
	)
	if err != nil {
		return errors.Wrap(err, "failed to validate tesseract in execution phase")
	}

	if err = s.executeAndAdd(extractDirtyEntries(msg.delta), msg.tesseract); err != nil {
		return err
	}

	return nil
}

// SyncAccountState synchronizes participant states up to either the previous or current tesseract.
// - If a genesis account was created in a failed interaction, it cannot be synced to its latest or previous height.
// - When syncUpToPrevTesseract is true:
//   - NoLock accounts are synced up to the current tesseract.
//   - MutateLock accounts are synced up to their previous height.
//
// - Otherwise, all accounts are synced up to the current tesseract.
func (s *Syncer) SyncAccountState(ts *common.Tesseract, syncUpToPrevTesseract bool,
	psInfo map[identifiers.Identifier]*participantInfo,
) error {
	ch := make(chan struct{})
	defer close(ch)

	var (
		syncCount int
		skip      bool
	)

	for id, participant := range ts.Participants() {
		info, ok := psInfo[id]
		if !ok {
			return errors.New("participant not found")
		}

		// Skip genesis accounts created in failed interactions.
		if participant.IsFailure() && info.newParticipant {
			continue
		}

		height := participant.Height

		if syncUpToPrevTesseract {
			height, skip = findPreviousHeight(info, height, participant.IsSuccess())
			if skip {
				continue
			}
		}

		if s.db.HasAccMetaInfoAt(id, height) {
			continue
		}

		peers, err := s.state.GetSystemObject().GetValidatorKramaIDs(ts.CommitInfo().RandomSet)
		if err != nil {
			s.logger.Error("failed to retrieve random set krama ids",
				"id", id, "height", height, "err", err)

			continue
		}

		s.logger.Debug("creating account sync request", "height", height, "id", id,
			"before-height", participant.Height, "info", info, "statehash", participant.StateHash,
		)

		err = s.NewSyncRequest(id, height, common.LatestSync, peers, false, ch)
		if err != nil {
			s.logger.Error("failed to create account sync request",
				"id", id, "height", height, "err", err)

			continue
		}

		syncCount++
	}

	if syncCount == 0 {
		return nil
	}

	s.logger.Debug("trying to sync the accounts for", ts.Hash())

	for range ch {
		syncCount--

		if syncCount == 0 {
			s.logger.Debug("synced the accounts successfully for", ts.Hash())

			return nil
		}
	}

	return nil
}

// fetchParticipantsInfo collects participant metadata (lock type, newParticipant, isSender)
// across all interactions in the provided set.
//
// Notes:
//   - MutateLock takes precedence over ReadLock/NoLock (i.e., higher mutability wins).
//   - If a participant appears multiple times, fields are merged intelligently:
//   - lock: most restrictive (mutate > read > noLock)
//   - newParticipant: true if true in any interaction
//   - isSender: true if participant is sender in any interaction
func fetchParticipantsInfo(ixns []*common.Interaction) map[identifiers.Identifier]*participantInfo {
	info := make(map[identifiers.Identifier]*participantInfo)

	for _, ixn := range ixns {
		for id, psInfo := range ixn.Participants() {
			oldInfo, exists := info[id]
			if !exists {
				info[id] = &participantInfo{
					lock:           psInfo.LockType,
					newParticipant: psInfo.IsGenesis,
					isSender:       id == ixn.SenderID(),
				}

				continue
			}

			if psInfo.LockType < oldInfo.lock {
				oldInfo.lock = psInfo.LockType
			}

			// If participant was ever new or sender, retain true
			oldInfo.newParticipant = oldInfo.newParticipant || psInfo.IsGenesis
			oldInfo.isSender = oldInfo.isSender || (id == ixn.SenderID())
		}
	}

	return info
}

// findPreviousHeight returns the previous height of a participant if the state changed.
// It returns (height, skip):
//   - If the lock is readLock or noLock then return same height
//   - If the previous height doesn't exist, skip is true.
//   - When a interaction fails, only the sender's height changes (decremented); others retain the same height.
func findPreviousHeight(info *participantInfo, height uint64, isSuccess bool) (newHeight uint64, skip bool) {
	switch {
	case info.isReadOrNoLock():
		return height, false

	case isSuccess && height == 0:
		return 0, true

	case isSuccess:
		return height - 1, false

	case info.isSender:
		return height - 1, false

	default:
		return height, false
	}
}

// getParticipantStorageStatus determines how fully the Tesseract state
// has been stored across participants.
//
// StorageStatus can be:
//   - NotStored: no participant achieved the target state.
//   - PartiallyStored: some (≥1) participants achieved it.
//   - FullyStored: all participants achieved it.
//
// Notes:
//   - No new state is created for noLock/readLock accounts or non-sender participants in failed interactions.
//     Therefore, storage decisions for those cases are not fully informed.
func (s *Syncer) getParticipantStorageStatus(ts *common.Tesseract, psInfo map[identifiers.Identifier]*participantInfo,
) (StorageStatus, error) {
	var hasStored, hasNotStored bool

	for id, participant := range ts.Participants() {
		height := participant.Height

		info, ok := psInfo[id]
		if !ok {
			return 0, fmt.Errorf("participant %v not found", id)
		}

		switch {
		case info.isReadOrNoLock():
			if !s.db.HasAccMetaInfoAt(id, height) {
				hasNotStored = true
			}

		case participant.IsFailure():
			if info.newParticipant {
				continue
			}

			if !s.db.HasAccMetaInfoAt(id, height) {
				hasNotStored = true
			}

		default:
			if s.db.HasAccMetaInfoAt(id, height) {
				hasStored = true
			} else {
				hasNotStored = true
			}
		}

		// Short-circuit if we find both cases
		if hasStored && hasNotStored {
			return PartiallyStored, nil
		}
	}

	switch {
	case hasStored:
		return FullyStored, nil
	default:
		return NotStored, nil
	}
}

func (s *Syncer) processTesseract(tsInfo *TesseractInfo, cid common.ClusterID) {
	defer s.consensus.ClearActiveAccounts(cid, tsInfo.tesseract.AccountIDs()...)

	ixns, err := s.lattice.GetInteractionsByTSHash(tsInfo.tesseract.Hash())
	if err != nil {
		if err := s.fillTSWithIxnsAndReceipts(tsInfo); err != nil {
			s.logger.Trace("failed to fetch ixns and receipts ", "err", err)

			return
		}
	} else {
		tsInfo.tesseract.SetIxns(common.NewInteractionsWithLeaderCheck(true, ixns...))
	}

	psInfo := fetchParticipantsInfo(tsInfo.tesseract.Interactions().IxList())

	status, err := s.getParticipantStorageStatus(tsInfo.tesseract, psInfo)
	if err != nil {
		s.tsJobQueue.delete(tsInfo.tesseract)
		s.logger.Error("failed to get participants storage status", "err", err)

		return
	}

	switch status {
	case FullyStored:
		s.tsJobQueue.delete(tsInfo.tesseract)

		return
	case PartiallyStored:
		if err := s.SyncAccountState(tsInfo.tesseract, false, psInfo); err != nil {
			s.logger.Error("failed to sync partial account state", "err", err)
		}

		s.tsJobQueue.delete(tsInfo.tesseract)

		return
	case NotStored:
		if err := s.SyncAccountState(tsInfo.tesseract, true, psInfo); err != nil {
			s.logger.Error("failed to sync account state", "err", err)

			s.tsJobQueue.delete(tsInfo.tesseract)

			return
		}
	}

	if tsInfo.committee == nil && !tsInfo.extractICSNodeset(s) {
		return
	}

	if err := s.addTSThroughExecution(tsInfo); err != nil {
		s.logger.Error("failed to add tesseract ", "ts-hash", tsInfo.tesseract.Hash(), "error", err)
	}

	s.tsJobQueue.delete(tsInfo.tesseract)
}

func (s *Syncer) tesseractWorker() {
	defer func() {
		s.tsWorkerLock.Lock()
		s.tsWorkerCount--
		s.tsWorkerLock.Unlock()
		s.logger.Debug("Closing tesseract worker")
	}()

	for {
		select {
		case <-s.tsWorkerSignal:
		case <-time.After(s.workerWaitTime):
		case <-s.ctx.Done():
			return
		}

		clusterID, err := common.GenerateClusterID()
		if err != nil {
			s.logger.Error("failed to generate cluster id", "err", err)

			continue
		}

		clusterID = createClusterID(tsWorker, clusterID)

		lockAccounts := func(ts *common.Tesseract) bool {
			if !s.consensus.AddActiveAccounts(ts.ConsensusInfo().AccountLocks, clusterID) {
				s.logger.Debug("accounts are active, tesseract is ignored", "ts-hash", ts.Hash())

				return false
			}

			return true
		}

		tsInfo := s.tsJobQueue.nextTesseractInfo(lockAccounts)
		if tsInfo == nil {
			continue
		}

		s.processTesseract(tsInfo, clusterID)
	}
}

func (s *Syncer) startTesseractWorkers() {
	for i := uint32(0); i < s.tsWorkerCount; i++ {
		go s.tesseractWorker()
	}
}

func updateJobHandler(s *Syncer) func(jq *AccountJobQueue, jb *AccountSyncJob) *AccountSyncJob {
	return func(jq *AccountJobQueue, jb *AccountSyncJob) *AccountSyncJob {
		if jb.getJobState() == Pending ||
			(jb.getJobState() == Sleep && time.Since(jb.lastModifiedAt) > time.Millisecond*20) {
			jb.updateJobState(Active)

			return jb
		}

		if jb.getJobState() == Done && jb.tesseractQueue.Len() == 0 {
			if err := jq.RemoveJob(jb); err != nil {
				log.Panicln(err)
			}

			if jb.tsWorkerSignal != nil {
				jb.tsWorkerSignal <- struct{}{} // Send a signal
			}

			s.metrics.captureJobTimeInQueue(jb.creationTime)
		}

		return nil
	}
}

func (s *Syncer) accountWorker() {
	defer func() {
		s.accountWorkerLock.Lock()
		s.accountWorkerCount--
		s.accountWorkerLock.Unlock()
		s.logger.Debug("Closing syncer account worker")
	}()

	for {
		select {
		case <-s.accountWorkerSignal:
		case <-time.After(s.workerWaitTime):
		case <-s.ctx.Done():
			return
		}

		job := s.accountJobQueue.NextJob(updateJobHandler(s))

		s.metrics.captureTotalJobs(float64(s.accountJobQueue.len()))

		if job == nil {
			continue
		}

		requestTime := time.Now()

		if err := s.jobProcessor(job); err != nil {
			s.logger.Error("Error from sync job processor", "err", err)
		}

		s.metrics.captureJobProcessingTime(requestTime)
	}
}

func (s *Syncer) hasSyncPeers() bool {
	return s.syncPeersPresent
}

func (s *Syncer) jobClosure(job *AccountSyncJob) error {
	if currentState := job.getJobState(); currentState == Sleep || currentState == Done {
		return nil
	}

	job.updateJobState(Pending)

	return nil
}

func (s *Syncer) isInitialSyncDone() bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.initialSyncDone
}

func (s *Syncer) setInitialSyncDone(val bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.initialSyncDone = val
}

func (s *Syncer) isBucketSyncDone() bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.bucketSyncDone
}

func (s *Syncer) setBucketSyncDone(val bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.bucketSyncDone = val
}

func (s *Syncer) RPCGetLatestAccountInfo(
	bestPeer identifiers.KramaID,
	id identifiers.Identifier,
) (*LatestAccountInfo, error) {
	resp := new(LatestAccountInfo)

	if err := s.rpcClient.MoiCall(
		context.Background(),
		bestPeer,
		"SYNCRPC",
		"GetLatestAccountInfo",
		id,
		resp,
		5*time.Second,
	); err != nil {
		s.logger.Error("failed to fetch account latest status",
			"RPC-error", err, "peer-id", bestPeer, "accountID", id)

		return nil, err
	}

	return resp, nil
}

func (s *Syncer) RPCGetIxns(
	peerID identifiers.KramaID,
	tsHash common.Hash,
	ixnHashes []common.Hash,
) (*IxnsResponse, error) {
	resp := new(IxnsResponse)

	if err := s.rpcClient.MoiCall(
		context.Background(),
		peerID,
		"SYNCRPC",
		"GetIxns",
		IxnsRequest{
			TSHash:    tsHash,
			IxnHashes: ixnHashes,
		},
		resp,
		5*time.Second,
	); err != nil {
		s.logger.Error("failed to fetch ixns", "RPC-error", err, "peer-id", peerID)

		return nil, err
	}

	return resp, nil
}

func (s *Syncer) chooseBestPeersForInitialSync(id identifiers.Identifier) (uint64, []identifiers.KramaID, error) {
	if s.hasSyncPeers() {
		for i := 0; i < 3; i++ {
			bestPeer := s.chooseAnySyncPeer()

			resp, err := s.RPCGetLatestAccountInfo(bestPeer.ID, id)
			if err != nil {
				continue
			}

			return resp.Height, []identifiers.KramaID{bestPeer.ID}, nil
		}

		return 0, nil, errors.New("unable to fetch latest account info from sync peers")
	}

	bestHeight, bestPeers, err := s.findLatestHeightAndBestPeers(id)
	if err != nil {
		return 0, nil, err
	}

	return bestHeight, bestPeers, nil
}

func (s *Syncer) chooseBestPeer(job *AccountSyncJob) (identifiers.KramaID, error) {
	// If initial sync is not done, then choose best peer from sync peers
	// to improve probability of success in syncing
	if !s.isInitialSyncDone() && s.hasSyncPeers() {
		bestPeer := s.chooseAnySyncPeer()

		return bestPeer.ID, nil
	}

	if job.bestPeerLen() > 0 {
		return job.chooseRandomBestPeer(), nil
	}

	return s.chooseBestSyncPeer(job)
}

func (s *Syncer) jobProcessor(job *AccountSyncJob) error {
	var (
		err      error
		jobState = job.getJobState()
	)

	s.logger.Debug(
		"Processing new job",
		"accountID", job.id,
		"current-height", job.currentHeight,
		"expected-height", job.getExpectedHeight(),
	)

	defer func() {
		if err = s.jobClosure(job); err != nil {
			log.Fatal(err)
		}
	}()

	if job.tsWorkerSignal == nil {
		// Attempt to lock the account for synchronization also make sure that account is not locked by this job.
		if !s.consensus.AddActiveAccounts(
			map[identifiers.Identifier]common.LockType{job.id: common.MutateLock}, job.clusterID) && !job.selfAccLock {
			s.logger.Debug("Account is active, job state set to sleep", "addr", job.id)
			job.updateJobState(Sleep)

			return nil
		}

		// self acc lock indicates if the account is locked by syncer or consensus
		if !job.selfAccLock {
			job.selfAccLock = true
			s.logger.Trace("added to active accounts", "id", job.id,
				"current-height", job.getCurrentHeight(), "expected-height", job.getExpectedHeight())
		}
	}

	s.metrics.captureActiveJobs(1)

	defer func() {
		s.metrics.captureActiveJobs(-1)
	}()

	bestPeer, err := s.chooseBestPeer(job)
	if err != nil {
		return err
	}

	tsInfo := job.tesseractQueue.Peek()

	if !job.snapDownloaded && s.isSnapSyncRequired(job.id) && job.mode != common.LatestSync {
		if err = s.fetchAndStoreSnap(bestPeer, job); err != nil {
			job.deleteBestPeer(bestPeer)

			return err
		}

		if err = s.publishEventSnapSync(job.jobStateEvent()); err != nil {
			s.logger.Error("failed to publish event bucket sync", "err", err)
		}
	}

	job.setLatticeSyncInProgress(true)

	group, groupCtx := errgroup.WithContext(context.Background())

	group.Go(func() error {
		if err = s.syncLattice(groupCtx, tsInfo, job, bestPeer); err != nil {
			job.deleteBestPeer(bestPeer)

			return errors.Wrap(err, "failed to sync lattice")
		}

		job.setLatticeSyncInProgress(false)

		return nil
	})

	group.Go(func() error {
		var tsInfo *TesseractInfo

		for job.getCurrentHeight() <= job.getExpectedHeight() {
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			default:
			}

			tsInfo = job.tesseractQueue.Peek()

			for tsInfo == nil {
				// If the sync lattice routine has finished and the tesseract queue is empty,
				// exit this routine because there is no one to fill the tesseract queue.
				if !job.isLatticeSyncInProgress() {
					return nil
				}

				select {
				case <-groupCtx.Done():
					return groupCtx.Err()
				case <-job.tesseractSignal:
					tsInfo = job.tesseractQueue.Peek()
				case <-time.After(50 * time.Millisecond):
				}
			}

			if !s.db.HasAccMetaInfoAt(tsInfo.id(), tsInfo.height()) {
				initial, err := s.state.IsInitialTesseract(
					tsInfo.tesseract,
					tsInfo.id(),
				)
				if err != nil {
					s.logger.Debug("failed to check initial tesseract", "err", err,
						"id", tsInfo.id(), "height", tsInfo.height())

					jobState = Sleep

					return nil
				}

				if !initial && tsInfo.height() != job.getCurrentHeight()+1 {
					s.logger.Error(
						"Missing tesseract",
						"accountID", tsInfo.id(),
						"height", tsInfo.height(),
						"err", err,
					)

					jobState = Sleep

					return nil
				}

				isTesseractAdded, err := s.syncTesseract(tsInfo)
				if err != nil {
					job.tesseractQueue.Pop()

					return err
				}

				if !isTesseractAdded {
					jobState = Sleep

					return nil
				}
			}

			job.tesseractQueue.Pop()

			shouldExit, err := s.postAdditionHook(job, tsInfo.height())
			if err != nil || shouldExit {
				jobState = Done

				return err
			}
		}

		return nil
	})

	/* Algorithm
	- Check the sync mode,
	- Check if snap sync is required
	- Check for the snap, if not available fetch the snap
	- Check the
	*/

	if err = group.Wait(); err != nil {
		job.updateJobState(jobState)

		return err
	}

	job.updateJobState(jobState)

	return nil
}

// postAdditionHook updates the status flags in the database after successful completion of the job
func (s *Syncer) postAdditionHook(job *AccountSyncJob, newHeight uint64) (bool, error) {
	if job.getCurrentHeight() < newHeight {
		job.updateCurrentHeight(newHeight)
	}

	if job.getExpectedHeight() != newHeight {
		return false, nil
	}

	if job.mode == common.FullSync {
		if err := s.db.UpdatePrimarySyncStatus(job.id); err != nil {
			return false, errors.Wrap(err, "failed to update account primary sync status")
		}
	}

	if err := s.updatePrincipalSyncStatus(); err != nil {
		return false, errors.Wrap(err, "failed to update principal sync status")
	}

	return true, nil
}

func (s *Syncer) signalNewAccountJob() {
	select {
	case s.accountWorkerSignal <- struct{}{}:
	default:
		s.logger.Error("failed to signal new job")
	}
}

func (s *Syncer) updatePrincipalSyncStatus() error {
	if atomic.LoadUint64(&s.pendingAccounts) == 0 {
		return nil
	}

	if !s.isPrincipalSyncDone {
		atomic.AddUint64(&s.pendingAccounts, ^uint64(0))
	}

	if atomic.LoadUint64(&s.pendingAccounts) <= uint64(0) && s.isBucketSyncDone() {
		s.isPrincipalSyncDone = true

		return s.db.UpdatePrincipalSyncStatus()
	}

	return nil
}

/*
func (s *Syncer) cleanGridAndReleasePendingJobs(tsInfo *TesseractInfo, job *AccountSyncJob) error {
	if tsInfo.tesseract.GridLength() == 1 {
		return nil
	}

	grid := s.gridStore.GetGrid(tsInfo.tesseract.GridHash())
	if grid == nil {
		return errors.New("grid not found")
	}

	for _, ts := range grid.ts {
		if ts.Identifier() == job.accountID {
			continue
		}

		pendingJob, ok := s.accountJobQueue.getJob(ts.Identifier())
		if !ok {
			return fmt.Errorf(" %s job not found", ts.Identifier())
		}

		if err := s.releasePendingJob(pendingJob, ts); err != nil {
			s.logger.Error("failed to update pending job status", "err", err)
		}
	}

	s.gridStore.CleanupGrid(tsInfo.tesseract.GridHash())

	return nil
}

// releasePendingJob pops the added tesseract and updates the job state
func (s *Syncer) releasePendingJob(job *AccountSyncJob, ts *types.ts) error {
	queuedTSInfo := job.tesseractQueue.Pop()
	if queuedTSInfo.tesseract.Height() != ts.Height() {
		return common.ErrHeightMismatch
	}

	shouldExit, err := s.postAdditionHook(job, ts.Height())
	if err != nil || shouldExit {
		return err
	}

	job.updateJobState(Pending)

	return nil
}

*/

func getBestPeers(heightPeersMap map[uint64][]identifiers.KramaID) (uint64, []identifiers.KramaID, error) {
	maxFrequencyHeight := uint64(0)
	maxFrequencyNodes := 0

	for h, nodes := range heightPeersMap {
		if len(nodes) > maxFrequencyNodes {
			maxFrequencyNodes = len(nodes)
			maxFrequencyHeight = h
		}
	}

	bestPeers, ok := heightPeersMap[maxFrequencyHeight]
	if !ok || len(bestPeers) == 0 {
		return maxFrequencyHeight, nil, errors.New("best peer not found")
	}

	return maxFrequencyHeight, bestPeers, nil
}

// findLatestHeightAndBestPeers returns the height reported from the majority of peers as best height
func (s *Syncer) findLatestHeightAndBestPeers(id identifiers.Identifier) (uint64, []identifiers.KramaID, error) {
	heightPeersMap := make(map[uint64][]identifiers.KramaID)

	// index tracks the no of peers responded
	index := 0

	for _, kramaID := range s.network.GetPeers() {
		if index == MaxPeersToDial {
			break
		}

		resp, err := s.RPCGetLatestAccountInfo(kramaID, id)
		if err != nil {
			continue
		}

		index++

		nodes, ok := heightPeersMap[resp.Height]
		if !ok {
			heightPeersMap[resp.Height] = make([]identifiers.KramaID, 0)
		}

		nodes = append(nodes, kramaID)
		heightPeersMap[resp.Height] = nodes
	}

	return getBestPeers(heightPeersMap)
}

func (s *Syncer) chooseBestSyncPeer(job *AccountSyncJob) (identifiers.KramaID, error) {
	if job.mode == common.LatestSync && job.tesseractQueue.Peek() != nil {
		randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
		randNumber := randomSource.Intn(len(job.tesseractQueue.Peek().tesseract.CommitInfo().RandomSet))

		validator, err := s.state.GetSystemObject().Validator(
			uint64(job.tesseractQueue.Peek().tesseract.CommitInfo().RandomSet[randNumber]),
		)
		if err != nil {
			return "", err
		}

		return validator.KramaID, nil
	}

	_, bestPeers, err := s.findLatestHeightAndBestPeers(job.id)
	if err != nil {
		return "", err
	}

	job.updateBestPeers(bestPeers)

	return bestPeers[0], nil
}

func (s *Syncer) chooseAnySyncPeer() config.NodeInfo {
	randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
	randNumber := randomSource.Intn(len(s.cfg.SyncPeers))

	return s.cfg.SyncPeers[randNumber]
}

// syncSystemAccount sends a sync request for the specified accountID and waits for it to complete within a given time.
// If the sync does not complete within the specified time, an error is returned.
func (s *Syncer) syncSystemAccount(id ...identifiers.Identifier) ([]identifiers.KramaID, error) {
	var (
		bestPeers  []identifiers.KramaID
		bestHeight uint64
		err        error
	)

	for _, id := range id {
		bestHeight, bestPeers, err = s.chooseBestPeersForInitialSync(id)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch best peers and height")
		}

		if err = s.NewSyncRequest(id, bestHeight, common.FullSync, bestPeers,
			false, nil); err != nil {
			return nil, err
		}

		err = func() error {
			ctx, cancel := context.WithTimeout(s.ctx, time.Duration(6000+(bestHeight*5000))*time.Millisecond)
			defer cancel()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
				}

				metaInfo, err := s.db.GetAccountMetaInfo(id)
				if err == nil && metaInfo.Height >= bestHeight {
					return nil
				}
			}
		}()
		if err != nil {
			return nil, err
		}
	}

	return bestPeers, nil
}

func (s *Syncer) initSync() error {
	var principalSyncTimeStamp int64

	s.isPrincipalSyncDone, principalSyncTimeStamp = s.db.IsPrincipalSyncDone()
	if s.isPrincipalSyncDone {
		s.logger.Info("Principal sync was finished at", "unix-time", principalSyncTimeStamp)
	}

	if s.isInitialSyncDone() {
		s.logger.Info("Initial sync is already done")

		return nil
	}

	// Sync all system accounts
	bestPeers, err := s.syncSystemAccount(common.SystemAccountID, common.SargaAccountID)
	if err != nil {
		s.logger.Error("failed to sync system account", "err", err)

		return err
	}

	if err := s.publishEventSystemAccounts(); err != nil {
		s.logger.Error("failed to publish event system accounts sync", "err", err)
	}

	s.logger.Info("System accounts sync successful")

	if err = s.loadSyncJobsFromDB(); err != nil {
		s.logger.Error("failed to load sync jobs from DB", "err", err)
	} else if err = s.publishEventLoadSyncJobsDB(); err != nil {
		s.logger.Error("failed to publish event load sync jobs from DB", "err", err)
	}

	return s.syncBucketsWithMaxAttempts(bestPeers, MaxBucketSyncAttempts)
}

func (s *Syncer) syncBucketsWithMaxAttempts(bestPeers []identifiers.KramaID, maxAttempts int) error {
	for i := 1; i < maxAttempts+1; i++ {
		randomNumber := rand.New(rand.NewSource(time.Now().UnixNano()))
		bestPeer := bestPeers[randomNumber.Intn(len(bestPeers))]

		requestTime := time.Now()

		if err := s.syncBuckets(bestPeer, i); err != nil {
			s.logger.Error("failed to sync buckets, retrying...!!!", "err", err)
			s.metrics.captureBucketSyncTime(requestTime)

			continue
		}

		s.metrics.captureBucketSyncTime(requestTime)
		s.logger.Info("Bucket sync successful")

		if err := s.publishEventBucketSync(); err != nil {
			s.logger.Error("failed to publish event bucket sync", "err", err)
		}

		return nil
	}

	return errors.New("bucket sync failed")
}

/*
func (s *Syncer) syncBucketSince(kramaID accountID.KramaID, sinceTs uint64) error {
	var (
		argsChan = make(chan *BucketSyncRequest, 1)
		respChan = make(chan *BucketSyncResponse, ChannelBufferSize)
	)

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		s.logger.Error("failed to decode peer ID", "err", err)
	}

	errGrp, grpCtx := errgroup.WithContext(s.ctx)

	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(grpCtx, peerID, "SYNCRPC", "SyncBucketsSince", argsChan, respChan); err != nil {
			s.logger.Error("failed to sync buckets", "err", err)

			return err
		}

		return nil
	})

	errGrp.Go(func() error {
		defer close(argsChan)

		for i := uint64(0); i < dhruva.MaxBucketCount; i++ {
			argsChan <- &BucketSyncRequest{
				BucketID:  i,
				Timestamp: sinceTs,
			}

			totalEntriesInBucket := uint64(0)
			err = func() error {
				ctx, cancel := context.WithTimeout(grpCtx, 2*time.Second)
				defer cancel()

				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case respMsg, ok := <-respChan:
						if !ok {
							s.logger.Error("Response channel closed")

							return nil
						}

						if i != respMsg.BucketID {
							return errors.New("invalid bucket accountID")
						}

						if respMsg.BucketCount == 0 {
							return nil
						}

						if totalEntriesInBucket == 0 {
							totalEntriesInBucket = respMsg.BucketCount
						}

						// send the data to meta info handler
						if err = s.handleAccountMetaInfo(respMsg.AccountMetaInfos, types.LatestSync); err != nil {
							s.logger.Error("Failed to create sync jobs from accMetaInfo", "err", err)

							return err
						}

						totalEntriesInBucket -= uint64(len(respMsg.AccountMetaInfos))
					}

					if totalEntriesInBucket == 0 {
						return nil
					}
				}
			}()
			if err != nil {
				return err
			}
		}

		return nil
	})

	return errGrp.Wait()
}
*/

func (s *Syncer) loadSyncJobsFromDB() error {
	accountSyncInfos, err := s.db.GetAccountsSyncStatus()
	if err != nil {
		return err
	}

	for _, v := range accountSyncInfos {
		if err = s.NewSyncRequest(
			v.ID,
			v.ExpectedHeight,
			v.Mode,
			nil,
			v.SnapshotDownloaded,
			nil,
		); err != nil {
			s.logger.Error("Failed to create sync request for job", "error", err, "accountID", v.ID)
		}
	}

	return nil
}

func (s *Syncer) syncBuckets(bestPeer identifiers.KramaID, attempts int) error {
	var (
		argsChan = make(chan *BucketSyncRequest, 1)
		respChan = make(chan *BucketSyncResponse, ChannelBufferSize)
	)

	peerID, err := bestPeer.DecodedPeerID()
	if err != nil {
		s.logger.Error("Failed to decode peer ID", "err", err)

		return err
	}

	s.logger.Info("Sending bucket sync request", "peerID", peerID)

	go func() {
		if err = s.rpcClient.Stream(
			context.Background(),
			peerID,
			"SYNCRPC",
			"SyncBuckets",
			argsChan,
			respChan,
			connTag(identifiers.Nil, "syncBuckets")); err != nil {
			s.logger.Error("Failed to sync buckets", "err", err)
		}
	}()

	defer close(argsChan)

	for i := uint64(0); i <= storage.MaxBucketCount; i++ {
		argsChan <- &BucketSyncRequest{
			BucketID: i,
		}

		totalEntriesInBucket := uint64(0)

		err = func() error {
			for {
				select {
				case <-time.After(time.Duration(5*attempts) * time.Second):
					return common.ErrTimeOut
				case respMsg, ok := <-respChan:
					if !ok {
						return errors.New("response chan closed")
					}

					if i != respMsg.BucketID {
						s.logger.Error("Invalid bucket", "err", err)

						return errors.New("invalid bucket accountID")
					}

					if respMsg.BucketCount == 0 {
						return nil
					}

					if totalEntriesInBucket == 0 {
						totalEntriesInBucket = respMsg.BucketCount
					}

					s.logger.Debug("Bucket info", "bucket-ID", respMsg.BucketID, "bucket-count", respMsg.BucketCount)

					// send the data to meta info handler
					if err = s.handleAccountMetaInfo(respMsg.AccountMetaInfos, common.FullSync); err != nil {
						s.logger.Error("Failed to create sync jobs from accMetaInfo", "err", err)

						return err
					}

					totalEntriesInBucket -= uint64(len(respMsg.AccountMetaInfos))

					if totalEntriesInBucket == 0 {
						return nil
					}
				}
			}
		}()
		if err != nil {
			return err
		}
	}

	s.setBucketSyncDone(true)

	return nil
}

func (s *Syncer) handleAccountMetaInfo(data [][]byte, syncMode common.SyncMode) error {
	acc := new(common.AccountMetaInfo)

	for _, v := range data {
		if err := polo.Depolorize(acc, v); err != nil {
			return err
		}

		localMetaInfo, err := s.db.GetAccountMetaInfo(acc.ID)
		if err == nil {
			if localMetaInfo.Height >= acc.Height {
				continue
			}
		}

		atomic.AddUint64(&s.pendingAccounts, 1)
		// TODO: Should improve this, accountJobQueue will consume most of the memory, if job processor is slow

		if err = s.NewSyncRequest(
			acc.ID,
			acc.Height,
			syncMode,
			nil,
			false,
			nil,
		); err != nil {
			s.logger.Error(
				"Failed to add new sync request",
				"accountID", acc.ID,
				"height", acc.Height,
				"err", err,
			)
		}
	}

	return nil
}

func (s *Syncer) isSnapSyncRequired(id identifiers.Identifier) bool {
	if !s.cfg.EnableSnapSync {
		return false
	}

	return !s.db.IsAccountPrimarySyncDone(id)
}

func (s *Syncer) fetchAndStoreSnap(bestPeer identifiers.KramaID, job *AccountSyncJob) error {
	ctx, cancel := context.WithTimeout(
		context.Background(), // TODO: Need to improve the timeouts
		time.Duration(5000+(job.getExpectedHeight())*5000)*time.Millisecond,
	)
	defer cancel()

	s.logger.Trace("Initiating snap sync request", "accountID", job.id)

	dropPrefix := func() {
		err := s.db.DropPrefix(storage.NewIdentifierKey(job.id).Bytes())
		if err != nil {
			panic(err) // This should never happen
		}
	}

	isSnapStored, err := s.fetchSnapShot(ctx, bestPeer, job.id, job.expectedHeight)
	if err != nil {
		if isSnapStored {
			dropPrefix()
		}

		return errors.Wrap(err, "failed to fetch snapshot")
	}

	s.logger.Trace("Snap fetch successful", "accountID", job.id)

	if err = job.updateSnap(true); err != nil {
		dropPrefix()

		return err
	}

	return nil
}

// This comment explains the process of sending a snapshot of an account.
// Let's assume the entire snapshot size is 1500MB. The sender retrieves buffered data from Badger,
// typically around 100MB but may vary.
// Initially, the sender transmits a start signal to the receiver, along with the size of the buffered data.
// This allows the receiver to allocate memory and verify for any missing data at the end.
// Following this, the sender breaks the buffered data into 256KB packets and starts sending them.
// Upon reaching the end of the buffered data, the sender sends an end signal, prompting the receiver
// to flush the received data to the database.
// Once the sender has sent the entire 1500MB snapshot, it signals the end of the process by sending the total size
// of the snapshot sent.
func (s *Syncer) fetchSnapShot(
	ctx context.Context,
	peer identifiers.KramaID,
	id identifiers.Identifier,
	expectedHeight uint64,
) (bool, error) {
	peerID, err := peer.DecodedPeerID()
	if err != nil {
		return false, errors.Wrap(err, "failed to decode peer ID")
	}

	var (
		reqChan      = make(chan *SnapRequest, 1)
		respChan     = make(chan *common.SnapResponse, 2)
		expectedSize = uint64(0)
		isSnapStored = false
	)

	currentSnap := &common.Snapshot{
		Prefix:  id.Bytes(),
		Entries: make([]byte, 0),
	}

	reqChan <- &SnapRequest{
		AccountID: id,
		Height:    expectedHeight,
	}
	close(reqChan)

	errGrp, grpCtx := errgroup.WithContext(ctx)
	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(
			grpCtx,
			peerID,
			"SYNCRPC",
			"SyncSnap",
			reqChan,
			respChan,
			connTag(id, "syncSnap"),
		); err != nil {
			s.logger.Error("failed to fetch snapshot ", "err", err)

			return err
		}

		return nil
	})

	errGrp.Go(func() error {
		for {
			select {
			case <-grpCtx.Done():
				return errors.New("context expired")

			case snapMsg, ok := <-respChan:
				if !ok {
					return errors.New("response chan closed")
				}

				if snapMsg.Start {
					currentSnap.Entries = make([]byte, 0, snapMsg.ChunkSize)
					expectedSize = snapMsg.ChunkSize

					continue
				}

				if snapMsg.End {
					if expectedSize != uint64(len(currentSnap.Entries)) {
						return errors.New("packets are missing")
					}

					if err := s.db.StoreAccountSnapShot(currentSnap); err != nil {
						return err
					}

					isSnapStored = true

					continue
				}

				// if the entire snapshot has been received, then return
				if snapMsg.MetaInfo != nil && currentSnap.TotalSnapSize == snapMsg.MetaInfo.TotalSnapSize {
					return nil
				}

				currentSnap.TotalSnapSize += uint64(len(snapMsg.Data))
				currentSnap.Entries = append(currentSnap.Entries, snapMsg.Data...)

				s.logger.Info("Received Snap info ", "current snap size ", currentSnap.TotalSnapSize)
			}
		}
	})

	if err = errGrp.Wait(); err != nil {
		return isSnapStored, err
	}

	return isSnapStored, nil
}

func (s *Syncer) registerRPCService() error {
	s.rpcClient = s.network.StartNewRPCServer(config.SyncProtocolRPC, "SYNCRPC")

	return s.network.RegisterNewRPCService(config.SyncProtocolRPC, "SYNCRPC", NewSyncRPCService(s))
}

func (s *Syncer) fromGenesis(id identifiers.Identifier, currentHeight uint64) bool {
	if currentHeight == 0 {
		_, err := s.db.GetAccountMetaInfo(id)
		if errors.Is(err, common.ErrAccountNotFound) {
			return true
		}
	}

	return false
}

func (s *Syncer) syncLattice(
	ctx context.Context,
	nextTS *TesseractInfo,
	job *AccountSyncJob,
	bestPeer identifiers.KramaID,
) error {
	var (
		endHeight   = job.getExpectedHeight()
		startHeight = job.getCurrentHeight()
		respChan    = make(chan *networkmsg.TesseractSyncMsg, 5)
		reqChan     = make(chan *LatticeRequest, 1)
	)

	if nextTS != nil {
		// check if we have tesseract for start height, if not sync from the start height
		if s.db.HasAccMetaInfoAt(job.id, startHeight) || nextTS.height() == startHeight {
			if int64(nextTS.height()-(startHeight+1)) <= 0 {
				return nil
			}
		}

		endHeight = nextTS.height() - 1
	}

	peerID, err := bestPeer.DecodedPeerID()
	if err != nil {
		return errors.Wrap(err, "failed to decode peerID")
	}

	s.logger.Debug("Sending lattice sync request", "accountID", job.id)

	fromGenesis := s.fromGenesis(job.id, job.getCurrentHeight())
	if !fromGenesis {
		startHeight++
	}

	requiredTesseractCount := endHeight - startHeight + 1

	reqChan <- &LatticeRequest{
		AccountID:   job.id,
		StartHeight: startHeight,
		EndHeight:   endHeight,
	}

	close(reqChan)

	ctx, cancel := context.WithTimeout(ctx, time.Duration(endHeight-startHeight+5)*time.Second)
	defer func() {
		cancel()
	}()

	grp, grpCtx := errgroup.WithContext(ctx)

	grp.Go(func() error {
		s.logger.Debug(
			"Fetching tesseract",
			"from", peerID,
			"for", job.id,
			"start-height", startHeight,
			"end-height", endHeight,
		)

		if err = s.rpcClient.Stream(
			grpCtx,
			peerID,
			"SYNCRPC",
			"SyncLattice",
			reqChan,
			respChan,
			connTag(job.id, "syncLattice"),
		); err != nil {
			s.logger.Error("Lattice fetch failed", "err", err)

			return err
		}

		return nil
	})

	grp.Go(func() error {
		for requiredTesseractCount > 0 {
			select {
			case <-grpCtx.Done():
				return grpCtx.Err()

			case msg, ok := <-respChan:
				if !ok {
					return errors.New("response channel closed")
				}

				tsInfo, err := s.tesseractInfoFromTesseractMsg(job.id, msg)
				if err != nil {
					s.logger.Error("Failed to parse tesseract info from message", "err", err)

					continue
				}

				if tsInfo.height() >= startHeight && tsInfo.height() <= endHeight {
					requiredTesseractCount--
				}

				if job.tesseractQueue.Has(tsInfo.height()) {
					continue
				}

				s.logger.Debug(
					"Adding tesseract to queue",
					"accountID", tsInfo.id(),
					"height", tsInfo.height(),
				)

				job.tesseractQueue.Push(tsInfo)
				job.signalNewTesseract()

				if job.getCurrentHeight() >= endHeight {
					return nil
				}
			}
		}

		return nil
	})

	if err = grp.Wait(); err != nil {
		return err
	}

	if err = s.publishEventLatticeSync(job.jobStateEvent()); err != nil {
		s.logger.Error("Failed to publish event lattice sync", "err", err)
	}

	return nil
}

func (s *Syncer) tesseractInfoFromTesseractMsg(
	id identifiers.Identifier,
	msg *networkmsg.TesseractSyncMsg,
) (*TesseractInfo, error) {
	var err error

	info := &TesseractInfo{
		accountID:     id,
		delta:         msg.Delta,
		shouldExecute: false,
	}

	info.tesseract, err = msg.GetTesseract()
	if err != nil {
		return nil, err
	}

	return info, nil
}

func extractDirtyEntries(delta map[string][]byte) map[common.Hash][]byte {
	dirty := make(map[common.Hash][]byte)

	for k, v := range delta {
		dirty[common.HexToHash(k)] = v
	}

	return dirty
}

func (s *Syncer) syncTesseract(msg *TesseractInfo) (bool, error) {
	var err error

	if err := s.fillTSWithIxnsAndReceipts(msg); err != nil {
		s.logger.Trace("failed to fetch ixns and receipts ", "err", err)

		return false, nil
	}

	if msg.committee == nil && !msg.extractICSNodeset(s) {
		return false, nil
	}

	syncTSThroughAgora := func() (bool, error) {
		err = s.consensus.ValidateTesseract(msg.id(), msg.tesseract, msg.committee, false)
		if err != nil {
			return false, errors.Wrap(err, "failed to validate tesseract")
		}

		ps := make(map[identifiers.Identifier]common.ParticipantInfo)

		ps[msg.id()] = common.ParticipantInfo{
			IsGenesis: msg.tesseract.TransitiveLink(msg.id()).IsNil(),
		}

		transition, err := s.state.LoadTransitionObjects(ps, nil)
		if err != nil {
			return false, err
		}

		// During the initial sync stage, we retrieve participant data from sync peers using snap sync.
		// If participant state exists, it indicates that all data related to the participant has been fetched,
		// allowing us to skip syncing for that participant.
		if s.isInitialSyncDone() ||
			!s.state.HasParticipantStateAt(msg.id(), msg.tesseract.StateHash(msg.id())) {
			if err = s.fetchTesseractState(
				msg.id(),
				msg.tesseract,
				msg.committee.GetNodes(),
				transition.MustGetObject(msg.id()),
			); err != nil {
				return false, errors.Wrap(err, "failed to fetch tesseract state")
			}
		}

		if err = s.lattice.AddTesseractWithState(
			msg.id(),
			extractDirtyEntries(msg.delta),
			msg.tesseract,
			transition,
			false,
		); err != nil {
			return false, errors.Wrap(err, "failed to add synced tesseract")
		}

		s.consensus.DeleteLockedTSInfo(msg.tesseract, true)

		if err = s.publishEventTesseractSync(msg.id(), msg.height()); err != nil {
			s.logger.Error("Failed to publish event lattice sync", "err", err)
		}

		// Clear the cache because the account state has changed
		s.state.RefreshCachedObject(msg.id(), nil)

		return true, nil
	}

	syncTSThroughExecution := func() (bool, error) {
		err := s.consensus.ValidateTesseract(
			identifiers.Nil,
			msg.tesseract,
			msg.committee,
			true,
		)
		if err != nil {
			return false, errors.Wrap(err, "failed to validate tesseract")
		}

		transition, err := s.state.LoadTransitionObjects(
			msg.tesseract.Interactions().Participants(),
			msg.tesseract.Participants(),
		)
		if err != nil {
			return false, errors.Wrap(err, "failed to load transition objects")
		}

		if err = s.consensus.ExecuteAndValidate(msg.tesseract, transition); err != nil {
			return false, err
		}

		if err = s.lattice.AddTesseractWithState(
			msg.id(),
			extractDirtyEntries(msg.delta),
			msg.tesseract,
			transition,
			false,
		); err != nil {
			return false, err
		}

		if err = s.publishEventTesseractSync(msg.id(), msg.height()); err != nil {
			s.logger.Error("Failed to publish event lattice sync", "err", err)
		}

		// Update the cache because the account state has changed
		s.state.RefreshCachedObject(msg.id(), transition.GetSystemObject())

		return true, nil
	}

	if msg.id() == common.SystemAccountID {
		return syncTSThroughExecution()
	}

	return syncTSThroughAgora()
}

func (s *Syncer) executeAndAdd(dirty map[common.Hash][]byte, ts *common.Tesseract) error {
	transition, err := s.state.LoadTransitionObjects(ts.Interactions().Participants(), nil)
	if err != nil {
		return errors.Wrap(err, "failed to load transition objects")
	}

	if err = s.consensus.ExecuteAndValidate(ts, transition); err != nil {
		return err
	}

	if err = s.lattice.AddTesseractWithState(identifiers.Nil, dirty, ts, transition, true); err != nil {
		return err
	}

	s.consensus.DeleteLockedTSInfo(ts, true)

	for id, participantState := range ts.Participants() {
		if err := s.publishEventTesseractSync(id, participantState.Height); err != nil {
			s.logger.Error("Failed to publish event lattice sync", "err", err)
		}

		// Clear the cache because the account state has changed
		s.state.RefreshCachedObject(id, transition.GetSystemObject())
	}

	return nil
}

// fetchTesseractState fetches the complete state(balance,context,approvals) of the given tesseract using agora
func (s *Syncer) fetchTesseractState(
	id identifiers.Identifier,
	tesseract *common.Tesseract,
	fetchContext []identifiers.KramaID,
	object *state.Object,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
	defer cancel()

	newSession, err := s.agora.NewSession(ctx, fetchContext, id, cid.AccountCID(tesseract.StateHash(id)))
	if err != nil {
		return err
	}
	defer newSession.Close()

	islocal, acc, blk, err := s.fetchAccount(ctx, newSession, tesseract.StateHash(id))
	if err != nil {
		s.logger.Error("Error fetching account data", "err", err)

		return err
	}

	if err = s.syncContextData(ctx, newSession, cid.ContextCID(acc.ContextHash), object); err != nil {
		s.logger.Error("Error fetching context data", "err", err)

		return err
	}

	if err = s.syncAssetTree(ctx, newSession, acc.AssetRoot, object); err != nil {
		return errors.Wrap(err, "failed to sync asset tree")
	}

	if err = s.syncLogicTree(ctx, newSession, acc.LogicRoot, object); err != nil {
		return errors.Wrap(err, "failed to sync logic tree")
	}

	if err = s.syncStorageTree(ctx, newSession, acc.StorageRoot, object); err != nil {
		return errors.Wrap(err, "failed to sync storage tree")
	}

	if err = s.fetchAndStoreData(
		ctx,
		newSession,
		cid.DeedsCID(acc.AssetDeeds),
		cid.AccountKeysCID(acc.KeysHash),
	); err != nil {
		s.logger.Error("Error fetching balance data", "err", err)

		return err
	}

	object.SetAccount(*acc)

	if !islocal {
		if err = s.db.SetAccount(id, tesseract.StateHash(id), blk.GetData()); err != nil {
			return err
		}
	}

	return nil
}

// getBlock retrieves a block of data with the given CID from either the database or from the network using agora
// Returns:
// - found: a boolean value indicating whether the block was found in the database (true) or not (false).
// - block: pointer to the retrieved block
// - err: error if any
func (s *Syncer) getBlock(ctx context.Context, session syncer.Session, cid cid.CID) (bool, *block.Block, error) {
	data, err := s.db.ReadEntry(dbKeyFromCID(session.ID(), cid))
	if err == nil {
		return true, block.NewBlock(cid, data), nil
	}

	if errors.Is(err, common.ErrKeyNotFound) {
		blk, err := session.GetBlock(ctx, cid)

		return false, blk, err
	}

	return false, nil, err
}

func (s *Syncer) getBlocks(ctx context.Context, session syncer.Session, cids ...cid.CID) ([]block.Block, error) {
	blks := make([]block.Block, 0, len(cids))
	keySet := cid.NewHashSet()

	for _, cID := range cids {
		if !cID.IsNil() {
			data, err := s.db.ReadEntry(dbKeyFromCID(session.ID(), cID))
			if err == nil {
				blks = append(blks, *block.NewBlock(cID, data))
			} else {
				keySet.Add(cID)
			}
		}
	}

	if keySet.Len() == 0 {
		return blks, nil
	}

	for blk := range session.GetBlocks(ctx, keySet.Keys()) {
		blks = append(blks, *blk)
	}

	for _, blk := range blks {
		cID := blk.GetCid()
		if keySet.Has(cID) {
			keySet.Remove(cID)
		}
	}

	if keySet.Len() != 0 {
		return nil, errors.New("missing blocks in syncer")
	}

	return blks, nil
}

func (s *Syncer) fetchInteractions(
	ctx context.Context,
	session syncer.Session,
	tsHash common.Hash,
) (
	[]*common.Interaction,
	error,
) {
	ixns := new(common.Interactions)

	rawIxns, err := s.db.GetInteractions(tsHash)
	if err == nil {
		if err = polo.Depolorize(&ixns, rawIxns); err != nil {
			return nil, err
		}

		return ixns.IxList(), nil
	}

	blk, err := session.GetBlock(ctx, cid.InteractionsCID(tsHash))
	if err != nil {
		return nil, err
	}

	data, err := block.DecompressData(blk.GetData(), s.compressor)
	if err != nil {
		return nil, err
	}

	err = polo.Depolorize(ixns, data)
	if err != nil {
		return nil, err
	}

	return ixns.IxList(), err
}

func (s *Syncer) fetchReceipts(
	ctx context.Context,
	session syncer.Session,
	tsHash common.Hash,
) (
	common.Receipts,
	error,
) {
	receipts := new(common.Receipts)

	rawReceipts, err := s.db.GetReceipts(tsHash)
	if err == nil {
		if err := receipts.FromBytes(rawReceipts); err != nil {
			return nil, err
		}

		return *receipts, nil
	}

	blk, err := session.GetBlock(ctx, cid.ReceiptsCID(tsHash))
	if err != nil {
		return nil, err
	}

	data, err := block.DecompressData(blk.GetData(), s.compressor)
	if err != nil {
		return nil, err
	}

	err = receipts.FromBytes(data)

	s.logger.Trace("Fetched receipts through agora", "ts-hash", tsHash)

	return *receipts, err
}

// fetchAccount retrieves the account data for a given state hash from either the local database or the session,
// and returns the account data, along with the block that contains it.
// This also returns a bool value, indicating whether the data was found in the local database (true) or not (false).
func (s *Syncer) fetchAccount(
	ctx context.Context,
	session syncer.Session,
	stateHash common.Hash,
) (
	bool,
	*common.Account,
	*block.Block,
	error,
) {
	islocal, blk, err := s.getBlock(ctx, session, cid.AccountCID(stateHash))
	if err != nil {
		return false, nil, nil, err
	}

	acc := new(common.Account)
	if err = acc.FromBytes(blk.GetData()); err != nil {
		return false, nil, nil, err
	}

	return islocal, acc, blk, nil
}

// fetchAndStoreData retrieves data blocks from the given session object and writes them to the database,
// using the specified CID values as keys.
func (s *Syncer) fetchAndStoreData(ctx context.Context, session syncer.Session, ids ...cid.CID) error {
	keySet := cid.NewHashSet()

	for _, cID := range ids {
		if !cID.IsNil() {
			if ok, err := s.db.Contains(dbKeyFromCID(session.ID(), cID)); !ok && err == nil {
				keySet.Add(cID)
			}
		}
	}

	if keySet.Len() == 0 {
		return nil
	}

	receivedBlocksCount := 0

	blocksChan := session.GetBlocks(ctx, keySet.Keys())
	for blk := range blocksChan {
		if err := s.db.CreateEntry(dbKeyFromCID(session.ID(), blk.GetCid()), blk.GetData()); err != nil {
			s.logger.Error("Error writing to DB", "err", err)

			continue
		}

		receivedBlocksCount++
	}

	if receivedBlocksCount == keySet.Len() {
		return nil
	}

	return errors.New("failed to fetch all keys")
}

// syncContextData fetches the meta context object
func (s *Syncer) syncContextData(ctx context.Context, session syncer.Session, cID cid.CID, object *state.Object) error {
	islocal, blk, err := s.getBlock(ctx, session, cID)
	if err != nil {
		return err
	}

	metaContextObject := new(state.MetaContextObject)
	if err = metaContextObject.FromBytes(blk.GetData()); err != nil {
		return err
	}

	if !islocal {
		if err = s.db.CreateEntry(dbKeyFromCID(session.ID(), cID), blk.GetData()); err != nil {
			return err
		}
	}

	object.SetMetaContextObject(metaContextObject)

	return nil
}

func (s *Syncer) syncStorageTree(
	ctx context.Context,
	session syncer.Session,
	newRoot common.Hash,
	object *state.Object,
) error {
	if newRoot.IsNil() {
		return nil
	}

	_, blk, err := s.getBlock(ctx, session, cid.StorageCID(newRoot))
	if err != nil {
		return err
	}

	metaStorageRoot := new(common.RootNode)
	if err = metaStorageRoot.FromBytes(blk.GetData()); err != nil {
		return err
	}

	var (
		rootHashToLogicID = make(map[cid.CID]string)

		storageTreeRoots = make(map[string]*common.RootNode, len(metaStorageRoot.HashTable))

		storageCIDs = make([]cid.CID, 0, len(metaStorageRoot.HashTable))
	)

	for logicID, storageRoot := range metaStorageRoot.HashTable {
		storageHash := common.BytesToHash(storageRoot)

		if storageHash == common.NilHash {
			continue
		}

		rootCID := cid.StorageCID(storageHash)

		storageCIDs = append(storageCIDs, rootCID)

		rootHashToLogicID[rootCID] = logicID
	}

	if len(storageCIDs) == 0 {
		// sync meta storage tree only
		return s.state.SyncStorageTrees(ctx, metaStorageRoot, storageTreeRoots, object)
	}

	s.logger.Debug("Syncing storage tree", "accountID", session.ID())

	blks, err := s.getBlocks(ctx, session, storageCIDs...)
	if err != nil {
		return err
	}

	for _, b := range blks {
		rootNode := new(common.RootNode)
		if err = polo.Depolorize(&rootNode, b.GetData()); err != nil {
			return err
		}

		logicID, ok := rootHashToLogicID[b.GetCid()]
		if !ok {
			s.logger.Error("Received unwanted block")

			continue
		}

		storageTreeRoots[logicID] = rootNode
	}

	if len(storageTreeRoots) != len(metaStorageRoot.HashTable) {
		return errors.New("failed to fetch storage tree info")
	}

	if err = s.state.SyncStorageTrees(ctx, metaStorageRoot, storageTreeRoots, object); err != nil {
		s.logger.Error("Failed to sync storage tree", "accountID", session.ID())

		return err
	}

	return nil
}

func (s *Syncer) syncLogicManifests(ctx context.Context, as syncer.Session, root *common.RootNode) error {
	cids := make([]cid.CID, 0)

	for _, rawLogicObject := range root.HashTable {
		manifestHash, err := state.GetManifestHashFromRawLogicObject(rawLogicObject)
		if err != nil {
			return err
		}

		cids = append(cids, cid.ManifestCID(manifestHash))
	}

	blks, err := s.getBlocks(ctx, as, cids...)
	if err != nil {
		return err
	}

	for _, blck := range blks {
		if err := s.db.CreateEntry(dbKeyFromCID(as.ID(), blck.GetCid()), blck.GetData()); err != nil {
			return err
		}
	}

	for _, cID := range cids {
		if stored, err := s.db.Contains(dbKeyFromCID(as.ID(), cID)); err != nil || !stored {
			s.logger.Error("Failed to fetch logic manifest", "accountID", as.ID(), "manifest-hash", cID.String())

			return errors.New("failed to fetch logic manifest")
		}
	}

	return nil
}

func (s *Syncer) syncAssetTree(
	ctx context.Context,
	as syncer.Session,
	newRoot common.Hash,
	object *state.Object,
) error {
	if newRoot.IsNil() {
		return nil
	}

	_, blk, err := s.getBlock(ctx, as, cid.AssetCID(newRoot))
	if err != nil {
		return nil
	}

	metaAssetRoot := new(common.RootNode)
	if err = metaAssetRoot.FromBytes(blk.GetData()); err != nil {
		return err
	}

	return s.state.SyncAssetTree(metaAssetRoot, object)
}

func (s *Syncer) syncLogicTree(
	ctx context.Context,
	as syncer.Session,
	newRoot common.Hash,
	object *state.Object,
) error {
	if newRoot.IsNil() {
		return nil
	}

	_, blk, err := s.getBlock(ctx, as, cid.LogicCID(newRoot))
	if err != nil {
		return nil
	}

	metaLogicRoot := new(common.RootNode)
	if err = metaLogicRoot.FromBytes(blk.GetData()); err != nil {
		return err
	}

	if err = s.syncLogicManifests(ctx, as, metaLogicRoot); err != nil {
		return err
	}

	return s.state.SyncLogicTree(metaLogicRoot, object)
}

func mergeIxns(ixHashes []common.Hash, ixns, nodeIxns []*common.Interaction) ([]*common.Interaction, error) {
	result := make([]*common.Interaction, 0, len(ixHashes))

	i, j := 0, 0

	for _, ixHash := range ixHashes {
		switch {
		case i < len(ixns) && ixns[i].Hash() == ixHash:
			result = append(result, ixns[i])
			i++
		case j < len(nodeIxns) && nodeIxns[j].Hash() == ixHash:
			result = append(result, nodeIxns[j])
			j++
		default:
			return nil, errors.New(fmt.Sprintf("ix-hash %v not found in node ixns", ixHash))
		}
	}

	return result, nil
}

func (s *Syncer) FetchIxns(tsInfo *TesseractInfo) ([]*common.Interaction, error) {
	ixns, missingIXHashes := s.ixpool.GetIxnsWithMissingIxns(tsInfo.ixnsHashes)
	if len(missingIXHashes) == 0 {
		return ixns, nil
	}

	s.logger.Trace("Ixns not found in ixpool", "ixns-hashes", missingIXHashes)
	s.metrics.AddIxMissCount(float64(len(missingIXHashes)))

	ts := tsInfo.tesseract

	randomNumber := rand.New(rand.NewSource(time.Now().UnixNano()))

	validator, err := s.state.GetSystemObject().Validator(
		uint64(ts.CommitInfo().RandomSet[randomNumber.Intn(len(ts.CommitInfo().RandomSet))]),
	)
	if err != nil {
		return nil, err
	}

	resp, err := s.RPCGetIxns(validator.KramaID, ts.Hash(), missingIXHashes)
	if err != nil {
		return nil, err
	}

	s.logger.Trace("fetched Ixns through rpc", "ixns-hashes", missingIXHashes, "peer-id", validator.KramaID)

	fetchedIxns := new(common.Interactions)

	if err := fetchedIxns.FromBytes(resp.Ixns); err != nil {
		return nil, err
	}

	if len(missingIXHashes) != len(fetchedIxns.IxList()) {
		return nil, errors.New(fmt.Sprintf("fetched ixn hash count doesn't match the "+
			"requested count expected: %v, actual %v", len(missingIXHashes), len(fetchedIxns.IxList())))
	}

	return mergeIxns(tsInfo.ixnsHashes, ixns, fetchedIxns.IxList())
}

func (s *Syncer) fillTSWithIxnsAndReceipts(tsInfo *TesseractInfo) error {
	var (
		ixns []*common.Interaction
		err  error
	)

	s.IxFetchLock.Lock()
	ts := tsInfo.tesseract

	if _, ok := s.IxFetchGrid[ts.Hash()]; ok {
		s.IxFetchLock.Unlock()

		return errors.New("another job is fetching ixns and receipts")
	}

	fetchIxns := ts.Interactions().Len() == 0
	fetchReceipts := !tsInfo.shouldExecute && len(ts.Receipts()) == 0

	if !fetchIxns && !fetchReceipts {
		s.IxFetchLock.Unlock()

		return nil
	}

	s.IxFetchGrid[ts.Hash()] = struct{}{}

	s.IxFetchLock.Unlock()

	defer func() {
		s.IxFetchLock.Lock()
		delete(s.IxFetchGrid, ts.Hash())
		s.IxFetchLock.Unlock()
	}()

	// retrieve ixns if they are not available
	if fetchIxns {
		if ixns, err = s.FetchIxns(tsInfo); err != nil {
			return err
		}

		ts.SetIxns(common.NewInteractionsWithLeaderCheck(true, ixns...))
	}

	// retrieve receipts only when ts execution is not needed and receipts are not available
	if fetchReceipts {
		err = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
			defer cancel()

			peers, err := s.state.GetSystemObject().GetValidatorKramaIDs(tsInfo.tesseract.CommitInfo().RandomSet)
			if err != nil {
				return errors.Wrap(err, "failed to retrieve random set krama ids")
			}

			newSession, err := s.agora.NewSession(
				ctx,
				peers,
				tsInfo.id(),
				cid.AccountCID(ts.StateHash(tsInfo.id())),
			)
			if err != nil {
				return errors.Wrap(err, "unable to create session")
			}
			defer newSession.Close()

			receipts, err := s.fetchReceipts(ctx, newSession, ts.Hash())
			if err != nil {
				return errors.Wrap(err, "unable to fetch receipts through agora")
			}

			ts.SetReceipts(receipts)

			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Syncer) closePendingMsgChan() {
	s.init.Do(func() {
		close(s.pendingMsgChan)
	})
}

func (s *Syncer) handleTSInfo(tsInfo *TesseractInfo) {
	if s.cfg.ShouldExecute {
		s.tsJobQueue.push(tsInfo.CreateTSInfoWithAddr(tsInfo.tesseract.AnyAccountID()))

		return
	}

	for id, ps := range tsInfo.tesseract.Participants() {
		info := tsInfo.CreateTSInfoWithAddr(id)

		if ps.StateHash.IsNil() {
			continue
		}

		lockType, ok := tsInfo.tesseract.ConsensusInfo().AccountLocks[id]
		if ok && (lockType > common.MutateLock) {
			continue
		}

		s.logger.Debug("need to sync ", "id", info.id(), "height", info.height())

		peers, err := s.state.GetSystemObject().GetValidatorKramaIDs(info.tesseract.CommitInfo().RandomSet)
		if err != nil {
			log.Println("Failed to find peers", info.tesseract.CommitInfo().RandomSet)
			s.logger.Error("failed to retrieve random set krama ids",
				"id", id, "err", err)

			continue
		}

		if err := s.NewSyncRequest(
			info.id(),
			info.height(),
			common.LatestSync,
			peers,
			false,
			nil,
			info,
		); err != nil {
			s.logger.Error("Error adding sync request", "err", err)
		}
	}
}

func (s *Syncer) msgHandler(msg *pubsub.Message) error {
	if msg.ValidatorData == nil {
		return errors.New("tesseract info not found")
	}

	data := msg.ValidatorData

	tsInfo, ok := data.(*TesseractInfo)
	if !ok {
		return errors.New("failed to type cast validator data to tesseract info")
	}

	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
		if !s.isInitialSyncDone() {
			s.pendingMsgChan <- tsInfo

			return nil
		}

		defer s.closePendingMsgChan()

		s.handleTSInfo(tsInfo)
	}

	return nil
}

func (s *Syncer) getTesseractWithRawIxnsAndReceipts(
	id identifiers.Identifier,
	height uint64,
	withInteractions, withReceipts bool,
) (ts *common.Tesseract, ixns, receipts []byte, err error) {
	ts, err = s.lattice.GetTesseractByHeight(id, height, false, false)
	if err != nil {
		return nil, nil, nil, err
	}

	if withInteractions && !ts.InteractionsHash().IsNil() {
		ixns, err = s.db.GetInteractions(ts.Hash())
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to load interactions")
		}
	}

	if withReceipts && !ts.ReceiptsHash().IsNil() {
		receipts, err = s.db.GetReceipts(ts.Hash())
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to load receipts")
		}
	}

	return ts, ixns, receipts, nil
}

// TesseractValidator is a custom validation logic to verify tesseract message
func (s *Syncer) TesseractValidator(
	ctx context.Context,
	pid peer.ID,
	msg *pubsub.Message,
) (pubsub.ValidationResult, error) {
	var (
		tsMsg = new(networkmsg.TesseractMsg)
		err   error
	)

	peerID, err := s.network.GetKramaID().DecodedPeerID()
	if err != nil {
		return pubsub.ValidationReject, err
	}

	if msg.GetFrom() == peerID {
		return pubsub.ValidationAccept, nil
	}

	// depolorize tesseract message
	if err = tsMsg.FromBytes(msg.GetData()); err != nil {
		return pubsub.ValidationReject, err
	}

	if err = tsMsg.DecompressTesseract(s.compressor); err != nil {
		return pubsub.ValidationReject, err
	}

	// get tesseract from tesseract message
	ts, err := tsMsg.GetTesseract()
	if err != nil {
		return pubsub.ValidationReject, err
	}

	// verify if tesseract has valid seal
	validSeal, err := s.state.IsSealValid(ts)
	if !validSeal || err != nil {
		return pubsub.ValidationReject, err
	}

	if err := s.mux.Post(utils.TSTrackerEvent{
		TSHash:     ts.Hash(),
		ExpiryTime: time.Now().Add(1 * time.Second),
	}); err != nil {
		s.logger.Error("Error posting tesseract tracker event", "ts-hash", ts.Hash())
	}

	// check db if tesseract already exists
	exists := s.db.HasTesseract(ts.Hash())
	if exists {
		return pubsub.ValidationIgnore, nil
	}

	// check cache if teserract already exists
	if s.tesseractRegistry.Contains(ts.Hash()) {
		return pubsub.ValidationIgnore, nil
	}

	// add tesseract to cache
	s.tesseractRegistry.Add(ts.Hash())

	s.logger.Debug(
		"ts received from",
		"sender", pid,
		"sealer", ts.SealBy(),
		"ts-hash", ts.Hash(),
	)

	tsInfo := &TesseractInfo{
		tesseract:     ts,
		committee:     nil,
		shouldExecute: s.cfg.ShouldExecute,
		ixnsHashes:    tsMsg.IxnsHashes,
		delta:         tsMsg.Extra,
	}

	msg.ValidatorData = tsInfo

	return pubsub.ValidationAccept, nil
}

// Start starts all event handlers and workers associated with sync sub protocol
func (s *Syncer) Start(minConnectedPeers int) error {
	s.logger.Info("Starting Syncer")

	if err := s.addSyncPeersToPeerstore(); err != nil {
		return err
	}

	sub := s.mux.Subscribe(utils.PendingAccountEvent{})
	go s.startPendingAccountEventHandler(sub)

	s.agora.Start()

	if err := s.registerRPCService(); err != nil {
		return err
	}

	if err := s.network.Subscribe(s.ctx, config.TesseractTopic, s.TesseractValidator, false, s.msgHandler); err != nil {
		return err
	}

	go s.queueHandler()

	for s.network.GetPeerSetLen() < minConnectedPeers {
		time.Sleep(1 * time.Second)
	}

	s.logger.Info("Connected to minimum number of required peers", "count", minConnectedPeers)

	s.startAccountWorkers()
	s.startTesseractWorkers()

	go func() {
		if err := s.initSync(); err != nil {
			s.logger.Error("Initial sync failed", "err", err)

			return
		}

		for {
			select {
			case <-s.ctx.Done():
				s.logger.Error("Initial sync failed", "err", s.ctx.Err())

				return

			case <-time.After(500 * time.Millisecond):
				s.logger.Debug("Sync in progress", "pending jobs", s.accountJobQueue.len())
			}

			if s.accountJobQueue.len() == 0 {
				s.setInitialSyncDone(true)
				s.logger.Info("Initial sync successful")

				return
			}
		}
	}()

	go s.startSyncEventHandler()

	return nil
}

func (s *Syncer) startPendingAccountEventHandler(sub *utils.Subscription) {
	s.tracker.StartSyncStatusTracker(s.ctx, sub)
}

func (s *Syncer) startSyncEventHandler() {
	sub := s.mux.Subscribe(utils.SyncRequestEvent{})
	defer sub.Unsubscribe()

	for event := range sub.Chan() {
		req, ok := event.Data.(utils.SyncRequestEvent)
		if ok {
			if !s.isInitialSyncDone() {
				continue
			}

			if err := s.NewSyncRequest(
				req.ID,
				req.Height,
				common.LatestSync,
				[]identifiers.KramaID{req.BestPeer},
				false,
				nil,
			); err != nil {
				s.logger.Error("Failed to handle sync request from consensus engine", "err", err)
			}
		}
	}
}

// GetAccountSyncStatus returns the sync status of an account
func (s *Syncer) GetAccountSyncStatus(id identifiers.Identifier) (*args.AccSyncStatus, error) {
	var currentHeight, expectedHeight uint64

	job, ok := s.accountJobQueue.getJob(id)
	if !ok {
		accountInfo, err := s.db.GetAccountMetaInfo(id)
		if err != nil {
			return nil, err
		}

		currentHeight = accountInfo.Height
		expectedHeight = 0
	} else {
		currentHeight = job.currentHeight
		expectedHeight = job.expectedHeight
	}

	isPrimarySyncDone := s.db.IsAccountPrimarySyncDone(id)

	return &args.AccSyncStatus{
		CurrentHeight:     hexutil.Uint64(currentHeight),
		ExpectedHeight:    hexutil.Uint64(expectedHeight),
		IsPrimarySyncDone: isPrimarySyncDone,
	}, nil
}

// GetSyncJobInfo returns the sync job meta info for given accountID
func (s *Syncer) GetSyncJobInfo(id identifiers.Identifier) (*args.SyncJobInfo, error) {
	if id.IsNil() {
		return nil, common.ErrInvalidIdentifier
	}

	job, ok := s.accountJobQueue.getJob(id)
	if !ok {
		return nil, common.ErrSyncJobNotFound
	}

	kramaIDs := utils.ConvertMapToSlice(job.bestPeers)

	return &args.SyncJobInfo{
		SyncMode:              job.mode.String(),
		SnapDownloaded:        job.snapDownloaded,
		ExpectedHeight:        job.expectedHeight,
		CurrentHeight:         job.currentHeight,
		JobState:              job.jobState.String(),
		LastModifiedAt:        job.lastModifiedAt,
		TesseractQueueLen:     job.tesseractQueue.Len(),
		BestPeers:             kramaIDs,
		LatticeSyncInProgress: job.latticeSyncInProgress,
	}, nil
}

// GetNodeSyncStatus returns the node sync status
func (s *Syncer) GetNodeSyncStatus(includePendingAccounts bool) *args.NodeSyncStatus {
	isPrincipalSyncDone, principalSyncTimeStamp := s.db.IsPrincipalSyncDone()
	totalPendingAccounts := s.tracker.ReadPendingAccounts()

	nodeSyncStatus := &args.NodeSyncStatus{
		TotalPendingAccounts:  hexutil.Uint64(totalPendingAccounts),
		IsPrincipalSyncDone:   isPrincipalSyncDone,
		PrincipalSyncDoneTime: hexutil.Uint64(principalSyncTimeStamp),
		IsInitialSyncDone:     s.isInitialSyncDone(),
	}

	if includePendingAccounts {
		nodeSyncStatus.PendingAccounts = s.accountJobQueue.getPendingAccounts()
		nodeSyncStatus.PendingTesseractHash = s.tsJobQueue.getPendingTesseractHashes()
	}

	return nodeSyncStatus
}

// startAccountWorkers will start the sync job workers
func (s *Syncer) startAccountWorkers() {
	for i := uint32(0); i < s.accountWorkerCount; i++ {
		go s.accountWorker()
	}
}

func (s *Syncer) Close() {
	s.ctxCancel()

	s.agora.Close()
}

func (s *Syncer) queueHandler() {
	for {
		select {
		case <-s.ctx.Done():
			return

		case msg, ok := <-s.pendingMsgChan:
			if !ok {
				for _, tsInfo := range s.pendingMsgQueue {
					s.handleTSInfo(tsInfo)
				}

				s.pendingMsgQueue = nil

				return
			}

			s.pendingMsgQueue = append(s.pendingMsgQueue, msg)
		}
	}
}

func (s *Syncer) fetchContextForAgora(id identifiers.Identifier, ts common.Tesseract) ([]identifiers.KramaID, error) {
	peers := make([]identifiers.KramaID, 0)

	for {
		if len(peers) >= 10 {
			break
		}

		// fetch the context delta
		deltaGroup, _ := ts.GetContextDelta(id)
		// add the delta peers to the list
		peers = append(peers, deltaGroup.ConsensusNodes...)

		consensusNodes, err := s.state.GetConsensusNodesByHash(id, ts.LockedContextHash(id))
		if err == nil {
			peers = append(peers, consensusNodes...)

			break
		}

		if ts.TransitiveLink(id).IsNil() {
			break
		}

		t, err := s.lattice.GetTesseract(ts.TransitiveLink(id), false, false)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}

		ts = *t
	}

	return peers, nil
}

func (s *Syncer) post(ev interface{}) error {
	return s.mux.Post(ev)
}

func (s *Syncer) publishEventLoadSyncJobsDB() error {
	return s.post(eventLoadSyncJobsDB{})
}

func (s *Syncer) publishEventBucketSync() error {
	return s.post(eventBucketSync{})
}

func (s *Syncer) publishEventSystemAccounts() error {
	return s.post(utils.SystemAccountsSyncedEvent{})
}

func (s *Syncer) publishEventSnapSync(state eventDataJobState) error {
	return s.post(eventSnapSync{state})
}

func (s *Syncer) publishEventLatticeSync(state eventDataJobState) error {
	return s.post(eventLatticeSync{state})
}

func (s *Syncer) publishEventTesseractSync(id identifiers.Identifier, height uint64) error {
	return s.post(
		eventTesseractSync{
			eventDataJobState{
				id:     id,
				height: height,
			},
		},
	)
}

func dbKeyFromCID(id identifiers.Identifier, cid cid.CID) []byte {
	return storage.DBKey(id, storage.PrefixTag(cid.ContentType()), cid.Key())
}

func connTag(id identifiers.Identifier, service string) string {
	return fmt.Sprintf("%s:%s", service, id.Hex())
}

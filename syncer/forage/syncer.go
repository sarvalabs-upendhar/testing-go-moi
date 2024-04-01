package forage

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
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
	TesseractFetchTimeOut = 15 * time.Second
	DefaultWorkerWaitTime = 500 * time.Millisecond
)

var DefaultMinConnectedPeers = 6

type lattice interface {
	ExecuteAndValidate(ts *common.Tesseract) error
	AddTesseractWithState(
		addr identifiers.Address,
		dirtyStorage map[common.Hash][]byte,
		ts *common.Tesseract,
		allParticipants bool,
	) error
	GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error)
	GetTesseractByHeight(
		address identifiers.Address,
		height uint64,
		withInteractions bool,
	) (*common.Tesseract, error)
	GetTesseractHeightEntry(address identifiers.Address, height uint64) (common.Hash, error)
	ValidateTesseract(addr identifiers.Address, ts *common.Tesseract, ics *common.ICSNodeSet, allParticipants bool) error
	IsInitialTesseract(ts *common.Tesseract, addr identifiers.Address) (bool, error)
	IsSealValid(ts *common.Tesseract) (bool, error)
}

type stateManager interface {
	SyncStorageTrees(
		ctx context.Context,
		address identifiers.Address,
		newRoot *common.RootNode,
		logicStorageTreeRoots map[string]*common.RootNode,
	) error
	SyncLogicTree(
		address identifiers.Address,
		newRoot *common.RootNode,
	) error
	CreateDirtyObject(
		addr identifiers.Address,
		accType common.AccountType,
	) *state.Object
	GetParticipantContextRaw(
		address identifiers.Address,
		hash common.Hash,
		rawContext map[string][]byte,
	) error
	FetchICSNodeSet(
		ts *common.Tesseract,
		info *common.ICSClusterInfo,
	) (*common.ICSNodeSet, error)
	GetICSNodeSetFromRawContext(
		ts *common.Tesseract,
		rawContext map[string][]byte,
		clusterInfo *common.ICSClusterInfo,
	) (*common.ICSNodeSet, error)
	HasParticipantStateAt(addr identifiers.Address, stateHash common.Hash) bool
}

type store interface {
	NewBatchWriter() db.BatchWriter
	CreateEntry([]byte, []byte) error
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	SetAccount(addr identifiers.Address, stateHash common.Hash, data []byte) error
	SetInteractions(tsHash common.Hash, data []byte) error
	SetReceipts(tsHash common.Hash, data []byte) error
	GetInteractions(tesseractHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id identifiers.Address) (*common.AccountMetaInfo, error)
	UpdateTesseractStatus(addr identifiers.Address, height uint64, tsHash common.Hash) error
	SetAccountSyncStatus(address identifiers.Address, status *common.AccountSyncStatus) error
	CleanupAccountSyncStatus(address identifiers.Address) error
	StoreAccountSnapShot(snap *common.Snapshot) error
	GetReceipts(tsHash common.Hash) ([]byte, error)
	GetAccountsSyncStatus() ([]*common.AccountSyncStatus, error)
	DropPrefix(prefix []byte) error
	UpdatePrimarySyncStatus(address identifiers.Address) error
	IsAccountPrimarySyncDone(address identifiers.Address) bool
	HasTesseract(tsHash common.Hash) bool
	SetTesseract(tsHash common.Hash, data []byte) error
	UpdatePrincipalSyncStatus() error
	GetBucketCount(bucketNumber uint64) (uint64, error)
	StreamAccountMetaInfosRaw(ctx context.Context, bucketNumber uint64, response chan []byte) error
	GetRecentUpdatedAccMetaInfosRaw(ctx context.Context, bucketID uint64, sinceTS uint64) ([][]byte, error)
	IsPrincipalSyncDone() (bool, int64)
	StreamSnapshot(
		ctx context.Context,
		address identifiers.Address,
		sinceTS uint64,
		respChan chan<- common.SnapResponse,
	) (uint64, error)
	UpdateAccMetaInfo(
		id identifiers.Address,
		height uint64,
		tesseractHash common.Hash,
		accType common.AccountType,
	) (int32, bool, error)
	SetTesseractHeightEntry(addr identifiers.Address, height uint64, tsHash common.Hash) error
	HasAccMetaInfoAt(addr identifiers.Address, height uint64) bool
	GetAccount(addr identifiers.Address, stateHash common.Hash) ([]byte, error)
}

type ixpool interface {
	GetIxns(ixHashes common.Hashes) (common.Interactions, error)
}

type p2pServer interface {
	GetPeers() []kramaid.KramaID
	StartNewRPCServer(protocol protocol.ID, tag string) *rpc.Client
	RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error
	GetKramaID() kramaid.KramaID
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

type Syncer struct {
	lock                sync.RWMutex
	cfg                 *config.SyncerConfig
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	network             p2pServer
	mux                 *utils.TypeMux
	execLock            sync.RWMutex
	agora               syncer.BlockSync
	db                  store
	tesseractRegistry   *common.HashRegistry
	jobQueue            *JobQueue
	rpcClient           *rpc.Client
	lattice             lattice
	state               stateManager
	ixpool              ixpool
	logger              hclog.Logger
	workerLock          sync.Mutex
	jobWorkerCount      uint32
	workerSignal        chan struct{}
	isPrincipalSyncDone bool
	bucketSyncDone      bool
	pendingAccounts     uint64
	consensusSlots      *ktypes.Slots
	lastActiveTimeStamp uint64
	accountsLock        sync.RWMutex
	lockedAccounts      map[identifiers.Address]struct{}
	metrics             *Metrics
	initialSyncDone     bool
	pendingMsgChan      chan *TesseractInfo
	pendingMsgQueue     []*TesseractInfo
	init                sync.Once
	execGrid            map[common.Hash]struct{}
	tracker             *SyncStatusTracker
	workerWaitTime      time.Duration
	trustedPeersPresent bool
	IxFetchGrid         map[common.Hash]struct{}
	IxFetchLock         sync.Mutex
}

func NewSyncer(
	cfg *config.SyncerConfig,
	logger hclog.Logger,
	node *p2p.Server,
	mux *utils.TypeMux,
	db store,
	lattice lattice,
	sm stateManager,
	ixpool ixpool,
	slots *ktypes.Slots,
	lastActiveTimeStamp uint64,
	syncerMetrics *Metrics,
	blockSync syncer.BlockSync,
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
		lattice:             lattice,
		state:               sm,
		ixpool:              ixpool,
		jobWorkerCount:      10,
		workerWaitTime:      DefaultWorkerWaitTime,
		jobQueue:            NewJobQueue(mux),
		logger:              logger.Named("Syncer"),
		workerSignal:        make(chan struct{}),
		tesseractRegistry:   common.NewHashRegistry(60),
		consensusSlots:      slots,
		lastActiveTimeStamp: lastActiveTimeStamp,
		lockedAccounts:      make(map[identifiers.Address]struct{}, 0),
		metrics:             syncerMetrics,
		pendingMsgQueue:     make([]*TesseractInfo, 0),
		pendingMsgChan:      make(chan *TesseractInfo, 10),
		execGrid:            make(map[common.Hash]struct{}),
		tracker:             NewSyncStatusTracker(0),
		trustedPeersPresent: len(cfg.TrustedPeers) > 0,
		IxFetchGrid:         make(map[common.Hash]struct{}),
	}

	return s, nil
}

func (s *Syncer) addTrustedPeersToPeerstore() error {
	for i := 0; i < len(s.cfg.TrustedPeers); i++ {
		bestPeer := s.cfg.TrustedPeers[i]

		peerID, err := bestPeer.ID.DecodedPeerID()
		if err != nil {
			s.logger.Error("Failed to get peer ID from krama ID", "krama-ID", bestPeer.ID, "err", err)

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
	addr identifiers.Address,
	expectedHeight uint64,
	syncMode common.SyncMode,
	bestPeers []kramaid.KramaID,
	snapDownloaded bool,
	tesseracts ...*TesseractInfo,
) (err error) {
	job, ok := s.jobQueue.getJob(addr)
	if job == nil {
		job = &SyncJob{
			db:              s.db,
			logger:          s.logger,
			address:         addr,
			mode:            syncMode,
			tesseractQueue:  NewTesseractQueue(),
			jobState:        Pending,
			snapDownloaded:  snapDownloaded,
			tesseractSignal: make(chan struct{}, 1),
			bestPeers:       make(map[kramaid.KramaID]struct{}),
		}

		job.updateBestPeers(bestPeers)

		metaInfo, err := s.db.GetAccountMetaInfo(addr)
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

	tsHash, _ := s.lattice.GetTesseractHeightEntry(job.address, job.getCurrentHeight())
	if job.getCurrentHeight() == job.getExpectedHeight() && tsHash != common.NilHash {
		s.logger.Debug("Tesseract found, avoiding new sync request")

		return nil
	}

	if !s.hasTrustedPeers() && syncMode == common.FullSync {
		var height uint64

		height, bestPeers, err = s.findLatestHeightAndBestPeers(addr)
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
		if err = s.jobQueue.AddJob(job); err != nil {
			return err
		}

		s.metrics.captureTotalJobs(float64(len(s.jobQueue.jobs)))

		if err = job.commitJob(); err != nil {
			return errors.Wrap(err, "failed to commit job")
		}

		s.signalNewJob()
	}

	return nil
}

func (s *Syncer) worker() {
	defer func() {
		s.workerLock.Lock()
		s.jobWorkerCount--
		s.workerLock.Unlock()
		s.logger.Debug("Closing syncer worker")
	}()

	for {
		select {
		case <-s.workerSignal:
		case <-time.After(s.workerWaitTime):
		case <-s.ctx.Done():
			return
		}

		job := s.jobQueue.NextJob()

		s.metrics.captureTotalJobs(float64(s.jobQueue.len()))

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

func (s *Syncer) hasTrustedPeers() bool {
	return s.trustedPeersPresent
}

func (s *Syncer) jobClosure(job *SyncJob) error {
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
	bestPeer kramaid.KramaID,
	addr identifiers.Address,
) (*LatestAccountInfo, error) {
	resp := new(LatestAccountInfo)

	if err := s.rpcClient.MoiCall(
		context.Background(),
		bestPeer,
		"SYNCRPC",
		"GetLatestAccountInfo",
		addr,
		resp,
		5*time.Second,
	); err != nil {
		s.logger.Error("Failed to fetch account latest status",
			"RPC-error", err, "krama-id", bestPeer, "address", addr)

		return nil, err
	}

	return resp, nil
}

func (s *Syncer) chooseBestPeersForInitialSync(addr identifiers.Address) (uint64, []kramaid.KramaID, error) {
	if s.hasTrustedPeers() {
		for i := 0; i < 3; i++ {
			bestPeer := s.chooseAnyTrustedPeer()

			resp, err := s.RPCGetLatestAccountInfo(bestPeer.ID, addr)
			if err != nil {
				continue
			}

			return resp.Height, []kramaid.KramaID{bestPeer.ID}, nil
		}

		return 0, nil, errors.New("unable to fetch latest account info from trusted peers")
	}

	bestHeight, bestPeers, err := s.findLatestHeightAndBestPeers(addr)
	if err != nil {
		return 0, nil, err
	}

	return bestHeight, bestPeers, nil
}

func (s *Syncer) chooseBestPeer(job *SyncJob) (kramaid.KramaID, error) {
	// If initial sync is not done, then choose best peer from trusted peers
	// to improve probability of success in syncing
	if !s.isInitialSyncDone() && s.hasTrustedPeers() {
		bestPeer := s.chooseAnyTrustedPeer()

		return bestPeer.ID, nil
	}

	if job.bestPeerLen() > 0 {
		return job.chooseRandomBestPeer(), nil
	}

	return s.chooseBestSyncPeer(job)
}

func (s *Syncer) jobProcessor(job *SyncJob) error {
	var (
		err      error
		jobState = job.getJobState()
	)

	s.logger.Debug(
		"Processing new job",
		"addr", job.address,
		"current-height", job.currentHeight,
		"expected-height", job.getExpectedHeight(),
	)

	defer func() {
		if err = s.jobClosure(job); err != nil {
			log.Fatal(err)
		}
	}()

	if s.consensusSlots.AreAccountsActive(job.address) {
		s.logger.Debug("Account is active job state set to sleep")
		job.updateJobState(Sleep)

		return nil
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

	if !job.snapDownloaded && s.isSnapSyncRequired(job.address) && job.mode != common.LatestSync {
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

			if !s.db.HasAccMetaInfoAt(tsInfo.address(), tsInfo.height()) {
				initial, err := s.lattice.IsInitialTesseract(
					tsInfo.tesseract,
					tsInfo.address(),
				)
				if err != nil {
					jobState = Sleep

					return nil
				}

				if !initial && tsInfo.height() != job.getCurrentHeight()+1 {
					s.logger.Error(
						"Missing tesseract",
						"addr", tsInfo.address(),
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
func (s *Syncer) postAdditionHook(job *SyncJob, newHeight uint64) (bool, error) {
	if job.getCurrentHeight() < newHeight {
		job.updateCurrentHeight(newHeight)
	}

	if job.getExpectedHeight() != newHeight {
		return false, nil
	}

	if job.mode == common.FullSync {
		if err := s.db.UpdatePrimarySyncStatus(job.address); err != nil {
			return false, errors.Wrap(err, "failed to update account primary sync status")
		}
	}

	if err := s.updatePrincipalSyncStatus(); err != nil {
		return false, errors.Wrap(err, "failed to update principal sync status")
	}

	return true, nil
}

func (s *Syncer) signalNewJob() {
	select {
	case s.workerSignal <- struct{}{}:
	default:
		s.logger.Error("Failed to signal new job")
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
func (s *Syncer) cleanGridAndReleasePendingJobs(tsInfo *TesseractInfo, job *SyncJob) error {
	if tsInfo.tesseract.GridLength() == 1 {
		return nil
	}

	grid := s.gridStore.GetGrid(tsInfo.tesseract.GridHash())
	if grid == nil {
		return errors.New("grid not found")
	}

	for _, ts := range grid.ts {
		if ts.Address() == job.address {
			continue
		}

		pendingJob, ok := s.jobQueue.getJob(ts.Address())
		if !ok {
			return fmt.Errorf(" %s job not found", ts.Address())
		}

		if err := s.releasePendingJob(pendingJob, ts); err != nil {
			s.logger.Error("Failed to update pending job status", "err", err)
		}
	}

	s.gridStore.CleanupGrid(tsInfo.tesseract.GridHash())

	return nil
}

// releasePendingJob pops the added tesseract and updates the job state
func (s *Syncer) releasePendingJob(job *SyncJob, ts *types.Tesseract) error {
	queuedTSInfo := job.tesseractQueue.Pop()
	if queuedTSInfo.tesseract.Height() != ts.Height() {
		return errors.New("height mismatch")
	}

	shouldExit, err := s.postAdditionHook(job, ts.Height())
	if err != nil || shouldExit {
		return err
	}

	job.updateJobState(Pending)

	return nil
}

*/

func getBestPeers(heightPeersMap map[uint64][]kramaid.KramaID) (uint64, []kramaid.KramaID, error) {
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
func (s *Syncer) findLatestHeightAndBestPeers(addr identifiers.Address) (uint64, []kramaid.KramaID, error) {
	heightPeersMap := make(map[uint64][]kramaid.KramaID)

	// index tracks the no of peers responded
	index := 0

	for _, kramaID := range s.network.GetPeers() {
		if index == MaxPeersToDial {
			break
		}

		resp, err := s.RPCGetLatestAccountInfo(kramaID, addr)
		if err != nil {
			continue
		}

		index++

		nodes, ok := heightPeersMap[resp.Height]
		if !ok {
			heightPeersMap[resp.Height] = make([]kramaid.KramaID, 0)
		}

		nodes = append(nodes, kramaID)
		heightPeersMap[resp.Height] = nodes
	}

	return getBestPeers(heightPeersMap)
}

func (s *Syncer) chooseBestSyncPeer(job *SyncJob) (kramaid.KramaID, error) {
	if job.mode == common.LatestSync && job.tesseractQueue.Peek() != nil {
		randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
		randNumber := randomSource.Intn(len(job.tesseractQueue.Peek().clusterInfo.RandomSet))

		return job.tesseractQueue.Peek().clusterInfo.RandomSet[randNumber], nil
	}

	_, bestPeers, err := s.findLatestHeightAndBestPeers(job.address)
	if err != nil {
		return "", err
	}

	job.updateBestPeers(bestPeers)

	return bestPeers[0], nil
}

func (s *Syncer) chooseAnyTrustedPeer() config.NodeInfo {
	randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
	randNumber := randomSource.Intn(len(s.cfg.TrustedPeers))

	return s.cfg.TrustedPeers[randNumber]
}

// syncSystemAccount sends a sync request for the specified address and waits for it to complete within a given time.
// If the sync does not complete within the specified time, an error is returned.
func (s *Syncer) syncSystemAccount(address ...identifiers.Address) ([]kramaid.KramaID, error) {
	var (
		bestPeers  []kramaid.KramaID
		bestHeight uint64
		err        error
	)

	for _, addr := range address {
		bestHeight, bestPeers, err = s.chooseBestPeersForInitialSync(addr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch best peers and height")
		}

		if err = s.NewSyncRequest(addr, bestHeight, common.FullSync, bestPeers, false); err != nil {
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

				metaInfo, err := s.db.GetAccountMetaInfo(addr)
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
	bestPeers, err := s.syncSystemAccount(common.GuardianLogicAddr, common.SargaAddress)
	if err != nil {
		s.logger.Error("Failed to sync system account", "err", err)

		return err
	}

	if err := s.publishEventSystemAccounts(); err != nil {
		s.logger.Error("failed to publish event system accounts sync", "err", err)
	}

	s.logger.Info("System accounts sync successful")

	if err = s.loadSyncJobsFromDB(); err != nil {
		s.logger.Error("Failed to load sync jobs from DB", "err", err)
	} else if err = s.publishEventLoadSyncJobsDB(); err != nil {
		s.logger.Error("failed to publish event load sync jobs from DB", "err", err)
	}

	return s.syncBucketsWithMaxAttempts(bestPeers, MaxBucketSyncAttempts)
}

func (s *Syncer) syncBucketsWithMaxAttempts(bestPeers []kramaid.KramaID, maxAttempts int) error {
	for i := 1; i < maxAttempts+1; i++ {
		randomNumber := rand.New(rand.NewSource(time.Now().UnixNano()))
		bestPeer := bestPeers[randomNumber.Intn(len(bestPeers))]

		requestTime := time.Now()

		if err := s.syncBuckets(bestPeer, i); err != nil {
			s.logger.Error("Failed to sync buckets, retrying...!!!", "err", err)
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
func (s *Syncer) syncBucketSince(kramaID id.KramaID, sinceTs uint64) error {
	var (
		argsChan = make(chan *BucketSyncRequest, 1)
		respChan = make(chan *BucketSyncResponse, ChannelBufferSize)
	)

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		s.logger.Error("Failed to decode peer ID", "err", err)
	}

	errGrp, grpCtx := errgroup.WithContext(s.ctx)

	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(grpCtx, peerID, "SYNCRPC", "SyncBucketsSince", argsChan, respChan); err != nil {
			s.logger.Error("Failed to sync buckets", "err", err)

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
							return errors.New("invalid bucket id")
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
			v.Address,
			v.ExpectedHeight,
			v.Mode,
			nil,
			v.SnapshotDownloaded,
		); err != nil {
			s.logger.Error("Failed to create sync request for job", "error", err, "address", v.Address)
		}
	}

	return nil
}

func (s *Syncer) syncBuckets(bestPeer kramaid.KramaID, attempts int) error {
	var (
		argsChan = make(chan *BucketSyncRequest, 1)
		respChan = make(chan *BucketSyncResponse, ChannelBufferSize)
	)

	peerID, err := bestPeer.DecodedPeerID()
	if err != nil {
		s.logger.Error("Failed to decode peer ID", "err", err)

		return err
	}

	go func() {
		if err = s.rpcClient.Stream(
			context.Background(),
			peerID,
			"SYNCRPC",
			"SyncBuckets",
			argsChan,
			respChan,
			connTag(identifiers.NilAddress, "syncBuckets")); err != nil {
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

						return errors.New("invalid bucket id")
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

		localMetaInfo, err := s.db.GetAccountMetaInfo(acc.Address)
		if err == nil {
			if localMetaInfo.Height >= acc.Height {
				continue
			}
		}

		atomic.AddUint64(&s.pendingAccounts, 1)
		// TODO: Should improve this, jobQueue will consume most of the memory, if job processor is slow

		if err = s.NewSyncRequest(
			acc.Address,
			acc.Height,
			syncMode,
			nil,
			false,
		); err != nil {
			s.logger.Error(
				"Failed to add new sync request",
				"addr", acc.Address,
				"height", acc.Height,
				"err", err,
			)
		}
	}

	return nil
}

func (s *Syncer) isSnapSyncRequired(address identifiers.Address) bool {
	if !s.cfg.EnableSnapSync {
		return false
	}

	return !s.db.IsAccountPrimarySyncDone(address)
}

func (s *Syncer) fetchAndStoreSnap(bestPeer kramaid.KramaID, job *SyncJob) error {
	ctx, cancel := context.WithTimeout(
		context.Background(), // TODO: Need to improve the timeouts
		time.Duration(5000+(job.getExpectedHeight())*5000)*time.Millisecond,
	)
	defer cancel()

	s.logger.Trace("Initiating snap sync request", "addr", job.address)

	dropPrefix := func() {
		err := s.db.DropPrefix(job.address.Bytes())
		if err != nil {
			panic(err) // This should never happen
		}
	}

	isSnapStored, err := s.fetchSnapShot(ctx, bestPeer, job.address, job.expectedHeight)
	if err != nil {
		if isSnapStored {
			dropPrefix()
		}

		return errors.Wrap(err, "failed to fetch snapshot")
	}

	s.logger.Trace("Snap fetch successful", "addr", job.address)

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
	peer kramaid.KramaID,
	address identifiers.Address,
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
		Prefix:  address.Bytes(),
		Entries: make([]byte, 0),
	}

	reqChan <- &SnapRequest{
		Address: address,
		Height:  expectedHeight,
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
			connTag(address, "syncSnap"),
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

func (s *Syncer) fromGenesis(addr identifiers.Address, currentHeight uint64) bool {
	if currentHeight == 0 {
		_, err := s.db.GetAccountMetaInfo(addr)
		if errors.Is(err, common.ErrAccountNotFound) {
			return true
		}
	}

	return false
}

func (s *Syncer) syncLattice(
	ctx context.Context,
	nextTS *TesseractInfo,
	job *SyncJob,
	bestPeer kramaid.KramaID,
) error {
	var (
		endHeight   = job.getExpectedHeight()
		startHeight = job.getCurrentHeight()
		respChan    = make(chan *networkmsg.TesseractSyncMsg, 5)
		reqChan     = make(chan *LatticeRequest, 1)
	)

	if nextTS != nil {
		// check if we have tesseract for start height, if not sync from the start height
		if s.db.HasAccMetaInfoAt(job.address, startHeight) || nextTS.height() == startHeight {
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

	s.logger.Debug("Sending lattice sync request", "addr", job.address)

	fromGenesis := s.fromGenesis(job.address, job.getCurrentHeight())
	if !fromGenesis {
		startHeight++
	}

	requiredTesseractCount := endHeight - startHeight + 1

	reqChan <- &LatticeRequest{
		Address:     job.address,
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
			"for", job.address,
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
			connTag(job.address, "syncLattice"),
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

				tsInfo, err := s.tesseractInfoFromTesseractMsg(job.address, msg)
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
					"addr", tsInfo.address(),
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

	if err := grp.Wait(); err != nil {
		return err
	}

	if err := s.publishEventLatticeSync(job.jobStateEvent()); err != nil {
		s.logger.Error("Failed to publish event lattice sync", "err", err)
	}

	return nil
}

func (s *Syncer) tesseractInfoFromTesseractMsg(
	addr identifiers.Address,
	msg *networkmsg.TesseractSyncMsg,
) (*TesseractInfo, error) {
	var err error

	info := &TesseractInfo{
		addr:          addr,
		delta:         msg.Delta,
		shouldExecute: false,
		clusterInfo:   new(common.ICSClusterInfo),
	}

	info.tesseract, err = msg.GetTesseract()
	if err != nil {
		return nil, err
	}

	if !info.tesseract.ICSHash().IsNil() {
		if err = polo.Depolorize(info.clusterInfo, info.delta[info.tesseract.ICSHash().String()]); err != nil {
			return nil, err
		}
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

func (s *Syncer) isAnyOtherParticipantStored(msg *TesseractInfo) bool {
	if s.db.HasAccMetaInfoAt(msg.address(), msg.height()) {
		return false
	}

	for addr, participant := range msg.tesseract.Participants() {
		if s.db.HasAccMetaInfoAt(addr, participant.Height) {
			return true
		}
	}

	return false
}

func (s *Syncer) syncTesseract(msg *TesseractInfo) (bool, error) {
	var err error

	if err := s.fillTSWithIxnsAndReceipts(msg); err != nil {
		s.logger.Trace("failed to fetch ixns and receipts ", "err", err)

		return false, nil
	}

	if msg.icsNodeSet == nil && !msg.extractICSNodeset(s) {
		return false, nil
	}

	syncTSThroughAgora := func() (bool, error) {
		err = s.lattice.ValidateTesseract(msg.address(), msg.tesseract, msg.icsNodeSet, false)
		if err != nil {
			return false, errors.Wrap(err, "failed to validate tesseract")
		}

		if msg.tesseract.TransitiveLink(msg.address()).IsNil() {
			s.state.CreateDirtyObject(msg.address(), common.AccTypeFromIxType(msg.tesseract.Interactions()[0].Type()))
		}

		// During the initial sync stage, we retrieve participant data from trusted peers using snap sync.
		// If participant state exists, it indicates that all data related to the participant has been fetched,
		// allowing us to skip syncing for that participant.
		if s.isInitialSyncDone() ||
			!s.state.HasParticipantStateAt(msg.address(), msg.tesseract.StateHash(msg.address())) {
			if err = s.fetchTesseractState(msg.address(), msg.tesseract, msg.icsNodeSet.GetNodes(false)); err != nil {
				return false, errors.Wrap(err, "failed to fetch tesseract state")
			}
		}

		if err = s.lattice.AddTesseractWithState(
			msg.address(),
			extractDirtyEntries(msg.delta),
			msg.tesseract,
			false,
		); err != nil {
			return false, errors.Wrap(err, "failed to add synced tesseract")
		}

		if err := s.publishEventTesseractSync(msg.address(), msg.height()); err != nil {
			s.logger.Error("Failed to publish event lattice sync", "err", err)
		}

		return true, nil
	}

	if !msg.shouldExecute {
		return syncTSThroughAgora()
	}

	s.execLock.Lock()

	if _, execInProgress := s.execGrid[msg.tesseract.Hash()]; execInProgress {
		s.execLock.Unlock()

		return false, nil
	}

	// if execution is not in progress and
	// this participant is not added but other participants are added through agora to DB
	// then sync this tesseract through agora
	if s.isAnyOtherParticipantStored(msg) {
		s.execLock.Unlock()

		return syncTSThroughAgora()
	}

	s.execGrid[msg.tesseract.Hash()] = struct{}{}
	s.execLock.Unlock()

	defer func() {
		s.execLock.Lock()
		delete(s.execGrid, msg.tesseract.Hash())
		s.execLock.Unlock()
	}()

	// TODO is it okay to just check height for genesis identification ?
	// send job to sleep state, if any one of the transitive link is absent
	for address, participantState := range msg.tesseract.Participants() {
		if !s.db.HasTesseract(participantState.TransitiveLink) && participantState.Height != 0 {
			s.logger.Trace("Missing transitive links", "addr", address)

			return false, nil
		}
	}

	// In case if other job already executed, added tesseracts, then remove this tesseract from job and
	// update job's current height
	if s.db.HasAccMetaInfoAt(msg.address(), msg.height()) {
		return true, nil
	}

	err = s.lattice.ValidateTesseract(msg.address(), msg.tesseract, msg.icsNodeSet, true)
	if err != nil {
		return false, errors.Wrap(err, "failed to validate tesseract in execution phase")
	}

	s.accountsLock.Lock()

	addresses := msg.tesseract.Addresses()

	for _, addr := range addresses {
		if _, ok := s.lockedAccounts[addr]; ok {
			s.accountsLock.Unlock()

			return false, nil
		}
	}

	for _, addr := range addresses {
		s.lockedAccounts[addr] = struct{}{}
	}

	s.accountsLock.Unlock()

	defer func() {
		s.accountsLock.Lock()
		for _, addr := range addresses {
			delete(s.lockedAccounts, addr)
		}
		s.accountsLock.Unlock()
	}()

	if err = s.executeAndAdd(extractDirtyEntries(msg.delta), msg.tesseract); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Syncer) executeAndAdd(dirty map[common.Hash][]byte, ts *common.Tesseract) error {
	if err := s.lattice.ExecuteAndValidate(ts); err != nil {
		return err
	}

	if err := s.lattice.AddTesseractWithState(identifiers.NilAddress, dirty, ts, true); err != nil {
		return err
	}

	for addr, participantState := range ts.Participants() {
		if err := s.publishEventTesseractSync(addr, participantState.Height); err != nil {
			s.logger.Error("Failed to publish event lattice sync", "err", err)
		}
	}

	return nil
}

// fetchTesseractState fetches the complete state(balance,context,approvals) of the given tesseract using agora
func (s *Syncer) fetchTesseractState(
	addr identifiers.Address,
	tesseract *common.Tesseract,
	fetchContext []kramaid.KramaID,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
	defer cancel()

	newSession, err := s.agora.NewSession(ctx, fetchContext, addr, cid.AccountCID(tesseract.StateHash(addr)))
	if err != nil {
		return err
	}
	defer newSession.Close()

	islocal, acc, blk, err := s.fetchAccount(ctx, newSession, tesseract.StateHash(addr))
	if err != nil {
		s.logger.Error("Error fetching account data", "err", err)

		return err
	}

	if err = s.fetchAndStoreData(
		ctx,
		newSession,
		cid.BalanceCID(acc.Balance),
		cid.ApprovalsCID(acc.AssetApprovals),
		cid.RegistryCID(acc.AssetRegistry),
		// receiptsCID(tesseract.GridHash()),
	); err != nil {
		s.logger.Error("Error fetching balance data", "err", err)

		return err
	}

	if err = s.syncContextData(ctx, newSession, cid.ContextCID(acc.ContextHash)); err != nil {
		s.logger.Error("Error fetching context data", "err", err)

		return err
	}

	if err = s.syncLogicTree(ctx, newSession, acc.LogicRoot); err != nil {
		return errors.Wrap(err, "failed to sync logic tree")
	}

	if err = s.syncStorageTree(ctx, newSession, acc.StorageRoot); err != nil {
		return errors.Wrap(err, "failed to sync storage tree")
	}

	if !islocal {
		if err = s.db.SetAccount(addr, tesseract.StateHash(addr), blk.GetData()); err != nil {
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
	common.Interactions,
	error,
) {
	ixns := new(common.Interactions)

	rawIxns, err := s.db.GetInteractions(tsHash)
	if err == nil {
		if err := ixns.FromBytes(rawIxns); err != nil {
			return nil, err
		}

		return *ixns, nil
	}

	blk, err := session.GetBlock(ctx, cid.InteractionsCID(tsHash))
	if err != nil {
		return nil, err
	}

	err = ixns.FromBytes(blk.GetData())

	return *ixns, err
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

	err = receipts.FromBytes(blk.GetData())

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

// syncContextData fetches the behavioural context and random context associated with the given hash using agora
func (s *Syncer) syncContextData(ctx context.Context, session syncer.Session, cID cid.CID) error {
	islocal, blk, err := s.getBlock(ctx, session, cID)
	if err != nil {
		return err
	}

	metaContextObject := new(state.MetaContextObject)
	if err = metaContextObject.FromBytes(blk.GetData()); err != nil {
		return err
	}

	if err = s.fetchAndStoreData(
		ctx,
		session,
		cid.ContextCID(metaContextObject.RandomContext),
		cid.ContextCID(metaContextObject.BehaviouralContext),
	); err != nil {
		return err
	}

	if !islocal {
		if err = s.db.CreateEntry(dbKeyFromCID(session.ID(), cID), blk.GetData()); err != nil {
			return err
		}
	}

	return nil
}

func (s *Syncer) syncStorageTree(ctx context.Context, session syncer.Session, newRoot common.Hash) error {
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
		return s.state.SyncStorageTrees(ctx, session.ID(), metaStorageRoot, storageTreeRoots)
	}

	s.logger.Debug("Syncing storage tree", "address", session.ID())

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

	if err = s.state.SyncStorageTrees(ctx, session.ID(), metaStorageRoot, storageTreeRoots); err != nil {
		s.logger.Error("Failed to sync storage tree", "addr", session.ID())

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
			s.logger.Error("Failed to fetch logic manifest", "addr", as.ID(), "manifest-hash", cID.String())

			return errors.New("failed to fetch logic manifest")
		}
	}

	return nil
}

func (s *Syncer) syncLogicTree(ctx context.Context, as syncer.Session, newRoot common.Hash) error {
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

	return s.state.SyncLogicTree(as.ID(), metaLogicRoot)
}

func (s *Syncer) fillTSWithIxnsAndReceipts(tsInfo *TesseractInfo) error {
	var (
		ixns common.Interactions
		err  error
	)

	ts := tsInfo.tesseract

	fetchIxns := len(ts.Interactions()) == 0
	fetchReceipts := !tsInfo.shouldExecute && len(ts.Receipts()) == 0

	if !fetchIxns && !fetchReceipts {
		return nil
	}

	s.IxFetchLock.Lock()

	if _, ok := s.IxFetchGrid[ts.Hash()]; ok {
		s.IxFetchLock.Unlock()

		return errors.New("another job is fetching ixns and receipts")
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
		ixns, err = s.ixpool.GetIxns(tsInfo.ixnsHashes)
		if err != nil {
			s.logger.Trace("Ixns not found in ixpool",
				"ixns-hashes", tsInfo.ixnsHashes, "addr", tsInfo.address())

			err = func() error {
				ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
				defer cancel()

				newSession, err := s.agora.NewSession(
					ctx,
					tsInfo.clusterInfo.RandomSet,
					tsInfo.address(),
					cid.AccountCID(ts.StateHash(tsInfo.address())),
				)
				if err != nil {
					return errors.Wrap(err, "unable to create session")
				}
				defer newSession.Close()

				ixns, err = s.fetchInteractions(ctx, newSession, ts.Hash())
				if err != nil {
					return errors.Wrap(err, "unable to fetch interactions through agora")
				}

				s.logger.Trace("fetched Ixns through agora",
					"ixns-hashes", tsInfo.ixnsHashes, "addr", tsInfo.address())

				return nil
			}()

			if err != nil {
				return err
			}
		}

		ts.SetIxns(ixns)
	}

	// retrieve receipts only when Tesseract execution is not needed and receipts are not available
	if fetchReceipts {
		err = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
			defer cancel()

			newSession, err := s.agora.NewSession(
				ctx,
				tsInfo.clusterInfo.RandomSet,
				tsInfo.address(),
				cid.AccountCID(ts.StateHash(tsInfo.address())),
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

		for _, addr := range tsInfo.tesseract.Addresses() {
			info := tsInfo.CreateTSInfoWithAddr(addr)
			if err := s.NewSyncRequest(
				info.address(),
				info.height(),
				common.LatestSync,
				info.clusterInfo.RandomSet,
				false,
				info,
			); err != nil {
				s.logger.Error("Error adding sync request", "err", err)
			}
		}

		s.init.Do(func() {
			close(s.pendingMsgChan)
		})
	}

	return nil
}

func (s *Syncer) getTesseractWithRawIxnsAndReceipts(
	address identifiers.Address,
	height uint64,
	withInteractions, withReceipts bool,
) (ts *common.Tesseract, ixns, receipts []byte, err error) {
	ts, err = s.lattice.GetTesseractByHeight(address, height, false)
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

	// get tesseract from tesseract message
	ts, err := tsMsg.GetTesseract()
	if err != nil {
		return pubsub.ValidationReject, err
	}

	// verify if tesseract has valid seal
	validSeal, err := s.lattice.IsSealValid(ts)
	if !validSeal || err != nil {
		return pubsub.ValidationReject, err
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
		"Tesseract received from",
		"sender", pid,
		"sealer", ts.SealBy(),
		"ts-hash", ts.Hash(),
	)

	clusterInfo := new(common.ICSClusterInfo)
	if err = clusterInfo.FromBytes(tsMsg.Extra[ts.ICSHash().String()]); err != nil {
		return pubsub.ValidationReject, err
	}

	tsInfo := &TesseractInfo{
		tesseract:     ts,
		clusterInfo:   clusterInfo,
		icsNodeSet:    nil,
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

	if err := s.addTrustedPeersToPeerstore(); err != nil {
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

	s.logger.Info("Connected to minimum number of required peers")

	s.startWorkers()

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
				s.logger.Debug("Sync in progress", "pending jobs", s.jobQueue.len())
			}

			if s.jobQueue.len() == 0 {
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
				req.Address,
				req.Height,
				common.LatestSync,
				[]kramaid.KramaID{req.BestPeer},
				false,
			); err != nil {
				s.logger.Error("Failed to handle sync request from krama engine", "err", err)
			}
		}
	}
}

// GetAccountSyncStatus returns the sync status of an account
func (s *Syncer) GetAccountSyncStatus(addr identifiers.Address) (*args.AccSyncStatus, error) {
	var currentHeight, expectedHeight uint64

	job, ok := s.jobQueue.getJob(addr)
	if !ok {
		accountInfo, err := s.db.GetAccountMetaInfo(addr)
		if err != nil {
			return nil, err
		}

		currentHeight = accountInfo.Height
		expectedHeight = 0
	} else {
		currentHeight = job.currentHeight
		expectedHeight = job.expectedHeight
	}

	isPrimarySyncDone := s.db.IsAccountPrimarySyncDone(addr)

	return &args.AccSyncStatus{
		CurrentHeight:     hexutil.Uint64(currentHeight),
		ExpectedHeight:    hexutil.Uint64(expectedHeight),
		IsPrimarySyncDone: isPrimarySyncDone,
	}, nil
}

// GetSyncJobInfo returns the sync job meta info for given address
func (s *Syncer) GetSyncJobInfo(addr identifiers.Address) (*args.SyncJobInfo, error) {
	if addr.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	job, ok := s.jobQueue.getJob(addr)
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
		nodeSyncStatus.PendingAccounts = s.jobQueue.GetPendingAccounts()
	}

	return nodeSyncStatus
}

// startWorkers will start the sync job workers
func (s *Syncer) startWorkers() {
	for i := uint32(0); i < s.jobWorkerCount; i++ {
		go s.worker()
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
					for addr := range tsInfo.tesseract.Participants() {
						info := tsInfo.CreateTSInfoWithAddr(addr)
						if err := s.NewSyncRequest(
							info.address(),
							info.height(),
							common.LatestSync,
							info.clusterInfo.RandomSet,
							false,
							info); err != nil {
							s.logger.Error("Error adding sync request", "err", err)
						}
					}
				}

				s.pendingMsgQueue = nil

				return
			}

			s.pendingMsgQueue = append(s.pendingMsgQueue, msg)
		}
	}
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
	return s.post(eventSystemAccounts{})
}

func (s *Syncer) publishEventSnapSync(state eventDataJobState) error {
	return s.post(eventSnapSync{state})
}

func (s *Syncer) publishEventLatticeSync(state eventDataJobState) error {
	return s.post(eventLatticeSync{state})
}

func (s *Syncer) publishEventTesseractSync(addr identifiers.Address, height uint64) error {
	return s.post(
		eventTesseractSync{
			eventDataJobState{
				address: addr,
				height:  height,
			},
		})
}

func dbKeyFromCID(address identifiers.Address, cid cid.CID) []byte {
	return storage.DBKey(address, storage.PrefixTag(cid.ContentType()), cid.Key())
}

func connTag(address identifiers.Address, service string) string {
	return fmt.Sprintf("%s:%s", service, address.Hex())
}

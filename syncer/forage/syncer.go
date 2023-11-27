package forage

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/jsonrpc/args"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/syncer"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/network/rpc"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
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
	ExecuteAndValidate(ts ...*common.Tesseract) error
	AddTesseracts(dirtyStorage map[common.Hash][]byte, tesseracts ...*common.Tesseract) error
	GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error)
	GetTesseractByHeight(
		address common.Address,
		height uint64,
		withInteractions bool,
	) (*common.Tesseract, error)
	ValidateTesseract(ts *common.Tesseract, ics *common.ICSNodeSet) error
	IsInitialTesseract(ts *common.Tesseract) (bool, error)
}

type stateManager interface {
	SyncStorageTrees(
		ctx context.Context,
		address common.Address,
		newRoot *common.RootNode,
		logicStorageTreeRoots map[string]*common.RootNode,
	) error
	SyncLogicTree(
		address common.Address,
		newRoot *common.RootNode,
	) error
	CreateDirtyObject(
		addr common.Address,
		accType common.AccountType,
	) *state.Object
	GetParticipantContextRaw(
		address common.Address,
		hash common.Hash,
		rawContext map[common.Hash][]byte,
	) error
	FetchICSNodeSet(
		ts *common.Tesseract,
		info *common.ICSClusterInfo,
	) (*common.ICSNodeSet, error)
	GetICSNodeSetFromRawContext(
		ts *common.Tesseract,
		rawContext map[common.Hash][]byte,
		clusterInfo *common.ICSClusterInfo,
	) (*common.ICSNodeSet, error)
}

type store interface {
	NewBatchWriter() db.BatchWriter
	CreateEntry([]byte, []byte) error
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	SetAccount(addr common.Address, stateHash common.Hash, data []byte) error
	SetInteractions(gridHash common.Hash, data []byte) error
	GetInteractions(gridHash common.Hash) ([]byte, error)
	GetAccountMetaInfo(id common.Address) (*common.AccountMetaInfo, error)
	UpdateTesseractStatus(addr common.Address, height uint64, tsHash common.Hash, status bool) error
	SetAccountSyncStatus(address common.Address, status *common.AccountSyncStatus) error
	CleanupAccountSyncStatus(address common.Address) error
	StoreAccountSnapShot(snap *common.Snapshot) error
	GetReceipts(gridHash common.Hash) ([]byte, error)
	GetAccountsSyncStatus() ([]*common.AccountSyncStatus, error)
	DropPrefix(prefix []byte) error
	UpdatePrimarySyncStatus(address common.Address) error
	IsAccountPrimarySyncDone(address common.Address) bool
	HasTesseract(tsHash common.Hash) bool
	SetTesseract(tsHash common.Hash, data []byte) error
	UpdatePrincipalSyncStatus() error
	GetBucketCount(bucketNumber uint64) (uint64, error)
	StreamAccountMetaInfosRaw(ctx context.Context, bucketNumber uint64, response chan []byte) error
	GetRecentUpdatedAccMetaInfosRaw(ctx context.Context, bucketID uint64, sinceTS uint64) ([][]byte, error)
	IsPrincipalSyncDone() (bool, int64)
	GetAccountSnapshot(
		ctx context.Context,
		address common.Address,
		sinceTS uint64,
	) (*common.Snapshot, error)
	HasTesseractAt(addr common.Address, height uint64) bool
	UpdateAccMetaInfo(
		id common.Address,
		height uint64,
		tesseractHash common.Hash,
		accType common.AccountType,
		latticeExists, stateExists bool,
	) (int32, bool, error)
	SetTesseractHeightEntry(addr common.Address, height uint64, tsHash common.Hash) error
}

type Syncer struct {
	lock                sync.RWMutex
	cfg                 *config.SyncerConfig
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	network             *p2p.Server
	mux                 *utils.TypeMux
	gridStore           *GridStore
	execLock            sync.RWMutex
	agora               syncer.BlockSync
	db                  store
	tesseractRegistry   *common.HashRegistry
	jobQueue            *JobQueue
	rpcClient           *rpc.Client
	lattice             lattice
	state               stateManager
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
	lockedAccounts      map[common.Address]common.Hash
	metrics             *Metrics
	initialSyncDone     bool
	pendingMsgChan      chan *TesseractInfo
	pendingMsgQueue     []*TesseractInfo
	init                sync.Once
	execGrid            map[common.Hash]common.Address
	tracker             *SyncStatusTracker
	workerWaitTime      time.Duration
}

func NewSyncer(
	cfg *config.SyncerConfig,
	logger hclog.Logger,
	node *p2p.Server,
	mux *utils.TypeMux,
	db store,
	lattice lattice,
	sm stateManager,
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
		jobWorkerCount:      10,
		workerWaitTime:      DefaultWorkerWaitTime,
		jobQueue:            NewJobQueue(mux),
		gridStore:           NewGridStore(),
		logger:              logger.Named("Syncer"),
		workerSignal:        make(chan struct{}),
		tesseractRegistry:   common.NewHashRegistry(60),
		consensusSlots:      slots,
		lastActiveTimeStamp: lastActiveTimeStamp,
		lockedAccounts:      make(map[common.Address]common.Hash, 0),
		metrics:             syncerMetrics,
		pendingMsgQueue:     make([]*TesseractInfo, 0),
		pendingMsgChan:      make(chan *TesseractInfo, 10),
		execGrid:            make(map[common.Hash]common.Address),
		tracker:             NewSyncStatusTracker(0),
	}

	return s, nil
}

func (s *Syncer) NewSyncRequest(
	addr common.Address,
	expectedHeight uint64,
	syncMode common.SyncMode,
	bestPeers []id.KramaID,
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
			tesseractSignal: make(chan struct{}, 1),
			bestPeers:       make(map[id.KramaID]struct{}),
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
		if job.tesseractQueue.Has(v.tesseract.Height()) || v.tesseract.Height() < job.getCurrentHeight() {
			continue
		}

		job.tesseractQueue.Push(v)
	}

	if job.getExpectedHeight() < expectedHeight {
		if err = job.updateExpectedHeight(expectedHeight); err != nil {
			return err
		}
	}

	ts, _ := s.lattice.GetTesseractByHeight(job.address, job.getCurrentHeight(), false)
	if job.getCurrentHeight() == job.getExpectedHeight() && ts != nil {
		s.logger.Debug("Tesseract found avoiding new sync request")

		return nil
	}

	if syncMode == common.FullSync && len(job.bestPeers) == 0 && len(bestPeers) == 0 {
		var height uint64

		height, bestPeers, err = s.findLatestHeightAndBestPeers(addr)
		if err != nil {
			return errors.Wrap(err, "failed to find best peers for sync")
		}

		// if system account best peers doesn't have latest height then it will be updated here
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

func (s *Syncer) jobProcessor(job *SyncJob) error {
	var (
		err      error
		bestPeer id.KramaID
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

	if job.bestPeerLen() > 0 {
		bestPeer = job.chooseRandomBestPeer()
	} else {
		bestPeer, err = s.chooseBestSyncPeer(job)
		if err != nil {
			return errors.Wrap(err, "failed to fetch best peer")
		}
	}

	tsInfo := job.tesseractQueue.Peek()

	if !job.snapDownloaded && s.isSnapSyncRequired(job.address) && job.mode != common.LatestSync {
		if err = s.fetchAndStoreSnap(bestPeer, job); err != nil {
			job.deleteBestPeer(bestPeer)

			return err
		}

		if err := s.publishEventSnapSync(job.jobStateEvent()); err != nil {
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

			if !s.db.HasTesseract(tsInfo.tesseract.Hash()) {
				initial, err := s.lattice.IsInitialTesseract(tsInfo.tesseract)
				if err != nil {
					jobState = Sleep

					return nil
				}

				if !initial && tsInfo.tesseract.Height() != job.getCurrentHeight()+1 {
					s.logger.Error(
						"Missing tesseract",
						"addr", tsInfo.tesseract.Address(),
						"height", tsInfo.tesseract.Height(),
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

			shouldExit, err := s.postAdditionHook(job, tsInfo.tesseract.Height())
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

func getBestPeers(heightPeersMap map[uint64][]id.KramaID) (uint64, []id.KramaID, error) {
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

// findLatestHeightAndBestPeers returns the height reported from majority of peers as best height
func (s *Syncer) findLatestHeightAndBestPeers(addr common.Address) (uint64, []id.KramaID, error) {
	heightPeersMap := make(map[uint64][]id.KramaID)

	// index tracks the no of peers responded
	index := 0

	for _, kramaID := range s.network.GetPeers() {
		if index == MaxPeersToDial {
			break
		}

		resp := new(LatestAccountInfo)
		if err := s.rpcClient.MoiCall(
			context.Background(),
			kramaID,
			"SYNCRPC",
			"GetLatestAccountInfo",
			addr,
			resp,
			time.Minute*2,
		); err != nil {
			s.logger.Error("Failed to fetch account latest status", "RPC-error", err, kramaID)

			continue
		}

		index++

		nodes, ok := heightPeersMap[resp.Height]
		if !ok {
			heightPeersMap[resp.Height] = make([]id.KramaID, 0)
		}

		nodes = append(nodes, kramaID)
		heightPeersMap[resp.Height] = nodes
	}

	return getBestPeers(heightPeersMap)
}

func (s *Syncer) chooseBestSyncPeer(job *SyncJob) (id.KramaID, error) {
	if job.mode == common.LatestSync && job.tesseractQueue.Peek() != nil {
		randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
		randNumber := randomSource.Intn(len(job.tesseractQueue.Peek().clusterInfo.RandomSet))

		return job.tesseractQueue.Peek().clusterInfo.RandomSet[randNumber], nil
	}

	_, bestPeers, err := s.findLatestHeightAndBestPeers(job.address)
	if err != nil {
		return "", err
	}

	return bestPeers[0], nil
}

// syncSystemAccount sends a sync request for the specified address and waits for it to complete within a given time.
// If the sync does not complete within the specified time, an error is returned.
func (s *Syncer) syncSystemAccount(address ...common.Address) ([]id.KramaID, error) {
	var (
		bestPeers  []id.KramaID
		bestHeight uint64
		err        error
	)

	for _, addr := range address {
		bestHeight, bestPeers, err = s.findLatestHeightAndBestPeers(addr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch best peers and height")
		}

		if err = s.NewSyncRequest(addr, bestHeight, common.FullSync, bestPeers); err != nil {
			return nil, err
		}

		err = func() error {
			ctx, cancel := context.WithTimeout(s.ctx, time.Duration(5000+(bestHeight*5000))*time.Millisecond)
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

func (s *Syncer) syncBucketsWithMaxAttempts(bestPeers []id.KramaID, maxAttempts int) error {
	randomNumber := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 1; i < maxAttempts+1; i++ {
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
		syncJob, err := SyncJobFromCanonicalInfo(s.logger, s.db, v)
		if err != nil {
			return err
		}

		accountMetaInfo, err := s.db.GetAccountMetaInfo(v.Address)
		if err == nil {
			syncJob.updateCurrentHeight(accountMetaInfo.Height)
		}

		if err = s.jobQueue.AddJob(syncJob); err != nil {
			s.logger.Error("Failed to add job in job queue", "err", err)

			continue
		}

		s.metrics.captureTotalJobs(float64(len(s.jobQueue.jobs)))
	}

	return nil
}

func (s *Syncer) syncBuckets(kramaID id.KramaID, attempts int) error {
	var (
		argsChan = make(chan *BucketSyncRequest, 1)
		respChan = make(chan *BucketSyncResponse, ChannelBufferSize)
	)

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		s.logger.Error("Failed to decode peer ID", "err", err)

		return err
	}

	errGrp, grpCtx := errgroup.WithContext(s.ctx)

	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(grpCtx, peerID, "SYNCRPC", "SyncBuckets", argsChan, respChan); err != nil {
			s.logger.Error("Failed to sync buckets", "err", err)

			return err
		}

		return nil
	})

	errGrp.Go(func() error {
		defer close(argsChan)

		for i := uint64(0); i <= storage.MaxBucketCount; i++ {
			argsChan <- &BucketSyncRequest{
				BucketID: i,
			}

			totalEntriesInBucket := uint64(0)

			err = func() error {
				for {
					select {
					case <-grpCtx.Done():
						return grpCtx.Err()
					case respMsg, ok := <-respChan:
						if !ok {
							return nil
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

					case <-time.After(time.Duration(5*attempts) * time.Second):
						return common.ErrTimeOut
					}
				}
			}()
			if err != nil {
				return err
			}
		}

		s.setBucketSyncDone(true)

		return nil
	})

	return errGrp.Wait()
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

func (s *Syncer) isSnapSyncRequired(address common.Address) bool {
	if !s.cfg.EnableSnapSync {
		return false
	}

	return !s.db.IsAccountPrimarySyncDone(address)
}

func (s *Syncer) fetchAndStoreSnap(bestPeer id.KramaID, job *SyncJob) error {
	ctx, cancel := context.WithTimeout(
		context.Background(), // TODO: Need to improve the timeouts
		time.Duration((1000+job.getExpectedHeight())*500)*time.Millisecond,
	)
	defer cancel()

	s.logger.Trace("Initiating snap sync request", "addr", job.address)

	snap, err := s.fetchSnapShort(ctx, bestPeer, job.address, job.expectedHeight)
	if err != nil {
		return errors.Wrap(err, "failed to fetch snapshot")
	}

	s.logger.Trace("Snap fetch successful", "addr", job.address)

	storeErr := func() error {
		if err = s.db.StoreAccountSnapShot(snap); err != nil {
			return err
		}

		if err = job.updateSnap(true); err != nil {
			return err
		}

		return nil
	}()

	if storeErr != nil {
		err = s.db.DropPrefix(job.address.Bytes())
		if err != nil {
			panic(err) // This should never happen
		}
	}

	return storeErr
}

func (s *Syncer) fetchSnapShort(
	ctx context.Context,
	peer id.KramaID,
	address common.Address,
	expectedHeight uint64,
) (*common.Snapshot, error) {
	peerID, err := peer.DecodedPeerID()
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode peer ID")
	}

	var (
		snapInfo *SnapMetaInfo
		reqChan  = make(chan *SnapRequest, 1)
		respChan = make(chan *SnapResponse, 2)
	)

	currentSnap := &common.Snapshot{
		Prefix: address.Bytes(),
	}

	reqChan <- &SnapRequest{
		Address: address,
		Height:  expectedHeight,
	}

	errGrp, grpCtx := errgroup.WithContext(ctx)
	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(
			grpCtx,
			peerID,
			"SYNCRPC",
			"SyncSnap",
			reqChan,
			respChan,
		); err != nil {
			return err
		}

		return nil
	})

	errGrp.Go(func() error {
		defer close(reqChan)

		for {
			select {
			case <-grpCtx.Done():
				return errors.New("context expired")

			case snapMsg, ok := <-respChan:
				if !ok {
					return errors.New("response chan closed")
				}

				if snapMsg.MetaInfo != nil && snapInfo == nil {
					snapInfo = snapMsg.MetaInfo
					currentSnap.CreatedAt = snapMsg.MetaInfo.CreatedAt
					currentSnap.Entries = make([]byte, 0, snapInfo.TotalSnapSize)
				}

				currentSnap.Size += uint64(len(snapMsg.Data))
				currentSnap.Entries = append(currentSnap.Entries, snapMsg.Data...)

				s.logger.Info("Snap info ", "total snap size ", snapInfo.TotalSnapSize,
					"current snap size", currentSnap.Size)
				if snapInfo != nil && currentSnap.Size == snapInfo.TotalSnapSize {
					return nil
				}
			}
		}
	})

	if err = errGrp.Wait(); err != nil {
		return nil, err
	}

	return currentSnap, nil
}

func (s *Syncer) registerRPCService() error {
	s.rpcClient = s.network.StartNewRPCServer(config.SyncProtocolRPC, "SYNCRPC")

	return s.network.RegisterNewRPCService(config.SyncProtocolRPC, "SYNCRPC", NewSyncRPCService(s))
}

func (s *Syncer) fromGenesis(addr common.Address, currentHeight uint64) bool {
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
	bestPeer id.KramaID,
) error {
	var (
		endHeight   = job.getExpectedHeight()
		startHeight = job.getCurrentHeight()
		respChan    = make(chan *networkmsg.TesseractMessage, 5)
		reqChan     = make(chan *LatticeRequest, 1)
	)

	if nextTS != nil {
		// check if we have tesseract for start height, if not sync from the start height
		if s.db.HasTesseractAt(job.address, startHeight) || nextTS.tesseract.Height() == startHeight {
			if int64(nextTS.tesseract.Height()-(startHeight+1)) <= 0 {
				return nil
			}
		}

		endHeight = nextTS.tesseract.Height() - 1
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
			"FetchLattice",
			reqChan,
			respChan,
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

				tsInfo, err := s.tesseractInfoFromTesseractMsg(msg)
				if err != nil {
					s.logger.Error("Failed to parse tesseract info from message", "err", err)

					continue
				}

				if tsInfo.tesseract.Height() >= startHeight && tsInfo.tesseract.Height() <= endHeight {
					requiredTesseractCount--
				}

				if job.tesseractQueue.Has(tsInfo.tesseract.Height()) {
					continue
				}

				s.logger.Debug(
					"Adding tesseract to queue",
					"addr", tsInfo.tesseract.Address(),
					"height", tsInfo.tesseract.Height(),
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
		s.logger.Error("failed to publish event lattice sync", "err", err)
	}

	return nil
}

func (s *Syncer) tesseractInfoFromTesseractMsg(msg *networkmsg.TesseractMessage) (*TesseractInfo, error) {
	var err error

	info := &TesseractInfo{
		delta:         msg.Delta,
		shouldExecute: false,
		clusterInfo:   new(common.ICSClusterInfo),
	}

	info.tesseract, err = msg.GetTesseract()
	if err != nil {
		return nil, err
	}

	if !info.tesseract.ICSHash().IsNil() {
		if err = polo.Depolorize(info.clusterInfo, info.delta[info.tesseract.ICSHash()]); err != nil {
			return nil, err
		}
	}

	return info, nil
}

func (s *Syncer) areGridTesseractsStored(msg *TesseractInfo) bool {
	if s.db.HasTesseractAt(msg.tesseract.Address(), msg.tesseract.Height()) {
		return false
	}

	for addr, tsHashAndNumber := range msg.tesseract.Header().Extra.GridID.Parts.Grid {
		if s.db.HasTesseractAt(addr, tsHashAndNumber.Height) {
			return true
		}
	}

	return false
}

func (s *Syncer) syncTesseract(msg *TesseractInfo) (bool, error) {
	var err error

	if msg.icsNodeSet == nil {
		for _, contextLock := range msg.tesseract.ContextLock() {
			if contextLock.ContextHash.IsNil() {
				continue
			}

			if _, ok := msg.delta[contextLock.ContextHash]; !ok {
				msg.icsNodeSet, err = s.state.FetchICSNodeSet(msg.tesseract, msg.clusterInfo)
				if err != nil {
					s.logger.Error("Failed to fetch node set", "err", err)

					return false, nil
				}
			} else {
				msg.icsNodeSet, err = s.state.GetICSNodeSetFromRawContext(msg.tesseract, msg.delta, msg.clusterInfo)
				if err != nil {
					s.logger.Error("Failed to fetch node set", "err", err)

					return false, nil
				}
			}

			break
		}
	}

	var execInProgress bool

	s.execLock.RLock()
	_, execInProgress = s.execGrid[msg.tesseract.GridHash()]
	s.execLock.RUnlock()

	if (!execInProgress && s.areGridTesseractsStored(msg)) || !msg.shouldExecute {
		err = s.lattice.ValidateTesseract(msg.tesseract, msg.icsNodeSet)
		if err != nil {
			return false, errors.Wrap(err, "failed to validate tesseract")
		}

		if err = s.fetchTesseractState(msg.tesseract, msg.icsNodeSet.GetNodes()); err != nil {
			return false, errors.Wrap(err, "failed to fetch tesseract state")
		}

		if err = s.lattice.AddTesseracts(msg.delta, msg.tesseract); err != nil {
			return false, errors.Wrap(err, "failed to add synced tesseract")
		}

		if err := s.publishEventTesseractSync(msg.tesseract.Address(), msg.tesseract.Height()); err != nil {
			s.logger.Error("failed to publish event lattice sync", "err", err)
		}

		return true, nil
	}

	if execInProgress {
		return false, nil
	}

	grid := s.gridStore.GetGrid(msg.tesseract.GridHash())
	if grid == nil {
		// in case if other job already executed, added tesseracts,
		// and removed them from grid store then send this job to sleep state
		// so that this job updates its current height next time
		if s.db.HasTesseract(msg.tesseract.Hash()) {
			return false, nil
		}

		grid = s.gridStore.NewGrid(msg.tesseract.GridHash())
	}

	if !grid.HasTesseract(msg.tesseract) {
		err = s.lattice.ValidateTesseract(msg.tesseract, msg.icsNodeSet)
		if err != nil {
			return false, errors.Wrap(err, "failed to validate tesseract")
		}

		grid.AddTesseract(msg.tesseract)
	}

	if !grid.IsGridComplete(msg.tesseract.GridLength()) {
		return false, nil
	}

	s.execLock.Lock()
	if _, ok := s.execGrid[msg.tesseract.GridHash()]; ok {
		s.execLock.Unlock()

		return false, nil
	}

	s.execGrid[msg.tesseract.GridHash()] = msg.tesseract.Address()
	s.execLock.Unlock()

	defer func() {
		s.execLock.Lock()
		delete(s.execGrid, msg.tesseract.GridHash())
		s.execLock.Unlock()
	}()

	s.accountsLock.Lock()
	for _, ts := range grid.ts {
		if _, ok := s.lockedAccounts[ts.Address()]; ok {
			s.accountsLock.Unlock()

			return false, nil
		}
	}

	for _, ts := range grid.ts {
		s.lockedAccounts[ts.Address()] = ts.GridHash()
	}

	s.accountsLock.Unlock()

	defer func() {
		s.accountsLock.Lock()
		for _, ts := range grid.ts {
			delete(s.lockedAccounts, ts.Address())
		}
		s.accountsLock.Unlock()
	}()

	if err := s.executeAndAdd(msg.delta, grid); err != nil {
		return false, err
	}

	s.gridStore.CleanupGrid(msg.tesseract.GridHash())

	return true, nil
}

func (s *Syncer) executeAndAdd(dirty map[common.Hash][]byte, grid *Grid) error {
	if err := s.lattice.ExecuteAndValidate(grid.Tesseracts()...); err != nil {
		return err
	}

	if err := s.lattice.AddTesseracts(dirty, grid.Tesseracts()...); err != nil {
		return err
	}

	for _, ts := range grid.Tesseracts() {
		if err := s.publishEventTesseractSync(ts.Address(), ts.Height()); err != nil {
			s.logger.Error("failed to publish event lattice sync", "err", err)
		}
	}

	return nil
}

// fetchTesseractState fetches the complete state(balance,context,approvals) of the given tesseract using agora
func (s *Syncer) fetchTesseractState(tesseract *common.Tesseract, fetchContext []id.KramaID) error {
	ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
	defer cancel()

	newSession, err := s.agora.NewSession(ctx, fetchContext, tesseract.Address(), cid.AccountCID(tesseract.StateHash()))
	if err != nil {
		return err
	}
	defer newSession.Close()

	islocal, acc, block, err := s.fetchAccount(ctx, newSession, tesseract.StateHash())
	if err != nil {
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

	if tesseract.PrevHash().IsNil() {
		s.state.CreateDirtyObject(tesseract.Address(), common.AccTypeFromIxType(tesseract.Interactions()[0].Type()))
	}

	if err = s.syncLogicTree(ctx, newSession, acc.LogicRoot); err != nil {
		return errors.Wrap(err, "failed to sync logic tree")
	}

	if err = s.syncStorageTree(ctx, newSession, acc.StorageRoot); err != nil {
		return errors.Wrap(err, "failed to sync storage tree")
	}

	if !islocal {
		if err = s.db.SetAccount(tesseract.Address(), tesseract.StateHash(), block.GetData()); err != nil {
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

func (s *Syncer) msgHandler(msg *pubsub.Message) error {
	var (
		tsMsg = new(networkmsg.TesseractMessage)
		err   error
	)

	if err = tsMsg.FromBytes(msg.GetData()); err != nil {
		return err
	}

	ts, err := tsMsg.GetTesseract()
	if err != nil {
		return err
	}

	exists := s.db.HasTesseract(ts.Hash())
	if exists {
		return common.ErrAlreadyKnown
	}

	if !s.tesseractRegistry.Contains(ts.Hash()) {
		s.tesseractRegistry.Add(ts.Hash())

		s.logger.Debug(
			"Tesseract received from",
			"sender", tsMsg.Sender,
			"sealer", ts.Sealer(),
			"ts-hash", ts.Hash(),
			"addr", ts.Address(),
			"height", ts.Height(),
		)

		clusterInfo := new(common.ICSClusterInfo)
		if err = clusterInfo.FromBytes(tsMsg.Delta[ts.ICSHash()]); err != nil {
			return err
		}

		tsInfo := &TesseractInfo{
			tesseract:     ts,
			clusterInfo:   clusterInfo,
			icsNodeSet:    nil,
			shouldExecute: s.cfg.ShouldExecute,
			delta:         tsMsg.Delta,
		}

		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
			if !s.isInitialSyncDone() {
				s.pendingMsgChan <- tsInfo

				return nil
			}

			if err = s.NewSyncRequest(
				tsInfo.tesseract.Address(),
				tsInfo.tesseract.Height(),
				common.LatestSync,
				tsInfo.clusterInfo.RandomSet,
				tsInfo); err != nil {
				s.logger.Error("Error adding sync request", "err", err)
			}

			s.init.Do(func() {
				close(s.pendingMsgChan)
			})
		}
	}

	return nil
}

func (s *Syncer) getTesseractWithRawIxnsAndReceipts(
	address common.Address,
	height uint64,
	withInteractions, withReceipts bool,
) (ts *common.Tesseract, ixns, receipts []byte, err error) {
	ts, err = s.lattice.GetTesseractByHeight(address, height, false)
	if err != nil {
		return nil, nil, nil, err
	}

	if withInteractions && !ts.InteractionHash().IsNil() {
		ixns, err = s.db.GetInteractions(ts.GridHash())
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to load interactions")
		}
	}

	if withReceipts && !ts.ReceiptHash().IsNil() {
		receipts, err = s.db.GetReceipts(ts.GridHash())
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to load receipts")
		}
	}

	return ts, ixns, receipts, nil
}

// Start starts all event handlers and workers associated with sync sub protocol
func (s *Syncer) Start(minConnectedPeers int) error {
	sub := s.mux.Subscribe(utils.PendingAccountEvent{})

	go s.startPendingAccountEventHandler(sub)

	s.agora.Start()

	if err := s.registerRPCService(); err != nil {
		return err
	}

	if err := s.network.Subscribe(s.ctx, config.TesseractTopic, s.msgHandler); err != nil {
		return err
	}

	go s.queueHandler()

	for s.network.Peers.Len() < minConnectedPeers {
		time.Sleep(2 * time.Second)
	}

	s.startWorkers()

	go func() {
		if err := s.initSync(); err != nil {
			s.logger.Error("Initial sync failed", "err", err)

			return
		}

		s.setInitialSyncDone(true)

		s.logger.Info("Initial sync successful")
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
				[]id.KramaID{req.BestPeer},
			); err != nil {
				s.logger.Error("Failed to handle sync request from krama engine", "err", err)
			}
		}
	}
}

// GetAccountSyncStatus returns the sync status of an account
func (s *Syncer) GetAccountSyncStatus(addr common.Address) (*args.AccSyncStatus, error) {
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
					if err := s.NewSyncRequest(
						tsInfo.tesseract.Address(),
						tsInfo.tesseract.Height(),
						common.LatestSync,
						tsInfo.clusterInfo.RandomSet,
						tsInfo); err != nil {
						s.logger.Error("Error adding sync request", "err", err)
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

func (s *Syncer) publishEventTesseractSync(addr common.Address, height uint64) error {
	return s.post(
		eventTesseractSync{
			eventDataJobState{
				address: addr,
				height:  height,
			},
		})
}

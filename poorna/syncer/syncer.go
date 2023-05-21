package syncer

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/guna"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna"
	"github.com/sarvalabs/moichain/poorna/agora"
	"github.com/sarvalabs/moichain/poorna/agora/session"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/poorna/moirpc"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
	"golang.org/x/sync/errgroup"
)

const (
	TesseractTopic        = "MOI_PUBSUB_TESSERACT"
	MaxBucketSyncAttempts = 3
	ChannelBufferSize     = 10
	TesseractFetchTimeOut = 15 * time.Second
)

type lattice interface {
	ExecuteAndValidate(ts ...*types.Tesseract) error
	AddTesseracts(dirtyStorage map[types.Hash][]byte, tesseracts ...*types.Tesseract) error
	GetTesseract(hash types.Hash, withInteractions bool) (*types.Tesseract, error)
	GetTesseractByHeight(
		address types.Address,
		height uint64,
		withInteractions bool,
	) (*types.Tesseract, error)
	ValidateTesseract(ts *types.Tesseract, ics *types.ICSNodeSet) error
	FetchICSNodeSet(
		ts *types.Tesseract,
		info *types.ICSClusterInfo,
	) (*types.ICSNodeSet, error)
	IsInitialTesseract(ts *types.Tesseract) (bool, error)
}

type state interface {
	SyncStorageTrees(
		address types.Address,
		newRoot *types.RootNode,
		logicStorageTreeRoots map[string]*types.RootNode,
	) error
	SyncLogicTree(
		address types.Address,
		newRoot *types.RootNode,
	) error
	CreateDirtyObject(
		addr types.Address,
		accType types.AccountType,
	) *guna.StateObject
	GetParticipantContextRaw(
		address types.Address,
		hash types.Hash,
		rawContext map[types.Hash][]byte,
	) error
	GetICSNodeSetFromRawContext(
		ts *types.Tesseract,
		rawContext map[types.Hash][]byte,
		clusterInfo *types.ICSClusterInfo,
	) (*types.ICSNodeSet, error)
}

type store interface {
	NewBatchWriter() db.BatchWriter
	CreateEntry([]byte, []byte) error
	UpdateEntry([]byte, []byte) error
	ReadEntry([]byte) ([]byte, error)
	Contains([]byte) (bool, error)
	DeleteEntry([]byte) error
	SetAccount(addr types.Address, stateHash types.Hash, data []byte) error
	GetInteractions(gridHash types.Hash) ([]byte, error)
	GetAccountMetaInfo(id types.Address) (*types.AccountMetaInfo, error)
	UpdateTesseractStatus(addr types.Address, height uint64, tsHash types.Hash, status bool) error
	SetAccountSyncStatus(address types.Address, status *types.AccountSyncStatus) error
	CleanupAccountSyncStatus(address types.Address) error
	StoreAccountSnapShot(snap *types.Snapshot) error
	GetReceipts(gridHash types.Hash) ([]byte, error)
	GetAccountsSyncStatus() ([]*types.AccountSyncStatus, error)
	DropPrefix(prefix []byte) error
	UpdatePrimarySyncStatus(address types.Address) error
	IsAccountPrimarySyncDone(address types.Address) bool
	HasTesseract(tsHash types.Hash) bool
	UpdatePrincipalSyncStatus() error
	GetBucketCount(bucketNumber uint64) (uint64, error)
	StreamAccountMetaInfosRaw(ctx context.Context, bucketNumber uint64, response chan []byte) error
	GetRecentUpdatedAccMetaInfosRaw(ctx context.Context, bucketID uint64, sinceTS uint64) ([][]byte, error)
	IsPrincipalSyncDone() (bool, int64)
	GetAccountSnapshot(
		ctx context.Context,
		address types.Address,
		sinceTS uint64,
	) (*types.Snapshot, error)
}

type Syncer struct {
	cfg                 *common.SyncerConfig
	ctx                 context.Context
	network             *poorna.Server
	mux                 *utils.TypeMux
	gridStore           *GridStore
	agora               *agora.Agora
	db                  store
	tesseractRegistry   *types.HashRegistry
	jobQueue            *JobQueue
	rpcClient           *moirpc.Client
	lattice             lattice
	state               state
	logger              hclog.Logger
	workerLock          sync.Mutex
	jobWorkerCount      uint32
	workerSignal        chan struct{}
	isPrincipalSyncDone bool
	isBucketSyncDone    bool
	pendingAccounts     uint64
	consensusSlots      *ktypes.Slots
	lastActiveTimeStamp uint64
	accountsLock        sync.RWMutex
	lockedAccounts      map[types.Address]types.Hash
}

func NewSyncer(
	ctx context.Context,
	cfg *common.SyncerConfig,
	logger hclog.Logger,
	node *poorna.Server,
	mux *utils.TypeMux,
	db store,
	lattice lattice,
	sm state,
	metrics *agora.Metrics,
	slots *ktypes.Slots,
	lastActiveTimeStamp uint64,
) (*Syncer, error) {
	agoraInstance, err := agora.NewAgora(ctx, logger, db, node, metrics)
	if err != nil {
		return nil, errors.Wrap(err, "error initiating agora")
	}

	s := &Syncer{
		ctx:            ctx,
		network:        node,
		cfg:            cfg,
		mux:            mux,
		agora:          agoraInstance,
		db:             db,
		lattice:        lattice,
		state:          sm,
		jobWorkerCount: 10,
		jobQueue: &JobQueue{
			jobs: make(map[types.Address]*SyncJob),
		},
		gridStore:           NewGridStore(),
		logger:              logger.Named("Syncer"),
		workerSignal:        make(chan struct{}),
		tesseractRegistry:   types.NewHashRegistry(60),
		consensusSlots:      slots,
		lastActiveTimeStamp: lastActiveTimeStamp,
		lockedAccounts:      make(map[types.Address]types.Hash, 0),
	}

	return s, nil
}

func (s *Syncer) NewSyncRequest(
	addr types.Address,
	expectedHeight uint64,
	syncMode types.SyncMode,
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
			bestPeers:       bestPeers,
			tesseractSignal: make(chan struct{}, 1),
		}

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
		s.logger.Debug("tesseract found avoiding new sync request")

		return nil
	}

	if syncMode == types.FullSync && len(job.bestPeers) == 0 && len(bestPeers) == 0 {
		_, bestPeers, err = s.findLatestHeightAndBestPeers(addr, expectedHeight)
		if err != nil {
			return errors.Wrap(err, "failed to find best peers for sync")
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
		s.logger.Info("Closing syncer worker")
	}()

	for {
		select {
		case <-s.workerSignal:
		case <-time.After(1 * time.Second):
		case <-s.ctx.Done():
			return
		}

		job := s.jobQueue.NextJob()
		if job == nil {
			continue
		}

		if err := s.jobProcessor(job); err != nil {
			s.logger.Error("Error from sync job processor", "error", err)
		}
	}
}

func (s *Syncer) jobClosure(job *SyncJob) error {
	if currentState := job.getJobState(); currentState == Sleep || currentState == Done {
		return nil
	}

	job.updateJobState(Pending)

	return nil
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
		"expected-height", job.expectedHeight,
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

	if len(job.bestPeers) > 0 {
		randSource := rand.New(rand.NewSource(time.Now().UnixNano()))
		bestPeer = job.bestPeers[randSource.Intn(len(job.bestPeers))]
	} else {
		bestPeer, err = s.chooseBestSyncPeer(job)
		if err != nil {
			return errors.Wrap(err, "failed to fetch best peer")
		}
	}

	if job.mode == types.FullSync {
		if !job.snapDownloaded && s.isSnapSyncRequired(job.address) {
			if err = s.fetchAndStoreSnap(bestPeer, job); err != nil {
				return err
			}
		}
	}

	tsInfo := job.tesseractQueue.Peek()
	group, groupCtx := errgroup.WithContext(context.Background())

	group.Go(func() error {
		if err = s.syncLattice(groupCtx, tsInfo, job, bestPeer); err != nil {
			return errors.Wrap(err, "failed to sync lattice")
		}

		return nil
	})

	group.Go(func() error {
		for job.getCurrentHeight() <= job.getExpectedHeight() {
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			default:
			}

			tsInfo = job.tesseractQueue.Peek()
			for tsInfo == nil {
				select {
				case <-groupCtx.Done():
					return groupCtx.Err()
				case <-job.tesseractSignal:
					tsInfo = job.tesseractQueue.Peek()
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
						"missing tesseract ",
						"addr", tsInfo.tesseract.Address(),
						"at", tsInfo.tesseract.Height(),
					)

					jobState = Sleep

					return nil
				}

				isTesseractAdded, err := s.syncTesseract(tsInfo, job)
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

	if job.mode == types.FullSync {
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
		fmt.Println("Failed to signal new job")
	}
}

func (s *Syncer) updatePrincipalSyncStatus() error {
	if atomic.LoadUint64(&s.pendingAccounts) == 0 {
		return nil
	}

	if !s.isPrincipalSyncDone {
		atomic.AddUint64(&s.pendingAccounts, ^uint64(0))
	}

	if atomic.LoadUint64(&s.pendingAccounts) <= uint64(0) && s.isBucketSyncDone {
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
			s.logger.Error("Failed to update pending job status")
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

func (s *Syncer) findLatestHeightAndBestPeers(addr types.Address, localHeight uint64) (uint64, []id.KramaID, error) {
	//	responses := make([]*LatestAccountInfo, 0)
	bestPeers := make([]id.KramaID, 0)
	maxHeight := localHeight

	for _, kramaID := range s.network.GetPeers() {
		resp := new(LatestAccountInfo)
		if err := s.rpcClient.MoiCall(
			kramaID,
			"SYNCRPC",
			"GetLatestAccountInfo",
			addr,
			resp,
			time.Minute*2,
		); err != nil {
			s.logger.Error("Failed to fetch account latest status", "rpcError", err)

			continue
		}

		// TODO: Check if we need this
		// responses = append(responses, resp)

		if resp.Height >= maxHeight {
			maxHeight = resp.Height

			bestPeers = append(bestPeers, kramaID)
		}
	}

	return maxHeight, bestPeers, nil
}

func (s *Syncer) chooseBestSyncPeer(job *SyncJob) (id.KramaID, error) {
	var (
		maxHeight  = job.expectedHeight
		bestPeerID id.KramaID
	)

	if job.mode == types.LatestSync && job.tesseractQueue.Peek() != nil {
		randomSource := rand.New(rand.NewSource(time.Now().UnixNano()))
		randNumber := randomSource.Intn(len(job.tesseractQueue.Peek().clusterInfo.RandomSet))

		return job.tesseractQueue.Peek().clusterInfo.RandomSet[randNumber], nil
	}

	if len(s.network.GetPeers()) == 0 {
		return "", errors.New("empty peers list")
	}

	for _, kramaID := range s.network.GetPeers() {
		resp := new(LatestAccountInfo)
		if err := s.rpcClient.MoiCall(
			kramaID,
			"SYNCRPC",
			"GetLatestAccountInfo",
			job.address,
			resp,
			time.Minute*2,
		); err != nil {
			s.logger.Error("Failed to fetch account latest status", "rpcError", err)

			continue
		}

		if resp.Height >= maxHeight {
			maxHeight = resp.Height
			bestPeerID = kramaID
		}
	}

	return bestPeerID, nil
}

// syncSystemAccount sends a sync request for the specified address and waits for it to complete within a given time.
// If the sync does not complete within the specified time, an error is returned.
func (s *Syncer) syncSystemAccount(address types.Address) ([]id.KramaID, error) {
	bestHeight, bestPeers, err := s.findLatestHeightAndBestPeers(address, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch best peers and height")
	}

	if err = s.NewSyncRequest(address, bestHeight, types.FullSync, bestPeers); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(5000+(bestHeight*2000))*time.Millisecond)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}

		metaInfo, err := s.db.GetAccountMetaInfo(types.SargaAddress)
		if err == nil && metaInfo.Height == bestHeight {
			break
		}
	}

	return bestPeers, nil
}

func (s *Syncer) initSync() error {
	var principalSyncTimeStamp int64

	for s.network.Peers.Len() < 10 {
		time.Sleep(2 * time.Second)
	}

	s.isPrincipalSyncDone, principalSyncTimeStamp = s.db.IsPrincipalSyncDone()
	if s.isPrincipalSyncDone {
		s.logger.Info("Principal sync was finished at", "unix-time", principalSyncTimeStamp)
	}

	// Sync all system accounts
	bestPeers, err := s.syncSystemAccount(types.SargaAddress)
	if err != nil {
		s.logger.Error("Failed to sync sarga account", "error", err)

		return err
	}

	if err = s.loadSyncJobsFromDB(); err != nil {
		s.logger.Error("failed to load sync jobs from db", "error", err)
	}

	return s.syncBucketsWithMaxAttempts(bestPeers, MaxBucketSyncAttempts)
}

func (s *Syncer) syncBucketsWithMaxAttempts(bestPeers []id.KramaID, maxAttempts int) error {
	randomNumber := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 1; i < maxAttempts; i++ {
		bestPeer := bestPeers[randomNumber.Intn(len(bestPeers))]
		if err := s.syncBuckets(bestPeer, i); err != nil {
			s.logger.Error("failed to sync buckets, retrying...!!!", "error", err)

			continue
		}

		s.logger.Info("Bucket Sync Successful")

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
		s.logger.Error("failed to decode peer id", "error", err)
	}

	errGrp, grpCtx := errgroup.WithContext(s.ctx)

	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(grpCtx, peerID, "SYNCRPC", "SyncBucketsSince", argsChan, respChan); err != nil {
			s.logger.Error("failed to sync buckets", "error", err)

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
							s.logger.Error("Response chan closed")

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
							s.logger.Error("failed to create sync jobs from accMetaInfo", "error", err)

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
	var localHeight uint64

	accountSyncInfos, err := s.db.GetAccountsSyncStatus()
	if err != nil {
		return err
	}

	for _, v := range accountSyncInfos {
		accountMetaInfo, err := s.db.GetAccountMetaInfo(v.Address)
		if err == nil {
			localHeight = accountMetaInfo.Height
		}

		syncJob, err := SyncJobFromCanonicalInfo(s.logger, s.db, localHeight, v)
		if err != nil {
			return err
		}

		if err = s.jobQueue.AddJob(syncJob); err != nil {
			return err
		}
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
		s.logger.Error("failed to decode peer id", "error", err)

		return err
	}

	errGrp, grpCtx := errgroup.WithContext(s.ctx)

	errGrp.Go(func() error {
		if err = s.rpcClient.Stream(grpCtx, peerID, "SYNCRPC", "SyncBuckets", argsChan, respChan); err != nil {
			s.logger.Error("failed to sync buckets", "error", err)

			return err
		}

		return nil
	})

	errGrp.Go(func() error {
		defer close(argsChan)

		for i := uint64(0); i <= dhruva.MaxBucketCount; i++ {
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
							s.logger.Error("Invalid bucket")

							return errors.New("invalid bucket id")
						}

						if respMsg.BucketCount == 0 {
							return nil
						}

						if totalEntriesInBucket == 0 {
							totalEntriesInBucket = respMsg.BucketCount
						}

						// send the data to meta info handler
						if err = s.handleAccountMetaInfo(respMsg.AccountMetaInfos, types.FullSync); err != nil {
							s.logger.Error("failed to create sync jobs from accMetaInfo", "error", err)

							return err
						}

						totalEntriesInBucket -= uint64(len(respMsg.AccountMetaInfos))

						if totalEntriesInBucket == 0 {
							return nil
						}

					case <-time.After(time.Duration(5*attempts) * time.Second):
						return types.ErrTimeOut
					}
				}
			}()
			if err != nil {
				return err
			}
		}

		s.isBucketSyncDone = true

		return nil
	})

	return errGrp.Wait()
}

func (s *Syncer) handleAccountMetaInfo(data [][]byte, syncMode types.SyncMode) error {
	acc := new(types.AccountMetaInfo)
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
				"error", err,
			)
		}
	}

	return nil
}

func (s *Syncer) isSnapSyncRequired(address types.Address) bool {
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

	snap, err := s.fetchSnapShort(ctx, bestPeer, job.address, job.expectedHeight)
	if err != nil {
		return errors.Wrap(err, "failed to fetch snapshot")
	}

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
	address types.Address,
	expectedHeight uint64,
) (*types.Snapshot, error) {
	peerID, err := peer.DecodedPeerID()
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode peer ID")
	}

	var (
		snapInfo *SnapMetaInfo
		reqChan  = make(chan *SnapRequest, 1)
		respChan = make(chan *SnapResponse, 2)
	)

	currentSnap := &types.Snapshot{
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

				log.Println("Current Snap Size", snapInfo.TotalSnapSize, currentSnap.Size)
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
	s.rpcClient = s.network.StartNewRPCServer(common.SyncProtocolRPC)

	return s.network.RegisterNewRPCService(common.SyncProtocolRPC, "SYNCRPC", NewSyncRPCService(s))
}

func (s *Syncer) fromGenesis(addr types.Address, currentHeight uint64) bool {
	if currentHeight == 0 {
		_, err := s.db.GetAccountMetaInfo(addr)
		if errors.Is(err, types.ErrAccountNotFound) {
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
		respChan    = make(chan *ptypes.TesseractMessage, 5)
		reqChan     = make(chan *LatticeRequest, 1)
	)

	if nextTS != nil {
		if int64(nextTS.tesseract.Height()-(startHeight+1)) <= 0 {
			return nil
		}

		endHeight = nextTS.tesseract.Height() - 1
	}

	peerID, err := bestPeer.DecodedPeerID()
	if err != nil {
		return errors.Wrap(err, "failed to decode peerID")
	}

	s.logger.Debug("Sending lattice sync request", job.address)

	ctx, cancel := context.WithTimeout(ctx, time.Duration(endHeight+10)*time.Second)
	defer func() {
		cancel()
	}()

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
			s.logger.Error("Lattice fetch failed", "error", err)

			return err
		}

		return nil
	})

	grp.Go(func() error {
		defer close(reqChan)
		for requiredTesseractCount > 0 {
			select {
			case <-grpCtx.Done():
				return grpCtx.Err()

			case msg, ok := <-respChan:
				if !ok {
					return errors.New("receiver channel closed")
				}

				if msg.Tesseract.Height() >= startHeight && msg.Tesseract.Height() <= endHeight {
					requiredTesseractCount--
				}

				if job.tesseractQueue.Has(msg.Tesseract.Height()) {
					continue
				}

				tsInfo, err := s.tesseractInfoFromTesseractMsg(msg)
				if err != nil {
					s.logger.Error("failed to parse tesseract info from msg", "error", err)

					continue
				}

				s.logger.Debug(
					"adding tesseract to queue",
					"address", tsInfo.tesseract.Address(),
					"height", tsInfo.tesseract.Height(),
				)

				job.tesseractQueue.Push(tsInfo)
				job.signalNewTesseract()

				if job.getCurrentHeight() == endHeight {
					return nil
				}
			}
		}

		return nil
	})

	return grp.Wait()
}

func (s *Syncer) tesseractInfoFromTesseractMsg(msg *ptypes.TesseractMessage) (*TesseractInfo, error) {
	var err error

	info := &TesseractInfo{
		delta:         make(map[types.Hash][]byte),
		shouldExecute: false,
	}

	info.tesseract, err = msg.GetTesseract()
	if err != nil {
		return nil, err
	}

	clusterInfo := new(types.ICSClusterInfo)

	if !info.tesseract.ICSHash().IsNil() {
		info.delta[info.tesseract.ICSHash()] = msg.Delta[info.tesseract.ICSHash()]

		if err = polo.Depolorize(clusterInfo, info.delta[info.tesseract.ICSHash()]); err != nil {
			return nil, err
		}
	}

	info.icsNodeSet, err = s.state.GetICSNodeSetFromRawContext(info.tesseract, msg.Delta, clusterInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load ICSNodeSet")
	}

	return info, nil
}

func (s *Syncer) syncTesseract(msg *TesseractInfo, job *SyncJob) (bool, error) {
	var err error

	if msg.icsNodeSet == nil {
		msg.icsNodeSet, err = s.lattice.FetchICSNodeSet(msg.tesseract, msg.clusterInfo)
		if err != nil {
			s.logger.Error("failed to fetch node set", "error", err)

			return false, nil
		}
	}

	if !msg.shouldExecute {
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

		return true, nil
	}

	grid := s.gridStore.GetGrid(msg.tesseract.GridHash())
	if grid == nil {
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

func (s *Syncer) executeAndAdd(dirty map[types.Hash][]byte, grid *Grid) error {
	if err := s.lattice.ExecuteAndValidate(grid.Tesseracts()...); err != nil {
		return err
	}

	if err := s.lattice.AddTesseracts(dirty, grid.Tesseracts()...); err != nil {
		return err
	}

	return nil
}

// fetchTesseractState fetches the complete state(balance,context,approvals) of the given tesseract using agora
func (s *Syncer) fetchTesseractState(tesseract *types.Tesseract, fetchContext []id.KramaID) error {
	ctx, cancel := context.WithTimeout(context.Background(), TesseractFetchTimeOut) // TODO:Optimise timeout duration
	defer cancel()

	newSession, err := s.agora.NewSession(ctx, fetchContext, tesseract.Address(), accountCID(tesseract.StateHash()))
	if err != nil {
		return err
	}
	defer newSession.Close()

	islocal, acc, block, err := s.fetchAccount(ctx, newSession, tesseract.StateHash())
	if err != nil {
		return err
	}

	if err = s.fetchData(
		ctx,
		newSession,
		balanceCID(acc.Balance),
		approvalsCID(acc.AssetApprovals),
		// receiptsCID(tesseract.GridHash()),
	); err != nil {
		s.logger.Error("Error fetching balance data", "error", err)

		return err
	}

	if err = s.syncContextData(ctx, newSession, contextCID(acc.ContextHash)); err != nil {
		s.logger.Error("Error fetching context data", "error", err)

		return err
	}

	if tesseract.PrevHash().IsNil() {
		s.state.CreateDirtyObject(tesseract.Address(), types.AccTypeFromIxType(tesseract.Interactions()[0].Type()))
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
func (s *Syncer) getBlock(ctx context.Context, session *session.Session, cid atypes.CID) (bool, *atypes.Block, error) {
	data, err := s.db.ReadEntry(dbKeyFromCID(session.ID(), cid))
	if err == nil {
		return true, atypes.NewBlock(cid, data), nil
	}

	if errors.Is(err, types.ErrKeyNotFound) {
		block, err := session.GetBlock(ctx, cid)

		return false, block, err
	}

	return false, nil, err
}

// fetchAccount retrieves the account data for a given state hash from either the local database or the session,
// and returns the account data, along with the block that contains it.
// This also returns a bool value, indicating whether the data was found in the local database (true) or not (false).
func (s *Syncer) fetchAccount(
	ctx context.Context,
	session *session.Session,
	stateHash types.Hash,
) (
	bool,
	*types.Account,
	*atypes.Block,
	error,
) {
	islocal, block, err := s.getBlock(ctx, session, accountCID(stateHash))
	if err != nil {
		return false, nil, nil, err
	}

	acc := new(types.Account)
	if err = acc.FromBytes(block.GetData()); err != nil {
		return false, nil, nil, err
	}

	return islocal, acc, block, nil
}

// fetchData retrieves data blocks from the given session object and writes them to the database,
// using the specified CID values as keys.
func (s *Syncer) fetchData(ctx context.Context, session *session.Session, ids ...atypes.CID) error {
	keySet := atypes.NewHashSet()

	for _, cid := range ids {
		if !cid.IsNil() {
			if ok, err := s.db.Contains(dbKeyFromCID(session.ID(), cid)); !ok && err == nil {
				keySet.Add(cid)
			}
		}
	}

	if keySet.Len() == 0 {
		s.logger.Debug("Returning from get blocks : keySet is empty")

		return nil
	}

	receivedBlocksCount := 0

	blocksChan := session.GetBlocks(ctx, keySet.Keys())
	for block := range blocksChan {
		if err := s.db.CreateEntry(dbKeyFromCID(session.ID(), block.GetCid()), block.GetData()); err != nil {
			s.logger.Error("Error writing to db", "error", err)

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
func (s *Syncer) syncContextData(ctx context.Context, session *session.Session, cid atypes.CID) error {
	islocal, block, err := s.getBlock(ctx, session, cid)
	if err != nil {
		return err
	}

	metaContextObject := new(gtypes.MetaContextObject)
	if err = metaContextObject.FromBytes(block.GetData()); err != nil {
		return err
	}

	if err = s.fetchData(
		ctx,
		session,
		contextCID(metaContextObject.RandomContext),
		contextCID(metaContextObject.BehaviouralContext),
	); err != nil {
		return err
	}

	if !islocal {
		if err = s.db.CreateEntry(dbKeyFromCID(session.ID(), cid), block.GetData()); err != nil {
			return err
		}
	}

	return nil
}

func (s *Syncer) syncStorageTree(ctx context.Context, session *session.Session, newRoot types.Hash) error {
	if newRoot.IsNil() {
		return nil
	}

	_, block, err := s.getBlock(ctx, session, storageCID(newRoot))
	if err != nil {
		return err
	}

	metaStorageRoot := new(types.RootNode)
	if err = metaStorageRoot.FromBytes(block.GetData()); err != nil {
		return err
	}

	var (
		rootHashToLogicID = make(map[atypes.CID]string)

		storageTreeRoots = make(map[string]*types.RootNode, len(metaStorageRoot.HashTable))

		storageCIDs = make([]atypes.CID, 0, len(metaStorageRoot.HashTable))
	)

	for logicID, storageRoot := range metaStorageRoot.HashTable {
		rootCID := storageCID(types.BytesToHash(storageRoot))

		storageCIDs = append(storageCIDs, rootCID)

		rootHashToLogicID[rootCID] = logicID
	}

	ch := session.GetBlocks(ctx, storageCIDs)

	for block := range ch {
		rootNode := new(types.RootNode)
		if err = polo.Depolorize(&rootNode, block.GetData()); err != nil {
			return err
		}

		logicID, ok := rootHashToLogicID[block.GetCid()]
		if !ok {
			s.logger.Error("Received unwanted block")

			continue
		}

		storageTreeRoots[logicID] = rootNode
	}

	if len(storageTreeRoots) != len(metaStorageRoot.HashTable) {
		return errors.New("failed to fetch storage tree info")
	}

	if err := s.state.SyncStorageTrees(session.ID(), metaStorageRoot, storageTreeRoots); err != nil {
		s.logger.Error("Failed to sync storage tree", session.ID())

		return err
	}

	return nil
}

func (s *Syncer) syncLogicTree(ctx context.Context, session *session.Session, newRoot types.Hash) error {
	if newRoot.IsNil() {
		return nil
	}

	_, block, err := s.getBlock(ctx, session, logicCID(newRoot))
	if err != nil {
		return nil
	}

	metaLogicRoot := new(types.RootNode)
	if err = metaLogicRoot.FromBytes(block.GetData()); err != nil {
		return err
	}

	return s.state.SyncLogicTree(session.ID(), metaLogicRoot)
}

func (s *Syncer) tesseractHandler(msg *pubsub.Message) error {
	var (
		tsMsg = new(ptypes.TesseractMessage)
		err   error
	)

	if err = tsMsg.FromBytes(msg.GetData()); err != nil {
		return err
	}

	exists := s.db.HasTesseract(tsMsg.Tesseract.Hash())
	if exists {
		return types.ErrAlreadyKnown
	}

	ts, err := tsMsg.GetTesseract()
	if err != nil {
		return err
	}

	if !s.tesseractRegistry.Contains(ts.Hash()) {
		s.tesseractRegistry.Add(ts.Hash())

		s.logger.Trace(
			"Tesseract Received from",
			"Sender", tsMsg.Sender,
			"Sealer", ts.Sealer(),
			"Hash", ts.Hash(),
			"Address", ts.Address(),
		)

		clusterInfo := new(types.ICSClusterInfo)
		if err = clusterInfo.FromBytes(tsMsg.Delta[ts.ICSHash()]); err != nil {
			return err
		}

		if err = s.NewSyncRequest(
			ts.Address(),
			ts.Height(),
			types.LatestSync,
			clusterInfo.RandomSet,
			&TesseractInfo{
				tesseract:     ts,
				clusterInfo:   clusterInfo,
				icsNodeSet:    nil,
				shouldExecute: s.cfg.ShouldExecute,
				delta:         tsMsg.Delta,
			}); err != nil {
			s.logger.Error("Error adding sync request")
		}
	}

	return nil
}

func (s *Syncer) getTesseractWithRawIxnsAndReceipts(
	address types.Address,
	height uint64,
	withInteractions, withReceipts bool,
) (ts *types.Tesseract, ixns, receipts []byte, err error) {
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
func (s *Syncer) Start() error {
	s.agora.Start()

	if err := s.registerRPCService(); err != nil {
		return err
	}

	if err := s.network.Subscribe(s.ctx, TesseractTopic, s.tesseractHandler); err != nil {
		return err
	}

	s.startWorkers()

	go func() {
		if err := s.initSync(); err != nil {
			s.logger.Error("Initial sync failed", "error", err)

			return
		}

		s.logger.Info("Initial sync successful")
	}()

	go s.startSyncEventHandler()

	return nil
}

func (s *Syncer) startSyncEventHandler() {
	sub := s.mux.Subscribe(utils.SyncRequestEvent{})
	defer sub.Unsubscribe()

	for event := range sub.Chan() {
		req, ok := event.Data.(utils.SyncRequestEvent)
		if ok {
			if err := s.NewSyncRequest(
				req.Address,
				req.Height,
				types.LatestSync,
				[]id.KramaID{req.BestPeer},
			); err != nil {
				s.logger.Error("failed to handle sync request from krama engine", "error", err)
			}
		}
	}
}

// startWorkers will start the sync job workers
func (s *Syncer) startWorkers() {
	for i := uint32(0); i < s.jobWorkerCount; i++ {
		go s.worker()
	}
}

func (s *Syncer) Close() {
	log.Println("Closing syncer")
}

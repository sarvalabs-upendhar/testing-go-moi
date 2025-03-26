package forage

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
)

type JobState int

func (js JobState) String() string {
	switch js {
	case Pending:
		return "PENDING"
	case Active:
		return "ACTIVE"
	case Sleep:
		return "SLEEP"
	case Done:
		return "DONE"
	}

	return "INVALID JOB"
}

const (
	Pending JobState = iota
	Active
	Sleep
	Done
)

type AccountJobQueue struct {
	mtx   sync.RWMutex
	mux   *utils.TypeMux
	krama kramaEngine
	jobs  map[identifiers.Identifier]*AccountSyncJob
}

func NewAccountJobQueue(mux *utils.TypeMux, krama kramaEngine) *AccountJobQueue {
	return &AccountJobQueue{
		jobs:  make(map[identifiers.Identifier]*AccountSyncJob),
		mux:   mux,
		krama: krama,
	}
}

func (jq *AccountJobQueue) len() int {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	return len(jq.jobs)
}

func (jq *AccountJobQueue) getJob(id identifiers.Identifier) (*AccountSyncJob, bool) {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	job, ok := jq.jobs[id]

	return job, ok
}

func (jq *AccountJobQueue) AddJob(job *AccountSyncJob) error {
	jq.mtx.Lock()

	defer func() {
		jq.mtx.Unlock()
	}()

	if _, ok := jq.jobs[job.id]; ok {
		return errors.New("job already exists")
	}

	jq.jobs[job.id] = job

	if err := jq.mux.Post(utils.PendingAccountEvent{ID: job.id, Count: 1}); err != nil {
		log.Println("Error sending pending account event", "err", err)
	}

	return nil
}

func (jq *AccountJobQueue) NextJob(
	updateJob func(jq *AccountJobQueue, jb *AccountSyncJob) *AccountSyncJob,
) *AccountSyncJob {
	jq.mtx.Lock()
	defer func() {
		jq.mtx.Unlock()
	}()

	for _, syncJob := range jq.jobs {
		if j := updateJob(jq, syncJob); j != nil {
			return j
		}
	}

	return nil
}

func (jq *AccountJobQueue) RemoveJob(job *AccountSyncJob) error {
	if err := job.done(); err != nil {
		return err
	}

	delete(jq.jobs, job.id)

	// If the account sync is triggered by tesseract worker, tesseract worker should clear active accounts
	if job.tsWorkerSignal == nil {
		// unlock the account as it is synced
		jq.krama.ClearActiveAccounts(accountWorker, job.id)
	}

	if err := jq.mux.Post(utils.PendingAccountEvent{ID: job.id, Count: -1}); err != nil {
		log.Println("Error sending pending account event", "err", err)
	}

	if err := jq.publishEventJobDone(job.jobStateEvent()); err != nil {
		return err
	}

	return nil
}

func (jq *AccountJobQueue) getPendingAccounts() []identifiers.Identifier {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	pendingAccounts := make([]identifiers.Identifier, 0, len(jq.jobs))

	for _, jb := range jq.jobs {
		pendingAccounts = append(pendingAccounts, jb.id)
	}

	return pendingAccounts
}

func (jq *AccountJobQueue) post(ev interface{}) error {
	return jq.mux.Post(ev)
}

func (jq *AccountJobQueue) publishEventJobDone(state eventDataJobState) error {
	return jq.post(eventJobDone{state})
}

type AccountSyncJob struct {
	mtx                   sync.RWMutex
	logger                hclog.Logger
	db                    store
	id                    identifiers.Identifier
	mode                  common.SyncMode
	creationTime          time.Time
	snapDownloaded        bool
	expectedHeight        uint64
	currentHeight         uint64
	jobState              JobState
	lastModifiedAt        time.Time
	tesseractQueue        *TesseractQueue
	tesseractSignal       chan struct{}
	bestPeers             map[identifiers.KramaID]struct{}
	latticeSyncInProgress bool
	selfAccLock           bool
	tsWorkerSignal        chan struct{}
}

func AccountSyncJobFromCanonicalInfo(
	logger hclog.Logger,
	db store,
	data *common.AccountSyncStatus,
) (*AccountSyncJob, error) {
	modifiedTime := new(time.Time)

	err := modifiedTime.UnmarshalText(data.LastModifiedAt)
	if err != nil {
		return nil, err
	}

	// Ensure the job state is set to 'pending' to allow proper creation and progress tracking.
	// If the state is mistakenly stored as 'active',
	// workers may misinterpret its status and the job won't make progress.
	return &AccountSyncJob{
		db:              db,
		logger:          logger.Named("Sync-Job"),
		id:              data.ID,
		creationTime:    time.Now(),
		snapDownloaded:  data.SnapshotDownloaded,
		mode:            data.Mode,
		expectedHeight:  data.ExpectedHeight,
		jobState:        Pending,
		lastModifiedAt:  *modifiedTime,
		tesseractQueue:  NewTesseractQueue(),
		tesseractSignal: make(chan struct{}, 1),
		bestPeers:       make(map[identifiers.KramaID]struct{}),
	}, nil
}

func (j *AccountSyncJob) isLatticeSyncInProgress() bool {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.latticeSyncInProgress
}

func (j *AccountSyncJob) setLatticeSyncInProgress(val bool) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.latticeSyncInProgress = val
}

func (j *AccountSyncJob) bestPeerLen() int {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	return len(j.bestPeers)
}

func (j *AccountSyncJob) updateBestPeers(peers []identifiers.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	for _, peer := range peers {
		j.bestPeers[peer] = struct{}{}
	}
}

func (j *AccountSyncJob) deleteBestPeer(peer identifiers.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	delete(j.bestPeers, peer)
}

func (j *AccountSyncJob) chooseRandomBestPeer() identifiers.KramaID {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	randSource := rand.New(rand.NewSource(time.Now().UnixNano()))
	num := randSource.Intn(len(j.bestPeers))

	for peer := range j.bestPeers {
		if num == 0 {
			return peer
		}

		num--
	}

	return ""
}

func (j *AccountSyncJob) getJobState() JobState {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.jobState
}

func (j *AccountSyncJob) getExpectedHeight() uint64 {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.expectedHeight
}

func (j *AccountSyncJob) updateSnap(snapReceived bool) error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.snapDownloaded = snapReceived

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.id, canonicalJob)
}

func (j *AccountSyncJob) done() error {
	if err := j.db.CleanupAccountSyncStatus(j.id); err != nil {
		return errors.Wrap(err, "failed to delete entry in db")
	}

	return nil
}

func (j *AccountSyncJob) getCurrentHeight() uint64 {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.currentHeight
}

func (j *AccountSyncJob) updateCurrentHeight(h uint64) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.currentHeight = h
}

func (j *AccountSyncJob) updateJobState(newState JobState) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.logger.Debug("Updating job state", "accountID", j.id, "state", newState)

	j.jobState = newState
	j.lastModifiedAt = time.Now()
}

func (j *AccountSyncJob) updateExpectedHeight(newHeight uint64) error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.expectedHeight = newHeight

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.id, canonicalJob)
}

func (j *AccountSyncJob) commitJob() error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.id, canonicalJob)
}

func (j *AccountSyncJob) canonicalJob() (*common.AccountSyncStatus, error) {
	rawTime, err := j.lastModifiedAt.MarshalText()
	if err != nil {
		return nil, err
	}

	return &common.AccountSyncStatus{
		ID:                 j.id,
		SnapshotDownloaded: j.snapDownloaded,
		Mode:               j.mode,
		State:              int32(j.jobState),
		LastModifiedAt:     rawTime,
		ExpectedHeight:     j.expectedHeight,
	}, nil
}

func (j *AccountSyncJob) signalNewTesseract() {
	select {
	case j.tesseractSignal <- struct{}{}:
	default:
		go func() {
			j.tesseractSignal <- struct{}{}
		}()
	}
}

func (j *AccountSyncJob) jobStateEvent() eventDataJobState {
	return eventDataJobState{
		id:     j.id,
		height: j.getCurrentHeight(),
	}
}

package forage

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
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

type JobQueue struct {
	mtx   sync.RWMutex
	mux   *utils.TypeMux
	krama kramaEngine
	jobs  map[identifiers.Identifier]*SyncJob
}

func NewJobQueue(mux *utils.TypeMux, krama kramaEngine) *JobQueue {
	return &JobQueue{
		jobs:  make(map[identifiers.Identifier]*SyncJob),
		mux:   mux,
		krama: krama,
	}
}

func (jq *JobQueue) len() int {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	return len(jq.jobs)
}

func (jq *JobQueue) getJob(id identifiers.Identifier) (*SyncJob, bool) {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	job, ok := jq.jobs[id]

	return job, ok
}

func (jq *JobQueue) AddJob(job *SyncJob) error {
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

func (jq *JobQueue) NextJob(updateJob func(jq *JobQueue, jb *SyncJob) *SyncJob) *SyncJob {
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

func (jq *JobQueue) RemoveJob(job *SyncJob) error {
	if err := job.done(); err != nil {
		return err
	}

	delete(jq.jobs, job.id)

	// unlock the account as it is synced
	jq.krama.ClearActiveAccount(job.id, "syncer")

	if err := jq.mux.Post(utils.PendingAccountEvent{ID: job.id, Count: -1}); err != nil {
		log.Println("Error sending pending account event", "err", err)
	}

	if err := jq.publishEventJobDone(job.jobStateEvent()); err != nil {
		return err
	}

	return nil
}

func (jq *JobQueue) GetPendingAccounts() []identifiers.Identifier {
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

func (jq *JobQueue) post(ev interface{}) error {
	return jq.mux.Post(ev)
}

func (jq *JobQueue) publishEventJobDone(state eventDataJobState) error {
	return jq.post(eventJobDone{state})
}

type SyncJob struct {
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
	bestPeers             map[kramaid.KramaID]struct{}
	latticeSyncInProgress bool
	selfAccLock           bool
}

func SyncJobFromCanonicalInfo(
	logger hclog.Logger,
	db store,
	data *common.AccountSyncStatus,
) (*SyncJob, error) {
	modifiedTime := new(time.Time)

	err := modifiedTime.UnmarshalText(data.LastModifiedAt)
	if err != nil {
		return nil, err
	}

	// Ensure the job state is set to 'pending' to allow proper creation and progress tracking.
	// If the state is mistakenly stored as 'active',
	// workers may misinterpret its status and the job won't make progress.
	return &SyncJob{
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
		bestPeers:       make(map[kramaid.KramaID]struct{}),
	}, nil
}

func (j *SyncJob) isLatticeSyncInProgress() bool {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.latticeSyncInProgress
}

func (j *SyncJob) setLatticeSyncInProgress(val bool) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.latticeSyncInProgress = val
}

func (j *SyncJob) bestPeerLen() int {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	return len(j.bestPeers)
}

func (j *SyncJob) updateBestPeers(peers []kramaid.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	for _, peer := range peers {
		j.bestPeers[peer] = struct{}{}
	}
}

func (j *SyncJob) deleteBestPeer(peer kramaid.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	delete(j.bestPeers, peer)
}

func (j *SyncJob) chooseRandomBestPeer() kramaid.KramaID {
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

func (j *SyncJob) getJobState() JobState {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.jobState
}

func (j *SyncJob) getExpectedHeight() uint64 {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.expectedHeight
}

func (j *SyncJob) updateSnap(snapReceived bool) error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.snapDownloaded = snapReceived

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.id, canonicalJob)
}

func (j *SyncJob) done() error {
	if err := j.db.CleanupAccountSyncStatus(j.id); err != nil {
		return errors.Wrap(err, "failed to delete entry in db")
	}

	return nil
}

func (j *SyncJob) getCurrentHeight() uint64 {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.currentHeight
}

func (j *SyncJob) updateCurrentHeight(h uint64) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.currentHeight = h
}

func (j *SyncJob) updateJobState(newState JobState) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.logger.Debug("Updating job state", "accountID", j.id, "state", newState)

	j.jobState = newState
	j.lastModifiedAt = time.Now()
}

func (j *SyncJob) updateExpectedHeight(newHeight uint64) error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.expectedHeight = newHeight

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.id, canonicalJob)
}

func (j *SyncJob) commitJob() error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.id, canonicalJob)
}

func (j *SyncJob) canonicalJob() (*common.AccountSyncStatus, error) {
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

func (j *SyncJob) signalNewTesseract() {
	select {
	case j.tesseractSignal <- struct{}{}:
	default:
		go func() {
			j.tesseractSignal <- struct{}{}
		}()
	}
}

func (j *SyncJob) jobStateEvent() eventDataJobState {
	return eventDataJobState{
		id:     j.id,
		height: j.getCurrentHeight(),
	}
}

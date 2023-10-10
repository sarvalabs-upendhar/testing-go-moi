package forage

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/utils"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
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
	mtx  sync.RWMutex
	mux  *utils.TypeMux
	jobs map[common.Address]*SyncJob
}

func NewJobQueue(mux *utils.TypeMux) *JobQueue {
	return &JobQueue{
		jobs: make(map[common.Address]*SyncJob),
		mux:  mux,
	}
}

func (jq *JobQueue) len() int {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	return len(jq.jobs)
}

func (jq *JobQueue) getJob(address common.Address) (*SyncJob, bool) {
	jq.mtx.RLock()
	defer func() {
		jq.mtx.RUnlock()
	}()

	job, ok := jq.jobs[address]

	return job, ok
}

func (jq *JobQueue) AddJob(job *SyncJob) error {
	jq.mtx.Lock()

	defer func() {
		jq.mtx.Unlock()
	}()

	if _, ok := jq.jobs[job.address]; ok {
		return errors.New("job already exists")
	}

	jq.jobs[job.address] = job

	if err := jq.mux.Post(utils.PendingAccountEvent{Address: job.address, Count: 1}); err != nil {
		log.Println("Error sending pending account event", "err", err)
	}

	return nil
}

func (jq *JobQueue) NextJob() *SyncJob {
	jq.mtx.Lock()
	defer func() {
		jq.mtx.Unlock()
	}()

	updateJob := func(jb *SyncJob) *SyncJob {
		if jb.getJobState() == Pending ||
			(jb.getJobState() == Sleep && time.Since(jb.lastModifiedAt) > time.Millisecond*200) {
			jb.updateJobState(Active)

			return jb
		}

		if jb.getJobState() == Done && jb.tesseractQueue.Len() == 0 {
			if err := jq.RemoveJob(jb); err != nil {
				log.Panicln(err)
			}
		}

		return nil
	}

	for _, syncJob := range jq.jobs {
		if j := updateJob(syncJob); j != nil {
			return j
		}
	}

	return nil
}

func (jq *JobQueue) RemoveJob(job *SyncJob) error {
	if err := job.done(); err != nil {
		return err
	}

	delete(jq.jobs, job.address)

	if err := jq.mux.Post(utils.PendingAccountEvent{Address: job.address, Count: -1}); err != nil {
		log.Println("Error sending pending account event", "err", err)
	}

	if err := jq.publishEventJobDone(job.jobStateEvent()); err != nil {
		return err
	}

	return nil
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
	address               common.Address
	mode                  common.SyncMode
	snapDownloaded        bool
	expectedHeight        uint64
	currentHeight         uint64
	jobState              JobState
	lastModifiedAt        time.Time
	tesseractQueue        *TesseractQueue
	tesseractSignal       chan struct{}
	bestPeers             map[id.KramaID]struct{}
	latticeSyncInProgress bool
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

	return &SyncJob{
		db:              db,
		logger:          logger.Named("Sync-Job"),
		address:         data.Address,
		snapDownloaded:  data.SnapshotDownloaded,
		mode:            data.Mode,
		expectedHeight:  data.ExpectedHeight,
		lastModifiedAt:  *modifiedTime,
		tesseractQueue:  NewTesseractQueue(),
		tesseractSignal: make(chan struct{}, 1),
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

func (j *SyncJob) updateBestPeers(peers []id.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	for _, peer := range peers {
		j.bestPeers[peer] = struct{}{}
	}
}

func (j *SyncJob) deleteBestPeer(peer id.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	delete(j.bestPeers, peer)
}

func (j *SyncJob) chooseRandomBestPeer() id.KramaID {
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

	return j.db.SetAccountSyncStatus(j.address, canonicalJob)
}

func (j *SyncJob) done() error {
	if err := j.db.CleanupAccountSyncStatus(j.address); err != nil {
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

	j.logger.Debug("Updating job state", "addr", j.address, "state", newState)

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

	return j.db.SetAccountSyncStatus(j.address, canonicalJob)
}

func (j *SyncJob) commitJob() error {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	canonicalJob, err := j.canonicalJob()
	if err != nil {
		return err
	}

	return j.db.SetAccountSyncStatus(j.address, canonicalJob)
}

func (j *SyncJob) canonicalJob() (*common.AccountSyncStatus, error) {
	rawTime, err := j.lastModifiedAt.MarshalText()
	if err != nil {
		return nil, err
	}

	return &common.AccountSyncStatus{
		Address:            j.address,
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
		address: j.address,
		height:  j.getCurrentHeight(),
	}
}

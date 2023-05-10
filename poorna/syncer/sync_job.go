package syncer

import (
	"log"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
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
	jobs map[types.Address]*SyncJob
}

func (jq *JobQueue) getJob(address types.Address) (*SyncJob, bool) {
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

	return nil
}

func (jq *JobQueue) NextJob() *SyncJob {
	jq.mtx.Lock()
	defer func() {
		jq.mtx.Unlock()
	}()

	for _, syncJob := range jq.jobs {
		job := syncJob

		j := func(jb *SyncJob) *SyncJob {
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
		}(job)

		if j != nil {
			return j
		}
	}

	return nil
}

func (jq *JobQueue) RemoveJob(job *SyncJob) error {
	defer func() {
		delete(jq.jobs, job.address)
	}()

	return job.done()
}

type SyncJob struct {
	mtx             sync.RWMutex
	logger          hclog.Logger
	db              store
	address         types.Address
	mode            types.SyncMode
	snapDownloaded  bool
	expectedHeight  uint64
	currentHeight   uint64
	jobState        JobState
	lastModifiedAt  time.Time
	hash            types.Hash
	tesseractQueue  *TesseractQueue
	tesseractSignal chan struct{}
	bestPeers       []id.KramaID
}

func SyncJobFromCanonicalInfo(
	logger hclog.Logger,
	db store,
	currentHeight uint64,
	data *types.AccountSyncStatus,
) (*SyncJob, error) {
	modifiedTime := new(time.Time)

	err := modifiedTime.UnmarshalText(data.LastModifiedAt)
	if err != nil {
		return nil, err
	}

	return &SyncJob{
		db:             db,
		logger:         logger,
		address:        data.Address,
		snapDownloaded: data.SnapshotDownloaded,
		mode:           data.Mode,
		expectedHeight: data.ExpectedHeight,
		lastModifiedAt: *modifiedTime,
		tesseractQueue: NewTesseractQueue(),
		hash:           data.CurrentHash,
		currentHeight:  currentHeight,
	}, nil
}

func (j *SyncJob) updateBestPeers(peers []id.KramaID) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	if len(peers) == 0 {
		return
	}

	j.bestPeers = append(j.bestPeers, peers...)
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

	j.logger.Debug("Updating job state", "address", j.address, "status", newState)

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

func (j *SyncJob) canonicalJob() (*types.AccountSyncStatus, error) {
	rawTime, err := j.lastModifiedAt.MarshalText()
	if err != nil {
		return nil, err
	}

	return &types.AccountSyncStatus{
		Address:            j.address,
		SnapshotDownloaded: j.snapDownloaded,
		Mode:               j.mode,
		CurrentHash:        j.hash,
		State:              int32(j.jobState),
		LastModifiedAt:     rawTime,
		ExpectedHeight:     j.expectedHeight,
	}, nil
}

func (j *SyncJob) signalNewTesseract() {
	select {
	case j.tesseractSignal <- struct{}{}:
	default:
	}
}

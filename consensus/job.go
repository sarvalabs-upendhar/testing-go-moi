package consensus

import (
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/consensus/types"
)

type jobState int

const (
	Pending jobState = iota
	Active
	Sleep
	Done
)

type PreparedMessage struct {
	msg    *types.Prepared
	sender identifiers.KramaID
}

type job struct {
	mtx          sync.RWMutex
	creationTime time.Time
	msgs         map[common.Hash]*PreparedMessage
	clusterID    common.ClusterID
}

func (j *job) nextPrepared() *PreparedMessage {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	for hash, msg := range j.msgs {
		j.deleteMsg(hash)

		return msg
	}

	return nil
}

func (j *job) deleteMsg(hash common.Hash) {
	delete(j.msgs, hash)
}

type jobQueue struct {
	mtx        sync.RWMutex
	jobs       map[common.ClusterID]*job
	activeJobs map[common.ClusterID]struct{} // TO avoid processing same cluster parallelly
}

func newJobQueue() *jobQueue {
	return &jobQueue{
		jobs:       make(map[common.ClusterID]*job),
		activeJobs: make(map[common.ClusterID]struct{}),
	}
}

func (jq *jobQueue) len() int {
	jq.mtx.Lock()
	defer jq.mtx.Unlock()

	return len(jq.jobs)
}

// Push appends the sender to the existing job's sender list if it exists,
// or creates a new job if no existing job is found.
func (jq *jobQueue) push(clusterID common.ClusterID, msg *types.Prepared, sender identifiers.KramaID) error {
	jq.mtx.Lock()
	defer jq.mtx.Unlock()

	hash, err := msg.Hash()
	if err != nil {
		return err
	}

	if j, ok := jq.jobs[clusterID]; ok {
		j.mtx.Lock()

		j.msgs[hash] = &PreparedMessage{msg: msg, sender: sender}

		j.mtx.Unlock()

		return nil
	}

	jq.jobs[clusterID] = &job{
		creationTime: time.Now(),
		clusterID:    clusterID,
		msgs: map[common.Hash]*PreparedMessage{
			hash: {msg: msg, sender: sender},
		},
	}

	return nil
}

func (jq *jobQueue) next() *job {
	jq.mtx.Lock()
	defer jq.mtx.Unlock()

	for _, j := range jq.jobs {
		if _, ok := jq.activeJobs[j.clusterID]; ok {
			continue
		}

		jq.delete(j.clusterID)

		jq.activeJobs[j.clusterID] = struct{}{}

		return j
	}

	return nil
}

func (jq *jobQueue) deleteActiveJob(clusterID common.ClusterID) {
	jq.mtx.Lock()
	defer jq.mtx.Unlock()

	delete(jq.activeJobs, clusterID)
}

func (jq *jobQueue) delete(id common.ClusterID) {
	delete(jq.jobs, id)
}

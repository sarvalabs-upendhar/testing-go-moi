package forage

import (
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractJob struct {
	creationTime time.Time
	tsInfo       *TesseractInfo
}
type TesseractJobQueue struct {
	mtx     sync.RWMutex
	metrics *Metrics
	jobs    map[common.Hash]*TesseractJob
}

func NewTesseractJobQueue(metrics *Metrics) *TesseractJobQueue {
	return &TesseractJobQueue{
		metrics: metrics,
		jobs:    make(map[common.Hash]*TesseractJob),
	}
}

func (tq *TesseractJobQueue) push(info *TesseractInfo) {
	tq.mtx.Lock()
	defer tq.mtx.Unlock()

	tq.jobs[info.tesseract.Hash()] = &TesseractJob{
		creationTime: time.Now(),
		tsInfo:       info,
	}
}

func (tq *TesseractJobQueue) nextTesseractInfo(lockAccounts func(ts *common.Tesseract) bool) *TesseractInfo {
	tq.mtx.Lock()
	defer tq.mtx.Unlock()

	for _, job := range tq.jobs {
		if lockAccounts(job.tsInfo.tesseract) {
			return job.tsInfo
		}
	}

	return nil
}

// delete deletes the tesseract and also unlocks the participants
func (tq *TesseractJobQueue) delete(ts *common.Tesseract) {
	tq.mtx.Lock()
	defer tq.mtx.Unlock()

	job, ok := tq.jobs[ts.Hash()]
	if ok {
		tq.metrics.captureTSTimeInQueue(job.creationTime)
		delete(tq.jobs, ts.Hash())
	}
}

func (tq *TesseractJobQueue) getPendingTesseractHashes() []common.Hash {
	tq.mtx.RLock()
	defer func() {
		tq.mtx.RUnlock()
	}()

	hashes := make([]common.Hash, 0, len(tq.jobs))

	for hash := range tq.jobs {
		hashes = append(hashes, hash)
	}

	return hashes
}

package session

import (
	"log"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/syncer/cid"
)

type WantTracker struct {
	mtx       sync.RWMutex
	fetched   *Queue
	liveWants map[cid.CID]time.Time
}

func NewWantTracker() *WantTracker {
	return &WantTracker{
		fetched:   NewCidQueue(),
		liveWants: make(map[cid.CID]time.Time),
	}
}

func (wt *WantTracker) UpdateLiveWants(keys *cid.CIDSet) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	if err := keys.ForEach(func(c cid.CID) error {
		reqTime, ok := wt.liveWants[c]
		if !ok || time.Since(reqTime) > 200*time.Millisecond {
			wt.liveWants[c] = time.Now()
		}

		return nil
	}); err != nil {
		log.Print("error removing redundant keys")
	}
}

func (wt *WantTracker) RemoveRedundantKeys(cids *cid.CIDSet) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	redundantKeys := make([]cid.CID, 0)

	if err := cids.ForEach(func(c cid.CID) error {
		reqTime, ok := wt.liveWants[c]

		if ok && time.Since(reqTime) < 200*time.Millisecond {
			redundantKeys = append(redundantKeys, c)
		}

		return nil
	}); err != nil {
		log.Print("error removing redundant keys")
	}

	for _, cID := range redundantKeys {
		cids.Remove(cID)
	}
}

func (wt *WantTracker) RemoveCid(cid cid.CID) {
	wt.mtx.Lock()
	defer wt.mtx.Unlock()

	delete(wt.liveWants, cid)
}

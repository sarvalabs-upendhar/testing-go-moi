package forage

import (
	"context"
	"log"
	"sync"

	"github.com/sarvalabs/go-moi/common/utils"
)

// SyncStatusTracker represents a sync status tracker.
type SyncStatusTracker struct {
	mtx             sync.RWMutex
	pendingAccounts uint64
}

// NewSyncStatusTracker creates a new SyncStatusTracker with the given count
func NewSyncStatusTracker(count uint64) *SyncStatusTracker {
	return &SyncStatusTracker{
		pendingAccounts: count,
	}
}

// StartSyncStatusTracker starts the sync status tracker
func (s *SyncStatusTracker) StartSyncStatusTracker(ctx context.Context, eventSub *utils.Subscription) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-eventSub.Chan():
			if ok {
				event, ok := msg.Data.(utils.PendingAccountEvent)
				if !ok {
					log.Println("Error casting event data to pending account event")
				} else {
					s.UpdatePendingAccounts(event.Count)
				}
			}
		}
	}
}

// UpdatePendingAccounts increments pending count by given count
func (s *SyncStatusTracker) UpdatePendingAccounts(count int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if count < 0 {
		s.pendingAccounts--

		return
	}

	s.pendingAccounts++
}

// ReadPendingAccounts returns the total pending accounts
func (s *SyncStatusTracker) ReadPendingAccounts() uint64 {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.pendingAccounts
}

package session

import (
	"sync"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type InterestManager struct {
	mutex sync.RWMutex
	wants map[cid.CID]map[common.Address]bool
}

func NewInterestManager() *InterestManager {
	return &InterestManager{
		wants: make(map[cid.CID]map[common.Address]bool),
	}
}

func (im *InterestManager) RecordSessionInterest(addr common.Address, ids ...cid.CID) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// For each key
	for _, c := range ids {
		// Record that the session wants the blocks
		if want, ok := im.wants[c]; ok {
			want[addr] = true
		} else {
			im.wants[c] = map[common.Address]bool{addr: true}
		}
	}
}

func (im *InterestManager) RemoveSession(addr common.Address) []cid.CID {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKeys := make([]cid.CID, 0)

	// For each known key
	for c := range im.wants {
		// Remove the session from the list of sessions that want the key
		delete(im.wants[c], addr)

		// If there are no more sessions that want the key
		if len(im.wants[c]) == 0 {
			// Clean up the list memory
			delete(im.wants, c)
			// Add the key to the list of keys that no session is interested in
			deletedKeys = append(deletedKeys, c)
		}
	}

	return deletedKeys
}

func (im *InterestManager) RemoveSessionInterest(addr common.Address, ids ...cid.CID) []cid.CID {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKs := make([]cid.CID, 0, len(ids))

	// For each key
	for _, c := range ids {
		// If there is a list of sessions that want the key
		if _, ok := im.wants[c]; ok {
			// Remove the session from the list of sessions that want the key
			delete(im.wants[c], addr)

			// If there are no more sessions that want the key
			if len(im.wants[c]) == 0 {
				// Clean up the list memory
				delete(im.wants, c)
				// Add the key to the list of keys that no session is interested in
				deletedKs = append(deletedKs, c)
			}
		}
	}

	return deletedKs
}

func (im *InterestManager) InterestedSessions(
	blocks []block.Block,
) (map[common.Address][]block.Block, []block.Block) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	sessions := make(map[common.Address][]block.Block)
	orphans := make([]block.Block, 0)

	for _, blk := range blocks {
		interestedSessions, ok := im.wants[blk.GetCid()]
		if ok {
			for addr := range interestedSessions {
				sessions[addr] = append(sessions[addr], blk)
			}
		} else {
			orphans = append(orphans, blk)
		}
	}

	return sessions, orphans
}

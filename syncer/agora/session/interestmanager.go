package session

import (
	"sync"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type InterestManager struct {
	mutex sync.RWMutex
	wants map[cid.CID]map[identifiers.Address]bool
}

func NewInterestManager() *InterestManager {
	return &InterestManager{
		wants: make(map[cid.CID]map[identifiers.Address]bool),
	}
}

func (im *InterestManager) RecordSessionInterest(addr identifiers.Address, ids ...cid.CID) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// For each key
	for _, c := range ids {
		// Record that the session wants the blocks
		if want, ok := im.wants[c]; ok {
			want[addr] = true
		} else {
			im.wants[c] = map[identifiers.Address]bool{addr: true}
		}
	}
}

func (im *InterestManager) RemoveSessionInterest(addr identifiers.Address, ids ...cid.CID) []cid.CID {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKeys := make([]cid.CID, 0, len(ids))

	// For each key
	for _, c := range ids {
		// If there is a list of sessions that want the key
		if _, ok := im.wants[c]; ok {
			deleteSession(c, im.wants, addr, &deletedKeys)
		}
	}

	return deletedKeys
}

func (im *InterestManager) InterestedSessions(
	blocks []block.Block,
) (map[identifiers.Address][]block.Block, []block.Block) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	sessions := make(map[identifiers.Address][]block.Block)
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

func deleteSession(
	c cid.CID,
	wants map[cid.CID]map[identifiers.Address]bool,
	addr identifiers.Address,
	deletedKeys *[]cid.CID,
) {
	// Remove the session from the list of sessions that want the key
	delete(wants[c], addr)

	// If there are no more sessions that want the key
	if len(wants[c]) == 0 {
		// Clean up the list memory
		delete(wants, c)

		// Add the key to the list of keys that no session is interested in
		*deletedKeys = append(*deletedKeys, c)
	}
}

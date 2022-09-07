package session

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"sync"
)

type InterestManager struct {
	mutex sync.RWMutex
	wants map[ktypes.Hash]map[ktypes.Address]bool
}

func NewInterestManager() *InterestManager {
	return &InterestManager{
		wants: make(map[ktypes.Hash]map[ktypes.Address]bool),
	}
}

func (im *InterestManager) RecordSessionInterest(addr ktypes.Address, ids ...ktypes.Hash) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// For each key
	for _, c := range ids {
		// Record that the session wants the blocks
		if want, ok := im.wants[c]; ok {
			want[addr] = true
		} else {
			im.wants[c] = map[ktypes.Address]bool{addr: true}
		}
	}
}

func (im *InterestManager) RemoveSession(addr ktypes.Address) []ktypes.Hash {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKeys := make([]ktypes.Hash, 0)

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

func (im *InterestManager) RemoveSessionInterest(addr ktypes.Address, ids ...ktypes.Hash) []ktypes.Hash {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKs := make([]ktypes.Hash, 0, len(ids))

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

func (im *InterestManager) InterestedSessions(blocks []types.Block) (map[ktypes.Address][]types.Block, []types.Block) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	sessions := make(map[ktypes.Address][]types.Block)
	orphans := make([]types.Block, 0)

	for _, block := range blocks {
		interestedSessions, ok := im.wants[block.GetID()]
		if ok {
			for addr := range interestedSessions {
				sessions[addr] = append(sessions[addr], block)
			}
		} else {
			orphans = append(orphans, block)
		}
	}

	return sessions, orphans
}

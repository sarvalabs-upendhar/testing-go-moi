package session

import (
	"sync"

	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/moichain/types"
)

type InterestManager struct {
	mutex sync.RWMutex
	wants map[atypes.CID]map[types.Address]bool
}

func NewInterestManager() *InterestManager {
	return &InterestManager{
		wants: make(map[atypes.CID]map[types.Address]bool),
	}
}

func (im *InterestManager) RecordSessionInterest(addr types.Address, ids ...atypes.CID) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// For each key
	for _, c := range ids {
		// Record that the session wants the blocks
		if want, ok := im.wants[c]; ok {
			want[addr] = true
		} else {
			im.wants[c] = map[types.Address]bool{addr: true}
		}
	}
}

func (im *InterestManager) RemoveSession(addr types.Address) []atypes.CID {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKeys := make([]atypes.CID, 0)

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

func (im *InterestManager) RemoveSessionInterest(addr types.Address, ids ...atypes.CID) []atypes.CID {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKs := make([]atypes.CID, 0, len(ids))

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
	blocks []atypes.Block,
) (map[types.Address][]atypes.Block, []atypes.Block) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	sessions := make(map[types.Address][]atypes.Block)
	orphans := make([]atypes.Block, 0)

	for _, block := range blocks {
		interestedSessions, ok := im.wants[block.GetCid()]
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

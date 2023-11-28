package ixpool

import (
	"sync"

	"github.com/sarvalabs/go-moi/common"
)

// Lookup map used to find interactions present in the pool
type lookupMap struct {
	sync.RWMutex
	all map[common.Hash]*common.Interaction
}

func NewLookupMap() *lookupMap {
	l := &lookupMap{
		all: make(map[common.Hash]*common.Interaction, 0),
	}

	return l
}

// add inserts the provided Interaction into the map.
// It returns true if the insertion is successful, otherwise false. [thread-safe]
func (m *lookupMap) add(ix *common.Interaction) bool {
	m.Lock()
	defer m.Unlock()

	if _, exists := m.all[ix.Hash()]; exists {
		return false
	}

	m.all[ix.Hash()] = ix

	return true
}

// remove deletes the given Interactions from the map. [thread-safe]
func (m *lookupMap) remove(ixs ...*common.Interaction) {
	m.Lock()
	defer m.Unlock()

	for _, ix := range ixs {
		delete(m.all, ix.Hash())
	}
}

// get returns the Interaction associated with the given hash. [thread-safe]
func (m *lookupMap) get(hash common.Hash) (*common.Interaction, bool) {
	m.RLock()
	defer m.RUnlock()

	ix, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return ix, true
}

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

// add inserts the given Interaction into the map. [thread-safe]
func (m *lookupMap) add(ixs ...*common.Interaction) {
	m.Lock()
	defer m.Unlock()

	for _, ix := range ixs {
		m.all[ix.Hash()] = ix
	}
}

// remove deletes the given Interactions from the map. [thread-safe]
func (m *lookupMap) remove(ixs common.Interactions) {
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

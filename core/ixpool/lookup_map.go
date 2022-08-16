package ixpool

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"sync"
)

// Lookup map used to find transactions present in the pool
type lookupMap struct {
	sync.RWMutex
	all map[ktypes.Hash]*ktypes.Interaction
}

func NewLookupMap() *lookupMap {
	l := &lookupMap{
		all: make(map[ktypes.Hash]*ktypes.Interaction, 0),
	}

	return l
}

// add inserts the given Interaction into the map. [thread-safe]
func (m *lookupMap) add(txs ...*ktypes.Interaction) {
	m.Lock()
	defer m.Unlock()

	for _, ix := range txs {
		m.all[ix.GetIxHash()] = ix
	}
}

// remove deletes the given Interactions from the map. [thread-safe]
func (m *lookupMap) remove(ixs ktypes.Interactions) {
	m.Lock()
	defer m.Unlock()

	for _, ix := range ixs {
		delete(m.all, ix.GetIxHash())
	}
}

// get returns the Interaction associated with the given hash. [thread-safe]
func (m *lookupMap) get(hash ktypes.Hash) (*ktypes.Interaction, bool) {
	m.RLock()
	defer m.RUnlock()

	ix, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return ix, true
}

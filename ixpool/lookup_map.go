package ixpool

import (
	"sync"

	"github.com/sarvalabs/moichain/types"
)

// Lookup map used to find transactions present in the pool
type lookupMap struct {
	sync.RWMutex
	all map[types.Hash]*types.Interaction
}

func NewLookupMap() *lookupMap {
	l := &lookupMap{
		all: make(map[types.Hash]*types.Interaction, 0),
	}

	return l
}

// add inserts the given Interaction into the map. [thread-safe]
func (m *lookupMap) add(txs ...*types.Interaction) {
	m.Lock()
	defer m.Unlock()

	for _, ix := range txs {
		hash, err := ix.GetIxHash()
		if err != nil {
			continue
		}

		m.all[hash] = ix
	}
}

// remove deletes the given Interactions from the map. [thread-safe]
func (m *lookupMap) remove(ixs types.Interactions) {
	m.Lock()
	defer m.Unlock()

	for _, ix := range ixs {
		hash, err := ix.GetIxHash()
		if err != nil {
			continue
		}

		delete(m.all, hash)
	}
}

// get returns the Interaction associated with the given hash. [thread-safe]
func (m *lookupMap) get(hash types.Hash) (*types.Interaction, bool) {
	m.RLock()
	defer m.RUnlock()

	ix, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return ix, true
}

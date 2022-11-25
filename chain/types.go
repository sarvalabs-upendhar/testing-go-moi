package chain

import (
	"sync"

	"github.com/deckarep/golang-set"

	"github.com/sarvalabs/moichain/types"
)

type GridCache struct {
	mtx   sync.Mutex
	grids map[types.Hash]map[types.Hash]*types.Tesseract
}

func NewGridCache() *GridCache {
	return &GridCache{
		grids: make(map[types.Hash]map[types.Hash]*types.Tesseract),
	}
}

func (g *GridCache) AddTesseract(ts *types.Tesseract) bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	if ts.GridLength() == 1 {
		return true
	}

	grid, ok := g.grids[ts.Header.GridHash]
	if !ok {
		grid = map[types.Hash]*types.Tesseract{ts.Hash(): ts}
		g.grids[ts.Header.GridHash] = grid
	}

	grid[ts.Hash()] = ts

	return int32(len(grid)) == ts.GridLength()
}

func (g *GridCache) CleanupGrid(gridID types.Hash) []*types.Tesseract {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	grid, ok := g.grids[gridID]
	if !ok {
		return nil
	}

	tesseracts := make([]*types.Tesseract, 0, len(grid))

	for _, ts := range grid {
		tesseracts = append(tesseracts, ts)
	}

	return tesseracts
}

// KnownCache is a cache for known hashes.
type KnownCache struct {
	hashes mapset.Set
	max    int
}

// NewKnownCache creates a new knownCache with a max capacity.
func NewKnownCache(max int) *KnownCache {
	return &KnownCache{
		max:    max,
		hashes: mapset.NewSet(),
	}
}

// Add adds a list of elements to the set.
func (k *KnownCache) Add(data ...interface{}) {
	for k.hashes.Cardinality() > max(0, k.max-len(data)) {
		k.hashes.Pop()
	}

	for _, hash := range data {
		k.hashes.Add(hash)
	}
}

// Contains returns whether the given item is in the set.
func (k *KnownCache) Contains(data interface{}) bool {
	return k.hashes.Contains(data)
}

// Cardinality returns the number of elements in the set.
func (k *KnownCache) Cardinality() int {
	return k.hashes.Cardinality()
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

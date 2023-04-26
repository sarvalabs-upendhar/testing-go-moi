package lattice

import (
	"sync"

	"github.com/deckarep/golang-set"

	"github.com/sarvalabs/moichain/types"
)

type GroupCache struct {
	mtx   sync.Mutex
	group map[types.Hash]map[types.Hash]*types.Tesseract
}

func NewGridCache() *GroupCache {
	return &GroupCache{
		group: make(map[types.Hash]map[types.Hash]*types.Tesseract),
	}
}

func (g *GroupCache) AddTesseract(ts *types.Tesseract) (bool, error) {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	if ts.GridLength() == 1 {
		return true, nil
	}

	tsHash, err := ts.Hash()
	if err != nil {
		return false, err
	}

	if _, ok := g.group[ts.GroupHash()]; !ok {
		g.group[ts.GroupHash()] = make(map[types.Hash]*types.Tesseract)
	}

	g.group[ts.GroupHash()][tsHash] = ts

	return int32(len(g.group[ts.GroupHash()])) == ts.GridLength(), nil
}

func (g *GroupCache) CleanupGrid(gridID types.Hash) []*types.Tesseract {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	grid, ok := g.group[gridID]
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

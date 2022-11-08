package chain

import (
	"sync"

	"gitlab.com/sarvalabs/moichain/types"
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

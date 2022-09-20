package chain

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"sync"
)

type GridCache struct {
	mtx   sync.Mutex
	grids map[ktypes.Hash]map[ktypes.Hash]*ktypes.Tesseract
}

func NewGridCache() *GridCache {
	return &GridCache{
		grids: make(map[ktypes.Hash]map[ktypes.Hash]*ktypes.Tesseract),
	}
}

func (g *GridCache) AddTesseract(ts *ktypes.Tesseract) bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	if ts.GridLength() == 1 {
		return true
	}

	grid, ok := g.grids[ts.Header.GridHash]
	if !ok {
		grid = map[ktypes.Hash]*ktypes.Tesseract{ts.Hash(): ts}
		g.grids[ts.Header.GridHash] = grid
	}

	grid[ts.Hash()] = ts

	return int32(len(grid)) == ts.GridLength()
}

func (g *GridCache) CleanupGrid(gridID ktypes.Hash) []*ktypes.Tesseract {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	grid, ok := g.grids[gridID]
	if !ok {
		return nil
	}

	tesseracts := make([]*ktypes.Tesseract, 0, len(grid))

	for _, ts := range grid {
		tesseracts = append(tesseracts, ts)
	}

	return tesseracts
}

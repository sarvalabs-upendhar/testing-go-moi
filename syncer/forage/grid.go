package forage

import (
	"sync"

	"github.com/sarvalabs/moichain/syncer/cid"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/storage"
)

func dbKeyFromCID(address common.Address, cid cid.CID) []byte {
	return storage.DBKey(address, storage.Prefix(cid.ContentType()), cid.Key())
}

type Grid struct {
	mtx sync.RWMutex
	ts  map[common.Hash]*common.Tesseract
}

func NewGrid() *Grid {
	return &Grid{
		ts: make(map[common.Hash]*common.Tesseract),
	}
}

func (g *Grid) AddTesseract(ts *common.Tesseract) {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	g.ts[ts.Hash()] = ts
}

func (g *Grid) IsGridComplete(gridLength int32) bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	return int32(len(g.ts)) == gridLength
}

func (g *Grid) HasTesseract(ts *common.Tesseract) bool {
	g.mtx.RLock()
	defer g.mtx.RUnlock()

	_, ok := g.ts[ts.Hash()]

	return ok
}

func (g *Grid) PendingTesseractHashes() []common.Hash {
	g.mtx.RLock()
	defer g.mtx.RUnlock()

	pendingTesseracts := make([]common.Hash, 0)

	for _, ts := range g.ts {
		if info, ok := ts.Extra().GridID.Parts.Grid[ts.Address()]; !ok {
			pendingTesseracts = append(pendingTesseracts, info.Hash)
		}

		return pendingTesseracts
	}

	return pendingTesseracts
}

func (g *Grid) Tesseracts() []*common.Tesseract {
	g.mtx.RLock()
	defer g.mtx.RUnlock()

	ts := make([]*common.Tesseract, 0, len(g.ts))

	for _, t := range g.ts {
		ts = append(ts, t)
	}

	return ts
}

func (g *Grid) Size() int32 {
	return int32(len(g.ts))
}

func (g *Grid) Lock(write bool) {
	if write {
		g.mtx.Lock()

		return
	}

	g.mtx.RLock()
}

func (g *Grid) Unlock(write bool) {
	if write {
		g.mtx.Unlock()

		return
	}

	g.mtx.RUnlock()
}

type GridStore struct {
	mtx   sync.Mutex
	grids map[common.Hash]*Grid
}

func NewGridStore() *GridStore {
	return &GridStore{
		grids: make(map[common.Hash]*Grid),
	}
}

func (gs *GridStore) HasTesseract(ts *common.Tesseract) bool {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	grid, ok := gs.grids[ts.GridHash()]
	if !ok {
		return ok
	}

	return grid.HasTesseract(ts)
}

func (gs *GridStore) GetGrid(gridID common.Hash) *Grid {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	grid, ok := gs.grids[gridID]
	if !ok {
		return nil
	}

	return grid
}

func (gs *GridStore) NewGrid(gridID common.Hash) *Grid {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	gs.grids[gridID] = NewGrid()

	return gs.grids[gridID]
}

func (gs *GridStore) CleanupGrid(gridID common.Hash) {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	delete(gs.grids, gridID)
}

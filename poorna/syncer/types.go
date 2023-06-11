package syncer

import (
	"container/heap"
	"log"
	"sync"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/dhruva"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
)

type AccDetailsQueue struct {
	queue []*types.AccountMetaInfo
	lock  sync.RWMutex
}

func (a *AccDetailsQueue) Push(data []*types.AccountMetaInfo) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.queue = append(a.queue, data...)
}

func (a *AccDetailsQueue) Pop() (*types.AccountMetaInfo, error) {
	if len(a.queue) > 0 {
		a.lock.Lock()
		defer a.lock.Unlock()

		data := a.queue[0]
		a.queue = a.queue[1:]

		return data, nil
	}

	return nil, errors.New("Queue is empty")
}

func (a *AccDetailsQueue) Len() int {
	a.lock.Lock()
	defer a.lock.Unlock()

	return len(a.queue)
}

type TesseractResponse struct {
	Data  []byte
	Delta map[types.Hash][]byte
}

/*
func receiptsCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Receipt.Byte(), hash)
}
*/

func approvalsCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Approvals.Byte(), hash)
}

func accountCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Account.Byte(), hash)
}

func contextCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Context.Byte(), hash)
}

func storageCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Storage.Byte(), hash)
}

func logicCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Logic.Byte(), hash)
}

func balanceCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Balance.Byte(), hash)
}

func registryCID(hash types.Hash) atypes.CID {
	return atypes.ContentID(dhruva.Registry.Byte(), hash)
}

func dbKeyFromCID(address types.Address, cid atypes.CID) []byte {
	return dhruva.DBKey(address, dhruva.Prefix(cid.ContentType()), cid.Key())
}

type BucketSyncRequest struct {
	BucketID  uint64
	Timestamp uint64
}

type BucketSyncResponse struct {
	BucketID         uint64
	BucketCount      uint64
	AccountMetaInfos [][]byte
}

type SnapRequest struct {
	Address types.Address
	Height  uint64
}

type SnapMetaInfo struct {
	Hash          types.Hash
	CreatedAt     int64
	TotalSnapSize uint64
}

type SnapResponse struct {
	MetaInfo *SnapMetaInfo
	Data     []byte
}

type LatticeRequest struct {
	Address     types.Address
	StartHeight uint64
	EndHeight   uint64
}

type TesseractInfo struct {
	tesseract     *types.Tesseract
	shouldExecute bool
	clusterInfo   *types.ICSClusterInfo
	icsNodeSet    *types.ICSNodeSet
	delta         map[types.Hash][]byte
}

type TesseractQueue struct {
	mtx   sync.RWMutex
	set   map[uint64]struct{}
	Items sortedTesseractMsg
}

func NewTesseractQueue() *TesseractQueue {
	return &TesseractQueue{
		Items: make(sortedTesseractMsg, 0),
		set:   make(map[uint64]struct{}),
	}
}

func (s *TesseractQueue) Push(ts ...*TesseractInfo) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	log.Println("Adding tesseract to queue")

	for _, t := range ts {
		if _, ok := s.set[t.tesseract.Height()]; !ok {
			heap.Push(&s.Items, t)

			s.set[t.tesseract.Height()] = struct{}{}
		}
	}
}

func (s *TesseractQueue) Has(height uint64) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	_, ok := s.set[height]

	return ok
}

func (s *TesseractQueue) Pop() *TesseractInfo {
	if s.Len() == 0 {
		return nil
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	tsInfo, ok := heap.Pop(&s.Items).(*TesseractInfo)
	if !ok {
		panic("invalid element in tesseract queue")
	}

	delete(s.set, tsInfo.tesseract.Height())

	return tsInfo
}

func (s *TesseractQueue) Len() uint64 {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return uint64(len(s.Items))
}

func (s *TesseractQueue) Peek() *TesseractInfo {
	if s.Len() == 0 {
		return nil
	}

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.Items.Peek()
}

type sortedTesseractMsg []*TesseractInfo

func (q *sortedTesseractMsg) Peek() *TesseractInfo {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[0]
}

func (q *sortedTesseractMsg) Len() int {
	return len(*q)
}

func (q *sortedTesseractMsg) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *sortedTesseractMsg) Less(i, j int) bool {
	return (*q)[i].tesseract.Height() < (*q)[j].tesseract.Height()
}

func (q *sortedTesseractMsg) Push(x interface{}) {
	ix, ok := x.(*TesseractInfo)
	if !ok {
		return
	}

	*q = append(*q, ix)
}

func (q *sortedTesseractMsg) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]

	return x
}

type GridStore struct {
	mtx   sync.Mutex
	grids map[types.Hash]*Grid
}

type Grid struct {
	mtx sync.RWMutex
	ts  map[types.Hash]*types.Tesseract
}

func NewGrid() *Grid {
	return &Grid{
		ts: make(map[types.Hash]*types.Tesseract),
	}
}

func (g *Grid) AddTesseract(ts *types.Tesseract) {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	g.ts[ts.Hash()] = ts
}

func (g *Grid) IsGridComplete(gridLength int32) bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	return int32(len(g.ts)) == gridLength
}

func (g *Grid) HasTesseract(ts *types.Tesseract) bool {
	g.mtx.RLock()
	defer g.mtx.RUnlock()

	_, ok := g.ts[ts.Hash()]

	return ok
}

func (g *Grid) PendingTesseractHashes() []types.Hash {
	g.mtx.RLock()
	defer g.mtx.RUnlock()

	pendingTesseracts := make([]types.Hash, 0)

	for _, ts := range g.ts {
		if info, ok := ts.Extra().GridID.Parts.Grid[ts.Address()]; !ok {
			pendingTesseracts = append(pendingTesseracts, info.Hash)
		}

		return pendingTesseracts
	}

	return pendingTesseracts
}

func (g *Grid) Tesseracts() []*types.Tesseract {
	g.mtx.RLock()
	defer g.mtx.RUnlock()

	ts := make([]*types.Tesseract, 0, len(g.ts))

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

func NewGridStore() *GridStore {
	return &GridStore{
		grids: make(map[types.Hash]*Grid),
	}
}

func (gs *GridStore) HasTesseract(ts *types.Tesseract) bool {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	grid, ok := gs.grids[ts.GridHash()]
	if !ok {
		return ok
	}

	return grid.HasTesseract(ts)
}

func (gs *GridStore) GetGrid(gridID types.Hash) *Grid {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	grid, ok := gs.grids[gridID]
	if !ok {
		return nil
	}

	return grid
}

func (gs *GridStore) NewGrid(gridID types.Hash) *Grid {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	gs.grids[gridID] = NewGrid()

	return gs.grids[gridID]
}

func (gs *GridStore) CleanupGrid(gridID types.Hash) {
	gs.mtx.Lock()
	defer gs.mtx.Unlock()

	delete(gs.grids, gridID)
}

package forage

import (
	"container/heap"
	"sync"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractInfo struct {
	addr          identifiers.Address
	tesseract     *common.Tesseract
	shouldExecute bool
	clusterInfo   *common.ICSClusterInfo
	icsNodeSet    *common.ICSNodeSet
	ixnsHashes    common.Hashes
	delta         map[string][]byte
}

func (ti *TesseractInfo) CreateTSInfoWithAddr(addr identifiers.Address) *TesseractInfo {
	return &TesseractInfo{
		addr:          addr,
		tesseract:     ti.tesseract,
		shouldExecute: ti.shouldExecute,
		clusterInfo:   ti.clusterInfo,
		icsNodeSet:    ti.icsNodeSet,
		ixnsHashes:    ti.ixnsHashes,
		delta:         ti.delta,
	}
}

func (ti *TesseractInfo) address() identifiers.Address {
	return ti.addr
}

func (ti *TesseractInfo) height() uint64 {
	return ti.tesseract.Participants()[ti.addr].Height
}

func (ti *TesseractInfo) extractICSNodeset(s *Syncer) bool {
	var err error

	for _, contextHash := range ti.tesseract.PreviousContext() {
		if contextHash.IsNil() {
			continue
		}

		if _, ok := ti.delta[contextHash.String()]; !ok {
			ti.icsNodeSet, err = s.state.FetchICSNodeSet(ti.tesseract, ti.clusterInfo)
			if err != nil {
				s.logger.Error("Failed to fetch node set", "err", err)

				return false
			}
		} else {
			ti.icsNodeSet, err = s.state.GetICSNodeSetFromRawContext(ti.tesseract, ti.delta, ti.clusterInfo)
			if err != nil {
				s.logger.Error("Failed to fetch node set", "err", err)

				return false
			}
		}

		break
	}

	return true
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

	for _, t := range ts {
		if _, ok := s.set[t.height()]; !ok {
			heap.Push(&s.Items, t)

			s.set[t.height()] = struct{}{}
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

	delete(s.set, tsInfo.height())

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
	return (*q)[i].height() < (*q)[j].height()
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

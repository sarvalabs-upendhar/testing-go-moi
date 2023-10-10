package forage

import (
	"container/heap"
	"sync"

	"github.com/sarvalabs/go-moi/common"
)

type TesseractInfo struct {
	tesseract     *common.Tesseract
	shouldExecute bool
	clusterInfo   *common.ICSClusterInfo
	icsNodeSet    *common.ICSNodeSet
	delta         map[common.Hash][]byte
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

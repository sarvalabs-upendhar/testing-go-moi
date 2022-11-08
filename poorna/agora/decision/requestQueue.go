package decision

import (
	"errors"
	"sync"
	"time"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/types"
)

type Request struct {
	PeerID    id.KramaID
	SessionID types.Address
	StateHash types.Hash
	WantList  []types.Hash
	ReqTime   time.Time
}
type RequestQueue struct {
	mtx       sync.RWMutex
	elems     []*Request
	keys      map[id.KramaID]struct{}
	length    int
	maxLength int
}

func NewRequestQueue(maxSize int) *RequestQueue {
	return &RequestQueue{
		elems:     make([]*Request, 0, maxSize),
		keys:      make(map[id.KramaID]struct{}, maxSize),
		length:    0,
		maxLength: maxSize,
	}
}

func (q *RequestQueue) Push(element *Request) error {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if element == nil {
		return nil
	}

	if q.length >= q.maxLength {
		return errors.New("queue is full")
	}

	q.elems = append(q.elems, element)

	q.length++
	q.keys[element.PeerID] = struct{}{}

	return nil
}

func (q *RequestQueue) Pop() *Request {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if q.length > 0 {
		out := q.elems[0]
		q.elems = q.elems[1:]
		q.length--
		delete(q.keys, out.PeerID)

		return out
	}

	return nil
}

func (q *RequestQueue) Len() int {
	q.mtx.RLock()
	defer q.mtx.RUnlock()

	return q.length
}

func (q *RequestQueue) Contains(id id.KramaID) bool {
	q.mtx.RLock()
	defer q.mtx.RUnlock()

	_, ok := q.keys[id]

	return ok
}

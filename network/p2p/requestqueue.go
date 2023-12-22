package p2p

import (
	"math"
	"sync"

	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

type RequestQueue struct {
	mtx       sync.RWMutex
	elems     []*networkmsg.HelloMsg
	keys      map[id.KramaID]struct{}
	length    int
	maxLength int
}

func NewRequestQueue(maxSize int) *RequestQueue {
	return &RequestQueue{
		elems:     make([]*networkmsg.HelloMsg, 0, maxSize),
		keys:      make(map[id.KramaID]struct{}, maxSize),
		length:    0,
		maxLength: maxSize,
	}
}

func (q *RequestQueue) Push(element *networkmsg.HelloMsg) error {
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
	q.keys[element.KramaID] = struct{}{}

	return nil
}

func (q *RequestQueue) Pop(count int) []*networkmsg.HelloMsg {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if q.length > 0 {
		index := int(math.Min(float64(count), float64(q.length)))
		out := q.elems[:index]
		q.elems = q.elems[index:]

		q.length -= index

		for _, msg := range out {
			delete(q.keys, msg.KramaID)
		}

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

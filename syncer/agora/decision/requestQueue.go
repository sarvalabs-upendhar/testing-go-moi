package decision

import (
	"errors"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type Request struct {
	PeerID    identifiers.KramaID
	SessionID identifiers.Identifier
	StateHash cid.CID
	WantList  []cid.CID
	ReqTime   time.Time
}

func NewRequest(peerID identifiers.KramaID,
	sessionID identifiers.Identifier,
	stateHash cid.CID,
	wantList []cid.CID,
	reqTime time.Time,
) *Request {
	return &Request{
		PeerID:    peerID,
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  wantList,
		ReqTime:   reqTime,
	}
}

type RequestQueue struct {
	mtx       sync.RWMutex
	elems     []*Request
	keys      map[identifiers.KramaID]struct{}
	length    int
	maxLength int
}

func NewRequestQueue(maxSize int) *RequestQueue {
	return &RequestQueue{
		elems:     make([]*Request, 0, maxSize),
		keys:      make(map[identifiers.KramaID]struct{}, maxSize),
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

func (q *RequestQueue) Contains(id identifiers.KramaID) bool {
	q.mtx.RLock()
	defer q.mtx.RUnlock()

	_, ok := q.keys[id]

	return ok
}

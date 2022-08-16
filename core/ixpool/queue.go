package ixpool

import (
	"container/heap"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"sync"
	"sync/atomic"
)

type InteractionQueue interface {
	Pop() interface{}
	Peek() *ktypes.Interaction
	Len() int
}

type WaitInteractions struct {
	waitCounter int32
	ix          *ktypes.Interaction
}

// A thread-safe wrapper of a minNonceQueue.
// All methods assume the (correct) lock is held.
type accountQueue struct {
	sync.RWMutex
	wLock uint32
	queue minNonceQueue
}

func newAccountQueue() *accountQueue {
	q := accountQueue{
		queue: make(minNonceQueue, 0),
	}

	heap.Init(&q.queue)

	return &q
}

func (q *accountQueue) lock(write bool) {
	switch write {
	case true:
		q.Lock()
		atomic.StoreUint32(&q.wLock, 1)
	case false:
		q.RLock()
		atomic.StoreUint32(&q.wLock, 0)
	}
}

func (q *accountQueue) unlock() {
	if atomic.SwapUint32(&q.wLock, 0) == 1 {
		q.Unlock()
	} else {
		q.RUnlock()
	}
}

// prune removes all Interactions from the queue
// with nonce lower than given.
func (q *accountQueue) prune(nonce uint64) (pruned []*ktypes.Interaction) {
	for {
		tx := q.peek()
		if tx == nil ||
			tx.Nonce() >= nonce {
			break
		}

		tx = q.pop()
		pruned = append(pruned, tx)
	}

	return
}

// push pushes the given Interactions onto the queue.
func (q *accountQueue) push(tx *ktypes.Interaction) {
	heap.Push(&q.queue, tx)
}

// peek returns the first Interaction from the queue without removing it.
func (q *accountQueue) peek() *ktypes.Interaction {
	if q.length() == 0 {
		return nil
	}

	return q.queue.Peek()
}

// pop removes the first Interactions from the queue and returns it.
func (q *accountQueue) pop() *ktypes.Interaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*ktypes.Interaction) // nolint
}

// length returns the number of Interactions in the queue.
func (q *accountQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// Interactions sorted by nonce (ascending)
type minNonceQueue []*ktypes.Interaction

/* Queue methods required by the heap interface */

func (q *minNonceQueue) Peek() *ktypes.Interaction {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[0]
}

func (q *minNonceQueue) Len() int {
	return len(*q)
}

func (q *minNonceQueue) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *minNonceQueue) Less(i, j int) bool {
	return (*q)[i].Nonce() < (*q)[j].Nonce()
}

func (q *minNonceQueue) Push(x interface{}) {
	ix, ok := x.(*ktypes.Interaction)
	if !ok {
		return
	}

	*q = append(*q, ix)
}

func (q *minNonceQueue) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]

	return x
}

/*
type pricedQueue struct {
	queue maxPriceQueue
}

func newPricedQueue() *pricedQueue {
	q := pricedQueue{
		queue: make(maxPriceQueue, 0),
	}

	heap.Init(&q.queue)

	return &q
}

// clear empties the underlying queue
// and returns the removed Interactions.
func (q *pricedQueue) clear() {
	for {
		tx := q.pop()
		if tx == nil {
			break
		}
	}
}

// Pushes the given Interactions onto the queue.
func (q *pricedQueue) push(tx *ktypes.Interaction) {
	heap.Push(&q.queue, tx)
}

// Pop removes the first Interaction from the queue
// or nil if the queue is empty.
func (q *pricedQueue) pop() *ktypes.Interaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*ktypes.Interaction)
}

// length returns the number of Interactions in the queue.
func (q *pricedQueue) length() uint64 {
	return uint64(q.queue.Len())
}
*/
// Interactions sorted by gas price (descending)
type maxPriceQueue []*ktypes.Interaction

/* Queue methods required by the heap interface */

func (q *maxPriceQueue) Peek() *ktypes.Interaction {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[0]
}

func (q *maxPriceQueue) Len() int {
	return len(*q)
}

func (q *maxPriceQueue) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *maxPriceQueue) Less(i, j int) bool {
	return (*q)[i].GasPrice().Uint64() > (*q)[j].GasPrice().Uint64()
}

func (q *maxPriceQueue) Push(x interface{}) {
	*q = append(*q, x.(*ktypes.Interaction)) // nolint
}

func (q *maxPriceQueue) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]

	return x
}

// Interactions sorted by wait counter  (descending)
type maxWaitQueue []*WaitInteractions

// Queue methods required by the heap interface

func (wq *maxWaitQueue) Peek() *ktypes.Interaction {
	if wq.Len() == 0 {
		return nil
	}

	return (*wq)[0].ix
}

func (wq *maxWaitQueue) Len() int {
	return len(*wq)
}

func (wq *maxWaitQueue) Swap(i, j int) {
	(*wq)[i], (*wq)[j] = (*wq)[j], (*wq)[i]
}

func (wq *maxWaitQueue) Less(i, j int) bool {
	return (*wq)[i].waitCounter > (*wq)[j].waitCounter
}

func (wq *maxWaitQueue) Push(x interface{}) {
	ix, ok := x.(*WaitInteractions)
	if !ok {
		return
	}

	*wq = append(*wq, ix)
}

func (wq *maxWaitQueue) Pop() interface{} {
	old := wq
	n := len(*old)
	x := (*old)[n-1]
	*wq = (*old)[0 : n-1]

	return x.ix
}

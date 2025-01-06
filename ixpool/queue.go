package ixpool

import (
	"container/heap"

	"github.com/sarvalabs/go-moi/common"
)

type InteractionQueue interface {
	Pop() interface{}
	Peek() *common.Interaction
	Len() int
}

type WaitInteractions struct {
	waitCounter int32
	ix          *common.Interaction
}

// A thread-safe wrapper of a minSequenceIDQueue.
// All methods assume the (correct) lock is held.
type ixQueue struct {
	queue minSequenceIDQueue
}

func newAccountQueue() *ixQueue {
	q := ixQueue{
		queue: make(minSequenceIDQueue, 0),
	}

	heap.Init(&q.queue)

	return &q
}

// prune removes all Interactions from the queue
// with sequenceID lower than given.
func (q *ixQueue) prune(sequenceID uint64) (pruned []*common.Interaction) {
	for {
		ix := q.peek()
		if ix == nil ||
			ix.SequenceID() >= sequenceID {
			break
		}

		ix = q.pop()
		pruned = append(pruned, ix)
	}

	return
}

func (q *ixQueue) clear() (dropped []*common.Interaction) {
	// copy ixs
	dropped = q.queue

	// clear queue
	q.queue = q.queue[:0]

	return dropped
}

// push pushes the given Interactions onto the queue.
func (q *ixQueue) push(ix *common.Interaction) {
	heap.Push(&q.queue, ix)
}

// peek returns the first Interaction from the queue without removing it.
func (q *ixQueue) peek() *common.Interaction {
	return q.queue.Peek()
}

func (q *ixQueue) list() []*common.Interaction {
	return q.queue.List()
}

// pop removes the first Interactions from the queue and returns it.
func (q *ixQueue) pop() *common.Interaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*common.Interaction) //nolint
}

// length returns the number of Interactions in the queue.
func (q *ixQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// Interactions sorted by sequenceID (ascending)
type minSequenceIDQueue []*common.Interaction

/* Queue methods required by the heap interface */

func (q *minSequenceIDQueue) List() []*common.Interaction {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[:]
}

func (q *minSequenceIDQueue) Peek() *common.Interaction {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[0]
}

func (q *minSequenceIDQueue) Len() int {
	return len(*q)
}

func (q *minSequenceIDQueue) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *minSequenceIDQueue) Less(i, j int) bool {
	return (*q)[i].SequenceID() < (*q)[j].SequenceID()
}

func (q *minSequenceIDQueue) Push(x interface{}) {
	ix, ok := x.(*common.Interaction)
	if !ok {
		return
	}

	*q = append(*q, ix)
}

func (q *minSequenceIDQueue) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]

	return x
}

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

// Push the given Interactions onto the queue.
func (q *pricedQueue) Push(ix *common.Interaction) {
	heap.Push(&q.queue, ix)
}

// Pop removes the first Interaction from the queue
// or return nil if the queue is empty.
func (q *pricedQueue) Pop() interface{} {
	if q.Len() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*common.Interaction) //nolint
}

// Peek returns the first Interaction from the queue
// or nil if the queue is empty.
func (q *pricedQueue) Peek() *common.Interaction {
	return q.queue.Peek()
}

// Len returns the number of Interactions in the queue.
func (q *pricedQueue) Len() int {
	return q.queue.Len()
}

// Interactions sorted by gas price (descending)
type maxPriceQueue []*common.Interaction

/* Queue methods required by the heap interface */

func (q *maxPriceQueue) Peek() *common.Interaction {
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
	return (*q)[i].FuelPrice().Uint64() > (*q)[j].FuelPrice().Uint64()
}

func (q *maxPriceQueue) Push(x interface{}) {
	*q = append(*q, x.(*common.Interaction)) //nolint
}

func (q *maxPriceQueue) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]

	return x
}

type waitQueue struct {
	queue maxWaitQueue
}

func newWaitQueue() *waitQueue {
	q := waitQueue{
		queue: make(maxWaitQueue, 0),
	}

	heap.Init(&q.queue)

	return &q
}

// Push the given Interactions onto the queue.
func (q *waitQueue) Push(ix interface{}) {
	heap.Push(&q.queue, ix)
}

// Pop removes the first Interaction from the queue
// or return nil if the queue is empty.
func (q *waitQueue) Pop() interface{} {
	if q.Len() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*common.Interaction) //nolint
}

// Peek returns the first Interaction from the queue
// or nil if the queue is empty.
func (q *waitQueue) Peek() *common.Interaction {
	return q.queue.Peek()
}

// Len returns the number of Interactions in the queue.
func (q *waitQueue) Len() int {
	return q.queue.Len()
}

// Interactions sorted by wait counter  (descending)
type maxWaitQueue []*WaitInteractions

// Queue methods required by the heap interface

func (wq *maxWaitQueue) Peek() *common.Interaction {
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

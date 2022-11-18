package types

import "container/heap"

// PQ is a basic priority queue.
type PQ interface {
	// Push adds the ele
	Push(Element)
	// Pop removes and returns the highest priority Element in PQ.
	Pop() Element
	// Peek returns the highest priority Element in PQ (without removing it).
	Peek() Element
	// Remove removes the item at the given index from the PQ.
	Remove(index int) Element
	// Len returns the number of elements in the PQ.
	Len() int
	// Update `fixes` the PQ.
	Update(index int)
}

// Element describes elements that can be added to the PQ. Clients must implement
// this interface.
type Element interface {
	// SetIndex stores the int index.
	SetIndex(int)
	// Index returns the last given by SetIndex(int).
	Index() int
}

// ElementComparator returns true if pri(a) > pri(b)
type ElementComparator func(a, b Element) bool

func NewPriorityQueue(cmp ElementComparator) PQ {
	q := &PriorityQueue{
		data: &heapWrapper{
			elems: make([]Element, 0),
			cmp:   cmp,
		},
	}

	heap.Init(q.data)

	return q
}

type heapWrapper struct {
	elems []Element
	cmp   ElementComparator
}

func (q *heapWrapper) Len() int {
	return len(q.elems)
}

// Less delegates the decision to the comparator
func (q *heapWrapper) Less(i, j int) bool {
	return q.cmp(q.elems[i], q.elems[j])
}

// Swap swaps the elements with indexes i and j.
func (q *heapWrapper) Swap(i, j int) {
	q.elems[i], q.elems[j] = q.elems[j], q.elems[i]
	q.elems[i].SetIndex(i)
	q.elems[j].SetIndex(j)
}

// Note that Push and Pop in this interface are for package heap's
// implementation to call. To add and remove things from the heap, wrap with
// the pq struct to call heap.Push and heap.Pop.

func (q *heapWrapper) Push(x interface{}) { // where to put the elem?
	if t, ok := x.(Element); ok {
		t.SetIndex(len(q.elems))
		q.elems = append(q.elems, t)
	}
}

func (q *heapWrapper) Pop() interface{} {
	old := q.elems
	n := len(old)
	elem := old[n-1] // remove the last

	elem.SetIndex(-1) // for safety

	q.elems = old[0 : n-1] // shrink

	return elem
}

type PriorityQueue struct {
	data *heapWrapper
}

func (pq *PriorityQueue) Push(e Element) {
	heap.Push(pq.data, e)
}

func (pq *PriorityQueue) Pop() Element {
	if element, ok := heap.Pop(pq.data).(Element); ok {
		return element
	}

	return nil
}

func (pq *PriorityQueue) Peek() Element {
	if len(pq.data.elems) == 0 {
		return nil
	}

	return pq.data.elems[0]
}

func (pq *PriorityQueue) Remove(index int) Element {
	if element, ok := heap.Remove(pq.data, index).(Element); ok {
		return element
	}

	return nil
}

func (pq *PriorityQueue) Update(index int) {
	heap.Fix(pq.data, index)
}

func (pq *PriorityQueue) Len() int {
	return pq.data.Len()
}

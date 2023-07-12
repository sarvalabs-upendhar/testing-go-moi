package ixpool

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
)

func TestPricedQueue_Push(t *testing.T) {
	pricedQueue := newPricedQueue()

	testcases := []struct {
		name     string
		ixs      common.Interactions
		expected []uint64
	}{
		{
			name: "Push ixs into the priced queue",
			ixs: common.Interactions{
				newIxWithFuelPrice(t, 0, common.NilAddress, 8),
				newIxWithFuelPrice(t, 0, common.NilAddress, 2),
			},
			expected: []uint64{8, 2},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			for _, ix := range testcase.ixs {
				pricedQueue.Push(ix)
			}

			require.Len(t, pricedQueue.queue, len(testcase.ixs))

			for _, expectedPrice := range testcase.expected {
				ix, ok := pricedQueue.Pop().(*common.Interaction)
				require.True(t, ok)
				require.Equal(t, expectedPrice, ix.FuelPrice().Uint64())
			}
		})
	}
}

func TestPricedQueue_Pop(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      common.Interactions
		testFn   func(pq *pricedQueue, ixs common.Interactions)
		expected []uint64
	}{
		{
			name: "Empty priced queue",
			ixs:  common.Interactions{},
		},
		{
			name: "Pop the ixs by highest cost in order",
			ixs: common.Interactions{
				newIxWithFuelPrice(t, 0, common.NilAddress, 8),
				newIxWithFuelPrice(t, 0, common.NilAddress, 2),
				newIxWithFuelPrice(t, 0, common.NilAddress, 4),
				newIxWithFuelPrice(t, 0, common.NilAddress, 12),
				newIxWithFuelPrice(t, 0, common.NilAddress, 8),
			},
			testFn: func(pq *pricedQueue, ixs common.Interactions) {
				for _, ix := range ixs {
					pq.Push(ix)
				}
			},
			expected: []uint64{12, 8, 8, 4, 2},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			pricedQueue := newPricedQueue()

			if testcase.testFn != nil {
				testcase.testFn(pricedQueue, testcase.ixs)
			}

			if len(testcase.ixs) == 0 {
				ix := pricedQueue.Pop()
				require.Nil(t, ix)

				return
			}

			for _, expectedPrice := range testcase.expected {
				ix, ok := pricedQueue.Pop().(*common.Interaction)
				require.True(t, ok)
				require.Equal(t, expectedPrice, ix.FuelPrice().Uint64())
			}

			require.Len(t, pricedQueue.queue, 0)
		})
	}
}

func TestPricedQueue_Peek(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      common.Interactions
		testFn   func(pq *pricedQueue, ixs common.Interactions)
		expected []uint64
	}{
		{
			name: "Empty priced queue",
			ixs:  common.Interactions{},
		},
		{
			name: "Peek the ixs by highest cost in order",
			ixs: common.Interactions{
				newIxWithFuelPrice(t, 0, common.NilAddress, 8),
				newIxWithFuelPrice(t, 0, common.NilAddress, 2),
				newIxWithFuelPrice(t, 0, common.NilAddress, 4),
				newIxWithFuelPrice(t, 0, common.NilAddress, 12),
				newIxWithFuelPrice(t, 0, common.NilAddress, 8),
			},
			testFn: func(pq *pricedQueue, ixs common.Interactions) {
				for _, ix := range ixs {
					pq.Push(ix)
				}
			},
			expected: []uint64{12, 8, 8, 4, 2},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			pricedQueue := newPricedQueue()

			if testcase.testFn != nil {
				testcase.testFn(pricedQueue, testcase.ixs)
			}

			if len(testcase.ixs) == 0 {
				ix := pricedQueue.Peek()
				require.Nil(t, ix)

				return
			}

			for _, expectedPrice := range testcase.expected {
				require.Equal(t, expectedPrice, pricedQueue.Peek().FuelPrice().Uint64())
				// Remove the first Interaction from the queue
				pricedQueue.Pop()
			}
		})
	}
}

func TestPricedQueue_Len(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      common.Interactions
		testFn   func(pq *pricedQueue, ixs common.Interactions)
		expected int
	}{
		{
			name:     "Empty priced queue",
			ixs:      common.Interactions{},
			expected: 0,
		},
		{
			name: "should return the expected length",
			ixs: common.Interactions{
				newIxWithFuelPrice(t, 0, common.NilAddress, 8),
				newIxWithFuelPrice(t, 0, common.NilAddress, 2),
			},
			testFn: func(pq *pricedQueue, ixs common.Interactions) {
				for _, ix := range ixs {
					pq.Push(ix)
				}
			},
			expected: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			pricedQueue := newPricedQueue()

			if testcase.testFn != nil {
				testcase.testFn(pricedQueue, testcase.ixs)
			}

			require.Equal(t, testcase.expected, pricedQueue.Len())
		})
	}
}

func TestWaitQueue_Push(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      []*WaitInteractions
		expected []uint64
	}{
		{
			name: "Push ixs into the wait queue",
			ixs: []*WaitInteractions{
				newIxWithWaitCounter(t, 2, common.NilAddress, 2),
				newIxWithWaitCounter(t, 1, common.NilAddress, 1),
				newIxWithWaitCounter(t, 4, common.NilAddress, 4),
			},
			expected: []uint64{4, 2, 1},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			waitQueue := newWaitQueue()

			for _, ix := range testcase.ixs {
				waitQueue.Push(ix)
			}

			require.Len(t, waitQueue.queue, len(testcase.ixs))

			for _, expectedNonce := range testcase.expected {
				ix, ok := waitQueue.Pop().(*common.Interaction)
				require.True(t, ok)
				require.Equal(t, expectedNonce, ix.Nonce())
			}
		})
	}
}

func TestWaitQueue_Pop(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      []*WaitInteractions
		testFn   func(wq *waitQueue, ixs []*WaitInteractions)
		expected []uint64
	}{
		{
			name: "Empty wait queue",
			ixs:  []*WaitInteractions{},
		},
		{
			name: "Pop the ixs by highest wait time in order",
			ixs: []*WaitInteractions{
				newIxWithWaitCounter(t, 2, common.NilAddress, 2),
				newIxWithWaitCounter(t, 5, common.NilAddress, 5),
				newIxWithWaitCounter(t, 1, common.NilAddress, 1),
				newIxWithWaitCounter(t, 8, common.NilAddress, 8),
				newIxWithWaitCounter(t, 5, common.NilAddress, 5),
			},
			testFn: func(wq *waitQueue, ixs []*WaitInteractions) {
				for _, ix := range ixs {
					wq.Push(ix)
				}
			},
			expected: []uint64{8, 5, 5, 2, 1},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			waitQueue := newWaitQueue()

			if testcase.testFn != nil {
				testcase.testFn(waitQueue, testcase.ixs)
			}

			if len(testcase.ixs) == 0 {
				ix := waitQueue.Pop()
				require.Nil(t, ix)

				return
			}

			for _, expectedNonce := range testcase.expected {
				ix, ok := waitQueue.Pop().(*common.Interaction)
				require.True(t, ok)
				require.Equal(t, expectedNonce, ix.Nonce())
			}

			require.Len(t, waitQueue.queue, 0)
		})
	}
}

func TestWaitQueue_Peek(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      []*WaitInteractions
		testFn   func(wq *waitQueue, ixs []*WaitInteractions)
		expected []uint64
	}{
		{
			name: "Empty wait queue",
			ixs:  []*WaitInteractions{},
		},
		{
			name: "Peek the ixs by highest wait time in order",
			ixs: []*WaitInteractions{
				newIxWithWaitCounter(t, 4, common.NilAddress, 4),
				newIxWithWaitCounter(t, 2, common.NilAddress, 2),
				newIxWithWaitCounter(t, 4, common.NilAddress, 4),
				newIxWithWaitCounter(t, 10, common.NilAddress, 10),
				newIxWithWaitCounter(t, 4, common.NilAddress, 4),
			},
			testFn: func(wq *waitQueue, ixs []*WaitInteractions) {
				for _, ix := range ixs {
					wq.Push(ix)
				}
			},
			expected: []uint64{10, 4, 4, 4, 2},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			waitQueue := newWaitQueue()

			if testcase.testFn != nil {
				testcase.testFn(waitQueue, testcase.ixs)
			}

			if len(testcase.ixs) == 0 {
				ix := waitQueue.Peek()
				require.Nil(t, ix)

				return
			}

			for _, expectedNonce := range testcase.expected {
				require.Equal(t, expectedNonce, waitQueue.Peek().Nonce())
				// Remove the first Interaction from the queue
				waitQueue.Pop()
			}
		})
	}
}

func TestWaitQueue_Len(t *testing.T) {
	testcases := []struct {
		name     string
		ixs      []*WaitInteractions
		testFn   func(wq *waitQueue, ixs []*WaitInteractions)
		expected int
	}{
		{
			name:     "Empty wait queue",
			ixs:      []*WaitInteractions{},
			expected: 0,
		},
		{
			name: "should return the expected length",
			ixs: []*WaitInteractions{
				newIxWithWaitCounter(t, 0, common.NilAddress, 8),
				newIxWithWaitCounter(t, 0, common.NilAddress, 2),
			},
			testFn: func(wq *waitQueue, ixs []*WaitInteractions) {
				for _, ix := range ixs {
					wq.Push(ix)
				}
			},
			expected: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			waitQueue := newWaitQueue()

			if testcase.testFn != nil {
				testcase.testFn(waitQueue, testcase.ixs)
			}

			require.Equal(t, testcase.expected, waitQueue.Len())
		})
	}
}

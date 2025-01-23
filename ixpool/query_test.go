package ixpool

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
)

func TestIxPool_GetNonce(t *testing.T) {
	sm := NewMockStateManager(t)
	addr1 := tests.RandomIdentifier(t)
	addr2 := tests.RandomIdentifier(t)

	sm.setTestMOIBalance(t, addr1, addr2)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(1)
	}, true, sm, nil, nil)

	testcases := []struct {
		name          string
		id            identifiers.Identifier
		testFn        func(id identifiers.Identifier)
		expectedNonce uint64
	}{
		{
			name: "IxPool accounts without interaction sender state",
			id:   tests.RandomIdentifier(t),
			testFn: func(id identifiers.Identifier) {
				sm.setLatestSequenceID(t, id, 0, 4)
			},
			expectedNonce: 4,
		},
		{
			name: "IxPool accounts with interaction sender state",
			id:   tests.RandomIdentifier(t),
			testFn: func(id identifiers.Identifier) {
				ixPool.getOrCreateAccountQueue(id, 0, 5)
			},
			expectedNonce: 5,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.id)
			}

			// Should return the sequenceID either from ixpool account if it exists or from the latest state object
			nonce, err := ixPool.GetSequenceID(testcase.id, 0)
			require.NoError(t, err)
			require.Equal(t, testcase.expectedNonce, nonce)
		})
	}
}

type expectedIxQueue struct {
	pending int
	queued  int
}

func TestIxPool_GetIxs(t *testing.T) {
	id := tests.RandomIdentifier(t)

	testcases := []struct {
		name            string
		id              identifiers.Identifier
		ixs             []*common.Interaction
		inclQueued      bool
		expectedIxQueue expectedIxQueue
	}{
		{
			name: "Without queued interactions",
			id:   id,
			ixs: append(
				// promoted
				createTestIxs(t, 6, 8, id),
				// enqueued
				createTestIxs(t, 10, 13, id)...,
			),
			inclQueued: false,
			expectedIxQueue: expectedIxQueue{
				pending: 2,
				queued:  0,
			},
		},
		{
			name: "With queued interactions",
			id:   id,
			ixs: append(
				// promoted
				createTestIxs(t, 6, 8, id),
				// enqueued
				createTestIxs(t, 10, 13, id)...,
			),
			inclQueued: true,
			expectedIxQueue: expectedIxQueue{
				pending: 2,
				queued:  3,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			sm.setAccountKeysAndPublicKeys(t, []identifiers.Identifier{id}, tests.GetTestPublicKeys(t, 1))

			addAndProcessIxs(t, sm, ixPool, testcase.ixs...)

			pendingIxs, queuedIxs := ixPool.GetIxs(testcase.id, testcase.inclQueued)

			require.Equal(t, testcase.expectedIxQueue.pending, len(pendingIxs))
			require.Equal(t, testcase.expectedIxQueue.queued, len(queuedIxs))
		})
	}
}

func TestIxPool_GetAllIxs(t *testing.T) {
	ids := tests.GetRandomIDs(t, 2)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Identifier][]*common.Interaction
		inclQueued      bool
		expectedIxQueue map[identifiers.Identifier]expectedIxQueue
	}{
		{
			name: "Without queued interactions",
			accounts: map[identifiers.Identifier][]*common.Interaction{
				ids[0]: append(
					// promoted
					createTestIxs(t, 1, 3, ids[0]),
					// enqueued
					createTestIxs(t, 7, 10, ids[0])...,
				),
				ids[1]: append(
					// promoted
					createTestIxs(t, 6, 8, ids[1]),
					// enqueued
					createTestIxs(t, 10, 13, ids[1])...,
				),
			},
			inclQueued: false,
			expectedIxQueue: map[identifiers.Identifier]expectedIxQueue{
				ids[0]: {
					pending: 2,
					queued:  0,
				},
				ids[1]: {
					pending: 2,
					queued:  0,
				},
			},
		},
		{
			name: "With queued interactions",
			accounts: map[identifiers.Identifier][]*common.Interaction{
				ids[0]: append(
					// promoted
					createTestIxs(t, 1, 3, ids[0]),
					// enqueued
					createTestIxs(t, 7, 10, ids[0])...,
				),
				ids[1]: append(
					// promoted
					createTestIxs(t, 6, 8, ids[1]),
					// enqueued
					createTestIxs(t, 10, 13, ids[1])...,
				),
			},
			inclQueued: true,
			expectedIxQueue: map[identifiers.Identifier]expectedIxQueue{
				ids[0]: {
					pending: 2,
					queued:  3,
				},
				ids[1]: {
					pending: 2,
					queued:  3,
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, len(ids)))

			for _, ixs := range testcase.accounts {
				addAndProcessIxs(t, sm, ixPool, ixs...)
			}

			pendingIxs, queuedIxs := ixPool.GetAllIxs(testcase.inclQueued)

			for id, ixQueue := range testcase.expectedIxQueue {
				require.Equal(t, ixQueue.pending, len(pendingIxs[id]))
				require.Equal(t, ixQueue.queued, len(queuedIxs[id]))
			}
		})
	}
}

func TestIxPool_GetAccountWaitTime(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(1)
	}, true, sm, nil, nil)

	testcases := []struct {
		name        string
		id          identifiers.Identifier
		testFn      func(id identifiers.Identifier, waitTime time.Duration)
		expectedErr error
	}{
		{
			name:        "Account without state",
			id:          tests.RandomIdentifier(t),
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "Account with state",
			id:   tests.RandomIdentifier(t),
			testFn: func(id identifiers.Identifier, baseTime time.Duration) {
				ixPool.getOrCreateAccountQueue(id, 0, 0)
				err := ixPool.IncrementWaitTime(id, baseTime)
				require.NoError(t, err)
			},
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			baseTime := 2000 * time.Millisecond

			if testcase.testFn != nil {
				testcase.testFn(testcase.id, baseTime)
			}

			waitTime, err := ixPool.GetAccountWaitTime(testcase.id)

			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.InDelta(t,
				utils.ExponentialTimeout(baseTime, 1).Milliseconds(),
				waitTime.Int64(),
				float64(baseTime.Milliseconds()*2),
			)
		})
	}
}

func TestIxPool_GetAllAccountsWaitTime(t *testing.T) {
	addressList := tests.GetRandomIDs(t, 4)
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(1)
	}, true, sm, nil, nil)

	testcases := []struct {
		name     string
		accounts map[identifiers.Identifier]int
	}{
		{
			name: "Accounts with different delay count",
			accounts: map[identifiers.Identifier]int{
				addressList[0]: 1,
				addressList[1]: 5,
				addressList[2]: 4,
				addressList[3]: MaxWaitCounter + 1,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			baseTime := 1500 * time.Millisecond

			for id, delta := range testcase.accounts {
				ixPool.getOrCreateAccountQueue(id, 0, 0)

				for i := 0; i < delta; i++ {
					require.NoError(t, ixPool.IncrementWaitTime(id, baseTime))
				}
			}

			accountWaitTime := ixPool.GetAllAccountsWaitTime()

			for id, delta := range testcase.accounts {
				acc := ixPool.accounts.getAccount(id)
				waitTime := accountWaitTime[id]

				require.NotNil(t, waitTime)

				if delta > MaxWaitCounter {
					require.InDelta(t,
						utils.ExponentialTimeout(baseTime, acc.delayCounter).Milliseconds(),
						waitTime.Int64(),
						float64(0),
					)

					continue
				}

				require.InDelta(t,
					utils.ExponentialTimeout(baseTime, acc.delayCounter).Milliseconds(),
					waitTime.Int64(),
					float64(baseTime.Milliseconds()*int64(math.Pow(2, float64(delta)))),
				)
			}
		})
	}
}

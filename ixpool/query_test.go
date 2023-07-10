package ixpool

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

func TestIxPool_GetNonce(t *testing.T) {
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)

	sm.setTestMOIBalance(addr1, addr2)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(1)
	}, true, sm)

	testcases := []struct {
		name          string
		address       types.Address
		testFn        func(addr types.Address)
		expectedNonce uint64
	}{
		{
			name:    "IxPool accounts without interaction sender state",
			address: tests.RandomAddress(t),
			testFn: func(addr types.Address) {
				sm.setLatestNonce(addr, 4)
			},
			expectedNonce: 4,
		},
		{
			name:    "IxPool accounts with interaction sender state",
			address: tests.RandomAddress(t),
			testFn: func(addr types.Address) {
				ixPool.accounts.initOnce(addr, 5)
			},
			expectedNonce: 5,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.address)
			}

			// Should return the nonce either from ixpool account if it exists or from the latest state object
			nonce, err := ixPool.GetNonce(testcase.address)
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
	address := tests.RandomAddress(t)

	testcases := []struct {
		name            string
		address         types.Address
		ixs             types.Interactions
		inclQueued      bool
		expectedIxQueue expectedIxQueue
	}{
		{
			name:    "Without queued interactions",
			address: address,
			ixs: append(
				// promoted
				createTestIxs(t, types.IxValueTransfer, 6, 8, address),
				// enqueued
				createTestIxs(t, types.IxValueTransfer, 10, 13, address)...,
			),
			inclQueued: false,
			expectedIxQueue: expectedIxQueue{
				pending: 2,
				queued:  0,
			},
		},
		{
			name:    "With queued interactions",
			address: address,
			ixs: append(
				// promoted
				createTestIxs(t, types.IxValueTransfer, 6, 8, address),
				// enqueued
				createTestIxs(t, types.IxValueTransfer, 10, 13, address)...,
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
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			addAndProcessIxs(t, sm, ixPool, testcase.ixs)

			pendingIxs, queuedIxs := ixPool.GetIxs(testcase.address, testcase.inclQueued)

			require.Equal(t, testcase.expectedIxQueue.pending, len(pendingIxs))
			require.Equal(t, testcase.expectedIxQueue.queued, len(queuedIxs))
		})
	}
}

func TestIxPool_GetAllIxs(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 2)

	testcases := []struct {
		name            string
		accounts        map[types.Address]types.Interactions
		inclQueued      bool
		expectedIxQueue map[types.Address]expectedIxQueue
	}{
		{
			name: "Without queued interactions",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: append(
					// promoted
					createTestIxs(t, types.IxValueTransfer, 1, 3, addresses[0]),
					// enqueued
					createTestIxs(t, types.IxValueTransfer, 7, 10, addresses[0])...,
				),
				addresses[1]: append(
					// promoted
					createTestIxs(t, types.IxValueTransfer, 6, 8, addresses[1]),
					// enqueued
					createTestIxs(t, types.IxValueTransfer, 10, 13, addresses[1])...,
				),
			},
			inclQueued: false,
			expectedIxQueue: map[types.Address]expectedIxQueue{
				addresses[0]: {
					pending: 2,
					queued:  0,
				},
				addresses[1]: {
					pending: 2,
					queued:  0,
				},
			},
		},
		{
			name: "With queued interactions",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: append(
					// promoted
					createTestIxs(t, types.IxValueTransfer, 1, 3, addresses[0]),
					// enqueued
					createTestIxs(t, types.IxValueTransfer, 7, 10, addresses[0])...,
				),
				addresses[1]: append(
					// promoted
					createTestIxs(t, types.IxValueTransfer, 6, 8, addresses[1]),
					// enqueued
					createTestIxs(t, types.IxValueTransfer, 10, 13, addresses[1])...,
				),
			},
			inclQueued: true,
			expectedIxQueue: map[types.Address]expectedIxQueue{
				addresses[0]: {
					pending: 2,
					queued:  3,
				},
				addresses[1]: {
					pending: 2,
					queued:  3,
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			for _, ixs := range testcase.accounts {
				addAndProcessIxs(t, sm, ixPool, ixs)
			}

			pendingIxs, queuedIxs := ixPool.GetAllIxs(testcase.inclQueued)

			for addr, ixQueue := range testcase.expectedIxQueue {
				require.Equal(t, ixQueue.pending, len(pendingIxs[addr]))
				require.Equal(t, ixQueue.queued, len(queuedIxs[addr]))
			}
		})
	}
}

func TestIxPool_GetAccountWaitTime(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(1)
	}, true, sm)

	testcases := []struct {
		name        string
		address     types.Address
		testFn      func(addr types.Address, waitTime time.Duration)
		expectedErr error
	}{
		{
			name:        "Account without state",
			address:     tests.RandomAddress(t),
			expectedErr: types.ErrAccountNotFound,
		},
		{
			name:    "Account with state",
			address: tests.RandomAddress(t),
			testFn: func(addr types.Address, baseTime time.Duration) {
				ixPool.createAccountOnce(addr, 0)
				err := ixPool.IncrementWaitTime(addr, baseTime)
				require.NoError(t, err)
			},
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			baseTime := 2000 * time.Millisecond

			if testcase.testFn != nil {
				testcase.testFn(testcase.address, baseTime)
			}

			waitTime, err := ixPool.GetAccountWaitTime(testcase.address)

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
	addressList := tests.GetRandomAddressList(t, 4)
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(1)
	}, true, sm)

	testcases := []struct {
		name     string
		accounts map[types.Address]int
	}{
		{
			name: "Accounts with different delay count",
			accounts: map[types.Address]int{
				addressList[0]: 1,
				addressList[1]: 5,
				addressList[2]: 10,
				addressList[3]: MaxWaitCounter + 1,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			baseTime := 1500 * time.Millisecond

			for addr, delta := range testcase.accounts {
				ixPool.createAccountOnce(addr, 0)

				for i := 0; i < delta; i++ {
					require.NoError(t, ixPool.IncrementWaitTime(addr, baseTime))
				}
			}

			accountWaitTime := ixPool.GetAllAccountsWaitTime()

			for addr, delta := range testcase.accounts {
				acc := ixPool.accounts.get(addr)
				waitTime := accountWaitTime[addr]

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

package ixpool

import (
	"math"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

func TestIxPool_AddInteractions_checkIx(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name        string
		ix          *types.Interaction
		preTestFn   func(interaction *types.Interaction)
		expectedErr error
	}{
		{
			name:        "Interaction with invalid address",
			ix:          newIxWithoutAddress(t, 0),
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Already known Interaction",
			ix:   newTestInteraction(t, 0, types.NilAddress, nil),
			preTestFn: func(interaction *types.Interaction) {
				ixPool.allIxs.add(interaction)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name:        "New valid interaction",
			ix:          newTestInteraction(t, 0, types.NilAddress, nil),
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.preTestFn != nil {
				testcase.preTestFn(testcase.ix)
			}

			err := ixPool.checkIx(testcase.ix)
			// check's whether the invalid or known interactions are discarded
			require.Equal(t, testcase.expectedErr, err)
		})
	}
}

func TestIxPool_AddInteractions(t *testing.T) {
	testcases := []struct {
		name         string
		ixs          types.Interactions
		expectedIxs  int
		expectedErrs int
	}{
		{
			name: "All the interactions are invalid",
			ixs: types.Interactions{
				newIxWithoutAddress(t, 0),
				newIxWithFuelPrice(t, 1, types.NilAddress, 10),
			},
			expectedIxs:  0,
			expectedErrs: 2,
		},
		{
			name: "Some interactions are valid",
			ixs: types.Interactions{
				newIxWithoutAddress(t, 0),
				newTestInteraction(t, 1, types.NilAddress, nil),
				newTestInteraction(t, 2, types.NilAddress, nil),
			},
			expectedIxs:  2,
			expectedErrs: 1,
		},
		{
			name:         "All the interactions are valid",
			ixs:          createTestIxs(t, 0, 2, types.NilAddress),
			expectedIxs:  2,
			expectedErrs: 0,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			var (
				enqueuedIxs types.Interactions
				newIxsEvent utils.NewIxsEvent
				wg          sync.WaitGroup
				errs        []error
			)

			ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(100)
			})

			wg.Add(1)

			go func() {
				defer wg.Done()
				errs = ixPool.AddInteractions(testcase.ixs)
			}()

			if testcase.expectedIxs > 0 {
				enqueuedIxs, newIxsEvent = waitForNewIxs(t, ixPool)
			}

			wg.Wait()

			// checks whether the invalid interactions are filtered
			require.Equal(t, testcase.expectedErrs, len(errs))

			// checks whether the valid interactions are sent over the enqueue request channel
			require.Equal(t, testcase.expectedIxs, len(enqueuedIxs))

			// checks whether the new interactions are sent as an event over the subscribed channel
			require.Equal(t, testcase.expectedIxs, len(newIxsEvent.Ixs))
		})
	}
}

type expectedResult struct {
	nonce            uint64
	enqueued         uint64
	promoted         uint64
	promotedAccounts int
}

func TestIxPool_handleEnqueueRequest(t *testing.T) {
	address := tests.RandomAddress(t)

	testcases := []struct {
		name     string
		ixs      types.Interactions
		testFn   func(ixPool *IxPool, interactions types.Interactions)
		expected expectedResult
	}{
		{
			name: "Enqueue ixs with higher nonce",
			ixs: types.Interactions{
				newTestInteraction(t, 0, address, nil),
				newTestInteraction(t, 5, address, nil),
			},
			expected: expectedResult{
				enqueued:         2,
				promotedAccounts: 1,
			},
		},
		{
			name: "All the ixs are with low nonce",
			ixs:  createTestIxs(t, 0, 5, address),
			testFn: func(ixPool *IxPool, interactions types.Interactions) {
				ixPool.createAccountOnce(interactions[0].Sender(), 5)
			},
			expected: expectedResult{
				enqueued:         0,
				promotedAccounts: 0,
			},
		},
		{
			name: "Should not enqueue ixs with low nonce",
			ixs:  createTestIxs(t, 0, 6, address),
			testFn: func(ixPool *IxPool, interactions types.Interactions) {
				ixPool.createAccountOnce(interactions[0].Sender(), 5)
			},
			expected: expectedResult{
				enqueued:         1,
				promotedAccounts: 1,
			},
		},
		{
			name: "Promote ixs with expected nonce",
			ixs:  createTestIxs(t, 0, 3, address),
			expected: expectedResult{
				enqueued:         3,
				promotedAccounts: 1,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(100)
			})
			senderAddress := testcase.ixs[0].Sender()

			if testcase.testFn != nil {
				testcase.testFn(ixPool, testcase.ixs)
			}

			promotedAccounts := getPromotedAccounts(t, ixPool, testcase.ixs, testcase.expected.promotedAccounts)

			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expected.promotedAccounts, len(promotedAccounts))
		})
	}
}

func TestIxPool_handlePromoteRequest(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name     string
		ixs      types.Interactions
		popIx    func(address types.Address)
		expected expectedResult
	}{
		{
			name: "Promote one ix",
			ixs:  createTestIxs(t, 0, 1, types.NilAddress),
			expected: expectedResult{
				nonce:    1,
				enqueued: 0,
				promoted: 1,
			},
		},
		{
			name: "Promote several ixs",
			ixs:  createTestIxs(t, 0, 3, tests.RandomAddress(t)),
			expected: expectedResult{
				nonce:    3,
				enqueued: 0,
				promoted: 3,
			},
		},
		{
			name: "Should not promote if the enqueue is empty",
			ixs:  createTestIxs(t, 0, 1, types.NilAddress),
			popIx: func(address types.Address) {
				ixPool.accounts.get(address).enqueued.pop()
			},
			expected: expectedResult{
				nonce:    0,
				enqueued: 0,
				promoted: 0,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			promoteRequest := addAndEnqueueIxs(t, ixPool, testcase.ixs, senderAddress)

			if testcase.popIx != nil {
				testcase.popIx(senderAddress)
			}

			ixPool.handlePromoteRequest(promoteRequest)

			// checks whether the account's nonce is updated
			require.Equal(t, testcase.expected.nonce, ixPool.accounts.get(senderAddress).getNonce())
			// checks whether the ixs are removed from the enqueue
			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			// checks whether the ixs are promoted
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_createAccountOnce(t *testing.T) {
	ixPool, mockStateManager := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name          string
		address       types.Address
		nonce         uint64
		testFn        func(addr types.Address)
		expectedNonce uint64
	}{
		{
			name:    "Create an account with latest state object nonce",
			address: tests.RandomAddress(t),
			nonce:   1,
			testFn: func(addr types.Address) {
				mockStateManager.setLatestNonce(addr, 5)
			},
			expectedNonce: 5,
		},
		{
			name:          "Create an account with given nonce as the state object doesn't exists",
			address:       tests.RandomAddress(t),
			nonce:         2,
			expectedNonce: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.address)
			}

			account := ixPool.createAccountOnce(testcase.address, testcase.nonce)
			require.NotNil(t, account)
			// checks whether an account is created with expected nonce
			require.Equal(t, testcase.expectedNonce, account.getNonce())
		})
	}
}

func TestIxPool_ResetWithHeaders(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name               string
		ixs                types.Interactions
		nonce              int
		incrementCounter   func(acc *account)
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the interactions with low nonce",
			ixs:                createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "Prune some interactions with low nonce",
			ixs:                createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce:              1,
			expectedPromotions: 3,
		},
		{
			name:  "Reset wait time",
			ixs:   createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce: 3,
			incrementCounter: func(acc *account) {
				// increment the account's delay counter
				acc.incrementCounter(2)
			},
			expectedPromotions: 1,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			if testcase.incrementCounter != nil {
				testcase.incrementCounter(ixPool.accounts.get(senderAddress))
				// check whether the delay counter is updated
				require.Equal(t, int32(1), ixPool.accounts.get(senderAddress).delayCounter)
			}

			ts := getTesseractWithIxs(t, senderAddress, testcase.nonce)

			// reset with headers removes the interactions with nonce lesser than given tesseract's interaction nonce
			// from the queues and resets the delay counter to default value
			ixPool.ResetWithHeaders(ts)

			require.Equal(t, int32(0), ixPool.accounts.get(senderAddress).delayCounter)
			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_resetAccount_enqueued(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name             string
		ixs              types.Interactions
		nonce            uint64
		promote          bool
		expectedEnqueues uint64
	}{
		{
			name:             "Prune all ixs with low nonce",
			ixs:              createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce:            5,
			expectedEnqueues: 0,
		},
		{
			name:             "No low nonce ixs to prune",
			ixs:              createTestIxs(t, 0, 6, tests.RandomAddress(t))[2:6],
			nonce:            1,
			expectedEnqueues: 4,
		},
		{
			name:             "Prune some ixs with low nonce",
			ixs:              createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce:            3,
			promote:          true,
			expectedEnqueues: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			addAndEnqueueIxsWithoutPromoting(t, ixPool, testcase.ixs, senderAddress)

			// On reset should prune the ixs from the enqueue if the nonce is lesser than the given nonce
			go ixPool.resetAccount(senderAddress, testcase.nonce)

			time.Sleep(100 * time.Millisecond)

			if testcase.promote {
				<-ixPool.promoteReqCh
			}

			ixPool.accounts.get(senderAddress).enqueued.lock(false)
			defer ixPool.accounts.get(senderAddress).enqueued.unlock()

			require.Equal(t, testcase.expectedEnqueues, ixPool.accounts.get(senderAddress).enqueued.length())
		})
	}
}

func TestIxPool_resetAccount_promoted(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name               string
		ixs                types.Interactions
		nonce              uint64
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the ixs with low nonce",
			ixs:                createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "No low nonce ixs to prune",
			ixs:                createTestIxs(t, 0, 6, tests.RandomAddress(t))[1:6],
			nonce:              0,
			expectedPromotions: 5,
		},
		{
			name:               "Prune some ixs with low nonce",
			ixs:                createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			nonce:              3,
			expectedPromotions: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			// On reset should prune the ixs from the promoted queue if the nonce is lesser than the given nonce
			ixPool.resetAccount(senderAddress, testcase.nonce)

			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_resetAccount(t *testing.T) {
	address := tests.RandomAddress(t)

	testcases := []struct {
		name     string
		ixs      types.Interactions
		nonce    uint64
		promote  bool
		signal   bool
		expected expectedResult
	}{
		{
			name: "Prune all ixs with low nonce",
			ixs: append(
				// promoted
				createTestIxs(t, 0, 3, address),
				// enqueued
				createTestIxs(t, 4, 7, address)...,
			),
			nonce: 7,
			expected: expectedResult{
				enqueued: 0,
				promoted: 0,
			},
		},
		{
			name: "No low nonce ixs to prune",
			ixs: append(
				// promoted
				createTestIxs(t, 6, 8, address),
				// enqueued
				createTestIxs(t, 10, 13, address)...,
			),
			nonce: 5,
			expected: expectedResult{
				enqueued: 3,
				promoted: 2,
			},
		},
		{
			name: "Prune all promoted and 1 enqueued",
			ixs: append(
				// promoted
				createTestIxs(t, 0, 3, address),
				// enqueued
				createTestIxs(t, 4, 7, address)...,
			),
			nonce:   5,
			promote: true,
			expected: expectedResult{
				enqueued: 2,
				promoted: 0,
			},
		},
		{
			name: "Prune signals promotion",
			ixs: append(
				// promoted
				createTestIxs(t, 0, 3, address),
				// enqueued
				createTestIxs(t, 4, 7, address)...,
			),
			nonce:  5,
			signal: true,
			expected: expectedResult{
				enqueued: 0,
				promoted: 2,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(100)
			})

			senderAddress := testcase.ixs[0].Sender()

			addAndProcessIxs(t, ixPool, testcase.ixs)

			go ixPool.resetAccount(senderAddress, testcase.nonce)

			if testcase.signal {
				ixPool.handlePromoteRequest(<-ixPool.promoteReqCh)
			} else if testcase.promote {
				<-ixPool.promoteReqCh
			}

			time.Sleep(100 * time.Millisecond)

			ixPool.accounts.get(senderAddress).enqueued.lock(false)
			ixPool.accounts.get(senderAddress).promoted.lock(false)

			defer func() {
				ixPool.accounts.get(senderAddress).enqueued.unlock()
				ixPool.accounts.get(senderAddress).promoted.unlock()
			}()

			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_Pop(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name               string
		ixs                types.Interactions
		expectedPromotions uint64
	}{
		{
			name:               "Prune the ix from the promoted queue",
			ixs:                createTestIxs(t, 0, 5, tests.RandomAddress(t)),
			expectedPromotions: 4,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.accounts.get(senderAddress).promoted.length())

			ix := ixPool.accounts.get(senderAddress).promoted.peek()

			ixPool.Pop(ix)

			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_Drop(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name string
		ixs  types.Interactions
	}{
		{
			name: "Remove the account form accounts map",
			ixs:  createTestIxs(t, 0, 5, tests.RandomAddress(t)),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.accounts.get(senderAddress).promoted.length())

			ix := ixPool.accounts.get(senderAddress).promoted.peek()

			ixPool.Drop(ix)

			require.Zero(t, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_IncrementWaitTime_InvalidAccount(t *testing.T) {
	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	err := ixPool.IncrementWaitTime(types.Address{0x0}, 1500*time.Millisecond)
	require.Error(t, err)
}

func TestIxPool_IncrementWaitTime(t *testing.T) {
	testcases := []struct {
		name            string
		addr            types.Address
		delta           int
		shouldReset     bool
		expectedCounter int32
	}{
		{
			name:            "Increment the wait counter by 1",
			addr:            types.Address{0x01},
			delta:           1,
			shouldReset:     false,
			expectedCounter: 1,
		},
		{
			name:            "Increment the wait counter by 5",
			addr:            types.Address{0x02},
			delta:           5,
			shouldReset:     false,
			expectedCounter: 5,
		},

		{
			name:            "Wait counter greater than max value",
			addr:            types.Address{0x03},
			delta:           MaxWaitCounter + 1,
			shouldReset:     true,
			expectedCounter: 0,
		},
	}

	ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			var initTime time.Time

			baseTime := 1500 * time.Millisecond
			acc := ixPool.createAccountOnce(testcase.addr, 0)

			for i := 0; i < testcase.delta; i++ {
				require.NoError(t, ixPool.IncrementWaitTime(testcase.addr, baseTime))
				initTime = time.Now()
			}

			require.Equal(t, testcase.expectedCounter, acc.delayCounter)

			if testcase.shouldReset {
				require.InDelta(t,
					utils.ExponentialTimeout(baseTime, acc.delayCounter).Milliseconds(),
					initTime.Sub(acc.waitTime).Milliseconds(),
					float64(0),
				)

				return
			}

			require.InDelta(t,
				utils.ExponentialTimeout(baseTime, acc.delayCounter).Milliseconds(),
				acc.waitTime.Sub(initTime).Milliseconds(),
				float64(baseTime.Milliseconds()*int64(math.Pow(2, float64(testcase.delta)))),
			)
		})
	}
}

func TestIxPool_validateIx(t *testing.T) {
	ixPool, mockStateManager := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = big.NewInt(100)
	})

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction)
		expectedErr error
	}{
		{
			name:        "Oversized data error",
			ix:          newIxWithPayload(t, 5, types.NilAddress, make([]byte, ixMaxSize+2)),
			expectedErr: ErrOversizedData,
		},
		{
			name:        "Invalid address error",
			ix:          newIxWithoutAddress(t, 5),
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Nonce too low error",
			ix:   newTestInteraction(t, 9, types.NilAddress, nil),
			testFn: func(interaction *types.Interaction) {
				mockStateManager.setLatestNonce(interaction.Sender(), 10)
			},
			expectedErr: ErrNonceTooLow,
		},
		{
			name:        "Underpriced error",
			ix:          newIxWithFuelPrice(t, 5, types.NilAddress, 50),
			expectedErr: types.ErrUnderpriced,
		},
		{
			name:        "Valid ix should not return error",
			ix:          newTestInteraction(t, 0, types.NilAddress, nil),
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.ix)
			}

			err := ixPool.validateIx(testcase.ix)
			require.Equal(t, testcase.expectedErr, err)
		})
	}
}

type (
	accounts      map[types.Address]types.Interactions
	delayCounters map[types.Address]int32
)

func TestIxPool_Executables_Wait_Mode(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 4)

	testcases := []struct {
		name               string
		accounts           accounts
		delayCounters      delayCounters
		updateDelayCounter func(ixPool *IxPool, delayCounters delayCounters)
		expectedAddrList   []types.Address
	}{
		{
			name: "One ix per account",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: createTestIxs(t, 0, 1, addresses[0]),
				addresses[1]: createTestIxs(t, 0, 1, addresses[1]),
				addresses[2]: createTestIxs(t, 0, 1, addresses[2]),
				addresses[3]: createTestIxs(t, 0, 1, addresses[3]),
			},
			delayCounters: map[types.Address]int32{
				addresses[0]: 3,
				addresses[1]: 4,
				addresses[2]: 1,
				addresses[3]: 2,
			},
			updateDelayCounter: func(ixPool *IxPool, delayCounters delayCounters) {
				for addr := range delayCounters {
					acc := ixPool.accounts.get(addr)
					setDelayCounter(t, acc, delayCounters[addr])
				}
			},
			expectedAddrList: []types.Address{
				addresses[1],
				addresses[0],
				addresses[3],
				addresses[2],
			},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: createTestIxs(t, 0, 2, addresses[0]),
				addresses[1]: createTestIxs(t, 0, 2, addresses[1]),
				addresses[2]: createTestIxs(t, 0, 2, addresses[2]),
				addresses[3]: createTestIxs(t, 0, 2, addresses[3]),
			},
			delayCounters: map[types.Address]int32{
				addresses[0]: 3,
				addresses[1]: 4,
				addresses[2]: 1,
				addresses[3]: 2,
			},
			updateDelayCounter: func(ixPool *IxPool, delayCounters delayCounters) {
				for addr := range delayCounters {
					acc := ixPool.accounts.get(addr)
					setDelayCounter(t, acc, delayCounters[addr])
				}
			},
			expectedAddrList: []types.Address{
				addresses[1],
				addresses[0],
				addresses[3],
				addresses[2],
				addresses[1],
				addresses[0],
				addresses[3],
				addresses[2],
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(0)
			})

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				errs := ixPool.AddInteractions(ixs)
				require.Len(t, errs, 0)
			}

			if testcase.updateDelayCounter != nil {
				testcase.updateDelayCounter(ixPool, testcase.delayCounters)
			}

			time.Sleep(100 * time.Millisecond)

			successfulIxs := getSuccessfulIxs(t, ixPool, len(testcase.expectedAddrList))

			// check's whether the interactions are processed in the expected order based on delay counter
			for index, expectedAddress := range testcase.expectedAddrList {
				require.Equal(t, expectedAddress, successfulIxs[index].Sender())
			}
		})
	}
}

func TestIxPool_Executables_Cost_Mode(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 5)

	testcases := []struct {
		name              string
		accounts          map[types.Address]types.Interactions
		expectedPriceList []uint64
	}{
		{
			name: "One ix per account",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: {
					newIxWithFuelPrice(t, 0, addresses[0], 1),
				},
				addresses[1]: {
					newIxWithFuelPrice(t, 0, addresses[1], 2),
				},
				addresses[2]: {
					newIxWithFuelPrice(t, 0, addresses[2], 3),
				},
				addresses[3]: {
					newIxWithFuelPrice(t, 0, addresses[3], 4),
				},
				addresses[4]: {
					newIxWithFuelPrice(t, 0, addresses[4], 5),
				},
			},
			expectedPriceList: []uint64{5, 4, 3, 2, 1},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: {
					newIxWithFuelPrice(t, 0, addresses[0], 3),
					newIxWithFuelPrice(t, 1, addresses[0], 3),
				},
				addresses[1]: {
					newIxWithFuelPrice(t, 0, addresses[1], 2),
					newIxWithFuelPrice(t, 1, addresses[1], 2),
				},
				addresses[2]: {
					newIxWithFuelPrice(t, 0, addresses[2], 1),
					newIxWithFuelPrice(t, 1, addresses[2], 1),
				},
			},
			expectedPriceList: []uint64{3, 2, 1, 3, 2, 1},
		},
		{
			name: "Several ixs from multiple accounts with same fuel cost",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: {
					newIxWithFuelPrice(t, 0, addresses[0], 6),
					newIxWithFuelPrice(t, 1, addresses[0], 3),
				},
				addresses[1]: {
					newIxWithFuelPrice(t, 0, addresses[1], 5),
					newIxWithFuelPrice(t, 1, addresses[1], 4),
				},
				addresses[2]: {
					newIxWithFuelPrice(t, 0, addresses[2], 6),
					newIxWithFuelPrice(t, 1, addresses[2], 2),
				},
			},
			expectedPriceList: []uint64{6, 6, 5, 4, 3, 2},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 1
				c.PriceLimit = big.NewInt(0)
			})

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				errs := ixPool.AddInteractions(ixs)
				require.Len(t, errs, 0)
			}

			time.Sleep(100 * time.Millisecond)

			successfulIxs := getSuccessfulIxs(t, ixPool, len(testcase.expectedPriceList))

			// check's whether the interactions are processed in the expected order based on gas cost
			for index, expectedPrice := range testcase.expectedPriceList {
				require.Equal(t, expectedPrice, successfulIxs[index].FuelPrice().Uint64())
			}
		})
	}
}

func TestIxPool_Executables_Wait_Time(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 5)

	testcases := []struct {
		name            string
		accounts        map[types.Address]types.Interactions
		accountWaitTime map[types.Address]time.Time
		updateWaitTime  func(ixPool *IxPool, accountWaitTime map[types.Address]time.Time)
		expectedNonce   map[types.Address]uint64
	}{
		{
			name: "One ix per account",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: createTestIxs(t, 7, 8, addresses[0]),
				addresses[1]: createTestIxs(t, 8, 9, addresses[1]),
				addresses[2]: createTestIxs(t, 5, 6, addresses[2]),
				addresses[3]: createTestIxs(t, 6, 7, addresses[3]),
			},
			accountWaitTime: map[types.Address]time.Time{
				addresses[0]: time.Now().Add(1000 * time.Millisecond),
				addresses[1]: time.Now().Add(-100),
				addresses[2]: time.Now().Add(-200),
				addresses[3]: time.Now().Add(-150),
			},
			updateWaitTime: func(ixPool *IxPool, accountWaitTime map[types.Address]time.Time) {
				for addr := range accountWaitTime {
					acc := ixPool.accounts.get(addr)
					acc.waitTime = accountWaitTime[addr]
				}
			},
			expectedNonce: map[types.Address]uint64{
				addresses[1]: 8,
				addresses[2]: 5,
				addresses[3]: 6,
			},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[types.Address]types.Interactions{
				addresses[0]: createTestIxs(t, 4, 6, addresses[0]),
				addresses[1]: createTestIxs(t, 5, 7, addresses[1]),
				addresses[2]: createTestIxs(t, 6, 8, addresses[2]),
				addresses[3]: createTestIxs(t, 7, 9, addresses[3]),
			},
			accountWaitTime: map[types.Address]time.Time{
				addresses[0]: time.Now().Add(-200),
				addresses[1]: time.Now().Add(-100),
				addresses[2]: time.Now().Add(1000 * time.Millisecond),
				addresses[3]: time.Now().Add(-150),
			},
			updateWaitTime: func(ixPool *IxPool, accountWaitTime map[types.Address]time.Time) {
				for addr := range accountWaitTime {
					acc := ixPool.accounts.get(addr)
					acc.waitTime = accountWaitTime[addr]
				}
			},
			expectedNonce: map[types.Address]uint64{
				addresses[0]: 4,
				addresses[1]: 5,
				addresses[3]: 7,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool, _ := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 0
				c.PriceLimit = big.NewInt(0)
			})

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				errs := ixPool.AddInteractions(ixs)
				require.Len(t, errs, 0)
			}

			time.Sleep(100 * time.Millisecond)

			if testcase.accountWaitTime != nil {
				testcase.updateWaitTime(ixPool, testcase.accountWaitTime)
			}

			mintedIxs := mintIxs(t, ixPool)

			require.Len(t, mintedIxs, len(testcase.expectedNonce))

			ixNonce := getIxNonce(t, mintedIxs)

			for addr, nonce := range testcase.expectedNonce {
				require.NotNil(t, ixNonce[addr])
				require.Equal(t, nonce, ixNonce[addr])
			}
		})
	}
}

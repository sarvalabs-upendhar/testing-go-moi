package ixpool

import (
	"math"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

func TestIxPool_AddInteractions_checkIx(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)

	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		preTestFn   func(ixPool *IxPool, interaction *types.Interaction)
		expectedErr error
	}{
		{
			name:        "Interaction with invalid address",
			ix:          newIxWithoutAddress(t, 0),
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Already known Interaction",
			ix:   newTestInteraction(t, types.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *types.Interaction) {
				ixPool.allIxs.add(interaction)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "New valid interaction",
			ix:   newTestInteraction(t, types.IxValueTransfer, 0, addrs[0], nil),
		},
		{
			name: "ixpool overflow",
			ix:   newTestInteraction(t, types.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *types.Interaction) {
				ixPool.gauge.increase(common.DefaultMaxIXPoolSlots)
			},
			expectedErr: types.ErrIXPoolOverFlow,
		},
		{
			name: "fill ixpool to the limit",
			ix:   newTestInteraction(t, types.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *types.Interaction) {
				ixPool.gauge.increase(common.DefaultMaxIXPoolSlots - 1)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = common.DefaultIxPriceLimit
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			if test.preTestFn != nil {
				test.preTestFn(ixPool, test.ix)
			}

			err := ixPool.checkIx(test.ix)

			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, len(ixPool.allIxs.all))
		})
	}
}

func TestIxPool_AddInteractions_HighPressure(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)

	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name      string
		ix        *types.Interaction
		preTestFn func(ixPool *IxPool, interaction *types.Interaction)
	}{
		{
			name: "prune should be signalled when ixpool overflows",
			ix:   newTestInteraction(t, types.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *types.Interaction) {
				ixPool.gauge.increase(common.DefaultMaxIXPoolSlots)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = common.DefaultIxPriceLimit
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			if test.preTestFn != nil {
				test.preTestFn(ixPool, test.ix)
			}

			go func() {
				err := ixPool.checkIx(test.ix)
				require.Equal(t, types.ErrIXPoolOverFlow, err)
			}()

			_, ok := <-ixPool.pruneCh
			require.True(t, ok)
		})
	}
}

func TestIxPool_AddInteractions(t *testing.T) {
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	sm.setBalance(addr1, types.KMOITokenAssetID, big.NewInt(1000))
	// set some MOI balance for fuel checks
	sm.setBalance(addr1, types.KMOITokenAssetID, big.NewInt(1000))

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
				newIxWithFuelPrice(t, 1, types.NilAddress, 1),
			},
			expectedIxs:  0,
			expectedErrs: 2,
		},
		{
			name: "Some interactions are valid",
			ixs: types.Interactions{
				newIxWithoutAddress(t, 0),
				newTestInteraction(t, types.IxValueTransfer, 1, addr1, nil),
				newTestInteraction(t, types.IxValueTransfer, 2, addr1, nil),
			},
			expectedIxs:  2,
			expectedErrs: 1,
		},
		{
			name:         "All the interactions are valid",
			ixs:          createTestIxs(t, types.IxValueTransfer, 0, 2, addr1),
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

			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

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
	sm := NewMockStateManager(t)
	sm.setBalance(address, types.KMOITokenAssetID, big.NewInt(1000))

	testcases := []struct {
		name     string
		ixs      types.Interactions
		testFn   func(ixPool *IxPool, interactions types.Interactions)
		expected expectedResult
	}{
		{
			name: "Enqueue ixs with higher nonce",
			ixs: types.Interactions{
				newTestInteraction(t, types.IxValueTransfer, 0, address, nil),
				newTestInteraction(t, types.IxValueTransfer, 5, address, nil),
			},
			expected: expectedResult{
				enqueued:         2,
				promotedAccounts: 1,
			},
		},
		{
			name: "All the ixs are with low nonce",
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 5, address),
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
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 6, address),
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
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 3, address),
			expected: expectedResult{
				enqueued:         3,
				promotedAccounts: 1,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)
			senderAddress := testcase.ixs[0].Sender()

			if testcase.testFn != nil {
				testcase.testFn(ixPool, testcase.ixs)
			}

			require.Equal(t, uint64(0), ixPool.gauge.read())
			promotedAccounts := getPromotedAccounts(t, ixPool, testcase.ixs, testcase.expected.promotedAccounts)

			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expected.promotedAccounts, len(promotedAccounts))
			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, ixPool.gauge.read())
		})
	}
}

func TestIxPool_handlePromoteRequest(t *testing.T) {
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)
	addr3 := tests.RandomAddress(t)

	sm.setTestMOIBalance(addr1, addr2, addr3)

	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
		c.MaxSlots = common.DefaultMaxIXPoolSlots
	}, true, sm)

	testcases := []struct {
		name     string
		ixs      types.Interactions
		popIx    func(address types.Address)
		expected expectedResult
	}{
		{
			name: "Promote one ix",
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 1, addr1),
			expected: expectedResult{
				nonce:    1,
				enqueued: 0,
				promoted: 1,
			},
		},
		{
			name: "Promote several ixs",
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 3, addr2),
			expected: expectedResult{
				nonce:    3,
				enqueued: 0,
				promoted: 3,
			},
		},
		{
			name: "Should not promote if the enqueue is empty",
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 1, addr3),
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
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, true, sm)

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
				sm.setLatestNonce(addr, 5)
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
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)
	addr3 := tests.RandomAddress(t)

	sm.setTestMOIBalance(addr1, addr2, addr3)

	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
		c.MaxSlots = common.DefaultMaxIXPoolSlots
	}, true, sm)

	testcases := []struct {
		name               string
		ixs                types.Interactions
		nonce              int
		incrementCounter   func(acc *account)
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the interactions with low nonce",
			ixs:                createTestIxs(t, types.IxValueTransfer, 0, 5, addr1),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "Prune some interactions with low nonce",
			ixs:                createTestIxs(t, types.IxValueTransfer, 0, 5, addr2),
			nonce:              1,
			expectedPromotions: 3,
		},
		{
			name:  "Reset wait time",
			ixs:   createTestIxs(t, types.IxValueTransfer, 0, 5, addr3),
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
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)
	addr3 := tests.RandomAddress(t)

	sm.setTestMOIBalance(addr1, addr2, addr3)

	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
		c.MaxSlots = common.DefaultMaxIXPoolSlots
	}, true, sm)

	testcases := []struct {
		name             string
		ixs              types.Interactions
		nonce            uint64
		promote          bool
		expectedEnqueues uint64
	}{
		{
			name:             "Prune all ixs with low nonce",
			ixs:              createTestIxs(t, types.IxValueTransfer, 0, 5, addr1),
			nonce:            5,
			expectedEnqueues: 0,
		},
		{
			name:             "No low nonce ixs to prune",
			ixs:              createTestIxs(t, types.IxValueTransfer, 0, 6, addr2)[2:6],
			nonce:            1,
			expectedEnqueues: 4,
		},
		{
			name:             "Prune some ixs with low nonce",
			ixs:              createTestIxs(t, types.IxValueTransfer, 0, 5, addr3),
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
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)
	addr3 := tests.RandomAddress(t)

	sm.setTestMOIBalance(addr1, addr2, addr3)

	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
		c.MaxSlots = common.DefaultMaxIXPoolSlots
	}, true, sm)

	testcases := []struct {
		name               string
		ixs                types.Interactions
		nonce              uint64
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the ixs with low nonce",
			ixs:                createTestIxs(t, types.IxValueTransfer, 0, 5, addr1),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "No low nonce ixs to prune",
			ixs:                createTestIxs(t, types.IxValueTransfer, 0, 6, addr2)[1:6],
			nonce:              0,
			expectedPromotions: 5,
		},
		{
			name:               "Prune some ixs with low nonce",
			ixs:                createTestIxs(t, types.IxValueTransfer, 0, 5, addr3),
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
				createTestIxs(t, types.IxValueTransfer, 0, 3, address),
				// enqueued
				createTestIxs(t, types.IxValueTransfer, 4, 7, address)...,
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
				createTestIxs(t, types.IxValueTransfer, 6, 8, address),
				// enqueued
				createTestIxs(t, types.IxValueTransfer, 10, 13, address)...,
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
				createTestIxs(t, types.IxValueTransfer, 0, 3, address),
				// enqueued
				createTestIxs(t, types.IxValueTransfer, 4, 7, address)...,
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
				createTestIxs(t, types.IxValueTransfer, 0, 3, address),
				// enqueued
				createTestIxs(t, types.IxValueTransfer, 4, 7, address)...,
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
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			senderAddress := testcase.ixs[0].Sender()

			require.Equal(t, uint64(0), ixPool.gauge.read())
			addAndProcessIxs(t, sm, ixPool, testcase.ixs)
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

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

			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, ixPool.gauge.read())
			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_Pop(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(addr1)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
		c.MaxSlots = common.DefaultMaxIXPoolSlots
	}, true, sm)

	testcases := []struct {
		name               string
		ixs                types.Interactions
		expectedPromotions uint64
	}{
		{
			name:               "Prune the ix from the promoted queue",
			ixs:                createTestIxs(t, types.IxValueTransfer, 0, 5, addr1),
			expectedPromotions: 4,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			require.Equal(t, uint64(0), ixPool.gauge.read())
			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.accounts.get(senderAddress).promoted.length())

			ix := ixPool.accounts.get(senderAddress).promoted.peek()

			ixPool.Pop(ix)
			require.Equal(t, testcase.expectedPromotions, ixPool.gauge.read())
			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_Drop(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(addr1)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
		c.MaxSlots = common.DefaultMaxIXPoolSlots
	}, true, sm)

	testcases := []struct {
		name string
		ixs  types.Interactions
	}{
		{
			name: "Remove the account form accounts map",
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 5, addr1),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.accounts.get(senderAddress).promoted.length())

			ix := ixPool.accounts.get(senderAddress).promoted.peek()

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			ixPool.Drop(ix)

			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Zero(t, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_IncrementWaitTime_InvalidAccount(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, false, sm)

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

	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, false, sm)

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
	sm := NewMockStateManager(t)
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)
	addr3 := tests.RandomAddress(t)
	sm.setTestMOIBalance(addr1, addr2, addr3)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, true, sm)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction)
		expectedErr error
	}{
		{
			name:        "Oversized data error",
			ix:          newIxWithPayload(t, types.IxValueTransfer, 5, addr1, make([]byte, ixMaxSize+2)),
			expectedErr: ErrOversizedData,
		},
		{
			name:        "Invalid address error",
			ix:          newIxWithoutAddress(t, 5),
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Nonce too low error",
			ix:   newTestInteraction(t, types.IxValueTransfer, 9, addr2, nil),
			testFn: func(interaction *types.Interaction) {
				sm.setLatestNonce(interaction.Sender(), 10)
			},
			expectedErr: ErrNonceTooLow,
		},
		{
			name:        "Underpriced error",
			ix:          newIxWithFuelPrice(t, 5, addr1, 50),
			expectedErr: types.ErrUnderpriced,
		},
		{
			name: "Ix with negative transfer value",
			ix: newTestInteraction(t, types.IxValueTransfer, 0, addr3, func(ixData *types.IxData) {
				ixData.Input.Type = types.IxValueTransfer
				ixData.Input.TransferValues = map[types.AssetID]*big.Int{
					"assetID1": new(big.Int).Neg(big.NewInt(20)),
				}
			}),
			expectedErr: types.ErrInvalidValue,
		},
		{
			name: "Ix with invalid assetID",
			ix: newTestInteraction(t, types.IxValueTransfer, 0, addr1, func(ixData *types.IxData) {
				ixData.Input.Type = types.IxValueTransfer
				ixData.Input.TransferValues = map[types.AssetID]*big.Int{
					"assetID1": big.NewInt(20),
				}
			}),
			expectedErr: types.ErrFetchingBalance,
		},
		{
			name: "Ix with insufficient funds",
			ix: newTestInteraction(t, types.IxValueTransfer, 0, addr1, func(ixData *types.IxData) {
				ixData.Input.Type = types.IxValueTransfer
				ixData.Input.TransferValues = map[types.AssetID]*big.Int{
					"assetID1": big.NewInt(20),
				}
			}),
			testFn: func(interaction *types.Interaction) {
				sm.balance[interaction.Sender()] = map[types.AssetID]*big.Int{
					"assetID1": big.NewInt(10),
				}
			},
			expectedErr: types.ErrInsufficientFunds,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.ix)
			}

			err := ixPool.validateIx(testcase.ix)

			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_validateIx_WithSign(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, false, sm)

	address, mnemonic := tests.RandomAddressWithMnemonic(t)
	addr2 := tests.RandomAddress(t)

	sm.setTestMOIBalance(address, addr2)

	ixArgs := types.SendIXArgs{
		Sender:    address,
		Type:      types.IxValueTransfer,
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(1),
		TransferValues: map[types.AssetID]*big.Int{
			"assetID1": big.NewInt(5),
		},
	}

	rawSign := tests.GetIXSignature(t, &ixArgs, mnemonic)

	ix := tests.CreateIX(t, getIXParams(
		address,
		types.IxValueTransfer,
		big.NewInt(1),
		map[types.AssetID]*big.Int{
			"assetID1": big.NewInt(5),
		},
		rawSign,
	))

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction)
		expectedErr error
	}{
		{
			name: "invalid signature",
			ix: newTestInteraction(t, types.IxValueTransfer, 0, addr2, func(ixData *types.IxData) {
				ixData.Input.TransferValues = map[types.AssetID]*big.Int{
					"assetID1": big.NewInt(5),
				}
			}),
			testFn: func(interaction *types.Interaction) {
				sm.setBalance(interaction.Sender(), "assetID1", big.NewInt(10))
			},
			expectedErr: types.ErrInvalidIXSignature,
		},
		{
			name: "valid signature",
			ix:   ix,
			testFn: func(interaction *types.Interaction) {
				sm.setBalance(interaction.Sender(), "assetID1", big.NewInt(10))
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.ix)
			}

			err := ixPool.validateIx(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateAssetCreate(t *testing.T) {
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, false, nil)

	address := tests.RandomAddress(t)
	validAssetPayload := types.AssetCreatePayload{
		Standard: types.MAS1,
		Supply:   big.NewInt(1),
	}

	invalidAssetStandardPayload := types.AssetCreatePayload{
		Standard: 2,
	}

	invalidAssetSupplyPayload := types.AssetCreatePayload{
		Standard: types.MAS1,
		Supply:   big.NewInt(33),
	}

	rawValidAssetPayload, err := validAssetPayload.Bytes()
	require.NoError(t, err)

	rawInValidAssetStandardPayload, err := invalidAssetStandardPayload.Bytes()
	require.NoError(t, err)

	rawInvalidAssetSupplyPayload, err := invalidAssetSupplyPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		expectedErr error
	}{
		{
			name: "should return error if asset standard is invalid",
			ix: newTestInteraction(t, types.IxAssetCreate, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawInValidAssetStandardPayload
			}),
			expectedErr: types.ErrInvalidAssetStandard,
		},
		{
			name: "should return error if asset supply is invalid",
			ix: newTestInteraction(t, types.IxAssetCreate, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawInvalidAssetSupplyPayload
			}),
			expectedErr: types.ErrInvalidAssetSupply,
		},
		{
			name: "should return success if asset standard is valid",
			ix: newTestInteraction(t, types.IxAssetCreate, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawValidAssetPayload
			}),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := ixPool.validateAssetCreate(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateAssetMint(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = big.NewInt(1)
	}, false, sm)

	address := tests.RandomAddress(t)
	assetID := tests.GetRandomAssetID(t, address)
	assetPayload := types.AssetMintOrBurnPayload{
		Asset: assetID,
	}

	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	NFTAssetPayload := types.AssetMintOrBurnPayload{
		Asset: types.NewAssetIDv0(false, false, 1, types.MAS1, address),
	}

	rawNFTAssetPayload, err := NFTAssetPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction)
		expectedErr error
	}{
		{
			name: "should return error if asset not found",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			expectedErr: types.ErrAssetNotFound,
		},
		{
			name: "should return error if operator address mismatch",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *types.Interaction) {
				sm.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: tests.RandomAddress(t),
				})
			},
			expectedErr: errors.New("Operator address mismatch"),
		},
		{
			name: "should return success if asset mint data is valid",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *types.Interaction) {
				sm.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
		},
		{
			name: "should return error if non fungible token minted",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawNFTAssetPayload
			}),
			testFn: func(interaction *types.Interaction) {
				sm.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
			expectedErr: types.ErrMintNonFungibleToken,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.ix)
			}

			err := ixPool.validateAssetMint(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateAssetBurn(t *testing.T) {
	address := tests.RandomAddress(t)
	assetID := tests.GetRandomAssetID(t, address)
	assetPayload := types.AssetMintOrBurnPayload{
		Asset:  assetID,
		Amount: big.NewInt(100),
	}

	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction, msm *MockStateManager)
		expectedErr error
	}{
		{
			name: "asset not found",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			expectedErr: types.ErrAssetNotFound,
		},
		{
			name: "balance not found",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *types.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
			expectedErr: types.ErrFetchingBalance,
		},
		{
			name: "insufficient funds",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *types.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.balance[interaction.Sender()] = map[types.AssetID]*big.Int{
					assetID: big.NewInt(10),
				}
				mockStateManager.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
			expectedErr: types.ErrInsufficientFunds,
		},
		{
			name: "operator address mismatch",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *types.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.balance[interaction.Sender()] = map[types.AssetID]*big.Int{
					assetID: big.NewInt(1000),
				}
				mockStateManager.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: tests.RandomAddress(t),
				})
			},
			expectedErr: errors.New("Operator address mismatch"),
		},
		{
			name: "valid asset burn data",
			ix: newTestInteraction(t, types.IxAssetMint, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *types.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.balance[interaction.Sender()] = map[types.AssetID]*big.Int{
					assetID: big.NewInt(1000),
				}
				mockStateManager.setAssetInfo(assetID, &types.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
			}, false, sm)

			if testcase.testFn != nil {
				testcase.testFn(testcase.ix, sm)
			}

			err := ixPool.validateAssetBurn(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateLogicDeployPayload(t *testing.T) {
	address := tests.RandomAddress(t)
	invalidLogicPayload := types.LogicPayload{}
	validLogicPayload := types.LogicPayload{
		Manifest: []byte{1, 2},
	}

	invalidRawLogicPayload, err := invalidLogicPayload.Bytes()
	require.NoError(t, err)

	validRawLogicPayload, err := validLogicPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction, msm *MockStateManager)
		expectedErr error
	}{
		{
			name: "should return error if manifest is empty",
			ix: newTestInteraction(t, types.IxLogicDeploy, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = invalidRawLogicPayload
			}),
			expectedErr: types.ErrEmptyManifest,
		},
		{
			name: "should return error if receiver account is already registered",
			ix: newTestInteraction(t, types.IxLogicDeploy, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = invalidRawLogicPayload
			}),
			testFn: func(interaction *types.Interaction, msm *MockStateManager) {
				msm.registerAccount(interaction.Receiver())
			},
			expectedErr: errors.New("account registered"),
		},
		{
			name: "should return success if logic payload is valid",
			ix: newTestInteraction(t, types.IxLogicDeploy, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = validRawLogicPayload
			}),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
			}, false, sm)

			if testcase.testFn != nil {
				testcase.testFn(testcase.ix, sm)
			}

			err := ixPool.validateLogicDeployPayload(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateLogicInvokePayload(t *testing.T) {
	address := tests.RandomAddress(t)
	invalidLogicPayload := types.LogicPayload{}
	validLogicPayload := types.LogicPayload{
		Logic:    "logicID-1",
		Callsite: "seeder!",
	}

	invalidRawLogicPayload, err := invalidLogicPayload.Bytes()
	require.NoError(t, err)

	validRawLogicPayload, err := validLogicPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *types.Interaction
		testFn      func(interaction *types.Interaction, msm *MockStateManager)
		expectedErr error
	}{
		{
			name: "should return error if call site is empty",
			ix: newTestInteraction(t, types.IxLogicInvoke, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = invalidRawLogicPayload
			}),
			expectedErr: types.ErrEmptyCallSite,
		},
		{
			name: "should return success if logic payload is valid",
			ix: newTestInteraction(t, types.IxLogicInvoke, 0, address, func(ixData *types.IxData) {
				ixData.Input.Payload = validRawLogicPayload
			}),
			testFn: func(interaction *types.Interaction, msm *MockStateManager) {
				msm.registerLogicID("logicID-1")
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
			}, false, sm)

			if testcase.testFn != nil {
				testcase.testFn(testcase.ix, sm)
			}

			err := ixPool.validateLogicInvokePayload(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
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
				addresses[0]: createTestIxs(t, types.IxValueTransfer, 0, 1, addresses[0]),
				addresses[1]: createTestIxs(t, types.IxValueTransfer, 0, 1, addresses[1]),
				addresses[2]: createTestIxs(t, types.IxValueTransfer, 0, 1, addresses[2]),
				addresses[3]: createTestIxs(t, types.IxValueTransfer, 0, 1, addresses[3]),
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
				addresses[0]: createTestIxs(t, types.IxValueTransfer, 0, 2, addresses[0]),
				addresses[1]: createTestIxs(t, types.IxValueTransfer, 0, 2, addresses[1]),
				addresses[2]: createTestIxs(t, types.IxValueTransfer, 0, 2, addresses[2]),
				addresses[3]: createTestIxs(t, types.IxValueTransfer, 0, 2, addresses[3]),
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
			sm := NewMockStateManager(t)
			sm.setTestMOIBalance(addresses...)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

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

/*
FIXME: Currently the fuel price is always set to 1
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
			sm := NewMockStateManager(t)
			sm.setTestMOIBalance(addresses...)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = 1
				c.PriceLimit = big.NewInt(1)
			}, true, sm)

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				errs := ixPool.AddInteractions(ixs)
				log.Println(errs)
				require.Len(t, errs, 1)
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
*/

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
				addresses[0]: createTestIxs(t, types.IxValueTransfer, 7, 8, addresses[0]),
				addresses[1]: createTestIxs(t, types.IxValueTransfer, 8, 9, addresses[1]),
				addresses[2]: createTestIxs(t, types.IxValueTransfer, 5, 6, addresses[2]),
				addresses[3]: createTestIxs(t, types.IxValueTransfer, 6, 7, addresses[3]),
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
				addresses[0]: createTestIxs(t, types.IxValueTransfer, 4, 6, addresses[0]),
				addresses[1]: createTestIxs(t, types.IxValueTransfer, 5, 7, addresses[1]),
				addresses[2]: createTestIxs(t, types.IxValueTransfer, 6, 8, addresses[2]),
				addresses[3]: createTestIxs(t, types.IxValueTransfer, 7, 9, addresses[3]),
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
			sm := NewMockStateManager(t)
			sm.setTestMOIBalance(addresses...)
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

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

func TestIxPool_RemoveNonceHoleAccounts(t *testing.T) {
	sm := NewMockStateManager(t)
	addr := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(addr...)

	testcases := []struct {
		name                string
		ixs                 types.Interactions
		ixPoolCallback      func(i *IxPool)
		hasNonceHoles       bool
		expectedEnqueuedIxs uint64
	}{
		{
			name: "accounts without nonce holes",
			ixs:  createTestIxs(t, types.IxValueTransfer, 0, 5, addr[0]),
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[0], 0)
			},
			expectedEnqueuedIxs: 5,
		},
		{
			name: "accounts with nonce holes",
			ixs:  createTestIxs(t, types.IxValueTransfer, 2, 8, addr[1]),
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[1], 0)
			},
			hasNonceHoles:       true,
			expectedEnqueuedIxs: 0,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			if test.ixPoolCallback != nil {
				test.ixPoolCallback(ixPool)
			}

			senderAddress := test.ixs[0].Sender()

			// make sure gauge is zero initially
			require.Equal(t, uint64(0), ixPool.gauge.read())

			if test.hasNonceHoles {
				createNonceHolesInEnqueue(t, ixPool, test.ixs, senderAddress)
				require.NotEqual(t, ixPool.accounts.get(senderAddress).getNonce(),
					ixPool.accounts.get(senderAddress).enqueued.length())
			} else {
				addAndEnqueueIxsWithoutPromoting(t, ixPool, test.ixs, senderAddress)
			}

			// make sure gauge is increased after ixns enqueued
			require.Equal(t, slotsRequired(test.ixs...), ixPool.gauge.read())

			ixPool.removeNonceHoleAccounts()

			// make sure gauge decreased
			require.Equal(t, test.expectedEnqueuedIxs, ixPool.gauge.read())

			ixPool.accounts.get(senderAddress).enqueued.lock(false)
			defer ixPool.accounts.get(senderAddress).enqueued.unlock()

			require.Equal(t, test.expectedEnqueuedIxs, ixPool.accounts.get(senderAddress).enqueued.length())
		})
	}
}

func TestIxPool_RemoveNonceHoleAccounts_WithEmptyEnqueues(t *testing.T) {
	sm := NewMockStateManager(t)
	addr := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(addr...)

	testcases := []struct {
		name           string
		ixs            types.Interactions
		ixPoolCallback func(i *IxPool)
	}{
		{
			name: "accounts with empty enqueues",
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[0], 0)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = big.NewInt(1)
				c.MaxSlots = common.DefaultMaxIXPoolSlots
			}, true, sm)

			if test.ixPoolCallback != nil {
				test.ixPoolCallback(ixPool)
			}

			// make sure gauge is zero initially
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.get(addr[0]).getNonce())
			require.Equal(t, uint64(0), ixPool.accounts.get(addr[0]).enqueued.length())

			ixPool.removeNonceHoleAccounts()

			// make sure gauge is not decreased
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.get(addr[0]).getNonce())
			require.Equal(t, uint64(0), ixPool.accounts.get(addr[0]).enqueued.length())
		})
	}
}

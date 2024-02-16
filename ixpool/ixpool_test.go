package ixpool

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
)

var defaultIxPriceLimit = big.NewInt(1)

const contextTimeout = 5 * time.Second

func TestIxPool_validateAndEnqueueIx_ReplaceIx(t *testing.T) {
	var (
		defaultPayload = make([]byte, 3*ixSlotSize)
		price          = 1
		sm             = NewMockStateManager(t)
		addrs          = tests.GetAddresses(t, 1)
	)

	sm.setTestMOIBalance(addrs...)

	getIxParams := func(from identifiers.Address, nonce uint64, price int, payload []byte) *tests.CreateIxParams {
		return &tests.CreateIxParams{
			IxDataCallback: func(ix *common.IxData) {
				ix.Input.Sender = from
				ix.Input.Nonce = nonce
				ix.Input.Type = common.IxValueTransfer
				ix.Input.FuelPrice = big.NewInt(int64(price))
				ix.Input.Payload = payload
				ix.Input.FuelLimit = 1
				ix.Input.TransferValues = map[identifiers.AssetID]*big.Int{
					common.KMOITokenAssetID: big.NewInt(1),
				}
			},
		}
	}

	testcases := []struct {
		name              string
		ix                *common.Interaction
		preTestFn         func(ixPool *IxPool)
		expectedGaugeSize uint64
		replaceInPromote  bool
		expectedErr       error
	}{
		{
			name: "successfully replace ixn in enqueue",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)))
				require.NoError(t, err)
			},
			expectedGaugeSize: 4,
		},
		{
			name: "successfully replace ixn in promoted queue",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)))
				require.NoError(t, err)
				ixPool.handlePromoteRequest(<-ixPool.promoteReqCh)
			},
			replaceInPromote:  true,
			expectedGaugeSize: 4,
		},
		{
			name: "successfully replace with lower size ixn",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(
					tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload)),
				)
				require.NoError(t, err)
			},
			expectedGaugeSize: 1,
		},
		{
			name: "failed to replace with cheaper ixn",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, 0, nil)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)))
				require.NoError(t, err)
			},
			expectedErr: common.ErrUnderpriced,
		},
		{
			name: "failed to replace with same ixn",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)))
				require.NoError(t, err)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "failed to replace with ixn which will overflow ixpool",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, nil)))
				require.NoError(t, err)

				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots - 1)
			},
			expectedErr: common.ErrIXPoolOverFlow,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sender := test.ix.Sender()
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixPool)
			}

			err := ixPool.validateAndEnqueueIx(test.ix)

			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)

			// check if ixn got replaced
			acc := ixPool.accounts.get(sender)
			require.Equal(t, 1, len(ixPool.allIxs.all))
			require.Equal(t, test.expectedGaugeSize, ixPool.gauge.read())

			require.Equal(t, 1, len(acc.nonceToIX.mapping))
			ixInNonceMap := acc.nonceToIX.get(test.ix.Nonce())
			require.Equal(t, test.ix, ixInNonceMap)

			if test.replaceInPromote {
				require.Equal(t, 1, int(acc.promoted.length()))
				promotedIx := acc.promoted.peek()
				require.Equal(t, test.ix, promotedIx)

				return
			}

			require.Equal(t, 1, int(acc.enqueued.length()))
			enqueuedIx := acc.enqueued.peek()
			require.Equal(t, test.ix, enqueuedIx)
		})
	}
}

func TestIxPool_validateAndEnqueueIx(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(ixPool *IxPool, interaction *common.Interaction)
		expectedErr error
	}{
		{
			name:        "Interaction with invalid address",
			ix:          newIxWithoutAddress(t, 0),
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Already known Interaction",
			ix:   newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				err := ixPool.validateAndEnqueueIx(interaction)
				require.NoError(t, err)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "New valid interaction",
			ix:   newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil),
		},
		{
			name: "ixpool overflow",
			ix:   newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots)
			},
			expectedErr: common.ErrIXPoolOverFlow,
		},
		{
			name: "fill ixpool to the limit",
			ix:   newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots - 1)
			},
		},
		{
			name: "failed to add lower nonce ixn",
			ix:   newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.createAccountOnce(addrs[0], 1)
			},
			expectedErr: ErrNonceTooLow,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixPool, test.ix)
			}

			err := ixPool.validateAndEnqueueIx(test.ix)

			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, len(ixPool.allIxs.all))
		})
	}
}

func TestIxPool_addedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name string
		ixn  common.Interactions
	}{
		{
			name: "check for add ixn event",
			ixn:  common.Interactions{newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil)},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			ixAddedEventSub := ixPool.mux.Subscribe(utils.AddedInteractionEvent{})

			ixAddedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixAddedEventSub, ixAddedResp, len(test.ixn))

			err := ixPool.validateAndEnqueueIx(test.ixn[0])
			require.NoError(t, err)

			data := tests.WaitForResponse(t, ixAddedResp, utils.AddedInteractionEvent{})
			event, ok := data.(utils.AddedInteractionEvent)
			require.True(t, ok)
			require.Equal(t, test.ixn, event.Ixs)
		})
	}
}

func TestIxPool_validateAndEnqueueIx_HighPressure(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name                  string
		ix                    *common.Interaction
		preTestFn             func(ixPool *IxPool, interaction *common.Interaction)
		expectedEnqueueLength uint64
		expectedError         error
	}{
		{
			name: "prune should be signalled when ixpool overflows",
			ix:   newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase((80 * config.DefaultMaxIXPoolSlots / 100) + 1)
			},
			expectedEnqueueLength: 1,
		},
		{
			name: "reject the future ix",
			ix:   newTestInteraction(t, common.IxValueTransfer, 1, addrs[0], nil),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.createAccountOnce(interaction.Sender(), 0)
				ixPool.gauge.increase((80 * config.DefaultMaxIXPoolSlots / 100) + 1)
			},
			expectedError: common.ErrRejectFutureIx,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sender := test.ix.Sender()
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixPool, test.ix)
			}

			var (
				err  error
				g, _ = errgroup.WithContext(context.Background()) // use err group to get error from go routine
			)

			g.Go(
				func() error {
					return ixPool.validateAndEnqueueIx(test.ix)
				})

			_, ok := <-ixPool.pruneCh
			require.True(t, ok)

			err = g.Wait()

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			acc := ixPool.accounts.get(sender)
			acc.enqueued.lock(false)
			defer acc.enqueued.unlock()

			require.Equal(t, test.expectedEnqueueLength, acc.enqueued.length())
		})
	}
}

func TestIxPool_handleEnqueueRequest(t *testing.T) {
	address := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setBalance(address, common.KMOITokenAssetID, big.NewInt(1000))

	testcases := []struct {
		name           string
		ixs            common.Interactions
		testFn         func(ixPool *IxPool, interactions common.Interactions)
		expectedResult expectedResult
		expectedErrors int
	}{
		{
			name: "Enqueue ixs with higher nonce",
			ixs: common.Interactions{
				newTestInteraction(t, common.IxValueTransfer, 0, address, nil),
				newTestInteraction(t, common.IxValueTransfer, 5, address, nil),
			},
			expectedResult: expectedResult{
				enqueued:         2,
				promotedAccounts: 1,
			},
		},
		{
			name: "All the ixs are with low nonce",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 5, address),
			testFn: func(ixPool *IxPool, interactions common.Interactions) {
				ixPool.createAccountOnce(interactions[0].Sender(), 5)
			},
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 0,
			},
			expectedErrors: 5,
		},
		{
			name: "Should not enqueue ixs with low nonce",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 6, address),
			testFn: func(ixPool *IxPool, interactions common.Interactions) {
				ixPool.createAccountOnce(interactions[0].Sender(), 5)
			},
			expectedResult: expectedResult{
				enqueued:         1,
				promotedAccounts: 1,
			},
			expectedErrors: 5,
		},
		{
			name: "Promote ixs with expected nonce",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 3, address),
			expectedResult: expectedResult{
				enqueued:         3,
				promotedAccounts: 1,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)
			senderAddress := testcase.ixs[0].Sender()

			if testcase.testFn != nil {
				testcase.testFn(ixPool, testcase.ixs)
			}

			require.Equal(t, uint64(0), ixPool.gauge.read())
			promotedAccounts := getPromotedAccounts(t,
				ixPool,
				testcase.ixs,
				testcase.expectedResult.promotedAccounts,
				testcase.expectedErrors,
			)

			require.Equal(t, testcase.expectedResult.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expectedResult.promotedAccounts, len(promotedAccounts))
			require.Equal(t, testcase.expectedResult.enqueued+testcase.expectedResult.promoted, ixPool.gauge.read())
		})
	}
}

func TestIxPool_handlePromoteRequest(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil)

	testcases := []struct {
		name     string
		ixs      common.Interactions
		popIx    func(address identifiers.Address)
		expected expectedResult
	}{
		{
			name: "Promote one ix",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 1, addrs[0]),
			expected: expectedResult{
				nonce:    1,
				enqueued: 0,
				promoted: 1,
			},
		},
		{
			name: "Promote several ixs",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 3, addrs[1]),
			expected: expectedResult{
				nonce:    3,
				enqueued: 0,
				promoted: 3,
			},
		},
		{
			name: "Should not promote if the enqueue is empty",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 1, addrs[2]),
			popIx: func(address identifiers.Address) {
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

			errs := ixPool.AddInteractions(testcase.ixs)
			require.Len(t, errs, 0)

			if testcase.popIx != nil {
				testcase.popIx(senderAddress)
			}

			ixPool.handlePromoteRequest(<-ixPool.promoteReqCh)

			// checks whether the account's nonce is updated
			require.Equal(t, testcase.expected.nonce, ixPool.accounts.get(senderAddress).getNonce())
			// checks whether the ixs are removed from the enqueue
			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			// checks whether the ixs are promoted
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_promoteIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name string
		ixs  common.Interactions
	}{
		{
			name: "check for promoted ixn event",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 3, addrs[0]),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			ixPromotedEventSub := ixPool.mux.Subscribe(utils.PromotedInteractionEvent{})

			ixPromotedResp := make(chan tests.Result, len(testcase.ixs))

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPromotedEventSub, ixPromotedResp, len(testcase.ixs))

			addAndProcessIxs(t, sm, ixPool, testcase.ixs)

			data := tests.WaitForResponse(t, ixPromotedResp, utils.PromotedInteractionEvent{})
			event, ok := data.(utils.PromotedInteractionEvent)
			require.True(t, ok)
			require.Equal(t, testcase.ixs, event.Ixs)
		})
	}
}

func TestIxPool_createAccountOnce(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, true, sm, nil)

	testcases := []struct {
		name          string
		address       identifiers.Address
		nonce         uint64
		testFn        func(addr identifiers.Address)
		expectedNonce uint64
	}{
		{
			name:    "Create an account with latest state object nonce",
			address: tests.RandomAddress(t),
			nonce:   1,
			testFn: func(addr identifiers.Address) {
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
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil)

	testcases := []struct {
		name               string
		ixs                common.Interactions
		nonce              int
		incrementCounter   func(acc *account)
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the interactions with low nonce",
			ixs:                createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "Prune some interactions with low nonce",
			ixs:                createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[1]),
			nonce:              1,
			expectedPromotions: 3,
		},
		{
			name:  "Reset wait time",
			ixs:   createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[2]),
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
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name             string
		ixs              common.Interactions
		nonce            uint64
		promote          bool
		expectedEnqueued uint64
	}{
		{
			name:             "Prune all ixs with low nonce",
			ixs:              createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[0]),
			nonce:            5,
			expectedEnqueued: 0,
		},
		{
			name:             "No low nonce ixs to prune",
			ixs:              createTestIxs(t, common.IxValueTransfer, 0, 6, addrs[1])[2:6],
			nonce:            1,
			expectedEnqueued: 4,
		},
		{
			name:             "Prune some ixs with low nonce",
			ixs:              createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[2]),
			nonce:            3,
			promote:          true,
			expectedEnqueued: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			errs := ixPool.AddInteractions(testcase.ixs)
			require.Len(t, errs, 0)

			// On reset should prune the ixs from the enqueue if the nonce is lesser than the given nonce
			go ixPool.resetAccount(senderAddress, testcase.nonce)

			time.Sleep(100 * time.Millisecond)

			if testcase.promote {
				<-ixPool.promoteReqCh // receive promote signal from add interactions
				<-ixPool.promoteReqCh // receive promote signal from reset account
			}

			ixPool.accounts.get(senderAddress).enqueued.lock(false)
			defer ixPool.accounts.get(senderAddress).enqueued.unlock()

			require.Equal(t, testcase.expectedEnqueued, ixPool.accounts.get(senderAddress).enqueued.length())
		})
	}
}

func TestIxPool_prunedEnqueuedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name             string
		ixs              common.Interactions
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some enqueue ixs with low nonce and check for events",
			ixs:              createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[1]),
			nonce:            3,
			expectEventCount: 3,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)
			senderAddress := testcase.ixs[0].Sender()

			ixPrunedEnqueueEventSub := ixPool.mux.Subscribe(utils.PrunedEnqueuedInteractionEvent{})

			ixPrunedEnqueueResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedEnqueueEventSub, ixPrunedEnqueueResp, len(testcase.ixs))

			addAndEnqueueIxsWithoutPromoting(t, ixPool, testcase.ixs, senderAddress)

			// listen to promote request channel to avoid block in ixPool.resetAccount
			go func() {
				for range testcase.ixs {
					<-ixPool.promoteReqCh
				}
			}()

			// resetAccount should prune the ixs from the enqueue if the nonce is lesser than the given nonce
			ixPool.resetAccount(senderAddress, testcase.nonce)

			data := tests.WaitForResponse(t, ixPrunedEnqueueResp, utils.PrunedEnqueuedInteractionEvent{})
			event, ok := data.(utils.PrunedEnqueuedInteractionEvent)
			require.True(t, ok)

			for i := 0; i < testcase.expectEventCount; i++ {
				require.Equal(t, testcase.ixs[i], event.Ixs[i])
			}
		})
	}
}

func TestIxPool_resetAccount_promoted(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil)

	testcases := []struct {
		name               string
		ixs                common.Interactions
		nonce              uint64
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the ixs with low nonce",
			ixs:                createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "No low nonce ixs to prune",
			ixs:                createTestIxs(t, common.IxValueTransfer, 0, 6, addrs[1])[1:6],
			nonce:              0,
			expectedPromotions: 5,
		},
		{
			name:               "Prune some ixs with low nonce",
			ixs:                createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[2]),
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

func TestIxPool_prunedPromotedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(addrs...)

	testcases := []struct {
		name             string
		ixs              common.Interactions
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some promoted ixs with low nonce and check for events",
			ixs:              createTestIxs(t, common.IxValueTransfer, 0, 5, addrs[1]),
			nonce:            3,
			expectEventCount: 3,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			ixPrunedPromotedEventSub := ixPool.mux.Subscribe(utils.PrunedPromotedInteractionEvent{})

			ixPrunedPromotedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedPromotedEventSub, ixPrunedPromotedResp, len(testcase.ixs))

			senderAddress := testcase.ixs[0].Sender()
			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			// resetAccount should prune the ixs from the promoted queue if the nonce is lesser than the given nonce
			ixPool.resetAccount(senderAddress, testcase.nonce)

			data := tests.WaitForResponse(t, ixPrunedPromotedResp, utils.PrunedPromotedInteractionEvent{})
			event, ok := data.(utils.PrunedPromotedInteractionEvent)
			require.True(t, ok)

			for i := 0; i < testcase.expectEventCount; i++ {
				require.Equal(t, testcase.ixs[i], event.Ixs[i])
			}
		})
	}
}

func TestIxPool_resetAccount(t *testing.T) {
	address := tests.RandomAddress(t)

	testcases := []struct {
		name     string
		ixs      common.Interactions
		nonce    uint64
		promote  bool
		signal   bool
		expected expectedResult
	}{
		{
			name: "Prune all ixs with low nonce",
			ixs: append(
				// promoted
				createTestIxs(t, common.IxValueTransfer, 0, 3, address),
				// enqueued
				createTestIxs(t, common.IxValueTransfer, 4, 7, address)...,
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
				createTestIxs(t, common.IxValueTransfer, 6, 8, address),
				// enqueued
				createTestIxs(t, common.IxValueTransfer, 10, 13, address)...,
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
				createTestIxs(t, common.IxValueTransfer, 0, 3, address),
				// enqueued
				createTestIxs(t, common.IxValueTransfer, 4, 7, address)...,
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
				createTestIxs(t, common.IxValueTransfer, 0, 3, address),
				// enqueued
				createTestIxs(t, common.IxValueTransfer, 4, 7, address)...,
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
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			senderAddress := testcase.ixs[0].Sender()

			require.Equal(t, uint64(0), ixPool.gauge.read())
			addAndProcessIxs(t, sm, ixPool, testcase.ixs)
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			acc := ixPool.accounts.get(senderAddress)
			require.Equal(t, len(testcase.ixs), len(acc.nonceToIX.mapping))

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

			acc = ixPool.accounts.get(senderAddress)
			require.Equal(t, testcase.expected.enqueued, acc.enqueued.length())
			require.Equal(t, testcase.expected.promoted, acc.promoted.length())
			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, uint64(len(acc.nonceToIX.mapping)))
		})
	}
}

func TestIxPool_Pop(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(addr1)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil)

	testcases := []struct {
		name               string
		ixs                common.Interactions
		expectedPromotions uint64
	}{
		{
			name:               "Prune the ix from the promoted queue",
			ixs:                createTestIxs(t, common.IxValueTransfer, 0, 5, addr1),
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

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil)

	testcases := []struct {
		name string
		ixs  common.Interactions
	}{
		{
			name: "Remove the account form accounts map and check for dropped events",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 5, addr1),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixDroppedEventSub := ixPool.mux.Subscribe(utils.DroppedInteractionEvent{})

			ixDroppedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixDroppedEventSub, ixDroppedResp, len(testcase.ixs))

			senderAddress := testcase.ixs[0].Sender()
			addAndPromoteIxs(t, ixPool, testcase.ixs, senderAddress)

			acc := ixPool.accounts.get(senderAddress)
			ix := acc.promoted.peek()
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.ixs)), uint64(len(acc.nonceToIX.mapping)))

			ixPool.Drop(ix)
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.get(senderAddress).promoted.length())

			data := tests.WaitForResponse(t, ixDroppedResp, utils.DroppedInteractionEvent{})
			event, ok := data.(utils.DroppedInteractionEvent)
			require.True(t, ok)
			require.Equal(t, testcase.ixs, event.Ixs)

			require.Equal(t, 0, len(acc.nonceToIX.mapping))
		})
	}
}

func TestIxPool_IncrementWaitTime_InvalidAccount(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil)

	err := ixPool.IncrementWaitTime(identifiers.Address{0x0}, 1500*time.Millisecond)
	require.Error(t, err)
}

func TestIxPool_IncrementWaitTime(t *testing.T) {
	testcases := []struct {
		name            string
		addr            identifiers.Address
		delta           int
		shouldReset     bool
		expectedCounter int32
	}{
		{
			name:            "Increment the wait counter by 1",
			addr:            identifiers.Address{0x01},
			delta:           1,
			shouldReset:     false,
			expectedCounter: 1,
		},
		{
			name:            "Increment the wait counter by 5",
			addr:            identifiers.Address{0x02},
			delta:           5,
			shouldReset:     false,
			expectedCounter: 5,
		},

		{
			name:            "Wait counter greater than max value",
			addr:            identifiers.Address{0x03},
			delta:           MaxWaitCounter + 1,
			shouldReset:     true,
			expectedCounter: 0,
		},
	}

	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil)

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
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, true, sm, nil)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name:        "Oversized data error",
			ix:          newIxWithPayload(t, common.IxValueTransfer, 5, addrs[0], make([]byte, ixMaxSize+2)),
			expectedErr: ErrOversizedData,
		},
		{
			name:        "Invalid address error",
			ix:          newIxWithoutAddress(t, 5),
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Nonce too low error",
			ix:   newTestInteraction(t, common.IxValueTransfer, 9, addrs[1], nil),
			testFn: func(interaction *common.Interaction) {
				sm.setLatestNonce(interaction.Sender(), 10)
			},
			expectedErr: ErrNonceTooLow,
		},
		{
			name:        "Underpriced error",
			ix:          newIxWithFuelPrice(t, 5, addrs[0], 50),
			expectedErr: common.ErrUnderpriced,
		},
		{
			name: "Ix with negative transfer value",
			ix: newTestInteraction(t, common.IxValueTransfer, 0, addrs[2], func(ixData *common.IxData) {
				ixData.Input.Type = common.IxValueTransfer
				ixData.Input.TransferValues = map[identifiers.AssetID]*big.Int{
					"assetID1": new(big.Int).Neg(big.NewInt(20)),
				}
			}),
			expectedErr: common.ErrInvalidValue,
		},
		{
			name: "Ix with invalid assetID",
			ix: newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], func(ixData *common.IxData) {
				ixData.Input.Type = common.IxValueTransfer
				ixData.Input.TransferValues = map[identifiers.AssetID]*big.Int{
					"assetID1": big.NewInt(20),
				}
			}),
			expectedErr: common.ErrFetchingBalance,
		},
		{
			name: "Ix with insufficient funds",
			ix: newTestInteraction(t, common.IxValueTransfer, 0, addrs[0], func(ixData *common.IxData) {
				ixData.Input.Type = common.IxValueTransfer
				ixData.Input.TransferValues = map[identifiers.AssetID]*big.Int{
					"assetID1": big.NewInt(20),
				}
			}),
			testFn: func(interaction *common.Interaction) {
				sm.balance[interaction.Sender()] = map[identifiers.AssetID]*big.Int{
					"assetID1": big.NewInt(10),
				}
			},
			expectedErr: common.ErrInsufficientFunds,
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
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil)

	address, mnemonic := tests.RandomAddressWithMnemonic(t)
	addr2 := tests.RandomAddress(t)

	sm.setTestMOIBalance(address, addr2)

	ixArgs := common.SendIXArgs{
		Sender:    address,
		Type:      common.IxValueTransfer,
		FuelPrice: defaultIxPriceLimit,
		FuelLimit: 1,
		TransferValues: map[identifiers.AssetID]*big.Int{
			"assetID1": big.NewInt(5),
		},
	}

	rawSign := tests.GetIXSignature(t, &ixArgs, mnemonic)

	ix := tests.CreateIX(t, getIXParams(
		address,
		common.IxValueTransfer,
		defaultIxPriceLimit,
		map[identifiers.AssetID]*big.Int{
			"assetID1": big.NewInt(5),
		},
		rawSign,
	))

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "invalid signature",
			ix: newTestInteraction(t, common.IxValueTransfer, 0, addr2, func(ixData *common.IxData) {
				ixData.Input.TransferValues = map[identifiers.AssetID]*big.Int{
					"assetID1": big.NewInt(5),
				}
			}),
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(interaction.Sender(), "assetID1", big.NewInt(10))
			},
			expectedErr: common.ErrInvalidIXSignature,
		},
		{
			name: "valid signature",
			ix:   ix,
			testFn: func(interaction *common.Interaction) {
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
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, nil, nil)

	address := tests.RandomAddress(t)
	validAssetPayload := common.AssetCreatePayload{
		Standard: common.MAS1,
		Supply:   defaultIxPriceLimit,
	}

	invalidAssetStandardPayload := common.AssetCreatePayload{
		Standard: 2,
	}

	invalidAssetSupplyPayload := common.AssetCreatePayload{
		Standard: common.MAS1,
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
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name: "should return error if asset standard is invalid",
			ix: newTestInteraction(t, common.IxAssetCreate, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawInValidAssetStandardPayload
			}),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name: "should return error if asset supply is invalid",
			ix: newTestInteraction(t, common.IxAssetCreate, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawInvalidAssetSupplyPayload
			}),
			expectedErr: common.ErrInvalidAssetSupply,
		},
		{
			name: "should return success if asset standard is valid",
			ix: newTestInteraction(t, common.IxAssetCreate, 0, address, func(ixData *common.IxData) {
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
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil)

	address := tests.RandomAddress(t)
	assetID := tests.GetRandomAssetID(t, address)
	assetPayload := common.AssetMintOrBurnPayload{
		Asset: assetID,
	}

	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	NFTAssetPayload := common.AssetMintOrBurnPayload{
		Asset: identifiers.NewAssetIDv0(false, false, 1, uint16(common.MAS1), address),
	}

	rawNFTAssetPayload, err := NFTAssetPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "should return error if asset not found",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			expectedErr: common.ErrAssetNotFound,
		},
		{
			name: "should return error if operator address mismatch",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *common.Interaction) {
				sm.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: tests.RandomAddress(t),
				})
			},
			expectedErr: common.ErrOperatorMismatch,
		},
		{
			name: "should return success if asset mint data is valid",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *common.Interaction) {
				sm.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
		},
		{
			name: "should return error if non fungible token minted",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawNFTAssetPayload
			}),
			testFn: func(interaction *common.Interaction) {
				sm.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
			expectedErr: common.ErrMintNonFungibleToken,
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
	assetPayload := common.AssetMintOrBurnPayload{
		Asset:  assetID,
		Amount: big.NewInt(100),
	}

	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction, msm *MockStateManager)
		expectedErr error
	}{
		{
			name: "asset not found",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			expectedErr: common.ErrAssetNotFound,
		},
		{
			name: "balance not found",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *common.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
			expectedErr: common.ErrFetchingBalance,
		},
		{
			name: "insufficient funds",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *common.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.balance[interaction.Sender()] = map[identifiers.AssetID]*big.Int{
					assetID: big.NewInt(10),
				}
				mockStateManager.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
			expectedErr: common.ErrInsufficientFunds,
		},
		{
			name: "operator address mismatch",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *common.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.balance[interaction.Sender()] = map[identifiers.AssetID]*big.Int{
					assetID: big.NewInt(1000),
				}
				mockStateManager.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: tests.RandomAddress(t),
				})
			},
			expectedErr: common.ErrOperatorMismatch,
		},
		{
			name: "valid asset burn data",
			ix: newTestInteraction(t, common.IxAssetMint, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawAssetPayload
			}),
			testFn: func(interaction *common.Interaction, mockStateManager *MockStateManager) {
				mockStateManager.balance[interaction.Sender()] = map[identifiers.AssetID]*big.Int{
					assetID: big.NewInt(1000),
				}
				mockStateManager.setAssetInfo(assetID, &common.AssetDescriptor{
					Operator: interaction.Sender(),
				})
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
			}, false, sm, nil)

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
	invalidLogicPayload := common.LogicPayload{}
	validLogicPayload := common.LogicPayload{
		Manifest: []byte{1, 2},
	}

	invalidRawLogicPayload, err := invalidLogicPayload.Bytes()
	require.NoError(t, err)

	validRawLogicPayload, err := validLogicPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		setHook     func(c *MockExecutionManager)
		expectedErr error
	}{
		{
			name: "should return error if manifest is empty",
			ix: newTestInteraction(t, common.IxLogicDeploy, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = invalidRawLogicPayload
			}),
			expectedErr: common.ErrEmptyManifest,
		},
		{
			name: "should return success if logic payload is valid",
			ix: newTestInteraction(t, common.IxLogicDeploy, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = validRawLogicPayload
			}),
		},
		{
			name: "should return error if callsite is invalid",
			ix: newTestInteraction(t, common.IxLogicDeploy, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = validRawLogicPayload
			}),
			setHook: func(exec *MockExecutionManager) {
				exec.validateLogicDeployHook = func() error {
					return errors.New("invalid callsite")
				}
			},
			expectedErr: errors.New("failed to validate logic deploy"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			exec := NewMockExecutionManager(t)
			if testcase.setHook != nil {
				testcase.setHook(exec)
			}

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
			}, false, sm, exec)

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
	payloadWithoutCallsite := common.LogicPayload{}
	payloadWithoutLogicID := common.LogicPayload{
		Callsite: "seeder!",
	}
	validLogicPayload := common.LogicPayload{
		Logic:    "logicID-1",
		Callsite: "seeder!",
	}

	rawPayloadWithoutCallsite, err := payloadWithoutCallsite.Bytes()
	require.NoError(t, err)

	rawPayloadWithoutLogicID, err := payloadWithoutLogicID.Bytes()
	require.NoError(t, err)

	validRawLogicPayload, err := validLogicPayload.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction, msm *MockStateManager)
		setHook     func(c *MockExecutionManager)
		expectedErr error
	}{
		{
			name: "should return error if call site is empty",
			ix: newTestInteraction(t, common.IxLogicInvoke, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawPayloadWithoutCallsite
			}),
			expectedErr: common.ErrEmptyCallSite,
		},
		{
			name: "should return error if logicID is empty",
			ix: newTestInteraction(t, common.IxLogicInvoke, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = rawPayloadWithoutLogicID
			}),
			expectedErr: common.ErrMissingLogicID,
		},
		{
			name: "should return success if logic payload is valid",
			ix: newTestInteraction(t, common.IxLogicInvoke, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = validRawLogicPayload
			}),
			testFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID("logicID-1")
			},
		},
		{
			name: "should return error if callsite is invalid",
			ix: newTestInteraction(t, common.IxLogicInvoke, 0, address, func(ixData *common.IxData) {
				ixData.Input.Payload = validRawLogicPayload
			}),
			setHook: func(exec *MockExecutionManager) {
				exec.validateLogicInvokeHook = func() error {
					return errors.New("invalid callsite")
				}
			},
			expectedErr: errors.New("failed to validate logic invoke"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			exec := NewMockExecutionManager(t)
			if testcase.setHook != nil {
				testcase.setHook(exec)
			}

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
			}, false, sm, exec)

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
	accounts      map[identifiers.Address]common.Interactions
	delayCounters map[identifiers.Address]int32
)

func TestIxPool_Executables_Wait_Mode(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 4)

	testcases := []struct {
		name               string
		accounts           accounts
		delayCounters      delayCounters
		updateDelayCounter func(ixPool *IxPool, delayCounters delayCounters)
		expectedAddrList   []identifiers.Address
	}{
		{
			name: "One ix per account",
			accounts: map[identifiers.Address]common.Interactions{
				addresses[0]: createTestIxs(t, common.IxValueTransfer, 0, 1, addresses[0]),
				addresses[1]: createTestIxs(t, common.IxValueTransfer, 0, 1, addresses[1]),
				addresses[2]: createTestIxs(t, common.IxValueTransfer, 0, 1, addresses[2]),
				addresses[3]: createTestIxs(t, common.IxValueTransfer, 0, 1, addresses[3]),
			},
			delayCounters: map[identifiers.Address]int32{
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
			expectedAddrList: []identifiers.Address{
				addresses[1],
				addresses[0],
				addresses[3],
				addresses[2],
			},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[identifiers.Address]common.Interactions{
				addresses[0]: createTestIxs(t, common.IxValueTransfer, 0, 2, addresses[0]),
				addresses[1]: createTestIxs(t, common.IxValueTransfer, 0, 2, addresses[1]),
				addresses[2]: createTestIxs(t, common.IxValueTransfer, 0, 2, addresses[2]),
				addresses[3]: createTestIxs(t, common.IxValueTransfer, 0, 2, addresses[3]),
			},
			delayCounters: map[identifiers.Address]int32{
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
			expectedAddrList: []identifiers.Address{
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
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

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
				c.PriceLimit = defaultIxPriceLimit
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
		accounts        map[identifiers.Address]common.Interactions
		accountWaitTime map[identifiers.Address]time.Time
		updateWaitTime  func(ixPool *IxPool, accountWaitTime map[identifiers.Address]time.Time)
		expectedNonce   map[identifiers.Address]uint64
	}{
		{
			name: "One ix per account",
			accounts: map[identifiers.Address]common.Interactions{
				addresses[0]: createTestIxs(t, common.IxValueTransfer, 7, 8, addresses[0]),
				addresses[1]: createTestIxs(t, common.IxValueTransfer, 8, 9, addresses[1]),
				addresses[2]: createTestIxs(t, common.IxValueTransfer, 5, 6, addresses[2]),
				addresses[3]: createTestIxs(t, common.IxValueTransfer, 6, 7, addresses[3]),
			},
			accountWaitTime: map[identifiers.Address]time.Time{
				addresses[0]: time.Now().Add(1000 * time.Millisecond),
				addresses[1]: time.Now().Add(-100),
				addresses[2]: time.Now().Add(-200),
				addresses[3]: time.Now().Add(-150),
			},
			updateWaitTime: func(ixPool *IxPool, accountWaitTime map[identifiers.Address]time.Time) {
				for addr := range accountWaitTime {
					acc := ixPool.accounts.get(addr)
					acc.waitTime = accountWaitTime[addr]
				}
			},
			expectedNonce: map[identifiers.Address]uint64{
				addresses[1]: 8,
				addresses[2]: 5,
				addresses[3]: 6,
			},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[identifiers.Address]common.Interactions{
				addresses[0]: createTestIxs(t, common.IxValueTransfer, 4, 6, addresses[0]),
				addresses[1]: createTestIxs(t, common.IxValueTransfer, 5, 7, addresses[1]),
				addresses[2]: createTestIxs(t, common.IxValueTransfer, 6, 8, addresses[2]),
				addresses[3]: createTestIxs(t, common.IxValueTransfer, 7, 9, addresses[3]),
			},
			accountWaitTime: map[identifiers.Address]time.Time{
				addresses[0]: time.Now().Add(-200),
				addresses[1]: time.Now().Add(-100),
				addresses[2]: time.Now().Add(1000 * time.Millisecond),
				addresses[3]: time.Now().Add(-150),
			},
			updateWaitTime: func(ixPool *IxPool, accountWaitTime map[identifiers.Address]time.Time) {
				for addr := range accountWaitTime {
					acc := ixPool.accounts.get(addr)
					acc.waitTime = accountWaitTime[addr]
				}
			},
			expectedNonce: map[identifiers.Address]uint64{
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
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

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
		ixs                 common.Interactions
		ixPoolCallback      func(i *IxPool)
		expectedEnqueuedIxs uint64
	}{
		{
			name: "accounts without nonce holes",
			ixs:  createTestIxs(t, common.IxValueTransfer, 0, 5, addr[0]),
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[0], 0)
			},
			expectedEnqueuedIxs: 5,
		},
		{
			name: "accounts with nonce holes",
			ixs:  createTestIxs(t, common.IxValueTransfer, 2, 8, addr[1]),
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[1], 0)
			},
			expectedEnqueuedIxs: 0,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			if test.ixPoolCallback != nil {
				test.ixPoolCallback(ixPool)
			}

			senderAddress := test.ixs[0].Sender()

			// make sure gauge is zero initially
			require.Equal(t, uint64(0), ixPool.gauge.read())

			addAndEnqueueIxsWithoutPromoting(t, ixPool, test.ixs, senderAddress)

			// make sure gauge is increased after ixns enqueued
			require.Equal(t, slotsRequired(test.ixs...), ixPool.gauge.read())

			acc := ixPool.accounts.get(test.ixs[0].Sender())
			require.Equal(t, len(test.ixs), len(acc.nonceToIX.mapping))

			ixPool.removeNonceHoleAccounts()

			// make sure gauge decreased
			require.Equal(t, test.expectedEnqueuedIxs, ixPool.gauge.read())
			require.Equal(t, test.expectedEnqueuedIxs, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, int(test.expectedEnqueuedIxs), len(acc.nonceToIX.mapping))
		})
	}
}

func TestIxPool_RemoveNonceHoleAccounts_WithEmptyEnqueues(t *testing.T) {
	sm := NewMockStateManager(t)
	addr := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(addr...)

	testcases := []struct {
		name           string
		ixs            common.Interactions
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
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

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

package ixpool

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/sarvalabs/go-polo"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/state"

	"github.com/stretchr/testify/require"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
)

var defaultIxPriceLimit = big.NewInt(1)

const (
	contextTimeout = 5 * time.Second
	ixMaxSize      = 128 * 1024 // 128Kb
)

func TestGetNextView(t *testing.T) {
	testcases := []struct {
		name         string
		currentView  uint64
		nodePosition uint64
	}{
		{
			name:         "simple test: current view-0, node position-0",
			currentView:  0,
			nodePosition: 0,
		},
		{
			name:         "simple test: current view-0, node position-1",
			currentView:  0,
			nodePosition: 1,
		},
		{
			name:         "simple test: current view-0, node position-2",
			currentView:  0,
			nodePosition: 2,
		},
		{
			name:         "simple test: current view-0, node position-3",
			currentView:  0,
			nodePosition: 3,
		},
		{
			name:         "roundoff test: current view-1, node position-0",
			currentView:  1,
			nodePosition: 0,
		},
		{
			name:         "roundoff test: current view-7, node position-0",
			currentView:  7,
			nodePosition: 0,
		},
		{
			name:         "roundoff test: current view-7, node position-1",
			currentView:  7,
			nodePosition: 1,
		},
		{
			name:         "roundoff test: current view-7, node position-2",
			currentView:  7,
			nodePosition: 2,
		},
		{
			name:         "roundoff test: current view-7, node position-3",
			currentView:  7,
			nodePosition: 3,
		},
		{
			name:         "roundoff test: current view-7, node position-3",
			currentView:  288497950,
			nodePosition: 0,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := &IxPool{}

			allocatedView := ixPool.getNextView(testcase.currentView, testcase.nodePosition)
			validateAllocatedView(t, allocatedView, testcase.currentView, testcase.nodePosition)
		})
	}
}

func TestAllocateView(t *testing.T) {
	sm := NewMockStateManager(t)
	sm.SetAccountMetaInfo(common.SargaAddress, &common.AccountMetaInfo{
		PositionInContextSet: 3,
	})

	testcases := []struct {
		name          string
		ixns          []*common.Interaction
		currentView   uint64
		shouldPropose bool
		allottedView  uint64
	}{
		{
			name: "this node is not found in context set of leader account",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
					0, tests.RandomAddress(t), nil,
				),
			},
			currentView:   0,
			shouldPropose: false,
			allottedView:  0,
		},
		{
			name: "this node is found in context set of leader account",
			ixns: []*common.Interaction{
				newTestInteraction(t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					0, tests.RandomAddress(t), nil,
				),
				newTestInteraction(t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					0, tests.RandomAddress(t), nil,
				),
			},
			currentView:   4,
			shouldPropose: true,
			allottedView:  7,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := &IxPool{
				sm:     sm,
				logger: hclog.NewNullLogger(),
			}

			ixPool.allocateView(testcase.currentView, testcase.ixns...)

			for _, ixn := range testcase.ixns {
				require.Equal(t, testcase.shouldPropose, ixn.ShouldPropose())

				if !testcase.shouldPropose {
					require.Equal(t, uint64(0), ixn.AllottedView())

					continue
				}

				validateAllocatedView(t, ixn.AllottedView(), testcase.currentView, 3)
			}
		})
	}
}

func TestIxPool_validateAndEnqueueIx_ReplaceIx(t *testing.T) {
	var (
		transferPayload = &common.AssetActionPayload{
			Beneficiary: tests.RandomAddress(t),
			AssetID:     common.KMOITokenAssetID,
			Amount:      big.NewInt(1),
		}

		defaultPayload, _ = transferPayload.Bytes()

		price = 1
		sm    = NewMockStateManager(t)
		addrs = tests.GetAddresses(t, 1)
	)

	sm.SetAccountMetaInfo(addrs[0], &common.AccountMetaInfo{
		PositionInContextSet: 1,
	})
	sm.SetAccountMetaInfo(transferPayload.Beneficiary, &common.AccountMetaInfo{
		PositionInContextSet: 1,
	})

	sm.setTestMOIBalance(t, addrs...)
	sm.registerAccounts(transferPayload.Beneficiary)

	getIxParams := func(
		from identifiers.Address, nonce uint64,
		price int, payload []byte, noOfTxs int,
	) *tests.CreateIxParams {
		return &tests.CreateIxParams{
			IxDataCallback: func(ix *common.IxData) {
				ix.Sender = from
				ix.Nonce = nonce
				ix.FuelPrice = big.NewInt(int64(price))
				ix.FuelLimit = 1
				ix.IxOps = make([]common.IxOpRaw, noOfTxs)
				ix.Funds = []common.IxFund{
					{
						AssetID: transferPayload.AssetID,
						Amount:  transferPayload.Amount,
					},
				}

				ix.Participants = []common.IxParticipant{
					{
						Address:  from,
						LockType: common.MutateLock,
					},
					{
						Address:  transferPayload.Beneficiary,
						LockType: common.MutateLock,
					},
				}

				for i := 0; i < noOfTxs; i++ {
					ix.IxOps[i] = common.IxOpRaw{
						Type:    common.IxAssetTransfer,
						Payload: payload,
					}
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
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 1, price, defaultPayload, 20)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 1, price, defaultPayload, 1)))
				require.NoError(t, err)
			},
			expectedGaugeSize: 4,
		},
		{
			name: "successfully replace ixn in promoted queue",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 20)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 1)))
				require.NoError(t, err)
			},
			replaceInPromote:  true,
			expectedGaugeSize: 4,
		},
		{
			name: "successfully replace with lower size ixn",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 1, price, defaultPayload, 1)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(
					tests.CreateIX(t, getIxParams(addrs[0], 1, price, defaultPayload, 30)),
				)
				require.NoError(t, err)
			},
			expectedGaugeSize: 1,
		},
		{
			name: "failed to replace with cheaper ixn",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, 0, defaultPayload, 1)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 1)))
				require.NoError(t, err)
			},
			expectedErr: common.ErrUnderpriced,
		},
		{
			name: "failed to replace with same ixn",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 1)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 1)))
				require.NoError(t, err)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "failed to replace with ixn which will overflow ixpool",
			ix:   tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 30)),
			preTestFn: func(ixPool *IxPool) {
				err := ixPool.validateAndEnqueueIx(tests.CreateIX(t, getIxParams(addrs[0], 0, price, defaultPayload, 1)))
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
			}, true, sm, nil, nil)

			ixPool.createAccountOnce(sender, 0)

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

				require.True(t, promotedIx.ShouldPropose())

				return
			}

			require.Equal(t, 1, int(acc.enqueued.length()))
			enqueuedIx := acc.enqueued.peek()
			require.Equal(t, test.ix, enqueuedIx)

			require.False(t, enqueuedIx.ShouldPropose())
			require.Equal(t, uint64(0), enqueuedIx.AllottedView())
		})
	}
}

func TestIxPool_validateAndEnqueueIx(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(ixPool *IxPool, interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "Interaction with invalid address",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				interaction.SetSender(identifiers.NilAddress)
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Already known Interaction",
			ix: newTestInteraction(
				t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
				0, addrs[0], nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				err := ixPool.validateAndEnqueueIx(interaction)
				require.NoError(t, err)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "New valid interaction",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], nil,
			),
		},
		{
			name: "ixpool overflow",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots)
			},
			expectedErr: common.ErrIXPoolOverFlow,
		},
		{
			name: "fill ixpool to the limit",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots - 1)
			},
		},
		{
			name: "failed to add lower nonce ixn",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], nil,
			),
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
			}, true, sm, nil, nil)

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

func TestIxPool_AddRemoteInteractions(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs[0])

	testcases := []struct {
		name              string
		ixns              []*common.Interaction
		sigVerification   bool
		preTestFn         func(interactions []*common.Interaction)
		expectedAddedIxns int
		expectedResult    pubsub.ValidationResult
	}{
		{
			name: "Reject interactions with oversized data",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], nil,
				),
				newIxWithPayload(t, common.IxAssetCreate, 5, addrs[0], make([]byte, IxMaxSize+2)),
			},
			expectedAddedIxns: 1,
			expectedResult:    pubsub.ValidationReject,
		},
		{
			name: "Reject interactions with invalid address",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], nil,
				),
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], nil,
				),
			},
			preTestFn: func(interactions []*common.Interaction) {
				interactions[1].SetSender(identifiers.NilAddress)
			},
			expectedAddedIxns: 1,
			expectedResult:    pubsub.ValidationReject,
		},
		{
			name: "Reject interactions with invalid signature",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], nil,
				),
			},
			sigVerification: true,
			expectedResult:  pubsub.ValidationReject,
		},
		{
			name:              "valid ixn group",
			ixns:              createTestIxs(t, 0, 2, addrs[0]),
			expectedAddedIxns: 2,
			expectedResult:    pubsub.ValidationAccept,
		},
		{
			name: "Ignore ixns as more than half of them have errors and count greater than 10",
			ixns: append(
				createTestIxs(t, 0, 5, addrs[0]),
				createTestIxs(t, 0, 6, addrs[1])...,
			),
			expectedAddedIxns: 5,
			expectedResult:    pubsub.ValidationIgnore,
		},
		{
			name: "accept ixns as less than half of them have errors and count greater than 10",
			ixns: append(
				createTestIxs(t, 0, 6, addrs[0]),
				createTestIxs(t, 0, 5, addrs[1])...,
			),
			expectedAddedIxns: 6,
			expectedResult:    pubsub.ValidationAccept,
		},
		{
			name:           "accept ixns if length of ixns less than or equal to 10 and all ixns have errors",
			ixns:           createTestIxs(t, 0, 10, addrs[1]),
			expectedResult: pubsub.ValidationAccept,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, !test.sigVerification, sm, nil, nil)

			if test.preTestFn != nil {
				test.preTestFn(test.ixns)
			}

			res := ixPool.AddRemoteInteractions(test.ixns...)
			require.Equal(t, test.expectedResult, res)
			require.Equal(t, test.expectedAddedIxns, len(ixPool.allIxs.all))
		})
	}
}

func TestIxPool_AddLocalInteractions_Broadcast(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs[0])

	testcases := []struct {
		name             string
		ixns             []*common.Interaction
		enableIxFlooding bool
		preTestFn        func(interactions []*common.Interaction)
		broadcastedIxns  []*common.Interaction
	}{
		{
			name: "broadcast only valid ixn group",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, addrs[1]),
					0, addrs[0], nil,
				),
				newTestInteraction(
					t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
					0, addrs[0], nil,
				),
			},
			preTestFn: func(interactions []*common.Interaction) {
				interactions[1].SetSender(identifiers.NilAddress)
			},
			broadcastedIxns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, addrs[1]),
					0, addrs[0], nil,
				),
			},
		},
		{
			name: "do not broadcast zero ixns",
			ixns: createTestIxs(t, 0, 2, addrs[1]),
		},
		{
			name:             "do not broadcast ixns if flooding enabled",
			enableIxFlooding: true,
			ixns:             createTestIxs(t, 0, 2, addrs[0]),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
				c.EnableIxFlooding = test.enableIxFlooding
			}, true, sm, nil, newMockNetwork(""))

			if test.preTestFn != nil {
				test.preTestFn(test.ixns)
			}

			ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, test.ixns...))

			net, ok := ixPool.network.(*mockNetwork)
			require.True(t, ok)

			if test.broadcastedIxns != nil {
				require.Equal(t, len(test.broadcastedIxns), len(net.broadcasted))

				expected, err := polo.Polorize(test.broadcastedIxns)
				require.NoError(t, err)

				actual, ok := net.broadcasted[config.IxTopic]
				require.True(t, ok)

				require.Equal(t, expected, actual)

				return
			}

			require.Equal(t, 0, len(net.broadcasted))
		})
	}
}

func TestIxPool_IxValidator(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs[0])

	validIxns := createTestIxs(t, 0, 2, addrs[0])
	rawData, err := polo.Polorize(validIxns)
	require.NoError(t, err)

	zeroIxns := []*common.Interaction{}
	zeroIxnsRawData, err := polo.Polorize(zeroIxns)
	require.NoError(t, err)

	excessIxns := createTestIxs(t, 0, 3, addrs[0])
	excessIxnsRawData, err := polo.Polorize(excessIxns)
	require.NoError(t, err)

	KID := tests.RandomKramaID(t, 0)
	PID, err := KID.DecodedPeerID()
	require.NoError(t, err)

	from, err := PID.Marshal()
	require.NoError(t, err)

	testcases := []struct {
		name              string
		msgData           []byte
		from              []byte
		createDuplicate   bool
		expectedAddedIxns int
		shouldCache       bool
		expectedResult    pubsub.ValidationResult
		expectedError     error
	}{
		{
			name:              "accept valid ixn group",
			msgData:           rawData,
			expectedAddedIxns: len(validIxns),
			shouldCache:       true,
			expectedResult:    pubsub.ValidationAccept,
		},
		{
			name:           "accept ixns if original sender is us ",
			msgData:        rawData,
			from:           from,
			expectedResult: pubsub.ValidationAccept,
		},
		{
			name:              "ignore duplicate ixn group",
			msgData:           rawData,
			createDuplicate:   true,
			expectedAddedIxns: len(validIxns),
			shouldCache:       true,
			expectedResult:    pubsub.ValidationIgnore,
		},
		{
			name:           "reject ixns as decoding ixns fails",
			msgData:        []byte{},
			shouldCache:    true,
			expectedResult: pubsub.ValidationReject,
			expectedError:  errors.New("failed to depolorize interactions"),
		},
		{
			name:           "reject zero ixns",
			msgData:        zeroIxnsRawData,
			shouldCache:    true,
			expectedResult: pubsub.ValidationReject,
			expectedError:  errors.New("invalid number of ixns"),
		},
		{
			name:           "reject excess ixns",
			msgData:        excessIxnsRawData,
			shouldCache:    true,
			expectedResult: pubsub.ValidationReject,
			expectedError:  errors.New("invalid number of ixns"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
				c.MaxIxGroupSize = 2
				c.EnableRawIxFiltering = true
			}, true, sm, nil, newMockNetwork(KID))

			msg := &pubsub.Message{
				Message: &pb.Message{
					Data: test.msgData,
					From: test.from,
				},
			}

			if test.createDuplicate {
				_, err := ixPool.IxValidator(context.Background(), "", msg)
				require.NoError(t, err)
			}

			res, err := ixPool.IxValidator(context.Background(), "", msg)

			// check if raw msg cached
			_, exists := ixPool.msgCache.innerCheck(test.msgData)
			if test.shouldCache {
				require.True(t, exists)
			} else {
				require.False(t, exists)
			}

			require.Equal(t, test.expectedResult, res)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAddedIxns, len(ixPool.allIxs.all))
		})
	}
}

func TestIxPool_addedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name string
		ixn  []*common.Interaction
	}{
		{
			name: "check for add ixn event",
			ixn: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], nil,
				),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, nil)

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
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name                   string
		ix                     *common.Interaction
		preTestFn              func(ixPool *IxPool, interaction *common.Interaction)
		expectedPromotedLength uint64
		expectedError          error
	}{
		{
			name: "prune should be signalled when ixpool overflows",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase((80 * config.DefaultMaxIXPoolSlots / 100) + 1)
			},
			expectedPromotedLength: 1,
		},
		{
			name: "reject the future ix",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				1, addrs[0], nil,
			),
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
			}, true, sm, nil, nil)

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
			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

			require.Equal(t, test.expectedPromotedLength, acc.promoted.length())
		})
	}
}

func TestIxPool_handleEnqueueRequest(t *testing.T) {
	address := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setBalance(t, address, common.KMOITokenAssetID, big.NewInt(1000))

	testcases := []struct {
		name           string
		ixs            []*common.Interaction
		testFn         func(ixPool *IxPool, interactions []*common.Interaction)
		expectedResult expectedResult
		expectedErrors int
	}{
		{
			name: "Enqueue ixs with higher nonce",
			ixs: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, address, nil,
				),
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					5, address, nil,
				),
			},
			expectedResult: expectedResult{
				enqueued:         1,
				promotedAccounts: 1,
			},
		},
		{
			name: "All the ixs are with low nonce",
			ixs:  createTestIxs(t, 0, 5, address),
			testFn: func(ixPool *IxPool, interactions []*common.Interaction) {
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
			ixs:  createTestIxs(t, 0, 6, address),
			testFn: func(ixPool *IxPool, interactions []*common.Interaction) {
				ixPool.createAccountOnce(interactions[0].Sender(), 5)
			},
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 1,
			},
			expectedErrors: 5,
		},
		{
			name: "Promote ixs with expected nonce",
			ixs:  createTestIxs(t, 0, 3, address),
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 3,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))
			senderAddress := testcase.ixs[0].Sender()

			if testcase.testFn != nil {
				testcase.testFn(ixPool, testcase.ixs)
			}

			require.Equal(t, uint64(0), ixPool.gauge.read())

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Equal(t, testcase.expectedErrors, len(errs))

			require.Equal(t, testcase.expectedResult.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expectedResult.promotedAccounts, ixPool.accounts.get(senderAddress).promoted.length())
			require.Equal(t, testcase.expectedResult.enqueued+testcase.expectedResult.promotedAccounts, ixPool.gauge.read())
		})
	}
}

func TestIxPool_handlePromoteRequest(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)

	addr, err := identifiers.NewAddressFromHex("0x0000000000000000000000000516a2efe9cd53c3d54e1f9a6e60e9077e9f9384")
	require.NoError(t, err)

	addrs[0] = addr

	sm.setTestMOIBalance(t, addrs...)
	sm.SetAccountMetaInfo(common.SargaAddress, &common.AccountMetaInfo{
		PositionInContextSet: 1,
	})

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil, newMockNetwork(""))
	ixPool.UpdateCurrentView(4)

	testcases := []struct {
		name                  string
		ixs                   []*common.Interaction
		popIx                 func(address identifiers.Address)
		shouldPropose         bool
		expected              expectedResult
		expectedAllocatedView uint64
	}{
		{
			name: "Promote one ix",
			ixs:  createTestIxs(t, 0, 1, addrs[0]),
			expected: expectedResult{
				nonce:    1,
				enqueued: 0,
				promoted: 1,
			},
			shouldPropose:         true,
			expectedAllocatedView: 5,
		},
		{
			name:          "Promote several ixs",
			ixs:           createTestIxs(t, 0, 3, addrs[1]),
			shouldPropose: true,
			expected: expectedResult{
				nonce:    3,
				enqueued: 0,
				promoted: 3,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			if testcase.popIx != nil {
				testcase.popIx(senderAddress)
			}

			// checks whether the account's nonce is updated
			require.Equal(t, testcase.expected.nonce, ixPool.accounts.get(senderAddress).getNonce())
			// checks whether the ixs are removed from the enqueue
			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			// checks whether the ixs are promoted
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.get(senderAddress).promoted.length())

			if !testcase.shouldPropose {
				require.Equal(t, testcase.shouldPropose, testcase.ixs[0].ShouldPropose())

				return
			}

			require.Equal(t, testcase.shouldPropose, testcase.ixs[0].ShouldPropose())

			validateAllocatedView(t, testcase.ixs[0].AllottedView(), 4, 1)
		})
	}
}

func TestIxPool_promoteIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name string
		ixs  []*common.Interaction
	}{
		{
			name: "check for promoted ixn event",
			ixs:  createTestIxs(t, 0, 1, addrs[0]),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPromotedEventSub := ixPool.mux.Subscribe(utils.PromotedInteractionEvent{})

			ixPromotedResp := make(chan tests.Result, len(testcase.ixs))

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPromotedEventSub, ixPromotedResp, len(testcase.ixs))

			addAndProcessIxs(t, sm, ixPool, testcase.ixs...)

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
	}, true, sm, nil, nil)

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
				sm.setLatestNonce(t, addr, 5)
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

func TestIxPool_ResetWithHeaders_resetWaitTime(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(t, addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil, newMockNetwork(""))

	testcases := []struct {
		name               string
		ixs                []*common.Interaction
		nonce              int
		incrementCounter   func(acc *account)
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the interactions with low nonce",
			ixs:                createTestIxs(t, 0, 5, addrs[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "Prune some interactions with low nonce",
			ixs:                createTestIxs(t, 0, 5, addrs[1]),
			nonce:              1,
			expectedPromotions: 3,
		},
		{
			name:  "Reset wait time",
			ixs:   createTestIxs(t, 0, 5, addrs[2]),
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

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

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

func TestIxPool_ResetWithHeaders_enqueued(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectedEnqueued uint64
		expectedPromoted uint64
	}{
		{
			name:             "Prune all ixs with low nonce",
			ixs:              createTestIxs(t, 1, 5, addrs[0]),
			nonce:            5,
			expectedEnqueued: 0,
		},
		{
			name:             "No low nonce ixs to prune",
			ixs:              createTestIxs(t, 1, 7, addrs[1])[2:6],
			nonce:            1,
			expectedEnqueued: 4,
		},
		{
			name:             "Prune some ixs with low nonce",
			ixs:              createTestIxs(t, 1, 6, addrs[2]),
			nonce:            3,
			expectedPromoted: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPool.createAccountOnce(senderAddress, 0)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))

			// On reset should prune the ixs from the enqueue if the nonce is lesser than the given nonce
			ixPool.ResetWithHeaders(ts)

			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

			require.Equal(t, testcase.expectedEnqueued, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, testcase.expectedPromoted, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_prunedEnqueuedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some enqueue ixs with low nonce and check for events",
			ixs:              createTestIxs(t, 1, 6, addrs[1]),
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
			}, true, sm, nil, newMockNetwork(""))
			senderAddress := testcase.ixs[0].Sender()

			ixPool.createAccountOnce(senderAddress, 0)

			ixPrunedEnqueueEventSub := ixPool.mux.Subscribe(utils.PrunedEnqueuedInteractionEvent{})

			ixPrunedEnqueueResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedEnqueueEventSub, ixPrunedEnqueueResp, len(testcase.ixs))

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			// ResetWithHeaders should prune the ixs from the enqueue if the nonce is lesser than the given nonce
			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))
			ixPool.ResetWithHeaders(ts)

			data := tests.WaitForResponse(t, ixPrunedEnqueueResp, utils.PrunedEnqueuedInteractionEvent{})
			event, ok := data.(utils.PrunedEnqueuedInteractionEvent)
			require.True(t, ok)

			for i := 0; i < testcase.expectEventCount; i++ {
				require.Equal(t, testcase.ixs[i], event.Ixs[i])
			}
		})
	}
}

func TestIxPool_ResetWithHeaders_promoted(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(t, addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil, newMockNetwork(""))

	testcases := []struct {
		name               string
		ixs                []*common.Interaction
		nonce              uint64
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the ixs with low nonce",
			ixs:                createTestIxs(t, 0, 5, addrs[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "No low nonce ixs to prune",
			ixs:                createTestIxs(t, 0, 6, addrs[1])[1:6],
			nonce:              0,
			expectedPromotions: 5,
		},
		{
			name:               "Prune some ixs with low nonce",
			ixs:                createTestIxs(t, 0, 5, addrs[2]),
			nonce:              3,
			expectedPromotions: 1,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))
			// On reset should prune the ixs from the promoted queue if the nonce is lesser than the given nonce
			ixPool.ResetWithHeaders(ts)

			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.get(senderAddress).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_prunedPromotedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs...)

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some promoted ixs with low nonce and check for events",
			ixs:              createTestIxs(t, 0, 5, addrs[1]),
			nonce:            3,
			expectEventCount: 4,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPrunedPromotedEventSub := ixPool.mux.Subscribe(utils.PrunedPromotedInteractionEvent{})

			ixPrunedPromotedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedPromotedEventSub, ixPrunedPromotedResp, len(testcase.ixs))

			senderAddress := testcase.ixs[0].Sender()

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))
			ixPool.ResetWithHeaders(ts)

			data := tests.WaitForResponse(t, ixPrunedPromotedResp, utils.PrunedPromotedInteractionEvent{})
			event, ok := data.(utils.PrunedPromotedInteractionEvent)
			require.True(t, ok)

			for i := 0; i < testcase.expectEventCount; i++ {
				require.Equal(t, testcase.ixs[i], event.Ixs[i])
			}
		})
	}
}

func TestIxPool_ResetWithHeaders(t *testing.T) {
	address := tests.RandomAddress(t)

	testcases := []struct {
		name     string
		ixs      []*common.Interaction
		nonce    uint64
		promote  bool
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
			name: "Prune all promoted",
			ixs: append(
				// promoted
				createTestIxs(t, 1, 3, address),
				// enqueued
				createTestIxs(t, 5, 7, address)...,
			),
			nonce:   2,
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
			nonce: 4,
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
			}, true, sm, nil, newMockNetwork(""))

			senderAddress := testcase.ixs[0].Sender()
			// ixPool.createAccountOnce(senderAddress, 0)

			require.Equal(t, uint64(0), ixPool.gauge.read())

			addAndProcessIxs(t, sm, ixPool, testcase.ixs...)
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			acc := ixPool.accounts.get(senderAddress)
			require.Equal(t, len(testcase.ixs), len(acc.nonceToIX.mapping))

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))
			ixPool.ResetWithHeaders(ts)

			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

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
	sm.setTestMOIBalance(t, addr1)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil, newMockNetwork(""))

	testcases := []struct {
		name               string
		ixs                []*common.Interaction
		expectedPromotions uint64
	}{
		{
			name:               "Prune the ix from the promoted queue",
			ixs:                createTestIxs(t, 0, 5, addr1),
			expectedPromotions: 4,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].Sender()

			require.Equal(t, uint64(0), ixPool.gauge.read())

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

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
	sm.setTestMOIBalance(t, addr1)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil, newMockNetwork(""))

	testcases := []struct {
		name string
		ixs  []*common.Interaction
	}{
		{
			name: "Remove the account form accounts map and check for dropped events",
			ixs:  createTestIxs(t, 0, 5, addr1),
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
			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

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

func TestIxPool_Drop_FinalizedIx(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, addr1)

	ixs := createTestIxs(t, 0, 5, addr1)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, nil, newMockNetwork(""))

	testcases := []struct {
		name          string
		finalizedIx   *common.Interaction
		pendingIxs    []*common.Interaction
		preTestFn     func()
		expectedError error
	}{
		{
			name:        "latest state object nonce greater than the dropped ix nonce",
			finalizedIx: ixs[0],
			pendingIxs:  ixs[1:],
			preTestFn: func() {
				sm.setLatestNonce(t, addr1, 1)
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixDroppedEventSub := ixPool.mux.Subscribe(utils.DroppedInteractionEvent{})

			ixDroppedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixDroppedEventSub, ixDroppedResp, len(testcase.pendingIxs))

			senderAddress := testcase.pendingIxs[0].Sender()
			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.pendingIxs...))
			require.Len(t, errs, 0)

			acc := ixPool.accounts.get(senderAddress)
			require.Equal(t, uint64(len(testcase.pendingIxs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.pendingIxs)), uint64(len(acc.nonceToIX.mapping)))

			testcase.preTestFn()

			ixPool.Drop(testcase.finalizedIx)

			require.Equal(t, uint64(len(testcase.pendingIxs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.pendingIxs)), uint64(len(acc.nonceToIX.mapping)))
		})
	}
}

func TestIxPool_IncrementWaitTime_InvalidAccount(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

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
	}, false, sm, nil, nil)

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
	sm.setTestMOIBalance(t, addrs...)

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, true, sm, nil, nil)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name:        "Oversized data error",
			ix:          newIxWithPayload(t, common.IxAssetCreate, 5, addrs[0], make([]byte, IxMaxSize+2)),
			expectedErr: ErrOversizedData,
		},
		{
			name: "Invalid address error",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, tests.RandomAddress(t), nil,
			),
			testFn: func(interaction *common.Interaction) {
				interaction.SetSender(identifiers.NilAddress)
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Nonce too low error",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				9, addrs[1], nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.setLatestNonce(t, interaction.Sender(), 10)
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
			ix: newTestInteraction(
				t, common.IxAssetTransfer,
				common.AssetActionPayload{
					Beneficiary: addrs[1],
					AssetID:     "assetID1",
					Amount:      new(big.Int).Neg(big.NewInt(20)),
				},
				0, addrs[2], nil,
			),
			expectedErr: common.ErrInvalidValue,
		},
		{
			name: "Ix with invalid assetID",
			ix: newTestInteraction(
				t, common.IxAssetTransfer,
				common.AssetActionPayload{
					Beneficiary: addrs[1],
					AssetID:     "assetID1",
					Amount:      big.NewInt(20),
				},
				0, addrs[0], nil),
			expectedErr: common.ErrFetchingBalance,
		},
		{
			name: "Ix with insufficient funds",
			ix: newTestInteraction(
				t, common.IxAssetTransfer,
				common.AssetActionPayload{
					AssetID: "assetID1",
					Amount:  big.NewInt(20),
				},
				0, addrs[0], nil,
			),
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
	}, false, sm, nil, nil)

	address, mnemonic := tests.RandomAddressWithMnemonic(t)
	addr2 := tests.RandomAddress(t)

	sm.setTestMOIBalance(t, address, addr2)

	ixArgs := common.IxData{
		Sender:    address,
		FuelPrice: defaultIxPriceLimit,
		FuelLimit: 1,
		Funds: []common.IxFund{
			{
				AssetID: "assetID1",
				Amount:  big.NewInt(5),
			},
		},
		IxOps: []common.IxOpRaw{
			{
				Type: common.IxAssetTransfer,
				Payload: func() []byte {
					transferPayload := &common.AssetActionPayload{
						Beneficiary: addr2,
						AssetID:     "assetID1",
						Amount:      big.NewInt(5),
					}

					payload, _ := transferPayload.Bytes()

					return payload
				}(),
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  address,
				LockType: common.MutateLock,
			},
			{
				Address:  addr2,
				LockType: common.MutateLock,
			},
		},
	}

	rawSign := tests.GetIXSignature(t, &ixArgs, mnemonic)

	ix := tests.CreateIX(t, getIXParams(
		address,
		common.IxAssetTransfer,
		defaultIxPriceLimit,
		common.AssetActionPayload{
			Beneficiary: addr2,
			AssetID:     "assetID1",
			Amount:      big.NewInt(5),
		},
		rawSign,
	))

	sm.registerAccounts(addr2)
	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "invalid signature",
			ix: newTestInteraction(
				t, common.IxAssetTransfer,
				common.AssetActionPayload{
					AssetID: "assetID1",
					Amount:  big.NewInt(5),
				},
				0, addr2,
				nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(t, interaction.Sender(), "assetID1", big.NewInt(10))
			},
			expectedErr: common.ErrInvalidIXSignature,
		},
		{
			name: "valid signature",
			ix:   ix,
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(t, interaction.Sender(), "assetID1", big.NewInt(10))
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.ix)
			}

			err := ixPool.validateIx(test.ix)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateFunds(t *testing.T) {
	address := tests.RandomAddress(t)
	sm := NewMockStateManager(t)

	sm.setBalance(t, address, common.KMOITokenAssetID, big.NewInt(10))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	testcases := []struct {
		name        string
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name: "negative asset fund",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, address, func(ixData *common.IxData) {
					ixData.Funds = []common.IxFund{
						{
							AssetID: common.KMOITokenAssetID,
							Amount:  new(big.Int).Neg(big.NewInt(20)),
						},
					}
				},
			),
			expectedErr: common.ErrInvalidValue,
		},
		{
			name: "account not registered",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, tests.RandomAddress(t), nil,
			),
			expectedErr: common.ErrFetchingBalance,
		},
		{
			name: "insufficient asset funds",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, address, func(ixData *common.IxData) {
					ixData.Funds = []common.IxFund{
						{
							AssetID: common.KMOITokenAssetID,
							Amount:  big.NewInt(20),
						},
					}
				},
			),
			expectedErr: common.ErrInsufficientFunds,
		},
		{
			name: "valid asset funds",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, address, func(ixData *common.IxData) {
					ixData.Funds = []common.IxFund{
						{
							AssetID: common.KMOITokenAssetID,
							Amount:  big.NewInt(5),
						},
					}
				},
			),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := ixPool.validateFunds(test.ix)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateOperations(t *testing.T) {
	address := tests.RandomAddress(t)
	sm := NewMockStateManager(t)

	sm.setBalance(t, address, common.KMOITokenAssetID, big.NewInt(10))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	validAssetPayload := common.AssetCreatePayload{
		Standard: common.MAS1,
		Supply:   defaultIxPriceLimit,
	}

	invalidAssetPayload := common.AssetCreatePayload{
		Standard: 2,
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(ix *common.Interaction)
		expectedErr error
	}{
		{
			name: "ix with invalid operation type",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, address, nil),
			testFn: func(ix *common.Interaction) {
				ix.GetIxOp(0).OpType = common.IxInvalid
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name:        "ix with invalid operation payload",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetPayload, 0, address, nil),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name: "ix with valid operation payload",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, address, nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.ix)
			}

			err := ixPool.validateOperations(testcase.ix)
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
	}, false, nil, nil, nil)

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

	testcases := []struct {
		name        string
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name:        "should return error if asset standard is invalid",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetStandardPayload, 0, address, nil),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name:        "should return error if asset supply is invalid",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetSupplyPayload, 0, address, nil),
			expectedErr: common.ErrInvalidAssetSupply,
		},
		{
			name: "should return success if asset standard is valid",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, address, nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := ixPool.validateAssetCreate(testcase.ix, 0)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateAssetSupply(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	address := tests.RandomAddress(t)
	assetID := tests.GetRandomAssetID(t, address)
	validAssetPayload := common.AssetSupplyPayload{
		AssetID: assetID,
		Amount:  big.NewInt(100),
	}
	invalidAssetPayload := common.AssetSupplyPayload{
		AssetID: assetID,
		Amount:  big.NewInt(0),
	}
	NFTAssetPayload := common.AssetSupplyPayload{
		AssetID: identifiers.NewAssetIDv0(false, false, 1, uint16(common.MAS1), address),
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name:        "should return error if amount is less than or equal to zero",
			ix:          newTestInteraction(t, common.IxAssetMint, invalidAssetPayload, 0, address, nil),
			expectedErr: common.ErrInvalidValue,
		},
		{
			name:        "should return error if non fungible token minted",
			ix:          newTestInteraction(t, common.IxAssetMint, NFTAssetPayload, 0, address, nil),
			expectedErr: common.ErrMintOrBurnNonFungibleToken,
		},
		{
			name: "valid asset supply payload",
			ix:   newTestInteraction(t, common.IxAssetMint, validAssetPayload, 0, address, nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := ixPool.validateAssetSupply(testcase.ix, 0)
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

	testcases := []struct {
		name        string
		ix          *common.Interaction
		setHook     func(c *MockExecutionManager)
		expectedErr error
	}{
		{
			name:        "should return error if manifest is empty",
			ix:          newTestInteraction(t, common.IxLogicDeploy, invalidLogicPayload, 0, address, nil),
			expectedErr: common.ErrEmptyManifest,
		},
		{
			name: "should return success if logic payload is valid",
			ix:   newTestInteraction(t, common.IxLogicDeploy, validLogicPayload, 0, address, nil),
		},
		{
			name: "should return error if callsite is invalid",
			ix:   newTestInteraction(t, common.IxLogicDeploy, validLogicPayload, 0, address, nil),
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
			}, false, sm, exec, nil)

			err := ixPool.validateLogicDeployPayload(testcase.ix, 0)
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
		Callsite: "seeder",
	}
	validLogicPayload := common.LogicPayload{
		Logic:    "logicID-1",
		Callsite: "seeder",
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(interaction *common.Interaction, msm *MockStateManager)
		setHook     func(c *MockExecutionManager)
		expectedErr error
	}{
		{
			name:        "should return error if call site is empty",
			ix:          newTestInteraction(t, common.IxLogicInvoke, payloadWithoutCallsite, 0, address, nil),
			expectedErr: common.ErrEmptyCallSite,
		},
		{
			name:        "should return error if logicID is empty",
			ix:          newTestInteraction(t, common.IxLogicInvoke, payloadWithoutLogicID, 0, address, nil),
			expectedErr: common.ErrMissingLogicID,
		},
		{
			name: "should return error if receiver object not found",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, address, nil),
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, "logicID-1")
			},
			expectedErr: errors.New("state object not found"),
		},
		{
			name: "should return success if logic payload is valid",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, address, nil),
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, "logicID-1")
				msm.setLatestStateObject(interaction.GetIxOp(0).Target(), &state.Object{})
				msm.setLatestStateObject(interaction.Sender(), &state.Object{})
			},
		},
		{
			name: "should return error if callsite is invalid",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, address, nil),
			setHook: func(exec *MockExecutionManager) {
				exec.validateLogicInvokeHook = func() error {
					return errors.New("invalid callsite")
				}
			},
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, "logicID-1")
				msm.setLatestStateObject(interaction.GetIxOp(0).Target(), &state.Object{})
				msm.setLatestStateObject(interaction.Sender(), &state.Object{})
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
			}, false, sm, exec, nil)

			if testcase.preTestFn != nil {
				testcase.preTestFn(testcase.ix, sm)
			}

			err := ixPool.validateLogicInvokePayload(testcase.ix, 0)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

type (
	accounts      map[identifiers.Address][]*common.Interaction
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
			accounts: map[identifiers.Address][]*common.Interaction{
				addresses[0]: createTestIxs(t, 0, 1, addresses[0]),
				addresses[1]: createTestIxs(t, 0, 1, addresses[1]),
				addresses[2]: createTestIxs(t, 0, 1, addresses[2]),
				addresses[3]: createTestIxs(t, 0, 1, addresses[3]),
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
			accounts: map[identifiers.Address][]*common.Interaction{
				addresses[0]: createTestIxs(t, 0, 2, addresses[0]),
				addresses[1]: createTestIxs(t, 0, 2, addresses[1]),
				addresses[2]: createTestIxs(t, 0, 2, addresses[2]),
				addresses[3]: createTestIxs(t, 0, 2, addresses[3]),
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
			sm.setTestMOIBalance(t, addresses...)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, ixs...))
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

//
//// FIXME: Currently the fuel price is always set to 1
// func TestIxPool_Executables_Cost_Mode(t *testing.T) {
//	addresses := tests.GetRandomAddressList(t, 5)
//
//	testcases := []struct {
//		name              string
//		accounts          map[types.Address]types.Interactions
//		expectedPriceList []uint64
//	}{
//		{
//			name: "One ix per account",
//			accounts: map[types.Address]types.Interactions{
//				addresses[0]: {
//					newIxWithFuelPrice(t, 0, addresses[0], 1),
//				},
//				addresses[1]: {
//					newIxWithFuelPrice(t, 0, addresses[1], 2),
//				},
//				addresses[2]: {
//					newIxWithFuelPrice(t, 0, addresses[2], 3),
//				},
//				addresses[3]: {
//					newIxWithFuelPrice(t, 0, addresses[3], 4),
//				},
//				addresses[4]: {
//					newIxWithFuelPrice(t, 0, addresses[4], 5),
//				},
//			},
//			expectedPriceList: []uint64{5, 4, 3, 2, 1},
//		},
//		{
//			name: "Several ixs from multiple accounts",
//			accounts: map[types.Address]types.Interactions{
//				addresses[0]: {
//					newIxWithFuelPrice(t, 0, addresses[0], 3),
//					newIxWithFuelPrice(t, 1, addresses[0], 3),
//				},
//				addresses[1]: {
//					newIxWithFuelPrice(t, 0, addresses[1], 2),
//					newIxWithFuelPrice(t, 1, addresses[1], 2),
//				},
//				addresses[2]: {
//					newIxWithFuelPrice(t, 0, addresses[2], 1),
//					newIxWithFuelPrice(t, 1, addresses[2], 1),
//				},
//			},
//			expectedPriceList: []uint64{3, 2, 1, 3, 2, 1},
//		},
//		{
//			name: "Several ixs from multiple accounts with same fuel cost",
//			accounts: map[types.Address]types.Interactions{
//				addresses[0]: {
//					newIxWithFuelPrice(t, 0, addresses[0], 6),
//					newIxWithFuelPrice(t, 1, addresses[0], 3),
//				},
//				addresses[1]: {
//					newIxWithFuelPrice(t, 0, addresses[1], 5),
//					newIxWithFuelPrice(t, 1, addresses[1], 4),
//				},
//				addresses[2]: {
//					newIxWithFuelPrice(t, 0, addresses[2], 6),
//					newIxWithFuelPrice(t, 1, addresses[2], 2),
//				},
//			},
//			expectedPriceList: []uint64{6, 6, 5, 4, 3, 2},
//		},
//	}
//
//	for _, testcase := range testcases {
//		t.Run(testcase.name, func(t *testing.T) {
//			sm := NewMockStateManager(t)
//			sm.setTestMOIBalance(addresses...)
//			ixPool := CreateTestIxpool(t, func(c *common.IxPoolConfig) {
//				c.Mode = 1
//				c.PriceLimit = defaultIxPriceLimit
//			}, true, sm)
//
//			ixPool.Start()
//			defer ixPool.Close()
//
//			for _, ixs := range testcase.accounts {
//				errs := ixPool.AddLocalInteractions(ixs)
//				log.Println(errs)
//				require.Len(t, errs, 1)
//			}
//
//			time.Sleep(100 * time.Millisecond)
//
//			successfulIxs := getSuccessfulIxs(t, ixPool, len(testcase.expectedPriceList))
//
//			// check's whether the interactions are processed in the expected order based on gas cost
//			for index, expectedPrice := range testcase.expectedPriceList {
//				require.Equal(t, expectedPrice, successfulIxs[index].FuelPrice().Uint64())
//			}
//		})
//	}
//}

func TestIxPool_Executables_Wait_Time(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 5)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Address][]*common.Interaction
		accountWaitTime map[identifiers.Address]time.Time
		updateWaitTime  func(ixPool *IxPool, accountWaitTime map[identifiers.Address]time.Time)
		expectedNonce   map[identifiers.Address]uint64
	}{
		{
			name: "One ix per account",
			accounts: map[identifiers.Address][]*common.Interaction{
				addresses[0]: createTestIxs(t, 7, 8, addresses[0]),
				addresses[1]: createTestIxs(t, 8, 9, addresses[1]),
				addresses[2]: createTestIxs(t, 5, 6, addresses[2]),
				addresses[3]: createTestIxs(t, 6, 7, addresses[3]),
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
			accounts: map[identifiers.Address][]*common.Interaction{
				addresses[0]: createTestIxs(t, 4, 6, addresses[0]),
				addresses[1]: createTestIxs(t, 5, 7, addresses[1]),
				addresses[2]: createTestIxs(t, 6, 8, addresses[2]),
				addresses[3]: createTestIxs(t, 7, 9, addresses[3]),
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
			sm.setTestMOIBalance(t, addresses...)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, ixs...))
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
	sm.setTestMOIBalance(t, addr...)

	testcases := []struct {
		name                string
		ixs                 []*common.Interaction
		ixPoolCallback      func(i *IxPool)
		expectedEnqueuedIxs uint64
		expectedPromotedIxs uint64
	}{
		{
			name: "accounts without nonce holes",
			ixs:  createTestIxs(t, 0, 5, addr[0]),
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[0], 0)
			},
			expectedEnqueuedIxs: 0,
			expectedPromotedIxs: 5,
		},
		{
			name: "accounts with nonce holes",
			ixs:  createTestIxs(t, 2, 8, addr[1]),
			ixPoolCallback: func(i *IxPool) {
				i.accounts.initOnce(addr[1], 0)
			},
			expectedEnqueuedIxs: 0,
			expectedPromotedIxs: 0,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			if test.ixPoolCallback != nil {
				test.ixPoolCallback(ixPool)
			}

			senderAddress := test.ixs[0].Sender()

			// make sure gauge is zero initially
			require.Equal(t, uint64(0), ixPool.gauge.read())

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, test.ixs...))
			require.Len(t, errs, 0)

			// make sure gauge is increased after ixns enqueued
			require.Equal(t, slotsRequired(test.ixs...), ixPool.gauge.read())

			acc := ixPool.accounts.get(test.ixs[0].Sender())
			require.Equal(t, len(test.ixs), len(acc.nonceToIX.mapping))

			ixPool.removeNonceHoleAccounts()

			// make sure gauge decreased
			require.Equal(t, test.expectedPromotedIxs, ixPool.gauge.read())
			require.Equal(t, test.expectedEnqueuedIxs, ixPool.accounts.get(senderAddress).enqueued.length())
			require.Equal(t, test.expectedPromotedIxs, ixPool.accounts.get(senderAddress).promoted.length())
			require.Equal(t, int(test.expectedPromotedIxs), len(acc.nonceToIX.mapping))
		})
	}
}

func TestIxPool_RemoveNonceHoleAccounts_WithEmptyEnqueues(t *testing.T) {
	sm := NewMockStateManager(t)
	addr := tests.GetAddresses(t, 1)
	sm.setTestMOIBalance(t, addr...)

	testcases := []struct {
		name           string
		ixs            []*common.Interaction
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
			}, true, sm, nil, nil)

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

func TestIxPool_GetIxParticipants(t *testing.T) {
	testcases := []struct {
		name                 string
		ix                   *common.Interaction
		expectedParticipants map[identifiers.Address]struct{}
	}{
		{
			name: "ix participants with beneficiary",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, tests.RandomAddress(t), nil,
			),
		},
		{
			name: "ix participants with beneficiary and payer",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, tests.RandomAddress(t), func(ixData *common.IxData) {
					ixData.Payer = tests.RandomAddress(t)
					ixData.Participants = append(ixData.Participants, common.IxParticipant{
						Address:  ixData.Payer,
						LockType: common.MutateLock,
					})
				}),
		},
		{
			name: "ix participants without beneficiary and payer",
			ix: newTestInteraction(
				t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
				0, tests.RandomAddress(t), nil,
			),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			participants := getIxParticipants(test.ix)

			require.Equal(t, len(test.ix.IxParticipants()), len(participants))

			for _, participant := range test.ix.IxParticipants() {
				require.NotNil(t, participants[participant.Address])
			}
		})
	}
}

func TestIxBatchRegistry_ProcessableBatches(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := make([]identifiers.Address, 2)
	addr, err := identifiers.NewAddressFromHex("0x0000000000000000000000000516a2efe9cd53c3d54e1f9a6e60e9077e9f9384")
	require.NoError(t, err)

	addrs[0] = addr

	addr, err = identifiers.NewAddressFromHex("0x0000000000000000000000000516a2efe9cd53c3d54e1f9a6e60e9077e9f9385")
	require.NoError(t, err)

	addrs[1] = addr

	sm.SetAccountMetaInfo(addrs[0], &common.AccountMetaInfo{
		PositionInContextSet: 0,
	})

	testcases := []struct {
		name              string
		currentView       uint64
		getIxns           func() (input []*common.Interaction, expected []*common.Interaction)
		expectedBatchList []CreateBatches
	}{
		{
			name:        "get processable batches",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 4, addrs[0], sm)

				for _, ixn := range ixns {
					ixn.SetShouldPropose(true)
					ixn.UpdateAllottedView(4)
				}

				return ixns, ixns[:3]
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount: 3,
						psCount:  4,
					},
				},
			},
		},
		{
			name:        "get processable batches from ixns where current view of an ixn is expired",
			currentView: TotalContextNodes,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, addrs[0], sm)

				for _, ixn := range ixns {
					ixn.SetShouldPropose(true)
					ixn.UpdateAllottedView(4)
				}

				ixns[1].UpdateAllottedView(0)

				return ixns, ixns
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount: 3,
						psCount:  4,
					},
				},
			},
		},
		{
			name:        "get processable batches from ixns in an optimal way",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns1 := createTestAssetTransferIxs(t, 0, 1, addrs[0], sm)

				ixns1[0].SetShouldPropose(true)
				ixns1[0].UpdateAllottedView(4)

				ixns2 := createTestAssetTransferIxs(t, 0, 1, addrs[1], sm)

				ixns2[0].SetShouldPropose(true)
				ixns2[0].UpdateAllottedView(4)

				return append(ixns1, ixns2...), append(ixns1, ixns2...)
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount: 2,
						psCount:  4,
					},
				},
			},
		},
		{
			name:        "get processable batches without nonce holes (one of the ixn cannot be proposed)",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, addrs[0], sm)

				for _, ixn := range ixns {
					ixn.SetShouldPropose(true)
					ixn.UpdateAllottedView(4)
				}

				ixns[1].SetShouldPropose(false)

				return ixns, ixns[:1]
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount: 1,
						psCount:  2,
					},
				},
			},
		},
		{
			name:        "get processable batches without nonce holes (ixn allotted view > current view)",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, addrs[0], sm)

				for _, ixn := range ixns {
					ixn.SetShouldPropose(true)
					ixn.UpdateAllottedView(4)
				}

				ixns[1].UpdateAllottedView(5)

				return ixns, ixns[:1]
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount: 1,
						psCount:  2,
					},
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPool.UpdateCurrentView(testcase.currentView)
			input, expectedIxns := testcase.getIxns()

			insertIxnsInPromotedQueue(ixPool, input)

			batches := ixPool.ProcessableBatches()

			index := 0

			for _, expectedBatches := range testcase.expectedBatchList {
				for i := 0; i < expectedBatches.batchCount; i++ {
					require.Equal(t, expectedBatches.batch.psCount, batches[i].PsCount())
					require.Equal(t, expectedBatches.batch.ixnCount, batches[i].IxCount())

					for _, ix := range batches[i].IxList() {
						require.Equal(t, expectedIxns[index], ix)

						index++
					}
				}
			}
		})
	}
}

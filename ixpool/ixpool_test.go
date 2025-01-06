package ixpool

import (
	"context"
	"encoding/hex"
	"math"
	"math/big"
	"testing"
	"time"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/crypto"

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

// Each interaction is represented by a group of participants,
// participants are represented by numbers to draw a comparison on the lexicographic order or addresses
// 999 is used to represent sarga account.
func TestIsEligibleForProposal(t *testing.T) {
	testcases := []struct {
		name                   string
		input                  [][]int
		expectedEligibleInputs [][]int
		expectedEligibility    []bool
	}{
		{
			name:                   "all ixns are eligible when they are mutually exclusive",
			input:                  [][]int{{0, 1}, {2, 3}, {4, 5}},
			expectedEligibleInputs: [][]int{{0, 1}, {2, 3}, {4, 5}},
			expectedEligibility:    []bool{true, true, true},
		},
		{
			name:                   "chained ixns with different leader accounts",
			input:                  [][]int{{0, 1}, {1, 2}},
			expectedEligibleInputs: [][]int{{0, 1}},
			expectedEligibility:    []bool{true, false},
		},
		{
			name:                   "chained ixns with same leader accounts",
			input:                  [][]int{{1, 0}, {2, 0}},
			expectedEligibleInputs: [][]int{{1, 0}, {2, 0}},
			expectedEligibility:    []bool{true, true},
		},
		{
			name:                   "chained ixns with different leader accounts",
			input:                  [][]int{{2, 0}, {2, 1}},
			expectedEligibleInputs: [][]int{{2, 0}},
			expectedEligibility:    []bool{true, false},
		},
		{
			name:                   "max unique participants per leader - multiple senders",
			input:                  [][]int{{0, 1}, {0, 2}, {2, 0}, {3, 0}, {4, 0}, {5, 4}},
			expectedEligibleInputs: [][]int{{0, 1}, {0, 2}, {2, 0}, {3, 0}, {5, 4}},
			expectedEligibility:    []bool{true, true, true, true, false, true},
		},
		{
			name:                   "max unique participants per leader - single sender",
			input:                  [][]int{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 5}},
			expectedEligibleInputs: [][]int{{0, 1}, {0, 2}, {0, 3}},
			expectedEligibility:    []bool{true, true, true, false, false},
		},
		{
			name:                   "ixns loop",
			input:                  [][]int{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {2, 0}, {3, 0}},
			expectedEligibleInputs: [][]int{{0, 1}, {0, 2}, {0, 3}, {1, 0}, {2, 0}, {3, 0}},
			expectedEligibility:    []bool{true, true, true, true, true, true},
		},
		{
			name:                   "chained ixns and ixns loop",
			input:                  [][]int{{0, 1}, {1, 2}, {2, 3}, {2, 0}},
			expectedEligibleInputs: [][]int{{0, 1}, {2, 3}},
			expectedEligibility:    []bool{true, false, true, false},
		},
		{
			name:                   "chained ixns and ixns loop",
			input:                  [][]int{{0, 1}, {1, 0}, {1, 2}, {2, 0}},
			expectedEligibleInputs: [][]int{{0, 1}, {1, 0}, {2, 0}},
			expectedEligibility:    []bool{true, true, false, true},
		},
		{
			name:                   "more than 2 participants per ixn",
			input:                  [][]int{{0, 1, 2}, {0, 3, 1}, {0, 4, 1}},
			expectedEligibleInputs: [][]int{{0, 1, 2}, {0, 3, 1}},
			expectedEligibility:    []bool{true, true, false},
		},
		{
			name:                   "more than 2 participants per ixn",
			input:                  [][]int{{0, 1, 2}, {0, 3, 1}, {2, 1, 0}, {3, 1, 2}},
			expectedEligibleInputs: [][]int{{0, 1, 2}, {0, 3, 1}, {2, 1, 0}},
			expectedEligibility:    []bool{true, true, true, false},
		},
		{
			name:                   "lot of ixns",
			input:                  [][]int{{0, 1}, {0, 1}, {0, 1}, {0, 1}, {0, 1}, {0, 1}},
			expectedEligibleInputs: [][]int{{0, 1}, {0, 1}, {0, 1}, {0, 1}, {0, 1}, {0, 1}},
			expectedEligibility:    []bool{true, true, true, true, true, true},
		},
		{
			name:                   "don't count genesis accounts",
			input:                  [][]int{{0, 999}, {1, 999}, {2, 999}, {3, 999}},
			expectedEligibleInputs: [][]int{{0, 999}, {1, 999}, {2, 999}}, // 999 refers to sarga
			expectedEligibility:    []bool{true, true, true, false},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			var (
				participantToAcquirer  = make(map[identifiers.Address]identifiers.Address)
				acquirerToParticipants = make(map[identifiers.Address]map[identifiers.Address]struct{})
			)

			ixns := createIxnsFromParticipants(t, testcase.input)
			eligibility := make([]bool, len(ixns))

			for i, ix := range ixns {
				index := isEligibleForProposal(ix, participantToAcquirer, acquirerToParticipants)
				eligibility[i] = index
			}

			require.Equal(t, len(testcase.expectedEligibility), len(eligibility))
			require.Equal(t, testcase.expectedEligibility, eligibility)
		})
	}
}

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
					0, tests.RandomAddress(t), 0, nil,
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
					0, tests.RandomAddress(t), 0, nil,
				),
				newTestInteraction(t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					0, tests.RandomAddress(t), 0, nil,
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)
	sm.registerAccounts(transferPayload.Beneficiary)
	sm.setAccountKeysAndPublicKeys(t, addrs...)

	getIxParams := func(
		from identifiers.Address, sequenceID uint64,
		price int, payload []byte, noOfTxs int,
	) *tests.CreateIxParams {
		return &tests.CreateIxParams{
			IxDataCallback: func(ix *common.IxData) {
				ix.Sender = common.Sender{
					Address:    from,
					SequenceID: sequenceID,
				}
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
			sender := test.ix.SenderAddr()
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, nil)

			ixPool.getOrCreateAccountQueue(sender, 0, 0)

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
			acc := ixPool.accounts.getAccountQueue(sender, 0)
			require.Equal(t, 1, len(ixPool.allIxs.all))
			require.Equal(t, test.expectedGaugeSize, ixPool.gauge.read())

			require.Equal(t, 1, len(acc.sequenceIDToIX.mapping))
			ixInNonceMap := acc.sequenceIDToIX.get(test.ix.SequenceID())
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
				0, addrs[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				interaction.SetSender(common.Sender{
					Address: identifiers.NilAddress,
				})
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "Already known Interaction",
			ix: newTestInteraction(
				t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
				0, addrs[0], 0, nil,
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
				0, addrs[0], 0, nil,
			),
		},
		{
			name: "ixpool overflow",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], 0, nil,
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
				0, addrs[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots - 1)
			},
		},
		{
			name: "failed to add lower sequenceID ixn",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
				0, addrs[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.getOrCreateAccountQueue(addrs[0], 0, 1)
			},
			expectedErr: ErrSequenceIDTooLow,
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
					0, addrs[0], 0, nil,
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
					0, addrs[0], 0, nil,
				),
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], 0, nil,
				),
			},
			preTestFn: func(interactions []*common.Interaction) {
				interactions[1].SetSender(common.Sender{
					Address: identifiers.NilAddress,
				})
			},
			expectedAddedIxns: 1,
			expectedResult:    pubsub.ValidationReject,
		},
		{
			name: "Reject interactions with invalid signature",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], 0, nil,
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
					0, addrs[0], 0, nil,
				),
				newTestInteraction(
					t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
					0, addrs[0], 0, nil,
				),
			},
			preTestFn: func(interactions []*common.Interaction) {
				interactions[1].SetSender(common.Sender{
					Address: identifiers.NilAddress,
				})
			},
			broadcastedIxns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, addrs[1]),
					0, addrs[0], 0, nil,
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

	testcases := []struct {
		name string
		ixn  []*common.Interaction
	}{
		{
			name: "check for add ixn event",
			ixn: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, addrs[0], 0, nil,
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
				0, addrs[0], 0, nil,
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
				1, addrs[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.getOrCreateAccountQueue(interaction.SenderAddr(), 0, 0)
				ixPool.gauge.increase((80 * config.DefaultMaxIXPoolSlots / 100) + 1)
			},
			expectedError: common.ErrRejectFutureIx,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sender := test.ix.SenderAddr()
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

			acc := ixPool.accounts.getAccountQueue(sender, 0)
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
	sm.setAccountKeysAndPublicKeys(t, address)

	testcases := []struct {
		name           string
		ixs            []*common.Interaction
		testFn         func(ixPool *IxPool, interactions []*common.Interaction)
		expectedResult expectedResult
		expectedErrors int
	}{
		{
			name: "Enqueue ixs with higher sequenceID",
			ixs: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					0, address, 0, nil,
				),
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.NilAddress),
					5, address, 0, nil,
				),
			},
			expectedResult: expectedResult{
				enqueued:         1,
				promotedAccounts: 1,
			},
		},
		{
			name: "All the ixs are with low sequenceID",
			ixs:  createTestIxs(t, 0, 5, address),
			testFn: func(ixPool *IxPool, interactions []*common.Interaction) {
				ixPool.getOrCreateAccountQueue(interactions[0].SenderAddr(), 0, 5)
			},
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 0,
			},
			expectedErrors: 5,
		},
		{
			name: "Should not enqueue ixs with low sequenceID",
			ixs:  createTestIxs(t, 0, 6, address),
			testFn: func(ixPool *IxPool, interactions []*common.Interaction) {
				ixPool.getOrCreateAccountQueue(interactions[0].SenderAddr(), 0, 5)
			},
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 1,
			},
			expectedErrors: 5,
		},
		{
			name: "Promote ixs with expected sequenceID",
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
			senderAddress := testcase.ixs[0].SenderAddr()

			if testcase.testFn != nil {
				testcase.testFn(ixPool, testcase.ixs)
			}

			require.Equal(t, uint64(0), ixPool.gauge.read())

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Equal(t, testcase.expectedErrors, len(errs))

			require.Equal(t, testcase.expectedResult.enqueued,
				ixPool.accounts.getAccountQueue(senderAddress, 0).enqueued.length())
			require.Equal(t, testcase.expectedResult.promotedAccounts,
				ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())
			require.Equal(t,
				testcase.expectedResult.enqueued+testcase.expectedResult.promotedAccounts, ixPool.gauge.read())
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)
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
			senderAddress := testcase.ixs[0].SenderAddr()

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			if testcase.popIx != nil {
				testcase.popIx(senderAddress)
			}

			// checks whether the account's sequenceID is updated
			require.Equal(t, testcase.expected.nonce, ixPool.accounts.getAccountQueue(senderAddress, 0).getSequenceID())
			// checks whether the ixs are removed from the enqueue
			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.getAccountQueue(senderAddress, 0).enqueued.length())
			// checks whether the ixs are promoted
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())

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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
			name:    "Create an account with latest state object sequenceID",
			address: tests.RandomAddress(t),
			nonce:   1,
			testFn: func(addr identifiers.Address) {
				sm.setLatestSequenceID(t, addr, 0, 5)
			},
			expectedNonce: 5,
		},
		{
			name:          "Create an account with given sequenceID as the state object doesn't exists",
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

			_, accountKey := ixPool.getOrCreateAccountQueue(testcase.address, 0, testcase.nonce)
			require.NotNil(t, accountKey)
			// checks whether an account is created with expected sequenceID
			require.Equal(t, testcase.expectedNonce, accountKey.getSequenceID())
		})
	}
}

func TestIxPool_ResetWithHeaders_resetWaitTime(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(t, addrs...)
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
			name:               "Prune all the interactions with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, addrs[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "Prune some interactions with low sequenceID",
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
			senderAddress := testcase.ixs[0].SenderAddr()

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			if testcase.incrementCounter != nil {
				testcase.incrementCounter(ixPool.accounts.getAccount(senderAddress))
				// check whether the delay counter is updated
				require.Equal(t, int32(1), ixPool.accounts.getAccount(senderAddress).delayCounter)
			}

			ts := getTesseractWithIxs(t, senderAddress, testcase.nonce)

			// reset with headers removes the interactions with sequenceID lesser than given tesseract's interaction sequenceID
			// from the queues and resets the delay counter to default value
			ixPool.ResetWithHeaders(ts)

			require.Equal(t, int32(0), ixPool.accounts.getAccount(senderAddress).delayCounter)
			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_enqueued(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 3)
	sm.setTestMOIBalance(t, addrs...)
	sm.setAccountKeysAndPublicKeys(t, addrs...)

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectedEnqueued uint64
		expectedPromoted uint64
	}{
		{
			name:             "Prune all ixs with low sequenceID",
			ixs:              createTestIxs(t, 1, 5, addrs[0]),
			nonce:            5,
			expectedEnqueued: 0,
		},
		{
			name:             "No low sequenceID ixs to prune",
			ixs:              createTestIxs(t, 1, 7, addrs[1])[2:6],
			nonce:            1,
			expectedEnqueued: 4,
		},
		{
			name:             "Prune some ixs with low sequenceID",
			ixs:              createTestIxs(t, 1, 6, addrs[2]),
			nonce:            3,
			expectedPromoted: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].SenderAddr()

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			ixPool.getOrCreateAccountQueue(senderAddress, 0, 0)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))

			// On reset should prune the ixs from the enqueue if the sequenceID is lesser than the given sequenceID
			ixPool.ResetWithHeaders(ts)

			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

			require.Equal(t, testcase.expectedEnqueued, ixPool.accounts.getAccountQueue(senderAddress, 0).enqueued.length())
			require.Equal(t, testcase.expectedPromoted, ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_prunedEnqueuedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs...)
	sm.setAccountKeysAndPublicKeys(t, addrs...)

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some enqueue ixs with low sequenceID and check for events",
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
			senderAddress := testcase.ixs[0].SenderAddr()

			ixPool.getOrCreateAccountQueue(senderAddress, 0, 0)

			ixPrunedEnqueueEventSub := ixPool.mux.Subscribe(utils.PrunedEnqueuedInteractionEvent{})

			ixPrunedEnqueueResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedEnqueueEventSub, ixPrunedEnqueueResp, len(testcase.ixs))

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			// ResetWithHeaders should prune the ixs from the enqueue if the sequenceID is lesser than the given sequenceID
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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
			name:               "Prune all the ixs with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, addrs[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "No low sequenceID ixs to prune",
			ixs:                createTestIxs(t, 0, 6, addrs[1])[1:6],
			nonce:              0,
			expectedPromotions: 5,
		},
		{
			name:               "Prune some ixs with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, addrs[2]),
			nonce:              3,
			expectedPromotions: 1,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderAddress := testcase.ixs[0].SenderAddr()

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))
			// On reset should prune the ixs from the promoted queue if the sequenceID is lesser than the given sequenceID
			ixPool.ResetWithHeaders(ts)

			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_prunedPromotedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	addrs := tests.GetAddresses(t, 2)
	sm.setTestMOIBalance(t, addrs...)
	sm.setAccountKeysAndPublicKeys(t, addrs...)

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some promoted ixs with low sequenceID and check for events",
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

			senderAddress := testcase.ixs[0].SenderAddr()

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
			name: "Prune all ixs with low sequenceID",
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
			name: "No low sequenceID ixs to prune",
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
			sm.setAccountKeysAndPublicKeys(t, address)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil, newMockNetwork(""))

			senderAddress := testcase.ixs[0].SenderAddr()
			// ixPool.createAccountOnce(senderAddress, 0)

			require.Equal(t, uint64(0), ixPool.gauge.read())

			addAndProcessIxs(t, sm, ixPool, testcase.ixs...)
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			acc := ixPool.accounts.getAccountQueue(senderAddress, 0)
			require.Equal(t, len(testcase.ixs), len(acc.sequenceIDToIX.mapping))

			ts := getTesseractWithIxs(t, senderAddress, int(testcase.nonce))
			ixPool.ResetWithHeaders(ts)

			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, ixPool.gauge.read())

			acc = ixPool.accounts.getAccountQueue(senderAddress, 0)
			require.Equal(t, testcase.expected.enqueued, acc.enqueued.length())
			require.Equal(t, testcase.expected.promoted, acc.promoted.length())
			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, uint64(len(acc.sequenceIDToIX.mapping)))
		})
	}
}

func TestIxPool_Pop(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, addr1)
	sm.setAccountKeysAndPublicKeys(t, addr1)

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
			senderAddress := testcase.ixs[0].SenderAddr()

			require.Equal(t, uint64(0), ixPool.gauge.read())

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())

			ix := ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.peek()

			ixPool.Pop(ix)
			require.Equal(t, testcase.expectedPromotions, ixPool.gauge.read())
			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())
		})
	}
}

func TestIxPool_Drop(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, addr1)
	sm.setAccountKeysAndPublicKeys(t, addr1)

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

			senderAddress := testcase.ixs[0].SenderAddr()
			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			acc := ixPool.accounts.getAccountQueue(senderAddress, 0)
			ix := acc.promoted.peek()

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.ixs)), uint64(len(acc.sequenceIDToIX.mapping)))

			ixPool.Drop(ix)
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())

			data := tests.WaitForResponse(t, ixDroppedResp, utils.DroppedInteractionEvent{})
			event, ok := data.(utils.DroppedInteractionEvent)
			require.True(t, ok)
			require.Equal(t, testcase.ixs, event.Ixs)

			require.Equal(t, 0, len(acc.sequenceIDToIX.mapping))
		})
	}
}

func TestIxPool_Drop_FinalizedIx(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, addr1)
	sm.setAccountKeysAndPublicKeys(t, addr1)

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
			name:        "latest state object sequenceID greater than the dropped ix sequenceID",
			finalizedIx: ixs[0],
			pendingIxs:  ixs[1:],
			preTestFn: func() {
				sm.setLatestSequenceID(t, addr1, 0, 1)
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

			senderAddress := testcase.pendingIxs[0].SenderAddr()
			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.pendingIxs...))
			require.Len(t, errs, 0)

			acc := ixPool.accounts.getAccountQueue(senderAddress, 0)
			require.Equal(t, uint64(len(testcase.pendingIxs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.pendingIxs)), uint64(len(acc.sequenceIDToIX.mapping)))

			testcase.preTestFn()

			ixPool.Drop(testcase.finalizedIx)

			require.Equal(t, uint64(len(testcase.pendingIxs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.pendingIxs)), uint64(len(acc.sequenceIDToIX.mapping)))
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
			acc, _ := ixPool.getOrCreateAccountQueue(testcase.addr, 0, 0)

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
	sm.setAccountKeysAndPublicKeys(t, addrs...)

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
				0, tests.RandomAddress(t), 0, nil,
			),
			testFn: func(interaction *common.Interaction) {
				interaction.SetSender(common.Sender{
					Address: identifiers.NilAddress,
				})
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "SequenceID too low error",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				9, addrs[1], 0, nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.setLatestSequenceID(t, interaction.SenderAddr(), 0, 10)
			},
			expectedErr: ErrSequenceIDTooLow,
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
				0, addrs[2], 0, nil,
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
				0, addrs[0], 0, nil),
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
				0, addrs[0], 0, nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.balance[interaction.SenderAddr()] = map[identifiers.AssetID]*big.Int{
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
	sm.setAccountKeysAndPublicKeys(t, address, addr2)

	ixArgs := common.IxData{
		Sender: common.Sender{
			Address: address,
		},
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
				0, addr2, 0,
				nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(t, interaction.SenderAddr(), "assetID1", big.NewInt(10))
			},
			expectedErr: common.ErrInvalidIXSignature,
		},
		{
			name: "valid signature",
			ix:   ix,
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(t, interaction.SenderAddr(), "assetID1", big.NewInt(10))
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
				0, address, 0, func(ixData *common.IxData) {
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
				0, tests.RandomAddress(t), 0, nil,
			),
			expectedErr: common.ErrFetchingBalance,
		},
		{
			name: "insufficient asset funds",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, address, 0, func(ixData *common.IxData) {
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
				0, address, 0, func(ixData *common.IxData) {
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
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, address, 0, nil),
			testFn: func(ix *common.Interaction) {
				ix.GetIxOp(0).OpType = common.IxInvalid
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name:        "ix with invalid operation payload",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetPayload, 0, address, 0, nil),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name: "ix with valid operation payload",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, address, 0, nil),
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
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetStandardPayload, 0, address, 0, nil),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name:        "should return error if asset supply is invalid",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetSupplyPayload, 0, address, 0, nil),
			expectedErr: common.ErrInvalidAssetSupply,
		},
		{
			name: "should return success if asset standard is valid",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, address, 0, nil),
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
			ix:          newTestInteraction(t, common.IxAssetMint, invalidAssetPayload, 0, address, 0, nil),
			expectedErr: common.ErrInvalidValue,
		},
		{
			name:        "should return error if non fungible token minted",
			ix:          newTestInteraction(t, common.IxAssetMint, NFTAssetPayload, 0, address, 0, nil),
			expectedErr: common.ErrMintOrBurnNonFungibleToken,
		},
		{
			name: "valid asset supply payload",
			ix:   newTestInteraction(t, common.IxAssetMint, validAssetPayload, 0, address, 0, nil),
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

func TestIxPool_ValidateParticipantRegister(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	address := tests.RandomAddress(t)
	sm.registerAccounts(address)

	participantCreatePayload := common.ParticipantCreatePayload{
		Address: tests.RandomAddress(t),
		KeysPayload: []common.KeyAddPayload{
			{
				PublicKey:          tests.RandomAddress(t).Bytes(),
				Weight:             1000,
				SignatureAlgorithm: 0,
			},
		},
		Amount: big.NewInt(100),
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "should return success if participant register data is valid",
			ix:   newTestInteraction(t, common.IxParticipantCreate, participantCreatePayload, 0, address, 0, nil),
			preTestFn: func(interaction *common.Interaction) {
				sm.setBalance(t, address, common.KMOITokenAssetID, big.NewInt(1000))
			},
		},
		{
			name: "invalid address",
			ix: newTestInteraction(t, common.IxParticipantCreate, common.ParticipantCreatePayload{
				Address: identifiers.NilAddress,
			}, 0, address, 0, nil),
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "invalid weight",
			ix: newTestInteraction(t, common.IxParticipantCreate, common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomAddress(t).Bytes(),
						Weight:             400,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(100),
			}, 0, address, 0, nil),
			expectedErr: common.ErrInvalidWeight,
		},
		{
			name: "invalid signature algorithms",
			ix: newTestInteraction(t, common.IxParticipantCreate, common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomAddress(t).Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 1,
					},
				},
				Amount: big.NewInt(100),
			}, 0, address, 0, nil),
			expectedErr: common.ErrInvalidSignatureAlgorithm,
		},
		{
			name: "invalid amount",
			ix: newTestInteraction(t, common.IxParticipantCreate, common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomAddress(t).Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Amount: big.NewInt(0),
			}, 0, address, 0, nil),
			preTestFn: func(interaction *common.Interaction) {
				sm.setBalance(t, address, common.KMOITokenAssetID, big.NewInt(1000))
			},
			expectedErr: common.ErrInvalidValue,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.preTestFn != nil {
				testcase.preTestFn(testcase.ix)
			}

			err := ixPool.validateParticipantCreate(testcase.ix, 0)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_VerifyParticipantSignatures(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	rawPayload := []byte{1, 2, 3}

	accs, err := cmdCommon.GetAccountsWithMnemonic(3)
	require.NoError(t, err)

	sig, err := crypto.GetSignature(rawPayload, accs[0].Mnemonic)
	require.NoError(t, err)

	rawSig, err := hex.DecodeString(sig)
	require.NoError(t, err)

	sm.setAccountKeysAndPublicKeys(t, accs[0].Addr)

	sig2, err := crypto.GetSignature(rawPayload, accs[2].Mnemonic)
	require.NoError(t, err)

	rawSig2, err := hex.DecodeString(sig2)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		addr        identifiers.Address
		signatures  common.Signatures
		preTestFn   func()
		expectedErr error
	}{
		{
			name: "should return success if participant register data is valid",
			addr: accs[0].Addr,
			signatures: common.Signatures{
				{
					Address:   accs[0].Addr,
					KeyID:     0,
					Signature: rawSig,
				},
			},
		},
		{
			name: "public key not found",
			addr: accs[1].Addr,
			preTestFn: func() {
				sm.setAccountKeys(accs[1].Addr, common.AccountKeys{
					{
						Weight: 1000,
					},
				})
			},
			signatures: common.Signatures{
				{
					Address:   accs[1].Addr,
					KeyID:     0,
					Signature: rawSig,
				},
			},
			expectedErr: common.ErrPublicKeyNotFound,
		},
		{
			name: "invalid ix signature",
			addr: accs[0].Addr,
			signatures: common.Signatures{
				{
					Address:   accs[0].Addr,
					KeyID:     0,
					Signature: tests.RandomHash(t).Bytes(),
				},
			},
			expectedErr: common.ErrInvalidIXSignature,
		},
		{
			name: "invalid weight due to revoked key",
			addr: accs[2].Addr,
			preTestFn: func() {
				sm.setAccountKeys(accs[2].Addr, common.AccountKeys{
					{
						Weight:  1000,
						Revoked: true,
					},
				})
				sm.setPublicKey(accs[2].Addr, 0, accs[2].Addr.Bytes())
			},
			signatures: common.Signatures{
				{
					Address:   accs[2].Addr,
					KeyID:     0,
					Signature: rawSig2,
				},
			},
			expectedErr: common.ErrInvalidWeight,
		},
		{
			name: "account keys not found",
			addr: tests.RandomAddress(t),
			signatures: common.Signatures{
				{
					Address:   accs[0].Addr,
					KeyID:     0,
					Signature: rawSig,
				},
			},
			expectedErr: errors.New("account keys not found"),
		},
		{
			name: "invalid key id in signature",
			addr: accs[0].Addr,
			signatures: common.Signatures{
				{
					Address:   accs[0].Addr,
					KeyID:     1,
					Signature: rawSig,
				},
			},
			expectedErr: errors.New("invalid key id in signature"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.preTestFn != nil {
				testcase.preTestFn()
			}

			err := ixPool.verifyParticipantSignatures(testcase.addr, rawPayload, testcase.signatures)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_VerifySignatures(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	accs, err := cmdCommon.GetAccountsWithMnemonic(3)
	require.NoError(t, err)

	ixParams := getIXParams(
		accs[0].Addr,
		common.IxAssetTransfer,
		defaultIxPriceLimit,
		common.AssetActionPayload{
			Beneficiary: tests.RandomAddress(t),
			AssetID:     "assetID1",
			Amount:      big.NewInt(5),
		},
		nil,
	)

	ixParams.SignaturesCallback = func(data *common.IxData, sig *common.Signatures) {
		data.Participants = append(data.Participants, common.IxParticipant{
			Address: accs[1].Addr,
			Notary:  true,
		})

		rawSign := tests.GetIXSignature(t, data, accs[0].Mnemonic)
		(*sig)[0] = common.Signature{
			Address:   data.Sender.Address,
			KeyID:     data.Sender.KeyID,
			Signature: rawSign,
		}

		rawSign = tests.GetIXSignature(t, data, accs[1].Mnemonic)
		*sig = append(*sig, common.Signature{
			Address:   accs[1].Addr,
			KeyID:     0,
			Signature: rawSign,
		})
	}

	sm.setAccountKeysAndPublicKeys(t, accs[0].Addr, accs[1].Addr)

	testcases := []struct {
		name        string
		preTestFn   func(params *common.IxData)
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name: "should return success if signatures are valid",
			preTestFn: func(params *common.IxData) {
			},
			ix: tests.CreateIX(t, ixParams),
		},
		{
			name: "sender's signature is not found",
			ix: tests.CreateIX(t, &tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.Sender.Address = accs[0].Addr
				},
				SignaturesCallback: func(data *common.IxData, sig *common.Signatures) {
					*sig = nil // remove the senders signature
				},
			}),
			expectedErr: common.ErrInvalidSenderSignature,
		},
		{
			name: "sender's signature is not found",
			ix: tests.CreateIX(t, &tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.Sender.Address = accs[0].Addr
				},
			}),
			expectedErr: errors.New("invalid sender's signature"),
		},
		{
			name: "invalid notary participant signature",
			ix: tests.CreateIX(t, &tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.Sender.Address = accs[0].Addr
					ix.Participants = append(ix.Participants, common.IxParticipant{
						Address: accs[1].Addr,
						Notary:  true,
					})
				},
				SignaturesCallback: func(data *common.IxData, sig *common.Signatures) {
					rawSign := tests.GetIXSignature(t, data, accs[0].Mnemonic)
					(*sig)[0] = common.Signature{
						Address:   data.Sender.Address,
						KeyID:     data.Sender.KeyID,
						Signature: rawSign,
					}
				},
			}),
			expectedErr: errors.New("invalid notary participant signature"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := ixPool.verifySignatures(testcase.ix)
			if testcase.expectedErr != nil {
				require.ErrorContains(t, err, testcase.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIxPool_ValidateAccountConfigure(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil, nil)

	address := tests.RandomAddress(t)
	addAccConfigurePayload := common.AccountConfigurePayload{
		Add: []common.KeyAddPayload{
			{
				Weight: 300,
			},
		},
	}

	revokeAccConfigurePayload := common.AccountConfigurePayload{
		Revoke: []common.KeyRevokePayload{
			{
				KeyID: 300,
			},
		},
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "should return success if acc key is added",
			ix:   newTestInteraction(t, common.IXAccountConfigure, addAccConfigurePayload, 0, address, 0, nil),
		},
		{
			name: "should return success if acc key is revoked",
			ix:   newTestInteraction(t, common.IXAccountConfigure, revokeAccConfigurePayload, 0, address, 0, nil),
			preTestFn: func(interaction *common.Interaction) {
				sm.setAccountKeys(address, common.AccountKeys{{
					ID: 300,
				}})
			},
		},
		{
			name: "can not provide both add and revoke payload",
			ix: newTestInteraction(t, common.IXAccountConfigure, common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						Weight: 300,
					},
				},
				Revoke: []common.KeyRevokePayload{
					{
						KeyID: 300,
					},
				},
			}, 0, address, 0, nil),
			expectedErr: common.ErrInvalidAccountConfigure,
		},
		{
			name: "both add and revoke can not be empty",
			ix: newTestInteraction(t, common.IXAccountConfigure, common.AccountConfigurePayload{
				Add:    []common.KeyAddPayload{},
				Revoke: []common.KeyRevokePayload{},
			}, 0, address, 0, nil),
			expectedErr: common.ErrInvalidAccountConfigure,
		},
		{
			name: "invalid signature algorithm",
			ix: newTestInteraction(t, common.IXAccountConfigure, common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						SignatureAlgorithm: 1,
					},
				},
			}, 0, address, 0, nil),
			expectedErr: common.ErrInvalidSignatureAlgorithm,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.preTestFn != nil {
				testcase.preTestFn(testcase.ix)
			}

			err := ixPool.validateAccountConfigure(testcase.ix, 0)
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
			ix:          newTestInteraction(t, common.IxLogicDeploy, invalidLogicPayload, 0, address, 0, nil),
			expectedErr: common.ErrEmptyManifest,
		},
		{
			name: "should return success if logic payload is valid",
			ix:   newTestInteraction(t, common.IxLogicDeploy, validLogicPayload, 0, address, 0, nil),
		},
		{
			name: "should return error if callsite is invalid",
			ix:   newTestInteraction(t, common.IxLogicDeploy, validLogicPayload, 0, address, 0, nil),
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
			ix:          newTestInteraction(t, common.IxLogicInvoke, payloadWithoutCallsite, 0, address, 0, nil),
			expectedErr: common.ErrEmptyCallSite,
		},
		{
			name:        "should return error if logicID is empty",
			ix:          newTestInteraction(t, common.IxLogicInvoke, payloadWithoutLogicID, 0, address, 0, nil),
			expectedErr: common.ErrMissingLogicID,
		},
		{
			name: "should return error if receiver object not found",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, address, 0, nil),
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, "logicID-1")
			},
			expectedErr: errors.New("state object not found"),
		},
		{
			name: "should return success if logic payload is valid",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, address, 0, nil),
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, "logicID-1")
				msm.setLatestStateObject(interaction.GetIxOp(0).Target(), &state.Object{})
				msm.setLatestStateObject(interaction.SenderAddr(), &state.Object{})
			},
		},
		{
			name: "should return error if callsite is invalid",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, address, 0, nil),
			setHook: func(exec *MockExecutionManager) {
				exec.validateLogicInvokeHook = func() error {
					return errors.New("invalid callsite")
				}
			},
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, "logicID-1")
				msm.setLatestStateObject(interaction.GetIxOp(0).Target(), &state.Object{})
				msm.setLatestStateObject(interaction.SenderAddr(), &state.Object{})
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
					acc := ixPool.accounts.getAccount(addr)
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
					acc := ixPool.accounts.getAccount(addr)
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
			sm.setAccountKeysAndPublicKeys(t, addresses...)
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
				require.Equal(t, expectedAddress, successfulIxs[index].SenderAddr())
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
					acc := ixPool.accounts.getAccount(addr)
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
					acc := ixPool.accounts.getAccount(addr)
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
			sm.setAccountKeysAndPublicKeys(t, addresses...)
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
	sm.setAccountKeysAndPublicKeys(t, addr...)

	testcases := []struct {
		name                string
		ixs                 []*common.Interaction
		ixPoolCallback      func(i *IxPool)
		expectedEnqueuedIxs uint64
		expectedPromotedIxs uint64
	}{
		{
			name: "accounts without sequenceID holes",
			ixs:  createTestIxs(t, 0, 5, addr[0]),
			ixPoolCallback: func(i *IxPool) {
				i.getOrCreateAccountQueue(addr[0], 0, 0)
			},
			expectedEnqueuedIxs: 0,
			expectedPromotedIxs: 5,
		},
		{
			name: "accounts with sequenceID holes",
			ixs:  createTestIxs(t, 2, 8, addr[1]),
			ixPoolCallback: func(i *IxPool) {
				i.getOrCreateAccountQueue(addr[1], 0, 0)
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

			senderAddress := test.ixs[0].SenderAddr()

			// make sure gauge is zero initially
			require.Equal(t, uint64(0), ixPool.gauge.read())

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, test.ixs...))
			require.Len(t, errs, 0)

			// make sure gauge is increased after ixns enqueued
			require.Equal(t, slotsRequired(test.ixs...), ixPool.gauge.read())

			acc := ixPool.accounts.getAccountQueue(test.ixs[0].SenderAddr(), 0)
			require.Equal(t, len(test.ixs), len(acc.sequenceIDToIX.mapping))

			ixPool.removeSequenceIDHoleAccounts()

			// make sure gauge decreased
			require.Equal(t, test.expectedPromotedIxs, ixPool.gauge.read())
			require.Equal(t, test.expectedEnqueuedIxs, ixPool.accounts.getAccountQueue(senderAddress, 0).enqueued.length())
			require.Equal(t, test.expectedPromotedIxs, ixPool.accounts.getAccountQueue(senderAddress, 0).promoted.length())
			require.Equal(t, int(test.expectedPromotedIxs), len(acc.sequenceIDToIX.mapping))
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
				i.getOrCreateAccountQueue(addr[0], 0, 0)
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
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(addr[0], 0).getSequenceID())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(addr[0], 0).enqueued.length())

			ixPool.removeSequenceIDHoleAccounts()

			// make sure gauge is not decreased
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(addr[0], 0).getSequenceID())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(addr[0], 0).enqueued.length())
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
				0, tests.RandomAddress(t), 0, nil,
			),
		},
		{
			name: "ix participants with beneficiary and payer",
			ix: newTestInteraction(
				t, common.IxAssetTransfer, tests.CreateAssetActionPayload(t, identifiers.NilAddress),
				0, tests.RandomAddress(t), 0, func(ixData *common.IxData) {
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
				0, tests.RandomAddress(t), 0, nil,
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
				ixns := createTestAssetTransferIxs(t, 0, 4, addrs[0], 1, sm)

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
			name:        "get processable batches with multiple keys for same address",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 1, addrs[0], 4, sm)

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
			currentView: common.BehaviouralContextSize,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, addrs[0], 1, sm)

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
				ixns1 := createTestAssetTransferIxs(t, 0, 1, addrs[0], 1, sm)

				ixns1[0].SetShouldPropose(true)
				ixns1[0].UpdateAllottedView(4)

				ixns2 := createTestAssetTransferIxs(t, 0, 1, addrs[1], 1, sm)

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
			name:        "get processable batches without sequenceID holes (one of the ixn cannot be proposed)",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, addrs[0], 1, sm)

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
			name:        "get processable batches without sequenceID holes (ixn allotted view > current view)",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, addrs[0], 1, sm)

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
		{
			name:        "batches should be picked in sorted participants order",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createIxnsFromParticipants(t, [][]int{{6, 1}, {6, 5}, {6, 0}, {2, 0}, {0, 1}, {0, 3}, {0, 5}})
				for _, ixn := range ixns {
					ixn.SetShouldPropose(true)
					ixn.UpdateAllottedView(4)
				}

				return ixns, ixns[4:]
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

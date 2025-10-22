package ixpool

import (
	"context"
	"encoding/hex"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/poi"

	"github.com/hashicorp/go-hclog"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/sarvalabs/go-polo"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/state"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
)

var defaultIxPriceLimit = big.NewInt(1)

const (
	contextTimeout = 5 * time.Second
	ixMaxSize      = 128 * 1024 // 128Kb
)

func TestGetPrimaryAccountConsensusNodesHash(t *testing.T) {
	ids := tests.GetIdentifiers(t, 3)
	hashes := tests.GetHashes(t, 2)

	testcases := []struct {
		name          string
		id            identifiers.Identifier
		preTestFn     func(sm *MockStateManager, ixPool *IxPool)
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name: "fetch consensus nodes hash from db",
			id:   ids[1],
			preTestFn: func(sm *MockStateManager, ixPool *IxPool) {
				sm.SetAccountMetaInfo(ids[1], &common.AccountMetaInfo{
					ConsensusNodesHash: hashes[1],
				})
			},
			expectedHash: hashes[1],
		},
		{
			name: "fetch consensus nodes hash from cache",
			id:   ids[0],
			preTestFn: func(sm *MockStateManager, ixPool *IxPool) {
				ixPool.consensusNodesHash.Add(ids[0], hashes[0])
			},
			expectedHash: hashes[0],
		},
		{
			name:          "account not found",
			id:            ids[2],
			expectedError: errors.New("account meta info not found"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

			if testcase.preTestFn != nil {
				testcase.preTestFn(sm, ixPool)
			}

			ixHash, err := ixPool.getPrimaryAccountConsensusNodesHash(testcase.id)

			if testcase.expectedError != nil {
				require.ErrorContains(t, err, testcase.expectedError.Error())

				return
			}

			require.NoError(t, err)

			require.Equal(t, testcase.expectedHash, ixHash)

			val, isCached := ixPool.consensusNodesHash.Get(testcase.id)
			require.True(t, isCached)

			require.Equal(t, testcase.expectedHash, val.(common.Hash)) //nolint:forcetypeassert
		})
	}
}

func TestGetConsensusNodesHash(t *testing.T) {
	ids := tests.GetIdentifiers(t, 1)
	hashes := tests.GetHashes(t, 1)

	subAccount := tests.RandomSubAccountIdentifier(t, 1)

	testcases := []struct {
		name          string
		id            identifiers.Identifier
		preTestFn     func(sm *MockStateManager, ixPool *IxPool)
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name: "fetch sub account consensus nodes hash",
			id:   subAccount,
			preTestFn: func(sm *MockStateManager, ixPool *IxPool) {
				sm.SetAccountMetaInfo(subAccount, &common.AccountMetaInfo{
					InheritedAccount: ids[0],
				})

				sm.SetAccountMetaInfo(ids[0], &common.AccountMetaInfo{
					ConsensusNodesHash: hashes[0],
				})
			},
			expectedHash: hashes[0],
		},
		{
			name: "fetch primary consensus nodes hash from db",
			id:   ids[0],
			preTestFn: func(sm *MockStateManager, ixPool *IxPool) {
				ixPool.consensusNodesHash.Add(ids[0], hashes[0])
			},
			expectedHash: hashes[0],
		},
		{
			name:          "sub-account not found",
			id:            subAccount,
			expectedError: errors.New("account meta info not found"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

			if testcase.preTestFn != nil {
				testcase.preTestFn(sm, ixPool)
			}

			ixHash, err := ixPool.getConsensusNodesHash(testcase.id)

			if testcase.expectedError != nil {
				require.ErrorContains(t, err, testcase.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.expectedHash, ixHash)
		})
	}
}

// Each interaction is represented by a group of participants,
// participants are represented by numbers to draw a comparison on the lexicographic order or ids
// 999 is used to represent sarga account.
// 0-100 is reserved for primary accounts and 101-200 reserved for sub accounts
func TestIsEligibleForProposal(t *testing.T) {
	testcases := []struct {
		name                   string
		input                  [][]int
		preTestFn              func(sm *MockStateManager, ixPool *IxPool, ixns []*common.Interaction)
		expectedEligibleInputs [][]int
		expectedEligibility    []bool
	}{
		{
			name:  "ixns from sub accounts to logic account",
			input: [][]int{{101, 0}, {102, 0}, {103, 0}, {104, 0}, {105, 0}, {106, 0}, {107, 0}},
			preTestFn: func(sm *MockStateManager, ixPool *IxPool, ixns []*common.Interaction) {
				logicID := ixns[0].IxParticipants()[1].ID

				for _, ix := range ixns {
					for id := range ix.Participants() {
						if id.IsParticipantVariant() {
							sm.SetAccountMetaInfo(id, &common.AccountMetaInfo{
								InheritedAccount: logicID,
							})
						}
					}
				}
			},
			expectedEligibleInputs: [][]int{{101, 0}, {102, 0}, {103, 0}, {104, 0}, {105, 0}, {106, 0}, {107, 0}},
			expectedEligibility:    []bool{true, true, true, true, true, true, true},
		},
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
			name:                   "max unique consensus nodes hash per leader - multiple senders",
			input:                  [][]int{{0, 1}, {0, 2}, {2, 0}, {3, 0}, {4, 0}, {5, 4}},
			expectedEligibleInputs: [][]int{{0, 1}, {0, 2}, {2, 0}, {3, 0}, {5, 4}},
			expectedEligibility:    []bool{true, true, true, true, false, true},
		},
		{
			name:                   "max unique consensus nodes hash per leader - single sender",
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
				participantToAcquirer         = make(map[identifiers.Identifier]identifiers.Identifier)
				acquirerToConsensusNodeHashes = make(map[identifiers.Identifier]map[common.Hash]struct{})
				registry                      = newBatchRegistry()
			)

			ixns := createIxnsFromParticipants(t, testcase.input)
			eligibility := make([]bool, len(ixns))

			sm := NewMockStateManager(t)
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			if testcase.preTestFn != nil {
				testcase.preTestFn(sm, ixPool, ixns)
			}

			// generate random consensus nodes hash for each identifier
			for _, ixn := range ixns {
				for id := range ixn.Participants() {
					if !id.IsParticipantVariant() {
						ixPool.consensusNodesHash.Add(id, tests.RandomHash(t))
					}
				}
			}

			for i, ix := range ixns {
				index := ixPool.isEligibleForProposal(ix, participantToAcquirer, acquirerToConsensusNodeHashes, registry)
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
	sm.SetAccountMetaInfo(common.SargaAccountID, &common.AccountMetaInfo{
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
					t, common.IxAssetAction, common.NewTransferParams(identifiers.Nil, big.NewInt(1)),
					0, tests.RandomIdentifier(t), 0, nil,
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
					0, tests.RandomIdentifier(t), 0, nil,
				),
				newTestInteraction(t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					0, tests.RandomIdentifier(t), 0, nil,
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

// FIXME: this test is flaky
func TestIxPool_validateAndEnqueueIx_ReplaceIx(t *testing.T) {
	var (
		price = 1
		sm    = NewMockStateManager(t)
		ids   = tests.GetIdentifiers(t, 2)
	)

	tp := &common.TransferParams{
		Beneficiary: ids[1],
		Amount:      big.NewInt(1),
	}

	ap, err := common.GetAssetActionPayload(common.KMOITokenAssetID, common.TransferEndpoint, tp)
	require.NoError(t, err)

	sm.SetAccountMetaInfo(ids[0], &common.AccountMetaInfo{
		PositionInContextSet: 1,
	})
	sm.SetAccountMetaInfo(tp.Beneficiary, &common.AccountMetaInfo{
		PositionInContextSet: 1,
	})

	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 2))
	sm.registerAccounts(tp.Beneficiary)

	getIxParams := func(
		from identifiers.Identifier, sequenceID uint64,
		price int, payload any, noOfTxs int,
	) *tests.CreateIxParams {
		return &tests.CreateIxParams{
			IxDataCallback: func(ix *common.IxData) {
				ix.Sender = common.Sender{
					ID:         from,
					SequenceID: sequenceID,
				}
				ix.FuelPrice = big.NewInt(int64(price))
				ix.FuelLimit = 1
				ix.IxOps = make([]common.IxOpRaw, 0)

				tests.AddParticipants(t, ix, []common.IxParticipant{
					{
						ID:       from,
						LockType: common.MutateLock,
					},
					{
						ID:       tp.Beneficiary,
						LockType: common.MutateLock,
					},
				}...)

				for i := 0; i < noOfTxs; i++ {
					tests.AddIxOp(t, ix, common.IxAssetAction, common.KMOITokenAssetID, payload)
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
			ix:   tests.CreateIX(t, getIxParams(ids[0], 1, price, ap, 20)),
			preTestFn: func(ixPool *IxPool) {
				ixn := tests.CreateIX(t, getIxParams(ids[0], 1, price, ap, 1))
				sm.registerIxParticipants(ixn)

				err = ixPool.validateAndEnqueueIx(ixn)
				require.NoError(t, err)
			},
			// TODO: Avoid hardcoding the expected gauge size
			expectedGaugeSize: 3,
		},
		{
			name: "successfully replace ixn in promoted queue",
			ix:   tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 20)),
			preTestFn: func(ixPool *IxPool) {
				ixn := tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 1))
				sm.registerIxParticipants(ixn)

				err = ixPool.validateAndEnqueueIx(ixn)
				require.NoError(t, err)
			},
			replaceInPromote:  true,
			expectedGaugeSize: 3,
		},
		{
			name: "successfully replace with lower size ixn",
			ix:   tests.CreateIX(t, getIxParams(ids[0], 1, price, ap, 1)),
			preTestFn: func(ixPool *IxPool) {
				ixn := tests.CreateIX(t, getIxParams(ids[0], 1, price, ap, 30))
				sm.registerIxParticipants(ixn)

				err := ixPool.validateAndEnqueueIx(ixn)
				require.NoError(t, err)
			},
			expectedGaugeSize: 1,
		},
		{
			name: "failed to replace with cheaper ixn",
			ix:   tests.CreateIX(t, getIxParams(ids[0], 0, 0, ap, 1)),
			preTestFn: func(ixPool *IxPool) {
				ixn := tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 1))
				sm.registerIxParticipants(ixn)

				err = ixPool.validateAndEnqueueIx(ixn)
				require.NoError(t, err)
			},
			expectedErr: common.ErrUnderpriced,
		},
		{
			name: "failed to replace with same ixn",
			ix:   tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 1)),
			preTestFn: func(ixPool *IxPool) {
				ixn := tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 1))
				sm.registerIxParticipants(ixn)

				err = ixPool.validateAndEnqueueIx(ixn)
				require.NoError(t, err)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "failed to replace with ixn which will overflow ixpool",
			ix:   tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 30)),
			preTestFn: func(ixPool *IxPool) {
				ixn := tests.CreateIX(t, getIxParams(ids[0], 0, price, ap, 1))
				sm.registerIxParticipants(ixn)

				err = ixPool.validateAndEnqueueIx(ixn)
				require.NoError(t, err)

				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots - 1)
			},
			expectedErr: common.ErrIXPoolOverFlow,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sender := test.ix.SenderID()
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			ixPool.getOrCreateAccountQueue(sender, 0, 0)

			if test.preTestFn != nil {
				test.preTestFn(ixPool)
			}

			sm.registerIxParticipants(test.ix)

			err = ixPool.validateAndEnqueueIx(test.ix)

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
	ids := tests.GetIdentifiers(t, 1)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 1))

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(ixPool *IxPool, interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "Interaction with invalid id",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				0, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				interaction.SetSender(common.Sender{
					ID: identifiers.Nil,
				})
			},
			expectedErr: common.ErrInvalidIdentifier,
		},
		{
			name: "Already known Interaction",
			ix: newTestInteraction(
				t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
				0, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				sm.registerIxParticipants(interaction)

				err := ixPool.validateAndEnqueueIx(interaction)
				require.NoError(t, err)
			},
			expectedErr: ErrAlreadyKnown,
		},
		{
			name: "New valid interaction",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				0, ids[0], 0, nil,
			),
		},
		{
			name: "ixpool overflow",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				0, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots)
			},
			expectedErr: common.ErrIXPoolOverFlow,
		},
		{
			name: "fill ixpool to the limit",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				0, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase(config.DefaultMaxIXPoolSlots - 1)
			},
		},
		{
			name: "failed to add lower sequenceID ixn",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				0, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.getOrCreateAccountQueue(ids[0], 0, 1)
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
			}, true, sm, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixPool, test.ix)
			}

			for id, ps := range test.ix.Participants() {
				if ps.IsGenesis {
					continue
				}

				sm.registerAccounts(id)
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
	ids := tests.GetIdentifiers(t, 2)
	sm.setTestMOIBalance(t, ids[0])
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 2))

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
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, ids[1]),
					0, ids[0], 0, nil,
				),
				newIxWithPayload(t, common.IxAssetCreate, 5, ids[0], make([]byte, IxMaxSize+2)),
			},
			expectedAddedIxns: 1,
			expectedResult:    pubsub.ValidationReject,
		},
		{
			name: "Reject interactions with invalid id",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
					0, ids[0], 0, nil,
				),
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
					0, ids[0], 0, nil,
				),
			},
			preTestFn: func(interactions []*common.Interaction) {
				interactions[1].SetSender(common.Sender{
					ID: identifiers.Nil,
				})
			},
			expectedAddedIxns: 1,
			expectedResult:    pubsub.ValidationReject,
		},
		{
			name: "Reject interactions with invalid signature",
			ixns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
					0, ids[0], 0, nil,
				),
			},
			sigVerification: true,
			expectedResult:  pubsub.ValidationReject,
		},
		{
			name:              "valid ixn group",
			ixns:              createTestIxs(t, 0, 2, ids[0]),
			expectedAddedIxns: 2,
			expectedResult:    pubsub.ValidationAccept,
		},
		{
			name: "Ignore ixns as more than half of them have errors and count greater than 10",
			ixns: append(
				createTestIxs(t, 0, 5, ids[0]),
				createTestIxs(t, 0, 6, ids[1])...,
			),
			expectedAddedIxns: 5,
			expectedResult:    pubsub.ValidationIgnore,
		},
		{
			name: "accept ixns as less than half of them have errors and count greater than 10",
			ixns: append(
				createTestIxs(t, 0, 6, ids[0]),
				createTestIxs(t, 0, 5, ids[1])...,
			),
			expectedAddedIxns: 6,
			expectedResult:    pubsub.ValidationAccept,
		},
		{
			name:           "accept ixns if length of ixns less than or equal to 10 and all ixns have errors",
			ixns:           createTestIxs(t, 0, 10, ids[1]),
			expectedResult: pubsub.ValidationAccept,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, !test.sigVerification, sm, nil)

			if test.preTestFn != nil {
				test.preTestFn(test.ixns)
			}

			sm.registerIxParticipants(test.ixns...)

			res := ixPool.AddRemoteInteractions(test.ixns...)
			require.Equal(t, test.expectedResult, res)
			require.Equal(t, test.expectedAddedIxns, len(ixPool.allIxs.all))
		})
	}
}

func TestIxPool_AddLocalInteractions_Broadcast(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 2)
	sm.setTestMOIBalance(t, ids[0])
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 2))

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
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, ids[1]),
					0, ids[0], 0, nil,
				),
				newTestInteraction(
					t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
					0, ids[0], 0, nil,
				),
			},
			preTestFn: func(interactions []*common.Interaction) {
				interactions[1].SetSender(common.Sender{
					ID: identifiers.Nil,
				})
			},
			broadcastedIxns: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, ids[1]),
					0, ids[0], 0, nil,
				),
			},
		},
		{
			name: "do not broadcast zero ixns",
			ixns: createTestIxs(t, 0, 2, ids[1]),
		},
		{
			name:             "do not broadcast ixns if flooding enabled",
			enableIxFlooding: true,
			ixns:             createTestIxs(t, 0, 2, ids[0]),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
				c.EnableIxFlooding = test.enableIxFlooding
			}, true, sm, newMockNetwork(""))

			if test.preTestFn != nil {
				test.preTestFn(test.ixns)
			}

			sm.registerIxParticipants(test.ixns...)

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
	ids := tests.GetIdentifiers(t, 2)
	sm.setTestMOIBalance(t, ids[0])
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 2))

	validIxns := createTestIxs(t, 0, 2, ids[0])
	rawData, err := polo.Polorize(validIxns)
	require.NoError(t, err)

	sm.registerIxParticipants(validIxns...)

	zeroIxns := []*common.Interaction{}
	zeroIxnsRawData, err := polo.Polorize(zeroIxns)
	require.NoError(t, err)

	excessIxns := createTestIxs(t, 0, 3, ids[0])
	excessIxnsRawData, err := polo.Polorize(excessIxns)
	require.NoError(t, err)

	sm.registerIxParticipants(excessIxns...)

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
			}, true, sm, newMockNetwork(KID))

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
	ids := tests.GetIdentifiers(t, 1)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 1))

	testcases := []struct {
		name string
		ixn  []*common.Interaction
	}{
		{
			name: "check for add ixn event",
			ixn: []*common.Interaction{
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
					0, ids[0], 0, nil,
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
			}, true, sm, nil)

			ixAddedEventSub := ixPool.mux.Subscribe(utils.AddedInteractionEvent{})

			ixAddedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixAddedEventSub, ixAddedResp, len(test.ixn))

			sm.registerIxParticipants(test.ixn[0])

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
	ids := tests.GetIdentifiers(t, 1)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 1))

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
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				0, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.gauge.increase((80 * config.DefaultMaxIXPoolSlots / 100) + 1)
			},
			expectedPromotedLength: 1,
		},
		{
			name: "reject the future ix",
			ix: newTestInteraction(
				t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
				1, ids[0], 0, nil,
			),
			preTestFn: func(ixPool *IxPool, interaction *common.Interaction) {
				ixPool.getOrCreateAccountQueue(interaction.SenderID(), 0, 0)
				ixPool.gauge.increase((80 * config.DefaultMaxIXPoolSlots / 100) + 1)
			},
			expectedError: common.ErrRejectFutureIx,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sender := test.ix.SenderID()
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixPool, test.ix)
			}

			sm.registerIxParticipants(test.ix)

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
	id := tests.RandomIdentifier(t)
	sm := NewMockStateManager(t)
	sm.setBalance(t, id, common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(1000))
	sm.setAccountKeysAndPublicKeys(t, []identifiers.Identifier{id}, [][]byte{tests.GetTestPublicKey(t)})

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
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
					0, id, 0, nil,
				),
				newTestInteraction(
					t, common.IxParticipantCreate, tests.CreateParticipantCreatePayload(t, identifiers.Nil),
					5, id, 0, nil,
				),
			},
			expectedResult: expectedResult{
				enqueued:         1,
				promotedAccounts: 1,
			},
		},
		{
			name: "All the ixs are with low sequenceID",
			ixs:  createTestIxs(t, 0, 5, id),
			testFn: func(ixPool *IxPool, interactions []*common.Interaction) {
				ixPool.getOrCreateAccountQueue(interactions[0].SenderID(), 0, 5)
			},
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 0,
			},
			expectedErrors: 5,
		},
		{
			name: "Should not enqueue ixs with low sequenceID",
			ixs:  createTestIxs(t, 0, 6, id),
			testFn: func(ixPool *IxPool, interactions []*common.Interaction) {
				ixPool.getOrCreateAccountQueue(interactions[0].SenderID(), 0, 5)
			},
			expectedResult: expectedResult{
				enqueued:         0,
				promotedAccounts: 1,
			},
			expectedErrors: 5,
		},
		{
			name: "Promote ixs with expected sequenceID",
			ixs:  createTestIxs(t, 0, 3, id),
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
			}, true, sm, newMockNetwork(""))
			senderID := testcase.ixs[0].SenderID()

			if testcase.testFn != nil {
				testcase.testFn(ixPool, testcase.ixs)
			}

			require.Equal(t, uint64(0), ixPool.gauge.read())

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Equal(t, testcase.expectedErrors, len(errs))

			require.Equal(t, testcase.expectedResult.enqueued,
				ixPool.accounts.getAccountQueue(senderID, 0).enqueued.length())
			require.Equal(t, testcase.expectedResult.promotedAccounts,
				ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())
			require.Equal(t,
				testcase.expectedResult.enqueued+testcase.expectedResult.promotedAccounts, ixPool.gauge.read())
		})
	}
}

func TestIxPool_handlePromoteRequest(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 3)

	ids[0] = identifiers.RandomParticipantIDv0().AsIdentifier()

	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 3))
	sm.SetAccountMetaInfo(common.SargaAccountID, &common.AccountMetaInfo{
		PositionInContextSet: 1,
	})

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, newMockNetwork(""))
	ixPool.UpdateCurrentView(4)

	testcases := []struct {
		name                  string
		ixs                   []*common.Interaction
		popIx                 func(id identifiers.Identifier)
		shouldPropose         bool
		expected              expectedResult
		expectedAllocatedView uint64
	}{
		{
			name: "Promote one ix",
			ixs:  createTestIxs(t, 0, 1, ids[0]),
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
			ixs:           createTestIxs(t, 0, 3, ids[1]),
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
			senderID := testcase.ixs[0].SenderID()

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			if testcase.popIx != nil {
				testcase.popIx(senderID)
			}

			// checks whether the account's sequenceID is updated
			require.Equal(t, testcase.expected.nonce, ixPool.accounts.getAccountQueue(senderID, 0).getSequenceID())
			// checks whether the ixs are removed from the enqueue
			require.Equal(t, testcase.expected.enqueued, ixPool.accounts.getAccountQueue(senderID, 0).enqueued.length())
			// checks whether the ixs are promoted
			require.Equal(t, testcase.expected.promoted, ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())

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
	ids := tests.GetIdentifiers(t, 1)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 1))

	testcases := []struct {
		name string
		ixs  []*common.Interaction
	}{
		{
			name: "check for promoted ixn event",
			ixs:  createTestIxs(t, 0, 1, ids[0]),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

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
	}, true, sm, nil)

	testcases := []struct {
		name          string
		id            identifiers.Identifier
		nonce         uint64
		testFn        func(id identifiers.Identifier)
		expectedNonce uint64
	}{
		{
			name:  "Create an account with latest state object sequenceID",
			id:    tests.RandomIdentifier(t),
			nonce: 1,
			testFn: func(id identifiers.Identifier) {
				sm.setLatestSequenceID(t, id, 0, 5)
			},
			expectedNonce: 5,
		},
		{
			name:          "Create an account with given sequenceID as the state object doesn't exists",
			id:            tests.RandomIdentifier(t),
			nonce:         2,
			expectedNonce: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if testcase.testFn != nil {
				testcase.testFn(testcase.id)
			}

			_, accountKey := ixPool.getOrCreateAccountQueue(testcase.id, 0, testcase.nonce)
			require.NotNil(t, accountKey)
			// checks whether an account is created with expected sequenceID
			require.Equal(t, testcase.expectedNonce, accountKey.getSequenceID())
		})
	}
}

func TestIxPool_ResetWithHeaders_resetWaitTime(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 3)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 3))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, newMockNetwork(""))

	testcases := []struct {
		name               string
		ixs                []*common.Interaction
		nonce              int
		incrementCounter   func(acc *account)
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the interactions with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, ids[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "Prune some interactions with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, ids[1]),
			nonce:              1,
			expectedPromotions: 3,
		},
		{
			name:  "Reset wait time",
			ixs:   createTestIxs(t, 0, 5, ids[2]),
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
			senderID := testcase.ixs[0].SenderID()

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			if testcase.incrementCounter != nil {
				testcase.incrementCounter(ixPool.accounts.getAccount(senderID))
				// check whether the delay counter is updated
				require.Equal(t, int32(1), ixPool.accounts.getAccount(senderID).delayCounter)
			}

			ts := getTesseractWithIxs(t, senderID, testcase.nonce)

			// reset with headers removes the interactions with sequenceID lesser than given tesseract's interaction sequenceID
			// from the queues and resets the delay counter to default value
			ixPool.ResetWithHeaders(ts)

			require.Equal(t, int32(0), ixPool.accounts.getAccount(senderID).delayCounter)
			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_enqueued(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 3)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 3))

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectedEnqueued uint64
		expectedPromoted uint64
	}{
		{
			name:             "Prune all ixs with low sequenceID",
			ixs:              createTestIxs(t, 1, 5, ids[0]),
			nonce:            5,
			expectedEnqueued: 0,
		},
		{
			name:             "No low sequenceID ixs to prune",
			ixs:              createTestIxs(t, 1, 7, ids[1])[2:6],
			nonce:            1,
			expectedEnqueued: 4,
		},
		{
			name:             "Prune some ixs with low sequenceID",
			ixs:              createTestIxs(t, 1, 6, ids[2]),
			nonce:            3,
			expectedPromoted: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderID := testcase.ixs[0].SenderID()

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

			ixPool.getOrCreateAccountQueue(senderID, 0, 0)

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderID, int(testcase.nonce))

			// On reset should prune the ixs from the enqueue if the sequenceID is lesser than the given sequenceID
			ixPool.ResetWithHeaders(ts)

			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

			require.Equal(t, testcase.expectedEnqueued, ixPool.accounts.getAccountQueue(senderID, 0).enqueued.length())
			require.Equal(t, testcase.expectedPromoted, ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_prunedEnqueuedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 2)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 2))

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some enqueue ixs with low sequenceID and check for events",
			ixs:              createTestIxs(t, 1, 6, ids[1]),
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
			}, true, sm, newMockNetwork(""))
			senderID := testcase.ixs[0].SenderID()

			ixPool.getOrCreateAccountQueue(senderID, 0, 0)

			ixPrunedEnqueueEventSub := ixPool.mux.Subscribe(utils.PrunedEnqueuedInteractionEvent{})

			ixPrunedEnqueueResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedEnqueueEventSub, ixPrunedEnqueueResp, len(testcase.ixs))

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			// ResetWithHeaders should prune the ixs from the enqueue if the sequenceID is lesser than the given sequenceID
			ts := getTesseractWithIxs(t, senderID, int(testcase.nonce))
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
	ids := tests.GetIdentifiers(t, 3)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 3))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, newMockNetwork(""))

	testcases := []struct {
		name               string
		ixs                []*common.Interaction
		nonce              uint64
		expectedPromotions uint64
	}{
		{
			name:               "Prune all the ixs with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, ids[0]),
			nonce:              5,
			expectedPromotions: 0,
		},
		{
			name:               "No low sequenceID ixs to prune",
			ixs:                createTestIxs(t, 0, 6, ids[1])[1:6],
			nonce:              0,
			expectedPromotions: 5,
		},
		{
			name:               "Prune some ixs with low sequenceID",
			ixs:                createTestIxs(t, 0, 5, ids[2]),
			nonce:              3,
			expectedPromotions: 1,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderID := testcase.ixs[0].SenderID()

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderID, int(testcase.nonce))
			// On reset should prune the ixs from the promoted queue if the sequenceID is lesser than the given sequenceID
			ixPool.ResetWithHeaders(ts)

			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())
		})
	}
}

func TestIxPool_ResetWithHeaders_prunedPromotedIxnEvent(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 2)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 2))

	testcases := []struct {
		name             string
		ixs              []*common.Interaction
		nonce            uint64
		expectEventCount int
	}{
		{
			name:             "Prune some promoted ixs with low sequenceID and check for events",
			ixs:              createTestIxs(t, 0, 5, ids[1]),
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
			}, true, sm, newMockNetwork(""))

			ixPrunedPromotedEventSub := ixPool.mux.Subscribe(utils.PrunedPromotedInteractionEvent{})

			ixPrunedPromotedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixPrunedPromotedEventSub, ixPrunedPromotedResp, len(testcase.ixs))

			senderID := testcase.ixs[0].SenderID()

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			ts := getTesseractWithIxs(t, senderID, int(testcase.nonce))
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
	ids := tests.GetIdentifiers(t, 1)

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
				createTestIxs(t, 0, 3, ids[0]),
				// enqueued
				createTestIxs(t, 4, 7, ids[0])...,
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
				createTestIxs(t, 6, 8, ids[0]),
				// enqueued
				createTestIxs(t, 10, 13, ids[0])...,
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
				createTestIxs(t, 1, 3, ids[0]),
				// enqueued
				createTestIxs(t, 5, 7, ids[0])...,
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
				createTestIxs(t, 0, 3, ids[0]),
				// enqueued
				createTestIxs(t, 4, 7, ids[0])...,
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
			sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 1))

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

			senderID := testcase.ixs[0].SenderID()
			// ixPool.createAccountOnce(senderID, 0)

			require.Equal(t, uint64(0), ixPool.gauge.read())

			addAndProcessIxs(t, sm, ixPool, testcase.ixs...)
			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			acc := ixPool.accounts.getAccountQueue(senderID, 0)
			require.Equal(t, len(testcase.ixs), len(acc.sequenceIDToIX.mapping))

			ixPool.consensusNodesHash.Add(senderID, tests.RandomHash(t))

			ts := getTesseractWithIxs(t, senderID, int(testcase.nonce))

			ixPool.ResetWithHeaders(ts)

			ixPool.mu.RLock()
			defer ixPool.mu.RUnlock()

			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, ixPool.gauge.read())

			acc = ixPool.accounts.getAccountQueue(senderID, 0)
			require.Equal(t, testcase.expected.enqueued, acc.enqueued.length())
			require.Equal(t, testcase.expected.promoted, acc.promoted.length())
			require.Equal(t, testcase.expected.enqueued+testcase.expected.promoted, uint64(len(acc.sequenceIDToIX.mapping)))

			_, ok := ixPool.consensusNodesHash.Get(senderID)
			require.False(t, ok)
		})
	}
}

func TestIxPool_Pop(t *testing.T) {
	id1 := tests.RandomIdentifier(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, id1)
	sm.setAccountKeysAndPublicKeys(t, []identifiers.Identifier{id1}, tests.GetTestPublicKeys(t, 1))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, newMockNetwork(""))

	testcases := []struct {
		name               string
		ixs                []*common.Interaction
		expectedPromotions uint64
	}{
		{
			name:               "Prune the ix from the promoted queue",
			ixs:                createTestIxs(t, 0, 5, id1),
			expectedPromotions: 4,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			senderID := testcase.ixs[0].SenderID()

			require.Equal(t, uint64(0), ixPool.gauge.read())

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())

			ix := ixPool.accounts.getAccountQueue(senderID, 0).promoted.peek()

			ixPool.Pop(ix)
			require.Equal(t, testcase.expectedPromotions, ixPool.gauge.read())
			require.Equal(t, testcase.expectedPromotions, ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())
		})
	}
}

func TestIxPool_Drop(t *testing.T) {
	id1 := tests.RandomIdentifier(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, id1)
	sm.setAccountKeysAndPublicKeys(t, []identifiers.Identifier{id1}, tests.GetTestPublicKeys(t, 1))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, newMockNetwork(""))

	testcases := []struct {
		name string
		ixs  []*common.Interaction
	}{
		{
			name: "Remove the account form accounts map and check for dropped events",
			ixs:  createTestIxs(t, 0, 5, id1),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixDroppedEventSub := ixPool.mux.Subscribe(utils.DroppedInteractionEvent{})

			ixDroppedResp := make(chan tests.Result, 1)

			ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
			defer cancel()

			go utils.HandleMuxEvents(ctx, ixDroppedEventSub, ixDroppedResp, len(testcase.ixs))

			senderID := testcase.ixs[0].SenderID()

			sm.registerIxParticipants(testcase.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.ixs...))
			require.Len(t, errs, 0)

			acc := ixPool.accounts.getAccountQueue(senderID, 0)
			ix := acc.promoted.peek()

			require.Equal(t, uint64(len(testcase.ixs)), ixPool.gauge.read())
			require.Equal(t, uint64(len(testcase.ixs)), uint64(len(acc.sequenceIDToIX.mapping)))

			ixPool.Drop(ix)
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())

			data := tests.WaitForResponse(t, ixDroppedResp, utils.DroppedInteractionEvent{})
			event, ok := data.(utils.DroppedInteractionEvent)
			require.True(t, ok)
			require.Equal(t, testcase.ixs, event.Ixs)

			require.Equal(t, 0, len(acc.sequenceIDToIX.mapping))
		})
	}
}

func TestIxPool_Drop_FinalizedIx(t *testing.T) {
	id1 := tests.RandomIdentifier(t)
	sm := NewMockStateManager(t)
	sm.setTestMOIBalance(t, id1)
	sm.setAccountKeysAndPublicKeys(t, []identifiers.Identifier{id1}, tests.GetTestPublicKeys(t, 1))

	ixs := createTestIxs(t, 0, 5, id1)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
		c.MaxSlots = config.DefaultMaxIXPoolSlots
	}, true, sm, newMockNetwork(""))

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
				sm.setLatestSequenceID(t, id1, 0, 1)
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

			senderID := testcase.pendingIxs[0].SenderID()

			sm.registerIxParticipants(testcase.pendingIxs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, testcase.pendingIxs...))
			require.Len(t, errs, 0)

			acc := ixPool.accounts.getAccountQueue(senderID, 0)
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
	}, false, sm, nil)

	err := ixPool.IncrementWaitTime(identifiers.Identifier{0x0}, 1500*time.Millisecond)
	require.Error(t, err)
}

func TestIxPool_IncrementWaitTime(t *testing.T) {
	testcases := []struct {
		name            string
		id              identifiers.Identifier
		delta           int
		shouldReset     bool
		expectedCounter int32
	}{
		{
			name:            "Increment the wait counter by 1",
			id:              identifiers.Identifier{0x01},
			delta:           1,
			shouldReset:     false,
			expectedCounter: 1,
		},
		{
			name:            "Increment the wait counter by 5",
			id:              identifiers.Identifier{0x02},
			delta:           5,
			shouldReset:     false,
			expectedCounter: 5,
		},

		{
			name:            "Wait counter greater than max value",
			id:              identifiers.Identifier{0x03},
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
			acc, _ := ixPool.getOrCreateAccountQueue(testcase.id, 0, 0)

			for i := 0; i < testcase.delta; i++ {
				require.NoError(t, ixPool.IncrementWaitTime(testcase.id, baseTime))

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
	ids := tests.GetIdentifiers(t, 3)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 3))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, true, sm, nil)

	invalidAssetID := identifiers.AssetID(ids[0])
	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name:        "Oversized data error",
			ix:          newIxWithPayload(t, common.IxAssetCreate, 5, ids[0], make([]byte, IxMaxSize+2)),
			expectedErr: ErrOversizedData,
		},
		{
			name: "Invalid id error",
			ix: newTestInteraction(
				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
				0, tests.RandomIdentifier(t), 0, nil,
			),
			testFn: func(interaction *common.Interaction) {
				interaction.SetSender(common.Sender{
					ID: identifiers.Nil,
				})
			},
			expectedErr: common.ErrInvalidIdentifier,
		},
		{
			name: "SequenceID too low error",
			ix: newTestInteraction(
				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
				9, ids[1], 0, nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.setLatestSequenceID(t, interaction.SenderID(), 0, 10)
			},
			expectedErr: ErrSequenceIDTooLow,
		},
		{
			name:        "Underpriced error",
			ix:          newIxWithFuelPrice(t, 5, ids[0], 50),
			expectedErr: common.ErrUnderpriced,
		},
		{
			name: "Ix with invalid assetID",
			ix: newTestInteraction(
				t, common.IxAssetAction,
				&common.AssetActionPayload{
					AssetID:  invalidAssetID,
					Callsite: common.TransferEndpoint,
				},
				0, ids[0], 0, nil),
			expectedErr: common.ErrInvalidAssetID,
		},
		{
			name: "Ix with insufficient funds",
			ix: newTestInteraction(
				t, common.IxAssetAction,
				&common.AssetActionPayload{
					AssetID:  identifiers.RandomAssetIDv0(),
					Callsite: common.TransferEndpoint,
				},
				0, ids[0], 0, nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.balance[interaction.SenderID()] = map[identifiers.AssetID]map[common.TokenID]*big.Int{
					identifiers.RandomAssetIDv0(): {common.DefaultTokenID: big.NewInt(0)},
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

			sm.registerIxParticipants(testcase.ix)

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

	id, mnemonic := tests.RandomIDWithMnemonic(t)
	id2 := tests.RandomIdentifier(t)

	_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, config.DefaultMoiWalletPath)
	require.NoError(t, err)

	sm.setTestMOIBalance(t, id, id2)
	sm.setAccountKeysAndPublicKeys(t, []identifiers.Identifier{id, id2}, [][]byte{publicKey, publicKey})

	ixArgs := common.IxData{
		Sender: common.Sender{
			ID: id,
		},
		FuelPrice: defaultIxPriceLimit,
		FuelLimit: 1,
		IxOps: []common.IxOpRaw{
			{
				Type: common.IxAssetAction,
				Payload: func() []byte {
					transferPayload, _ := common.GetAssetActionPayload(
						common.KMOITokenAssetID,
						common.TransferEndpoint,
						&common.TransferParams{
							Beneficiary: id2,
							Amount:      big.NewInt(5),
						},
					)

					payload, _ := transferPayload.Bytes()

					return payload
				}(),
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       common.KMOITokenAssetID.AsIdentifier(),
				LockType: common.NoLock,
			},
			{
				ID:       id2,
				LockType: common.MutateLock,
			},
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	}

	rawSign := tests.GetIXSignature(t, &ixArgs, mnemonic)

	ix := tests.CreateIX(t, getIXParams(t,
		id,
		common.IxAssetAction,
		defaultIxPriceLimit,
		common.KMOITokenAssetID,
		&common.TransferParams{
			Beneficiary: id2,
			Amount:      big.NewInt(5),
		},
		rawSign,
	))

	sm.registerAccounts(id2)
	testcases := []struct {
		name        string
		ix          *common.Interaction
		testFn      func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "invalid signature",
			ix: newTestInteraction(
				t, common.IxAssetAction,
				tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
				0, id2, 0,
				nil,
			),
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(t, interaction.SenderID(), identifiers.RandomAssetIDv0(), common.DefaultTokenID, big.NewInt(10))
			},
			expectedErr: common.ErrInvalidIXSignature,
		},
		{
			name: "valid signature",
			ix:   ix,
			testFn: func(interaction *common.Interaction) {
				sm.setBalance(t, interaction.SenderID(), common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(10))
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.ix)
			}

			sm.registerIxParticipants(test.ix)

			err = ixPool.validateIx(test.ix)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

// func TestIxPool_ValidateFunds(t *testing.T) {
//	id := tests.RandomIdentifier(t)
//	sm := NewMockStateManager(t)
//
//	sm.setBalance(t, id, common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(10))
//
//	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
//		c.Mode = WaitMode
//		c.PriceLimit = defaultIxPriceLimit
//	}, false, sm, nil)
//
//	testcases := []struct {
//		name        string
//		ix          *common.Interaction
//		expectedErr error
//	}{
//		{
//			name: "negative asset fund",
//			ix: newTestInteraction(
//				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
//				0, id, 0, func(ixData *common.IxData) {
//					ixData.Funds = []common.IxFund{
//						{
//							AssetID: common.KMOITokenAssetID,
//							Amount:  new(big.Int).Neg(big.NewInt(20)),
//						},
//					}
//				},
//			),
//			expectedErr: common.ErrInvalidValue,
//		},
//		{
//			name: "account not registered",
//			ix: newTestInteraction(
//				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
//				0, tests.RandomIdentifier(t), 0, nil,
//			),
//			expectedErr: common.ErrFetchingBalance,
//		},
//		{
//			name: "insufficient asset funds",
//			ix: newTestInteraction(
//				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
//				0, id, 0, func(ixData *common.IxData) {
//					ixData.Funds = []common.IxFund{
//						{
//							AssetID: common.KMOITokenAssetID,
//							Amount:  big.NewInt(20),
//						},
//					}
//				},
//			),
//			expectedErr: common.ErrInsufficientFunds,
//		},
//		{
//			name: "valid asset funds",
//			ix: newTestInteraction(
//				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
//				0, id, 0, func(ixData *common.IxData) {
//					ixData.Funds = []common.IxFund{
//						{
//							AssetID: common.KMOITokenAssetID,
//							Amount:  big.NewInt(5),
//						},
//					}
//				},
//			),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			err := ixPool.validateFunds(test.ix)
//			if test.expectedErr != nil {
//				require.ErrorContains(t, err, test.expectedErr.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}

func TestIxPool_ValidateOperations(t *testing.T) {
	id := tests.RandomIdentifier(t)
	sm := NewMockStateManager(t)

	sm.setBalance(t, id, common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(10))

	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil)

	validAssetPayload := &common.AssetCreatePayload{
		Symbol:   "&&&&",
		Standard: common.MAS1,
	}

	invalidAssetPayload := &common.AssetCreatePayload{
		Symbol:   "&&&&",
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
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, id, 0, nil),
			testFn: func(ix *common.Interaction) {
				ix.GetIxOp(0).OpType = common.IxInvalid
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name:        "ix with invalid operation payload",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetPayload, 0, id, 0, nil),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name: "ix with valid operation payload",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, id, 0, nil),
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
	}, false, nil, nil)

	id := tests.RandomIdentifier(t)
	validAssetPayload := &common.AssetCreatePayload{
		Symbol:    "ABC",
		Standard:  common.MAS1,
		MaxSupply: big.NewInt(10),
	}

	invalidAssetStandardPayload := &common.AssetCreatePayload{
		Symbol:   "@123",
		Standard: 2,
	}

	missingLogicPayload := &common.AssetCreatePayload{
		Symbol:   "123",
		Standard: common.MASX,
		Logic:    nil,
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name:        "should return error if asset standard is invalid",
			ix:          newTestInteraction(t, common.IxAssetCreate, invalidAssetStandardPayload, 0, id, 0, nil),
			expectedErr: common.ErrInvalidAssetStandard,
		},
		{
			name:        "should return error if logic payload is missing for MASX standard",
			ix:          newTestInteraction(t, common.IxAssetCreate, missingLogicPayload, 0, id, 0, nil),
			expectedErr: common.ErrInvalidLogicPayload,
		},
		{
			name: "should return success if asset standard is valid",
			ix:   newTestInteraction(t, common.IxAssetCreate, validAssetPayload, 0, id, 0, nil),
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

func TestIxPool_ValidateAccountInherit(t *testing.T) {
	sm := NewMockStateManager(t)
	ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
		c.Mode = WaitMode
		c.PriceLimit = defaultIxPriceLimit
	}, false, sm, nil)

	id := tests.RandomIdentifierWithZeroVariant(t)
	sm.registerAccounts(id)

	logicID, err := identifiers.GenerateLogicIDv0(identifiers.RandomFingerprint(), 0)
	require.NoError(t, err)

	accountInheritPayload := &common.AccountInheritPayload{
		TargetAccount:   logicID.AsIdentifier(),
		Value:           tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
		SubAccountIndex: 2,
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		expectedErr error
	}{
		{
			name: "should return success if payload is valid",
			ix:   newTestInteraction(t, common.IxAccountInherit, accountInheritPayload, 0, id, 0, nil),
		},
		{
			name: "sender should be primary account",
			ix: newTestInteraction(t, common.IxAccountInherit, &common.AccountInheritPayload{
				TargetAccount:   logicID.AsIdentifier(),
				Value:           tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
				SubAccountIndex: 2,
			}, 0, tests.RandomSubAccountIdentifier(t, 1), 0, nil),
			expectedErr: common.ErrSenderAccount,
		},
		{
			name: "target should be logic account",
			ix: newTestInteraction(t, common.IxAccountInherit, &common.AccountInheritPayload{
				TargetAccount:   tests.RandomIdentifier(t),
				Value:           tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
				SubAccountIndex: 2,
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidTargetAccount,
		},
		{
			name: "invalid asset id",
			ix: newTestInteraction(t, common.IxAccountInherit, &common.AccountInheritPayload{
				TargetAccount: logicID.AsIdentifier(),
				Value: &common.AssetActionPayload{
					AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				},
				SubAccountIndex: 2,
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidAssetID,
		},
		{
			name: "target is nil",
			ix: newTestInteraction(t, common.IxAccountInherit, &common.AccountInheritPayload{
				TargetAccount:   identifiers.Nil,
				Value:           tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
				SubAccountIndex: 2,
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidIdentifier,
		},
		// TODO: Improve tests
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := ixPool.validateAccountInherit(testcase.ix, 0)
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
	}, false, sm, nil)

	id := tests.RandomIdentifier(t)
	sm.registerAccounts(id)

	participantCreatePayload := &common.ParticipantCreatePayload{
		ID: tests.RandomIdentifier(t),
		KeysPayload: []common.KeyAddPayload{
			{
				PublicKey:          tests.RandomIdentifier(t).Bytes(),
				Weight:             1000,
				SignatureAlgorithm: 0,
			},
		},
		Value: tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		preTestFn   func(interaction *common.Interaction)
		expectedErr error
	}{
		{
			name: "should return success if participant register data is valid",
			ix:   newTestInteraction(t, common.IxParticipantCreate, participantCreatePayload, 0, id, 0, nil),
			preTestFn: func(interaction *common.Interaction) {
				sm.setBalance(t, id, common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(1000))
			},
		},
		{
			name: "empty id",
			ix: newTestInteraction(t, common.IxParticipantCreate, &common.ParticipantCreatePayload{
				ID:    identifiers.Nil,
				Value: tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidIdentifier,
		},
		{
			name: "sender id matches participant id",
			ix: newTestInteraction(t, common.IxParticipantCreate, &common.ParticipantCreatePayload{
				ID:    id,
				Value: tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidIdentifier,
		},
		{
			name: "invalid weight",
			ix: newTestInteraction(t, common.IxParticipantCreate, &common.ParticipantCreatePayload{
				ID: tests.RandomIdentifier(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomIdentifier(t).Bytes(),
						Weight:             400,
						SignatureAlgorithm: 0,
					},
				},
				Value: tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidWeight,
		},
		{
			name: "invalid signature algorithms",
			ix: newTestInteraction(t, common.IxParticipantCreate, &common.ParticipantCreatePayload{
				ID: tests.RandomIdentifier(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomIdentifier(t).Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 1,
					},
				},
				Value: tests.CreateAssetTransferPayload(t, tests.RandomIdentifier(t)),
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidSignatureAlgorithm,
		},
		{
			name: "invalid assetID",
			ix: newTestInteraction(t, common.IxParticipantCreate, &common.ParticipantCreatePayload{
				ID: tests.RandomIdentifier(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomIdentifier(t).Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Value: &common.AssetActionPayload{
					AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				},
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidAssetID,
		},
		{
			name: "invalid callsite",
			ix: newTestInteraction(t, common.IxParticipantCreate, &common.ParticipantCreatePayload{
				ID: tests.RandomIdentifier(t),
				KeysPayload: []common.KeyAddPayload{
					{
						PublicKey:          tests.RandomIdentifier(t).Bytes(),
						Weight:             1000,
						SignatureAlgorithm: 0,
					},
				},
				Value: &common.AssetActionPayload{
					AssetID: common.KMOITokenAssetID,
				},
			}, 0, id, 0, nil),
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
	}, false, sm, nil)

	rawPayload := []byte{1, 2, 3}

	accs, err := cmdCommon.GetAccountsWithMnemonic(4)
	require.NoError(t, err)

	sig, err := crypto.GetSignature(rawPayload, accs[0].Mnemonic)
	require.NoError(t, err)

	rawSig, err := hex.DecodeString(sig)
	require.NoError(t, err)

	sm.updateAccountKeys(t, accs[:3])

	sig2, err := crypto.GetSignature(rawPayload, accs[2].Mnemonic)
	require.NoError(t, err)

	rawSig2, err := hex.DecodeString(sig2)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		id          identifiers.Identifier
		signatures  common.Signatures
		preTestFn   func()
		expectedErr error
	}{
		{
			name: "should return success if participant register data is valid",
			id:   accs[0].ID,
			signatures: common.Signatures{
				{
					ID:        accs[0].ID,
					KeyID:     0,
					Signature: rawSig,
				},
			},
		},
		{
			name: "public key not found",
			id:   accs[3].ID,
			preTestFn: func() {
				sm.setAccountKeys(accs[3].ID, common.AccountKeys{
					{
						Weight: 1000,
					},
				})
			},
			signatures: common.Signatures{
				{
					ID:        accs[3].ID,
					KeyID:     0,
					Signature: rawSig,
				},
			},
			expectedErr: common.ErrPublicKeyNotFound,
		},
		{
			name: "invalid ix signature",
			id:   accs[0].ID,
			signatures: common.Signatures{
				{
					ID:        accs[0].ID,
					KeyID:     0,
					Signature: tests.RandomHash(t).Bytes(),
				},
			},
			expectedErr: common.ErrInvalidIXSignature,
		},
		{
			name: "invalid weight due to revoked key",
			id:   accs[2].ID,
			preTestFn: func() {
				sm.setAccountKeys(accs[2].ID, common.AccountKeys{
					{
						Weight:  1000,
						Revoked: true,
					},
				})
				sm.setPublicKey(accs[2].ID, 0, accs[2].PublicKey)
			},
			signatures: common.Signatures{
				{
					ID:        accs[2].ID,
					KeyID:     0,
					Signature: rawSig2,
				},
			},
			expectedErr: common.ErrInvalidWeight,
		},
		{
			name: "account keys not found",
			id:   tests.RandomIdentifier(t),
			signatures: common.Signatures{
				{
					ID:        accs[0].ID,
					KeyID:     0,
					Signature: rawSig,
				},
			},
			expectedErr: errors.New("account keys not found"),
		},
		{
			name: "invalid key id in signature",
			id:   accs[0].ID,
			signatures: common.Signatures{
				{
					ID:        accs[0].ID,
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

			err := ixPool.verifyParticipantSignatures(testcase.id, rawPayload, testcase.signatures)
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
	}, false, sm, nil)

	accs, err := cmdCommon.GetAccountsWithMnemonic(3)
	require.NoError(t, err)

	ixParams := getIXParams(
		t,
		accs[0].ID,
		common.IxAssetAction,
		defaultIxPriceLimit,
		identifiers.RandomAssetIDv0(),
		&common.TransferParams{
			Beneficiary: tests.RandomIdentifier(t),
			Amount:      big.NewInt(5),
		},
		nil,
	)

	ixParams.SignaturesCallback = func(data *common.IxData, sig *common.Signatures) {
		data.Participants = append(data.Participants, common.IxParticipant{
			ID:     accs[1].ID,
			Notary: true,
		})

		rawSign := tests.GetIXSignature(t, data, accs[0].Mnemonic)
		(*sig)[0] = common.Signature{
			ID:        data.Sender.ID,
			KeyID:     data.Sender.KeyID,
			Signature: rawSign,
		}

		rawSign = tests.GetIXSignature(t, data, accs[1].Mnemonic)
		*sig = append(*sig, common.Signature{
			ID:        accs[1].ID,
			KeyID:     0,
			Signature: rawSign,
		})
	}

	sm.updateAccountKeys(t, accs)

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
					ix.Sender.ID = accs[0].ID
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
					ix.Sender.ID = accs[0].ID
				},
			}),
			expectedErr: errors.New("invalid sender's signature"),
		},
		{
			name: "invalid notary participant signature",
			ix: tests.CreateIX(t, &tests.CreateIxParams{
				IxDataCallback: func(ix *common.IxData) {
					ix.Sender.ID = accs[0].ID
					ix.Participants = append(ix.Participants, common.IxParticipant{
						ID:     accs[1].ID,
						Notary: true,
					})
				},
				SignaturesCallback: func(data *common.IxData, sig *common.Signatures) {
					rawSign := tests.GetIXSignature(t, data, accs[0].Mnemonic)
					(*sig)[0] = common.Signature{
						ID:        data.Sender.ID,
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
	}, false, sm, nil)

	id := tests.RandomIdentifier(t)
	addAccConfigurePayload := &common.AccountConfigurePayload{
		Add: []common.KeyAddPayload{
			{
				Weight: 300,
			},
		},
	}

	revokeAccConfigurePayload := &common.AccountConfigurePayload{
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
			ix:   newTestInteraction(t, common.IxAccountConfigure, addAccConfigurePayload, 0, id, 0, nil),
		},
		{
			name: "should return success if acc key is revoked",
			ix:   newTestInteraction(t, common.IxAccountConfigure, revokeAccConfigurePayload, 0, id, 0, nil),
			preTestFn: func(interaction *common.Interaction) {
				sm.setAccountKeys(id, common.AccountKeys{{
					ID: 300,
				}})
			},
		},
		{
			name: "can not provide both add and revoke payload",
			ix: newTestInteraction(t, common.IxAccountConfigure, &common.AccountConfigurePayload{
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
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidAccountConfigure,
		},
		{
			name: "both add and revoke can not be empty",
			ix: newTestInteraction(t, common.IxAccountConfigure, &common.AccountConfigurePayload{
				Add:    []common.KeyAddPayload{},
				Revoke: []common.KeyRevokePayload{},
			}, 0, id, 0, nil),
			expectedErr: common.ErrInvalidAccountConfigure,
		},
		{
			name: "invalid signature algorithm",
			ix: newTestInteraction(t, common.IxAccountConfigure, &common.AccountConfigurePayload{
				Add: []common.KeyAddPayload{
					{
						SignatureAlgorithm: 1,
					},
				},
			}, 0, id, 0, nil),
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
	id := tests.RandomIdentifier(t)
	invalidLogicPayload := &common.LogicPayload{}
	validLogicPayload := &common.LogicPayload{
		Manifest: []byte{1, 2},
		Callsite: "Init",
	}

	testcases := []struct {
		name        string
		ix          *common.Interaction
		setHook     func(c *MockExecutionManager)
		expectedErr error
	}{
		{
			name:        "should return error if manifest is empty",
			ix:          newTestInteraction(t, common.IxLogicDeploy, invalidLogicPayload, 0, id, 0, nil),
			expectedErr: common.ErrEmptyManifest,
		},
		{
			name: "should return success if logic payload is valid",
			ix:   newTestInteraction(t, common.IxLogicDeploy, validLogicPayload, 0, id, 0, nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
			}, false, sm, nil)

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
	id := tests.RandomIdentifier(t)
	payloadWithoutCallsite := &common.LogicPayload{}
	payloadWithoutLogicID := &common.LogicPayload{
		Callsite: "seeder",
	}
	logicID := identifiers.RandomLogicIDv0()
	validLogicPayload := &common.LogicPayload{
		LogicID:  logicID,
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
			ix:          newTestInteraction(t, common.IxLogicInvoke, payloadWithoutCallsite, 0, id, 0, nil),
			expectedErr: common.ErrInvalidCallSite,
		},
		{
			name:        "should return error if logicID is empty",
			ix:          newTestInteraction(t, common.IxLogicInvoke, payloadWithoutLogicID, 0, id, 0, nil),
			expectedErr: common.ErrMissingLogicID,
		},
		{
			name:        "should return error if logic is not registered",
			ix:          newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, id, 0, nil),
			expectedErr: errors.New("logic is not registered"),
		},
		{
			name: "should return success if logic payload is valid",
			ix:   newTestInteraction(t, common.IxLogicInvoke, validLogicPayload, 0, id, 0, nil),
			preTestFn: func(interaction *common.Interaction, msm *MockStateManager) {
				msm.registerLogicID(t, logicID)
				msm.setLatestStateObject(interaction.GetIxOp(0).Target(), &state.Object{})
				msm.setLatestStateObject(interaction.SenderID(), &state.Object{})
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
	accounts      map[identifiers.Identifier][]*common.Interaction
	delayCounters map[identifiers.Identifier]int32
)

func TestIxPool_Executables_Wait_Mode(t *testing.T) {
	ids := tests.GetRandomIDs(t, 4)

	testcases := []struct {
		name               string
		accounts           accounts
		delayCounters      delayCounters
		updateDelayCounter func(ixPool *IxPool, delayCounters delayCounters)
		expectedAddrList   []identifiers.Identifier
	}{
		{
			name: "One ix per account",
			accounts: map[identifiers.Identifier][]*common.Interaction{
				ids[0]: createTestIxs(t, 0, 1, ids[0]),
				ids[1]: createTestIxs(t, 0, 1, ids[1]),
				ids[2]: createTestIxs(t, 0, 1, ids[2]),
				ids[3]: createTestIxs(t, 0, 1, ids[3]),
			},
			delayCounters: map[identifiers.Identifier]int32{
				ids[0]: 3,
				ids[1]: 4,
				ids[2]: 1,
				ids[3]: 2,
			},
			updateDelayCounter: func(ixPool *IxPool, delayCounters delayCounters) {
				for id := range delayCounters {
					acc := ixPool.accounts.getAccount(id)
					setDelayCounter(t, acc, delayCounters[id])
				}
			},
			expectedAddrList: []identifiers.Identifier{
				ids[1],
				ids[0],
				ids[3],
				ids[2],
			},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[identifiers.Identifier][]*common.Interaction{
				ids[0]: createTestIxs(t, 0, 2, ids[0]),
				ids[1]: createTestIxs(t, 0, 2, ids[1]),
				ids[2]: createTestIxs(t, 0, 2, ids[2]),
				ids[3]: createTestIxs(t, 0, 2, ids[3]),
			},
			delayCounters: map[identifiers.Identifier]int32{
				ids[0]: 3,
				ids[1]: 4,
				ids[2]: 1,
				ids[3]: 2,
			},
			updateDelayCounter: func(ixPool *IxPool, delayCounters delayCounters) {
				for id := range delayCounters {
					acc := ixPool.accounts.getAccount(id)
					setDelayCounter(t, acc, delayCounters[id])
				}
			},
			expectedAddrList: []identifiers.Identifier{
				ids[1],
				ids[0],
				ids[3],
				ids[2],
				ids[1],
				ids[0],
				ids[3],
				ids[2],
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			sm.setTestMOIBalance(t, ids...)
			sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, len(ids)))
			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				sm.registerIxParticipants(ixs...)

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
				require.Equal(t, expectedAddress, successfulIxs[index].SenderID())
			}
		})
	}
}

//
//// FIXME: Currently the fuel price is always set to 1
// func TestIxPool_Executables_Cost_Mode(t *testing.T) {
//	ids := tests.GetRandomIDs(t, 5)
//
//	testcases := []struct {
//		name              string
//		accounts          map[types.Identifier]types.Interactions
//		expectedPriceList []uint64
//	}{
//		{
//			name: "One ix per account",
//			accounts: map[types.Identifier]types.Interactions{
//				ids[0]: {
//					newIxWithFuelPrice(t, 0, ids[0], 1),
//				},
//				ids[1]: {
//					newIxWithFuelPrice(t, 0, ids[1], 2),
//				},
//				ids[2]: {
//					newIxWithFuelPrice(t, 0, ids[2], 3),
//				},
//				ids[3]: {
//					newIxWithFuelPrice(t, 0, ids[3], 4),
//				},
//				ids[4]: {
//					newIxWithFuelPrice(t, 0, ids[4], 5),
//				},
//			},
//			expectedPriceList: []uint64{5, 4, 3, 2, 1},
//		},
//		{
//			name: "Several ixs from multiple accounts",
//			accounts: map[types.Identifier]types.Interactions{
//				ids[0]: {
//					newIxWithFuelPrice(t, 0, ids[0], 3),
//					newIxWithFuelPrice(t, 1, ids[0], 3),
//				},
//				ids[1]: {
//					newIxWithFuelPrice(t, 0, ids[1], 2),
//					newIxWithFuelPrice(t, 1, ids[1], 2),
//				},
//				ids[2]: {
//					newIxWithFuelPrice(t, 0, ids[2], 1),
//					newIxWithFuelPrice(t, 1, ids[2], 1),
//				},
//			},
//			expectedPriceList: []uint64{3, 2, 1, 3, 2, 1},
//		},
//		{
//			name: "Several ixs from multiple accounts with same fuel cost",
//			accounts: map[types.Identifier]types.Interactions{
//				ids[0]: {
//					newIxWithFuelPrice(t, 0, ids[0], 6),
//					newIxWithFuelPrice(t, 1, ids[0], 3),
//				},
//				ids[1]: {
//					newIxWithFuelPrice(t, 0, ids[1], 5),
//					newIxWithFuelPrice(t, 1, ids[1], 4),
//				},
//				ids[2]: {
//					newIxWithFuelPrice(t, 0, ids[2], 6),
//					newIxWithFuelPrice(t, 1, ids[2], 2),
//				},
//			},
//			expectedPriceList: []uint64{6, 6, 5, 4, 3, 2},
//		},
//	}
//
//	for _, testcase := range testcases {
//		t.Run(testcase.name, func(t *testing.T) {
//			sm := NewMockStateManager(t)
//			sm.setTestMOIBalance(ids...)
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
	ids := tests.GetRandomIDs(t, 5)

	testcases := []struct {
		name            string
		accounts        map[identifiers.Identifier][]*common.Interaction
		accountWaitTime map[identifiers.Identifier]time.Time
		updateWaitTime  func(ixPool *IxPool, accountWaitTime map[identifiers.Identifier]time.Time)
		expectedNonce   map[identifiers.Identifier]uint64
	}{
		{
			name: "One ix per account",
			accounts: map[identifiers.Identifier][]*common.Interaction{
				ids[0]: createTestIxs(t, 7, 8, ids[0]),
				ids[1]: createTestIxs(t, 8, 9, ids[1]),
				ids[2]: createTestIxs(t, 5, 6, ids[2]),
				ids[3]: createTestIxs(t, 6, 7, ids[3]),
			},
			accountWaitTime: map[identifiers.Identifier]time.Time{
				ids[0]: time.Now().Add(1000 * time.Millisecond),
				ids[1]: time.Now().Add(-100),
				ids[2]: time.Now().Add(-200),
				ids[3]: time.Now().Add(-150),
			},
			updateWaitTime: func(ixPool *IxPool, accountWaitTime map[identifiers.Identifier]time.Time) {
				for id := range accountWaitTime {
					acc := ixPool.accounts.getAccount(id)
					acc.waitTime = accountWaitTime[id]
				}
			},
			expectedNonce: map[identifiers.Identifier]uint64{
				ids[1]: 8,
				ids[2]: 5,
				ids[3]: 6,
			},
		},
		{
			name: "Several ixs from multiple accounts",
			accounts: map[identifiers.Identifier][]*common.Interaction{
				ids[0]: createTestIxs(t, 4, 6, ids[0]),
				ids[1]: createTestIxs(t, 5, 7, ids[1]),
				ids[2]: createTestIxs(t, 6, 8, ids[2]),
				ids[3]: createTestIxs(t, 7, 9, ids[3]),
			},
			accountWaitTime: map[identifiers.Identifier]time.Time{
				ids[0]: time.Now().Add(-200),
				ids[1]: time.Now().Add(-100),
				ids[2]: time.Now().Add(1000 * time.Millisecond),
				ids[3]: time.Now().Add(-150),
			},
			updateWaitTime: func(ixPool *IxPool, accountWaitTime map[identifiers.Identifier]time.Time) {
				for id := range accountWaitTime {
					acc := ixPool.accounts.getAccount(id)
					acc.waitTime = accountWaitTime[id]
				}
			},
			expectedNonce: map[identifiers.Identifier]uint64{
				ids[0]: 4,
				ids[1]: 5,
				ids[3]: 7,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			sm.setTestMOIBalance(t, ids...)
			sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, len(ids)))

			ixPool := CreateTestIxpool(t, func(c *config.IxPoolConfig) {
				c.Mode = WaitMode
				c.PriceLimit = defaultIxPriceLimit
				c.MaxSlots = config.DefaultMaxIXPoolSlots
			}, true, sm, newMockNetwork(""))

			ixPool.Start()
			defer ixPool.Close()

			for _, ixs := range testcase.accounts {
				sm.registerIxParticipants(ixs...)

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

			for id, nonce := range testcase.expectedNonce {
				require.NotNil(t, ixNonce[id])
				require.Equal(t, nonce, ixNonce[id])
			}
		})
	}
}

func TestIxPool_RemoveNonceHoleAccounts(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 3)
	sm.setTestMOIBalance(t, ids...)
	sm.setAccountKeysAndPublicKeys(t, ids, tests.GetTestPublicKeys(t, 3))

	testcases := []struct {
		name                string
		ixs                 []*common.Interaction
		ixPoolCallback      func(i *IxPool)
		expectedEnqueuedIxs uint64
		expectedPromotedIxs uint64
	}{
		{
			name: "accounts without sequenceID holes",
			ixs:  createTestIxs(t, 0, 5, ids[0]),
			ixPoolCallback: func(i *IxPool) {
				i.getOrCreateAccountQueue(ids[0], 0, 0)
			},
			expectedEnqueuedIxs: 0,
			expectedPromotedIxs: 5,
		},
		{
			name: "accounts with sequenceID holes",
			ixs:  createTestIxs(t, 2, 8, ids[1]),
			ixPoolCallback: func(i *IxPool) {
				i.getOrCreateAccountQueue(ids[1], 0, 0)
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
			}, true, sm, newMockNetwork(""))

			if test.ixPoolCallback != nil {
				test.ixPoolCallback(ixPool)
			}

			senderID := test.ixs[0].SenderID()

			// make sure gauge is zero initially
			require.Equal(t, uint64(0), ixPool.gauge.read())

			sm.registerIxParticipants(test.ixs...)

			errs := ixPool.AddLocalInteractions(common.NewInteractionsWithLeaderCheck(true, test.ixs...))
			require.Len(t, errs, 0)

			// make sure gauge is increased after ixns enqueued
			require.Equal(t, slotsRequired(test.ixs...), ixPool.gauge.read())

			acc := ixPool.accounts.getAccountQueue(test.ixs[0].SenderID(), 0)
			require.Equal(t, len(test.ixs), len(acc.sequenceIDToIX.mapping))

			ixPool.removeSequenceIDHoleAccounts()

			// make sure gauge decreased
			require.Equal(t, test.expectedPromotedIxs, ixPool.gauge.read())
			require.Equal(t, test.expectedEnqueuedIxs, ixPool.accounts.getAccountQueue(senderID, 0).enqueued.length())
			require.Equal(t, test.expectedPromotedIxs, ixPool.accounts.getAccountQueue(senderID, 0).promoted.length())
			require.Equal(t, int(test.expectedPromotedIxs), len(acc.sequenceIDToIX.mapping))
		})
	}
}

func TestIxPool_RemoveNonceHoleAccounts_WithEmptyEnqueues(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := tests.GetIdentifiers(t, 1)
	sm.setTestMOIBalance(t, ids...)

	testcases := []struct {
		name           string
		ixs            []*common.Interaction
		ixPoolCallback func(i *IxPool)
	}{
		{
			name: "accounts with empty enqueues",
			ixPoolCallback: func(i *IxPool) {
				i.getOrCreateAccountQueue(ids[0], 0, 0)
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
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(ids[0], 0).getSequenceID())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(ids[0], 0).enqueued.length())

			ixPool.removeSequenceIDHoleAccounts()

			// make sure gauge is not decreased
			require.Equal(t, uint64(0), ixPool.gauge.read())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(ids[0], 0).getSequenceID())
			require.Equal(t, uint64(0), ixPool.accounts.getAccountQueue(ids[0], 0).enqueued.length())
		})
	}
}

func TestIxPool_GetIxParticipants(t *testing.T) {
	testcases := []struct {
		name                 string
		ix                   *common.Interaction
		expectedParticipants map[identifiers.Identifier]struct{}
	}{
		{
			name: "ix participants with beneficiary",
			ix: newTestInteraction(
				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
				0, tests.RandomIdentifier(t), 0, nil,
			),
		},
		{
			name: "ix participants with beneficiary and payer",
			ix: newTestInteraction(
				t, common.IxAssetAction, tests.CreateAssetTransferPayload(t, identifiers.Nil),
				0, tests.RandomIdentifier(t), 0, func(ixData *common.IxData) {
					ixData.Payer = tests.RandomIdentifier(t)
					ixData.Participants = append(ixData.Participants, common.IxParticipant{
						ID:       ixData.Payer,
						LockType: common.MutateLock,
					})
				}),
		},
		{
			name: "ix participants without beneficiary and payer",
			ix: newTestInteraction(
				t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
				0, tests.RandomIdentifier(t), 0, nil,
			),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			participants := getIxParticipants(t, test.ix)

			require.Equal(t, len(test.ix.IxParticipants()), len(participants))

			for _, participant := range test.ix.IxParticipants() {
				require.NotNil(t, participants[participant.ID])
			}
		})
	}
}

func TestIxBatchRegistry_ProcessableBatches(t *testing.T) {
	sm := NewMockStateManager(t)
	ids := make([]identifiers.Identifier, 2)

	// we create two participant id with the lowest possible value
	ps1, _ := identifiers.GenerateParticipantIDv0(tests.Min24Byte(t, 1), 0)
	ps2, _ := identifiers.GenerateParticipantIDv0(tests.Min24Byte(t, 2), 0)

	ids[0] = ps1.AsIdentifier()
	ids[1] = ps2.AsIdentifier()

	sm.SetAccountMetaInfo(ids[0], &common.AccountMetaInfo{
		PositionInContextSet: 0,
	})

	testcases := []struct {
		name        string
		currentView uint64
		// this function returns the interactions that will be added to the ixpool and
		// the interactions that are expected to be batched
		getIxns           func() (input []*common.Interaction, expected []*common.Interaction)
		preTestFn         func(sm *MockStateManager, ixPool *IxPool, ixns []*common.Interaction)
		expectedBatchList []CreateBatches
	}{
		{
			name:        "get processable batches",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 4, ids[0], 1, sm)

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
						ixnCount:                3,
						consensusNodesHashCount: 4,
					},
				},
			},
		},
		{
			name:        "get processable batches with multiple keys for same id",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 1, ids[0], 4, sm)

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
						ixnCount:                3,
						consensusNodesHashCount: 4,
					},
				},
			},
		},
		{
			name:        "get processable batches from ixns where current view of an ixn is expired",
			currentView: common.ConsensusNodesSize,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, ids[0], 1, sm)

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
						ixnCount:                3,
						consensusNodesHashCount: 4,
					},
				},
			},
		},
		{
			name:        "get processable batches from ixns in an optimal way",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns1 := createTestAssetTransferIxs(t, 0, 1, ids[0], 1, sm)

				ixns1[0].SetShouldPropose(true)
				ixns1[0].UpdateAllottedView(4)

				ixns2 := createTestAssetTransferIxs(t, 0, 1, ids[1], 1, sm)

				ixns2[0].SetShouldPropose(true)
				ixns2[0].UpdateAllottedView(4)

				return append(ixns1, ixns2...), append(ixns1, ixns2...)
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                2,
						consensusNodesHashCount: 4,
					},
				},
			},
		},
		{
			name:        "get processable batches without sequenceID holes (one of the ixn cannot be proposed)",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, ids[0], 1, sm)

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
						ixnCount:                1,
						consensusNodesHashCount: 2,
					},
				},
			},
		},
		{
			name:        "get processable batches without sequenceID holes (ixn allotted view > current view)",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createTestAssetTransferIxs(t, 0, 3, ids[0], 1, sm)

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
						ixnCount:                1,
						consensusNodesHashCount: 2,
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
						ixnCount:                3,
						consensusNodesHashCount: 4,
					},
				},
			},
		},
		{
			name:        "ixns from sub accounts to inherited account should be clubbed in to single batch",
			currentView: 4,
			getIxns: func() (input []*common.Interaction, expected []*common.Interaction) {
				ixns := createIxnsFromParticipants(t,
					[][]int{{101, 0}, {102, 0}, {103, 0}, {104, 0}, {105, 0}, {106, 0}, {107, 0}})
				for _, ixn := range ixns {
					ixn.SetShouldPropose(true)
					ixn.UpdateAllottedView(4)
				}

				return ixns, ixns
			},
			preTestFn: func(sm *MockStateManager, ixPool *IxPool, ixns []*common.Interaction) {
				logicID := ixns[0].IxParticipants()[1].ID

				for _, ix := range ixns {
					for id := range ix.Participants() {
						if id.IsParticipantVariant() {
							sm.SetAccountMetaInfo(id, &common.AccountMetaInfo{
								InheritedAccount: logicID,
							})
						}
					}
				}
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                7,
						consensusNodesHashCount: 1,
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
			}, true, sm, newMockNetwork(""))

			ixPool.UpdateCurrentView(testcase.currentView)
			input, expectedIxns := testcase.getIxns()

			if testcase.preTestFn != nil {
				testcase.preTestFn(sm, ixPool, input)
			}

			// generate random consensus nodes hash for each identifier
			for _, ixn := range input {
				for id := range ixn.Participants() {
					if !id.IsParticipantVariant() {
						ixPool.consensusNodesHash.Add(id, tests.RandomHash(t))
					}
				}
			}

			insertIxnsInPromotedQueue(ixPool, input)

			batches := ixPool.ProcessableBatches()

			index := 0

			for _, expectedBatches := range testcase.expectedBatchList {
				for i := 0; i < expectedBatches.batchCount; i++ {
					require.Equal(t, expectedBatches.batch.ixnCount, batches[i].IxCount())
					require.Equal(t, expectedBatches.batch.consensusNodesHashCount, batches[i].ConsensusNodesHashCount())

					for _, ix := range batches[i].IxList() {
						require.Equal(t, expectedIxns[index].SenderID(), ix.SenderID())

						index++
					}
				}
			}
		})
	}
}

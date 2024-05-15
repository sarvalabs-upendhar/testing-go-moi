package consensus

import (
	"context"
	"math/rand"
	"testing"

	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"

	"github.com/sarvalabs/go-moi/common/tests"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

func TestUpdateContextDelta(t *testing.T) {
	addrs := tests.GetAddresses(t, 4)
	kramaIDs := tests.RandomKramaIDs(t, 2)
	operator := kramaIDs[0]
	nodeset := createNodeSet(t, 3, 2)

	sm := newMockStateManager()

	testcases := []struct {
		name                 string
		sender               identifiers.Address
		receiver             identifiers.Address
		slot                 *ktypes.Slot
		enableDebugMode      bool
		expectedContextDelta map[identifiers.Address]*common.DeltaGroup
		expectedError        error
	}{
		{
			name:          "Slot is nil",
			slot:          nil,
			expectedError: errors.New("nil slot"),
		},
		{
			name: "participant with read lock",
			slot: NewTestSlot(t,
				operator,
				kramaIDs[1],
				nodeset,
				nil,
				map[identifiers.Address]*common.Participant{
					addrs[0]: {
						LockType: common.ReadLock,
					},
				},
			),
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[0]: nil,
			},
		},
		{
			name: "should not update context for non signer account",
			slot: NewTestSlot(t,
				operator,
				kramaIDs[1],
				nodeset,
				nil,
				map[identifiers.Address]*common.Participant{
					addrs[0]: {
						LockType: common.WriteLock,
						IsSigner: false, // this should be non signer participant
						AccType:  common.RegularAccount,
					},
				},
			),
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[0]: nil,
			},
		},
		{
			name: "should not update context for non genesis account in debug mode",
			slot: NewTestSlot(t,
				operator,
				kramaIDs[1],
				nodeset,
				nil,
				map[identifiers.Address]*common.Participant{
					addrs[0]: {
						LockType:  common.WriteLock,
						IsGenesis: false,
					},
				},
			),
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[0]: nil,
			},
			enableDebugMode: true,
		},
		{
			name: "should create new context for genesis account",
			slot: NewTestSlot(t,
				operator,
				kramaIDs[1],
				nodeset,
				nil,
				map[identifiers.Address]*common.Participant{
					addrs[0]: {
						LockType:  common.WriteLock,
						IsGenesis: true,
					},
				},
			),
			enableDebugMode: false,
		},
		{
			name: "should update context for all participants",
			slot: NewTestSlot(t,
				operator,
				kramaIDs[1],
				nodeset,
				nil,
				map[identifiers.Address]*common.Participant{
					addrs[0]: {
						LockType:        common.WriteLock,
						IsGenesis:       false,
						NodeSetPosition: 0,
					},
				},
			),
			enableDebugMode: false,
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[0]: {
					BehaviouralNodes: []kramaid.KramaID{operator},
					RandomNodes:      []kramaid.KramaID{nodeset.RandomSet().Ids[0]},
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			engineParams := &testKramaEngineParams{
				sm: sm,
				cfgCallback: func(cfg *config.ConsensusConfig) {
					cfg.EnableDebugMode = test.enableDebugMode
				},
			}

			engine := createTestKramaEngine(t, engineParams)

			err := engine.updateContextDelta(test.slot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for addr, ps := range test.slot.ClusterState().Participants {
				checkContextDelta(t, ps.IsGenesis, test.expectedContextDelta[addr], ps.ContextDelta)
			}
		})
	}
}

func TestExecuteAndValidate(t *testing.T) {
	var (
		address        = tests.RandomAddress(t)
		stateHash      = tests.RandomHash(t)
		receipts       = tests.CreateReceiptsWithTestData(t, tests.RandomHash(t))
		accStateHashes = common.AccStateHashes{
			address: {
				StateHash: stateHash,
			},
		}
		defaultClusterID = common.ClusterID("cluster-0")
		emptyReceipts    common.Receipts
	)

	receiptsHash, err := receipts.Hash()
	require.NoError(t, err)

	testcases := []struct {
		name                    string
		tsParams                *tests.CreateTesseractParams
		revertHook              func() error
		executeInteractionsHook func() (common.Receipts, common.AccStateHashes, error)
		expectedError           error
	}{
		{
			name: "should return error if execution fails",
			tsParams: &tests.CreateTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ConsensusInfo.ClusterID = defaultClusterID
				},
			},
			executeInteractionsHook: func() (common.Receipts, common.AccStateHashes, error) {
				return nil, nil, errors.New("failed to execute interactions")
			},
			expectedError: errors.New("failed to execute interactions"),
		},
		{
			name: "should return error if receipt validation fails",
			tsParams: &tests.CreateTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ReceiptsHash = tests.RandomHash(t)
					ts.ConsensusInfo.ClusterID = defaultClusterID
				},
			},
			executeInteractionsHook: func() (common.Receipts, common.AccStateHashes, error) {
				return emptyReceipts, nil, nil
			},
			expectedError: errors.New("failed to validate the tesseract"),
		},
		{
			name: "should return error if state hash validation fails",
			tsParams: &tests.CreateTesseractParams{
				Addresses: []identifiers.Address{address},
				Participants: common.ParticipantStates{
					address: {
						StateHash: tests.RandomHash(t),
					},
				},
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ConsensusInfo.ClusterID = defaultClusterID
					ts.ReceiptsHash = receiptsHash
				},
			},
			executeInteractionsHook: func() (common.Receipts, common.AccStateHashes, error) {
				return receipts, accStateHashes, nil
			},
			expectedError: errors.New("failed to validate the tesseract"),
		},
		{
			name: "should return error if revert fails",
			tsParams: &tests.CreateTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ConsensusInfo.ClusterID = defaultClusterID
				},
			},
			executeInteractionsHook: func() (common.Receipts, common.AccStateHashes, error) {
				return receipts, accStateHashes, nil // execution is reverted if hashes are invalid
			},
			revertHook: func() error {
				return errors.New("revert failed")
			},
			expectedError: errors.New("failed to revert the execution changes"),
		},
		{
			name: "receipts should be added to the tesseract",
			tsParams: &tests.CreateTesseractParams{
				Participants: common.ParticipantStates{
					address: {
						StateHash: stateHash,
					},
				},
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ConsensusInfo.ClusterID = defaultClusterID
					ts.ReceiptsHash = receiptsHash
				},
			},
			executeInteractionsHook: func() (common.Receipts, common.AccStateHashes, error) {
				return receipts, accStateHashes, nil
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.tsParams)

			params := &testKramaEngineParams{
				execCallback: func(exec *MockExec) {
					exec.executeInteractionsHook = test.executeInteractionsHook
					exec.revertHook = test.revertHook
				},
			}

			c := createTestKramaEngine(t, params)

			err := c.ExecuteAndValidate(ts)

			checkForExecutionCleanup(t, c.exec.(*MockExec), "cluster-0") //nolint

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, receipts, ts.Receipts())
		})
	}
}

func TestAreStateHashesValid(t *testing.T) {
	addresses := tests.GetAddresses(t, 2)
	stateHashes := tests.GetHashes(t, 2)
	accStateHashes := common.AccStateHashes{
		addresses[0]: {StateHash: stateHashes[0]},
		addresses[1]: {StateHash: stateHashes[1]},
	}

	testcases := []struct {
		name        string
		params      *tests.CreateTesseractParams
		stateHashes common.AccStateHashes
		isValid     bool
	}{
		{
			name: "state hash in receipt and tesseract doesn't match",
			params: &tests.CreateTesseractParams{
				Participants: common.ParticipantStates{
					addresses[0]: {
						StateHash: tests.RandomHash(t),
					},
				},
			},
			stateHashes: accStateHashes,
			isValid:     false,
		},
		{
			name: "valid state hashes",
			params: &tests.CreateTesseractParams{
				Addresses: []identifiers.Address{addresses[0], addresses[1]},
				Participants: common.ParticipantStates{
					addresses[0]: {
						StateHash: stateHashes[0],
					},
					addresses[1]: {
						StateHash: stateHashes[1],
					},
				},
			},
			stateHashes: accStateHashes,
			isValid:     true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.params)

			isValid := areStateHashesValid(ts, test.stateHashes)

			require.Equal(t, test.isValid, isValid)
		})
	}
}

func TestIsReceiptHashValid(t *testing.T) {
	var receipts common.Receipts

	receiptsHash, err := receipts.Hash()
	require.NoError(t, err)

	testcases := []struct {
		name      string
		paramsMap *tests.CreateTesseractParams
		isValid   bool
	}{
		{
			name: "valid receipt hash",
			paramsMap: &tests.CreateTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ReceiptsHash = receiptsHash
				},
			},
			isValid: true,
		},
		{
			name: "invalid receipt hash",
			paramsMap: &tests.CreateTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ReceiptsHash = tests.RandomHash(t)
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.paramsMap)

			isValid := isReceiptsHashValid(ts, receipts)
			require.Equal(t, test.isValid, isValid)
		})
	}
}

func TestFetchParticipantsAndNodeSet(t *testing.T) {
	ms := newMockStateManager()

	_, ixnPs := tests.CreateTestIxParticipants(t, 3, 1)

	// create an address without nodeSets
	addrsWithOutContext := tests.RandomAddress(t)
	ms.addAccMetaInfo(t, addrsWithOutContext, tests.RandomAccMetaInfo(t, rand.Uint64()))

	// add nodeSets and accountMetaInfo to state manager
	for addr, p := range ixnPs {
		// avoid storing meta info for genesis accounts
		if !p.IsGenesis {
			ms.addAccMetaInfo(t, addr, tests.RandomAccMetaInfo(t, rand.Uint64()))
		}

		ms.addNodeSet(t, addr, tests.RandomHash(t), createTestNodeSet(t, 3), createTestNodeSet(t, 3))
	}

	testcases := []struct {
		name            string
		ixnParticipants map[identifiers.Address]common.IxParticipant
		expectedError   error
	}{
		{
			name: "should return error if failed to fetch accMetaInfo",
			ixnParticipants: map[identifiers.Address]common.IxParticipant{
				tests.RandomAddress(t): {IsGenesis: false},
			},
			expectedError: common.ErrFetchingAccMetaInfo,
		},
		{
			name: "should return error if failed to fetch context info",
			ixnParticipants: map[identifiers.Address]common.IxParticipant{
				addrsWithOutContext: {IsGenesis: false, LockType: 1},
			},
			expectedError: common.ErrContextStateNotFound,
		},
		{
			name:            "should return participants info and nodeSet",
			ixnParticipants: ixnPs,
			expectedError:   nil,
		},
	}

	engine := createTestKramaEngine(t, &testKramaEngineParams{sm: ms})

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ps, ns, err := engine.fetchParticipantsAndNodeSet(context.Background(), test.ixnParticipants)
			if test.expectedError != nil {
				require.ErrorIs(t, err, test.expectedError)
			}
			checkParticipantInfo(t, ms, test.ixnParticipants, ps)
			checkNodeSetForParticipant(t, ms, ps, ns)
		})
	}
}

func NewTestSlot(
	t *testing.T,
	operator, self kramaid.KramaID,
	ns *common.ICSNodeSet,
	ixns common.Interactions,
	ps map[identifiers.Address]*common.Participant,
) *ktypes.Slot {
	t.Helper()

	s := ktypes.NewSlot(ktypes.OperatorSlot)
	s.UpdateClusterState(createTestClusterState(t,
		operator,
		self,
		ns,
		ixns,
		ps,
		nil,
	))

	return s
}

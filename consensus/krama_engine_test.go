package consensus

//
// import (
//	"context"
//	"math/big"
//	"math/rand"
//	"os"
//	"testing"
//	"time"
//
//	"github.com/sarvalabs/go-moi/crypto/vrf"
//	blst "github.com/supranational/blst/bindings/go"
//
//	"github.com/pkg/errors"
//	kramaid "github.com/sarvalabs/go-legacy-kramaid"
//	"github.com/sarvalabs/go-moi/identifiers"
//	"github.com/sarvalabs/go-moi/common/hexutil"
//	"github.com/sarvalabs/go-moi/state"
//	"github.com/stretchr/testify/require"
//
//	"github.com/sarvalabs/go-moi/common"
//	"github.com/sarvalabs/go-moi/common/config"
//
//	"github.com/sarvalabs/go-moi/common/tests"
//	ktypes "github.com/sarvalabs/go-moi/consensus/types"
//)
//
// var (
//	validCommitSign   = []byte{1}
//	invalidCommitSign = []byte{0}
//	testVRFOutput     = [32]byte{2, 3, 5}
//	testVRFProof      = []byte{1, 2, 9, 7, 6}
//)
//
// func TestUpdateContextDelta(t *testing.T) {
//	addrs := tests.GetAddresses(t, 4)
//	kramaIDs := tests.RandomKramaIDs(t, 2)
//	operator := kramaIDs[0]
//	nodeset := createNodeSet(t, 3, 2)
//
//	sm := NewMockStateManager()
//
//	testcases := []struct {
//		name                 string
//		sender               identifiers.Identifier
//		receiver             identifiers.Identifier
//		slot                 *ktypes.Slot
//		enableDebugMode      bool
//		expectedContextDelta map[identifiers.Identifier]*common.DeltaGroup
//		expectedError        error
//	}{
//		{
//			name:          "Slot is nil",
//			slot:          nil,
//			expectedError: errors.New("nil slot"),
//		},
//		{
//			name: "participant with read lock",
//			slot: NewTestSlot(t,
//				operator,
//				kramaIDs[1],
//				nodeset,
//				nil,
//				map[identifiers.Identifier]*common.Participant{
//					addrs[0]: {
//						LockType: common.ReadLock,
//					},
//				},
//			),
//			expectedContextDelta: map[identifiers.Identifier]*common.DeltaGroup{
//				addrs[0]: nil,
//			},
//		},
//		{
//			name: "should not update context for non signer account",
//			slot: NewTestSlot(t,
//				operator,
//				kramaIDs[1],
//				nodeset,
//				nil,
//				map[identifiers.Identifier]*common.Participant{
//					addrs[0]: {
//						LockType: common.MutateLock,
//						IsSigner: false, // this should be non signer participant
//						AccType:  common.RegularAccount,
//					},
//				},
//			),
//			expectedContextDelta: map[identifiers.Identifier]*common.DeltaGroup{
//				addrs[0]: nil,
//			},
//		},
//		{
//			name: "should not update context for non genesis account in debug mode",
//			slot: NewTestSlot(t,
//				operator,
//				kramaIDs[1],
//				nodeset,
//				nil,
//				map[identifiers.Identifier]*common.Participant{
//					addrs[0]: {
//						LockType:  common.MutateLock,
//						IsGenesis: false,
//					},
//				},
//			),
//			expectedContextDelta: map[identifiers.Identifier]*common.DeltaGroup{
//				addrs[0]: nil,
//			},
//			enableDebugMode: true,
//		},
//		{
//			name: "should create new context for genesis account",
//			slot: NewTestSlot(t,
//				operator,
//				kramaIDs[1],
//				nodeset,
//				nil,
//				map[identifiers.Identifier]*common.Participant{
//					addrs[0]: {
//						AccType:   common.RegularAccount,
//						LockType:  common.MutateLock,
//						IsGenesis: true,
//					},
//				},
//			),
//			enableDebugMode: false,
//		},
//		{
//			name: "should update context for all participants",
//			slot: NewTestSlot(t,
//				operator,
//				kramaIDs[1],
//				nodeset,
//				nil,
//				map[identifiers.Identifier]*common.Participant{
//					addrs[0]: {
//						LockType:        common.MutateLock,
//						IsGenesis:       false,
//						NodeSetPosition: 0,
//					},
//				},
//			),
//			enableDebugMode: false,
//			expectedContextDelta: map[identifiers.Identifier]*common.DeltaGroup{
//				addrs[0]: {
//					BehaviouralNodes: []kramaid.KramaID{operator},
//					RandomNodes:      []kramaid.KramaID{nodeset.RandomSet().Infos[0]},
//				},
//			},
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engineParams := &createKramaEngineParams{
//				sm: sm,
//				cfgCallback: func(cfg *config.ConsensusConfig) {
//					cfg.EnableDebugMode = test.enableDebugMode
//				},
//			}
//
//			engine := createTestKramaEngine(t, engineParams)
//
//			err := engine.updateContextDelta(test.slot)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//
//			for id, ps := range test.slot.ClusterState().Participants {
//				checkContextDelta(t, ps.IsGenesis, test.expectedContextDelta[id], ps.ContextDelta)
//			}
//		})
//	}
//}
//
// func TestExecuteAndValidate(t *testing.T) {
//	var (
//		address        = tests.RandomAddress(t)
//		stateHash      = tests.RandomHash(t)
//		_, receipts    = getIxAndReceipts(t, 1)
//		accStateHashes = common.AccStateHashes{
//			address: {
//				StateHash: stateHash,
//			},
//		}
//	)
//
//	receiptsHash, err := receipts.Hash()
//	require.NoError(t, err)
//
//	transition := state.NewTransition(nil)
//
//	for k, r := range receipts {
//		transition.SetReceipt(k, r)
//	}
//
//	testcases := []struct {
//		name                    string
//		tsParams                *tests.CreateTesseractParams
//		transition              *state.Transition
//		revertHook              func() error
//		executeInteractionsHook func() (common.AccStateHashes, error)
//		expectedError           error
//	}{
//		{
//			name: "should return error if execution fails",
//			executeInteractionsHook: func() (common.AccStateHashes, error) {
//				return nil, errors.New("failed to execute interactions")
//			},
//			expectedError: errors.New("failed to execute interactions"),
//		},
//		{
//			name: "should return error if receipt validation fails",
//			tsParams: &tests.CreateTesseractParams{
//				TSDataCallback: func(ts *tests.TesseractData) {
//					ts.ReceiptsHash = tests.RandomHash(t)
//				},
//			},
//			transition: transition,
//			executeInteractionsHook: func() (common.AccStateHashes, error) {
//				return nil, nil
//			},
//			expectedError: errors.New("failed to validate the tesseract"),
//		},
//		{
//			name: "should return error if state hash validation fails",
//			tsParams: &tests.CreateTesseractParams{
//				AccountIDs: []identifiers.Identifier{address},
//				Participants: common.ParticipantsState{
//					address: {
//						StateHash: tests.RandomHash(t),
//					},
//				},
//				TSDataCallback: func(ts *tests.TesseractData) {
//					ts.ReceiptsHash = receiptsHash
//				},
//			},
//			transition: transition,
//			executeInteractionsHook: func() (common.AccStateHashes, error) {
//				return accStateHashes, nil
//			},
//			expectedError: errors.New("failed to validate the tesseract"),
//		},
//		{
//			name: "successfully executed and validated tesseract",
//			tsParams: &tests.CreateTesseractParams{
//				Participants: common.ParticipantsState{
//					address: {
//						StateHash: stateHash,
//					},
//				},
//				TSDataCallback: func(ts *tests.TesseractData) {
//					ts.ReceiptsHash = receiptsHash
//				},
//			},
//			transition: transition,
//			executeInteractionsHook: func() (common.AccStateHashes, error) {
//				return accStateHashes, nil
//			},
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			ts := tests.CreateTesseract(t, test.tsParams)
//
//			params := &createKramaEngineParams{
//				execCallback: func(exec *MockExec) {
//					exec.executeInteractionsHook = test.executeInteractionsHook
//				},
//			}
//
//			engine := createTestKramaEngine(t, params)
//
//			err := engine.ExecuteAndValidate(ts, test.transition, nil)
//
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//			require.Equal(t, receipts, ts.Receipts())
//		})
//	}
//}
//
// func TestAreStateHashesValid(t *testing.T) {
//	addresses := tests.GetAddresses(t, 2)
//	stateHashes := tests.GetHashes(t, 2)
//	accStateHashes := common.AccStateHashes{
//		addresses[0]: {StateHash: stateHashes[0]},
//		addresses[1]: {StateHash: stateHashes[1]},
//	}
//
//	testcases := []struct {
//		name        string
//		params      *tests.CreateTesseractParams
//		stateHashes common.AccStateHashes
//		isValid     bool
//	}{
//		{
//			name: "state hash in receipt and tesseract doesn't match",
//			params: &tests.CreateTesseractParams{
//				Participants: common.ParticipantsState{
//					addresses[0]: {
//						StateHash: tests.RandomHash(t),
//					},
//				},
//			},
//			stateHashes: accStateHashes,
//			isValid:     false,
//		},
//		{
//			name: "valid state hashes",
//			params: &tests.CreateTesseractParams{
//				AccountIDs: []identifiers.Identifier{addresses[0], addresses[1]},
//				Participants: common.ParticipantsState{
//					addresses[0]: {
//						StateHash: stateHashes[0],
//					},
//					addresses[1]: {
//						StateHash: stateHashes[1],
//					},
//				},
//			},
//			stateHashes: accStateHashes,
//			isValid:     true,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			ts := tests.CreateTesseract(t, test.params)
//
//			isValid := areStateHashesValid(ts, test.stateHashes)
//
//			require.Equal(t, test.isValid, isValid)
//		})
//	}
//}
//
// func TestVerifySignatures(t *testing.T) {
//	getTesseractParamsWithVoteset := func(index int, value bool) *createTesseractParams {
//		return &createTesseractParams{
//			TSDataCallback: func(ts *tests.TesseractData) {
//				ts.ConsensusInfo.BFTVoteSet.SetIndex(index, value)
//			},
//		}
//	}
//
//	tesseractParams := map[int]*createTesseractParams{
//		0: getTesseractParamsWithVoteset(0, false),
//		1: getTesseractParamsWithVoteset(2, false),
//		2: tesseractParamsWithCommitSign(invalidCommitSign),
//		3: tesseractParamsWithCommitSign(validCommitSign), // MockVerifiers recognizes validCommitSign
//	}
//
//	ts := createTesseracts(t, 5, tesseractParams)
//
//	nodes := getICSNodeset(t, 1, 1)
//
//	engine := createTestKramaEngine(t, nil)
//
//	testcases := []struct {
//		name          string
//		ts            *common.Tesseract
//		expectedError error
//	}{
//		{
//			name:          "should return error for invalid quorum size",
//			ts:            ts[0],
//			expectedError: common.ErrQuorumFailed,
//		},
//		{
//			name:          "should return error for invalid random quorum size",
//			ts:            ts[1],
//			expectedError: common.ErrQuorumFailed,
//		},
//		{
//			name:          "should return error for invalid commit signature",
//			ts:            ts[2],
//			expectedError: common.ErrSignatureVerificationFailed,
//		},
//		{
//			name: "valid commit signature",
//			ts:   ts[3],
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			val, err := engine.verifySignatures(test.ts, nodes)
//
//			if test.expectedError != nil {
//				require.False(t, val)
//				require.EqualError(t, test.expectedError, err.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//			require.True(t, val)
//		})
//	}
//}
//
// func TestParseGenesisFile_InvalidPath(t *testing.T) {
//	engineParams := &createKramaEngineParams{
//		cfgCallback: func(cfg *config.ConsensusConfig) {
//			cfg.GenesisFilePath = "test2/3647663731config.json"
//		},
//	}
//
//	engine := createTestKramaEngine(t, engineParams)
//	_, _, _, _, err := engine.parseGenesisFile() //nolint
//
//	require.ErrorContains(t, err, "failed to open genesis file")
//}
//
// func TestParseGenesisFile(t *testing.T) {
//	id := tests.RandomAddress(t)
//	nodes := tests.RandomKramaIDs(t, 4)
//	dir, err := os.MkdirTemp(os.TempDir(), " ")
//	require.NoError(t, err)
//
//	t.Cleanup(func() {
//		err = os.RemoveAll(dir)
//		require.NoError(t, err)
//	})
//
//	testcases := []struct {
//		name                 string
//		path                 string
//		sargaAccount         common.AccountSetupArgs
//		genesisAccounts      []common.AccountSetupArgs
//		genesisLogics        []common.LogicSetupArgs
//		genesisAssetAccounts []common.AssetAccountSetupArgs
//		invalidAccPayload    bool
//		expectedError        error
//	}{
//		{
//			name:              "should return error for invalid genesis payload",
//			genesisAccounts:   nil,
//			invalidAccPayload: true,
//			expectedError:     errors.New("failed to parse genesis file"),
//		},
//		{
//			name:            "should return error if sarga account info is invalid",
//			sargaAccount:    getTestAccountWithAccType(t, tests.InvalidAccount),
//			genesisAccounts: []common.AccountSetupArgs{},
//			expectedError:   errors.New("invalid sarga account info"),
//		},
//		{
//			name:         "should return error if genesis account info is invalid",
//			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
//			genesisAccounts: []common.AccountSetupArgs{
//				getTestAccountWithAccType(t, tests.InvalidAccount),
//			},
//			expectedError: errors.New("invalid genesis account creation info"),
//		},
//		{
//			name:         "should succeed for valid genesis data without contract paths",
//			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
//			genesisAccounts: []common.AccountSetupArgs{
//				getTestAccountWithAccType(t, common.RegularAccount),
//				getTestAccountWithAccType(t, common.LogicAccount),
//			},
//		},
//		{
//			name:         "should succeed for valid genesis data with contract paths",
//			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
//			genesisAccounts: []common.AccountSetupArgs{
//				getTestAccountWithAccType(t, common.RegularAccount),
//				getTestAccountWithAccType(t, common.LogicAccount),
//			},
//			genesisLogics: getTestGenesisLogics(t),
//		},
//		{
//			name:         "should succeed for valid genesis asset account with invalid allocations",
//			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
//			genesisAccounts: []common.AccountSetupArgs{
//				getTestAccountWithAccType(t, common.RegularAccount),
//				getTestAccountWithAccType(t, common.LogicAccount),
//			},
//			genesisAssetAccounts: []common.AssetAccountSetupArgs{
//				// using nilAddress for allocations
//				getAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, identifiers.Nil), nodes[:2], nodes[2:]),
//			},
//		},
//		{
//			name:         "should succeed for valid genesis asset account with valid allocations",
//			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
//			genesisAccounts: []common.AccountSetupArgs{
//				getTestAccountWithAddress(t, id),
//				getTestAccountWithAccType(t, common.LogicAccount),
//			},
//			genesisAssetAccounts: []common.AssetAccountSetupArgs{
//				getAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, id), nodes[:2], nodes[2:]),
//			},
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			path := createMockGenesisFile(t, dir, test.invalidAccPayload, test.sargaAccount,
//				test.genesisAccounts, test.genesisAssetAccounts, test.genesisLogics)
//
//			engineParams := &createKramaEngineParams{
//				cfgCallback: func(cfg *config.ConsensusConfig) {
//					cfg.GenesisFilePath = path
//				},
//			}
//
//			engine := createTestKramaEngine(t, engineParams)
//
//			sargaAccount, genesisAccounts, assetAccounts, genesisLogics, err := engine.parseGenesisFile()
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//			require.NoError(t, err)
//
//			// check for sarga account
//			require.Equal(t, test.sargaAccount, *sargaAccount)
//
//			// check for genesis accounts
//			for i, genesisAccount := range test.genesisAccounts {
//				require.Equal(t, genesisAccount, genesisAccounts[i])
//			}
//
//			checkForLogicAccounts(t, test.genesisLogics, genesisLogics)
//
//			checkForAssetAccounts(t, test.genesisAssetAccounts, assetAccounts)
//		})
//	}
//}
//
// func TestSetupSargaAccount(t *testing.T) {
//	engine := createTestKramaEngine(t, nil)
//	nodes := tests.RandomKramaIDs(t, 12)
//
//	var emptyNodes []kramaid.KramaID
//
//	testcases := []struct {
//		name          string
//		sarga         *common.AccountSetupArgs
//		accounts      []common.AccountSetupArgs
//		assets        []common.AssetAccountSetupArgs
//		logics        []common.LogicSetupArgs
//		expectedError error
//	}{
//		{
//			name: "behavioural nodes and random nodes are empty",
//			sarga: getAccountSetupArgs(t,
//				common.SargaAddress,
//				2,
//				"moi-id",
//				emptyNodes,
//				emptyNodes,
//			),
//			expectedError: errors.New("context initiation failed in genesis"),
//		},
//		{
//			name: "other accounts added to sarga account",
//			sarga: getAccountSetupArgs(
//				t,
//				common.SargaAddress,
//				2,
//				"moi-id",
//				nodes[:2],
//				nodes[2:4],
//			),
//			accounts: []common.AccountSetupArgs{
//				*getAccountSetupArgs(
//					t,
//					tests.RandomAddress(t),
//					3,
//					"moi-id",
//					nodes[4:6],
//					nodes[6:8],
//				),
//				*getAccountSetupArgs(
//					t,
//					tests.RandomAddress(t),
//					9,
//					"moi-id",
//					nodes[8:10],
//					nodes[10:12],
//				),
//			},
//			assets: []common.AssetAccountSetupArgs{
//				getAssetAccountSetupArgs(
//					t,
//					getTestAssetCreationArgs(t, identifiers.Nil), nil, nil),
//				getAssetAccountSetupArgs(
//					t,
//					getTestAssetCreationArgs(t, identifiers.Nil), nil, nil),
//			},
//			logics: []common.LogicSetupArgs{
//				{
//					Name: "staking",
//				},
//				{
//					Name: "DEX",
//				},
//			},
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			obj, err := engine.setupSargaAccount(test.sarga, test.accounts, test.assets, test.logics)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//
//			validateContextInitialization(
//				t,
//				obj,
//				common.SargaAddress,
//				obj.ContextHash(),
//			)
//
//			checkSargaObjectAccounts(t, obj, test.accounts)
//			checkSargaObjectLogicAccounts(t, obj, test.logics)
//			checkSargaObjectAssetAccounts(t, obj, test.assets)
//		})
//	}
//}
//
// func TestSetupNewAccount(t *testing.T) {
//	engine := createTestKramaEngine(t, nil)
//	nodes := tests.RandomKramaIDs(t, 12)
//
//	testcases := []struct {
//		name               string
//		newAcc             *common.AccountSetupArgs
//		expectedError      error
//		behaviouralContext []kramaid.KramaID
//		randomContext      []kramaid.KramaID
//	}{
//		{
//			name: "behavioural nodes and random nodes are empty",
//			newAcc: getAccountSetupArgs(t,
//				tests.RandomAddress(t),
//				3,
//				"moi-id",
//				nil,
//				nil,
//			),
//			expectedError: errors.New("context initiation failed in genesis"),
//		},
//		{
//			name: "behavioural nodes and random nodes are set",
//			newAcc: getAccountSetupArgs(t,
//				tests.RandomAddress(t),
//				3,
//				"moi-id",
//				nodes[:6],
//				nodes[6:],
//			),
//			expectedError: nil,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			stateObject, err := engine.setupNewAccount(*test.newAcc)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//
//			validateContextInitialization(
//				t,
//				stateObject,
//				test.newAcc.Identifier,
//				stateObject.ContextHash(),
//			)
//		})
//	}
//}
//
// func TestExecuteGenesisContracts(t *testing.T) {
//	ids := tests.RandomKramaIDs(t, 1)
//
//	objectsMap := make(map[identifiers.Identifier]*state.Object)
//
//	testcases := []struct {
//		name          string
//		logics        []common.LogicSetupArgs
//		expectedError string
//	}{
//		{
//			name: "invalid logic name",
//			logics: []common.LogicSetupArgs{{
//				Name:               "test",
//				BehaviouralContext: ids,
//			}},
//			expectedError: "generated address does not exist in predefined contract address",
//		},
//		{
//			name: "missing behaviour context",
//			logics: []common.LogicSetupArgs{{
//				Name:               "staking-contract",
//				BehaviouralContext: nil,
//			}},
//			expectedError: "context initiation failed in genesis",
//		},
//		{
//			name:   "valid contract paths",
//			logics: getTestGenesisLogics(t),
//		},
//		{
//			name: "unable to deploy logic for contract",
//			logics: []common.LogicSetupArgs{
//				{
//					Name:               "staking-contract",
//					BehaviouralContext: tests.RandomKramaIDs(t, 1),
//				},
//			},
//			expectedError: "deployment failed for logic",
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engine := createTestKramaEngine(t, nil)
//
//			_, err := engine.setupGenesisLogics(objectsMap, test.logics)
//
//			if test.expectedError != "" {
//				require.ErrorContains(t, err, test.expectedError)
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}
//
// func TestSetupAssetAccounts(t *testing.T) {
//	nodes := tests.RandomKramaIDs(t, 8)
//	owner := tests.RandomAddress(t)
//	address := tests.GetAddresses(t, 2)
//	amount := []*big.Int{big.NewInt(100), big.NewInt(200)}
//
//	MOIAssetInfo := getAssetCreationArgs(
//		"MOI",
//		owner,
//		address,
//		amount,
//	)
//
//	MOIAssetSetupArgs := common.AssetAccountSetupArgs{
//		AssetInfo:          MOIAssetInfo,
//		BehaviouralContext: nodes[:2],
//		RandomContext:      nodes[2:4],
//	}
//
//	testcases := []struct {
//		name          string
//		assetAccs     []common.AssetAccountSetupArgs
//		smCallback    func(sm *MockStateManager, stateObjects map[identifiers.Identifier]*state.Object)
//		expectedError error
//	}{
//		{
//			name: "should succeed for valid asset info",
//			assetAccs: []common.AssetAccountSetupArgs{
//				MOIAssetSetupArgs,
//				getAssetAccountSetupArgs(
//					t,
//					*getAssetCreationArgs(
//						"WILL",
//						owner,
//						address,
//						amount,
//					),
//					nodes[4:6],
//					nodes[6:8],
//				),
//			},
//			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Identifier]*state.Object) {
//				stateObjects[owner] = sm.CreateStateObject(owner, common.RegularAccount, true)
//
//				for i := 0; i < len(address); i++ {
//					stateObjects[address[i]] = sm.CreateStateObject(address[i], common.RegularAccount, true)
//				}
//			},
//		},
//		{
//			name: "should return error if failed to create context",
//			assetAccs: []common.AssetAccountSetupArgs{
//				getAssetAccountSetupArgs(t, *MOIAssetInfo, nil, nil),
//			},
//			expectedError: errors.New("liveliness size not met"),
//		},
//		{
//			name: "should return error if owner account not found",
//			assetAccs: []common.AssetAccountSetupArgs{
//				MOIAssetSetupArgs,
//			},
//			expectedError: errors.New("operator account not found"),
//		},
//		{
//			name: "should return error if asset already registered in owner account",
//			assetAccs: []common.AssetAccountSetupArgs{
//				MOIAssetSetupArgs,
//			},
//			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Identifier]*state.Object) {
//				stateObjects[owner] = sm.CreateStateObject(owner, common.RegularAccount, true)
//				accAddress := common.CreateAddressFromString(MOIAssetInfo.Symbol)
//				_, err := stateObjects[MOIAssetInfo.Operator].CreateAsset(accAddress, MOIAssetInfo.AssetDescriptor())
//				require.NoError(t, err)
//			},
//			expectedError: common.ErrAssetAlreadyRegistered,
//		},
//		{
//			name: "should return error if allocation address not found in state objects",
//			assetAccs: []common.AssetAccountSetupArgs{
//				MOIAssetSetupArgs,
//			},
//			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Identifier]*state.Object) {
//				stateObjects[owner] = sm.CreateStateObject(owner, common.RegularAccount, true)
//			},
//			expectedError: errors.New("allocation address not found in state objects"),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			stateObjects := make(map[identifiers.Identifier]*state.Object)
//			sm := NewMockStateManager()
//
//			if test.smCallback != nil {
//				test.smCallback(sm, stateObjects)
//			}
//
//			engine := createTestKramaEngine(t, nil)
//
//			err := engine.setupAssetAccounts(stateObjects, test.assetAccs)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//
//			for i := 0; i < len(test.assetAccs); i++ {
//				assetInfo := test.assetAccs[i].AssetInfo
//				expectedAssetDescriptor, err := assetInfo.AssetDescriptor().Bytes()
//				require.NoError(t, err)
//
//				assetAccAddress := common.CreateAddressFromString(assetInfo.Symbol)
//				assetSO, ok := stateObjects[assetAccAddress]
//				require.True(t, ok)
//
//				assetID := identifiers.NewAssetIDv0(
//					assetInfo.IsLogical,
//					assetInfo.IsStateful,
//					assetInfo.Dimension.ToInt(),
//					assetInfo.Standard.ToInt(),
//					assetAccAddress,
//				)
//
//				validateContextInitialization(
//					t,
//					assetSO,
//					assetAccAddress,
//					assetSO.ContextHash(),
//				)
//
//				checkForAssetRegistry(t, assetSO, assetID, expectedAssetDescriptor)
//
//				ownerSO, ok := stateObjects[assetInfo.Operator]
//				require.True(t, ok)
//
//				checkForAssetRegistry(t, ownerSO, assetID, expectedAssetDescriptor)
//				checkForAllocations(t, stateObjects, assetInfo, assetID)
//			}
//		})
//	}
//}
//
// func TestAddGenesisTesseract(t *testing.T) {
//	var (
//		addresses     = tests.GetAddresses(t, 1)
//		stateHashes   = tests.GetHashes(t, 1)
//		contextHashes = tests.GetHashes(t, 1)
//	)
//
//	testcases := []struct {
//		name          string
//		cmCallBack    func(sm *MockChainManager)
//		expectedError error
//	}{
//		{
//			name: "adding genesis tesseract successful",
//		},
//		{
//			name: "failed to add genesis tesseract",
//			cmCallBack: func(cm *MockChainManager) {
//				cm.addTesseractHook = func() error {
//					return errors.New("failed to add tesseract")
//				}
//			},
//			expectedError: errors.New("failed to add tesseract"),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			cm := NewMockChainManager()
//
//			engineParams := &createKramaEngineParams{
//				cm:         cm,
//				cmCallback: test.cmCallBack,
//			}
//
//			engine := createTestKramaEngine(t, engineParams)
//
//			err := engine.addGenesisTesseract(addresses, stateHashes, contextHashes, nil)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//
//			ts, ok := cm.tesseractByAddress[addresses[0]]
//			require.True(t, ok)
//
//			require.Equal(t, contextHashes[0], ts.LatestContextHash(addresses[0]))
//		},
//		)
//	}
//}
//
// func TestSetupGenesis(t *testing.T) {
//	var (
//		sargaAccount    = getTestAccountWithAccType(t, common.SargaAccount)
//		genesisAccounts = []common.AccountSetupArgs{
//			getTestAccountWithAccType(t, common.RegularAccount),
//			getTestAccountWithAccType(t, common.LogicAccount),
//		}
//		assetSetupArgs = []common.AssetAccountSetupArgs{
//			{
//				AssetInfo: getAssetCreationArgs(
//					"MOI",
//					genesisAccounts[0].Identifier,
//					[]identifiers.Identifier{genesisAccounts[1].Identifier},
//					[]*big.Int{big.NewInt(12)},
//				),
//				BehaviouralContext: tests.RandomKramaIDs(t, 1),
//			},
//		}
//		logics = getTestGenesisLogics(t)
//	)
//
//	dir, err := os.MkdirTemp(os.TempDir(), " ")
//	require.NoError(t, err)
//
//	t.Cleanup(func() {
//		err = os.RemoveAll(dir)
//		require.NoError(t, err)
//	})
//
//	tmpDir, err := os.MkdirTemp("", "consensus-test")
//	if err != nil {
//		panic(err)
//	}
//
//	cfg := &config.ConsensusConfig{
//		GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount,
//			genesisAccounts, assetSetupArgs, logics),
//		GenesisTimestamp: uint64(time.Now().Unix()),
//		DirectoryPath:    tmpDir,
//	}
//
//	assetAccAddr := common.CreateAddressFromString(assetSetupArgs[0].AssetInfo.Symbol)
//	logicAddr := common.CreateAddressFromString(logics[0].Name)
//
//	testcases := []struct {
//		name          string
//		cfg           *config.ConsensusConfig
//		cmCallBack    func(sm *MockChainManager)
//		expectedError error
//	}{
//		{
//			name: "should succeed for valid genesis info",
//			cfg:  cfg,
//		},
//		{
//			name: "should return error if failed to add genesis tesseract",
//			cfg:  cfg,
//			cmCallBack: func(cm *MockChainManager) {
//				cm.addTesseractHook = func() error {
//					return errors.New("failed to add tesseract")
//				}
//			},
//			expectedError: errors.New("failed to add tesseract"),
//		},
//		{
//			name: "should return error if failed to parse genesis file",
//			cfg: &config.ConsensusConfig{
//				GenesisFilePath: createMockGenesisFile(t, dir, true, sargaAccount, genesisAccounts,
//					nil, nil),
//				GenesisTimestamp: uint64(time.Now().Unix()),
//				DirectoryPath:    tmpDir,
//			},
//			expectedError: errors.New("failed to parse genesis file"),
//		},
//		{
//			name: "should return error if failed to setup sarga account",
//			cfg: &config.ConsensusConfig{
//				GenesisFilePath: createMockGenesisFile(t, dir, false,
//					*getAccountSetupArgs(
//						t,
//						common.SargaAddress,
//						common.SargaAccount,
//						"moi-id",
//						nil,
//						nil,
//					), genesisAccounts, nil, nil),
//				GenesisTimestamp: uint64(time.Now().Unix()),
//				DirectoryPath:    tmpDir,
//			},
//			expectedError: errors.New("failed to setup sarga account"),
//		},
//		{
//			name: "should return error if failed to setup genesis account",
//			cfg: &config.ConsensusConfig{
//				GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount,
//					[]common.AccountSetupArgs{*getAccountSetupArgs(
//						t,
//						tests.RandomAddress(t),
//						common.RegularAccount,
//						"moi-id",
//						nil,
//						nil,
//					)}, nil, nil),
//				GenesisTimestamp: uint64(time.Now().Unix()),
//				DirectoryPath:    tmpDir,
//			},
//			expectedError: errors.New("failed to setup genesis account"),
//		},
//		{
//			name: "should return error if failed to setup genesis logic",
//			cfg: &config.ConsensusConfig{
//				GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts,
//					nil,
//					[]common.LogicSetupArgs{
//						{
//							Name:               "staking-contract",
//							Manifest:           hexutil.Bytes{0, 1},
//							BehaviouralContext: tests.RandomKramaIDs(t, 1),
//						},
//					}),
//				GenesisTimestamp: uint64(time.Now().Unix()),
//				DirectoryPath:    tmpDir,
//			},
//			expectedError: errors.New("failed to setup genesis logic"),
//		},
//		{
//			name: "should return error if failed to setup assets",
//			cfg: &config.ConsensusConfig{
//				GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts,
//					[]common.AssetAccountSetupArgs{
//						getAssetAccountSetupArgs(t, *getAssetCreationArgs(
//							"MOI",
//							tests.RandomAddress(t),
//							[]identifiers.Identifier{tests.RandomAddress(t)},
//							[]*big.Int{big.NewInt(12)},
//						), nil, nil),
//					}, nil),
//				GenesisTimestamp: uint64(time.Now().Unix()),
//				DirectoryPath:    tmpDir,
//			},
//			expectedError: errors.New("failed to setup asset accounts"),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			cm := NewMockChainManager()
//			sm := NewMockStateManager()
//
//			engineParams := &createKramaEngineParams{
//				smCallback: func(sm *MockStateManager) {
//					sm.GetAccountMetaInfoHook = func() error {
//						return errors.New("failed to get acc meta info hook")
//					}
//				},
//				cmCallback: test.cmCallBack,
//				cfg:        test.cfg,
//				cm:         cm,
//				sm:         sm,
//			}
//
//			engine := createTestKramaEngine(t, engineParams)
//
//			err = engine.SetupGenesis()
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//
//			checkForTS := func(id identifiers.Identifier) {
//				_, ok := cm.tesseractByAddress[id]
//				require.True(t, ok)
//			}
//
//			checkForTS(common.SargaAddress)
//
//			for _, genesisAccount := range genesisAccounts {
//				checkForTS(genesisAccount.Identifier)
//			}
//
//			checkForTS(assetAccAddr)
//			checkForTS(logicAddr)
//		})
//	}
//}
//
// func TestVerifyParticipantState(t *testing.T) {
//	address := tests.RandomAddress(t)
//
//	getTesseractParams := func(clusterID string, height uint64) *createTesseractParams {
//		return &createTesseractParams{
//			AccountIDs: []identifiers.Identifier{address},
//			Participants: common.ParticipantsState{
//				address: {
//					Height: height,
//				},
//			},
//			TSDataCallback: func(ts *tests.TesseractData) {
//				ts.ConsensusInfo.ClusterID = common.ClusterID(clusterID)
//			},
//		}
//	}
//
//	getTesseractParamsWithTimeStamp := func(height uint64, timestamp uint64) *createTesseractParams {
//		return &createTesseractParams{
//			AccountIDs: []identifiers.Identifier{address},
//			Participants: common.ParticipantsState{
//				address: {
//					Height: height,
//				},
//			},
//			TSDataCallback: func(ts *tests.TesseractData) {
//				ts.ConsensusInfo.ClusterID = "non-genesis"
//				ts.Timestamp = timestamp
//			},
//		}
//	}
//
//	testcases := []struct {
//		name          string
//		paramsMap     map[int]*createTesseractParams
//		smCallBack    func(sm *MockStateManager)
//		cmCallback    func(cm *MockChainManager)
//		expectedError error
//	}{
//		{
//			name: "valid tesseract ",
//			paramsMap: map[int]*createTesseractParams{
//				0: getTesseractParams("non-genesis", 0),
//				1: getTesseractParams("non-genesis", 1),
//			},
//		},
//		{
//			name: "should skip for genesis tesseract",
//			paramsMap: map[int]*createTesseractParams{
//				0: getTesseractParams("genesis", 0),
//			},
//		},
//		{
//			name: "should return error if previous tesseract not found",
//			paramsMap: map[int]*createTesseractParams{
//				0: getTesseractParams("non-genesis", 0),
//				1: getTesseractParams("non-genesis", 1),
//			},
//			cmCallback: func(cm *MockChainManager) {
//				cm.GetTesseractHook = func() error {
//					return common.ErrPreviousTesseractNotFound
//				}
//			},
//			expectedError: common.ErrPreviousTesseractNotFound,
//		},
//		{
//			name: "should return error for invalid height",
//			paramsMap: map[int]*createTesseractParams{
//				0: getTesseractParams("non-genesis", 0),
//				1: getTesseractParams("non-genesis", 2),
//			},
//			expectedError: common.ErrInvalidHeight,
//		},
//		{
//			name: "should return error for invalid time stamp",
//			paramsMap: map[int]*createTesseractParams{
//				0: getTesseractParamsWithTimeStamp(0, 3),
//				1: getTesseractParamsWithTimeStamp(1, 2),
//			},
//			expectedError: common.ErrInvalidBlockTime,
//		},
//		{
//			name: "should return error if sarga account not found",
//			paramsMap: map[int]*createTesseractParams{
//				0: getTesseractParams("non-genesis", 0),
//			},
//			smCallBack: func(sm *MockStateManager) {
//				sm.IsInitialTesseractHook = func() error {
//					return errors.New("Sarga account not found")
//				}
//			},
//			expectedError: errors.New("Sarga account not found"),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			ts := createTesseractsWithChain(t, len(test.paramsMap), test.paramsMap)
//			cm := NewMockChainManager()
//			cm.insertTesseracts(ts...)
//
//			engineParams := &createKramaEngineParams{
//				cm:         cm,
//				smCallback: test.smCallBack,
//				cmCallback: test.cmCallback,
//			}
//
//			engine := createTestKramaEngine(t, engineParams)
//
//			err := engine.verifyTransitions(ts[len(ts)-1].AnyAccountID(), ts[len(ts)-1], false)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}
//
// func TestValidateAccountCreationInfo(t *testing.T) {
//	var (
//		walletContext = tests.RandomKramaIDs(t, 4)
//		address       = tests.RandomAddress(t)
//	)
//
//	testcases := []struct {
//		name          string
//		accountInfo   *common.AccountSetupArgs
//		expectedError error
//	}{
//		{
//			name: "should fail for invalid account type",
//			accountInfo: getAccountSetupArgs(
//				t,
//				address,
//				tests.InvalidAccount,
//				"moi-id",
//				walletContext[:2],
//				walletContext[2:4],
//			),
//			expectedError: common.ErrInvalidAccountType,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engine := createTestKramaEngine(t, nil)
//
//			err := engine.validateAccountCreationInfo(*test.accountInfo)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}
//
// func TestValidateTesseract(t *testing.T) {
//	nodes := getICSNodeset(t, 1, 1)
//	tsParamsMap := map[int]*createTesseractParams{
//		0: tesseractParamsWithCommitSign(validCommitSign),
//		1: tesseractParamsWithCommitSign(invalidCommitSign),
//		2: tesseractParamsWithCommitSign(validCommitSign),
//	}
//
//	ts := createTesseracts(t, 7, tsParamsMap)
//
//	testcases := []struct {
//		name          string
//		ts            *common.Tesseract
//		smCallback    func(sm *MockStateManager)
//		dbCallback    func(db *MockDB)
//		expectedError error
//	}{
//		{
//			name: "valid tesseract",
//			ts:   ts[0],
//		},
//		{
//			name: "failed to verify transitions",
//			ts:   ts[0],
//			smCallback: func(sm *MockStateManager) {
//				sm.IsInitialTesseractHook = func() error {
//					return errors.New("failed to verify transitions")
//				}
//			},
//			expectedError: errors.New("failed to verify transitions"),
//		},
//		{
//			name:          "failed to verify signature",
//			ts:            ts[1],
//			expectedError: common.ErrSignatureVerificationFailed,
//		},
//		{
//			name: "should return error if tesseract already exits",
//			ts:   ts[2],
//			dbCallback: func(db *MockDB) {
//				db.setAccMetaInfoAt(ts[2].AnyAccountID())
//			},
//			expectedError: common.ErrAlreadyKnown,
//		},
//		{
//			name: "invalid tesseract seal",
//			ts:   ts[0],
//			smCallback: func(sm *MockStateManager) {
//				sm.isSealValid = func() bool {
//					return false
//				}
//			},
//			expectedError: common.ErrInvalidSeal,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engineParams := &createKramaEngineParams{
//				smCallback: test.smCallback,
//				dbCallback: test.dbCallback,
//			}
//
//			engine := createTestKramaEngine(t, engineParams)
//
//			err := engine.ValidateTesseract(test.ts.AnyAccountID(), test.ts, nodes, false)
//			if test.expectedError != nil {
//				require.ErrorContains(t, err, test.expectedError.Error())
//
//				return
//			}
//
//			require.NoError(t, err)
//		})
//	}
//}
//
// func TestIsReceiptHashValid(t *testing.T) {
//	var receipts common.Receipts
//
//	receiptsHash, err := receipts.Hash()
//	require.NoError(t, err)
//
//	testcases := []struct {
//		name      string
//		paramsMap *tests.CreateTesseractParams
//		isValid   bool
//	}{
//		{
//			name: "valid receipt hash",
//			paramsMap: &tests.CreateTesseractParams{
//				TSDataCallback: func(ts *tests.TesseractData) {
//					ts.ReceiptsHash = receiptsHash
//				},
//			},
//			isValid: true,
//		},
//		{
//			name: "invalid receipt hash",
//			paramsMap: &tests.CreateTesseractParams{
//				TSDataCallback: func(ts *tests.TesseractData) {
//					ts.ReceiptsHash = tests.RandomHash(t)
//				},
//			},
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			ts := tests.CreateTesseract(t, test.paramsMap)
//
//			isValid := isReceiptsHashValid(ts, receipts)
//			require.Equal(t, test.isValid, isValid)
//		})
//	}
//}
//
// func TestFetchParticipantsAndNodeSet(t *testing.T) {
//	ms := NewMockStateManager()
//
//	_, ixnPs := tests.CreateTestIxParticipants(t, 3, 1)
//
//	// create an address without nodeSets
//	addrsWithOutContext := tests.RandomAddress(t)
//	ms.addAccMetaInfo(t, addrsWithOutContext, tests.GetRandomAccMetaInfo(t, rand.Uint64()))
//
//	// add nodeSets and accountMetaInfo to state manager
//	for id, p := range ixnPs {
//		// avoid storing meta info for genesis accounts
//		if !p.IsGenesis {
//			ms.addAccMetaInfo(t, id, tests.GetRandomAccMetaInfo(t, rand.Uint64()))
//		}
//
//		ms.addNodeSet(t, id, tests.RandomHash(t), createTestNodeSet(t, 3), createTestNodeSet(t, 3))
//	}
//
//	testcases := []struct {
//		name            string
//		ixnParticipants map[identifiers.Identifier]common.ParticipantInfo
//		expectedError   error
//	}{
//		{
//			name: "should return error if failed to fetch accMetaInfo",
//			ixnParticipants: map[identifiers.Identifier]common.ParticipantInfo{
//				tests.RandomAddress(t): {IsGenesis: false},
//			},
//			expectedError: common.ErrFetchingAccMetaInfo,
//		},
//		{
//			name: "should return error if failed to fetch context info",
//			ixnParticipants: map[identifiers.Identifier]common.ParticipantInfo{
//				addrsWithOutContext: {IsGenesis: false, LockType: 1},
//			},
//			expectedError: common.ErrContextStateNotFound,
//		},
//		{
//			name:            "should return participants info and nodeSet",
//			ixnParticipants: ixnPs,
//			expectedError:   nil,
//		},
//	}
//
//	engine := createTestKramaEngine(t, &createKramaEngineParams{sm: ms})
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			ps, ns, err := engine.fetchParticipantsAndCommittee(context.Background(), test.ixnParticipants)
//			if test.expectedError != nil {
//				require.ErrorIs(t, err, test.expectedError)
//			}
//			checkParticipantInfo(t, ms, test.ixnParticipants, ps)
//			checkNodeSetForParticipant(t, ms, ps, ns)
//		})
//	}
//}
//
// func NewTestSlot(
//	t *testing.T,
//	operator, self kramaid.KramaID,
//	ns *ktypes.ICSCommittee,
//	ixns common.Interactions,
//	ps common.Participants,
// ) *ktypes.Slot {
//	t.Helper()
//
//	s := ktypes.NewSlot(ktypes.OperatorSlot, ps.IxnParticipants())
//	s.UpdateClusterState(createTestClusterState(t,
//		operator,
//		self,
//		ns,
//		ixns,
//		ps,
//		nil,
//	))
//
//	return s
//}
//
// func TestEngine_runLottery(t *testing.T) {
//	nonEligibleVault := createSortitionVault(t, false)
//	eligibleVault := createSortitionVault(t, true)
//
//	testcases := []struct {
//		name        string
//		lk          common.LotteryKey
//		params      *createKramaEngineParams
//		preTestFn   func(lk common.LotteryKey, engine *Engine)
//		expectedErr error
//	}{
//		{
//			name: "cache hit with eligible operator",
//			lk:   createTestLotteryKey(t, common.NilHash, common.NilHash),
//			params: &createKramaEngineParams{
//				vault: eligibleVault,
//			},
//			preTestFn: func(lk common.LotteryKey, engine *Engine) {
//				engine.lottery.cache.Add(
//					lk, NewLotteryResult(true, testVRFOutput, testVRFProof),
//				)
//				engine.lottery.AddICSOperatorInfo(lk, eligibleVault.KramaID(), 2)
//			},
//		},
//		{
//			name: "cache hit with non eligible operator",
//			lk:   createTestLotteryKey(t, common.NilHash, tests.RandomHash(t)),
//			preTestFn: func(lk common.LotteryKey, engine *Engine) {
//				engine.lottery.cache.Add(
//					lk, NewLotteryResult(false, [32]byte{}, []byte{}),
//				)
//			},
//			expectedErr: ErrOperatorNotEligible,
//		},
//		{
//			name: "cache miss with eligible operator",
//			// We need a constant value for Seed, so that priority can be estimated
//			lk: createTestLotteryKey(t, common.NilHash, common.NilHash),
//			params: &createKramaEngineParams{
//				vault: eligibleVault,
//				smCallback: func(sm *MockStateManager) {
//					updateGuardianIncentive(t, sm, eligibleVault.KramaID(), 10)
//					updateTotalIncentives(t, sm, 50)
//				},
//			},
//		},
//		{
//			name: "cache miss with non eligible operator",
//			params: &createKramaEngineParams{
//				vault: nonEligibleVault,
//				smCallback: func(sm *MockStateManager) {
//					updateGuardianIncentive(t, sm, nonEligibleVault.KramaID(), 5)
//					updateTotalIncentives(t, sm, 50)
//				},
//			},
//			expectedErr: ErrOperatorNotEligible,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engine := createTestKramaEngine(t, test.params)
//
//			if test.preTestFn != nil {
//				test.preTestFn(test.lk, engine)
//			}
//
//			output, proof, err := engine.runLottery(test.lk)
//
//			if test.expectedErr != nil {
//				require.Error(t, err)
//				require.Equal(t, test.expectedErr, err)
//
//				return
//			}
//
//			require.NoError(t, err)
//			require.NotEmpty(t, output)
//			require.NotEmpty(t, proof)
//			// priority is 2, based on calculation
//			checkForOperatorInfo(t, engine, test.lk, test.params.vault.KramaID(), 2)
//		})
//	}
//}
//
// func TestEngine_checkOperatorEligibility(t *testing.T) {
//	eligibleVault := createSortitionVault(t, true)
//	ls := NewLotteryResult(true, testVRFOutput, testVRFProof)
//	testcases := []struct {
//		name        string
//		params      *createKramaEngineParams
//		cs          *ktypes.ClusterState
//		req         ktypes.Request
//		preTestFn   func(lk common.LotteryKey, engine *Engine)
//		vrfOutput   [32]byte
//		vrfProof    []byte
//		expectedErr error
//	}{
//		{
//			name: "lottery disabled",
//			params: &createKramaEngineParams{
//				cfgCallback: func(cfg *config.ConsensusConfig) {
//					cfg.EnableSortition = false
//				},
//			},
//			cs:        &ktypes.ClusterState{},
//			req:       ktypes.Request{},
//			vrfProof:  nil,
//			vrfOutput: [32]byte{},
//		},
//
//		{
//			name: "should run lottery for self request",
//			params: &createKramaEngineParams{
//				cfgCallback: func(cfg *config.ConsensusConfig) {
//					cfg.EnableSortition = true
//				},
//			},
//			preTestFn: func(lk common.LotteryKey, engine *Engine) {
//				engine.lottery.cache.Add(
//					lk, ls,
//				)
//			},
//			cs: &ktypes.ClusterState{
//				LotteryKey: createTestLotteryKey(t, common.NilHash, tests.RandomHash(t)),
//			},
//			req: ktypes.Request{
//				Operator: eligibleVault.KramaID(),
//			},
//			vrfOutput: testVRFOutput,
//			vrfProof:  testVRFProof,
//		},
//		{
//			name: "should verify the lottery result for peer request",
//			params: &createKramaEngineParams{
//				cfgCallback: func(cfg *config.ConsensusConfig) {
//					cfg.EnableSortition = true
//				},
//			},
//			preTestFn: func(lk common.LotteryKey, engine *Engine) {
//				engine.lottery.cache.Add(
//					lk, ls,
//				)
//			},
//			cs: &ktypes.ClusterState{
//				operator:   tests.RandomKramaIDs(t, 1)[0],
//				LotteryKey: createTestLotteryKey(t, common.NilHash, tests.RandomHash(t)),
//			},
//			req: ktypes.Request{
//				Msg: &ktypes.CanonicalICSRequest{
//					VrfOutput: testVRFOutput,
//					VrfProof:  testVRFProof,
//				},
//			},
//			expectedErr: common.ErrPublicKeyNotFound,
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engine := createTestKramaEngine(t, test.params)
//
//			if test.preTestFn != nil {
//				test.preTestFn(test.cs.LotteryKey, engine)
//			}
//
//			err := engine.checkOperatorEligibility(test.cs, test.req)
//			if test.expectedErr != nil {
//				require.Error(t, err)
//				require.Equal(t, test.expectedErr, err)
//
//				return
//			}
//
//			require.NoError(t, err)
//			require.Equal(t, test.vrfOutput, test.cs.VRFOutput)
//			require.Equal(t, test.vrfProof, test.cs.VRFProof)
//		})
//	}
//}
//
// func TestEngine_verifyOperatorLottery(t *testing.T) {
//	vault := createSortitionVault(t, true)
//
//	sk := new(blst.SecretKey)
//	sk.Deserialize(vault.GetConsensusPrivateKey().Bytes())
//
//	icsSeed := [32]byte{1, 2, 5, 6, 9, 70}
//	vrfPrivKey := vrf.NewVRFSigner(sk)
//	icsOutput, icsProof, _ := vrfPrivKey.Evaluate(icsSeed[:])
//
//	testcases := []struct {
//		name        string
//		icsOutput   [32]byte
//		icsProof    []byte
//		lk          common.LotteryKey
//		params      *createKramaEngineParams
//		preTestFn   func(engine *Engine)
//		expectedErr error
//	}{
//		{
//			name:      "valid vrf proof",
//			icsOutput: icsOutput,
//			icsProof:  icsProof,
//			lk:        createTestLotteryKey(t, common.NilHash, icsSeed),
//			params: &createKramaEngineParams{
//				vault: vault,
//				smCallback: func(sm *MockStateManager) {
//					updateTotalIncentives(t, sm, uint64(ExpectedSortionSize))
//					updateGuardianIncentive(t, sm, vault.KramaID(), 5)
//					sm.setPublicKey(vault.kramaID, vault.GetConsensusPrivateKey().GetPublicKeyInBytes())
//				},
//			},
//		},
//		{
//			name:      "should return error if operator is not eligible",
//			icsOutput: icsOutput,
//			icsProof:  icsProof,
//			lk:        createTestLotteryKey(t, common.NilHash, icsSeed),
//			preTestFn: func(engine *Engine) {
//				// add an ics operator with high priority
//				engine.lottery.AddICSOperatorInfo(
//					createTestLotteryKey(t, common.NilHash, icsSeed),
//					tests.RandomKramaID(t, 0), 8)
//			},
//			params: &createKramaEngineParams{
//				vault: vault,
//				smCallback: func(sm *MockStateManager) {
//					updateTotalIncentives(t, sm, uint64(ExpectedSortionSize))
//					updateGuardianIncentive(t, sm, vault.KramaID(), 5)
//					sm.setPublicKey(vault.kramaID, vault.GetConsensusPrivateKey().GetPublicKeyInBytes())
//				},
//			},
//			expectedErr: ErrOperatorNotEligible,
//		},
//		{
//			name:      "invalid vrf proof",
//			icsOutput: icsOutput,
//			icsProof:  icsProof[4:],
//			lk:        createTestLotteryKey(t, common.NilHash, common.NilHash),
//			params: &createKramaEngineParams{
//				vault: vault,
//				smCallback: func(sm *MockStateManager) {
//					sm.setPublicKey(vault.kramaID, vault.GetConsensusPrivateKey().GetPublicKeyInBytes())
//				},
//			},
//			expectedErr: errors.New("invalid VRF proof"),
//		},
//	}
//
//	for _, test := range testcases {
//		t.Run(test.name, func(t *testing.T) {
//			engine := createTestKramaEngine(t, test.params)
//
//			if test.preTestFn != nil {
//				test.preTestFn(engine)
//			}
//
//			err := engine.verifyOperatorLottery(vault.kramaID, test.lk, test.icsOutput, test.icsProof)
//			if test.expectedErr != nil {
//				require.Error(t, err)
//				require.ErrorContains(t, test.expectedErr, err.Error())
//
//				return
//			}
//
//			checkForOperatorInfo(t, engine, test.lk, vault.kramaID, 5)
//			require.NoError(t, err)
//		})
//	}
//}

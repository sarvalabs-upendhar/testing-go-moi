package lattice

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
)

func TestHasTesseract(t *testing.T) {
	ts := createTesseract(t, nil)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts)
		},
	}
	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name         string
		hash         common.Hash
		hasTesseract bool
	}{
		{
			name:         "tesseract doesn't exist",
			hash:         tests.RandomHash(t),
			hasTesseract: false,
		},
		{
			name:         "tesseract exists",
			hash:         ts.Hash(),
			hasTesseract: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hasTesseract := c.hasTesseract(test.hash)
			require.Equal(t, test.hasTesseract, hasTesseract)
		})
	}
}

func TestFetchContextForAgora(t *testing.T) {
	// initializing address here as we need same address for multiple tesseracts in chain
	address := tests.RandomAddress(t)
	hash := tests.RandomHash(t)
	nodes := tests.RandomKramaIDs(t, 3)

	testcases := []struct {
		name          string
		tsCount       int
		paramsMap     map[int]*createTesseractParams
		smCallBack    func(sm *MockStateManager)
		shouldInsert  bool
		peerCount     int
		expectedError error
	}{
		{
			name:    "lattice has less than 10 context nodes",
			tsCount: 2,
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithContextDelta(t, address, 1, 3, 0),
				1: tesseractParamsWithContextDelta(t, address, 1, 1, 0),
			},
			shouldInsert: true,
			peerCount:    6,
		},
		{
			name:    "latest tesseracts context delta has more than 10 nodes",
			tsCount: 2,
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithContextDelta(t, address, 3, 5, 0),
				1: tesseractParamsWithContextDelta(t, address, 6, 5, 0),
			},
			shouldInsert: true,
			peerCount:    11,
		},
		{
			name:    "lattice has both context lock and context delta",
			tsCount: 2,
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithContextDelta(t, address, 3, 3, 0),
				1: {
					Addresses: []identifiers.Address{address},
					Participants: common.Participants{
						address: {
							ContextDelta:    *getDeltaGroup(t, 2, 2, 0),
							PreviousContext: hash,
						},
					},
				},
			},
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(hash, nodes[:2], nodes[2])
			},
			shouldInsert: true,
			peerCount:    7,
		},
		{
			name:    "previous tesseract doesn't exist",
			tsCount: 1,
			paramsMap: map[int]*createTesseractParams{
				0: {
					Addresses: []identifiers.Address{address},
					Participants: common.Participants{
						address: {
							TransitiveLink:  tests.RandomHash(t),
							ContextDelta:    *getDeltaGroup(t, 1, 3, 0),
							PreviousContext: hash,
						},
					},
				},
			},
			shouldInsert:  true,
			expectedError: errors.New("error fetching tesseract"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseractsWithChain(t, test.tsCount, test.paramsMap)

			chainParams := &CreateChainParams{
				smCallBack: test.smCallBack,
				chainManagerCallback: func(c *ChainManager) {
					if test.shouldInsert {
						insertTesseractsInCache(t, c, ts...)
					}
				},
			}

			c := createTestChainManager(t, chainParams)

			peers, err := c.fetchContextForAgora(ts[test.tsCount-1].AnyAddress(), *ts[test.tsCount-1])

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.peerCount, len(peers)) // check if peer count matches

			nodes := fetchContextFromLattice(t, ts[test.tsCount-1].AnyAddress(), *ts[test.tsCount-1], c)
			require.Equal(t, nodes, peers) // check if context nodes matches
		})
	}
}

func TestUpdateNodeInclusivity(t *testing.T) {
	senatus := mockSenatus(t) // initializing senatus here as we need to validate results by using this
	chainParams := &CreateChainParams{
		senatus: senatus,
	}
	c := createTestChainManager(t, chainParams)

	// create context delta
	deltaGroup := getDeltaGroup(t, 2, 2, 2)

	err := c.UpdateNodeInclusivity(*deltaGroup)
	require.NoError(t, err)

	validateDeltaGroup(t, senatus, *deltaGroup)
}

func TestGetTesseract(t *testing.T) {
	// set the interactions to nil for first tesseract
	ts := createTesseracts(t, 1, nil)
	tsWithInteractions := createTesseracts(t, 2, getTesseractParamsMapWithIxns(t, 3))
	ts = append(ts, tsWithInteractions...)

	chainParams := &CreateChainParams{
		chainManagerCallback: func(c *ChainManager) {
			insertTesseractsInCache(t, c, ts[0])
		},
		smCallBack: func(sm *MockStateManager) {
			sm.InsertTesseractsInDB(t, ts[1:]...)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name             string
		address          identifiers.Address
		tsHash           common.Hash
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:             "should return error if tesseract doesn't exist",
			tsHash:           tests.RandomHash(t),
			withInteractions: false,
			expectedError:    common.ErrFetchingTesseract,
		},
		{
			name:             "fetch tesseract without interactions from cache",
			tsHash:           ts[0].Hash(),
			withInteractions: false,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch tesseract without interactions from db",
			tsHash:           ts[1].Hash(),
			withInteractions: false,
			expectedTS:       ts[1],
		},
		{
			name:             "fetch tesseract with interactions from db",
			tsHash:           ts[2].Hash(),
			withInteractions: true,
			expectedTS:       ts[2],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualTS, err := c.GetTesseract(test.tsHash, test.withInteractions)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			tests.CheckForTesseract(t, test.expectedTS, actualTS, test.withInteractions)

			checkIfTesseractCachedInCM(t, c, test.withInteractions, test.tsHash)
		})
	}
}

func TestGetTesseractByHeight(t *testing.T) {
	type args struct {
		address          identifiers.Address
		height           uint64
		withInteractions bool
	}

	ts := createTesseract(t, getTesseractParamsMapWithIxns(t, 1)[0])

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertTesseractByHeight(t, db, ts)
		},
		smCallBack: func(sm *MockStateManager) {
			sm.InsertTesseractsInDB(t, ts)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		args          args
		expectedTS    *common.Tesseract
		expectedError error
	}{
		{
			name: "fetch tesseract with Interactions for a valid height",
			args: args{
				address:          ts.AnyAddress(),
				height:           ts.Height(ts.AnyAddress()),
				withInteractions: true,
			},
			expectedTS: ts,
		},
		{
			name: "fetch tesseract without Interactions for a valid height",
			args: args{
				address:          ts.AnyAddress(),
				height:           ts.Height(ts.AnyAddress()),
				withInteractions: false,
			},
			expectedTS: ts,
		},
		{
			name: "should return error for invalid height",
			args: args{
				address:          ts.AnyAddress(),
				height:           1,
				withInteractions: true,
			},
			expectedError: errors.New("failed to fetch tesseract height entry"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualTS, err := c.GetTesseractByHeight(test.args.address, test.args.height, test.args.withInteractions)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.CheckForTesseract(t, test.expectedTS, actualTS, test.args.withInteractions)
			checkIfTesseractCachedInCM(t, c, test.args.withInteractions, ts.Hash())
		})
	}
}

func TestGetTesseractHashByHeight(t *testing.T) {
	address := tests.RandomAddress(t)
	height := uint64(33)
	hash := tests.RandomHash(t)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			err := db.SetTesseractHeightEntry(address, height, hash)
			require.NoError(t, err)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		height        uint64
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name:         "successfully fetched tesseract hash",
			address:      address,
			height:       height,
			expectedHash: hash,
		},
		{
			name:          "failed to fetch tesseract hash",
			address:       tests.RandomAddress(t),
			height:        height,
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := c.GetTesseractHeightEntry(test.address, test.height)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedHash, hash)
		})
	}
}

func TestGetLatestTesseract(t *testing.T) {
	ts := createTesseract(t, getTesseractParamsMapWithIxns(t, 1)[0])

	chainParams := &CreateChainParams{
		smCallBack: func(sm *MockStateManager) {
			sm.InsertLatestTesseracts(t, ts)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name             string
		address          identifiers.Address
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:          "should return error for nil address",
			address:       identifiers.NilAddress,
			expectedError: common.ErrInvalidAddress,
		},
		{
			name:          "should return error for invalid address",
			address:       tests.RandomAddress(t),
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name:             "fetch tesseract without interactions",
			address:          ts.AnyAddress(),
			withInteractions: false,
			expectedTS:       ts,
		},
		{
			name:             "fetch tesseract with interactions",
			address:          ts.AnyAddress(),
			withInteractions: true,
			expectedTS:       ts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualTS, err := c.GetLatestTesseract(test.address, test.withInteractions)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.CheckForTesseract(t, test.expectedTS, actualTS, test.withInteractions)
		})
	}
}

func TestGetReceipt(t *testing.T) {
	ixs, receipts := getIxAndReceipts(t, 2) // get interactions and receipts

	tsHash := tests.RandomHash(t)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertReceipts(t, db, tsHash, receipts)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		tsHash        common.Hash
		ixHash        common.Hash
		expectedError error
	}{
		{
			name:   "receipt exists",
			tsHash: tsHash,
			ixHash: ixs[1].Hash(),
		},
		{
			name:          "should return error if receipt root hash is invalid",
			tsHash:        tests.RandomHash(t),
			ixHash:        ixs[1].Hash(),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:          "should return error if ixHash doesn't exist",
			tsHash:        tsHash,
			ixHash:        tests.RandomHash(t),
			expectedError: common.ErrReceiptNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualReceipts, err := c.getReceipt(test.ixHash, test.tsHash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, receipts[test.ixHash], actualReceipts)
		})
	}
}

func TestGetReceiptByIxHash(t *testing.T) {
	ixns, receipts := getIxAndReceipts(t, 2)
	tsHash := tests.RandomHash(t)
	unknownTSHash := tests.RandomHash(t)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertReceipts(t, db, tsHash, receipts)

			err := db.SetIXLookup(ixns[0].Hash(), tsHash)
			require.NoError(t, err)

			err = db.SetIXLookup(ixns[1].Hash(), unknownTSHash)
			require.NoError(t, err)

			receiptData, err := receipts.Bytes()
			require.NoError(t, err)

			err = db.SetReceipts(tsHash, receiptData)
			require.NoError(t, err)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		ixHash        common.Hash
		receipts      common.Receipts
		expectedError error
	}{
		{
			name:          "tesseract hash not found",
			ixHash:        tests.RandomHash(t),
			expectedError: common.ErrTSHashNotFound,
		},
		{
			name:          "receipt not found",
			ixHash:        ixns[1].Hash(),
			expectedError: common.ErrReceiptNotFound,
		},
		{
			name:     "fetch receipt successfully",
			ixHash:   ixns[0].Hash(),
			receipts: receipts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualReceipt, err := c.GetReceiptByIxHash(test.ixHash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.receipts[test.ixHash], actualReceipt)
		})
	}
}

func TestVerifySignatures(t *testing.T) {
	getTesseractParamsWithVoteset := func(index int, value bool) *createTesseractParams {
		return &createTesseractParams{
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.ConsensusInfo.BFTVoteSet.SetIndex(index, value)
			},
		}
	}

	tesseractParams := map[int]*createTesseractParams{
		0: getTesseractParamsWithVoteset(0, false),
		1: getTesseractParamsWithVoteset(2, false),
		2: getTesseractParamsWithVoteset(4, false),
		3: tesseractParamsWithCommitSign(invalidCommitSign),
		4: tesseractParamsWithCommitSign(validCommitSign), // MockVerifiers recognizes validCommitSign
	}

	ts := createTesseracts(t, 5, tesseractParams)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts...)
		},
	}

	c := createTestChainManager(t, chainParams)

	nodes := getICSNodeset(t, 1)

	testcases := []struct {
		name          string
		ts            *common.Tesseract
		expectedError error
	}{
		{
			name:          "should return error for invalid sender quorum size",
			ts:            ts[0],
			expectedError: common.ErrQuorumFailed,
		},
		{
			name:          "should return error for invalid receiver quorum size",
			ts:            ts[1],
			expectedError: common.ErrQuorumFailed,
		},
		{
			name:          "should return error for invalid random quorum size",
			ts:            ts[2],
			expectedError: common.ErrQuorumFailed,
		},
		{
			name:          "should return error for invalid commit signature",
			ts:            ts[3],
			expectedError: common.ErrSignatureVerificationFailed,
		},
		{
			name: "valid commit signature",
			ts:   ts[4],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			val, err := c.verifySignatures(test.ts, nodes)

			if test.expectedError != nil {
				require.False(t, val)
				require.EqualError(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			require.True(t, val)
		})
	}
}

func TestVerifyParticipantState(t *testing.T) {
	address := tests.RandomAddress(t)
	sargaTesseractHash := tests.RandomHash(t)

	getTesseractParams := func(clusterID string, height uint64) *createTesseractParams {
		return &createTesseractParams{
			Addresses: []identifiers.Address{address},
			Participants: common.Participants{
				address: {
					Height: height,
				},
			},
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.ConsensusInfo.ClusterID = common.ClusterID(clusterID)
			},
		}
	}

	getTesseractParamsWithTimeStamp := func(height uint64, timestamp uint64) *createTesseractParams {
		return &createTesseractParams{
			Addresses: []identifiers.Address{address},
			Participants: common.Participants{
				address: {
					Height: height,
				},
			},
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.ConsensusInfo.ClusterID = "non-genesis"
				ts.Timestamp = timestamp
			},
		}
	}

	smCallback := func(sm *MockStateManager) {
		sm.insertRegisteredAcc(address)
	}

	testcases := []struct {
		name          string
		paramsMap     map[int]*createTesseractParams
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "valid tesseract of non-registered account",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("non-genesis", 0),
			},
		},
		{
			name: "valid tesseract of registered account",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("non-genesis", 0),
				1: getTesseractParams("non-genesis", 1),
			},
			smCallBack: smCallback, // register the account using sm call back
		},
		{
			name: "should skip for genesis tesseract",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("genesis", 0),
			},
		},
		{
			name: "should return error if previous tesseract not found",
			paramsMap: map[int]*createTesseractParams{
				0: {
					Addresses: []identifiers.Address{address},
					Participants: common.Participants{
						address: {
							TransitiveLink: tests.RandomHash(t),
						},
					},
					TSDataCallback: func(ts *tests.TesseractData) {
						ts.ConsensusInfo.ClusterID = "non-genesis"
					},
				},
			},
			smCallBack:    smCallback,
			expectedError: errors.New("error fetching previous tesseract"),
		},
		{
			name: "should return error for invalid height",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("non-genesis", 0),
				1: getTesseractParams("non-genesis", 2),
			},
			smCallBack:    smCallback,
			expectedError: common.ErrInvalidHeight,
		},
		{
			name: "should return error for invalid time stamp",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParamsWithTimeStamp(0, 3),
				1: getTesseractParamsWithTimeStamp(1, 2),
			},
			smCallBack:    smCallback,
			expectedError: common.ErrInvalidBlockTime,
		},
		{
			name: "should return error if sarga account not found",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("non-genesis", 0),
			},
			smCallBack:    smCallbackWithAccRegistrationHook(),
			expectedError: errors.New("Sarga account not found"),
		},
		{
			name: "should return error if context lock contains  invalid tesseract hash of sarga account",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("non-genesis", 0),
			},
			smCallBack: func(sm *MockStateManager) {
				sm.accountRegistrationHook = func(tsHash common.Hash) (bool, error) {
					if tsHash != sargaTesseractHash {
						return false, errors.New("invalid hash")
					}

					return false, nil
				}
			},
			expectedError: errors.New("invalid hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseractsWithChain(t, len(test.paramsMap), test.paramsMap)

			chainParams := &CreateChainParams{
				smCallBack: test.smCallBack,
				chainManagerCallback: func(c *ChainManager) {
					insertTesseractsInCache(t, c, ts...)
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.verifyTransitions(ts[len(ts)-1].AnyAddress(), ts[len(ts)-1], false)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestValidateTesseract(t *testing.T) {
	nodes := getICSNodeset(t, 1)
	addresses := tests.GetAddresses(t, 2)

	tsParamsMap := map[int]*createTesseractParams{
		0: tesseractParamsWithCommitSign(validCommitSign),
		1: {
			Addresses: addresses,
			Participants: common.Participants{
				addresses[1]: {
					TransitiveLink: tests.RandomHash(t),
				},
			},
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.ConsensusInfo.CommitSignature = validCommitSign
			},
		},
		2: tesseractParamsWithCommitSign(invalidCommitSign),
		3: tesseractParamsWithCommitSign(validCommitSign),
		4: {
			Addresses: []identifiers.Address{addresses[0]},
			Participants: common.Participants{
				addresses[0]: {
					TransitiveLink: tests.RandomHash(t),
				},
			},
		},
		5: tesseractParamsWithCommitSign(validCommitSign),
		6: tesseractParamsWithCommitSign(validCommitSign),
	}

	ts := createTesseracts(t, 7, tsParamsMap)

	testcases := []struct {
		name            string
		ts              *common.Tesseract
		allParticipants bool
		smCallback      func(sm *MockStateManager)
		dbCallback      func(db *MockDB)
		expectedError   error
	}{
		{
			name: "valid tesseract",
			ts:   ts[0],
		},
		{
			name:            "check if all participants are verified",
			ts:              ts[1],
			allParticipants: true,
			smCallback:      smCallbackWithRegisteredAcc(addresses[1]),
			expectedError:   common.ErrPreviousTesseractNotFound,
		},
		{
			name:          "should return error if ics signature is invalid",
			ts:            ts[2],
			expectedError: errors.New("failed to verify signatures"),
		},
		{
			name:          "should return error if sarga account not state found",
			ts:            ts[3],
			smCallback:    smCallbackWithAccRegistrationHook(),
			expectedError: errors.New("Sarga account not found"),
		},
		{
			name:          "should be added to orphans if previous tesseract not found",
			ts:            ts[4],
			smCallback:    smCallbackWithRegisteredAcc(addresses[0]),
			expectedError: common.ErrPreviousTesseractNotFound,
		},
		{
			name: "should return error if tesseract already exits",
			ts:   ts[5],
			dbCallback: func(db *MockDB) {
				insertAccMetaInfo(t, db, ts[5])
			},
			expectedError: common.ErrAlreadyKnown,
		},
		{
			name:          "invalid tesseract seal",
			ts:            ts[6],
			expectedError: common.ErrInvalidSeal,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := mockStateManager()
			if !errors.Is(test.expectedError, common.ErrInvalidSeal) {
				signTesseract(t, sm, test.ts)
			}

			chainParams := &CreateChainParams{
				sm:         sm,
				smCallBack: test.smCallback,
				dbCallback: test.dbCallback,
			}

			c := createTestChainManager(t, chainParams)

			err := c.ValidateTesseract(test.ts.AnyAddress(), test.ts, nodes, test.allParticipants)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAddParticipantData(t *testing.T) {
	addresses := tests.GetAddresses(t, 2)

	testcases := []struct {
		name            string
		address         identifiers.Address
		tsParams        *createTesseractParams
		allParticipants bool
		smCallback      func(sm *MockStateManager)
		dbCallback      func(db *MockDB)
		expectedError   error
	}{
		{
			name:            "address is not specified when a specific participant need to be added",
			address:         identifiers.NilAddress,
			allParticipants: false,
			expectedError:   errors.New("address is not specified"),
		},
		{
			name:    "added only one participant data successfully",
			address: addresses[0],
			tsParams: &createTesseractParams{
				Addresses: addresses,
				Participants: common.Participants{
					addresses[0]: {
						Height: 11,
					},
					addresses[1]: {
						Height: 23,
					},
				},
			},
			allParticipants: false,
		},
		{
			name: "added all participant data successfully",
			tsParams: &createTesseractParams{
				Addresses: addresses,
				Participants: common.Participants{
					addresses[0]: {
						Height: 11,
					},
					addresses[1]: {
						Height: 23,
					},
				},
			},
			allParticipants: true,
		},
		{
			name: "failed to flush dirty object",
			smCallback: func(sm *MockStateManager) {
				sm.flushHook = func() error {
					return errors.New("failed to flush dirty entries")
				}
			},
			allParticipants: true,
			expectedError:   errors.New("failed to flush dirty entries"),
		},
		{
			name: "failed to set tesseract height entry",
			dbCallback: func(db *MockDB) {
				db.setTesseractHeightEntryHook = func() error {
					return errors.New("failed to set tesseract height entry")
				}
			},
			allParticipants: true,
			expectedError:   errors.New("failed to set tesseract height entry"),
		},
		{
			name: "failed to fetch acc type",
			smCallback: func(sm *MockStateManager) {
				sm.GetAccTypeHook = func() error {
					return errors.New("failed to fetch acc type")
				}
			},
			allParticipants: true,
			expectedError:   errors.New("failed to fetch acc type"),
		},
		{
			name: "failed to update account meta info",
			dbCallback: func(db *MockDB) {
				db.updateMetaInfoHook = func() (int32, bool, error) {
					return 0, false, errors.New("failed to update acc meta info")
				}
			},
			allParticipants: true,
			expectedError:   errors.New("failed to update acc meta info"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.tsParams)
			db := mockDB()
			sm := mockStateManager()

			for _, addr := range ts.Addresses() {
				sm.accountTypes[addr] = common.RegularAccount
			}

			chainParams := &CreateChainParams{
				db:         db,
				sm:         sm,
				smCallBack: test.smCallback,
				dbCallback: test.dbCallback,
			}

			c := createTestChainManager(t, chainParams)

			err := c.addParticipantsData(
				true,
				test.address,
				ts,
				test.allParticipants,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			participants := make(common.Participants)

			if test.allParticipants {
				participants = ts.Participants()
			} else {
				s, ok := ts.State(test.address)
				if !ok {
					panic(ok)
				}

				participants[test.address] = s
			}

			for addr, participant := range participants {
				require.True(t, sm.isCleanup(addr)) // check if dirty objects cleaned up

				found := sm.getFlushedDirtyObject(addr)
				require.True(t, found)

				checkForParticipantByHeight(t, c, addr, participant, true)

				// check if acc meta info inserted
				accMetaInfo, found := db.GetAccMetaInfo(addr)
				require.True(t, found)

				checkIfAccMetaInfoMatches(
					t,
					accMetaInfo,
					addr,
					participant.Height,
					ts.Hash(),
					common.RegularAccount,
				)

				hash, ok := c.tesseracts.Get(addr)
				require.True(t, ok)
				require.Equal(t, ts.Hash(), hash)
			}

			// make sure other accounts are not stored in this case
			if !test.allParticipants {
				for addr, p := range ts.Participants() {
					if addr != test.address {
						checkForParticipantByHeight(t, c, addr, p, false)
					}
				}
			}
		})
	}
}

func TestIsReceiptHashValid(t *testing.T) {
	var receipts common.Receipts

	receiptsHash, err := receipts.Hash()
	require.NoError(t, err)

	testcases := []struct {
		name      string
		paramsMap *createTesseractParams
		isValid   bool
	}{
		{
			name: "valid receipt hash",
			paramsMap: &createTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ReceiptsHash = receiptsHash
				},
			},
			isValid: true,
		},
		{
			name: "invalid receipt hash",
			paramsMap: &createTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ReceiptsHash = tests.RandomHash(t)
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.paramsMap)

			isValid := isReceiptsHashValid(ts, receipts)
			require.Equal(t, test.isValid, isValid)
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
		params      *createTesseractParams
		stateHashes common.AccStateHashes
		isValid     bool
	}{
		{
			name: "state hash in receipt and tesseract doesn't match",
			params: &createTesseractParams{
				Participants: common.Participants{
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
			params: &createTesseractParams{
				Addresses: []identifiers.Address{addresses[0], addresses[1]},
				Participants: common.Participants{
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
			ts := createTesseract(t, test.params)

			isValid := areStateHashesValid(ts, test.stateHashes)

			require.Equal(t, test.isValid, isValid)
		})
	}
}

func TestStoreReceipts(t *testing.T) {
	c := createTestChainManager(t, nil)
	_, receipts := getIxAndReceipts(t, 1)

	testcases := []struct {
		name               string
		batchWriterSetHook func() error
		params             *createTesseractParams
		expectedError      error
	}{
		{
			name: "receipts stored successfully",
			params: &createTesseractParams{
				Receipts: receipts,
			},
		},
		{
			name: "failed to store receipts",
			batchWriterSetHook: func() error {
				return errors.New("failed to write receipts")
			},
			params: &createTesseractParams{
				Receipts: receipts,
			},
			expectedError: errors.New("failed to write receipts"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.params)

			bw := newMockBatchWriter()
			bw.setHook = test.batchWriterSetHook

			err := c.storeReceipts(bw, ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if receipts are stored in batch writer
			val, found := bw.Get(storage.ReceiptsKey(ts.Hash()))
			require.True(t, found)

			receipts := new(common.Receipts)

			err = receipts.FromBytes(val)
			require.NoError(t, err)

			require.Equal(t, ts.Receipts(), *receipts)
		})
	}
}

func TestStoreInteractions(t *testing.T) {
	ixns := tests.CreateIxns(t, 2, nil)
	c := createTestChainManager(t, nil)

	testcases := []struct {
		name               string
		batchWriterSetHook func() error
		params             *createTesseractParams
		expectedError      error
	}{
		{
			name: "receipts stored successfully",
			params: &createTesseractParams{
				Ixns: ixns,
			},
		},
		{
			name: "failed to store receipts",
			batchWriterSetHook: func() error {
				return errors.New("failed to write ixns")
			},
			params: &createTesseractParams{
				Ixns: ixns,
			},
			expectedError: errors.New("failed to write ixns"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.params)

			bw := newMockBatchWriter()
			bw.setHook = test.batchWriterSetHook

			err := c.storeInteractions(bw, ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if ixns are stored in batch writer
			val, found := bw.Get(storage.InteractionsKey(ts.Hash()))
			require.True(t, found)

			actualIxns := new(common.Interactions)

			err = actualIxns.FromBytes(val)
			require.NoError(t, err)

			require.Equal(t, ts.Interactions(), *actualIxns)

			// check if ixnHash ==> tsHash pair stored in batch writer
			for _, ixn := range ts.Interactions() {
				val, found := bw.Get(ixn.Hash().Bytes())
				require.True(t, found)

				tsHash := common.BytesToHash(val)
				require.Equal(t, ts.Hash(), tsHash)
			}
		})
	}
}

func TestAddTesseractData(t *testing.T) {
	ixns, receipts := getIxAndReceipts(t, 1)
	c := createTestChainManager(t, nil)

	testcases := []struct {
		name               string
		batchWriterSetHook func() error
		params             *createTesseractParams
		expectedError      error
	}{
		{
			name: "tesseract added successfully",
			params: &createTesseractParams{
				Ixns:     ixns,
				Receipts: receipts,
			},
		},
		{
			name:   "failed to store tesseract",
			params: &createTesseractParams{},
			batchWriterSetHook: func() error {
				return errors.New("failed to store tesseract")
			},
			expectedError: errors.New("failed to store tesseract"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.params)

			bw := newMockBatchWriter()
			bw.setHook = test.batchWriterSetHook

			err := c.addTesseractData(bw, ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if ixns are stored in batch writer
			val, found := bw.Get(storage.TesseractKey(ts.Hash()))
			require.True(t, found)

			actualTS := new(common.CanonicalTesseract)

			err = actualTS.FromBytes(val)
			require.NoError(t, err)

			require.Equal(t, ts.Canonical(), actualTS)

			// check if ixns are stored in batch writer
			_, found = bw.Get(storage.InteractionsKey(ts.Hash()))
			require.True(t, found)

			// check if receipts are stored in batch writer
			_, found = bw.Get(storage.ReceiptsKey(ts.Hash()))
			require.True(t, found)
		})
	}
}

func TestAddTesseract(t *testing.T) {
	ixns, receipts := getIxAndReceipts(t, 1)
	addresses := tests.GetAddresses(t, 2)

	testcases := []struct {
		name             string
		tsParams         *createTesseractParams
		shouldCache      bool
		smCallback       func(sm *MockStateManager)
		dbCallback       func(db *MockDB)
		dbCallbackWithTS func(db *MockDB, ts *common.Tesseract)
		senatusCallback  func(senatus *MockSenatus)
		expectedError    error
	}{
		{
			name: "added tesseract successfully with caching",
			tsParams: &createTesseractParams{
				Addresses: addresses,
				Participants: common.Participants{
					addresses[0]: {
						Height:       11,
						ContextDelta: *getDeltaGroup(t, 1, 1, 1),
					},
					addresses[1]: {
						Height:       23,
						ContextDelta: *getDeltaGroup(t, 1, 1, 1),
					},
				},
				Ixns:     ixns,
				Receipts: receipts,
			},
			shouldCache: true,
		},
		{
			name: "added tesseract successfully without caching",
			tsParams: &createTesseractParams{
				Addresses: addresses,
				Participants: common.Participants{
					addresses[0]: {
						Height:       11,
						ContextDelta: *getDeltaGroup(t, 1, 1, 1),
					},
					addresses[1]: {
						Height:       23,
						ContextDelta: *getDeltaGroup(t, 1, 1, 1),
					},
				},
				Ixns:     ixns,
				Receipts: receipts,
			},
			shouldCache: false,
		},
		{
			name: "added only participant data as tesseract already exists in db",
			dbCallbackWithTS: func(db *MockDB, ts *common.Tesseract) {
				data, _ := ts.Bytes()
				err := db.SetTesseract(ts.Hash(), data)
				require.NoError(t, err)
			},
		},
		{
			name: "failed to add participant data",
			smCallback: func(sm *MockStateManager) {
				sm.flushHook = func() error {
					return errors.New("failed to flush dirty entries")
				}
			},
			expectedError: errors.New("failed to flush dirty entries"),
		},
		{
			name: "failed to add tesseract data",
			dbCallback: func(db *MockDB) {
				db.batchWriterSetHook = func() error {
					return errors.New("failed to store tesseract")
				}
			},
			expectedError: errors.New("failed to store tesseract"),
		},
		{
			name: "failed to flush tesseract data",
			dbCallback: func(db *MockDB) {
				db.batchWriterFlushHook = func() error {
					return errors.New("failed to flush tesseract data")
				}
			},
			expectedError: errors.New("failed to flush tesseract data"),
		},
		{
			name: "failed to update node inclusivity",
			tsParams: &createTesseractParams{
				Addresses: addresses[0:1],
				Participants: common.Participants{
					addresses[0]: {
						ContextDelta: *getDeltaGroup(t, 1, 1, 1),
					},
				},
			},
			senatusCallback: func(senatus *MockSenatus) {
				senatus.UpdateWalletCountHook = func() error {
					return errors.New("failed to update node inclusivity")
				}
			},
			expectedError: errors.New("failed to update node inclusivity"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.tsParams)
			db := mockDB()
			sm := mockStateManager()
			senatus := mockSenatus(t)
			ixpool := mockIXPool(t)

			if test.dbCallbackWithTS != nil {
				test.dbCallbackWithTS(db, ts)
			}

			for _, addr := range ts.Addresses() {
				sm.accountTypes[addr] = common.RegularAccount
			}

			chainParams := &CreateChainParams{
				db:              db,
				sm:              sm,
				senatus:         senatus,
				ixPool:          ixpool,
				smCallBack:      test.smCallback,
				dbCallback:      test.dbCallback,
				senatusCallback: test.senatusCallback,
			}

			c := createTestChainManager(t, chainParams)

			participants := ts.Participants()

			tsAddedEventSub := c.mux.Subscribe(utils.TesseractAddedEvent{}) // subscribe to tesseract added event

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tsAddedResp := make(chan tests.Result, 1)

			// keeps checking for event until timeout
			go utils.HandleMuxEvents(ctx, tsAddedEventSub, tsAddedResp, 1)

			err := c.addTesseract(
				test.shouldCache,
				identifiers.NilAddress,
				ts,
				true,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if participant data added
			for addr, participant := range participants {
				checkForParticipantByHeight(t, c, addr, participant, true)
			}

			if c.db.HasTesseract(ts.Hash()) {
				require.Nil(t, db.batchWriter)

				return
			}

			checkForTesseractData(t, ts, c, db, senatus, ixpool, tsAddedResp, test.shouldCache)
		})
	}
}

func TestAddTesseractWithState(t *testing.T) {
	addresses := tests.GetAddresses(t, 2)
	icsHash := tests.RandomHash(t)

	dirtyStorage := map[common.Hash][]byte{
		icsHash:             {1, 2, 3},
		tests.RandomHash(t): {4, 5, 6},
	}

	tsParams := &createTesseractParams{
		Addresses: addresses,
		Participants: common.Participants{
			addresses[0]: {
				Height:       11,
				ContextDelta: *getDeltaGroup(t, 1, 1, 1),
			},
			addresses[1]: {
				Height:       23,
				ContextDelta: *getDeltaGroup(t, 1, 1, 1),
			},
		},
		TSDataCallback: func(ts *tests.TesseractData) {
			ts.ConsensusInfo.ICSHash = icsHash
		},
	}

	testcases := []struct {
		name            string
		address         identifiers.Address
		tsParams        *createTesseractParams
		dirtyStorage    map[common.Hash][]byte
		allParticipants bool
		smCallback      func(sm *MockStateManager)
		dbCallback      func(db *MockDB)
		expectedError   error
	}{
		{
			name:            "added tesseract with state successfully",
			address:         identifiers.NilAddress,
			tsParams:        tsParams,
			dirtyStorage:    dirtyStorage,
			allParticipants: true,
		},
		{
			name:            "added tesseract's participant successfully",
			address:         addresses[0],
			tsParams:        tsParams,
			dirtyStorage:    dirtyStorage,
			allParticipants: false,
		},
		{
			name: "failed to add tesseract",
			tsParams: &createTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ConsensusInfo.ICSHash = icsHash
				},
			},
			smCallback: func(sm *MockStateManager) {
				sm.flushHook = func() error {
					return errors.New("failed to flush dirty entries")
				}
			},
			dirtyStorage:    dirtyStorage,
			allParticipants: true,
			expectedError:   errors.New("failed to flush dirty entries"),
		},
		{
			name:            "empty dirty storage",
			allParticipants: true,
			expectedError:   errors.New("empty dirty storage"),
		},
		{
			name: "failed to write dirty keys",
			tsParams: &createTesseractParams{
				TSDataCallback: func(ts *tests.TesseractData) {
					ts.ConsensusInfo.ICSHash = icsHash
				},
			},
			dbCallback: func(db *MockDB) {
				db.createEntryHook = func() error {
					return errors.New("failed to write dirty keys")
				}
			},
			dirtyStorage:    dirtyStorage,
			allParticipants: true,
			expectedError:   errors.New("failed to write dirty keys"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.tsParams)
			sm := mockStateManager()

			for _, addr := range ts.Addresses() {
				sm.accountTypes[addr] = common.RegularAccount
			}

			chainParams := &CreateChainParams{
				sm:         sm,
				smCallBack: test.smCallback,
				dbCallback: test.dbCallback,
			}

			c := createTestChainManager(t, chainParams)

			participants := ts.Participants()

			err := c.AddTesseractWithState(
				test.address,
				test.dirtyStorage,
				ts,
				test.allParticipants,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			var expectedAddedAccounts []identifiers.Address

			if test.allParticipants {
				expectedAddedAccounts = ts.Addresses()
			} else {
				expectedAddedAccounts = []identifiers.Address{test.address}
			}

			// check if participants data added
			for _, addr := range expectedAddedAccounts {
				checkForParticipantByHeight(t, c, addr, participants[addr], true)
			}

			// make sure other accounts are not stored in this case
			if !test.allParticipants {
				for _, expectedAddr := range expectedAddedAccounts {
					for addr, p := range participants {
						if addr != expectedAddr {
							checkForParticipantByHeight(t, c, addr, p, false)
						}
					}
				}
			}

			// check if dirty entries are stored in db
			for k, v := range dirtyStorage {
				value, err := c.db.ReadEntry(k.Bytes())
				require.NoError(t, err)
				require.Equal(t, v, value)
			}
		})
	}
}

func TestExecuteAndValidate(t *testing.T) {
	var (
		address        = tests.RandomAddress(t)
		stateHash      = tests.RandomHash(t)
		_, receipts    = getIxAndReceipts(t, 1)
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
		tsParams                *createTesseractParams
		revertHook              func() error
		executeInteractionsHook func() (common.Receipts, common.AccStateHashes, error)
		expectedError           error
	}{
		{
			name: "should return error if execution fails",
			tsParams: &createTesseractParams{
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
			tsParams: &createTesseractParams{
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
			tsParams: &createTesseractParams{
				Addresses: []identifiers.Address{address},
				Participants: common.Participants{
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
			tsParams: &createTesseractParams{
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
			name: "receipts should be added to dirty storage",
			tsParams: &createTesseractParams{
				Participants: common.Participants{
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
			ts := createTesseract(t, test.tsParams)

			chainParams := &CreateChainParams{
				execCallback: func(exec *MockExec) {
					exec.executeInteractionsHook = test.executeInteractionsHook
					exec.revertHook = test.revertHook
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.ExecuteAndValidate(ts)

			checkForExecutionCleanup(t, c, "cluster-0")

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, receipts, ts.Receipts())
		})
	}
}

func TestValidateAccountCreationInfo(t *testing.T) {
	var (
		c             = createTestChainManager(t, nil)
		walletContext = tests.RandomKramaIDs(t, 4)
		address       = tests.RandomAddress(t)
	)

	testcases := []struct {
		name          string
		accountInfo   *common.AccountSetupArgs
		expectedError error
	}{
		{
			name: "should fail for invalid account type",
			accountInfo: getAccountSetupArgs(
				t,
				address,
				tests.InvalidAccount,
				"moi-id",
				walletContext[:2],
				walletContext[2:4],
			),
			expectedError: common.ErrInvalidAccountType,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := c.validateAccountCreationInfo(*test.accountInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAddGenesisTesseract(t *testing.T) {
	var (
		addresses     = tests.GetAddresses(t, 2)
		stateHashes   = tests.GetHashes(t, 2)
		contextHashes = tests.GetHashes(t, 2)
		accType       = common.LogicAccount
	)

	testcases := []struct {
		name          string
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "adding genesis tesseract successful",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(addresses[0], accType)
			},
		},
		{
			name: "failed to add genesis tesseract",
			smCallBack: func(sm *MockStateManager) {
				sm.flushHook = func() error {
					return errors.New("failed to flush dirty entries")
				}
			},
			expectedError: errors.New("error adding genesis tesseract"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := mockStateManager()

			for _, addr := range addresses {
				sm.accountTypes[addr] = accType
			}

			chainParams := &CreateChainParams{
				sm:         sm,
				smCallBack: test.smCallBack,
			}

			c := createTestChainManager(t, chainParams)

			err := c.AddGenesisTesseract(addresses, stateHashes, contextHashes, uint64(time.Now().Unix()))
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for _, addr := range addresses {
				found := sm.getFlushedDirtyObject(addr)
				require.True(t, found)
			}
		},
		)
	}
}

func TestParseGenesisFile_InvalidPath(t *testing.T) {
	c := createTestChainManager(t, nil)
	_, _, _, _, err := c.ParseGenesisFile("test2/3647663731config.json") //nolint

	require.ErrorContains(t, err, "failed to open genesis file")
}

func TestParseGenesisFile(t *testing.T) {
	c := createTestChainManager(t, nil)
	addr := tests.RandomAddress(t)
	nodes := tests.RandomKramaIDs(t, 4)
	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	testcases := []struct {
		name                 string
		path                 string
		sargaAccount         common.AccountSetupArgs
		genesisAccounts      []common.AccountSetupArgs
		genesisLogics        []common.LogicSetupArgs
		genesisAssetAccounts []common.AssetAccountSetupArgs
		invalidAccPayload    bool
		expectedError        error
	}{
		{
			name:              "should return error for invalid genesis payload",
			genesisAccounts:   nil,
			invalidAccPayload: true,
			expectedError:     errors.New("failed to parse genesis file"),
		},
		{
			name:            "should return error if sarga account info is invalid",
			sargaAccount:    getTestAccountWithAccType(t, tests.InvalidAccount),
			genesisAccounts: []common.AccountSetupArgs{},
			expectedError:   errors.New("invalid sarga account info"),
		},
		{
			name:         "should return error if genesis account info is invalid",
			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
			genesisAccounts: []common.AccountSetupArgs{
				getTestAccountWithAccType(t, tests.InvalidAccount),
			},
			expectedError: errors.New("invalid genesis account creation info"),
		},
		{
			name:         "should succeed for valid genesis data without contract paths",
			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
			genesisAccounts: []common.AccountSetupArgs{
				getTestAccountWithAccType(t, common.RegularAccount),
				getTestAccountWithAccType(t, common.LogicAccount),
			},
		},
		{
			name:         "should succeed for valid genesis data with contract paths",
			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
			genesisAccounts: []common.AccountSetupArgs{
				getTestAccountWithAccType(t, common.RegularAccount),
				getTestAccountWithAccType(t, common.LogicAccount),
			},
			genesisLogics: getTestGenesisLogics(t),
		},
		{
			name:         "should succeed for valid genesis asset account with invalid allocations",
			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
			genesisAccounts: []common.AccountSetupArgs{
				getTestAccountWithAccType(t, common.RegularAccount),
				getTestAccountWithAccType(t, common.LogicAccount),
			},
			genesisAssetAccounts: []common.AssetAccountSetupArgs{
				// using nilAddress for allocations
				getAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, identifiers.NilAddress), nodes[:2], nodes[2:]),
			},
		},
		{
			name:         "should succeed for valid genesis asset account with valid allocations",
			sargaAccount: getTestAccountWithAccType(t, common.SargaAccount),
			genesisAccounts: []common.AccountSetupArgs{
				getTestAccountWithAddress(t, addr),
				getTestAccountWithAccType(t, common.LogicAccount),
			},
			genesisAssetAccounts: []common.AssetAccountSetupArgs{
				getAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, addr), nodes[:2], nodes[2:]),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			path := createMockGenesisFile(t, dir, test.invalidAccPayload, test.sargaAccount,
				test.genesisAccounts, test.genesisAssetAccounts, test.genesisLogics)

			sargaAccount, genesisAccounts, assetAccounts, genesisLogics, err := c.ParseGenesisFile(path)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}
			require.NoError(t, err)

			// check for sarga account
			require.Equal(t, test.sargaAccount, *sargaAccount)

			// check for genesis accounts
			for i, genesisAccount := range test.genesisAccounts {
				require.Equal(t, genesisAccount, genesisAccounts[i])
			}

			checkForLogicAccounts(t, test.genesisLogics, genesisLogics)

			checkForAssetAccounts(t, test.genesisAssetAccounts, assetAccounts)
		})
	}
}

func TestSetupGenesis(t *testing.T) {
	var (
		sargaAccount    = getTestAccountWithAccType(t, common.SargaAccount)
		genesisAccounts = []common.AccountSetupArgs{
			getTestAccountWithAccType(t, common.RegularAccount),
			getTestAccountWithAccType(t, common.LogicAccount),
		}
		genesisAccountHashes = AccHash{
			contextHash: tests.RandomHash(t),
			stateHash:   tests.RandomHash(t),
		}
		assetSetupArgs = []common.AssetAccountSetupArgs{
			{
				AssetInfo: getAssetCreationArgs(
					"MOI",
					genesisAccounts[0].Address,
					[]identifiers.Address{genesisAccounts[1].Address},
					[]*big.Int{big.NewInt(12)},
				),
				BehaviouralContext: tests.RandomKramaIDs(t, 1),
			},
		}
		logics = getTestGenesisLogics(t)
	)

	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	cfg := &config.ChainConfig{
		GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount,
			genesisAccounts, assetSetupArgs, logics),
		GenesisTimestamp: uint64(time.Now().Unix()),
	}

	assetAccAddr := common.CreateAddressFromString(assetSetupArgs[0].AssetInfo.Symbol)
	logicAddr := common.CreateAddressFromString(logics[0].Name)

	testcases := []struct {
		name          string
		cfg           *config.ChainConfig
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "should succeed for valid genesis info",
			cfg:  cfg,
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(common.SargaAddress, sargaAccount.AccType)
				sm.setAccType(assetAccAddr, common.AssetAccount)
				sm.setAccType(logicAddr, common.LogicAccount)

				for _, genesisAccount := range genesisAccounts {
					sm.setAccType(genesisAccount.Address, genesisAccount.AccType)
				}

				sm.newAccountHook = func() (common.Hash, common.Hash, error) {
					return genesisAccountHashes.stateHash, genesisAccountHashes.contextHash, nil
				}
			},
		},
		{
			name: "should return error if failed to add genesis tesseract",
			cfg:  cfg,
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(common.SargaAddress, sargaAccount.AccType)
			},
			expectedError: errors.New("error adding genesis tesseract"),
		},
		{
			name: "should return error if failed to parse genesis file",
			cfg: &config.ChainConfig{
				GenesisFilePath: createMockGenesisFile(t, dir, true, sargaAccount, genesisAccounts,
					nil, nil),
				GenesisTimestamp: uint64(time.Now().Unix()),
			},
			expectedError: errors.New("failed to parse genesis file"),
		},
		{
			name: "should return error if failed to setup sarga account",
			cfg: &config.ChainConfig{
				GenesisFilePath: createMockGenesisFile(t, dir, false,
					*getAccountSetupArgs(
						t,
						common.SargaAddress,
						common.SargaAccount,
						"moi-id",
						nil,
						nil,
					), genesisAccounts, nil, nil),
				GenesisTimestamp: uint64(time.Now().Unix()),
			},
			expectedError: errors.New("failed to setup sarga account"),
		},
		{
			name: "should return error if failed to setup genesis account",
			cfg: &config.ChainConfig{
				GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount,
					[]common.AccountSetupArgs{*getAccountSetupArgs(
						t,
						tests.RandomAddress(t),
						common.RegularAccount,
						"moi-id",
						nil,
						nil,
					)}, nil, nil),
				GenesisTimestamp: uint64(time.Now().Unix()),
			},
			expectedError: errors.New("failed to setup genesis account"),
		},
		{
			name: "should return error if failed to setup genesis logic",
			cfg: &config.ChainConfig{
				GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts,
					nil,
					[]common.LogicSetupArgs{
						{
							Name:               "staking-contract",
							Manifest:           hexutil.Bytes{0, 1},
							BehaviouralContext: tests.RandomKramaIDs(t, 1),
						},
					}),
				GenesisTimestamp: uint64(time.Now().Unix()),
			},
			expectedError: errors.New("failed to setup genesis logic"),
		},
		{
			name: "should return error if failed to setup assets",
			cfg: &config.ChainConfig{
				GenesisFilePath: createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts,
					[]common.AssetAccountSetupArgs{
						getAssetAccountSetupArgs(t, *getAssetCreationArgs(
							"MOI",
							tests.RandomAddress(t),
							[]identifiers.Address{tests.RandomAddress(t)},
							[]*big.Int{big.NewInt(12)},
						), nil, nil),
					}, nil),
				GenesisTimestamp: uint64(time.Now().Unix()),
			},
			expectedError: errors.New("failed to setup asset accounts"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := mockStateManager()
			chainParams := &CreateChainParams{
				sm:         sm,
				smCallBack: test.smCallBack,
			}

			c := createTestChainManager(t, chainParams)

			err = c.SetupGenesis(test.cfg)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForGenesisTesseract(
				t,
				sm,
				common.SargaAddress,
			)

			for _, genesisAccount := range genesisAccounts {
				checkForGenesisTesseract(
					t,
					sm,
					genesisAccount.Address,
				)
			}

			checkForGenesisTesseract(
				t,
				sm,
				assetAccAddr, // asset account address
			)

			checkForGenesisTesseract(
				t,
				sm,
				logicAddr, // logic account address
			)
		})
	}
}

func TestSetupSargaAccount(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.RandomKramaIDs(t, 12)

	var emptyNodes []kramaid.KramaID

	testcases := []struct {
		name          string
		sarga         *common.AccountSetupArgs
		accounts      []common.AccountSetupArgs
		assets        []common.AssetAccountSetupArgs
		logics        []common.LogicSetupArgs
		expectedError error
	}{
		{
			name: "behavioural nodes and random nodes are empty",
			sarga: getAccountSetupArgs(t,
				common.SargaAddress,
				2,
				"moi-id",
				emptyNodes,
				emptyNodes,
			),
			expectedError: errors.New("context initiation failed in genesis"),
		},
		{
			name: "other accounts added to sarga account",
			sarga: getAccountSetupArgs(
				t,
				common.SargaAddress,
				2,
				"moi-id",
				nodes[:2],
				nodes[2:4],
			),
			accounts: []common.AccountSetupArgs{
				*getAccountSetupArgs(
					t,
					tests.RandomAddress(t),
					3,
					"moi-id",
					nodes[4:6],
					nodes[6:8],
				),
				*getAccountSetupArgs(
					t,
					tests.RandomAddress(t),
					9,
					"moi-id",
					nodes[8:10],
					nodes[10:12],
				),
			},
			assets: []common.AssetAccountSetupArgs{
				getAssetAccountSetupArgs(
					t,
					getTestAssetCreationArgs(t, identifiers.NilAddress), nil, nil),
				getAssetAccountSetupArgs(
					t,
					getTestAssetCreationArgs(t, identifiers.NilAddress), nil, nil),
			},
			logics: []common.LogicSetupArgs{
				{
					Name: "staking",
				},
				{
					Name: "DEX",
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			obj, err := cm.SetupSargaAccount(test.sarga, test.accounts, test.assets, test.logics)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateContextInitialization(
				t,
				cm.sm,
				common.SargaAddress,
				test.sarga.BehaviouralContext,
				test.sarga.RandomContext,
				obj.ContextHash(),
			)

			obj, _ = cm.sm.GetDirtyObject(common.SargaAddress)

			checkSargaObjectAccounts(t, obj, test.accounts)
			checkSargaObjectLogicAccounts(t, obj, test.logics)
			checkSargaObjectAssetAccounts(t, obj, test.assets)
		})
	}
}

func TestSetupAssetAccounts(t *testing.T) {
	nodes := tests.RandomKramaIDs(t, 8)
	owner := tests.RandomAddress(t)
	address := tests.GetAddresses(t, 2)
	amount := []*big.Int{big.NewInt(100), big.NewInt(200)}

	MOIAssetInfo := getAssetCreationArgs(
		"MOI",
		owner,
		address,
		amount,
	)

	MOIAssetSetupArgs := common.AssetAccountSetupArgs{
		AssetInfo:          MOIAssetInfo,
		BehaviouralContext: nodes[:2],
		RandomContext:      nodes[2:4],
	}

	testcases := []struct {
		name          string
		assetAccs     []common.AssetAccountSetupArgs
		smCallback    func(sm *MockStateManager, stateObjects map[identifiers.Address]*state.Object)
		expectedError error
	}{
		{
			name: "should succeed for valid asset info",
			assetAccs: []common.AssetAccountSetupArgs{
				MOIAssetSetupArgs,
				getAssetAccountSetupArgs(
					t,
					*getAssetCreationArgs(
						"WILL",
						owner,
						address,
						amount,
					),
					nodes[4:6],
					nodes[6:8],
				),
			},
			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Address]*state.Object) {
				stateObjects[owner] = sm.CreateDirtyObject(owner, common.RegularAccount)

				for i := 0; i < len(address); i++ {
					stateObjects[address[i]] = sm.CreateDirtyObject(address[i], common.RegularAccount)
				}
			},
		},
		{
			name: "should return error if failed to create context",
			assetAccs: []common.AssetAccountSetupArgs{
				getAssetAccountSetupArgs(t, *MOIAssetInfo, nil, nil),
			},
			expectedError: errors.New("liveliness size not met"),
		},
		{
			name: "should return error if owner account not found",
			assetAccs: []common.AssetAccountSetupArgs{
				MOIAssetSetupArgs,
			},
			expectedError: errors.New("operator account not found"),
		},
		{
			name: "should return error if asset already registered in asset account",
			assetAccs: []common.AssetAccountSetupArgs{
				MOIAssetSetupArgs,
			},
			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Address]*state.Object) {
				accAddress := common.CreateAddressFromString(MOIAssetInfo.Symbol)
				stateObjects[accAddress] = sm.CreateDirtyObject(accAddress, common.AssetAccount)
				_, err := stateObjects[accAddress].CreateAsset(accAddress, MOIAssetInfo.AssetDescriptor())
				require.NoError(t, err)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name: "should return error if asset already registered in owner account",
			assetAccs: []common.AssetAccountSetupArgs{
				MOIAssetSetupArgs,
			},
			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Address]*state.Object) {
				stateObjects[owner] = sm.CreateDirtyObject(owner, common.RegularAccount)
				accAddress := common.CreateAddressFromString(MOIAssetInfo.Symbol)
				_, err := stateObjects[MOIAssetInfo.Operator].CreateAsset(accAddress, MOIAssetInfo.AssetDescriptor())
				require.NoError(t, err)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name: "should return error if allocation address not found in state objects",
			assetAccs: []common.AssetAccountSetupArgs{
				MOIAssetSetupArgs,
			},
			smCallback: func(sm *MockStateManager, stateObjects map[identifiers.Address]*state.Object) {
				stateObjects[owner] = sm.CreateDirtyObject(owner, common.RegularAccount)
			},
			expectedError: errors.New("allocation address not found in state objects"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateObjects := make(map[identifiers.Address]*state.Object)
			sm := mockStateManager()
			chainParams := &CreateChainParams{
				sm: sm,
			}

			if test.smCallback != nil {
				test.smCallback(sm, stateObjects)
			}

			cm := createTestChainManager(t, chainParams)

			err := cm.SetupAssetAccounts(stateObjects, test.assetAccs)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for i := 0; i < len(test.assetAccs); i++ {
				assetInfo := test.assetAccs[i].AssetInfo
				expectedAssetDescriptor, err := assetInfo.AssetDescriptor().Bytes()
				require.NoError(t, err)

				assetAccAddress := common.CreateAddressFromString(assetInfo.Symbol)
				assetSO, ok := stateObjects[assetAccAddress]
				require.True(t, ok)

				assetID := identifiers.NewAssetIDv0(
					assetInfo.IsLogical,
					assetInfo.IsStateful,
					assetInfo.Dimension.ToInt(),
					assetInfo.Standard.ToInt(),
					assetAccAddress,
				)

				validateContextInitialization(
					t,
					cm.sm,
					assetAccAddress,
					test.assetAccs[i].BehaviouralContext,
					test.assetAccs[i].RandomContext,
					assetSO.ContextHash(),
				)

				checkForAssetRegistry(t, assetSO, assetID, expectedAssetDescriptor)

				ownerSO, ok := stateObjects[assetInfo.Operator]
				require.True(t, ok)

				checkForAssetRegistry(t, ownerSO, assetID, expectedAssetDescriptor)
				checkForAllocations(t, stateObjects, assetInfo, assetID)
			}
		})
	}
}

func TestSetupNewAccount(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.RandomKramaIDs(t, 12)

	testcases := []struct {
		name               string
		newAcc             *common.AccountSetupArgs
		expectedError      error
		behaviouralContext []kramaid.KramaID
		randomContext      []kramaid.KramaID
	}{
		{
			name: "behavioural nodes and random nodes are empty",
			newAcc: getAccountSetupArgs(t,
				tests.RandomAddress(t),
				3,
				"moi-id",
				nil,
				nil,
			),
			expectedError: errors.New("context initiation failed in genesis"),
		},
		{
			name: "behavioural nodes and random nodes are set",
			newAcc: getAccountSetupArgs(t,
				tests.RandomAddress(t),
				3,
				"moi-id",
				nodes[:6],
				nodes[6:],
			),
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateObject, err := cm.SetupNewAccount(*test.newAcc)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateContextInitialization(
				t,
				cm.sm,
				test.newAcc.Address,
				test.behaviouralContext,
				test.randomContext,
				stateObject.ContextHash(),
			)
		})
	}
}

func TestExecuteGenesisContracts(t *testing.T) {
	logicID := identifiers.NewLogicIDv0(true, false, false, false, 0, common.StakingContractAddr)

	ids := tests.RandomKramaIDs(t, 1)

	objectsMap := make(map[identifiers.Address]*state.Object)

	testcases := []struct {
		name          string
		logics        []common.LogicSetupArgs
		smCallBack    func(sm *MockStateManager)
		expectedError string
	}{
		{
			name: "invalid logic name",
			logics: []common.LogicSetupArgs{{
				Name:               "test",
				BehaviouralContext: ids,
			}},
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), common.LogicAccount)
			},
			expectedError: "generated address does not exist in predefined contract address",
		},
		{
			name: "missing behaviour context",
			logics: []common.LogicSetupArgs{{
				Name:               "staking-contract",
				BehaviouralContext: nil,
			}},
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), common.LogicAccount)
			},
			expectedError: "context initiation failed in genesis",
		},
		{
			name: "valid contract paths",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), common.LogicAccount)
			},
			logics: getTestGenesisLogics(t),
		},
		{
			name: "unable to deploy logic for contract",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), common.LogicAccount)
			},
			logics: []common.LogicSetupArgs{
				{
					Name:               "staking-contract",
					BehaviouralContext: tests.RandomKramaIDs(t, 1),
				},
			},
			expectedError: "deployment failed for logic",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := mockStateManager()
			chainParams := &CreateChainParams{
				sm:         sm,
				smCallBack: test.smCallBack,
			}

			cm := createTestChainManager(t, chainParams)
			_, err := cm.SetupGenesisLogics(objectsMap, test.logics)

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGetInteractionsByTSHash(t *testing.T) {
	var (
		tsHash    = tests.RandomHash(t)
		paramsMap = tests.GetIxParamsMapWithAddresses(
			[]identifiers.Address{tests.RandomAddress(t)},
			[]identifiers.Address{tests.RandomAddress(t)},
		)
		ixns = tests.CreateIxns(t, 2, paramsMap)
		c    = createTestChainManager(t, nil)
	)

	rawIxns, err := ixns.Bytes()
	require.NoError(t, err)

	err = c.db.SetInteractions(tsHash, rawIxns)
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		tsHash               common.Hash
		expectedInteractions common.Interactions
		expectedError        error
	}{
		{
			name:                 "fetch interactions successfully",
			tsHash:               tsHash,
			expectedInteractions: ixns,
		},
		{
			name:          "failed to fetch interactions",
			tsHash:        tests.RandomHash(t),
			expectedError: common.ErrFetchingInteractions,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixns, err := c.getInteractionsByTSHash(test.tsHash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedInteractions, ixns)
		})
	}
}

func TestGetInteractionByTSHash(t *testing.T) {
	ts := createTesseracts(t, 1, getTesseractParamsMapWithIxns(t, 3))

	chainParams := &CreateChainParams{
		smCallBack: func(sm *MockStateManager) {
			sm.InsertTesseractsInDB(t, ts...)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name                 string
		tsHash               common.Hash
		ixIndex              int
		expectedInteraction  *common.Interaction
		expectedParticipants common.Participants
		expectedError        error
	}{
		{
			name:                 "fetch interactions successfully",
			tsHash:               ts[0].Hash(),
			ixIndex:              1,
			expectedParticipants: ts[0].Participants(),
			expectedInteraction:  ts[0].Interactions()[1],
		},
		{
			name:          "tesseract not found",
			tsHash:        tests.RandomHash(t),
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name:          "interaction index exceeded",
			tsHash:        ts[0].Hash(),
			ixIndex:       3,
			expectedError: common.ErrIndexOutOfRange,
		},
		{
			name:          "interaction index cannot be negative",
			tsHash:        ts[0].Hash(),
			ixIndex:       -1,
			expectedError: common.ErrIndexOutOfRange,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix, participants, err := c.GetInteractionAndParticipantsByTSHash(test.tsHash, test.ixIndex)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedInteraction, ix)
			require.Equal(t, test.expectedParticipants, participants)
		})
	}
}

func TestGetInteractionByIxHash(t *testing.T) {
	ixHash1 := tests.RandomHash(t)
	ixHash2 := tests.RandomHash(t)

	ts := createTesseracts(t, 1, nil)
	tsWithInteractions := createTesseracts(t, 1, getTesseractParamsMapWithIxns(t, 3))
	ts = append(ts, tsWithInteractions...)

	chainParams := &CreateChainParams{
		smCallBack: func(sm *MockStateManager) {
			sm.InsertTesseractsInDB(t, ts...)
		},
	}

	c := createTestChainManager(t, chainParams)

	err := c.db.SetIXLookup(ixHash1, ts[0].Hash())
	require.NoError(t, err)

	err = c.db.SetIXLookup(ts[1].Interactions()[1].Hash(), ts[1].Hash())
	require.NoError(t, err)

	err = c.db.SetIXLookup(ixHash2, tests.RandomHash(t))
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		ixHash               common.Hash
		expectedInteraction  *common.Interaction
		expectedParticipants common.Participants
		expectedIndex        int
		expectedError        error
	}{
		{
			name:                 "fetch interactions successfully",
			ixHash:               ts[1].Interactions()[1].Hash(),
			expectedParticipants: ts[1].Participants(),
			expectedInteraction:  ts[1].Interactions()[1],
			expectedIndex:        1,
		},
		{
			name:          "interaction not found",
			ixHash:        ixHash1,
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name:          "tesseract hash not found",
			ixHash:        tests.RandomHash(t),
			expectedError: common.ErrTSHashNotFound,
		},
		{
			name:          "tesseract not found",
			ixHash:        ixHash2,
			expectedError: common.ErrFetchingTesseract,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix, tsHash, participants, ixIndex, err := c.GetInteractionAndParticipantsByIxHash(test.ixHash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, ts[1].Hash(), tsHash)
			require.Equal(t, test.expectedInteraction, ix)
			require.Equal(t, test.expectedParticipants, participants)
			require.Equal(t, test.expectedIndex, ixIndex)
		})
	}
}

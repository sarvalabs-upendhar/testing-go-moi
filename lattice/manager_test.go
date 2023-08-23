package lattice

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/utils"

	"github.com/sarvalabs/go-moi/state"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
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
	nodes := tests.GetTestKramaIDs(t, 3)

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
					address: address,
					headerCallback: func(header *common.TesseractHeader) {
						setContextLock(header, address, getContextLockInfo(hash, common.NilHash, 0))
					},
					bodyCallback: func(body *common.TesseractBody) {
						setContextDelta(body, address, getDeltaGroup(t, 2, 2, 0))
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
			name:    "should previous tesseract doesn't exist",
			tsCount: 1,
			paramsMap: map[int]*createTesseractParams{
				0: {
					address: address,
					headerCallback: func(header *common.TesseractHeader) {
						header.PrevHash = tests.RandomHash(t)
					},
					bodyCallback: func(body *common.TesseractBody) {
						body.ContextDelta[address] = getDeltaGroup(t, 1, 3, 0)
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

			peers, err := c.fetchContextForAgora(*ts[test.tsCount-1])

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.peerCount, len(peers)) // check if peer count matches

			nodes := fetchContextFromLattice(t, *ts[test.tsCount-1], c)
			require.Equal(t, nodes, peers) // check if context nodes matches
		})
	}
}

func TestAddKnownHashes(t *testing.T) {
	tesseracts := createTesseracts(t, 3, nil)

	c := createTestChainManager(t, nil)
	c.AddKnownHashes(tesseracts)

	for _, ts := range tesseracts {
		isExists := c.knownTesseracts.Contains(ts.Hash())
		require.True(t, isExists)
	}
}

func TestUpdateNodeInclusivity(t *testing.T) {
	senatus := mockSenatus(t) // initializing senatus here as we need to validate results by using this
	chainParams := &CreateChainParams{
		senatus: senatus,
	}
	c := createTestChainManager(t, chainParams)

	// create context delta
	ctxDelta := make(common.ContextDelta)
	ctxDelta[tests.RandomAddress(t)] = getDeltaGroup(t, 6, 5, 2)
	ctxDelta[tests.RandomAddress(t)] = getDeltaGroup(t, 6, 6, 6)

	err := c.UpdateNodeInclusivity(ctxDelta)
	require.NoError(t, err)

	validateNodeInclusivity(t, senatus, ctxDelta)
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
		address          common.Address
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
				address:          ts.Address(),
				height:           ts.Height(),
				withInteractions: true,
			},
			expectedTS: ts,
		},
		{
			name: "fetch tesseract without Interactions for a valid height",
			args: args{
				address:          ts.Address(),
				height:           ts.Height(),
				withInteractions: false,
			},
			expectedTS: ts,
		},
		{
			name: "should return error for invalid height",
			args: args{
				address:          ts.Address(),
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
		address       common.Address
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
		address          common.Address
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:          "should return error for nil address",
			address:       common.NilAddress,
			expectedError: common.ErrInvalidAddress,
		},
		{
			name:          "should return error for invalid address",
			address:       tests.RandomAddress(t),
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name:             "fetch tesseract without interactions",
			address:          ts.Address(),
			withInteractions: false,
			expectedTS:       ts,
		},
		{
			name:             "fetch tesseract with interactions",
			address:          ts.Address(),
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

	gridHash := tests.RandomHash(t)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertReceipts(t, db, gridHash, receipts)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		gridHash      common.Hash
		ixHash        common.Hash
		expectedError error
	}{
		{
			name:     "receipt exists",
			gridHash: gridHash,
			ixHash:   ixs[1].Hash(),
		},
		{
			name:          "should return error if receipt root hash is invalid",
			gridHash:      tests.RandomHash(t),
			ixHash:        ixs[1].Hash(),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:          "should return error if ixHash doesn't exist",
			gridHash:      gridHash,
			ixHash:        tests.RandomHash(t),
			expectedError: common.ErrReceiptNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualReceipts, err := c.getReceipt(test.ixHash, test.gridHash)

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
	gridHash := tests.RandomHash(t)
	unknownGridHash := tests.RandomHash(t)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertReceipts(t, db, gridHash, receipts)

			err := db.SetIXGridLookup(ixns[0].Hash(), gridHash)
			require.NoError(t, err)

			err = db.SetIXGridLookup(ixns[1].Hash(), unknownGridHash)
			require.NoError(t, err)

			receiptData, err := receipts.Bytes()
			require.NoError(t, err)

			err = db.SetReceipts(gridHash, receiptData)
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
			name:          "grid hash not found",
			ixHash:        tests.RandomHash(t),
			expectedError: common.ErrGridHashNotFound,
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
			headerCallback: func(header *common.TesseractHeader) {
				header.Extra.VoteSet.SetIndex(index, value)
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

func TestVerifyHeaders(t *testing.T) {
	address := tests.RandomAddress(t)
	sargaTesseractHash := tests.RandomHash(t)

	getTesseractParams := func(clusterID string, height uint64) *createTesseractParams {
		return &createTesseractParams{
			address: address,
			headerCallback: func(header *common.TesseractHeader) {
				header.ClusterID = clusterID
				header.Height = height
			},
		}
	}

	getTesseractParamsWithTimeStamp := func(height uint64, timestamp int64) *createTesseractParams {
		return &createTesseractParams{
			address: address,
			headerCallback: func(header *common.TesseractHeader) {
				header.ClusterID = "non-genesis"
				header.Height = height
				header.Timestamp = timestamp
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
					address: address,
					headerCallback: func(header *common.TesseractHeader) {
						header.ClusterID = "non-genesis"
						header.PrevHash = tests.RandomHash(t)
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

			err := c.verifyHeaders(ts[len(ts)-1])
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAddTesseract(t *testing.T) {
	address := tests.RandomAddress(t)
	addresses := getAddresses(t, 4)
	ixns := createIxns(t, 2, getIxParamsMapWithAddresses(addresses[:2], addresses[2:]))
	_, receipts := getIxAndReceipts(t, 1)

	testcases := []struct {
		name                        string
		args                        testTSArgs
		accType                     common.AccountType
		tesseractsParams            map[int]*createTesseractParams
		smCallBack                  func(sm *MockStateManager)
		setTesseractHook            func() error
		setInteractionsHook         func() error
		setReceiptsHook             func() error
		setTesseractHeightEntryHook func() error
		setTSGridLookupHook         func() error
		updateWalletCountHook       func() error
		tsCount                     int
		latticeExists               bool
		expectedError               error
	}{
		{
			name: "add tesseract with state",
			args: testTSArgs{
				cache:           true,
				stateExists:     true,
				tesseractExists: false,
			},
			accType: common.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					address: address,
					ixns:    ixns,
					headerCallback: func(header *common.TesseractHeader) {
						header.PrevHash = tests.RandomHash(t)
						header.Extra = createCommitdataWithRandomGridHash(t)
					},
					bodyCallback: func(body *common.TesseractBody) {
						body.ContextDelta[address] = getDeltaGroup(t, 3, 3, 0)
					},
				},
			},
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(address, common.LogicAccount)
			},
			tsCount: 1,
		},
		{
			name: "add tesseract by avoiding cache",
			args: testTSArgs{
				cache:           false,
				stateExists:     false,
				tesseractExists: true,
			},
			accType: common.RegularAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					address: address,
				},
				1: {
					address:        address,
					ixns:           ixns,
					headerCallback: tests.HeaderCallbackWithGridHash(t),
				},
			},
			tsCount:       2,
			latticeExists: true,
		},
		{
			name:    "should return error if unable to store tesseract",
			args:    testTSArgs{},
			accType: common.LogicAccount,
			setTesseractHook: func() error {
				return errors.New("error writing tesseract to db")
			},
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					headerCallback: tests.HeaderCallbackWithGridHash(t),
				},
			},
			tsCount:       1,
			expectedError: errors.New("error writing tesseract to db"),
		},
		{
			name:    "should return error if unable to store interactions",
			args:    testTSArgs{},
			accType: common.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					ixns:           ixns,
					headerCallback: tests.HeaderCallbackWithGridHash(t),
				},
			},
			setInteractionsHook: func() error {
				return errors.New("error writing interactions to db")
			},
			tsCount:       1,
			expectedError: errors.New("error writing interactions to db"),
		},
		{
			name:    "should return error if unable to store receipts",
			args:    testTSArgs{},
			accType: common.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					ixns:           ixns,
					receipts:       receipts,
					headerCallback: tests.HeaderCallbackWithGridHash(t),
				},
			},
			setReceiptsHook: func() error {
				return errors.New("error writing receipts to db")
			},
			tsCount:       1,
			expectedError: errors.New("error writing receipts to db"),
		},
		{
			name:    "should return error if unable to store tesseract height entry",
			args:    testTSArgs{},
			accType: common.LogicAccount,
			setTesseractHeightEntryHook: func() error {
				return errors.New("failed to write tesseract height entry")
			},
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					headerCallback: tests.HeaderCallbackWithGridHash(t),
				},
			},
			tsCount:       1,
			expectedError: errors.New("failed to write tesseract height entry"),
		},
		{
			name:    "should return error if unable to store ix lookup",
			args:    testTSArgs{},
			accType: common.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					ixns: ixns,
				},
			},
			setTSGridLookupHook: func() error {
				return errors.New("error writing ts grid lookup to db")
			},
			tsCount:       1,
			expectedError: errors.New("error writing ts grid lookup to db"),
		},
		{
			name:    "should return error if unable to update node inclusivity",
			args:    testTSArgs{},
			accType: common.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					address:        address,
					ixns:           ixns,
					headerCallback: tests.HeaderCallbackWithGridHash(t),
					bodyCallback: func(body *common.TesseractBody) {
						body.ContextDelta[address] = getDeltaGroup(t, 3, 3, 0)
					},
				},
			},
			updateWalletCountHook: func() error {
				return common.ErrUpdatingInclusivity
			},
			tsCount:       1,
			expectedError: common.ErrUpdatingInclusivity,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tsCount := test.tsCount
			ts := createTesseractsWithChain(t, test.tsCount, test.tesseractsParams)

			db := mockDB()
			senatus := mockSenatus(t)
			sm := mockStateManager()
			ixPool := mockIXPool(t)

			sm.setAccType(address, test.accType)
			chainParams := &CreateChainParams{
				db:         db,
				senatus:    senatus,
				sm:         sm,
				ixPool:     ixPool,
				smCallBack: test.smCallBack,
				dbCallback: func(db *MockDB) {
					if tsCount-1 > 0 { // add all previous tesseracts
						insertTesseractsInDB(t, db, ts[0:(tsCount-1)]...)
					}
					db.setTesseractHook = test.setTesseractHook
					db.setInteractionsHook = test.setInteractionsHook
					db.setReceiptsHook = test.setReceiptsHook
					db.setTesseractHeightEntryHook = test.setTesseractHeightEntryHook
					db.setTSGridLookupHook = test.setTSGridLookupHook
				},
				senatusCallback: func(senatus *MockSenatus) {
					senatus.UpdateWalletCountHook = test.updateWalletCountHook
				},
			}

			c := createTestChainManager(t, chainParams)

			tsAddedEventSub := c.mux.Subscribe(utils.TesseractAddedEvent{}) // subscribe to tesseract added event

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tsAddedResp := make(chan result, 1)

			go handleMuxEvents(ctx, tsAddedEventSub, tsAddedResp) // keeps checking for event until timeout

			err := c.addTesseract(
				test.args.cache,
				address,
				ts[tsCount-1],
				test.args.tesseractExists,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkIfTesseractAdded(t,
				c,
				db,
				sm,
				senatus,
				ixPool,
				test.args,
				tsCount,
				test.latticeExists,
				test.accType,
				ts[tsCount-1],
			)

			validateTSAddedEvent(t, tsAddedResp, ts[tsCount-1])
		})
	}
}

func TestAddTesseractsWithState(t *testing.T) {
	var (
		accType   = common.LogicAccount
		addresses = getAddresses(t, 4)
		ixns      = createIxns(t, 2, getIxParamsMapWithAddresses(addresses[:2], addresses[2:]))
	)

	clusterInfo := getTestClusterInfo(t, 2)
	tsParamsMap := make(map[int]*createTesseractParams, 3)

	for i := 0; i < 3; i++ {
		tsParamsMap[i] = tesseractParamsWithICSClusterInfo(t, ixns, clusterInfo)
	}

	testcases := []struct {
		name             string
		dirtyStorage     map[common.Hash][]byte
		tesseractsParams map[int]*createTesseractParams
		createEntryHook  func() error
		setTesseractHook func() error
		tsCount          int
		expectedError    error
	}{
		{
			name:             "add tesseract with state",
			dirtyStorage:     getTestDirtyEntriesWithClusterInfo(t, clusterInfo, 2),
			tesseractsParams: tsParamsMap,
			tsCount:          3,
		},
		{
			name:          "should return error for empty tesseracts",
			dirtyStorage:  getTestDirtyEntriesWithClusterInfo(t, clusterInfo, 2),
			tsCount:       0,
			expectedError: errors.New("empty tesseracts"),
		},
		{
			name:             "should return error if ICS cluster info not found",
			dirtyStorage:     getTestDirtyEntriesWithClusterInfo(t, nil, 2),
			tesseractsParams: tsParamsMap,
			tsCount:          1,
			expectedError:    errors.New("cluster info not found"),
		},
		{
			name:         "should return error if unable to store dirty entries",
			dirtyStorage: getTestDirtyEntriesWithClusterInfo(t, clusterInfo, 2),
			createEntryHook: func() error {
				return errors.New("failed to write dirty keys")
			},
			tesseractsParams: tsParamsMap,
			tsCount:          1,
			expectedError:    errors.New("failed to write dirty keys"),
		},
		{
			name:             "should return error if unable to write tesseract",
			dirtyStorage:     getTestDirtyEntriesWithClusterInfo(t, clusterInfo, 2),
			tesseractsParams: tsParamsMap,
			tsCount:          1,
			setTesseractHook: func() error {
				return errors.New("error writing tesseract to db")
			},
			expectedError: errors.New("error writing tesseract to db"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tsCount := test.tsCount
			ts := createTesseracts(t, tsCount, test.tesseractsParams)

			chainParams := &CreateChainParams{
				smCallBack: func(sm *MockStateManager) {
					setAccountType(sm, accType, ts...)
				},
				dbCallback: func(db *MockDB) {
					db.createEntryHook = test.createEntryHook
					db.setTesseractHook = test.setTesseractHook
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.addTesseractsWithState(
				test.dirtyStorage,
				ts...,
			)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForDirtyEntries(t, c.db, test.dirtyStorage) // check if dirty entries are inserted in db
			checkForAddedTesseracts(t, c, ts, accType)
		})
	}
}

func TestValidateTesseract(t *testing.T) {
	address := tests.RandomAddress(t)
	nodes := getICSNodeset(t, 1)

	tsParamsMap := map[int]*createTesseractParams{
		0: tesseractParamsWithCommitSign(validCommitSign),
		1: tesseractParamsWithCommitSign(invalidCommitSign),
		2: tesseractParamsWithCommitSign(validCommitSign),
		3: {
			address:        address, // initialize address separately as it needs to be registered later
			headerCallback: getHeaderCallbackWithCommitSign(validCommitSign),
		},
		4: tesseractParamsWithCommitSign(validCommitSign),
		5: tesseractParamsWithCommitSign(validCommitSign),
		6: tesseractParamsWithCommitSign(validCommitSign),
	}

	ts := createTesseracts(t, 7, tsParamsMap)

	testcases := []struct {
		name                 string
		smCallback           func(sm *MockStateManager)
		dbCallback           func(db *MockDB)
		chainManagerCallback func(c *ChainManager)
		isOrphanTS           bool
		ts                   *common.Tesseract
		expectedError        error
	}{
		{
			name: "valid tesseract",
			ts:   ts[0],
		},
		{
			name:          "should return error if ics signature is invalid",
			ts:            ts[1],
			expectedError: errors.New("failed to verify signatures"),
		},
		{
			name:          "should return error if sarga account not state found",
			smCallback:    smCallbackWithAccRegistrationHook(),
			ts:            ts[2],
			expectedError: errors.New("Sarga account not found"),
		},
		{
			name:       "should be added to orphans if previous tesseract not found",
			smCallback: smCallbackWithRegisteredAcc(address),
			ts:         ts[3],
			isOrphanTS: true,
		},
		{
			name: "should return error if tesseract already exits",
			dbCallback: func(db *MockDB) {
				insertTesseractsInDB(t, db, ts[4]) // add tesseract to db
			},
			ts:            ts[4],
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
			chainParams := &CreateChainParams{
				sm:                   sm,
				smCallBack:           test.smCallback,
				dbCallback:           test.dbCallback,
				chainManagerCallback: test.chainManagerCallback,
			}

			c := createTestChainManager(t, chainParams)

			// generate seal at the end as there are modifications to header
			if !errors.Is(test.expectedError, common.ErrInvalidSeal) {
				signTesseract(t, sm, test.ts)
			}

			err := c.ValidateTesseract(test.ts, nodes)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			// check for orphan tesseracts in orphans cache
			if test.isOrphanTS {
				checkForOrphanTSInCache(t, c, test.ts)
			}
		})
	}
}

func TestAddTesseracts(t *testing.T) {
	var (
		hashes      = getHashes(t, 2, false)
		clusterInfo = getTestClusterInfo(t, 2)
	)

	tesseractParamsWithGroupHash := func(
		clusterInfo *common.ICSClusterInfo,
		groupHash common.Hash,
	) *createTesseractParams {
		rawData, err := clusterInfo.Bytes()
		require.NoError(t, err)

		return &createTesseractParams{
			headerCallback: func(header *common.TesseractHeader) {
				header.PrevHash = tests.RandomHash(t)
				header.GroupHash = groupHash
				header.Extra = createCommitdataWithRandomGridHash(t)
			},
			bodyCallback: func(body *common.TesseractBody) {
				body.ConsensusProof.ICSHash = common.GetHash(rawData)
			},
		}
	}

	testcases := []struct {
		name             string
		dirtyStorage     map[common.Hash][]byte
		tesseractsParams map[int]*createTesseractParams
		tsCount          int
		expectedError    error
	}{
		{
			name:         "valid tesseracts",
			dirtyStorage: getTestDirtyEntriesWithClusterInfo(t, clusterInfo, 5),
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGroupHash(clusterInfo, hashes[1]),
				1: tesseractParamsWithGroupHash(clusterInfo, hashes[1]),
			},
			tsCount: 2,
		},
		{
			name:          "should return error for nil grid",
			dirtyStorage:  nil,
			tsCount:       0,
			expectedError: errors.New("nil grid"),
		},
		{
			name:         "should return error for empty dirty storage",
			dirtyStorage: nil,
			tsCount:      2,
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGroupHash(nil, hashes[1]),
				1: tesseractParamsWithGroupHash(nil, hashes[1]),
			},
			expectedError: errors.New("empty dirty storage"),
		},
		{
			name:         "should return error if grid id doesn't match",
			dirtyStorage: getTestDirtyEntriesWithClusterInfo(t, nil, 5),
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGroupHash(nil, hashes[0]),
				1: tesseractParamsWithGroupHash(nil, hashes[1]),
			},
			tsCount:       2,
			expectedError: errors.New("grid id mismatch"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseracts(t, test.tsCount, test.tesseractsParams)

			chainParams := &CreateChainParams{
				db: mockDB(),
				smCallBack: func(sm *MockStateManager) {
					setAccountType(sm, common.LogicAccount, ts...)
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.AddTesseracts(test.dirtyStorage, ts...)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForAddedTesseracts(t, c, ts, common.LogicAccount)
		})
	}
}

func TestIsReceiptAndGroupHashValid(t *testing.T) {
	var receipts common.Receipts

	receiptRoot, err := receipts.Hash()
	require.NoError(t, err)

	groupHash := tests.RandomHash(t)

	testcases := []struct {
		name      string
		paramsMap map[int]*createTesseractParams
		isValid   bool
	}{
		{
			name: "receipt hashes doesn't match",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash, ""),
				1: tesseractParamsWithReceiptHash(t, tests.RandomHash(t), groupHash, ""),
			},
			isValid: false,
		},
		{
			name: "grid hashes doesn't match",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash, ""),
				1: tesseractParamsWithReceiptHash(t, receiptRoot, tests.RandomHash(t), ""),
			},
			isValid: false,
		},
		{
			name: "valid receipt hash and groupHash",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash, ""),
				1: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash, ""),
			},
			isValid: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseracts(t, 2, test.paramsMap)

			isValid := isReceiptAndGroupHashValid(ts, receipts)
			require.Equal(t, test.isValid, isValid)
		})
	}
}

func TestAreStateHashesValid(t *testing.T) {
	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	ixs, receipts := getIxAndReceiptsWithStateHash(t, common.ReceiptAccHashes{address: {StateHash: stateHash}}, 1)

	testcases := []struct {
		name     string
		receipts common.Receipts
		params   *createTesseractParams
		isValid  bool
	}{
		{
			name:     "receipt with given ix hash not found in receipts",
			params:   getTesseractParamsMapWithIxns(t, 1)[0],
			receipts: receipts,
			isValid:  false,
		},
		{
			name:     "state hash in receipt and tesseract doesn't match",
			params:   tesseractParamsWithStateHash(t, tests.RandomHash(t), ""),
			receipts: receipts,
			isValid:  false,
		},
		{
			name: "valid state hashes",
			params: &createTesseractParams{
				address: address,
				ixns:    ixs,
				bodyCallback: func(body *common.TesseractBody) {
					body.StateHash = stateHash
				},
			},
			receipts: receipts,
			isValid:  true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.params)

			isValid := areStateHashesValid([]*common.Tesseract{ts}, receipts)

			require.Equal(t, test.isValid, isValid)
		})
	}
}

func TestExecuteAndValidate(t *testing.T) {
	var (
		tsParamsMap    = getTSParamsMapWithStateHash(t, 1)
		address        = tests.RandomAddress(t)
		stateHash      = tests.RandomHash(t)
		ixns, receipts = getIxAndReceiptsWithStateHash(t, common.ReceiptAccHashes{address: {StateHash: stateHash}}, 1)
	)

	testcases := []struct {
		name                    string
		paramsMap               map[int]*createTesseractParams
		revertHook              func() error
		executeInteractionsHook func() (common.Receipts, error)
		expectedError           error
	}{
		{
			name:      "should return error if execution fails",
			paramsMap: tsParamsMap,
			executeInteractionsHook: func() (common.Receipts, error) {
				return nil, errors.New("failed to execute interactions")
			},
			expectedError: errors.New("failed to execute interactions"),
		},
		{
			name: "should return error if receipt validation fails",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, tests.RandomHash(t), common.NilHash, "cluster-0"),
			},
			executeInteractionsHook: func() (common.Receipts, error) {
				var emptyReceipts common.Receipts

				return emptyReceipts, nil
			},
			expectedError: errors.New("failed to validate the tesseract"),
		},
		{
			name: "should return error if state hash validation fails",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithStateHash(t, tests.RandomHash(t), "cluster-0"),
			},
			executeInteractionsHook: func() (common.Receipts, error) {
				var emptyReceipts common.Receipts

				return emptyReceipts, nil
			},
			expectedError: errors.New("failed to validate the tesseract"),
		},
		{
			name:      "should return error if revert fails",
			paramsMap: tsParamsMap,
			executeInteractionsHook: func() (common.Receipts, error) {
				return receipts, nil // execution is reverted if hashes are invalid
			},
			revertHook: func() error {
				return errors.New("revert failed")
			},
			expectedError: errors.New("failed to revert the execution changes"),
		},
		{
			name: "receipts should be added to dirty storage",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithGridInfo(t, address, stateHash,
					getReceiptHash(t, receipts), nil, ixns, 1, "cluster-0"),
			},
			executeInteractionsHook: func() (common.Receipts, error) {
				return receipts, nil
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseracts(t, 1, test.paramsMap)

			chainParams := &CreateChainParams{
				execCallback: func(exec *MockExec) {
					exec.executeInteractionsHook = test.executeInteractionsHook
					exec.revertHook = test.revertHook
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.ExecuteAndValidate(ts...)

			checkForExecutionCleanup(t, c, "cluster-0")

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, receipts, ts[0].Receipts())
		})
	}
}

func TestValidateAccountCreationInfo(t *testing.T) {
	var (
		c             = createTestChainManager(t, nil)
		walletContext = tests.GetTestKramaIDs(t, 4)
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
		address     = tests.RandomAddress(t)
		stateHash   = tests.RandomHash(t)
		contextHash = tests.RandomHash(t)
		accType     = common.LogicAccount
	)

	testcases := []struct {
		name          string
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "adding genesis tesseract successful",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(address, accType)
			},
		},
		{
			name:          "should fail if addTesseract fails",
			expectedError: errors.New("error adding genesis tesseract"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			chainParams := &CreateChainParams{
				smCallBack: test.smCallBack,
			}

			c := createTestChainManager(t, chainParams)

			err := c.AddGenesisTesseract(address, stateHash, contextHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForGenesisTesseract(
				t,
				c,
				address,
			)
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
	nodes := tests.GetTestKramaIDs(t, 4)
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
				getAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, common.NilAddress), nodes[:2], nodes[2:]),
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
					[]common.Address{genesisAccounts[1].Address},
					[]*big.Int{big.NewInt(12)},
				),
				BehaviouralContext: tests.GetTestKramaIDs(t, 1),
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

	path := createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts, assetSetupArgs, logics)

	assetAccAddr := common.CreateAddressFromString(assetSetupArgs[0].AssetInfo.Symbol)
	logicAddr := common.CreateAddressFromString(logics[0].Name)

	testcases := []struct {
		name          string
		path          string
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "should succeed for valid genesis info",
			path: path,
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
			path: path,
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(common.SargaAddress, sargaAccount.AccType)
			},
			expectedError: errors.New("error adding genesis tesseract"),
		},
		{
			name: "should return error if failed to parse genesis file",
			path: createMockGenesisFile(t, dir, true, sargaAccount, genesisAccounts,
				nil, nil),
			expectedError: errors.New("failed to parse genesis file"),
		},
		{
			name: "should return error if failed to setup sarga account",
			path: createMockGenesisFile(t, dir, false,
				*getAccountSetupArgs(
					t,
					common.SargaAddress,
					common.SargaAccount,
					"moi-id",
					nil,
					nil,
				), genesisAccounts, nil, nil),
			expectedError: errors.New("failed to setup sarga account"),
		},
		{
			name: "should return error if failed to setup genesis account",
			path: createMockGenesisFile(t, dir, false, sargaAccount,
				[]common.AccountSetupArgs{*getAccountSetupArgs(
					t,
					tests.RandomAddress(t),
					common.RegularAccount,
					"moi-id",
					nil,
					nil,
				)}, nil, nil),
			expectedError: errors.New("failed to setup genesis account"),
		},
		{
			name: "should return error if failed to setup genesis logic",
			path: createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts,
				nil,
				[]common.LogicSetupArgs{
					{
						Name:               "staking-contract",
						Manifest:           hexutil.Bytes{0, 1},
						BehaviouralContext: tests.GetTestKramaIDs(t, 1),
					},
				}),
			expectedError: errors.New("failed to setup genesis logic"),
		},
		{
			name: "should return error if failed to setup assets",
			path: createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts,
				[]common.AssetAccountSetupArgs{
					getAssetAccountSetupArgs(t, *getAssetCreationArgs(
						"MOI",
						tests.RandomAddress(t),
						[]common.Address{tests.RandomAddress(t)},
						[]*big.Int{big.NewInt(12)},
					), nil, nil),
				}, nil),
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

			err = c.SetupGenesis(test.path)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}
			require.NoError(t, err)

			checkForGenesisTesseract(
				t,
				c,
				common.SargaAddress,
			)

			for _, genesisAccount := range genesisAccounts {
				checkForGenesisTesseract(
					t,
					c,
					genesisAccount.Address,
				)
			}

			checkForGenesisTesseract(
				t,
				c,
				assetAccAddr, // asset account address
			)

			checkForGenesisTesseract(
				t,
				c,
				logicAddr, // logic account address
			)
		})
	}
}

func TestSetupSargaAccount(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.GetTestKramaIDs(t, 12)

	var emptyNodes []id.KramaID

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
					getTestAssetCreationArgs(t, common.NilAddress), nil, nil),
				getAssetAccountSetupArgs(
					t,
					getTestAssetCreationArgs(t, common.NilAddress), nil, nil),
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
	nodes := tests.GetTestKramaIDs(t, 8)
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
		smCallback    func(sm *MockStateManager, stateObjects map[common.Address]*state.Object)
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
			smCallback: func(sm *MockStateManager, stateObjects map[common.Address]*state.Object) {
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
			smCallback: func(sm *MockStateManager, stateObjects map[common.Address]*state.Object) {
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
			smCallback: func(sm *MockStateManager, stateObjects map[common.Address]*state.Object) {
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
			smCallback: func(sm *MockStateManager, stateObjects map[common.Address]*state.Object) {
				stateObjects[owner] = sm.CreateDirtyObject(owner, common.RegularAccount)
			},
			expectedError: errors.New("allocation address not found in state objects"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateObjects := make(map[common.Address]*state.Object)
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

				assetID := common.NewAssetIDv0(
					assetInfo.IsLogical,
					assetInfo.IsStateful,
					assetInfo.Dimension.ToInt(),
					common.AssetStandard(assetInfo.Standard.ToInt()),
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
	nodes := tests.GetTestKramaIDs(t, 12)

	testcases := []struct {
		name               string
		newAcc             *common.AccountSetupArgs
		expectedError      error
		behaviouralContext []id.KramaID
		randomContext      []id.KramaID
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
	logicID := common.NewLogicIDv0(true, false, false, false, 0, common.StakingContractAddr)

	ids := tests.GetTestKramaIDs(t, 1)

	objectsMap := make(map[common.Address]*state.Object)

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
					BehaviouralContext: tests.GetTestKramaIDs(t, 1),
				},
			},
			expectedError: "unable to deploy logic for contract",
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

// func CheckAssetCreation(
//	t *testing.T,
//	s *guna.Object,
//	assetDescriptor *types.AssetDescriptor,
// ) {
//	t.Helper()
//
//	expectedAssetID, expectedAssetHash, expectedData, err := getTestAssetID(assetDescriptor)
//	require.NoError(t, err)
//
//	actualData, err := s.GetDirtyEntry(expectedAssetHash.String()) // check if asset data inserted in dirty entries
//	require.NoError(t, err)
//	require.Equal(t, expectedData, actualData)
//
//	actualSupply, err := s.BalanceOf(expectedAssetID)
//	require.NoError(t, err)
//	require.Equal(t, assetDescriptor.Supply, actualSupply) // check total supply is stored in balances
//
//	require.Equal(t, s.Address(), assetDescriptor.Owner) // check if address is assigned to owner
//}

func TestGetTesseractPartsByGridHash(t *testing.T) {
	var (
		gridHash = tests.RandomHash(t)
		parts    = tests.CreateTesseractPartsWithTestData(t)
		c        = createTestChainManager(t, nil)
	)

	rawParts, err := parts.Bytes()
	require.NoError(t, err)

	err = c.db.SetTesseractParts(gridHash, rawParts)
	require.NoError(t, err)

	testcases := []struct {
		name          string
		gridHash      common.Hash
		expectedParts *common.TesseractParts
		expectedError error
	}{
		{
			name:          "fetch tesseract parts successfully",
			gridHash:      gridHash,
			expectedParts: parts,
		},
		{
			name:          "failed to fetch tesseract parts",
			gridHash:      tests.RandomHash(t),
			expectedError: errors.New("tesseract parts not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			parts, err := c.getTesseractPartsByGridHash(test.gridHash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedParts, parts)
		})
	}
}

func TestGetInteractionsByGridHash(t *testing.T) {
	var (
		gridHash  = tests.RandomHash(t)
		paramsMap = tests.GetIxParamsMapWithAddresses(
			[]common.Address{tests.RandomAddress(t)},
			[]common.Address{tests.RandomAddress(t)},
		)
		ixns = tests.CreateIxns(t, 2, paramsMap)
		c    = createTestChainManager(t, nil)
	)

	rawIxns, err := ixns.Bytes()
	require.NoError(t, err)

	err = c.db.SetInteractions(gridHash, rawIxns)
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		gridHash             common.Hash
		expectedInteractions common.Interactions
		expectedError        error
	}{
		{
			name:                 "fetch interactions successfully",
			gridHash:             gridHash,
			expectedInteractions: ixns,
		},
		{
			name:          "failed to fetch interactions",
			gridHash:      tests.RandomHash(t),
			expectedError: common.ErrFetchingInteractions,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixns, err := c.getInteractionsByGridHash(test.gridHash)

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
	var (
		gridHash             = tests.RandomHash(t)
		tsHash               = tests.RandomHash(t)
		tsHashWithoutIxns    = tests.RandomHash(t)
		tsHashWithoutParts   = tests.RandomHash(t)
		gridHashWithoutParts = tests.RandomHash(t)
		paramsMap            = tests.GetIxParamsMapWithAddresses(
			[]common.Address{tests.RandomAddress(t)},
			[]common.Address{tests.RandomAddress(t)},
		)
		ixns  = tests.CreateIxns(t, 2, paramsMap)
		parts = tests.CreateTesseractPartsWithTestData(t)
		c     = createTestChainManager(t, nil)
	)

	rawIxns, err := ixns.Bytes()
	require.NoError(t, err)

	rawParts, err := parts.Bytes()
	require.NoError(t, err)

	err = c.db.SetTSGridLookup(tsHash, gridHash)
	require.NoError(t, err)

	err = c.db.SetInteractions(gridHash, rawIxns)
	require.NoError(t, err)

	err = c.db.SetTesseractParts(gridHash, rawParts)
	require.NoError(t, err)

	err = c.db.SetTSGridLookup(tsHashWithoutIxns, tests.RandomHash(t))
	require.NoError(t, err)

	err = c.db.SetTSGridLookup(tsHashWithoutParts, gridHashWithoutParts)
	require.NoError(t, err)

	err = c.db.SetInteractions(gridHashWithoutParts, rawIxns)
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		tsHash               common.Hash
		ixIndex              int
		expectedInteractions *common.Interaction
		expectedParts        *common.TesseractParts
		expectedError        error
	}{
		{
			name:                 "fetch interactions successfully",
			tsHash:               tsHash,
			ixIndex:              1,
			expectedParts:        parts,
			expectedInteractions: ixns[1],
		},
		{
			name:          "grid hash not found",
			tsHash:        tests.RandomHash(t),
			expectedError: common.ErrGridHashNotFound,
		},
		{
			name:          "interactions not found",
			tsHash:        tsHashWithoutIxns,
			expectedError: common.ErrFetchingInteractions,
		},
		{
			name:          "tesseract parts not found",
			tsHash:        tsHashWithoutParts,
			expectedError: errors.New("tesseract parts not found"),
		},
		{
			name:          "interaction index exceeded",
			tsHash:        tsHash,
			ixIndex:       3,
			expectedError: common.ErrIndexOutOfRange,
		},
		{
			name:          "interaction index cannot be negative",
			tsHash:        tsHash,
			ixIndex:       -1,
			expectedError: common.ErrIndexOutOfRange,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix, parts, err := c.GetInteractionAndPartsByTSHash(test.tsHash, test.ixIndex)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedInteractions, ix)
			require.Equal(t, test.expectedParts, parts)
		})
	}
}

func TestGetInteractionByIxHash(t *testing.T) {
	paramsMap := tests.GetIxParamsMapWithAddresses(
		[]common.Address{tests.RandomAddress(t)},
		[]common.Address{tests.RandomAddress(t)},
	)
	ixns := tests.CreateIxns(t, 2, paramsMap)

	var (
		gridHash             = tests.RandomHash(t)
		ixHash               = ixns[1].Hash()
		ixHashWithoutIxn     = tests.RandomHash(t)
		ixHashWithoutIxns    = tests.RandomHash(t)
		ixHashWithoutParts   = ixns[0].Hash()
		gridHashWithoutParts = tests.RandomHash(t)
		parts                = tests.CreateTesseractPartsWithTestData(t)
		c                    = createTestChainManager(t, nil)
	)

	rawIxns, err := ixns.Bytes()
	require.NoError(t, err)

	rawParts, err := parts.Bytes()
	require.NoError(t, err)

	err = c.db.SetIXGridLookup(ixHash, gridHash)
	require.NoError(t, err)

	err = c.db.SetIXGridLookup(ixHashWithoutIxn, gridHash)
	require.NoError(t, err)

	err = c.db.SetIXGridLookup(ixHashWithoutParts, gridHashWithoutParts)
	require.NoError(t, err)

	err = c.db.SetInteractions(gridHash, rawIxns)
	require.NoError(t, err)

	err = c.db.SetTesseractParts(gridHash, rawParts)
	require.NoError(t, err)

	err = c.db.SetIXGridLookup(ixHashWithoutIxns, tests.RandomHash(t))
	require.NoError(t, err)

	err = c.db.SetIXGridLookup(ixHashWithoutParts, gridHashWithoutParts)
	require.NoError(t, err)

	err = c.db.SetInteractions(gridHashWithoutParts, rawIxns)
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		ixHash               common.Hash
		expectedInteractions *common.Interaction
		expectedParts        *common.TesseractParts
		expectedIndex        int
		expectedError        error
	}{
		{
			name:                 "fetch interactions successfully",
			ixHash:               ixHash,
			expectedParts:        parts,
			expectedInteractions: ixns[1],
			expectedIndex:        1,
		},
		{
			name:          "grid hash not found",
			ixHash:        tests.RandomHash(t),
			expectedError: common.ErrGridHashNotFound,
		},
		{
			name:          "interactions not found",
			ixHash:        ixHashWithoutIxns,
			expectedError: common.ErrFetchingInteractions,
		},
		{
			name:          "interaction not found",
			ixHash:        ixHashWithoutIxn,
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name:          "tesseract parts not found",
			ixHash:        ixns[0].Hash(),
			expectedError: errors.New("tesseract parts not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix, parts, ixIndex, err := c.GetInteractionAndPartsByIxHash(test.ixHash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedInteractions, ix)
			require.Equal(t, test.expectedParts, parts)
			require.Equal(t, test.expectedIndex, ixIndex)
		})
	}
}

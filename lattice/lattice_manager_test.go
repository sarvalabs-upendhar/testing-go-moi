package lattice

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/guna"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
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
		hash         types.Hash
		hasTesseract bool
	}{
		{
			name:         "tesseract doesn't exist",
			hash:         tests.RandomHash(t),
			hasTesseract: false,
		},
		{
			name:         "tesseract exists",
			hash:         getTesseractHash(t, ts),
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
					headerCallback: func(header *types.TesseractHeader) {
						setContextLock(header, address, getContextLockInfo(hash, types.NilHash, 0))
					},
					bodyCallback: func(body *types.TesseractBody) {
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
					headerCallback: func(header *types.TesseractHeader) {
						header.PrevHash = tests.RandomHash(t)
					},
					bodyCallback: func(body *types.TesseractBody) {
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

func TestFetchICSNodeSet(t *testing.T) {
	address := tests.RandomAddress(t)
	ctxHash := tests.RandomHash(t)
	nodes := tests.GetTestKramaIDs(t, 5)
	pk := getPublicKeys(t, 5)

	getTesseractParamsWithContextLock := func(address types.Address, ctxHash types.Hash) *createTesseractParams {
		return &createTesseractParams{
			address: address,
			headerCallback: func(header *types.TesseractHeader) {
				setContextLock(header, address, getContextLockInfo(ctxHash, types.NilHash, 0))
			},
		}
	}

	testcases := []struct {
		name            string
		tesseractParams *createTesseractParams
		smCallBack      func(sm *MockStateManager)
		behNodes        []id.KramaID
		randNodes       []id.KramaID
		randSet         []id.KramaID
		behPK           [][]byte
		randPK          [][]byte
		randsetPK       [][]byte
		expectedError   error
	}{
		{
			name:            "valid ics node set",
			tesseractParams: getTesseractParamsWithContextLock(address, ctxHash),
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(ctxHash, nodes[:2], nodes[2]) // insert behavioural and random context nodes
				sm.insertPublicKeys(nodes, pk)                      // insert public keys of all nodes
			},
			behNodes:  nodes[:2],
			randNodes: nodes[2:3],
			randSet:   nodes[3:5],
			behPK:     pk[:2],
			randPK:    pk[2:3],
			randsetPK: pk[3:5],
		},
		{
			name:            "should return error if random set's public keys not found",
			tesseractParams: getTesseractParamsWithContextLock(address, ctxHash),
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(ctxHash, nodes[:2], nodes[2])
				sm.insertPublicKeys(nodes[:3], pk[:3])
			},
			randSet:       nodes[3:5],
			expectedError: types.ErrKeyNotFound,
		},
		{
			name:            "should return error if context lock hash is invalid",
			tesseractParams: getTesseractParamsWithContextLock(address, ctxHash),
			randSet:         nodes[3:5],
			expectedError:   types.ErrContextStateNotFound,
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.tesseractParams)

			chainParams := &CreateChainParams{
				smCallBack: test.smCallBack,
			}

			c := createTestChainManager(t, chainParams)
			info := getTestClusterInfoWithRandomSet(t, test.randSet, 0)
			ics, err := c.fetchICSNodeSet(ts, info)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfICSNodeSetMatches(
				t,
				ics,
				ktypes.NewNodeSet(test.behNodes, test.behPK),
				ktypes.NewNodeSet(test.randNodes, test.randPK),
				ktypes.NewNodeSet(test.randSet, test.randsetPK),
			)
		})
	}
}

func TestAddKnownHashes(t *testing.T) {
	tesseracts := createTesseracts(t, 3, nil)

	c := createTestChainManager(t, nil)
	c.AddKnownHashes(tesseracts)

	for _, ts := range tesseracts {
		isExists := c.knownTesseracts.Contains(getTesseractHash(t, ts))
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
	ctxDelta := make(types.ContextDelta)
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
		tsHash           types.Hash
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "should return error if tesseract doesn't exist",
			tsHash:           tests.RandomHash(t),
			withInteractions: false,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "fetch tesseract without interactions from cache",
			tsHash:           getTesseractHash(t, ts[0]),
			withInteractions: false,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch tesseract without interactions from db",
			tsHash:           getTesseractHash(t, ts[1]),
			withInteractions: false,
			expectedTS:       ts[1],
		},
		{
			name:             "fetch tesseract with interactions from db",
			tsHash:           getTesseractHash(t, ts[2]),
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
		address          types.Address
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
		expectedTS    *types.Tesseract
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
			checkIfTesseractCachedInCM(t, c, test.args.withInteractions, getTesseractHash(t, ts))
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
		address       types.Address
		height        uint64
		expectedHash  types.Hash
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
			expectedError: types.ErrKeyNotFound,
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

func TestGetAssetDataByAssetHash(t *testing.T) {
	_, assetDescriptor := tests.CreateTestAsset(t, tests.RandomAddress(t))
	assetDescriptor.Owner = tests.RandomAddress(t)

	_, assetHash, assetObj, err := gtypes.GetAssetID(assetDescriptor)
	require.NoError(t, err)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertAssetDataByAssetHashInDB(t, db, assetHash, assetObj)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		assetHash     types.Hash
		expectedError error
	}{
		{
			name:      "asset data exists",
			assetHash: assetHash,
		},
		{
			name:          "should return error if asset data doesn't exist",
			assetHash:     tests.RandomHash(t),
			expectedError: types.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			assetData, err := c.GetAssetDataByAssetHash(test.assetHash.Bytes())

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}
			require.NoError(t, err)

			actualAssetData, err := polo.Polorize(assetData)
			require.NoError(t, err)
			require.Equal(t, assetObj, actualAssetData)
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
		address          types.Address
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:          "should return error for nil address",
			address:       types.NilAddress,
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:          "should return error for invalid address",
			address:       tests.RandomAddress(t),
			expectedError: types.ErrFetchingTesseract,
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
		gridHash      types.Hash
		ixHash        types.Hash
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
			expectedError: types.ErrKeyNotFound,
		},
		{
			name:          "should return error if ixHash doesn't exist",
			gridHash:      gridHash,
			ixHash:        tests.RandomHash(t),
			expectedError: types.ErrReceiptNotFound,
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
		ixHash        types.Hash
		receipts      types.Receipts
		expectedError error
	}{
		{
			name:          "grid hash not found",
			ixHash:        tests.RandomHash(t),
			expectedError: types.ErrGridHashNotFound,
		},
		{
			name:          "receipt not found",
			ixHash:        ixns[1].Hash(),
			expectedError: types.ErrReceiptNotFound,
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
			headerCallback: func(header *types.TesseractHeader) {
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
		ts            *types.Tesseract
		expectedError error
	}{
		{
			name:          "should return error for invalid sender quorum size",
			ts:            ts[0],
			expectedError: types.ErrQuorumFailed,
		},
		{
			name:          "should return error for invalid receiver quorum size",
			ts:            ts[1],
			expectedError: types.ErrQuorumFailed,
		},
		{
			name:          "should return error for invalid random quorum size",
			ts:            ts[2],
			expectedError: types.ErrQuorumFailed,
		},
		{
			name:          "should return error for invalid commit signature",
			ts:            ts[3],
			expectedError: types.ErrSignatureVerificationFailed,
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
			headerCallback: func(header *types.TesseractHeader) {
				header.ClusterID = clusterID
				header.Height = height
			},
		}
	}

	getTesseractParamsWithTimeStamp := func(height uint64, timestamp int64) *createTesseractParams {
		return &createTesseractParams{
			address: address,
			headerCallback: func(header *types.TesseractHeader) {
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
					headerCallback: func(header *types.TesseractHeader) {
						header.ClusterID = "non-genesis"
						header.PrevHash = tests.RandomHash(t)
					},
				},
			},
			smCallBack:    smCallback,
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "should return error for invalid height",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParams("non-genesis", 0),
				1: getTesseractParams("non-genesis", 2),
			},
			smCallBack:    smCallback,
			expectedError: types.ErrInvalidHeight,
		},
		{
			name: "should return error for invalid time stamp",
			paramsMap: map[int]*createTesseractParams{
				0: getTesseractParamsWithTimeStamp(0, 3),
				1: getTesseractParamsWithTimeStamp(1, 2),
			},
			smCallBack:    smCallback,
			expectedError: types.ErrInvalidBlockTime,
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
				sm.accountRegistrationHook = func(tsHash types.Hash) (bool, error) {
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
		accType                     types.AccountType
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
			accType: types.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					address: address,
					ixns:    ixns,
					headerCallback: func(header *types.TesseractHeader) {
						header.PrevHash = tests.RandomHash(t)
						header.Extra = createCommitdataWithRandomGridHash(t)
					},
					bodyCallback: func(body *types.TesseractBody) {
						body.ContextDelta[address] = getDeltaGroup(t, 3, 3, 0)
					},
				},
			},
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(address, types.LogicAccount)
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
			accType: types.RegularAccount,
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
			accType: types.LogicAccount,
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
			accType: types.LogicAccount,
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
			accType: types.LogicAccount,
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
			accType: types.LogicAccount,
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
			accType: types.LogicAccount,
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
			accType: types.LogicAccount,
			tesseractsParams: map[int]*createTesseractParams{
				0: {
					address:        address,
					ixns:           ixns,
					headerCallback: tests.HeaderCallbackWithGridHash(t),
					bodyCallback: func(body *types.TesseractBody) {
						body.ContextDelta[address] = getDeltaGroup(t, 3, 3, 0)
					},
				},
			},
			updateWalletCountHook: func() error {
				return types.ErrUpdatingInclusivity
			},
			tsCount:       1,
			expectedError: types.ErrUpdatingInclusivity,
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
			syncStatusSub := c.mux.Subscribe(utils.SyncStatusUpdate{})      // subscribe to tesseract added event

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tsAddedResp := make(chan result, 1)
			syncStatusResp := make(chan result, 1)

			go handleMuxEvents(ctx, tsAddedEventSub, tsAddedResp)  // keeps checking for event until timeout
			go handleMuxEvents(ctx, syncStatusSub, syncStatusResp) // keeps checking for event until timeout

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
			validateSyncStatusEvent(t, syncStatusResp)
		})
	}
}

func TestAddTesseractsWithState(t *testing.T) {
	var (
		accType   = types.LogicAccount
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
		dirtyStorage     map[types.Hash][]byte
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

			// add tesseracts to validated tesseracts so that we can verify if removed from validated tesseracts
			insertValidatedTesseracts(t, c, ts)

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
			checkForAddedTesseracts(t, true, c, ts, accType)
		})
	}
}

func TestAddSyncedTesseract(t *testing.T) {
	var (
		addresses   = getAddresses(t, 4)
		ixns        = createIxns(t, 2, getIxParamsMapWithAddresses(addresses[:2], addresses[2:]))
		clusterInfo = getTestClusterInfo(t, 2)
	)

	testcases := []struct {
		name             string
		clusterInfo      *ptypes.ICSClusterInfo
		tesseractsParams map[int]*createTesseractParams
		expectedError    error
	}{
		{
			name:             "should return error for nil cluster info",
			clusterInfo:      nil,
			tesseractsParams: map[int]*createTesseractParams{0: tesseractParamsWithICSClusterInfo(t, ixns, clusterInfo)},
			expectedError:    errors.New("nil cluster info"),
		},
		{
			name:             "valid cluster info should be added to db",
			clusterInfo:      clusterInfo,
			tesseractsParams: map[int]*createTesseractParams{0: tesseractParamsWithICSClusterInfo(t, ixns, clusterInfo)},
			expectedError:    nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseracts(t, 1, test.tesseractsParams)

			chainParams := &CreateChainParams{
				smCallBack: func(sm *MockStateManager) {
					setAccountType(sm, types.LogicAccount, ts...)
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.AddSyncedTesseract(test.clusterInfo, ts...)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForClusterInfo(t, c.db, clusterInfo)
			checkForAddedTesseracts(t, false, c, ts, types.LogicAccount)
		})
	}
}

func TestValidateTesseract(t *testing.T) {
	var sender id.KramaID

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
		ts                   *types.Tesseract
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
			expectedError: types.ErrAlreadyKnown,
		},
		{
			name: "should return error if tesseract has already been validated",
			chainManagerCallback: func(c *ChainManager) {
				c.validatedTesseracts.Store(getTesseractHash(t, ts[5]), nil)
			},
			ts:            ts[5],
			expectedError: types.ErrAlreadyKnown,
		},
		{
			name:          "invalid tesseract seal",
			ts:            ts[6],
			expectedError: types.ErrInvalidSeal,
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
			if !errors.Is(test.expectedError, types.ErrInvalidSeal) {
				sender = signTesseract(t, sm, test.ts)
			}

			err := c.validateTesseract(sender, test.ts, nodes)
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
		clusterInfo *ptypes.ICSClusterInfo,
		groupHash types.Hash,
	) *createTesseractParams {
		rawData, err := clusterInfo.Bytes()
		require.NoError(t, err)

		return &createTesseractParams{
			headerCallback: func(header *types.TesseractHeader) {
				header.PrevHash = tests.RandomHash(t)
				header.GroupHash = groupHash
				header.Extra = createCommitdataWithRandomGridHash(t)
			},
			bodyCallback: func(body *types.TesseractBody) {
				body.ConsensusProof.ICSHash = types.GetHash(rawData)
			},
		}
	}

	testcases := []struct {
		name             string
		dirtyStorage     map[types.Hash][]byte
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
					setAccountType(sm, types.LogicAccount, ts...)
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.AddTesseracts(
				ts,
				test.dirtyStorage,
			)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForAddedTesseracts(t, false, c, ts, types.LogicAccount)
		})
	}
}

func TestIsReceiptAndGroupHashValid(t *testing.T) {
	var receipts types.Receipts

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
				0: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash),
				1: tesseractParamsWithReceiptHash(t, tests.RandomHash(t), groupHash),
			},
			isValid: false,
		},
		{
			name: "grid hashes doesn't match",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash),
				1: tesseractParamsWithReceiptHash(t, receiptRoot, tests.RandomHash(t)),
			},
			isValid: false,
		},
		{
			name: "valid receipt hash and groupHash",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash),
				1: tesseractParamsWithReceiptHash(t, receiptRoot, groupHash),
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
	ixs, receipts := getIxAndReceiptsWithStateHash(t, map[types.Address]types.Hash{address: stateHash}, 1)

	testcases := []struct {
		name     string
		receipts types.Receipts
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
			params:   tesseractParamsWithStateHash(t, tests.RandomHash(t)),
			receipts: receipts,
			isValid:  false,
		},
		{
			name: "valid state hashes",
			params: &createTesseractParams{
				address: address,
				ixns:    ixs,
				bodyCallback: func(body *types.TesseractBody) {
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

			isValid := areStateHashesValid([]*types.Tesseract{ts}, receipts)

			require.Equal(t, test.isValid, isValid)
		})
	}
}

func TestExecuteAndValidate(t *testing.T) {
	var (
		tsParamsMap    = getTSParamsMapWithStateHash(t, 1)
		address        = tests.RandomAddress(t)
		stateHash      = tests.RandomHash(t)
		ixns, receipts = getIxAndReceiptsWithStateHash(t, map[types.Address]types.Hash{address: stateHash}, 1)
	)

	testcases := []struct {
		name                    string
		paramsMap               map[int]*createTesseractParams
		revertHook              func() error
		executeInteractionsHook func() (types.Receipts, error)
		expectedError           error
	}{
		{
			name:      "should return error if execution fails",
			paramsMap: tsParamsMap,
			executeInteractionsHook: func() (types.Receipts, error) {
				return nil, errors.New("failed to execute interactions")
			},
			expectedError: errors.New("failed to execute interactions"),
		},
		{
			name: "should return error if receipt validation fails",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithReceiptHash(t, tests.RandomHash(t), types.NilHash),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				var emptyReceipts types.Receipts

				return emptyReceipts, nil
			},
			expectedError: errors.New("failed to validate the tesseract"),
		},
		{
			name: "should return error if state hash validation fails",
			paramsMap: map[int]*createTesseractParams{
				0: tesseractParamsWithStateHash(t, tests.RandomHash(t)),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				var emptyReceipts types.Receipts

				return emptyReceipts, nil
			},
			expectedError: errors.New("failed to validate the tesseract"),
		},
		{
			name:      "should return error if revert fails",
			paramsMap: tsParamsMap,
			executeInteractionsHook: func() (types.Receipts, error) {
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
				0: tesseractParamsWithGridInfo(t, address, stateHash, getReceiptHash(t, receipts), nil, ixns, 1),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				return receipts, nil
			},
			expectedError: nil,
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

			err := c.executeAndValidate(ts)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, receipts, ts[0].Receipts())
		})
	}
}

func TestSendTesseractSyncRequest(t *testing.T) {
	tsParams := tesseractParamsWithContextDelta(
		t,
		tests.RandomAddress(t),
		1,
		3,
		0,
	)

	t.Run("should fire tesseract sync event with given tesseract", func(t *testing.T) {
		ts := createTesseract(t, tsParams)

		var (
			c       = createTestChainManager(t, nil)
			info    = getTestClusterInfo(t, 3)
			syncSub = c.mux.Subscribe(utils.TesseractSyncEvent{}) // subscribe to tesseract sync event

		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp := make(chan result, 1)

		go handleMuxEvents(ctx, syncSub, resp) // keeps checking for event until timeout

		c.sendTesseractSyncRequest(ts, info)

		validateTSSyncEvent(t, c, ts, resp, info)
	},
	)
}

func TestExecuteAndAdd(t *testing.T) {
	accType := types.LogicAccount
	address := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	ixns, receipts := getIxAndReceiptsWithStateHash(t, map[types.Address]types.Hash{address: stateHash}, 1)
	clusterInfo := getTestClusterInfo(t, 2)

	testcases := []struct {
		name                    string
		tsCount                 int
		clusterInfo             *ptypes.ICSClusterInfo
		executeInteractionsHook func() (types.Receipts, error)
		tesseractsParams        map[int]*createTesseractParams
		isTesseractCached       bool
		expectedError           error
	}{
		{
			name:              "should add tesseract to grid cache for pending group",
			clusterInfo:       clusterInfo,
			tsCount:           1,
			isTesseractCached: true,
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGridInfo(
					t,
					address,
					stateHash,
					getReceiptHash(t, receipts),
					clusterInfo,
					ixns,
					2,
				),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				return receipts, nil
			},
		},
		{
			name:        "should return error if addTesseractWithState fails",
			tsCount:     1,
			clusterInfo: getTestClusterInfo(t, 2),
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGridInfo(t, address, stateHash, getReceiptHash(t, receipts), clusterInfo, ixns, 1),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				return receipts, nil
			},
			expectedError: errors.New("cluster info not found"),
		},
		{
			name:        "should add tesseract if grid length is 1",
			tsCount:     1,
			clusterInfo: clusterInfo,
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGridInfo(t, address, stateHash, getReceiptHash(t, receipts), clusterInfo, ixns, 1),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				return receipts, nil
			},
		},
		{
			name:        "should return error if execution failed",
			tsCount:     1,
			clusterInfo: clusterInfo,
			tesseractsParams: map[int]*createTesseractParams{
				0: tesseractParamsWithGridInfo(t, address, stateHash, getReceiptHash(t, receipts), clusterInfo, ixns, 1),
			},
			executeInteractionsHook: func() (types.Receipts, error) {
				return nil, errors.New("execution failed")
			},
			expectedError: errors.New("execution failed"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tsCount := test.tsCount
			ts := createTesseracts(t, tsCount, test.tesseractsParams)

			db := mockDB()
			chainParams := &CreateChainParams{
				db: db,
				execCallback: func(exec *MockExec) {
					exec.executeInteractionsHook = test.executeInteractionsHook
				},
				smCallBack: func(sm *MockStateManager) {
					setAccountType(sm, accType, ts...)
				},
			}

			c := createTestChainManager(t, chainParams)

			err := c.executeAndAdd(ts[tsCount-1], test.clusterInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			// if the grid is incomplete check for the tesseract in grid cache
			if test.isTesseractCached {
				_, ok := c.groupCache.group[ts[0].GroupHash()]
				require.True(t, ok)

				added, err := c.db.HasTesseract(getTesseractHash(t, ts[0]))
				require.NoError(t, err)
				require.False(t, added)

				return
			}

			checkIfTesseractsAdded(
				t,
				accType,
				false,
				true,
				db,
				ts...,
			)
		})
	}
}

func TestAddTesseractWithOutState_SyncScenarios(t *testing.T) {
	var (
		respChan    = make(chan result, 1)
		ctxHash     = tests.RandomHash(t)
		address     = tests.RandomAddress(t)
		networkID   = tests.GetTestKramaIDs(t, 1)[0]
		kramaIDs    = tests.GetTestKramaIDs(t, 5)
		publicKeys  = getPublicKeys(t, 5)
		clusterInfo = getTestClusterInfoWithRandomSet(t, kramaIDs, 0)
	)

	testcases := []struct {
		name             string
		tesseractsParams *createTesseractParams
		selfID           id.KramaID
		clusterInfo      *ptypes.ICSClusterInfo
		preTestFn        func(c *ChainManager, ts *types.Tesseract)
		smCallBack       func(sm *MockStateManager)
		expectedError    error
	}{
		{
			name:             "node took part in ics, so shouldn't execute",
			clusterInfo:      clusterInfo,
			selfID:           kramaIDs[0], // this node should be part of ICS
			tesseractsParams: tesseractParamsWithContextHash(t, address, ctxHash, validCommitSign),
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(ctxHash, kramaIDs[:2], kramaIDs[2])
				sm.insertPublicKeys(kramaIDs, publicKeys)
			},
			preTestFn: func(c *ChainManager, ts *types.Tesseract) {
				networkID = signTesseract(t, c.sm.(*MockStateManager), ts) //nolint:forcetypeassert
			},
		},
		{
			name:             "should sync tesseract since handling node is not part in ics",
			clusterInfo:      clusterInfo,
			selfID:           tests.GetTestKramaIDs(t, 1)[0], // this node should not be part of ICS
			tesseractsParams: tesseractParamsWithContextHash(t, address, ctxHash, validCommitSign),
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(ctxHash, kramaIDs[:2], kramaIDs[2])
				sm.insertPublicKeys(kramaIDs, publicKeys)
			},
			preTestFn: func(c *ChainManager, ts *types.Tesseract) {
				networkID = signTesseract(t, c.sm.(*MockStateManager), ts) //nolint:forcetypeassert
			},
		},
		{
			name:             "should sync tesseract if should execute is set to false",
			clusterInfo:      clusterInfo,
			selfID:           tests.GetTestKramaIDs(t, 1)[0], // this node should not be part of ICS
			tesseractsParams: tesseractParamsWithContextHash(t, address, ctxHash, validCommitSign),
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(ctxHash, kramaIDs[:2], kramaIDs[2])
				sm.insertPublicKeys(kramaIDs, publicKeys)
			},
			preTestFn: func(c *ChainManager, ts *types.Tesseract) {
				networkID = signTesseract(t, c.sm.(*MockStateManager), ts) //nolint:forcetypeassert
				c.cfg.ShouldExecute = false
			},
		},
		{
			name:        "should return error if tesseract is already known",
			clusterInfo: clusterInfo,
			selfID:      networkID,
			tesseractsParams: &createTesseractParams{
				address: address,
			},
			preTestFn: func(c *ChainManager, ts *types.Tesseract) {
				insertTesseractsInDB(t, c.db, ts)
			},
			expectedError: types.ErrAlreadyKnown,
		},
		{
			name:        "should return error if ICSNodeSet fetch fails",
			clusterInfo: clusterInfo,
			selfID:      networkID,
			tesseractsParams: &createTesseractParams{
				address: address,
			},
			expectedError: errors.New("failed to fetch ICSNodeSet"),
		},
		{
			name:             "should return error if tesseract validation failed",
			clusterInfo:      clusterInfo,
			selfID:           networkID,
			tesseractsParams: tesseractParamsWithContextHash(t, address, ctxHash, invalidCommitSign),
			smCallBack: func(sm *MockStateManager) {
				sm.insertContextNodes(ctxHash, kramaIDs[:2], kramaIDs[2])
				sm.insertPublicKeys(kramaIDs, publicKeys)
			},
			expectedError: types.ErrInvalidSeal,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.tesseractsParams)

			chainParams := &CreateChainParams{
				db:         mockDB(),
				sm:         mockStateManager(),
				smCallBack: test.smCallBack,
				networkCallback: func(network *MockNetwork) {
					network.kramaID = test.selfID
				},
			}

			c := createTestChainManager(t, chainParams)

			if test.preTestFn != nil {
				test.preTestFn(c, ts)
			}

			syncSub := c.mux.Subscribe(utils.TesseractSyncEvent{})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			go handleMuxEvents(ctx, syncSub, respChan)

			err := c.AddTesseractWithOutState(ts, networkID, test.clusterInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			validateTSSyncEvent(t, c, ts, respChan, test.clusterInfo)
		})
	}
}

func TestAddTesseractWithOutState_ExecuteScenarios(t *testing.T) {
	var (
		address        = tests.RandomAddress(t)
		accType        = types.LogicAccount
		stateHash      = tests.RandomHash(t)
		contextHash    = tests.RandomHash(t)
		kramaIDs       = tests.GetTestKramaIDs(t, 5)
		publicKeys     = getPublicKeys(t, 5)
		networkID      = tests.GetTestKramaIDs(t, 1)[0]
		ixns, receipts = getIxAndReceiptsWithStateHash(t, map[types.Address]types.Hash{address: stateHash}, 1)
		clusterInfo    = getTestClusterInfoWithRandomSet(t, kramaIDs[2:], 1)
	)

	tesseractsParams := tesseractParamsForExecution(
		t,
		address,
		contextHash,
		stateHash,
		ixns,
		receipts,
		clusterInfo,
		1,
	)

	ts := createTesseract(t, tesseractsParams)

	db := mockDB()
	sm := mockStateManager()

	chainParams := &CreateChainParams{
		db: db,
		sm: sm,
		networkCallback: func(network *MockNetwork) {
			network.kramaID = networkID
		},
		execCallback: func(exec *MockExec) {
			exec.executeInteractionsHook = func() (types.Receipts, error) {
				return receipts, nil
			}
		},
		smCallBack: func(sm *MockStateManager) {
			sm.insertContextNodes(contextHash, kramaIDs[:2], kramaIDs[2])
			sm.insertPublicKeys(kramaIDs, publicKeys)
			sm.insertRegisteredAcc(address)
			sm.setAccType(address, accType)
		},
		chainManagerCallback: func(c *ChainManager) {
			c.cfg.ShouldExecute = true
		},
	}

	c := createTestChainManager(t, chainParams)

	sender := signTesseract(t, sm, ts)

	err := c.AddTesseractWithOutState(ts, sender, clusterInfo)
	require.NoError(t, err)

	// check if acc meta info inserted
	checkIfTesseractsAdded(
		t,
		accType,
		false,
		true,
		db,
		ts,
	)
}

func TestTesseractHandler_SkipScenarios(t *testing.T) {
	var (
		nodes        = tests.GetTestKramaIDs(t, 2)
		chainManager = createTestChainManager(t, &CreateChainParams{
			networkCallback: func(network *MockNetwork) {
				network.kramaID = nodes[0]
			},
		})
	)

	testcases := []struct {
		name            string
		tesseractParams *createTesseractParams
		info            *ptypes.ICSClusterInfo
		preTestFn       func(c *ChainManager, ts *types.Tesseract)
		expectedError   error
	}{
		{
			name: "should skip tesseract if the handling node is operator",
			tesseractParams: &createTesseractParams{
				headerCallback: func(header *types.TesseractHeader) {
					header.Operator = string(nodes[0])
				},
			},
			expectedError: errors.New("node is the operator of tesseract"),
		},
		{
			name: "should return error if AddTesseractWithOutState fails",
			info: getTestClusterInfo(t, 2),
			// passing an invalid clusterInfo
			tesseractParams: tesseractParamsWithICSClusterInfo(t, nil, getTestClusterInfo(t, 1)),
			expectedError:   errors.New("failed to fetch ICSNodeSet"),
		},
		{
			name: "should skip if tesseract is already available in db",
			preTestFn: func(c *ChainManager, ts *types.Tesseract) {
				insertTesseractsInDB(t, c.db, ts)
			},
			expectedError: types.ErrAlreadyKnown,
		},
		{
			name: "should skip if tesseract is already available in cache",
			info: getTestClusterInfo(t, 2),
			tesseractParams: &createTesseractParams{
				bodyCallback: func(body *types.TesseractBody) {
					body.ConsensusProof.ICSHash = tests.RandomHash(t)
				},
			},
			preTestFn: func(c *ChainManager, ts *types.Tesseract) {
				c.knownTesseracts.Add(ts.Hash())
			},
			expectedError: types.ErrAlreadyKnown,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := createTesseract(t, test.tesseractParams)
			msg := getPubSubMsg(t, ts, nodes[0], test.info)

			if test.preTestFn != nil {
				test.preTestFn(chainManager, ts)
			}

			err := chainManager.tesseractHandler(msg)
			require.ErrorContains(t, err, test.expectedError.Error())
		})
	}
}

func TestValidateAccountCreationInfo(t *testing.T) {
	var (
		c             = createTestChainManager(t, nil)
		walletContext = utils.KramaIDToString(tests.GetTestKramaIDs(t, 4))
		address       = tests.RandomAddress(t).Hex()
	)

	testcases := []struct {
		name          string
		accountInfo   AccountInfo
		expectedError error
	}{
		{
			name: "should fail for invalid address",
			accountInfo: getAccountInfo(
				t,
				"0xa6ba9853f131679d0da0f033516a3efe9cd53c3d54e1f9a6e60e9077e9f9384",
				types.LogicAccount,
				"moi-id",
				walletContext[:2],
				walletContext[2:4],
				[]*AssetInfo{},
				[]*BalanceInfo{},
			),
			expectedError: types.ErrInvalidAddress,
		},
		{
			name: "should fail for invalid account type",
			accountInfo: getAccountInfo(
				t,
				address,
				tests.InvalidAccount,
				"moi-id",
				walletContext[:2],
				walletContext[2:4],
				[]*AssetInfo{},
				[]*BalanceInfo{},
			),
			expectedError: types.ErrInvalidAccountType,
		},
		{
			name: "should fail for invalid asset id",
			accountInfo: getAccountInfo(
				t,
				address,
				types.LogicAccount,
				"moi-id",
				walletContext[:2],
				walletContext[2:4],
				[]*AssetInfo{},
				[]*BalanceInfo{
					getBalanceInfo(
						"0xa6ba9853f131679d0da0f033516a2efe9cd53c3d54e1f9a6e60e977e9f9384",
						1000,
					),
				},
			),
			expectedError: types.ErrInvalidAssetID,
		},
		{
			name: " should fail for invalid balance",
			accountInfo: getAccountInfo(
				t,
				address,
				types.LogicAccount,
				"moi-id",
				walletContext[:2],
				walletContext[2:4],
				[]*AssetInfo{},
				[]*BalanceInfo{
					getBalanceInfo(
						"0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
						-1,
					),
				},
			),
			expectedError: errors.New("invalid balance"),
		},
		{
			name: "should fail for invalid asset details",
			accountInfo: getAccountInfo(
				t,
				address,
				types.LogicAccount,
				"moi-id",
				walletContext[:2],
				walletContext[2:4],
				[]*AssetInfo{
					getAssetInfo(t, "btc", 4),
				},
				[]*BalanceInfo{},
			),
			expectedError: errors.New("invalid asset details"),
		},
		{
			name:        "should pass for valid asset info",
			accountInfo: getTestAccountWithAccType(t, types.LogicAccount),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accSetupArgs, err := c.validateAccountCreationInfo(test.accountInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForAccountCreation(t, test.accountInfo, accSetupArgs)
		})
	}
}

func TestAddGenesisTesseract(t *testing.T) {
	var (
		address      = tests.RandomAddress(t)
		stateHash    = tests.RandomHash(t)
		contextHash  = tests.RandomHash(t)
		accType      = types.LogicAccount
		contextDelta = map[types.Address]*types.DeltaGroup{
			address: getDeltaGroup(t, 2, 5, 3),
		}
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

			err := c.AddGenesisTesseract(address, stateHash, contextHash, contextDelta)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForGenesisTesseract(
				t,
				c,
				address,
				stateHash,
				contextHash,
				contextDelta,
			)
		},
		)
	}
}

func TestParseGenesisFile_InvalidPath(t *testing.T) {
	c := createTestChainManager(t, nil)
	_, _, _, err := c.ParseGenesisFile("test2/3647663731config.json")

	require.ErrorContains(t, err, "failed to open genesis file")
}

func TestParseGenesisFile(t *testing.T) {
	c := createTestChainManager(t, nil)

	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	testcases := []struct {
		name              string
		path              string
		sargaAccount      AccountInfo
		genesisAccounts   []AccountInfo
		genesisLogics     []GenesisLogic
		invalidAccPayload bool
		expectedError     error
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
			genesisAccounts: []AccountInfo{},
			expectedError:   errors.New("invalid sarga account info"),
		},
		{
			name:         "should return error if genesis account info is invalid",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []AccountInfo{
				getTestAccountWithAccType(t, tests.InvalidAccount),
			},
			expectedError: errors.New("invalid genesis account info"),
		},
		{
			name:         "should succeed for valid genesis data without contract paths",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []AccountInfo{
				getTestAccountWithAccType(t, types.RegularAccount),
				getTestAccountWithAccType(t, types.LogicAccount),
			},
		},
		{
			name:         "should succeed for valid genesis data with contract paths",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []AccountInfo{
				getTestAccountWithAccType(t, types.RegularAccount),
				getTestAccountWithAccType(t, types.LogicAccount),
			},
			genesisLogics: generateTestGenesisLogics(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			path := createMockGenesisFile(t, dir, test.invalidAccPayload, test.sargaAccount,
				test.genesisAccounts, test.genesisLogics)

			sargaAccount, genesisAccounts, genesisLogics, err := c.ParseGenesisFile(path)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}
			require.NoError(t, err)

			checkForAccountCreation(t, test.sargaAccount, sargaAccount)

			for i, genesisAccount := range test.genesisAccounts {
				checkForAccountCreation(t, genesisAccount, genesisAccounts[i]) // TODO: rename funcs
			}

			require.Equal(t, len(test.genesisLogics), len(genesisLogics))

			validateGenesisLogics(t, test.genesisLogics, genesisLogics)
		})
	}
}

func TestSetupGenesis(t *testing.T) {
	var (
		sargaAccount    = getTestAccountWithAccType(t, types.SargaAccount)
		sargaAddress    = types.HexToAddress(sargaAccount.Address)
		genesisAccounts = []AccountInfo{
			getTestAccountWithAccType(t, types.RegularAccount),
			getTestAccountWithAccType(t, types.LogicAccount),
		}
		sargaAccountHashes = AccHash{
			contextHash: tests.RandomHash(t),
			stateHash:   tests.RandomHash(t),
		}
		genesisAccountHashes = AccHash{
			contextHash: tests.RandomHash(t),
			stateHash:   tests.RandomHash(t),
		}
	)

	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	path := createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts, nil)

	testcases := []struct {
		name          string
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "should return error if failed to add genesis tesseract",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(sargaAddress, sargaAccount.AccountType)
			},
			expectedError: errors.New("error adding genesis tesseract"),
		},
		{
			name: "should succeed for valid genesis info",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(sargaAddress, sargaAccount.AccountType)
				for _, genesisAccount := range genesisAccounts {
					sm.setAccType(types.HexToAddress(genesisAccount.Address), genesisAccount.AccountType)
				}

				sm.newAccountHook = func() (types.Hash, types.Hash, error) {
					return genesisAccountHashes.stateHash, genesisAccountHashes.contextHash, nil
				}
			},
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

			err = c.SetupGenesis(path)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}
			require.NoError(t, err)

			checkForGenesisTesseract(
				t,
				c,
				sargaAddress,
				sargaAccountHashes.stateHash,
				sargaAccountHashes.contextHash,
				map[types.Address]*types.DeltaGroup{
					sargaAddress: {
						BehaviouralNodes: utils.KramaIDFromString(sargaAccount.BehaviourContext),
						RandomNodes:      utils.KramaIDFromString(sargaAccount.RandomContext),
					},
				},
			)

			for _, genesisAccount := range genesisAccounts {
				genesisAddress := types.HexToAddress(genesisAccount.Address)

				checkForGenesisTesseract(
					t,
					c,
					genesisAddress,
					genesisAccountHashes.stateHash,
					genesisAccountHashes.contextHash,
					map[types.Address]*types.DeltaGroup{
						genesisAddress: {
							BehaviouralNodes: utils.KramaIDFromString(genesisAccount.BehaviourContext),
							RandomNodes:      utils.KramaIDFromString(genesisAccount.RandomContext),
						},
					},
				)
			}
		})
	}
}

func getAccountSetupArgs(
	t *testing.T,
	address types.Address,
	behNodes []id.KramaID,
	randNodes []id.KramaID,
	accType int,
	assetDetails []*types.AssetDescriptor,
	balanceInfo map[types.AssetID]*big.Int,
) *gtypes.AccountSetupArgs {
	t.Helper()

	return &gtypes.AccountSetupArgs{
		Address:            address,
		MoiID:              tests.RandomAddress(t).Hex(),
		BehaviouralContext: behNodes,
		RandomContext:      randNodes,
		AccType:            types.AccountType(accType),
		Assets:             assetDetails,
		Balances:           balanceInfo,
	}
}

func TestSetupSargaAcc(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.GetTestKramaIDs(t, 12)

	var emptyNodes []id.KramaID

	testcases := []struct {
		name          string
		sarga         *gtypes.AccountSetupArgs
		accounts      []*gtypes.AccountSetupArgs
		logics        []GenesisLogic
		expectedError error
	}{
		{
			name: "behavioural nodes and random nodes are empty",
			sarga: getAccountSetupArgs(t,
				types.SargaAddress,
				emptyNodes,
				emptyNodes,
				2,
				nil,
				nil,
			),
			expectedError: errors.New("context initiation failed in genesis"),
		},
		{
			name: "invalid sarga account address",
			sarga: getAccountSetupArgs(t,
				tests.RandomAddress(t),
				emptyNodes,
				emptyNodes,
				2,
				nil,
				nil,
			),
			expectedError: errors.New("invalid sarga account address"),
		},
		{
			name: "other accounts added to sarga account",
			sarga: getAccountSetupArgs(t,
				types.SargaAddress,
				nodes[:2],
				nodes[2:4],
				2,
				nil,
				nil,
			),
			accounts: []*gtypes.AccountSetupArgs{
				getAccountSetupArgs(t,
					tests.RandomAddress(t),
					nodes[4:6],
					nodes[6:8],
					3,
					nil,
					nil,
				),
				getAccountSetupArgs(t,
					tests.RandomAddress(t),
					nodes[8:10],
					nodes[10:12],
					9,
					nil,
					nil,
				),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, contextHash, err := cm.SetupSargaAccount(test.sarga, test.accounts, test.logics)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateObjectCreation(t, cm.sm, types.SargaAddress, contextHash)

			obj, _ := cm.sm.GetDirtyObject(types.SargaAddress)

			checkSargaObjectAccounts(t, obj, test.accounts)
		})
	}
}

func TestSetupNewAccount(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.GetTestKramaIDs(t, 12)

	var emptyNodes []id.KramaID

	testcases := []struct {
		name          string
		newAcc        *gtypes.AccountSetupArgs
		expectedError error
	}{
		{
			name: "behavioural nodes and random nodes are empty",
			newAcc: getAccountSetupArgs(t,
				tests.RandomAddress(t),
				emptyNodes,
				emptyNodes,
				3,
				nil,
				nil,
			),
			expectedError: errors.New("context initiation failed in genesis"),
		},
		{
			name: "account with assets and balances",
			newAcc: getAccountSetupArgs(t,
				tests.RandomAddress(t),
				nodes[:2],
				nodes[2:4],
				2,
				[]*types.AssetDescriptor{
					getAsset(
						1,
						10000,
						"MOI",
						true,
						true,
					),
					getAsset(
						1,
						10000,
						"BTC",
						true,
						true,
					),
				},
				map[types.AssetID]*big.Int{
					"MOI": big.NewInt(12000),
					"BTC": big.NewInt(18000),
				},
			),
		},
		{
			name: "account without assets and balances",
			newAcc: getAccountSetupArgs(t,
				tests.RandomAddress(t),
				nodes[:2],
				nodes[2:4],
				2,
				make([]*types.AssetDescriptor, 0),
				make(map[types.AssetID]*big.Int),
			),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, contextHash, err := cm.SetupNewAccount(test.newAcc)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateObjectCreation(t, cm.sm, test.newAcc.Address, contextHash)

			obj, _ := cm.sm.GetDirtyObject(test.newAcc.Address)

			// check if balances are added
			for assetID, balance := range test.newAcc.Balances {
				bal, err := obj.BalanceOf(assetID)
				require.NoError(t, err)

				require.Equal(t, bal, balance)
			}

			journalIndex := 3 // index is 3 as there will be 3 entries before asset creation
			for _, asset := range test.newAcc.Assets {
				CheckAssetCreation(t, obj, asset)
				journalIndex++
			}
		})
	}
}

func TestExecuteGenesisContracts(t *testing.T) {
	logicID, err := types.NewLogicIDv0(true, false, false, false, 0, types.StakingContractAddr)
	require.NoError(t, err)

	ids := tests.GetTestKramaIDs(t, 1)
	nodes := utils.KramaIDToString(ids)

	testcases := []struct {
		name          string
		logics        []GenesisLogic
		smCallBack    func(sm *MockStateManager)
		expectedError string
	}{
		{
			name: "invalid logic name",
			logics: []GenesisLogic{{
				Name:               "test",
				BehaviouralContext: nodes,
			}},
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), types.LogicAccount)
			},
			expectedError: "generated address does not exist",
		},
		{
			name: "missing behaviour context",
			logics: []GenesisLogic{{
				Name:               "staking-contract",
				BehaviouralContext: nil,
			}},
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), types.LogicAccount)
			},
			expectedError: "context initiation failed in genesis",
		},
		{
			name: "valid contract paths",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(logicID.Address(), types.LogicAccount)
			},
			logics: generateTestGenesisLogics(t),
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
			_, err := cm.ExecuteGenesisLogics(test.logics)

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func CheckAssetCreation(
	t *testing.T,
	s *guna.StateObject,
	assetDescriptor *types.AssetDescriptor,
) {
	t.Helper()

	expectedAssetID, expectedAssetHash, expectedData, err := getTestAssetID(assetDescriptor)
	require.NoError(t, err)

	actualData, err := s.GetDirtyEntry(expectedAssetHash.String()) // check if asset data inserted in dirty entries
	require.NoError(t, err)
	require.Equal(t, expectedData, actualData)

	actualSupply, err := s.BalanceOf(expectedAssetID)
	require.NoError(t, err)
	require.Equal(t, assetDescriptor.Supply, actualSupply) // check total supply is stored in balances

	require.Equal(t, s.Address(), assetDescriptor.Owner) // check if address is assigned to owner
}

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
		gridHash      types.Hash
		expectedParts *types.TesseractParts
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
			[]types.Address{tests.RandomAddress(t)},
			[]types.Address{tests.RandomAddress(t)},
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
		gridHash             types.Hash
		expectedInteractions types.Interactions
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
			expectedError: types.ErrFetchingInteractions,
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
			[]types.Address{tests.RandomAddress(t)},
			[]types.Address{tests.RandomAddress(t)},
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
		tsHash               types.Hash
		ixIndex              int
		expectedInteractions *types.Interaction
		expectedParts        *types.TesseractParts
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
			expectedError: types.ErrGridHashNotFound,
		},
		{
			name:          "interactions not found",
			tsHash:        tsHashWithoutIxns,
			expectedError: types.ErrFetchingInteractions,
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
			expectedError: types.ErrIndexOutOfRange,
		},
		{
			name:          "interaction index cannot be negative",
			tsHash:        tsHash,
			ixIndex:       -1,
			expectedError: types.ErrIndexOutOfRange,
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
		[]types.Address{tests.RandomAddress(t)},
		[]types.Address{tests.RandomAddress(t)},
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
		ixHash               types.Hash
		expectedInteractions *types.Interaction
		expectedParts        *types.TesseractParts
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
			expectedError: types.ErrGridHashNotFound,
		},
		{
			name:          "interactions not found",
			ixHash:        ixHashWithoutIxns,
			expectedError: types.ErrFetchingInteractions,
		},
		{
			name:          "interaction not found",
			ixHash:        ixHashWithoutIxn,
			expectedError: types.ErrFetchingInteraction,
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

package lattice

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sarvalabs/moichain/guna"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common/tests"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
	"github.com/stretchr/testify/require"
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

			ics, err := c.FetchICSNodeSet(ts, info)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfICSNodeSetMatches(
				t,
				ics,
				types.NewNodeSet(test.behNodes, test.behPK),
				types.NewNodeSet(test.randNodes, test.randPK),
				types.NewNodeSet(test.randSet, test.randsetPK),
			)
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
			expectedError: errors.New("error fetching previous tesseract"),
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
		clusterInfo *types.ICSClusterInfo,
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

			err := c.AddTesseracts(test.dirtyStorage, ts...)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForAddedTesseracts(t, c, ts, types.LogicAccount)
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

			err := c.ExecuteAndValidate(ts...)
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
		accountInfo   *types.AccountSetupArgs
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
			expectedError: types.ErrInvalidAccountType,
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
		accType     = types.LogicAccount
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
				stateHash,
				contextHash,
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
	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	testcases := []struct {
		name                 string
		path                 string
		sargaAccount         types.AccountSetupArgs
		genesisAccounts      []types.AccountSetupArgs
		genesisLogics        []types.GenesisLogic
		genesisAssetAccounts []types.AssetAccountSetupArgs
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
			genesisAccounts: []types.AccountSetupArgs{},
			expectedError:   errors.New("invalid sarga account info"),
		},
		{
			name:         "should return error if genesis account info is invalid",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []types.AccountSetupArgs{
				getTestAccountWithAccType(t, tests.InvalidAccount),
			},
			expectedError: errors.New("invalid genesis account creation info"),
		},
		{
			name:         "should succeed for valid genesis data without contract paths",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []types.AccountSetupArgs{
				getTestAccountWithAccType(t, types.RegularAccount),
				getTestAccountWithAccType(t, types.LogicAccount),
			},
		},
		{
			name:         "should succeed for valid genesis data with contract paths",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []types.AccountSetupArgs{
				getTestAccountWithAccType(t, types.RegularAccount),
				getTestAccountWithAccType(t, types.LogicAccount),
			},
			genesisLogics: getTestGenesisLogics(t),
		},
		{
			name:         "should succeed for valid genesis asset account with invalid allocations",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []types.AccountSetupArgs{
				getTestAccountWithAccType(t, types.RegularAccount),
				getTestAccountWithAccType(t, types.LogicAccount),
			},
			genesisAssetAccounts: []types.AssetAccountSetupArgs{
				// using nilAddress for allocations
				getTestAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, types.NilAddress)),
			},
		},
		{
			name:         "should succeed for valid genesis asset account with valid allocations",
			sargaAccount: getTestAccountWithAccType(t, types.SargaAccount),
			genesisAccounts: []types.AccountSetupArgs{
				getTestAccountWithAddress(t, addr),
				getTestAccountWithAccType(t, types.LogicAccount),
			},
			genesisAssetAccounts: []types.AssetAccountSetupArgs{
				getTestAssetAccountSetupArgs(t, getTestAssetCreationArgs(t, addr)),
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
		sargaAccount    = getTestAccountWithAccType(t, types.SargaAccount)
		genesisAccounts = []types.AccountSetupArgs{
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

	path := createMockGenesisFile(t, dir, false, sargaAccount, genesisAccounts, nil, nil)

	testcases := []struct {
		name          string
		smCallBack    func(sm *MockStateManager)
		expectedError error
	}{
		{
			name: "should return error if failed to add genesis tesseract",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(types.SargaAddress, sargaAccount.AccType)
			},
			expectedError: errors.New("error adding genesis tesseract"),
		},
		{
			name: "should succeed for valid genesis info",
			smCallBack: func(sm *MockStateManager) {
				sm.setAccType(types.SargaAddress, sargaAccount.AccType)
				for _, genesisAccount := range genesisAccounts {
					sm.setAccType(genesisAccount.Address, genesisAccount.AccType)
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
				types.SargaAddress,
				sargaAccountHashes.stateHash,
				sargaAccountHashes.contextHash,
			)

			for _, genesisAccount := range genesisAccounts {
				checkForGenesisTesseract(
					t,
					c,
					genesisAccount.Address,
					genesisAccountHashes.stateHash,
					genesisAccountHashes.contextHash,
				)
			}
		})
	}
}

func getAccountSetupArgs(
	t *testing.T,
	address types.Address,
	accType types.AccountType,
	moiID string,
	behNodes []id.KramaID,
	randNodes []id.KramaID,
) *types.AccountSetupArgs {
	t.Helper()

	return &types.AccountSetupArgs{
		Address:            address,
		MoiID:              moiID,
		BehaviouralContext: behNodes,
		RandomContext:      randNodes,
		AccType:            accType,
	}
}

func TestSetupSargaAcc(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.GetTestKramaIDs(t, 12)

	var emptyNodes []id.KramaID

	testcases := []struct {
		name          string
		sarga         *types.AccountSetupArgs
		accounts      []types.AccountSetupArgs
		expectedError error
	}{
		{
			name: "behavioural nodes and random nodes are empty",
			sarga: getAccountSetupArgs(t,
				types.SargaAddress,
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
				types.SargaAddress,
				2,
				"moi-id",
				nodes[:2],
				nodes[2:4],
			),
			accounts: []types.AccountSetupArgs{
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
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			obj, err := cm.SetupSargaAccount(test.sarga, test.accounts, nil, nil)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateContextInitialization(
				t,
				cm.sm,
				types.SargaAddress,
				test.sarga.BehaviouralContext,
				test.sarga.RandomContext,
				obj.ContextHash(),
			)

			obj, _ = cm.sm.GetDirtyObject(types.SargaAddress)

			checkSargaObjectAccounts(t, obj, test.accounts)
		})
	}
}

func TestSetupNewAccount(t *testing.T) {
	cm := createTestChainManager(t, nil)
	nodes := tests.GetTestKramaIDs(t, 12)

	testcases := []struct {
		name               string
		newAcc             *types.AccountSetupArgs
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
	logicID := types.NewLogicIDv0(true, false, false, false, 0, types.StakingContractAddr)

	ids := tests.GetTestKramaIDs(t, 1)
	nodes := utils.KramaIDToString(ids)
	objectsMap := make(map[types.Address]*guna.StateObject)
	testcases := []struct {
		name          string
		logics        []types.GenesisLogic
		smCallBack    func(sm *MockStateManager)
		expectedError string
	}{
		{
			name: "invalid logic name",
			logics: []types.GenesisLogic{{
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
			logics: []types.GenesisLogic{{
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
			logics: getTestGenesisLogics(t),
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
//	s *guna.StateObject,
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

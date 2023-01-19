package guna

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/guna/tree"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common/tests"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/require"
)

func TestBalanceOf(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	balance := big.NewInt(123)
	sObj.setBalance(assetID, balance)

	testcases := []struct {
		name          string
		assetID       types.AssetID
		expectedError error
	}{
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: types.ErrAssetNotFound,
		},
		{
			name:    "fetched balance successfully",
			assetID: assetID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualBalance, err := sObj.BalanceOf(test.assetID)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, balance, actualBalance)
		})
	}
}

func TestAddBalance(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	sObj.setBalance(assetID, big.NewInt(123))

	testcases := []struct {
		name             string
		assetID          types.AssetID
		BalanceToBeAdded *big.Int
		expectedBalance  *big.Int
	}{
		{
			name:             "balance gets added if asset already exists",
			assetID:          assetID,
			BalanceToBeAdded: big.NewInt(123),
			expectedBalance:  big.NewInt(246),
		},
		{
			name:             "balance gets initialized if asset doesn't exist",
			assetID:          tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			BalanceToBeAdded: big.NewInt(123),
			expectedBalance:  big.NewInt(123),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj.AddBalance(test.assetID, test.BalanceToBeAdded)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID)
		})
	}
}

func TestSubBalance(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	sObj.setBalance(assetID, big.NewInt(124))

	sObj.SubBalance(assetID, big.NewInt(123))
	checkForBalances(t, sObj, big.NewInt(1), assetID)
}

func TestSetBalance(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	balance := big.NewInt(123)

	sObj.setBalance(assetID, balance)

	actualBalance, err := sObj.BalanceOf(assetID)
	require.NoError(t, err)
	require.Equal(t, balance, actualBalance)
}

func TestCopy(t *testing.T) {
	testcases := []struct {
		name        string
		soParams    *createStateObjectParams
		areTreesNil bool
	}{
		{
			name:     "copied whole state object",
			soParams: stateObjectParamsWithTestData(t, false),
		},
		{
			name:        "logic tree and meta storage tree are not copied",
			soParams:    stateObjectParamsWithTestData(t, true),
			areTreesNil: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			expectedSO := getCopiedStateObject(sObj)
			copiedSO := sObj.Copy()

			checkIfStateObjectAreEqual(t, expectedSO, copiedSO, test.areTreesNil)
		})
	}
}

func TestCommitBalanceObject(t *testing.T) {
	balance, balanceHash := getTestBalance(t, getAssetMap(getAssetIDsAndBalances(t, 2)))
	sObj := createTestStateObject(t, stateObjectParamsWithBalance(t, balanceHash, balance))

	actualBalanceHash, err := sObj.commitBalanceObject()
	require.NoError(t, err)

	checkForBalance(t, sObj, balance, types.BytesToHash(actualBalanceHash), 0)
}

func TestCommitAccount(t *testing.T) {
	inputAcc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
		acc.ContextHash = tests.RandomHash(t)
	})

	sObj := createTestStateObject(t, &createStateObjectParams{
		account: inputAcc,
	})

	actualAccHash, err := sObj.commitAccount()
	require.NoError(t, err)

	inputAcc.Nonce = 1 // change expected account nonce to 1 as it needs to be incremented

	checkForAccount(t, sObj, inputAcc, actualAccHash, 0)
}

func TestCommitContext(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ctxObject, _ := getContextObjects(t, tests.GetTestKramaIDs(t, 2), 2, 1)

	actualCtxHash, err := sObj.commitContextObject(ctxObject[0])
	require.NoError(t, err)

	checkForContextObject(t, sObj, *ctxObject[0], actualCtxHash)
}

func TestCommitActiveStorageTrees(t *testing.T) {
	logicIDs := getLogicIDs(t, 1)
	keys, values := getEntries(t, 1)

	astWithoutDirtyEntries := getASTWithDefaultFlushedEntries(t, 1, 1)
	astWithDirtyEntries := getASTWithDefaultDirtyEntries(t, 2, 1)

	mst := mockMerkleTreeWithDB()

	testcases := []struct {
		name          string
		ast           map[string]tree.MerkleTree
		mst           *MockMerkleTree
		expectedError error
	}{
		{
			name: "committed active storage trees successfully",
			ast:  astWithDirtyEntries,
			mst:  mst,
		},
		{
			name: "can not commit active storage tree without dirty entries",
			ast:  astWithoutDirtyEntries,
			mst:  mst,
		},
		{
			name: "should return error if failed to commit active storage trees",
			ast: getActiveStorageTreesWithCommitHook(
				t,
				logicIDs,
				keys,
				values,
				func() error {
					return errors.New("failed to commit active storage trees")
				},
			),
			mst:           mst,
			expectedError: errors.New("failed to commit active storage trees"),
		},
		{
			name: "should return error if failed to calculate root hash",
			ast: getActiveStorageTreesWithRootHook(
				t,
				logicIDs,
				keys,
				values,
				func() (types.Hash, error) {
					return types.NilHash, errors.New("failed to calculate root for ast")
				},
			),
			mst:           mst,
			expectedError: errors.New("failed to calculate root for ast"),
		},
		{
			name: "should return error if failed to set entries in mst",
			ast:  astWithDirtyEntries,
			mst: getMerkleTreeWithSetHook(
				t,
				keys,
				values,
				func() error {
					return errors.New("failed to set entries in mst")
				},
			),
			expectedError: errors.New("failed to set entries in mst"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			inputAST := getCopiedAST(test.ast)
			sObj := createTestStateObject(t, stateObjectParamsWithASTAndMST(t, test.ast, test.mst))

			err := sObj.commitActiveStorageTrees()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfActiveStorageTreesAreCommitted(t, inputAST, sObj)
		})
	}
}

func TestCommitMetaStorageTree(t *testing.T) {
	keys, values := getEntries(t, 1)

	mstWithDirtyEntries := getMerkleTreeWithDefaultDirtyEntries(t, 1)

	testcases := []struct {
		name          string
		mst           tree.MerkleTree
		storageRoot   types.Hash
		expectedError error
	}{
		{
			name:        "mst doesn't get committed as it doesn't have dirty entries",
			mst:         getMerkleTreeWithFlushedEntries(t, keys, values),
			storageRoot: tests.RandomHash(t),
		},
		{
			name: "committed meta storage tree successfully",
			mst:  mstWithDirtyEntries,
		},
		{
			name: "should return error if failed to commit meta storage tree",
			mst: getMerkleTreeWithCommitHook(t,
				keys,
				values,
				func() error {
					return errors.New("failed to commit mst")
				},
			),
			expectedError: errors.New("failed to commit mst"),
		},
		{
			name: "should return error if failed to calculate root hash",
			mst: getMerkleTreeWithRootHashHook(t,
				keys,
				values,
				func() (types.Hash, error) {
					return types.NilHash, errors.New("failed to calculate mst root")
				},
			),
			expectedError: errors.New("failed to calculate mst root"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			inputMST := test.mst.Copy()
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithMST(t, types.NilAddress, nil, test.mst, test.storageRoot),
			)

			actualRootHash, err := sObj.commitMetaStorageTree()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfMetaStorageTreeCommitted(t, inputMST, sObj, actualRootHash, 0)
		})
	}
}

func TestCommitStorage(t *testing.T) {
	logicIds := getLogicIDs(t, 1)
	keys, values := getEntries(t, 1)

	astWithDirtyEntries := getASTWithDefaultDirtyEntries(t, 2, 1)
	inputAST := getCopiedAST(astWithDirtyEntries)

	mstWithDirtyEntries := getMerkleTreeWithDefaultDirtyEntries(t, 1)

	inputMST := mstWithDirtyEntries.Copy()
	astWithCommitHook := getActiveStorageTreesWithCommitHook(
		t,
		logicIds,
		keys,
		values,
		func() error {
			return errors.New("failed to commit active storage trees")
		},
	)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		isMSTNil      bool
		expectedError error
	}{
		{
			name:     "meta storage tree is nil",
			soParams: stateObjectParamsWithASTAndMST(t, astWithDirtyEntries, nil),
			isMSTNil: true,
		},
		{
			name:          "should return error if failed to commit active storage trees",
			soParams:      stateObjectParamsWithAST(t, astWithCommitHook),
			expectedError: errors.New("failed to commit active storage trees"),
		},
		{
			name:     "storage committed successfully",
			soParams: stateObjectParamsWithASTAndMST(t, astWithDirtyEntries, mstWithDirtyEntries),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualRootHash, err := sObj.commitStorage()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if test.isMSTNil {
				require.Equal(t, sObj.data.StorageRoot, actualRootHash)

				return
			}

			checkIfActiveStorageTreesAreCommitted(t, inputAST, sObj)
			checkIfMetaStorageTreeCommitted(t, inputMST, sObj, actualRootHash, 0)
		})
	}
}

func TestCommitLogicsTree(t *testing.T) {
	keys, values := getEntries(t, 1)
	logicTree := getMerkleTreeWithFlushedEntries(t, keys, values)
	inputLogicTree := logicTree.Copy()

	testcases := []struct {
		name          string
		logicTree     tree.MerkleTree
		logicRoot     types.Hash
		expectedError error
	}{
		{
			name:      "logic tree is nil",
			logicRoot: tests.RandomHash(t),
		},
		{
			name:      "committed logic tree successfully",
			logicTree: logicTree,
		},
		{
			name: "should return error if failed to commit logic tree",
			logicTree: getMerkleTreeWithCommitHook(
				t,
				emptyKeys,
				emptyValues,
				func() error {
					return errors.New("failed to commit logic tree")
				},
			),
			expectedError: errors.New("failed to commit logic tree"),
		},
		{
			name: "should return error if failed to calculate root hash",
			logicTree: getMerkleTreeWithRootHashHook(
				t,
				emptyKeys,
				emptyValues,
				func() (types.Hash, error) {
					return types.NilHash, errors.New("failed to calculate logic root")
				},
			),
			expectedError: errors.New("failed to calculate logic root"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(t, types.NilAddress, nil, test.logicTree, test.logicRoot),
			)

			actualRootHash, err := sObj.commitLogics()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			if test.logicTree == nil {
				require.Equal(t, sObj.data.LogicRoot, actualRootHash)

				return
			}

			require.NoError(t, err)
			checkIfLogicTreeCommitted(t, inputLogicTree, sObj, actualRootHash)
		})
	}
}

func TestCommit(t *testing.T) {
	logicIds := getLogicIDs(t, 1)
	keys, values := getEntries(t, 1)

	balance, _ := getTestBalance(t, getAssetMap(getAssetIDsAndBalances(t, 2)))

	astWithDirtyEntries := getASTWithDefaultDirtyEntries(t, 2, 1)
	inputAST := getCopiedAST(astWithDirtyEntries)

	mstWithDirtyEntries := getMerkleTreeWithDefaultDirtyEntries(t, 1)
	inputMST := mstWithDirtyEntries.Copy()

	logicTree := getMerkleTreeWithFlushedEntries(t, keys, values)
	inputLogicTree := logicTree.Copy()

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name: "should return error if failed to commit logic tree",
			soParams: stateObjectParamsWithLogicTree(
				t,
				types.NilAddress,
				nil,
				getMerkleTreeWithCommitHook(
					t,
					emptyKeys,
					emptyValues,
					func() error {
						return errors.New("failed to commit logic tree")
					},
				),
				types.NilHash,
			),
			expectedError: errors.New("failed to commit logic tree"),
		},
		{
			name: "should return error if failed to commit storage tree",
			soParams: stateObjectParamsWithAST(t,
				getActiveStorageTreesWithCommitHook(
					t,
					logicIds,
					keys,
					values,
					func() error {
						return errors.New("failed to commit ast")
					},
				)),
			expectedError: errors.New("failed to commit ast"),
		},
		{
			name: "committed successfully",
			soParams: &createStateObjectParams{
				soCallback: func(so *StateObject) {
					so.activeStorageTrees = astWithDirtyEntries
					so.logicTree = logicTree
					so.metaStorageTree = mstWithDirtyEntries
					so.balance = balance
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualAccHash, err := sObj.Commit()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForBalance(t, sObj, balance, sObj.data.Balance, 0)
			checkIfLogicTreeCommitted(t, inputLogicTree, sObj, sObj.data.LogicRoot)
			checkIfActiveStorageTreesAreCommitted(t, inputAST, sObj)
			checkIfMetaStorageTreeCommitted(t, inputMST, sObj, sObj.data.StorageRoot, 1)
			// here we pass input account as we validated the account data in previous functions
			inputAcc := new(types.Account)
			inputAcc.StorageRoot = sObj.data.StorageRoot
			inputAcc.Balance = sObj.data.Balance
			inputAcc.LogicRoot = sObj.data.LogicRoot
			inputAcc.Nonce = 1

			checkForAccount(t, sObj, inputAcc, actualAccHash, 2)
		})
	}
}

func TestFlushLogicTree(t *testing.T) {
	testcases := []struct {
		name      string
		logicTree tree.MerkleTree
	}{
		{
			name:      "logic tree is nil",
			logicTree: nil,
		},
		{
			name:      "logic tree flushed to db successfully",
			logicTree: mockMerkleTreeWithDB(),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(t, types.NilAddress, nil, test.logicTree, types.NilHash),
			)

			err := sObj.flushLogicTree()
			require.NoError(t, err)

			if test.logicTree != nil {
				checkIfMerkleTreeFlushed(t, sObj.logicTree, true)

				return
			}
		})
	}
}

func TestFlushActiveStorageTrees(t *testing.T) {
	logicIds := getLogicIDs(t, 2)
	ast := getActiveStorageTrees(t, logicIds, emptyKeys, emptyValues)
	mst := mockMerkleTreeWithDB()

	testcases := []struct {
		name          string
		mst           tree.MerkleTree
		ast           map[string]tree.MerkleTree
		isMSTNil      bool
		shouldFlush   bool
		expectedError error
	}{
		{
			name:     "meta storage tree is nil",
			ast:      ast,
			isMSTNil: true,
		},
		{
			name:        "successfully flushed active storage tree",
			ast:         ast,
			mst:         mst,
			shouldFlush: true,
		},
		{
			name: "should return error if failed to flush active storage tree",
			ast: getActiveStorageTreesWithFlushHook(t,
				logicIds,
				emptyKeys,
				emptyValues,
				func() error {
					return errors.New("failed to flush ast")
				},
			),
			mst:           mst,
			expectedError: errors.New("failed to flush ast"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, stateObjectParamsWithASTAndMST(t, test.ast, test.mst))

			err := sObj.flushActiveStorageTrees()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfActiveStorageTreesFlushed(t, logicIds, sObj, test.shouldFlush)

			if test.isMSTNil {
				return
			}

			checkIfMerkleTreeFlushed(t, sObj.metaStorageTree, test.shouldFlush)
		})
	}
}

func TestCreateAsset(t *testing.T) {
	sObj := createTestStateObject(t, &createStateObjectParams{
		soCallback: func(so *StateObject) {
			so.balance.Balances = make(types.AssetMap)
		},
	})

	assetDescriptor := getDefaultAssetDescriptor(t, "btc")
	assetDescriptor.Owner = sObj.address
	assetID, _, _, err := gtypes.GetAssetID(assetDescriptor)
	require.NoError(t, err)

	sObj.setBalance(assetID, big.NewInt(2231))

	testcases := []struct {
		name            string
		assetDescriptor *types.AssetDescriptor
		expectedError   error
	}{
		{
			name:            "asset created successfully",
			assetDescriptor: getDefaultAssetDescriptor(t, "moi"),
		},
		{
			name:            "should return error if asset already exists",
			assetDescriptor: assetDescriptor,
			expectedError:   errors.New("asset already exists"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualAssetID, err := sObj.CreateAsset(test.assetDescriptor)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			expectedAssetID, _, _, err := getTestAssetID(test.assetDescriptor)
			require.NoError(t, err)
			require.Equal(t, expectedAssetID, actualAssetID)

			checkForAssetCreation(t, sObj, test.assetDescriptor, 0)
		})
	}
}

func TestGetMetaContextObjectCopy(t *testing.T) {
	hashes := getHashes(t, 4)
	mObj, mHash := getMetaContextObjects(t, hashes)

	testcases := []struct {
		name           string
		soParams       *createStateObjectParams
		expectedObject *gtypes.MetaContextObject
		expectedError  error
	}{
		{
			name:           "meta context object exists in cache",
			soParams:       stateObjectParamsWithMetaContextObject(t, mObj[0], mHash[0], false),
			expectedObject: mObj[0],
		},
		{
			name:           "meta context object exists in db",
			soParams:       stateObjectParamsWithMetaContextObject(t, mObj[1], mHash[1], true),
			expectedObject: mObj[1],
		},
		{
			name:          "should return error if meta context object doesn't exist",
			expectedError: errors.New("failed to fetch meta context object"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualObj, err := sObj.getMetaContextObjectCopy()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedObject, actualObj)
		})
	}
}

func TestGetContextObjectCopy(t *testing.T) {
	kramaIDs := tests.GetTestKramaIDs(t, 4)
	cObj, cHash := getContextObjects(t, kramaIDs, 2, 2)

	testcases := []struct {
		name           string
		hash           types.Hash
		soParams       *createStateObjectParams
		expectedObject *gtypes.ContextObject
		expectedError  error
	}{
		{
			name:           "context object exists in cache",
			hash:           cHash[0],
			soParams:       stateObjectParamsWithContextObject(t, cObj[0], cHash[0], false),
			expectedObject: cObj[0],
		},
		{
			name:           "context object exists in db",
			hash:           cHash[1],
			soParams:       stateObjectParamsWithContextObject(t, cObj[1], cHash[1], true),
			expectedObject: cObj[1],
		},
		{
			name:          "should return error if context object doesn't exist",
			expectedError: errors.New("failed to fetch context object"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualObj, err := sObj.getContextObjectCopy(test.hash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedObject, actualObj)
		})
	}
}

func TestCreateContext(t *testing.T) {
	sObj := createTestStateObject(t, nil)

	testcases := []struct {
		name             string
		behaviouralNodes []id.KramaID
		randomNodes      []id.KramaID
		expectedError    error
	}{
		{
			name:             "created context successfully",
			behaviouralNodes: tests.GetTestKramaIDs(t, 2),
			randomNodes:      tests.GetTestKramaIDs(t, 2),
			expectedError:    nil,
		},
		{
			name:             "should return error if failed to create context",
			behaviouralNodes: tests.GetTestKramaIDs(t, 0),
			randomNodes:      tests.GetTestKramaIDs(t, 0),
			expectedError:    errors.New("liveliness size not met"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			mHash, err := sObj.CreateContext(test.behaviouralNodes, test.randomNodes)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, sObj.ContextHash(), mHash)

			metaContext := getMetaContextObjFromCache(t, sObj, mHash)
			behaviouralContext := getContextObjFromCache(t, sObj, metaContext.BehaviouralContext)
			randomContext := getContextObjFromCache(t, sObj, metaContext.RandomContext)

			// check if context objects has input nodes
			require.Equal(t, test.behaviouralNodes, behaviouralContext.Ids)
			require.Equal(t, test.randomNodes, randomContext.Ids)

			checkForContextObject(t, sObj, *behaviouralContext, metaContext.BehaviouralContext)
			checkForContextObject(t, sObj, *randomContext, metaContext.RandomContext)
			checkForMetaContextObject(t, sObj, *metaContext, mHash)
		})
	}
}

func TestUpdateContext(t *testing.T) {
	kramaIDs := tests.GetTestKramaIDs(t, 4)
	cObj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	behaviouralNodes := tests.GetTestKramaIDs(t, 2)
	randomNodes := tests.GetTestKramaIDs(t, 2)

	testcases := []struct {
		name             string
		behaviouralNodes []id.KramaID
		randomNodes      []id.KramaID
		metaHash         types.Hash
		soParams         *createStateObjectParams
		mCtx             *gtypes.MetaContextObject
		expectedError    error
	}{
		{
			name:             "should return error if meta context object doesn't exist",
			behaviouralNodes: behaviouralNodes,
			randomNodes:      randomNodes,
			metaHash:         tests.RandomHash(t),
			expectedError:    errors.New("failed to fetch meta context object"),
		},
		{
			name:             "should return error if behaviour context object doesn't exist",
			behaviouralNodes: behaviouralNodes,
			randomNodes:      randomNodes,
			metaHash:         mHash[0],
			soParams:         stateObjectParamsWithMetaContextObject(t, mObj[0], mHash[0], false),
			expectedError:    errors.New("failed to fetch context object"),
		},
		{
			name:             "should return error if random ctx object doesn't exist",
			behaviouralNodes: behaviouralNodes[0:0],
			randomNodes:      randomNodes,
			metaHash:         mHash[0],
			soParams:         stateObjectParamsWithMetaContextObject(t, mObj[0], mHash[0], false),
			expectedError:    errors.New("failed to fetch context object"),
		},
		{
			name:             "context updated successfully",
			behaviouralNodes: behaviouralNodes,
			randomNodes:      randomNodes,
			metaHash:         mHash[0],
			soParams: &createStateObjectParams{
				soCallback: func(so *StateObject) {
					insertContextsInDB(t, so.db, cObj...)
					insertContextsInDB(t, so.db, cObj...)
					insertMetaContextsInDB(t, so.db, mObj...)
					insertContextHash(so, mHash[0])
				},
			},
			mCtx: mObj[0],
		},
		{
			name:             "empty behavioural and random nodes",
			behaviouralNodes: behaviouralNodes[0:0],
			randomNodes:      randomNodes[0:0],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			insertContextHash(sObj, test.metaHash)

			metaHash, err := sObj.UpdateContext(test.behaviouralNodes, test.randomNodes)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, sObj.ContextHash(), metaHash)

			if len(test.behaviouralNodes) == 0 && len(test.randomNodes) == 0 {
				return
			}

			checkForContextUpdate(t,
				sObj,
				cObj,
				metaHash,
				test.behaviouralNodes,
				test.randomNodes,
			)
		})
	}
}

func TestGetBalanceObject(t *testing.T) {
	assetIDs, bal := getAssetIDsAndBalances(t, 2)
	balance, balanceHash := getTestBalances(t, getAssetMaps(assetIDs, bal, 1), 1)

	soParams := &createStateObjectParams{
		soCallback: func(so *StateObject) {
			insertBalancesInDB(t, so.db, balanceHash, balance...)
		},
	}
	sObj := createTestStateObject(t, soParams)

	testcases := []struct {
		name            string
		hash            types.Hash
		expectedBalance *gtypes.BalanceObject
		expectedError   error
	}{
		{
			name: "hash is nil",
			hash: types.NilHash,
		},
		{
			name:          "should return error if balance object doesn't exist",
			hash:          tests.RandomHash(t),
			expectedError: types.ErrKeyNotFound,
		},
		{
			name:            "fetched balance object successfully",
			hash:            balanceHash[0],
			expectedBalance: balance[0],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualBalance, err := getBalanceObject(sObj.address, test.hash, sObj.db)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if !test.hash.IsNil() {
				require.Equal(t, test.expectedBalance, actualBalance)
			}
		})
	}
}

//nolint:dupl
func TestGetMetaStorageTree(t *testing.T) {
	keys, values := getEntries(t, 2)

	db := mockDB()
	address := tests.RandomAddress(t)
	storageTree, storageRoot := createTestKramaHashTree(t,
		db,
		address,
		keys,
		values,
	)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:     "fetch meta storage tree from db",
			soParams: stateObjectParamsWithMST(t, address, db, nil, storageRoot),
		},
		{
			name:     "fetch meta storage tree from cache",
			soParams: stateObjectParamsWithMST(t, address, db, storageTree, types.NilHash),
		},
		{
			name:          "should return error if failed to initiate storage tree",
			soParams:      stateObjectParamsWithMST(t, address, db, nil, tests.RandomHash(t)),
			expectedError: errors.New("failed to initiate storage tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualMetaStorageTree, err := sObj.getMetaStorageTree()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForEntriesInMerkleTree(t, actualMetaStorageTree, keys, values)

			checkForKramaHashTree(t, storageTree, actualMetaStorageTree)
			checkForKramaHashTree(t, storageTree, sObj.metaStorageTree) // check if mst stored in state object matches
		})
	}
}

func TestGetStorageTree(t *testing.T) {
	logicID := getLogicID(t, tests.RandomAddress(t))
	keys, values := getEntries(t, 2)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		logicID       types.LogicID
		expectedError error
	}{
		{
			name:     "fetched storage tree from active storage trees",
			soParams: stateObjectParamsWithAST(t, getActiveStorageTrees(t, []types.LogicID{logicID}, keys, values)),
			logicID:  logicID,
		},
		{
			name:          "should return error if failed to get meta storage tree",
			soParams:      stateObjectParamsWithInvalidMST(t),
			logicID:       getLogicID(t, tests.RandomAddress(t)),
			expectedError: errors.New("failed to initiate storage tree"),
		},
		{
			name:          "should return error if logic storage tree not found",
			logicID:       getLogicID(t, tests.RandomAddress(t)),
			expectedError: types.ErrLogicStorageTreeNotFound,
		},
		{
			name: "fetched storage tree from db",
			soParams: &createStateObjectParams{
				soCallback: func(so *StateObject) {
					so.metaStorageTree, _ = createMetaStorageTree(
						t,
						so.db,
						so.address,
						SargaLogicID,
						keys,
						values,
					)
				},
			},
			logicID: SargaLogicID,
		},
		{
			name: "should return error if failed to initiate logic storage tree",
			soParams: &createStateObjectParams{
				soCallback: func(so *StateObject) {
					so.metaStorageTree, _ = createTestKramaHashTree(
						t,
						so.db,
						so.address,
						[][]byte{SargaLogicID},
						[][]byte{tests.RandomHash(t).Bytes()},
					)
				},
			},
			logicID:       SargaLogicID,
			expectedError: errors.New("failed to initiate logic storage tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			storageTree, err := sObj.GetStorageTree(test.logicID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForEntriesInMerkleTree(t, storageTree, keys, values)
		})
	}
}

func TestSetStorageEntry(t *testing.T) {
	logicIDs := getLogicIDs(t, 2)
	keys, values := getEntries(t, 2)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:     "successfully set key value in storage tree",
			soParams: stateObjectParamsWithAST(t, getActiveStorageTrees(t, logicIDs, keys[:1], values[:1])),
		},
		{
			name:          "should return error if failed to get storage tree",
			soParams:      stateObjectParamsWithInvalidMST(t),
			expectedError: errors.New("failed to initiate storage tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			err := sObj.SetStorageEntry(logicIDs[0], keys[1], values[1])
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			storageTree, ok := sObj.activeStorageTrees[logicIDs[0].Hex()]
			require.True(t, ok)

			checkForEntryInMerkleTree(t, storageTree, keys[1], values[1])
		})
	}
}

func TestGetStorageEntry(t *testing.T) {
	logicIDs := getLogicIDs(t, 2)
	keys, values := getEntries(t, 1)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:     "successfully fetched entry in storage tree",
			soParams: stateObjectParamsWithAST(t, getActiveStorageTrees(t, logicIDs, keys, values)),
		},
		{
			name:          "should return error if failed to load meta storage tree",
			soParams:      stateObjectParamsWithInvalidMST(t),
			expectedError: errors.New("failed to initiate storage tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			value, err := sObj.GetStorageEntry(logicIDs[0], keys[0])
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, values[0], value)
		})
	}
}

//nolint:dupl
func TestGetMetaLogicTree(t *testing.T) {
	keys, values := getEntries(t, 2)
	db := mockDB()
	address := tests.RandomAddress(t)
	logicTree, logicRoot := createTestKramaHashTree(t,
		db,
		address,
		keys,
		values,
	)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:     "fetch logic tree from db",
			soParams: stateObjectParamsWithLogicTree(t, address, db, nil, logicRoot),
		},
		{
			name:     "fetch logic tree from cache",
			soParams: stateObjectParamsWithLogicTree(t, address, db, logicTree, types.NilHash),
		},
		{
			name:          "should return error if failed to initiate logic tree",
			soParams:      stateObjectParamsWithLogicTree(t, address, db, nil, tests.RandomHash(t)),
			expectedError: errors.New("failed to initiate logic tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualLogicTree, err := sObj.getMetaLogicTree()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForEntriesInMerkleTree(t, actualLogicTree, keys, values)
			checkForKramaHashTree(t, logicTree, actualLogicTree)
			checkForKramaHashTree(t, logicTree, sObj.logicTree) // check if logic tree stored in state object matches
		})
	}
}

func TestGetLogicObject(t *testing.T) {
	logicID := getLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	rawData, err := logicObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name: "should return error if logic tree not found",
			soParams: stateObjectParamsWithLogicTree(
				t,
				types.NilAddress,
				nil,
				nil,
				tests.RandomHash(t),
			),
			expectedError: errors.New("failed to initiate logic tree"),
		},
		{
			name:          "should return error if logic object not found",
			soParams:      stateObjectParamsWithLogicTree(t, types.NilAddress, nil, nil, types.NilHash),
			expectedError: types.ErrKeyNotFound,
		},
		{
			name: "fetched logic object from logic tree successfully",
			soParams: &createStateObjectParams{
				soCallback: func(so *StateObject) {
					so.logicTree = nil

					_, so.data.LogicRoot = createTestKramaHashTree(t,
						so.db,
						so.address,
						[][]byte{logicID},
						[][]byte{rawData},
					)
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualLogicObject, err := sObj.getLogicObject(logicID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, logicObject, actualLogicObject)
		})
	}
}

func TestIsLogicRegistered(t *testing.T) {
	logicID := getLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	rawData, err := logicObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		logicTree     tree.MerkleTree
		expectedError error
	}{
		{
			name:          "should return error if logic not registered",
			logicTree:     nil,
			expectedError: types.ErrKeyNotFound,
		},
		{
			name:      "logic registered",
			logicTree: getMerkleTreeWithEntries(t, [][]byte{logicID}, [][]byte{rawData}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(t, types.NilAddress, nil, test.logicTree, types.NilHash),
			)

			err = sObj.isLogicRegistered(logicID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInsertNewLogicObject(t *testing.T) {
	logicID := getLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	rawData, err := logicObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		logicTree     tree.MerkleTree
		logicRoot     types.Hash
		logicID       types.LogicID
		expectedError error
	}{
		{
			name:          "should return error if failed to load logic tree",
			logicRoot:     tests.RandomHash(t),
			logicID:       logicID,
			expectedError: errors.New("failed to load logic tree"),
		},
		{
			name:          "should return error if logic already registered",
			logicTree:     getMerkleTreeWithEntries(t, [][]byte{logicID}, [][]byte{rawData}),
			logicID:       logicID,
			expectedError: errors.New("logic already registered"),
		},
		{
			name:      "logic object inserted successfully",
			logicTree: getMerkleTreeWithEntries(t, [][]byte{logicID}, [][]byte{rawData}),
			logicID:   getLogicID(t, tests.RandomAddress(t)),
		},
		{
			name: "should return error if failed to add logic object to tree",
			logicTree: getMerkleTreeWithSetHook(
				t,
				emptyKeys,
				emptyValues,
				func() error {
					return errors.New("failed to add logic object to tree")
				},
			),
			logicID:       logicID,
			expectedError: errors.New("failed to add logic object to tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(t, types.NilAddress, nil, test.logicTree, test.logicRoot),
			)

			err = sObj.InsertNewLogicObject(test.logicID, logicObject)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			actualLogicObject, err := sObj.getLogicObject(logicID)
			require.NoError(t, err)
			require.Equal(t, logicObject, actualLogicObject)
		})
	}
}

func TestCreateStorageTreeForLogic(t *testing.T) {
	logicID := getLogicID(t, tests.RandomAddress(t))

	testcases := []struct {
		name          string
		logicID       types.LogicID
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:          "should return error if failed to fetch meta storage tree",
			soParams:      stateObjectParamsWithInvalidMST(t),
			expectedError: errors.New("failed to initiate storage tree"),
		},
		{
			name:    "successfully created storage tree for logic",
			logicID: logicID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualStorageTree, err := sObj.createStorageTreeForLogic(test.logicID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// make sure storage tree inserted in AST
			expectedLogicTree, ok := sObj.activeStorageTrees[logicID.Hex()]
			require.True(t, ok)

			checkForKramaHashTree(t, expectedLogicTree, actualStorageTree)
			checkForEntryInMST(t, sObj, logicID, types.NilHash.Bytes())
		})
	}
}

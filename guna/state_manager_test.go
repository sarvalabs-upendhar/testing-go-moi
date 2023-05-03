package guna

import (
	"context"
	"errors"
	"testing"

	lru "github.com/hashicorp/golang-lru"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/types"
)

func TestCreateStateObject(t *testing.T) {
	address := tests.RandomAddress(t)
	accType := types.LogicAccount

	sm := createTestStateManager(t, nil)
	so := sm.createStateObject(address, accType)

	validateStateObject(t, so, accType, address)
}

func TestCleanupDirtyObject(t *testing.T) {
	stateObject := createTestStateObject(t, nil)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.dirtyObjects[stateObject.address] = stateObject
		},
	}

	sm := createTestStateManager(t, smParams)

	sm.cleanupDirtyObject(stateObject.address)

	checkForDirtyObject(t, sm, stateObject.address, false)
}

func TestCreateDirtyObject(t *testing.T) {
	sm := createTestStateManager(t, nil)

	address := tests.RandomAddress(t)
	do := sm.CreateDirtyObject(address, types.SargaAccount)

	dirtyObject, ok := sm.dirtyObjects[address]

	require.True(t, ok)
	require.Equal(t, do.accType, types.SargaAccount)
	require.Equal(t, do, dirtyObject)
}

func TestGetLatestTesseractHash(t *testing.T) {
	accMetaInfo := getAccMetaInfos(t, 2)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			db.setAccountMetaInfo(accMetaInfo[0])
		},
		smCallBack: func(sm *StateManager) {
			storeInSmCache(sm, accMetaInfo[1].Address, accMetaInfo[1].TesseractHash)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		hash          types.Hash
		expectedError error
	}{
		{
			name:          "should return error if nil address",
			address:       types.NilAddress,
			hash:          accMetaInfo[0].TesseractHash,
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:          "fetches tesseract hash from cache",
			address:       accMetaInfo[1].Address,
			hash:          accMetaInfo[1].TesseractHash,
			expectedError: nil,
		},
		{
			name:          "should fail if tesseract not found",
			address:       tests.RandomAddress(t),
			hash:          accMetaInfo[0].TesseractHash,
			expectedError: types.ErrFetchingAccMetaInfo,
		},
		{
			name:          "fetches tesseract hash from db",
			address:       accMetaInfo[0].Address,
			hash:          accMetaInfo[0].TesseractHash,
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := sm.getLatestTesseractHash(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}
			require.NoError(t, err)
			require.Equal(t, test.hash, hash)
			checkForCache(t, sm, test.address)
		})
	}
}

func TestFetchTesseractFromDB(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseracts := tests.CreateTesseracts(t, 3, tesseractParams)

	tsParams := &tests.CreateTesseractParams{
		Height:         3,
		HeaderCallback: tests.HeaderCallbackWithGridHash(t),
	}

	ts := tests.CreateTesseract(t, tsParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts...)
			db.insertTesseract(t, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		hash             types.Hash
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "genesis tesseract with interactions",
			hash:             tesseracts[0].Hash(),
			withInteractions: true,
			expectedTS:       tesseracts[0],
			expectedError:    nil,
		},
		{
			name:             "non-genesis tesseract with interactions",
			hash:             tesseracts[1].Hash(),
			withInteractions: true,
			expectedTS:       tesseracts[1],
			expectedError:    nil,
		},
		{
			name:             "without interactions",
			hash:             tesseracts[1].Hash(),
			withInteractions: false,
			expectedTS:       tesseracts[1],
			expectedError:    nil,
		},
		{
			name:             "should fail if tesseract not found",
			hash:             tests.RandomHash(t),
			withInteractions: false,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "should fail if grid hash not found",
			hash:             getTesseractHash(t, tesseracts[2]),
			withInteractions: true,
			expectedError:    types.ErrGridHashNotFound,
		},
		{
			name:             "should fail if interactions not found",
			hash:             tesseracts[2].Hash(),
			withInteractions: true,
			expectedError:    types.ErrGridHashNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := sm.FetchTesseractFromDB(test.hash, test.withInteractions)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			validateTesseract(t, ts, test.expectedTS, test.withInteractions)
		})
	}
}

func TestGetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseracts := tests.CreateTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts[:2]...)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(tesseracts[2].Hash(), tesseracts[2])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		hash             types.Hash
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "fetches tesseract from cache", // only tesseracts without interactions exists in cache
			hash:             tesseracts[2].Hash(),
			withInteractions: false,
			expectedTS:       tesseracts[2],
			expectedError:    nil,
		},
		{
			name:             "should fail if tesseract not found",
			hash:             tests.RandomHash(t),
			withInteractions: true,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "with interactions",
			hash:             tesseracts[0].Hash(),
			withInteractions: true,
			expectedTS:       tesseracts[0],
			expectedError:    nil,
		},
		{
			name:             "without interactions",
			hash:             tesseracts[1].Hash(),
			withInteractions: false,
			expectedTS:       tesseracts[1],
			expectedError:    nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := sm.getTesseractByHash(test.hash, test.withInteractions)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			validateTesseract(t, ts, test.expectedTS, test.withInteractions)
			checkForTesseractInSMCache(t, sm, ts, test.withInteractions)
		})
	}
}

func TestGetStateObjectByHash(t *testing.T) {
	assetIDs, bal := getAssetIDsAndBalances(t, 2)
	balances, balanceHashes := getTestBalances(t, getAssetMaps(assetIDs, bal, 1), 2)
	accounts, stateHashes := getTestAccounts(t, balanceHashes, 2)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)   // insert account into db
			insertBalancesInDB(t, db, balanceHashes, balances[0]) // (stateHash : account, balanceHash : balance)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		stateHash     types.Hash
		address       types.Address
		sObj          *StateObject
		expectedError error
	}{
		{
			name:      "state object exists",
			stateHash: stateHashes[0],
			address:   so[0].address,
			sObj:      so[0],
		},
		{
			name:          "should fail if account not found",
			stateHash:     tests.RandomHash(t),
			address:       types.NilAddress,
			expectedError: types.ErrStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			so, err := sm.GetStateObjectByHash(test.address, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForStateObject(t, test.sObj, so)
		})
	}
}

func TestGetLatestStateObject(t *testing.T) {
	assetIDs, bal := getAssetIDsAndBalances(t, 2)
	balances, balanceHashes := getTestBalances(t, getAssetMaps(assetIDs, bal, 1), 2)
	accounts, stateHashes := getTestAccounts(t, balanceHashes, 2)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
		},
		1: stateObjectParamsWithBalance(t, balanceHashes[1], balances[1]),
	}

	so := createTestStateObjects(t, 2, soParams)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithStateHash(so[0].address, stateHashes[0]),
		1: getTesseractParamsWithStateHash(tests.RandomAddress(t), tests.RandomHash(t)),
	}

	tesseracts := tests.CreateTesseracts(t, 2, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
			insertBalancesInDB(t, db, balanceHashes, balances...)
			insertTesseractsInDB(t, db, tesseracts...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		sObj          *StateObject
		expectedError error
	}{
		{
			name:    "state object constructed from db",
			address: so[0].address,
			sObj:    so[0],
		},
		{
			name:          "should fail if tesseract not found",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:          "should fail if state object not found",
			address:       tesseracts[1].Address(),
			expectedError: types.ErrStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			latestStateObject, err := sm.GetLatestStateObject(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForStateObject(t, test.sObj, latestStateObject)
		})
	}
}

func TestGetStateObject(t *testing.T) {
	account, stateHash := getTestAccounts(t, []types.Hash{tests.RandomHash(t), tests.RandomHash(t)}, 2)

	so := NewStateObject(tests.RandomAddress(t), nil, mockJournal(), mockDB(), *account[0])
	so1 := NewStateObject(tests.RandomAddress(t), nil, mockJournal(), mockDB(), *account[1])

	ts := tests.CreateTesseract(t, getTesseractParamsWithStateHash(so1.Address(), stateHash[1]))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHash, account...)
			insertTesseractsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name      string
		address   types.Address
		stateHash types.Hash
		sObj      *StateObject
	}{
		{
			name:      "fetch state object from state hash",
			address:   so.address,
			stateHash: stateHash[0],
			sObj:      so,
		},
		{
			name:      "fetch latest state object",
			address:   ts.Address(),
			stateHash: types.NilHash,
			sObj:      so1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateObject, err := sm.getStateObject(test.address, test.stateHash)
			require.NoError(t, err)

			checkForStateObject(t, test.sObj, stateObject)
		})
	}
}

func TestGetLatestTesseract(t *testing.T) {
	tesseracts := tests.CreateTesseracts(t,
		2,
		tests.GetTesseractParamsMapWithIxns(
			t,
			3,
			2,
		),
	)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts[1:]...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		address          types.Address
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "should fail if tesseract address doesn't exist",
			address:          tests.RandomAddress(t),
			withInteractions: true,
			expectedError:    errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:             "should fail if tesseract hash doesn't exist",
			address:          tesseracts[0].Address(),
			withInteractions: true,
			expectedTS:       tesseracts[0],
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "with interactions",
			address:          tesseracts[1].Address(),
			withInteractions: true,
			expectedTS:       tesseracts[1],
			expectedError:    nil,
		},
		{
			name:             "without interactions",
			address:          tesseracts[1].Address(),
			withInteractions: false,
			expectedTS:       tesseracts[1],
			expectedError:    nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := sm.GetLatestTesseract(test.address, test.withInteractions)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			validateTesseract(t, ts, test.expectedTS, test.withInteractions)
		})
	}
}

func TestGetDirtyObject(t *testing.T) {
	soParams := map[int]*createStateObjectParams{
		0: {
			address: tests.RandomAddress(t),
			account: &types.Account{
				Nonce: 2,
			},
		}, // Add balance as we validate it

	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertDirtyObject(sm, so[0])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		sObj          *StateObject
		expectedError error
	}{
		{
			name:    "address in state manager's dirty object",
			address: so[0].address,
			sObj:    so[0],
		},
		{
			name:          "should fail if state object not found",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			so, err := sm.GetDirtyObject(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			checkForStateObject(t, test.sObj, so)
			checkForDirtyObject(t, sm, test.address, true)
		})
	}
}

func TestCleanup(t *testing.T) {
	so := createTestStateObject(t, nil)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertDirtyObject(sm, so)
		},
	}

	sm := createTestStateManager(t, smParams)

	sm.Cleanup(so.address)
	checkForDirtyObject(t, sm, so.address, false)
}

func TestRevert(t *testing.T) {
	address := tests.RandomAddress(t)
	getStateObjectParams := func() *createStateObjectParams {
		return &createStateObjectParams{
			address: address,
		}
	}

	soParams := map[int]*createStateObjectParams{
		0: getStateObjectParams(),
		1: getStateObjectParams(),
	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertDirtyObject(sm, so[0])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name           string
		snap           *StateObject
		expectedObject *StateObject
	}{
		{
			name:           "valid snap",
			snap:           so[1],
			expectedObject: so[1],
		},
		{
			name:           "nil snap",
			snap:           nil,
			expectedObject: so[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sm.Revert(test.snap)
			require.NoError(t, err)

			checkIfDirtyObjectEqual(t, sm, test.expectedObject.address, test.expectedObject)
		})
	}
}

func TestGetContextObject(t *testing.T) {
	kramaIDs, _ := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			db.setContext(t, obj[1])
		},
		smCallBack: func(sm *StateManager) {
			storeInSmCache(sm, cHash[0], obj[0])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		hash          types.Hash
		ctx           *gtypes.ContextObject
		expectedError error
	}{
		{
			name: "context object in cache",
			hash: cHash[0],
			ctx:  obj[0],
		},
		{
			name: "context object in db",
			hash: cHash[1],
			ctx:  obj[1],
		},
		{
			name:          "should fail if context object not found",
			hash:          tests.RandomHash(t),
			expectedError: types.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctx, err := sm.getContextObject(types.NilAddress, test.hash)

			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.ctx, ctx)

			obj, ok := sm.cache.Get(test.hash)
			require.True(t, ok)
			require.Equal(t, test.ctx, obj)
		})
	}
}

func TestGetMetaContextObject(t *testing.T) {
	kramaIDs, _ := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	_, hashes := getContextObjects(t, kramaIDs, 2, 4)
	mObj, hashes := getMetaContextObjects(t, hashes)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			db.setMetaContext(t, mObj[1])
		},
		smCallBack: func(sm *StateManager) {
			storeInSmCache(sm, hashes[0], mObj[0])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		hash          types.Hash
		ctx           *gtypes.MetaContextObject
		expectedError error
	}{
		{
			name: "meta context object in cache",
			hash: hashes[0],
			ctx:  mObj[0],
		},
		{
			name: "meta context object in db",
			hash: hashes[1],
			ctx:  mObj[1],
		},
		{
			name:          "should fail if meta context object not found",
			hash:          tests.RandomHash(t),
			expectedError: types.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctx, err := sm.getMetaContextObject(types.NilAddress, test.hash)
			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.ctx, ctx)

			obj, ok := sm.cache.Get(test.hash)
			require.True(t, ok)
			require.Equal(t, test.ctx, obj)
		})
	}
}

func TestGetContext(t *testing.T) {
	mObj := make([]*gtypes.MetaContextObject, 3)
	mHash := make([]types.Hash, 3)

	kramaIDs, _ := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj[0], mHash[0] = getMetaContextObject(t, cHash[0], cHash[1])
	mObj[1], mHash[1] = getMetaContextObject(t, cHash[2], types.NilHash)
	mObj[2], mHash[2] = getMetaContextObject(t, types.NilHash, cHash[3])

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		hash          types.Hash
		behCtx        *gtypes.ContextObject
		randCtx       *gtypes.ContextObject
		expectedError error
	}{
		{
			name:          "meta context object doesn't exist",
			hash:          tests.RandomHash(t),
			expectedError: errors.New("metaContextObject fetch failed"),
		},
		{
			name:          "behaviour context object doesn't exist",
			hash:          mHash[2],
			expectedError: errors.New("behaviouralContextObject fetch failed"),
		},
		{
			name:          "random context object doesn't exist",
			hash:          mHash[1],
			expectedError: errors.New("randomContextObject fetch failed"),
		},
		{
			name:    "valid context",
			hash:    mHash[0],
			behCtx:  obj[0],
			randCtx: obj[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			behCtx, randCtx, err := sm.getContext(types.NilAddress, test.hash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			checkIfContextMatches(t,
				test.behCtx,
				test.randCtx,
				behCtx,
				randCtx,
			)
		})
	}
}

func TestGetContextByHash(t *testing.T) {
	kramaIDs, _ := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ts := tests.CreateTesseract(t, getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[1]))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		hash          types.Hash
		behCtx        *gtypes.ContextObject
		randCtx       *gtypes.ContextObject
		expectedError error
	}{
		{
			name:          "address and context hash are nil",
			address:       types.NilAddress,
			hash:          types.NilHash,
			expectedError: types.ErrEmptyHashAndAddress,
		},
		{
			name:          "valid context hash",
			address:       ts.Address(),
			hash:          mHash[0],
			behCtx:        obj[0],
			randCtx:       obj[1],
			expectedError: nil,
		},
		{
			name:          "context doesn't exist",
			address:       tests.RandomAddress(t),
			hash:          tests.RandomHash(t),
			expectedError: errors.New("metaContextObject fetch failed"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, behCtx, randCtx, err := sm.GetContextByHash(test.address, test.hash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfContextMatches(t,
				test.behCtx,
				test.randCtx,
				behCtx,
				randCtx,
			)
		})
	}
}

func TestFetchParticipantContextByHash(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
	contract := NewMockContract(t, append(kramaIDs[:5], kramaIDs[11:]...), append(pk[:5], pk[11:]...))
	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		hash          types.Hash
		mockFn        func()
		expectedError error
	}{
		{
			name:          "context hash without state",
			hash:          tests.RandomHash(t),
			expectedError: errors.New("metaContextObject fetch failed"),
		},
		{
			name:          "behavioural context node's public keys not found",
			hash:          mHash[2],
			expectedError: types.ErrPublicKeyNotFound,
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name:          "random context node's public keys not found",
			hash:          mHash[1],
			expectedError: types.ErrPublicKeyNotFound,
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name: "valid hash and public keys",
			hash: mHash[0],
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.mockFn != nil {
				test.mockFn()
			}

			behCtx, randCtx, err := sm.fetchParticipantContextByHash(types.NilAddress, test.hash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(
				t,
				types.NewNodeSet(obj[0].Ids, pk[:2]),
				types.NewNodeSet(obj[1].Ids, pk[2:4]),
				behCtx,
				randCtx,
			)
		})
	}
}

func TestGetCommittedContextHash(t *testing.T) {
	ts := tests.CreateTesseract(t, getTesseractParamsWithContextHash(tests.RandomAddress(t), tests.RandomHash(t)))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		contextHash   types.Hash
		expectedError error
	}{
		{
			name:          "context doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:        "context exists",
			address:     ts.Address(),
			contextHash: ts.ContextHash(),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := sm.GetCommittedContextHash(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.contextHash, hash)
		})
	}
}

func TestFetchContextLock(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	contract := NewMockContract(t, kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	addresses := getAddresses(t, 10)
	ixns := tests.CreateIxns(t, 5, tests.GetIxParamsMapWithAddresses(addresses[:5], addresses[5:10]))

	getTesseractParams := func(
		ixns types.Interactions,
		addresses []types.Address,
		hashes ...types.Hash,
	) *tests.CreateTesseractParams {
		return &tests.CreateTesseractParams{
			Ixns: ixns,
			HeaderCallback: func(header *types.TesseractHeader) {
				header.ContextLock = mockContextLock()
				for i, address := range addresses {
					insertInContextLock(header, address, hashes[i])
				}
			},
		}
	}

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParams(
			ixns[0:1],
			[]types.Address{ixns[0].Sender(), ixns[0].Receiver()},
			mHash[0], mHash[1],
		),
		1: getTesseractParams(
			ixns[1:2],
			[]types.Address{ixns[1].Sender()},
			tests.RandomHash(t),
		),
		2: getTesseractParams(
			ixns[2:3],
			[]types.Address{ixns[2].Sender()},
			tests.RandomHash(t),
		),
		3: getTesseractParams(
			ixns[3:4],
			[]types.Address{ixns[3].Sender(), ixns[3].Receiver()},
			mHash[0], types.NilHash,
		),
		4: getTesseractParams(
			ixns[4:5],
			[]types.Address{types.SargaAddress},
			mHash[1],
		),
	}

	ts := tests.CreateTesseracts(t, 5, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		tess          *types.Tesseract
		nodes         *ICSNodes
		mockFn        func()
		expectedError error
	}{
		{
			name:          "context of sender not found",
			tess:          ts[1],
			expectedError: errors.New("metaContextObject fetch failed"),
		},
		{
			name:          "context of receiver not found",
			tess:          ts[2],
			expectedError: errors.New("metaContextObject fetch failed"),
		},
		{
			name: "receiver has nil context hash",
			tess: ts[3],
			nodes: getICSNodes(
				types.NewNodeSet(obj[0].Ids, pk[:2]),
				types.NewNodeSet(obj[1].Ids, pk[2:4]),
				nil,
				nil,
			),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name: "sarga address in context lock",
			tess: ts[4],
			nodes: getICSNodes(
				nil, nil,
				types.NewNodeSet(obj[2].Ids, pk[4:6]),
				types.NewNodeSet(obj[3].Ids, pk[6:8]),
			),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name: "valid context hashes",
			tess: ts[0],
			nodes: getICSNodes(
				types.NewNodeSet(obj[0].Ids, pk[:2]),
				types.NewNodeSet(obj[1].Ids, pk[2:4]),
				types.NewNodeSet(obj[2].Ids, pk[4:6]),
				types.NewNodeSet(obj[3].Ids, pk[6:8]),
			),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.mockFn != nil {
				test.mockFn()
			}

			icsNodes, err := sm.FetchContextLock(test.tess)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			if test.nodes.senderBeh != nil {
				checkIfNodesetEqual(
					t,
					test.nodes.senderBeh,
					test.nodes.senderRand,
					icsNodes.Nodes[types.SenderBehaviourSet],
					icsNodes.Nodes[types.SenderRandomSet],
				)
			}
			if test.nodes.receiverBeh != nil {
				checkIfNodesetEqual(
					t,
					test.nodes.receiverBeh,
					test.nodes.receiverRand,
					icsNodes.Nodes[types.ReceiverBehaviourSet],
					icsNodes.Nodes[types.ReceiverRandomSet],
				)
			}
		})
	}
}

func TestIsAccountRegistered_With_SargaObject(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(types.SargaAddress, cache, mockJournal(), db, types.Account{
		AccType: types.SargaAccount,
	})
	_, err = so.createStorageTreeForLogic(types.SargaLogicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(types.SargaLogicID, address.Bytes(), []byte{0x01})
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)
	assert.NoError(t, so.flush())

	ts := tests.CreateTesseract(t, getTesseractParamsWithStateHash(types.SargaAddress, stateHash))
	smParams := &createStateManagerParams{
		db: db,
		smCallBack: func(sm *StateManager) {
			insertTesseractsInDB(t, db, ts)
			sm.cache.Add(ts.Address(), tests.GetTesseractHash(t, ts))
			insertAccountsInDB(t, db, []types.Hash{stateHash}, so.Data())
		},
	}

	testcases := []struct {
		name          string
		address       types.Address
		smParams      *createStateManagerParams
		isRegistered  bool
		expectedError error
	}{
		{
			name:         "registered account",
			address:      address,
			smParams:     smParams,
			isRegistered: true,
		},
		{
			name:         "non-registered account",
			address:      tests.RandomAddress(t),
			smParams:     smParams,
			isRegistered: false,
		},
		{
			name:         "nil address",
			address:      types.NilAddress,
			smParams:     smParams,
			isRegistered: true,
		},
		{
			name:          "sarga object not found",
			address:       tests.RandomAddress(t),
			smParams:      nil,
			isRegistered:  true,
			expectedError: types.ErrObjectNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := createTestStateManager(t, test.smParams)

			isRegistered, err := sm.IsAccountRegistered(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.isRegistered, isRegistered)
		})
	}
}

func TestGetNonce(t *testing.T) {
	accounts0 := &types.Account{
		Nonce: 12,
	}

	stateHash0, err := accounts0.Hash()
	assert.NoError(t, err)

	account1 := &types.Account{
		Nonce: 22,
	}

	stateHash1, err := account1.Hash()
	assert.NoError(t, err)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Address: tests.RandomAddress(t),
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = stateHash1
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []types.Hash{stateHash0}, accounts0)
			insertAccountsInDB(t, db, []types.Hash{ts.StateHash()}, account1)
			insertTesseractsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(ts.Address(), tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		stateHash     types.Hash
		nonce         uint64
		expectedError error
	}{
		{
			name:    "fetch nonce from latest state",
			address: ts.Address(),
			nonce:   account1.Nonce,
		},
		{
			name:      "fetch nonce at particular state",
			address:   tests.RandomAddress(t),
			stateHash: stateHash0,
			nonce:     accounts0.Nonce,
		},
		{
			name:          "should return error if failed to fetch nonce",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch state object"),
		},
		{
			name:          "nil address",
			address:       types.NilAddress,
			expectedError: types.ErrInvalidAddress,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nonce, err := sm.GetNonce(test.address, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.nonce, nonce)
		})
	}
}

func TestGetBalances(t *testing.T) {
	assets, bal := getAssetIDsAndBalances(t, 2)
	balances, balanceHashes := getTestBalances(t, getAssetMaps(assets, bal, 1), 2)

	accounts0 := &types.Account{
		Nonce:   12,
		Balance: balanceHashes[0],
	}

	stateHash0, err := accounts0.Hash()
	assert.NoError(t, err)

	account1 := &types.Account{
		Nonce:   22,
		Balance: balanceHashes[1],
	}

	stateHash1, err := account1.Hash()
	assert.NoError(t, err)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Address: tests.RandomAddress(t),
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = stateHash1
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []types.Hash{stateHash0}, accounts0)
			insertAccountsInDB(t, db, []types.Hash{ts.StateHash()}, account1)
			insertBalancesInDB(t, db, balanceHashes, balances...)
			insertTesseractsInDB(t, db, ts)
		},

		smCallBack: func(sm *StateManager) {
			sm.cache.Add(ts.Address(), tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		stateHash     types.Hash
		balance       *gtypes.BalanceObject
		expectedError error
	}{
		{
			name:    "fetch balances from latest state",
			address: ts.Address(),
			balance: balances[1],
		},
		{
			name:      "fetch balances at particular state",
			address:   tests.RandomAddress(t),
			stateHash: stateHash0,
			balance:   balances[0],
		},
		{
			name:          "failed to fetch balances",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := sm.GetBalances(test.address, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.balance, balance)
		})
	}
}

func TestGetBalance(t *testing.T) {
	assetIDs, bal := getAssetIDsAndBalances(t, 2)
	balances, balanceHashes := getTestBalances(t, getAssetMaps(assetIDs, bal, 1), 2)
	accounts, stateHashes := getTestAccounts(t, balanceHashes, 1)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = stateHashes[0]
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts)
			insertAccountsInDB(t, db, stateHashes, accounts...)
			insertBalancesInDB(t, db, balanceHashes, balances...)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(ts.Address(), tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		assetID       types.AssetID
		stateHash     types.Hash
		balance       *gtypes.BalanceObject
		expectedError error
	}{
		{
			name:    "fetch balance from latest state",
			address: ts.Address(),
			assetID: assetIDs[0],
			balance: balances[0],
		},
		{
			name:      "fetch balance at particular state",
			address:   ts.Address(),
			assetID:   assetIDs[0],
			stateHash: stateHashes[0],
			balance:   balances[0],
		},
		{
			name:          "should return error if asset not found",
			address:       ts.Address(),
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: types.ErrAssetNotFound,
		},
		{
			name:          "should return error if failed to fetch balance",
			address:       tests.RandomAddress(t),
			assetID:       assetIDs[0],
			expectedError: errors.New("failed to fetch state object"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := sm.GetBalance(test.address, test.assetID, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.balance.AssetMap[test.assetID], balance)
		})
	}
}

func TestFetchLatestParticipantContext(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
	contract := NewMockContract(t, append(kramaIDs[:3], kramaIDs[7:10]...), append(pk[:3], pk[7:10]...))
	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[0]),
		1: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[1]),
		2: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[2]),
	}

	ts := tests.CreateTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts...)
		},
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsInDB(t, db, ts...)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		address       types.Address
		ctxHash       types.Hash
		behSet        *types.NodeSet
		randSet       *types.NodeSet
		mockFn        func()
		expectedError error
	}{
		{
			name:          "tesseract doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:    "behavioural context Nodes doesn't have public keys",
			address: ts[1].Address(),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
			expectedError: types.ErrPublicKeyNotFound,
		},
		{
			name:    "random context Nodes doesn't have public keys",
			address: ts[2].Address(),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
			expectedError: types.ErrPublicKeyNotFound,
		},
		{
			name:    "valid hash and public keys",
			address: ts[0].Address(),
			ctxHash: ts[0].ContextHash(),
			behSet:  types.NewNodeSet(obj[0].Ids, pk[:2]),
			randSet: types.NewNodeSet(obj[1].Ids, pk[2:4]),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.mockFn != nil {
				test.mockFn()
			}

			hash, behSet, randSet, err := sm.fetchLatestParticipantContext(test.address)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.ctxHash, hash)
			checkIfNodesetEqual(
				t,
				test.behSet,
				test.randSet,
				behSet,
				randSet,
			)
		})
	}
}

func TestGetReceiverContext_RegisteredAccount(t *testing.T) {
	db := mockDB()
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	contract := NewMockContract(t, kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[0])

	ts := tests.CreateTesseract(t, tesseractParams)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(types.NilAddress, ts.Address()),
		1: tests.GetIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
	}

	ixs := tests.CreateIxns(t, 2, ixParams)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(types.SargaAddress, cache, mockJournal(), db, types.Account{
		AccType: types.SargaAccount,
	})
	_, err = so.createStorageTreeForLogic(types.SargaLogicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(types.SargaLogicID, ixs[0].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)

	err = so.SetStorageEntry(types.SargaLogicID, ixs[1].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)

	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)
	assert.NoError(t, so.flush())

	sargaTesseract := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Address: types.SargaAddress,
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = stateHash
		},
	})

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts)
			insertAccountsInDB(t, db, []types.Hash{stateHash}, so.Data())
			insertTesseractsInDB(t, db, sargaTesseract)
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
			storeTesseractHashInCache(t, sm.cache, sargaTesseract)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		ix            *types.Interaction
		behSet        *types.NodeSet
		randSet       *types.NodeSet
		address       types.Address
		contextHash   types.Hash
		mockFn        func()
		expectedError error
	}{
		{
			name:        "context of receiver found",
			ix:          ixs[0],
			behSet:      types.NewNodeSet(obj[0].Ids, pk[:2]),
			randSet:     types.NewNodeSet(obj[1].Ids, pk[2:4]),
			address:     ixs[0].Receiver(),
			contextHash: ts.ContextHash(),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name:          "failed to fetch receiver context",
			ix:            ixs[1],
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nodeSet := make([]*types.NodeSet, 4)
			contextHashes := make(map[types.Address]types.Hash)

			if test.mockFn != nil {
				test.mockFn()
			}

			err := sm.getReceiverContext(test.ix, nodeSet, contextHashes)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(t,
				test.behSet,
				test.randSet,
				nodeSet[types.ReceiverBehaviourSet],
				nodeSet[types.ReceiverRandomSet],
			)
			require.Equal(t, test.contextHash, contextHashes[test.address])
		})
	}
}

func TestGetReceiverContext_Non_RegisteredAccount(t *testing.T) {
	db := mockDB()
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	contract := NewMockContract(t, kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
		1: tests.GetIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
	}

	ixs := tests.CreateIxns(t, 3, ixParams)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(types.SargaAddress, cache, mockJournal(), db, types.Account{
		AccType:     types.SargaAccount,
		ContextHash: mHash[0],
	})

	_, err = so.createStorageTreeForLogic(types.SargaLogicID)
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	assert.NoError(t, so.flush())

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Address: types.SargaAddress,
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = stateHash
			body.ContextHash = mHash[0]
		},
	})

	testcases := []struct {
		name          string
		ix            *types.Interaction
		soParams      *createStateObjectParams
		smParams      *createStateManagerParams
		behSet        *types.NodeSet
		randSet       *types.NodeSet
		address       types.Address
		contextHash   types.Hash
		mockFn        func()
		errorExpected bool
	}{
		{
			name: "context of sarga account found",
			ix:   ixs[0],
			smParams: &createStateManagerParams{
				db: db,
				dbCallback: func(db *MockDB) {
					insertTesseractsInDB(t, db, ts)
					insertAccountsInDB(t, db, []types.Hash{stateHash}, so.Data())
					insertMetaContextsInDB(t, db, mObj...)
					insertContextsInDB(t, db, obj...)
				},
				smCallBack: func(sm *StateManager) {
					storeTesseractHashInCache(t, sm.cache, ts)
				},
			},
			behSet:      types.NewNodeSet(obj[0].Ids, pk[:2]),
			randSet:     types.NewNodeSet(obj[1].Ids, pk[2:4]),
			address:     types.SargaAddress,
			contextHash: ts.ContextHash(),
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name:          "with out sarga object",
			ix:            ixs[2],
			errorExpected: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := createTestStateManager(t, test.smParams)
			nodeSet := make([]*types.NodeSet, 4)
			contextHashes := make(map[types.Address]types.Hash)

			if test.mockFn != nil {
				test.mockFn()
			}

			err = sm.getReceiverContext(test.ix, nodeSet, contextHashes)
			if test.errorExpected {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			checkIfNodesetEqual(t,
				test.behSet,
				test.randSet,
				nodeSet[types.ReceiverBehaviourSet],
				nodeSet[types.ReceiverRandomSet],
			)
			require.Equal(t, test.contextHash, contextHashes[test.address])
		})
	}
}

func TestFetchInteractionContext(t *testing.T) {
	db := mockDB()
	addrs := getAddresses(t, 2)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	contract := NewMockContract(t, kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(addrs[0], addrs[1]),
		1: tests.GetIxParamsWithAddress(types.NilAddress, types.NilAddress),
	}

	ixs := tests.CreateIxns(t, 2, ixParams)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(types.SargaAddress, cache, mockJournal(), db, types.Account{
		AccType: types.SargaAccount,
	})

	_, err = so.createStorageTreeForLogic(types.SargaLogicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(types.SargaLogicID, ixs[0].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)
	err = so.SetStorageEntry(types.SargaLogicID, ixs[1].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	assert.NoError(t, so.flush())

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithContextHash(ixs[0].Sender(), mHash[0]),
		1: getTesseractParamsWithContextHash(ixs[0].Receiver(), mHash[1]),
		2: getTesseractParamsWithStateHash(types.SargaAddress, stateHash),
	}

	ts := tests.CreateTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []types.Hash{stateHash}, so.Data())
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsInDB(t, db, ts...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts...)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		ix            *types.Interaction
		ics           *ICSNodes
		contextHashes map[types.Address]types.Hash
		mockFn        func()
		expectedError error
	}{
		{
			name: "both sender and receiver addresses has context",
			ix:   ixs[0],
			ics: getICSNodes(
				types.NewNodeSet(obj[0].Ids, pk[:2]),
				types.NewNodeSet(obj[1].Ids, pk[2:4]),
				types.NewNodeSet(obj[2].Ids, pk[4:6]),
				types.NewNodeSet(obj[3].Ids, pk[6:8]),
			),
			contextHashes: map[types.Address]types.Hash{
				ixs[0].Sender():   ts[0].ContextHash(),
				ixs[0].Receiver(): ts[1].ContextHash(),
			},
			mockFn: func() {
				retrievePublicKeys(t, contract)
			},
		},
		{
			name: "both sender and receiver addresses don't have context",
			ix:   ixs[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.mockFn != nil {
				test.mockFn()
			}

			contextHashes, nodeSet, err := sm.FetchInteractionContext(context.Background(), test.ix)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if test.ics == nil {
				require.Equal(t, len(test.contextHashes), 0)
				require.Nil(t, nodeSet[types.SenderBehaviourSet])
				require.Nil(t, nodeSet[types.SenderRandomSet])
				require.Nil(t, nodeSet[types.ReceiverBehaviourSet])
				require.Nil(t, nodeSet[types.ReceiverRandomSet])

				return
			}
			checkIfNodesetEqual(
				t,
				test.ics.senderBeh,
				test.ics.senderRand,
				nodeSet[types.SenderBehaviourSet],
				nodeSet[types.SenderRandomSet],
			)
			checkIfNodesetEqual(
				t,
				test.ics.receiverBeh,
				test.ics.receiverRand,
				nodeSet[types.ReceiverBehaviourSet],
				nodeSet[types.ReceiverRandomSet],
			)
			for i := range test.contextHashes {
				require.Equal(t, test.contextHashes[i], contextHashes[i])
			}
		})
	}
}

func TestGetAccountInfo(t *testing.T) {
	hash := tests.RandomHash(t)
	acc := &types.Account{
		Balance: tests.RandomHash(t),
	}

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			db.setAccount(t, hash, acc)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name            string
		stateHash       types.Hash
		expectedAccount *types.Account
		expectedError   error
	}{
		{
			name:            "account exists",
			stateHash:       hash,
			expectedAccount: acc,
		},
		{
			name:            "account doesn't exist",
			stateHash:       tests.RandomHash(t),
			expectedAccount: acc,
			expectedError:   types.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			acc, err := sm.GetAccountState(types.NilAddress, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccount, acc)
		})
	}
}

func TestGetAccTypeUsingStateObject(t *testing.T) {
	soParams := &createStateObjectParams{
		journal: mockJournal(),
		account: &types.Account{AccType: types.LogicAccount},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.dirtyObjects[so.address] = so
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		sObj          *StateObject
		expectedError error
	}{
		{
			name:    "state object exists",
			address: so.address,
			sObj:    so,
		},
		{
			name:          "state object doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: types.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accType, err := sm.GetAccTypeUsingStateObject(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, accType, types.LogicAccount)
		})
	}
}

func TestFlushDirtyObject(t *testing.T) {
	db := mockDB()
	dirtyEntries := getDirtyEntries(t, 5)
	keys, values := getEntries(t, 4)

	merkle := mockMerkleTreeWithDB()
	soParams := map[int]*createStateObjectParams{
		0: {
			db: db,
			metaStorageTreeCallback: func(so *StateObject) {
				so.metaStorageTree = merkle
				setEntries(t, merkle, keys, values)
			},
			soCallback: func(so *StateObject) {
				so.dirtyEntries = dirtyEntries
			},
		},
		1: {
			db: db,
			metaStorageTreeCallback: func(so *StateObject) {
				m := mockMerkleTreeWithDB()
				m.flushHook = func() error {
					return errors.New("flush failed")
				}
				so.metaStorageTree = m
			},
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	smCallback := func(sm *StateManager) {
		sm.cache = mockCache(t)
		insertDirtyObject(sm, so...)
	}

	smParams := &createStateManagerParams{
		db:         db,
		smCallBack: smCallback,
	}

	testcases := []struct {
		name          string
		address       types.Address
		smParams      *createStateManagerParams
		expectedError error
	}{
		{
			name:     "state object exists",
			address:  so[0].address,
			smParams: smParams,
		},
		{
			name:          "state object doesn't exist",
			address:       tests.RandomAddress(t),
			smParams:      smParams,
			expectedError: types.ErrKeyNotFound,
		},
		{
			name:          "flush failed",
			address:       so[1].address,
			smParams:      smParams,
			expectedError: errors.New("failed to flush active storage trees"),
		},
		{
			name:    "db failure",
			address: so[0].address,
			smParams: &createStateManagerParams{
				dbCallback: func(db *MockDB) {
					db.createEntryHook = func() error {
						return types.ErrKeyNotFound
					}
				},
				smCallBack: smCallback,
			},
			expectedError: errors.New("failed to write dirty entries"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := createTestStateManager(t, test.smParams)
			err := sm.FlushDirtyObject(test.address)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if meta storage tree entries flushed to db
			for i := 0; i < len(keys); i += 1 {
				val, err := merkle.dbStorage[string(keys[i])]
				require.True(t, err)
				require.Equal(t, values[i], val)
			}

			// check if dirty entries flushed to db
			for k, v := range dirtyEntries {
				val, err := sm.db.ReadEntry(types.Hex2Bytes(k))
				require.NoError(t, err)
				require.Equal(t, v, val)
			}
		})
	}
}

func TestIsAccountRegisteredAt(t *testing.T) {
	addresses := getAddresses(t, 3)
	db := mockDB()
	_, storageRoot := createMetaStorageTree(
		t,
		db,
		types.SargaAddress,
		types.SargaLogicID,
		[][]byte{addresses[2].Bytes()},
		[][]byte{types.NilHash.Bytes()},
	)

	balance, balanceHash := getTestBalance(t, getAssetMap(getAssetIDsAndBalances(t, 2)))

	cache, err := lru.New(100)
	require.NoError(t, err)

	so := NewStateObject(types.SargaAddress, cache, mockJournal(), db, types.Account{
		StorageRoot: storageRoot,
	})

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithStateHash(types.SargaAddress, stateHash),
		1: getTesseractParamsWithStateHash(tests.RandomAddress(t), tests.RandomHash(t)),
	}

	tesseracts := tests.CreateTesseracts(t, 2, tesseractParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts...)
			insertAccountsInDB(t, db, []types.Hash{stateHash}, so.Data())
			insertBalancesInDB(t, db, []types.Hash{balanceHash}, balance)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name                string
		tsHash              types.Hash
		address             types.Address
		isAccountRegistered bool
		expectedError       error
	}{
		{
			name:          "should fail if tesseract not found",
			tsHash:        tests.RandomHash(t),
			address:       addresses[0],
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name:          "should fail if state object not found",
			tsHash:        tesseracts[1].Hash(),
			address:       addresses[0],
			expectedError: types.ErrStateNotFound,
		},
		{
			name:                "non-registered account",
			tsHash:              tesseracts[0].Hash(),
			address:             addresses[1],
			isAccountRegistered: false,
		},
		{
			name:                "registered account",
			tsHash:              tesseracts[0].Hash(),
			address:             addresses[2],
			isAccountRegistered: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			isRegistered, err := sm.IsAccountRegisteredAt(test.address, test.tsHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.isAccountRegistered, isRegistered)
		})
	}
}

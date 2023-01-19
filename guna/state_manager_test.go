package guna

import (
	"context"
	"errors"
	"math/big"
	"testing"

	ktypes "github.com/sarvalabs/moichain/krama/types"

	"github.com/sarvalabs/moichain/guna/tree"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/stretchr/testify/require"
)

func TestCreateStateObject(t *testing.T) {
	address := tests.RandomAddress(t)
	accType := types.ContractAccount

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
	do := sm.CreateDirtyObject(address, 1)

	dirtyObject, ok := sm.dirtyObjects[address]
	require.True(t, ok)
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
	tesseractParams := getTesseractParamsMapWithIxns(t, 1, 2)
	tesseracts := createTesseracts(t, 2, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts...)
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
			name:             "with interactions",
			hash:             getTesseractHash(t, tesseracts[0]),
			withInteractions: true,
			expectedTS:       tesseracts[0],
			expectedError:    nil,
		},
		{
			name:             "without interactions",
			hash:             getTesseractHash(t, tesseracts[0]),
			withInteractions: false,
			expectedTS:       tesseracts[0],
			expectedError:    nil,
		},
		{
			name:             "should fail if tesseract not found",
			hash:             tests.RandomHash(t),
			withInteractions: false,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "should fail if interactions not found",
			hash:             getTesseractHash(t, tesseracts[1]),
			withInteractions: true,
			expectedError:    types.ErrFetchingInteractions,
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
	tesseractParams := getTesseractParamsMapWithIxns(t, 2, 2)
	tesseracts := createTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts[:2]...)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(getTesseractHash(t, tesseracts[2]), tesseracts[2])
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
			hash:             getTesseractHash(t, tesseracts[2]),
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
			hash:             getTesseractHash(t, tesseracts[0]),
			withInteractions: true,
			expectedTS:       tesseracts[0],
			expectedError:    nil,
		},
		{
			name:             "without interactions",
			hash:             getTesseractHash(t, tesseracts[1]),
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
	balances, balanceHashes := getTestBalances(t, 2)
	accounts, stateHashes := getTestAccounts(t, balanceHashes, 2)

	soParams := map[int]*createStateObjectParams{
		0: stateObjectParamsWithBalance(t, balanceHashes[0], balances[0]), // Add balance as we validate balance
		1: stateObjectParamsWithBalance(t, balanceHashes[1], balances[1]),
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
			name:          "state object exists",
			stateHash:     stateHashes[0],
			address:       so[0].address,
			sObj:          so[0],
			expectedError: nil,
		},
		{
			name:          "should fail if account not found",
			stateHash:     tests.RandomHash(t),
			address:       types.NilAddress,
			expectedError: types.ErrStateNotFound,
		},
		{
			name:          "should fail if balance not found",
			stateHash:     stateHashes[1],
			address:       so[1].address,
			expectedError: types.ErrFetchingBalance,
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
	balances, balanceHashes := getTestBalances(t, 2)
	accounts, stateHashes := getTestAccounts(t, balanceHashes, 2)

	soParams := map[int]*createStateObjectParams{
		0: stateObjectParamsWithBalance(t, balanceHashes[0], balances[0]), // Add balance as we validate balance
		1: stateObjectParamsWithBalance(t, balanceHashes[1], balances[1]),
	}

	so := createTestStateObjects(t, 2, soParams)

	tesseractParams := map[int]*createTesseractParams{
		0: getTesseractParamsWithStateHash(so[0].address, stateHashes[0]),
		1: getTesseractParamsWithStateHash(tests.RandomAddress(t), tests.RandomHash(t)),
	}

	tesseracts := createTesseracts(t, 2, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
			insertBalancesInDB(t, db, balanceHashes, balances...)
			insertTesseractsInDB(t, db, tesseracts...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
			insertStateObject(sm, so[1])
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
			name:          "state object exists in state manager",
			address:       so[1].address,
			sObj:          so[1],
			expectedError: nil,
		},
		{
			name:          "state object constructed from db",
			address:       so[0].address,
			sObj:          so[0],
			expectedError: nil,
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

func TestGetLatestTesseract(t *testing.T) {
	tesseracts := createTesseracts(t,
		2,
		getTesseractParamsMapWithIxns(
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
	balances, balanceHashes := getTestBalances(t, 2)

	soParams := map[int]*createStateObjectParams{
		0: stateObjectParamsWithBalance(t, balanceHashes[0], balances[0]), // Add balance as we validate it
		1: stateObjectParamsWithBalance(t, balanceHashes[1], balances[1]),
	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertDirtyObject(sm, so[0])
			insertObject(sm, so[1])
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
			name:          "address in state manager's objects",
			address:       so[0].address,
			sObj:          so[0],
			expectedError: nil,
		},
		{
			name:          "address in state manager's dirty object",
			address:       so[1].address,
			sObj:          so[1],
			expectedError: nil,
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
			insertObject(sm, so)
			insertDirtyObject(sm, so)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name    string
		address types.Address
	}{
		{
			name:    "object exists",
			address: so.address,
		},
		{
			name:    "object doesn't exist",
			address: tests.RandomAddress(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm.Cleanup(test.address)

			checkForDirtyObject(t, sm, test.address, false)
			checkForObject(t, sm, test.address, false)
		})
	}
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
			name:          "context object in cache",
			hash:          cHash[0],
			ctx:           obj[0],
			expectedError: nil,
		},
		{
			name:          "context object in db",
			hash:          cHash[1],
			ctx:           obj[1],
			expectedError: nil,
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
			name:          "meta context object in cache",
			hash:          hashes[0],
			ctx:           mObj[0],
			expectedError: nil,
		},
		{
			name:          "meta context object in db",
			hash:          hashes[1],
			ctx:           mObj[1],
			expectedError: nil,
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
			name:          "valid context",
			hash:          mHash[0],
			behCtx:        obj[0],
			randCtx:       obj[1],
			expectedError: nil,
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

	ts := createTesseract(t, getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[1]))

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
			address:       types.NilAddress,
			hash:          mHash[0],
			behCtx:        obj[0],
			randCtx:       obj[1],
			expectedError: nil,
		},
		{
			name:          "valid tesseract address",
			address:       ts.Address(),
			hash:          types.NilHash,
			behCtx:        obj[2],
			randCtx:       obj[3],
			expectedError: nil,
		},
		{
			name:          "tesseract doesn't exist",
			address:       tests.RandomAddress(t),
			hash:          types.NilHash,
			expectedError: errors.New("failed to fetch latest tesseract hash"),
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
	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
		senatusCallBack: func(s *MockSenatus) {
			setPublicKeys(s, kramaIDs, pk)
			removePublicKeys(s, kramaIDs[6:10])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		hash          types.Hash
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
		},
		{
			name:          "random context node's public keys not found",
			hash:          mHash[1],
			expectedError: types.ErrPublicKeyNotFound,
		},
		{
			name: "valid hash and public keys",
			hash: mHash[0],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			behCtx, randCtx, err := sm.fetchParticipantContextByHash(types.NilAddress, test.hash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(
				t,
				ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
				ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
				behCtx,
				randCtx,
			)
		})
	}
}

func TestGetCommittedContextHash(t *testing.T) {
	ts := createTesseract(t, getTesseractParamsWithContextHash(tests.RandomAddress(t), tests.RandomHash(t)))

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
			name:          "context exists",
			address:       ts.Address(),
			contextHash:   ts.ContextHash(),
			expectedError: nil,
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
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	addresses := getAddresses(t, 10)
	ixns := createIxns(t, 5, getIxParamsMapWithAddresses(addresses[:5], addresses[5:10]))

	getTesseractParams := func(
		ixns types.Interactions,
		addresses []types.Address,
		hashes ...types.Hash,
	) *createTesseractParams {
		return &createTesseractParams{
			ixns: ixns,
			tesseractCallback: func(ts *types.Tesseract) {
				ts.Header.ContextLock = mockContextLock()
				for i, address := range addresses {
					insertInContextLock(ts, address, hashes[i])
				}
			},
		}
	}

	tesseractParams := map[int]*createTesseractParams{
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
			[]types.Address{SargaAddress},
			mHash[1],
		),
	}

	ts := createTesseracts(t, 5, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
		senatusCallBack: func(s *MockSenatus) {
			setPublicKeys(s, kramaIDs, pk)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		tess          *types.Tesseract
		nodes         *ICSNodes
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
				ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
				ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
				nil,
				nil,
			),

			expectedError: nil,
		},
		{
			name: "sarga address in context lock",
			tess: ts[4],
			nodes: getICSNodes(
				nil, nil,
				ktypes.NewNodeSet(obj[2].Ids, pk[4:6]),
				ktypes.NewNodeSet(obj[3].Ids, pk[6:8]),
			),
			expectedError: nil,
		},
		{
			name: "valid context hashes",
			tess: ts[0],
			nodes: getICSNodes(
				ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
				ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
				ktypes.NewNodeSet(obj[2].Ids, pk[4:6]),
				ktypes.NewNodeSet(obj[3].Ids, pk[6:8]),
			),
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
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
					icsNodes.Nodes[ktypes.SenderBehaviourSet],
					icsNodes.Nodes[ktypes.SenderRandomSet],
				)
			}
			if test.nodes.receiverBeh != nil {
				checkIfNodesetEqual(
					t,
					test.nodes.receiverBeh,
					test.nodes.receiverRand,
					icsNodes.Nodes[ktypes.ReceiverBehaviourSet],
					icsNodes.Nodes[ktypes.ReceiverRandomSet],
				)
			}
		})
	}
}

func TestIsAccountRegistered_With_SargaObject(t *testing.T) {
	address := tests.RandomAddress(t)
	soParams := &createStateObjectParams{
		address: SargaAddress,
		activeStorageTreeCallback: func(activeStorageTrees map[string]tree.MerkleTree) {
			m := mockMerkleTreeWithDirtyStorage()
			err := m.Set(address.Bytes(), nil)
			require.NoError(t, err)

			activeStorageTrees[SargaLogicID.Hex()] = m
		},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertStateObject(sm, so)
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

func TestGetLatestNonce(t *testing.T) {
	soParams := &createStateObjectParams{
		address: SargaAddress,
		account: &types.Account{
			Nonce: 5,
		},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertStateObject(sm, so)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		nonce         uint64
		expectedError error
	}{
		{
			name:          "state object found",
			address:       so.address,
			nonce:         5,
			expectedError: nil,
		},
		{
			name:          "state object not found",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:          "nil address",
			address:       types.NilAddress,
			expectedError: types.ErrInvalidAddress,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nonce, err := sm.GetLatestNonce(test.address)
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
	soParams := &createStateObjectParams{
		address: SargaAddress,
		soCallback: func(so *StateObject) {
			AddAssetInBalance(t, so)
		},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertStateObject(sm, so)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		balance       *gtypes.BalanceObject
		expectedError error
	}{
		{
			name:          "state object found",
			address:       so.address,
			balance:       so.balance,
			expectedError: nil,
		},
		{
			name:          "state object not found",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := sm.GetBalances(test.address)
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
	soParams := &createStateObjectParams{
		address: SargaAddress,
		soCallback: func(so *StateObject) {
			so.balance.Balances[types.AssetID("MOI")] = big.NewInt(54332)
		},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			insertStateObject(sm, so)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       types.Address
		assetID       types.AssetID
		balance       *gtypes.BalanceObject
		expectedError error
	}{
		{
			name:          "state object found",
			address:       so.address,
			assetID:       types.AssetID("MOI"),
			balance:       so.balance,
			expectedError: nil,
		},
		{
			name:          "asset not found",
			address:       so.address,
			assetID:       types.AssetID("BTC"),
			expectedError: types.ErrAssetNotFound,
		},
		{
			name:          "state object not found",
			address:       tests.RandomAddress(t),
			assetID:       types.AssetID("MOI"),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := sm.GetBalance(test.address, test.assetID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.balance.Balances[test.assetID], balance)
		})
	}
}

func TestFetchLatestParticipantContext(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := map[int]*createTesseractParams{
		0: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[0]),
		1: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[1]),
		2: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[2]),
	}

	ts := createTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts...)
		},
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsInDB(t, db, ts...)
		},
		senatusCallBack: func(s *MockSenatus) {
			setPublicKeys(s, kramaIDs, pk)
			removePublicKeys(s, kramaIDs[4:6])
			removePublicKeys(s, kramaIDs[10:12])
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		address       types.Address
		ctxHash       types.Hash
		behSet        *ktypes.NodeSet
		randSet       *ktypes.NodeSet
		expectedError error
	}{
		{
			name:          "tesseract doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: types.ErrAccountNotFound,
		},
		{
			name:          "behavioural context Nodes doesn't have public keys",
			address:       ts[1].Address(),
			expectedError: types.ErrPublicKeyNotFound,
		},
		{
			name:          "random context Nodes doesn't have public keys",
			address:       ts[2].Address(),
			expectedError: types.ErrPublicKeyNotFound,
		},
		{
			name:          "valid hash and public keys",
			address:       ts[0].Address(),
			ctxHash:       ts[0].ContextHash(),
			behSet:        ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
			randSet:       ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
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
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[0])

	ts := createTesseract(t, tesseractParams)

	ixParams := map[int]*createIxParams{
		0: getIxParamsWithAddress(types.NilAddress, ts.Address()),
		1: getIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
	}

	ixs := createIxns(t, 2, ixParams)

	soParams := &createStateObjectParams{
		address: SargaAddress,
		activeStorageTreeCallback: func(activeStorageTrees map[string]tree.MerkleTree) {
			m := mockMerkleTreeWithDirtyStorage() // used to check if account registered

			storeInMerkleTree(t, m, ixs[0].Receiver().Bytes(), nil)
			storeInMerkleTree(t, m, ixs[1].Receiver().Bytes(), nil)

			activeStorageTrees[SargaLogicID.Hex()] = m
		},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts)
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
		smCallBack: func(sm *StateManager) {
			insertStateObject(sm, so)
			storeTesseractHashInCache(t, sm.cache, ts)
		},
		senatusCallBack: func(s *MockSenatus) {
			setPublicKeys(s, kramaIDs, pk)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		ix            *types.Interaction
		behSet        *ktypes.NodeSet
		randSet       *ktypes.NodeSet
		address       types.Address
		contextHash   types.Hash
		expectedError error
	}{
		{
			name:          "context of receiver found",
			ix:            ixs[0],
			behSet:        ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
			randSet:       ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
			address:       ixs[0].Receiver(),
			contextHash:   ts.ContextHash(),
			expectedError: nil,
		},
		{
			name:          "failed to fetch receiver context",
			ix:            ixs[1],
			expectedError: types.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nodeSet := make([]*ktypes.NodeSet, 4)
			contextHashes := make(map[types.Address]types.Hash)
			err := sm.getReceiverContext(test.ix, nodeSet, contextHashes)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(t,
				test.behSet,
				test.randSet,
				nodeSet[ktypes.ReceiverBehaviourSet],
				nodeSet[ktypes.ReceiverRandomSet],
			)
			require.Equal(t, test.contextHash, contextHashes[test.address])
		})
	}
}

func TestGetReceiverContext_Non_RegisteredAccount(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := getTesseractParamsWithContextHash(SargaAddress, mHash[0])

	ts := createTesseract(t, tesseractParams)

	ixParams := map[int]*createIxParams{
		0: getIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
		1: getIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
		2: getIxParamsWithAddress(types.NilAddress, tests.RandomAddress(t)),
	}

	ixs := createIxns(t, 3, ixParams)

	soParams := &createStateObjectParams{
		address: SargaAddress,
		activeStorageTreeCallback: func(activeStorageTrees map[string]tree.MerkleTree) {
			activeStorageTrees[SargaLogicID.Hex()] = mockMerkleTreeWithDirtyStorage()
		},
	}

	so := createTestStateObject(t, soParams)

	testcases := []struct {
		name          string
		ix            *types.Interaction
		soParams      *createStateObjectParams
		smParams      *createStateManagerParams
		behSet        *ktypes.NodeSet
		randSet       *ktypes.NodeSet
		address       types.Address
		contextHash   types.Hash
		expectedError error
	}{
		{
			name: "context of sarga account found",
			ix:   ixs[0],
			smParams: &createStateManagerParams{
				dbCallback: func(db *MockDB) {
					insertTesseractsInDB(t, db, ts)
					insertMetaContextsInDB(t, db, mObj...)
					insertContextsInDB(t, db, obj...)
				},
				smCallBack: func(sm *StateManager) {
					insertStateObject(sm, so)
					storeTesseractHashInCache(t, sm.cache, ts)
				},
				senatusCallBack: func(s *MockSenatus) {
					setPublicKeys(s, kramaIDs, pk)
				},
			},
			behSet:        ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
			randSet:       ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
			address:       SargaAddress,
			contextHash:   ts.ContextHash(),
			expectedError: nil,
		},
		{
			name: "context of sarga account not found",
			ix:   ixs[1],
			smParams: &createStateManagerParams{
				smCallBack: func(sm *StateManager) {
					insertStateObject(sm, so)
				},
			},
			expectedError: types.ErrAccountNotFound,
		},
		{
			name:          "with out sarga object",
			ix:            ixs[2],
			smParams:      nil,
			expectedError: types.ErrObjectNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := createTestStateManager(t, test.smParams)
			nodeSet := make([]*ktypes.NodeSet, 4)
			contextHashes := make(map[types.Address]types.Hash)

			err := sm.getReceiverContext(test.ix, nodeSet, contextHashes)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(t,
				test.behSet,
				test.randSet,
				nodeSet[ktypes.ReceiverBehaviourSet],
				nodeSet[ktypes.ReceiverRandomSet],
			)
			require.Equal(t, test.contextHash, contextHashes[test.address])
		})
	}
}

func TestFetchInteractionContext(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := map[int]*createTesseractParams{
		0: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[0]),
		1: getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[1]),
	}

	ts := createTesseracts(t, 2, tesseractParams)

	ixParams := map[int]*createIxParams{
		0: getIxParamsWithAddress(ts[0].Address(), ts[1].Address()),
		1: getIxParamsWithAddress(types.NilAddress, types.NilAddress),
	}

	ixs := createIxns(t, 2, ixParams)

	soParams := &createStateObjectParams{
		address: SargaAddress,
		activeStorageTreeCallback: func(activeStorageTrees map[string]tree.MerkleTree) {
			m := mockMerkleTreeWithDirtyStorage() // used to check if account registered
			storeInMerkleTree(t, m, ixs[0].Receiver().Bytes(), nil)
			storeInMerkleTree(t, m, ixs[1].Receiver().Bytes(), nil)

			activeStorageTrees[SargaLogicID.Hex()] = m
		},
	}

	so := createTestStateObject(t, soParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsInDB(t, db, ts...)
		},
		smCallBack: func(sm *StateManager) {
			insertObject(sm, so)
			storeTesseractHashInCache(t, sm.cache, ts...)
		},
		senatusCallBack: func(s *MockSenatus) {
			setPublicKeys(s, kramaIDs, pk)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		ix            *types.Interaction
		ics           *ICSNodes
		contextHashes map[types.Address]types.Hash
		expectedError error
	}{
		{
			name: "both sender and receiver addresses has context",
			ix:   ixs[0],
			ics: getICSNodes(
				ktypes.NewNodeSet(obj[0].Ids, pk[:2]),
				ktypes.NewNodeSet(obj[1].Ids, pk[2:4]),
				ktypes.NewNodeSet(obj[2].Ids, pk[4:6]),
				ktypes.NewNodeSet(obj[3].Ids, pk[6:8]),
			),
			contextHashes: map[types.Address]types.Hash{
				ixs[0].Sender():   ts[0].ContextHash(),
				ixs[0].Receiver(): ts[1].ContextHash(),
			},
			expectedError: nil,
		},
		{
			name:          "both sender and receiver addresses don't have context",
			ix:            ixs[1],
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			contextHashes, nodeSet, err := sm.FetchInteractionContext(context.Background(), test.ix)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if test.ics == nil {
				require.Equal(t, len(test.contextHashes), 0)
				require.Nil(t, nodeSet[ktypes.SenderBehaviourSet])
				require.Nil(t, nodeSet[ktypes.SenderRandomSet])
				require.Nil(t, nodeSet[ktypes.ReceiverBehaviourSet])
				require.Nil(t, nodeSet[ktypes.ReceiverRandomSet])

				return
			}
			checkIfNodesetEqual(
				t,
				test.ics.senderBeh,
				test.ics.senderRand,
				nodeSet[ktypes.SenderBehaviourSet],
				nodeSet[ktypes.SenderRandomSet],
			)
			checkIfNodesetEqual(
				t,
				test.ics.receiverBeh,
				test.ics.receiverRand,
				nodeSet[ktypes.ReceiverBehaviourSet],
				nodeSet[ktypes.ReceiverRandomSet],
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
		accType: types.ContractAccount,
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
			name:          "state object exists",
			address:       so.address,
			sObj:          so,
			expectedError: nil,
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
			require.Equal(t, accType, types.ContractAccount)
		})
	}
}

func TestSetupSargaAcc(t *testing.T) {
	sm := createTestStateManager(t, nil)
	nodes := tests.GetTestKramaIDs(t, 12)

	var emptyNodes []id.KramaID

	testcases := []struct {
		name          string
		sargaAcc      *gtypes.AccountSetupArgs
		otherAccounts []*gtypes.AccountSetupArgs
		expectedError error
	}{
		{
			name: "behavioural nodes and random nodes are empty",
			sargaAcc: getAccountSetupArgs(t,
				SargaAddress,
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
			sargaAcc: getAccountSetupArgs(t,
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
			sargaAcc: getAccountSetupArgs(t,
				SargaAddress,
				nodes[:2],
				nodes[2:4],
				2,
				nil,
				nil,
			),
			otherAccounts: []*gtypes.AccountSetupArgs{
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
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, contextHash, err := sm.SetupSargaAccount(test.sargaAcc, test.otherAccounts)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForObjectCreation(t, sm, SargaAddress, contextHash)

			obj, _ := sm.GetDirtyObject(SargaAddress)

			checkForOtherAccountsInSargaObject(t, obj, test.otherAccounts)
		})
	}
}

func TestSetupNewAccount(t *testing.T) {
	sm := createTestStateManager(t, nil)
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
			expectedError: nil,
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
			expectedError: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, contextHash, err := sm.SetupNewAccount(test.newAcc)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForObjectCreation(t, sm, test.newAcc.Address, contextHash)

			obj, _ := sm.GetDirtyObject(test.newAcc.Address)

			// check if balances are added
			for assetID, balance := range test.newAcc.Balances {
				bal, err := obj.BalanceOf(assetID)
				require.NoError(t, err)

				require.Equal(t, bal, balance)
			}

			journalIndex := 3 // index is 3 as there will be 3 entries before asset creation
			for _, asset := range test.newAcc.Assets {
				checkForAssetCreation(t, obj, asset, journalIndex)
				journalIndex++
			}
		})
	}
}

func TestFlushDirtyObject(t *testing.T) {
	dirtyEntries := getDirtyEntries(t, 5)

	keys, values := getEntries(t, 4)

	merkle := mockMerkleTreeWithDB()
	soParams := map[int]*createStateObjectParams{
		0: {
			metaStorageTreeCallback: func(so *StateObject) {
				so.metaStorageTree = merkle
				setEntries(t, merkle, keys, values)
			},
			soCallback: func(so *StateObject) {
				so.dirtyEntries = dirtyEntries
			},
		},
		1: {
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
		smCallBack: smCallback,
	}

	testcases := []struct {
		name          string
		address       types.Address
		smParams      *createStateManagerParams
		expectedError error
	}{
		{
			name:          "state object exists",
			address:       so[0].address,
			smParams:      smParams,
			expectedError: nil,
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
		SargaAddress,
		SargaLogicID,
		[][]byte{addresses[2].Bytes()},
		[][]byte{types.NilHash.Bytes()},
	)

	balance, balanceHash := getTestBalance(t)

	acc, stateHash := getTestAccount(t, func(acc *types.Account) {
		acc.StorageRoot = storageRoot
	})

	soParams := &createStateObjectParams{
		address: SargaAddress,
		journal: mockJournal(),
		db:      db,
		soCallback: func(so *StateObject) {
			so.data = *acc
		},
	}

	so := createTestStateObject(t, soParams)

	tesseractParams := map[int]*createTesseractParams{
		0: getTesseractParamsWithStateHash(SargaAddress, stateHash),
		1: getTesseractParamsWithStateHash(tests.RandomAddress(t), tests.RandomHash(t)),
	}

	tesseracts := createTesseracts(t, 2, tesseractParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts...)
			insertAccountsInDB(t, db, []types.Hash{stateHash}, acc)
			insertBalancesInDB(t, db, []types.Hash{balanceHash}, balance)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
			insertStateObject(sm, so)
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
			tsHash:        getTesseractHash(t, tesseracts[1]),
			address:       addresses[0],
			expectedError: types.ErrStateNotFound,
		},
		{
			name:                "non-registered account",
			tsHash:              getTesseractHash(t, tesseracts[0]),
			address:             addresses[1],
			isAccountRegistered: false,
		},
		{
			name:                "registered account",
			tsHash:              getTesseractHash(t, tesseracts[0]),
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

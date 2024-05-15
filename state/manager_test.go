package state

import (
	"context"
	"errors"
	"testing"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/hashicorp/golang-lru"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
)

func TestStateManager_CreateStateObject(t *testing.T) {
	address := tests.RandomAddress(t)
	accType := common.LogicAccount

	sm := createTestStateManager(t, nil)
	so := sm.CreateStateObject(address, accType)

	validateStateObject(t, so, accType, address)
}

func TestStateManager_CleanupDirtyObject(t *testing.T) {
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

func TestStateManager_CreateDirtyObject(t *testing.T) {
	sm := createTestStateManager(t, nil)

	address := tests.RandomAddress(t)
	do := sm.CreateDirtyObject(address, common.SargaAccount)

	dirtyObject, ok := sm.dirtyObjects[address]

	require.True(t, ok)
	require.Equal(t, do.accType, common.SargaAccount)
	require.Equal(t, do, dirtyObject)
}

func TestStateManager_GetLatestTesseractHash(t *testing.T) {
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
		address       identifiers.Address
		hash          common.Hash
		expectedError error
	}{
		{
			name:          "should return error if nil address",
			address:       identifiers.NilAddress,
			hash:          accMetaInfo[0].TesseractHash,
			expectedError: common.ErrInvalidAddress,
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
			expectedError: common.ErrFetchingAccMetaInfo,
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

func TestStateManager_FetchTesseractFromDB(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)

	// Set the clusterID to genesis identifier to avoid fetching interactions
	tesseractParams[0].TSDataCallback = func(ts *tests.TesseractData) {
		ts.ConsensusInfo.ClusterID = common.GenesisIdentifier
	}

	tesseracts := tests.CreateTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, tesseracts[:2]...)
			insertTesseractInDB(t, db, tesseracts[2])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		hash             common.Hash
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:             "genesis tesseract",
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
			expectedError:    common.ErrFetchingTesseract,
		},
		{
			name:             "should fail if interactions not found",
			hash:             tesseracts[2].Hash(),
			withInteractions: true,
			expectedError:    common.ErrFetchingInteractions,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := sm.FetchTesseractFromDB(test.hash, test.withInteractions)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateTesseract(t, ts, test.expectedTS, test.withInteractions)
		})
	}
}

func TestStateManager_GetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseracts := tests.CreateTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, tesseracts[:2]...)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(tesseracts[2].Hash(), tesseracts[2])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		hash             common.Hash
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:             "fetches tesseract from cache", // only tesseracts without interactions exists in cache
			hash:             tesseracts[2].Hash(),
			withInteractions: false,
			expectedTS:       tesseracts[2],
		},
		{
			name:          "should fail if tesseract not found",
			hash:          tests.RandomHash(t),
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name:             "with interactions",
			hash:             tesseracts[1].Hash(),
			withInteractions: true,
			expectedTS:       tesseracts[1],
		},
		{
			name:             "without interactions",
			hash:             tesseracts[1].Hash(),
			withInteractions: false,
			expectedTS:       tesseracts[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := sm.getTesseractByHash(test.hash, test.withInteractions)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateTesseract(t, ts, test.expectedTS, test.withInteractions)
			checkForTesseractInSMCache(t, sm, ts, test.withInteractions)
		})
	}
}

func TestStateManager_GetStateObjectByHash(t *testing.T) {
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
		stateHash     common.Hash
		address       identifiers.Address
		sObj          *Object
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
			address:       identifiers.NilAddress,
			expectedError: common.ErrStateNotFound,
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

func TestStateManager_GetLatestStateObject(t *testing.T) {
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
			insertTesseractsAndIxnsInDB(t, db, tesseracts...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		sObj          *Object
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
			address:       tesseracts[1].AnyAddress(),
			expectedError: common.ErrStateNotFound,
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

func TestStateManager_GetStateObject(t *testing.T) {
	account, stateHash := getTestAccounts(t, []common.Hash{tests.RandomHash(t), tests.RandomHash(t)}, 2)

	so := NewStateObject(tests.RandomAddress(t), nil, nil, mockDB(), *account[0], NilMetrics())
	so1 := NewStateObject(tests.RandomAddress(t), nil, nil, mockDB(), *account[1], NilMetrics())

	ts := tests.CreateTesseract(t, getTesseractParamsWithStateHash(so1.Address(), stateHash[1]))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHash, account...)
			insertTesseractsAndIxnsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name      string
		address   identifiers.Address
		stateHash common.Hash
		sObj      *Object
	}{
		{
			name:      "fetch state object from state hash",
			address:   so.address,
			stateHash: stateHash[0],
			sObj:      so,
		},
		{
			name:      "fetch latest state object",
			address:   so1.address,
			stateHash: common.NilHash,
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

func TestStateManager_GetLatestTesseract(t *testing.T) {
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
			insertTesseractsAndIxnsInDB(t, db, tesseracts[1:]...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		address          identifiers.Address
		withInteractions bool
		expectedTS       *common.Tesseract
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
			address:          tesseracts[0].AnyAddress(),
			withInteractions: true,
			expectedTS:       tesseracts[0],
			expectedError:    common.ErrFetchingTesseract,
		},
		{
			name:             "with interactions",
			address:          tesseracts[1].AnyAddress(),
			withInteractions: true,
			expectedTS:       tesseracts[1],
			expectedError:    nil,
		},
		{
			name:             "without interactions",
			address:          tesseracts[1].AnyAddress(),
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

func TestStateManager_GetDirtyObject(t *testing.T) {
	soParams := map[int]*createStateObjectParams{
		0: {
			address: tests.RandomAddress(t),
			account: &common.Account{
				Nonce: 2,
			},
		},
		1: {
			address: tests.RandomAddress(t),
			account: &common.Account{
				Nonce: 4,
			},
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	stateHash, err := so[1].Commit()
	assert.NoError(t, err)

	ts := tests.CreateTesseract(t, getTesseractParamsWithStateHash(so[1].address, stateHash))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, ts)
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so[1].Data())
		},
		smCallBack: func(sm *StateManager) {
			insertDirtyObject(sm, so[0])
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		sObj          *Object
		expectedError error
	}{
		{
			name:    "should retrieve state object from dirty object",
			address: so[0].address,
			sObj:    so[0],
		},
		{
			name:    "should retrieve state object from db",
			address: ts.AnyAddress(),
			sObj:    so[1],
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

func TestStateManager_Cleanup(t *testing.T) {
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

func TestStateManager_GetContextObject(t *testing.T) {
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
		hash          common.Hash
		ctx           *ContextObject
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
			expectedError: common.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctx, err := sm.getContextObject(identifiers.NilAddress, test.hash)

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

func TestStateManager_GetMetaContextObject(t *testing.T) {
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
		hash          common.Hash
		ctx           *MetaContextObject
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
			expectedError: common.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctx, err := sm.getMetaContextObject(identifiers.NilAddress, test.hash)
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

func TestStateManager_GetContext(t *testing.T) {
	mObj := make([]*MetaContextObject, 3)
	mHash := make([]common.Hash, 3)

	kramaIDs, _ := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj[0], mHash[0] = getMetaContextObject(t, cHash[0], cHash[1])
	mObj[1], mHash[1] = getMetaContextObject(t, cHash[2], common.NilHash)
	mObj[2], mHash[2] = getMetaContextObject(t, common.NilHash, cHash[3])

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		hash          common.Hash
		behCtx        *ContextObject
		randCtx       *ContextObject
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
			behCtx, randCtx, err := sm.getContext(identifiers.NilAddress, test.hash)

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

func TestStateManager_GetContextByHash(t *testing.T) {
	kramaIDs, _ := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ts := tests.CreateTesseract(t, getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[1]))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsAndIxnsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		hash          common.Hash
		behCtx        *ContextObject
		randCtx       *ContextObject
		expectedError error
	}{
		{
			name:          "address and context hash are nil",
			address:       identifiers.NilAddress,
			hash:          common.NilHash,
			expectedError: common.ErrEmptyHashAndAddress,
		},
		{
			name:          "valid context hash",
			address:       ts.AnyAddress(),
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

func TestStateManager_FetchParticipantContextByHash(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
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
		hash          common.Hash
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
			expectedError: common.ErrPublicKeyNotFound,
		},
		{
			name:          "random context node's public keys not found",
			hash:          mHash[1],
			expectedError: common.ErrPublicKeyNotFound,
			mockFn: func() {
				msenatus := mockSenatus(t)
				msenatus.AddPublicKeys(kramaIDs[4:6], pk[4:6])

				sm.senatus = msenatus
			},
		},
		{
			name: "valid hash and public keys",
			hash: mHash[0],
			mockFn: func() {
				msenatus := mockSenatus(t)
				_ = msenatus.AddPublicKeys(kramaIDs, pk)

				sm.senatus = msenatus
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.mockFn != nil {
				test.mockFn()
			}

			behCtx, randCtx, err := sm.fetchParticipantContextByHash(identifiers.NilAddress, test.hash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(
				t,
				common.NewNodeSet(obj[0].Ids, pk[:2], uint32(len(obj[0].Ids))),
				common.NewNodeSet(obj[1].Ids, pk[2:4], uint32(len(obj[1].Ids))),
				behCtx,
				randCtx,
			)
		})
	}
}

func TestStateManager_GetCommittedContextHash(t *testing.T) {
	ts := tests.CreateTesseract(t, getTesseractParamsWithContextHash(tests.RandomAddress(t), tests.RandomHash(t)))

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		contextHash   common.Hash
		expectedError error
	}{
		{
			name:          "context doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:        "context exists",
			address:     ts.AnyAddress(),
			contextHash: ts.LatestContextHash(ts.AnyAddress()),
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

/*

func TestStateManager_FetchContextLock(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	mocksenatus := mockSenatus(t)
	mocksenatus.AddPublicKeys(kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	addresses := tests.GetAddresses(t, 10)
	ixns := tests.CreateIxns(t, 5, tests.GetIxParamsMapWithAddresses(addresses[:5], addresses[5:10]))

	getTesseractParams := func(
		ixns common.Interactions,
		addresses []identifiers.Address,
		hashes ...common.Hash,
	) *tests.CreateTesseractParams {
		participants := make(common.Participants)

		for i, address := range addresses {
			participants[address] = common.State{
				PreviousContext: hashes[i],
			}
		}

		return &tests.CreateTesseractParams{
			Addresses:    addresses,
			Ixns:         ixns,
			Participants: participants,
		}
	}

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParams(
			ixns[0:1],
			[]identifiers.Address{ixns[0].Sender(), ixns[0].Receiver()},
			mHash[0], mHash[1],
		),
		1: getTesseractParams(
			ixns[1:2],
			[]identifiers.Address{ixns[1].Sender()},
			tests.RandomHash(t),
		),
		2: getTesseractParams(
			ixns[2:3],
			[]identifiers.Address{ixns[2].Receiver()},
			tests.RandomHash(t),
		),
		3: getTesseractParams(
			ixns[3:4],
			[]identifiers.Address{ixns[3].Sender(), ixns[3].Receiver()},
			mHash[0], common.NilHash,
		),
		4: getTesseractParams(
			ixns[4:5],
			[]identifiers.Address{common.SargaAddress},
			mHash[1],
		),
	}

	ts := tests.CreateTesseracts(t, 5, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
		smCallBack: func(sm *StateManager) {
			sm.senatus = mocksenatus
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		tess          *common.Tesseract
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
				common.NewNodeSet(obj[0].Ids, pk[:2], 0),
				common.NewNodeSet(obj[1].Ids, pk[2:4], 0),
				nil,
				nil,
			),
		},
		{
			name: "sarga address in context lock",
			tess: ts[4],
			nodes: getICSNodes(
				nil, nil,
				common.NewNodeSet(obj[2].Ids, pk[4:6], 0),
				common.NewNodeSet(obj[3].Ids, pk[6:8], 0),
			),
		},
		{
			name: "valid context hashes",
			tess: ts[0],
			nodes: getICSNodes(
				common.NewNodeSet(obj[0].Ids, pk[:2], 0),
				common.NewNodeSet(obj[1].Ids, pk[2:4], 0),
				common.NewNodeSet(obj[2].Ids, pk[4:6], 0),
				common.NewNodeSet(obj[3].Ids, pk[6:8], 0),
			),
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
					icsNodes.Sets[common.SenderBehaviourSet],
					icsNodes.Sets[common.SenderRandomSet],
				)
			}
			if test.nodes.receiverBeh != nil {
				checkIfNodesetEqual(
					t,
					test.nodes.receiverBeh,
					test.nodes.receiverRand,
					icsNodes.Sets[common.ReceiverBehaviourSet],
					icsNodes.Sets[common.ReceiverRandomSet],
				)
			}
		})
	}
}
*/

func TestStateManager_IsAccountRegistered_With_SargaObject(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		AccType: common.SargaAccount,
	}, NilMetrics())
	_, err = so.createStorageTreeForLogic(common.SargaLogicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(common.SargaLogicID, address.Bytes(), []byte{0x01})
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)
	assert.NoError(t, so.flush())

	ts := tests.CreateTesseract(t, getTesseractParamsWithStateHash(common.SargaAddress, stateHash))
	smParams := &createStateManagerParams{
		db: db,
		smCallBack: func(sm *StateManager) {
			insertTesseractsAndIxnsInDB(t, db, ts)
			sm.cache.Add(ts.AnyAddress(), tests.GetTesseractHash(t, ts))
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
		},
	}

	testcases := []struct {
		name          string
		address       identifiers.Address
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
			address:      identifiers.NilAddress,
			smParams:     smParams,
			isRegistered: true,
		},
		{
			name:          "sarga object not found",
			address:       tests.RandomAddress(t),
			smParams:      nil,
			isRegistered:  true,
			expectedError: common.ErrObjectNotFound,
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

func TestStateManager_GetNonce(t *testing.T) {
	address := tests.RandomAddress(t)

	accounts0 := &common.Account{
		Nonce: 12,
	}

	stateHash0, err := accounts0.Hash()
	assert.NoError(t, err)

	account1 := &common.Account{
		Nonce: 22,
	}

	stateHash1, err := account1.Hash()
	assert.NoError(t, err)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantStates{
			address: {
				StateHash: stateHash1,
			},
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash0}, accounts0)
			insertAccountsInDB(t, db, []common.Hash{ts.StateHash(address)}, account1)
			insertTesseractsAndIxnsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(ts.AnyAddress(), tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		stateHash     common.Hash
		nonce         uint64
		expectedError error
	}{
		{
			name:    "fetch nonce from latest state",
			address: ts.AnyAddress(),
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
			address:       identifiers.NilAddress,
			expectedError: common.ErrInvalidAddress,
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

func TestStateManager_GetBalances(t *testing.T) {
	address := tests.RandomAddress(t)
	assets, bal := getAssetIDsAndBalances(t, 2)
	balances, balanceHashes := getTestBalances(t, getAssetMaps(assets, bal, 1), 2)

	accounts0 := &common.Account{
		Nonce:   12,
		Balance: balanceHashes[0],
	}

	stateHash0, err := accounts0.Hash()
	assert.NoError(t, err)

	account1 := &common.Account{
		Nonce:   22,
		Balance: balanceHashes[1],
	}

	stateHash1, err := account1.Hash()
	assert.NoError(t, err)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantStates{
			address: {
				StateHash: stateHash1,
			},
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash0}, accounts0)
			insertAccountsInDB(t, db, []common.Hash{ts.StateHash(address)}, account1)
			insertBalancesInDB(t, db, balanceHashes, balances...)
			insertTesseractsAndIxnsInDB(t, db, ts)
		},

		smCallBack: func(sm *StateManager) {
			sm.cache.Add(ts.AnyAddress(), tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		stateHash     common.Hash
		balance       *BalanceObject
		expectedError error
	}{
		{
			name:    "fetch balances from latest state",
			address: ts.AnyAddress(),
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

func TestStateManager_GetBalance(t *testing.T) {
	address := tests.RandomAddress(t)
	assetIDs, bal := getAssetIDsAndBalances(t, 2)
	balances, balanceHashes := getTestBalances(t, getAssetMaps(assetIDs, bal, 1), 2)
	accounts, stateHashes := getTestAccounts(t, balanceHashes, 1)

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantStates{
			address: {
				StateHash: stateHashes[0],
			},
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, ts)
			insertAccountsInDB(t, db, stateHashes, accounts...)
			insertBalancesInDB(t, db, balanceHashes, balances...)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(address, tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		address       identifiers.Address
		assetID       identifiers.AssetID
		stateHash     common.Hash
		balance       *BalanceObject
		expectedError error
	}{
		{
			name:    "fetch balance from latest state",
			address: address,
			assetID: assetIDs[0],
			balance: balances[0],
		},
		{
			name:      "fetch balance at particular state",
			address:   address,
			assetID:   assetIDs[0],
			stateHash: stateHashes[0],
			balance:   balances[0],
		},
		{
			name:          "should return error if asset not found",
			address:       address,
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: common.ErrAssetNotFound,
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

func TestStateManager_SyncLogicStorageTree(t *testing.T) {
	logicIDs := tests.GetLogicIDs(t, 3)
	storageTrees := make([]tree.MerkleTree, 3)

	soParams := map[int]*createStateObjectParams{
		0: {
			address: logicIDs[0].Address(),
		},
		1: {
			address: logicIDs[1].Address(),
		},
		2: {
			address: logicIDs[2].Address(),
		},
	}

	so := createTestStateObjects(t, 3, soParams)

	for i := 0; i < 3; i++ {
		storageTree, err := so[i].createStorageTreeForLogic(logicIDs[i])
		assert.NoError(t, err)

		storageTrees[i] = storageTree
	}

	err := so[0].SetStorageEntry(logicIDs[0], logicIDs[0].Bytes(), []byte{0x01})
	assert.NoError(t, err)

	err = so[1].SetStorageEntry(logicIDs[1], logicIDs[0].Bytes(), []byte{0x02})
	assert.NoError(t, err)

	err = so[2].SetStorageEntry(logicIDs[2], logicIDs[2].Bytes(), []byte{0x02})
	assert.NoError(t, err)

	getStateHashes(t, so)

	newRoot := storageTrees[0].Root()

	sm := createTestStateManager(t, nil)

	testcases := []struct {
		name          string
		stateObject   *Object
		logicID       identifiers.LogicID
		newRoot       *common.RootNode
		expectedError error
	}{
		{
			name:        "logic tree root and new root are same",
			stateObject: so[0],
			logicID:     logicIDs[0],
			newRoot:     &newRoot,
		},
		{
			name:        "tree is synced successfully with the new root",
			stateObject: so[1],
			logicID:     logicIDs[1],
			newRoot:     &newRoot,
		},
		{
			name:          "tree is not synced properly",
			stateObject:   so[2],
			logicID:       logicIDs[2],
			newRoot:       &newRoot,
			expectedError: errors.New("updated root doesn't match"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sm.syncLogicStorageTree(test.stateObject, test.logicID, test.newRoot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			root := test.stateObject.storageTrees[test.logicID].Root()
			require.Equal(t, test.newRoot, &root)
		})
	}
}

func TestStateManager_SyncStorageTrees(t *testing.T) {
	db := mockDB()
	logicIDs := tests.GetLogicIDs(t, 3)

	soParams := map[int]*createStateObjectParams{
		0: {
			address: logicIDs[1].Address(),
		},
		1: {
			address: logicIDs[2].Address(),
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	stateObject := NewStateObject(logicIDs[0].Address(), mockCache(t), nil, db,
		common.Account{}, NilMetrics())

	storageTree, err := stateObject.createStorageTreeForLogic(logicIDs[0])
	assert.NoError(t, err)

	_, err = so[0].createStorageTreeForLogic(logicIDs[1])
	assert.NoError(t, err)

	err = stateObject.SetStorageEntry(logicIDs[0], logicIDs[0].Bytes(), []byte{0x01})
	assert.NoError(t, err)

	err = so[0].SetStorageEntry(logicIDs[1], logicIDs[0].Bytes(), []byte{0x02})
	assert.NoError(t, err)

	_, err = stateObject.Commit()
	assert.NoError(t, err)

	smParams := &createStateManagerParams{
		db: db,
		smCallBack: func(sm *StateManager) {
			sm.dirtyObjects[stateObject.address] = stateObject
			sm.dirtyObjects[so[0].address] = so[0]
		},
	}

	sm := createTestStateManager(t, smParams)

	newRoot := storageTree.Root()

	testcases := []struct {
		name                  string
		addr                  identifiers.Address
		newRoot               *common.RootNode
		logicID               identifiers.LogicID
		logicStorageTreeRoots map[string]*common.RootNode
		stateObject           *Object
		expectedError         error
	}{
		{
			name:    "failed to fetch state object",
			addr:    so[1].address,
			newRoot: &newRoot,
			logicStorageTreeRoots: map[string]*common.RootNode{
				string(logicIDs[0]): &newRoot,
			},
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:    "tree is synced successfully with the new root",
			addr:    stateObject.address,
			newRoot: &newRoot,
			logicID: logicIDs[0],
			logicStorageTreeRoots: map[string]*common.RootNode{
				string(logicIDs[0]): &newRoot,
			},
			stateObject: stateObject,
		},
		{
			name:    "tree is not synced properly",
			addr:    so[0].address,
			newRoot: &newRoot,
			logicID: logicIDs[1],
			logicStorageTreeRoots: map[string]*common.RootNode{
				string(logicIDs[1]): {
					MerkleRoot: newRoot.MerkleRoot,
					HashTable: map[string][]byte{
						string(logicIDs[0]): {0x03},
					},
				},
			},
			stateObject:   so[0],
			expectedError: errors.New("updated root doesn't match"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sm.SyncStorageTrees(context.Background(), test.addr, test.newRoot, test.logicStorageTreeRoots)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			entry := test.stateObject.storageTrees[test.logicID]
			require.NotNil(t, entry)

			root := entry.Root()
			require.Equal(t, test.newRoot, &root)
		})
	}
}

func TestStateManager_SyncLogicTree(t *testing.T) {
	db := mockDB()
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	rawData := logicID.Bytes()

	logicTree, err := tree.NewKramaHashTree(logicID.Address(), common.NilHash, db, blake256.New(),
		storage.Logic, nil, tree.NilMetrics())
	require.NoError(t, err)

	err = logicTree.Set(logicID.Bytes(), rawData)
	require.NoError(t, err)

	soParams := map[int]*createStateObjectParams{
		0: stateObjectParamsWithLogicTree(t, logicID.Address(), db, logicTree, common.NilHash, nil),
		1: {
			address: tests.RandomAddress(t),
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		db: db,
		smCallBack: func(sm *StateManager) {
			sm.dirtyObjects[so[0].address] = so[0]
		},
	}

	sm := createTestStateManager(t, smParams)

	newRoot := so[0].logicTree.Root()

	testcases := []struct {
		name          string
		addr          identifiers.Address
		newRoot       *common.RootNode
		logicTree     tree.MerkleTree
		expectedError error
	}{
		{
			name:      "logic tree synced successfully",
			addr:      so[0].address,
			newRoot:   &newRoot,
			logicTree: so[0].logicTree,
		},
		{
			name: "failed to fetch state object",
			addr: so[1].address,
			newRoot: &common.RootNode{
				MerkleRoot: tests.RandomHash(t),
				HashTable:  map[string][]byte{tests.RandomHash(t).String(): tests.RandomHash(t).Bytes()},
			},
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name: "tree is not synced properly",
			addr: so[0].address,
			newRoot: &common.RootNode{
				MerkleRoot: tests.RandomHash(t),
				HashTable:  map[string][]byte{tests.RandomHash(t).String(): tests.RandomHash(t).Bytes()},
			},
			logicTree:     so[0].logicTree,
			expectedError: errors.New("updated root doesn't match"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sm.SyncLogicTree(test.addr, test.newRoot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			root := test.logicTree.Root()
			require.Equal(t, test.newRoot, &root)
		})
	}
}

/*
func TestStateManager_GetNodeSet(t *testing.T) {
	mocksenatus := mockSenatus(t)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	mocksenatus.AddPublicKeys(kramaIDs[:2], pk[:2])

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.senatus = mocksenatus
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		ids           []kramaid.KramaID
		publicKeys    [][]byte
		expectedError error
	}{
		{
			name:       "fetched new node set as kramaIDs are present",
			ids:        kramaIDs[:2],
			publicKeys: pk[:2],
		},
		{
			name:          "should return error as the public keys are not present in senatus",
			ids:           kramaIDs[2:4],
			publicKeys:    pk[2:4],
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:       "fetched empty node set as no kramaIDs are present",
			ids:        nil,
			publicKeys: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nodeSet, err := sm.GetNodeSet(test.ids)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.Equal(t, test.ids, nodeSet.Ids)
			require.Equal(t, test.publicKeys, nodeSet.PublicKeys)
		})
	}
}
*/

func TestStateManager_GetICSNodeSetFromRawContext(t *testing.T) {
	addrs := tests.GetAddresses(t, 2)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
	mocksenatus := mockSenatus(t)
	mocksenatus.AddPublicKeys(kramaIDs, pk)

	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(addrs[0], identifiers.NilAddress),
		1: tests.GetIxParamsWithAddress(tests.RandomAddress(t), addrs[1]),
		2: tests.GetIxParamsWithAddress(tests.RandomAddress(t), common.SargaAddress),
	}

	ixs := tests.CreateIxns(t, 3, ixParams)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParams(addrs[0], common.Interactions{ixs[0]}, mHash[0]),
		1: getTesseractParams(addrs[1], common.Interactions{ixs[1]}, mHash[1]),
		2: getTesseractParams(common.SargaAddress, common.Interactions{ixs[2]}, mHash[1]),
	}

	ts := tests.CreateTesseracts(t, 3, tesseractParams)

	rawMetaObjects := getRawMetaObjects(t, mObj)
	rawContextObjects := getRawContextObjects(t, obj)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.senatus = mocksenatus
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name            string
		ts              *common.Tesseract
		rawContext      map[string][]byte
		expectedNodeSet map[int][]kramaid.KramaID
		expectedError   error
	}{
		{
			name: "fetched ics node set when context address is equal to ix sender address",
			ts:   ts[0],
			rawContext: map[string][]byte{
				mHash[0].String():                   rawMetaObjects[0],
				mObj[0].BehaviouralContext.String(): rawContextObjects[0],
				mObj[0].RandomContext.String():      rawContextObjects[1],
			},
			expectedNodeSet: map[int][]kramaid.KramaID{
				0: obj[0].Ids,
				1: obj[1].Ids,
				2: obj[4].Ids,
				3: obj[5].Ids,
			},
		},
		{
			name: "fetched ics node set when context address is equal to ix receiver address",
			ts:   ts[1],
			rawContext: map[string][]byte{
				mHash[1].String():                   rawMetaObjects[1],
				mObj[1].BehaviouralContext.String(): rawContextObjects[2],
				mObj[1].RandomContext.String():      rawContextObjects[3],
			},
			expectedNodeSet: map[int][]kramaid.KramaID{
				0: obj[2].Ids,
				1: obj[3].Ids,
				2: obj[4].Ids,
				3: obj[5].Ids,
			},
		},
		{
			name: "fetched ics node set when context address is equal to ix sarga address",
			ts:   ts[2],
			rawContext: map[string][]byte{
				mHash[1].String():                   rawMetaObjects[1],
				mObj[1].BehaviouralContext.String(): rawContextObjects[2],
				mObj[1].RandomContext.String():      rawContextObjects[3],
			},
			expectedNodeSet: map[int][]kramaid.KramaID{
				0: obj[2].Ids,
				1: obj[3].Ids,
				2: obj[4].Ids,
				3: obj[5].Ids,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			clusterInfo := &common.ICSClusterInfo{
				RandomSet:   obj[4].Ids,
				ObserverSet: obj[5].Ids,
				Responses:   createRandomArrayOfBits(t, 6),
			}

			nodeSet, err := sm.GetICSNodeSetFromRawContext(test.ts, test.rawContext, clusterInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			count := 0

			for setType, kramaIDs := range test.expectedNodeSet {
				count += len(kramaIDs)
				require.Equal(t, kramaIDs, nodeSet.Sets[setType].Ids)
			}

			for index, set := range nodeSet.Sets {
				if set != nil && clusterInfo.Responses[index] != nil {
					require.Equal(t, set.Responses, clusterInfo.Responses[index])
				}
			}

			require.Equal(t, count, nodeSet.TotalNodes())
			require.Equal(t, 0, len(test.rawContext)) // ensure context hashes removed from raw context
		})
	}
}

func TestStateManager_GetParticipantContextRaw(t *testing.T) {
	mObj := make([]*MetaContextObject, 5)
	mHash := make([]common.Hash, 5)

	kramaIDs := tests.RandomKramaIDs(t, 12)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj[0], mHash[0] = getMetaContextObject(t, cHash[0], cHash[1])
	mObj[1], mHash[1] = getMetaContextObject(t, cHash[2], common.NilHash)
	mObj[2], mHash[2] = getMetaContextObject(t, common.NilHash, cHash[3])
	mObj[3], mHash[3] = getMetaContextObject(t, cHash[4], common.NilHash)
	mObj[4], mHash[4] = getMetaContextObject(t, common.NilHash, cHash[5])

	rawMetaObjects := getRawMetaObjects(t, mObj)
	rawContextObjects := getRawContextObjects(t, obj)

	smParams := &createStateManagerParams{
		db: mockDB(),
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj[0:4]...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name               string
		addr               identifiers.Address
		hash               common.Hash
		expectedRawContext map[string][]byte
		expectedError      error
	}{
		{
			name:          "failed to fetch meta context object",
			addr:          tests.RandomAddress(t),
			hash:          tests.RandomHash(t),
			expectedError: errors.New("context state not found"),
		},
		{
			name:          "failed to fetch participant context info with random context",
			addr:          tests.RandomAddress(t),
			hash:          mHash[4],
			expectedError: errors.New("failed to fetch random context"),
		},
		{
			name:          "failed to fetch participant context info with behavioral context",
			addr:          tests.RandomAddress(t),
			hash:          mHash[3],
			expectedError: errors.New("failed to fetch behavioural context"),
		},
		{
			name: "participant context info with random context fetched successfully",
			addr: tests.RandomAddress(t),
			hash: mHash[2],
			expectedRawContext: map[string][]byte{
				mHash[2].String():              rawMetaObjects[2],
				mObj[2].RandomContext.String(): rawContextObjects[3],
			},
		},
		{
			name: "participant context info with behavioural context fetched successfully",
			addr: tests.RandomAddress(t),
			hash: mHash[1],
			expectedRawContext: map[string][]byte{
				mHash[1].String():                   rawMetaObjects[1],
				mObj[1].BehaviouralContext.String(): rawContextObjects[2],
			},
		},
		{
			name: "participant context info with behavioral and random context fetched successfully",
			addr: tests.RandomAddress(t),
			hash: mHash[0],
			expectedRawContext: map[string][]byte{
				mHash[0].String():                   rawMetaObjects[0],
				mObj[0].BehaviouralContext.String(): rawContextObjects[0],
				mObj[0].RandomContext.String():      rawContextObjects[1],
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rawContext := make(map[string][]byte)

			err := sm.GetParticipantContextRaw(test.addr, test.hash, rawContext)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedRawContext, rawContext)
		})
	}
}

func TestStateManager_GetStorageEntry(t *testing.T) {
	db := mockDB()
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))

	so := NewStateObject(logicID.Address(), mockCache(t), nil, db, common.Account{}, NilMetrics())

	_, err := so.createStorageTreeForLogic(logicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(logicID, logicID.Bytes(), []byte{0x01})
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)
	assert.NoError(t, so.flush())

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name                 string
		logicID              identifiers.LogicID
		slot                 []byte
		stateHash            common.Hash
		expectedStorageEntry []byte
		expectedError        error
	}{
		{
			name:          "should return an error if state object not found",
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			slot:          logicID.Bytes(),
			stateHash:     tests.RandomHash(t),
			expectedError: errors.New("failed to fetch state object"),
		},
		{
			name:          "should return an error if logic storage tree not found",
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			slot:          logicID.Bytes(),
			stateHash:     stateHash,
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name:                 "should fetch storage data successfully",
			logicID:              logicID,
			slot:                 logicID.Bytes(),
			stateHash:            stateHash,
			expectedStorageEntry: []byte{0x01},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			storageEntry, err := sm.GetStorageEntry(test.logicID, test.slot, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedStorageEntry, storageEntry)
		})
	}
}

func TestStateManager_IsLogicRegistered(t *testing.T) {
	db := mockDB()
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))

	engineio.RegisterEngine(pisa.NewEngine())

	so := NewStateObject(logicID.Address(), mockCache(t), nil, db, common.Account{}, NilMetrics())

	err := so.InsertNewLogicObject(logicID, logicObject)
	require.NoError(t, err)

	rootHash, err := so.commitLogics()
	require.NoError(t, err)

	err = so.flushLogicTree()
	require.NoError(t, err)

	acc, stateHash := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.LogicRoot = rootHash
	})

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{logicID.Address()},
		Participants: common.ParticipantStates{
			logicID.Address(): {
				StateHash: stateHash,
			},
		},
	})

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash}, acc)
			insertTesseractsAndIxnsInDB(t, db, ts)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(ts.AnyAddress(), tests.GetTesseractHash(t, ts))
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		logicID       identifiers.LogicID
		expectedError error
	}{
		{
			name:          "should return error if logic is not registered",
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:    "should register logic successfully",
			logicID: logicID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err = sm.IsLogicRegistered(test.logicID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedError, err)
		})
	}
}

func TestStateManager_GetAssetInfo(t *testing.T) {
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	assetInfo := tests.GetRandomAssetInfo(t, assetID.Address())

	rawAssetInfo, err := assetInfo.Bytes()
	assert.NoError(t, err)

	registry, registryHash := getTestRegistryObject(
		t,
		map[string][]byte{string(assetID): rawAssetInfo},
	)

	sObj := createTestStateObject(t, stateObjectParamsWithRegistry(t, registryHash, registry))

	stateHash, err := sObj.commitRegistryObject()
	assert.NoError(t, err)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash}, sObj.Data())
			insertAssetRegistryInDB(t, db, []common.Hash{registryHash}, registry)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name              string
		assetID           identifiers.AssetID
		stateHash         common.Hash
		expectedAssetInfo *common.AssetDescriptor
		expectedError     error
	}{
		{
			name:          "failed to fetch assetInfo as state object not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: errors.New("failed to fetch state object"),
		},
		{
			name:              "asset info fetched successfully",
			assetID:           assetID,
			stateHash:         stateHash,
			expectedAssetInfo: assetInfo,
		},
		{
			name:          "should return error as asset not found because of invalid assetID",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			stateHash:     stateHash,
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			assetInfo, err := sm.GetAssetInfo(test.assetID, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAssetInfo, assetInfo)
		})
	}
}

func TestStateManager_GetRegistry(t *testing.T) {
	registry, registryHash := getTestRegistryObject(
		t,
		map[string][]byte{tests.RandomHash(t).String(): tests.RandomHash(t).Bytes()},
	)

	soParams := map[int]*createStateObjectParams{
		0: stateObjectParamsWithRegistry(t, registryHash, registry),
		1: {
			address: tests.RandomAddress(t),
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	stateHashes := getStateHashes(t, so)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAssetRegistryInDB(t, db, []common.Hash{registryHash}, registry)
			insertAccountsInDB(t, db, []common.Hash{stateHashes[0]}, so[0].Data())
			insertAccountsInDB(t, db, []common.Hash{stateHashes[1]}, so[1].Data())
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name                    string
		address                 identifiers.Address
		stateHash               common.Hash
		expectedRegistryEntries *RegistryObject
		expectedError           error
	}{
		{
			name:          "failed to fetch state object",
			address:       tests.RandomAddress(t),
			stateHash:     tests.RandomHash(t),
			expectedError: errors.New("failed to fetch state object"),
		},
		{
			name:                    "fetched registry entries successfully",
			address:                 so[0].Address(),
			stateHash:               stateHashes[0],
			expectedRegistryEntries: registry,
		},
		{
			name:      "fetched empty registry object as it was not present in state object",
			address:   so[1].Address(),
			stateHash: stateHashes[1],
			expectedRegistryEntries: &RegistryObject{
				Entries: make(map[string][]byte),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			registryEntries, err := sm.GetRegistry(test.address, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedRegistryEntries, &RegistryObject{
				Entries: registryEntries,
			})
		})
	}
}

func TestStateManager_GetLogicManifest(t *testing.T) {
	db := mockDB()

	engineio.RegisterEngine(pisa.NewEngine())

	manifest, err := engineio.NewManifestFromFile("../compute/manifests/tokenledger.yaml")
	require.NoError(t, err)

	encodedManifest, err := manifest.Encode(common.POLO)
	require.NoError(t, err)

	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	logicObject.Manifest = manifest.Hash()

	so := NewStateObject(logicID.Address(), mockCache(t), nil, db, common.Account{}, NilMetrics())

	err = so.InsertNewLogicObject(logicID, logicObject)
	require.NoError(t, err)

	rootHash, err := so.commitLogics()
	require.NoError(t, err)

	err = so.flushLogicTree()
	require.NoError(t, err)

	acc, stateHash := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.LogicRoot = rootHash
	})

	accWithInvalidLogicRoot, stateHashWithInvalidLogicRoot := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.LogicRoot = tests.RandomHash(t)
	})

	err = db.CreateEntry(storage.LogicManifestKey(logicID.Address(), logicObject.ManifestHash()), encodedManifest)
	require.NoError(t, err)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash}, acc)
			insertAccountsInDB(t, db, []common.Hash{stateHashWithInvalidLogicRoot}, accWithInvalidLogicRoot)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		logicID          identifiers.LogicID
		stateHash        common.Hash
		expectedManifest []byte
		expectedError    error
	}{
		{
			name:          "failed to fetch state object",
			logicID:       logicID,
			stateHash:     tests.RandomHash(t),
			expectedError: common.ErrStateNotFound,
		},
		{
			name:          "failed to fetch logic tree",
			logicID:       logicID,
			stateHash:     stateHashWithInvalidLogicRoot,
			expectedError: errors.New("failed to fetch logic object"),
		},
		{
			name:             "logic manifest fetched successfully",
			logicID:          logicID,
			stateHash:        stateHash,
			expectedManifest: encodedManifest,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := sm.GetLogicManifest(test.logicID, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedManifest, manifest)
		})
	}
}

func TestStateManager_GetLogicIDs(t *testing.T) {
	var err error

	expectedLogicIDs := make([]identifiers.LogicID, 0)
	db := mockDB()
	address := tests.RandomAddress(t)
	so := NewStateObject(address, mockCache(t), nil, db, common.Account{}, NilMetrics())

	for i := 0; i < 3; i++ {
		logicID := tests.GetLogicID(t, tests.RandomAddress(t))
		logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))

		err := so.InsertNewLogicObject(logicID, logicObject)
		require.NoError(t, err)

		expectedLogicIDs = append(expectedLogicIDs, logicID)
	}

	rootHash, err := so.commitLogics()
	require.NoError(t, err)

	err = so.flushLogicTree()
	require.NoError(t, err)

	acc, stateHash := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.LogicRoot = rootHash
	})

	accWithInvalidLogicRoot, stateHashWithInvalidLogicRoot := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.LogicRoot = tests.RandomHash(t)
	})

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash}, acc)
			insertAccountsInDB(t, db, []common.Hash{stateHashWithInvalidLogicRoot}, accWithInvalidLogicRoot)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		addr             identifiers.Address
		stateHash        common.Hash
		expectedLogicIDs []identifiers.LogicID
		expectedError    error
	}{
		{
			name:          "failed to fetch state object",
			addr:          address,
			stateHash:     tests.RandomHash(t),
			expectedError: common.ErrStateNotFound,
		},
		{
			name:          "failed to fetch meta logic tree",
			addr:          address,
			stateHash:     stateHashWithInvalidLogicRoot,
			expectedError: errors.New("failed to load meta logic tree"),
		},
		{
			name:             "logic IDs fetched successfully",
			addr:             address,
			expectedLogicIDs: expectedLogicIDs,
			stateHash:        stateHash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicIDs, err := sm.GetLogicIDs(test.addr, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if logic ids match
			for _, expectedLogicID := range expectedLogicIDs {
				found := false

				for _, logicID := range logicIDs {
					if expectedLogicID == logicID {
						found = true
					}
				}

				require.True(t, found)
			}
		})
	}
}

func TestStateManager_doesRootMatch(t *testing.T) {
	root := common.RootNode{
		MerkleRoot: tests.RandomHash(t),
		HashTable:  map[string][]byte{tests.RandomHash(t).String(): tests.RandomHash(t).Bytes()},
	}

	testcases := []struct {
		name           string
		root1          common.RootNode
		root2          common.RootNode
		expectedResult bool
	}{
		{
			name:  "roots shouldn't match",
			root1: root,
			root2: common.RootNode{
				MerkleRoot: tests.RandomHash(t),
				HashTable:  map[string][]byte{tests.RandomHash(t).String(): tests.RandomHash(t).Bytes()},
			},
			expectedResult: false,
		},
		{
			name:           "roots should match",
			root1:          root,
			root2:          root,
			expectedResult: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			match, err := doesRootMatch(test.root1, test.root2)
			require.NoError(t, err)
			require.Equal(t, test.expectedResult, match)
		})
	}
}

func TestStateManager_FetchLatestParticipantContext(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
	mocksenatus := mockSenatus(t)
	mocksenatus.AddPublicKeys(kramaIDs[:4], pk[:4])
	mocksenatus.AddPublicKeys(kramaIDs[8:10], pk[8:10])
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
			sm.senatus = mocksenatus
		},
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsAndIxnsInDB(t, db, ts...)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		address       identifiers.Address
		ctxHash       common.Hash
		behSet        *common.NodeSet
		randSet       *common.NodeSet
		expectedError error
	}{
		{
			name:          "tesseract doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name:          "behavioural context Nodes doesn't have public keys",
			address:       ts[1].AnyAddress(),
			expectedError: common.ErrPublicKeyNotFound,
		},
		{
			name:          "random context Nodes doesn't have public keys",
			address:       ts[2].AnyAddress(),
			expectedError: common.ErrPublicKeyNotFound,
		},
		{
			name:    "valid hash and public keys",
			address: ts[0].AnyAddress(),
			ctxHash: ts[0].LatestContextHash(ts[0].AnyAddress()),
			behSet:  common.NewNodeSet(obj[0].Ids, pk[:2], uint32(len(obj[0].Ids))),
			randSet: common.NewNodeSet(obj[1].Ids, pk[2:4], uint32(len(obj[1].Ids))),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, behSet, randSet, err := sm.FetchLatestParticipantContext(test.address)

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

/*
func TestStateManager_GetReceiverContext_RegisteredAccount(t *testing.T) {
	db := mockDB()
	mocksenatus := mockSenatus(t)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	mocksenatus.AddPublicKeys(kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	tesseractParams := getTesseractParamsWithContextHash(tests.RandomAddress(t), mHash[0])

	ts := tests.CreateTesseract(t, tesseractParams)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(identifiers.NilAddress, ts.AnyAddress()),
		1: tests.GetIxParamsWithAddress(identifiers.NilAddress, tests.RandomAddress(t)),
	}

	ixs := tests.CreateIxns(t, 2, ixParams)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		AccType: common.SargaAccount,
	}, NilMetrics())
	_, err = so.createStorageTreeForLogic(common.SargaLogicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(common.SargaLogicID, ixs[0].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)

	err = so.SetStorageEntry(common.SargaLogicID, ixs[1].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)

	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)
	assert.NoError(t, so.flush())

	sargaTesseract := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{common.SargaAddress},
		Participants: common.Participants{
			common.SargaAddress: {
				StateHash: stateHash,
			},
		},
	})

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, ts)
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
			insertTesseractsAndIxnsInDB(t, db, sargaTesseract)
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts)
			storeTesseractHashInCache(t, sm.cache, sargaTesseract)
			sm.senatus = mocksenatus
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		ix            *common.Interaction
		behSet        *common.NodeSet
		randSet       *common.NodeSet
		address       identifiers.Address
		contextHash   common.Hash
		mockFn        func()
		expectedError error
	}{
		{
			name:        "context of receiver found",
			ix:          ixs[0],
			behSet:      common.NewNodeSet(obj[0].Ids, pk[:2], 0),
			randSet:     common.NewNodeSet(obj[1].Ids, pk[2:4], 0),
			address:     ixs[0].Receiver(),
			contextHash: ts.LatestContextHash(ts.AnyAddress()),
		},
		{
			name:          "failed to fetch receiver context",
			ix:            ixs[1],
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			nodeSet := make([]*common.NodeSet, 4)
			contextHashes := make(map[identifiers.Address]common.Hash)

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
				nodeSet[common.ReceiverBehaviourSet],
				nodeSet[common.ReceiverRandomSet],
			)
			require.Equal(t, test.contextHash, contextHashes[test.address])
		})
	}
}

func TestStateManager_GetReceiverContext_Non_RegisteredAccount(t *testing.T) {
	db := mockDB()
	mocksenatus := mockSenatus(t)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)
	mocksenatus.AddPublicKeys(kramaIDs, pk)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(identifiers.NilAddress, tests.RandomAddress(t)),
		1: tests.GetIxParamsWithAddress(identifiers.NilAddress, tests.RandomAddress(t)),
	}

	ixs := tests.CreateIxns(t, 3, ixParams)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		AccType:     common.SargaAccount,
		ContextHash: mHash[0],
	}, NilMetrics())

	_, err = so.createStorageTreeForLogic(common.SargaLogicID)
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	assert.NoError(t, so.flush())

	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{common.SargaAddress},
		Participants: common.Participants{
			common.SargaAddress: {
				StateHash:     stateHash,
				LatestContext: mHash[0],
			},
		},
	})

	testcases := []struct {
		name          string
		ix            *common.Interaction
		soParams      *createStateObjectParams
		smParams      *createStateManagerParams
		behSet        *common.NodeSet
		randSet       *common.NodeSet
		address       identifiers.Address
		contextHash   common.Hash
		preTestFn     func()
		errorExpected bool
	}{
		{
			name: "context of sarga account found",
			ix:   ixs[0],
			smParams: &createStateManagerParams{
				db: db,
				dbCallback: func(db *MockDB) {
					insertTesseractsAndIxnsInDB(t, db, ts)
					insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
					insertMetaContextsInDB(t, db, mObj...)
					insertContextsInDB(t, db, obj...)
				},
				smCallBack: func(sm *StateManager) {
					storeTesseractHashInCache(t, sm.cache, ts)
					sm.senatus = mocksenatus
				},
			},
			behSet:      common.NewNodeSet(obj[0].Ids, pk[:2], 0),
			randSet:     common.NewNodeSet(obj[1].Ids, pk[2:4], 0),
			address:     common.SargaAddress,
			contextHash: ts.LatestContextHash(ts.AnyAddress()),
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
			nodeSet := make([]*common.NodeSet, 4)
			contextHashes := make(map[identifiers.Address]common.Hash)

			if test.preTestFn != nil {
				test.preTestFn()
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
				nodeSet[common.ReceiverBehaviourSet],
				nodeSet[common.ReceiverRandomSet],
			)
			require.Equal(t, test.contextHash, contextHashes[test.address])
		})
	}
}

*/

func TestStateManager_FetchICSNodeSet(t *testing.T) {
	addr := tests.RandomAddress(t)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)
	mocksenatus := mockSenatus(t)
	mocksenatus.AddPublicKeys(kramaIDs[:8], pk[:8])

	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(addr, tests.RandomAddress(t)),
	}

	ixs := tests.CreateIxns(t, 1, ixParams)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParams(addr, common.Interactions{ixs[0]}, mHash[0]),
	}

	ts := tests.CreateTesseracts(t, 1, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, ts...)
			insertContextsInDB(t, db, obj...)
			insertMetaContextsInDB(t, db, mObj...)
		},
		smCallBack: func(sm *StateManager) {
			sm.senatus = mocksenatus
			storeTesseractHashInCache(t, sm.cache, ts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name            string
		ts              *common.Tesseract
		clusterInfo     *common.ICSClusterInfo
		expectedNodeSet map[int][]kramaid.KramaID
		expectedError   error
	}{
		{
			name: "updated ics node set fetched successfully",
			ts:   ts[0],
			clusterInfo: &common.ICSClusterInfo{
				RandomSet:   obj[2].Ids,
				ObserverSet: obj[3].Ids,
				Responses:   createRandomArrayOfBits(t, 6),
			},
			expectedNodeSet: map[int][]kramaid.KramaID{
				0: obj[0].Ids,
				1: obj[1].Ids,
				2: obj[2].Ids,
				3: obj[3].Ids,
			},
		},
		{
			name: "cluster info responses slice is empty",
			ts:   ts[0],
			clusterInfo: &common.ICSClusterInfo{
				RandomSet:   tests.RandomKramaIDs(t, 2),
				ObserverSet: tests.RandomKramaIDs(t, 2),
				Responses:   nil,
			},
			expectedError: errors.New("nil responses slice"),
		},
		{
			name: "failed to fetch updated ics node set as public keys of random set not present in senatus",
			ts:   ts[0],
			clusterInfo: &common.ICSClusterInfo{
				RandomSet:   obj[4].Ids,
				ObserverSet: obj[5].Ids,
				Responses:   createRandomArrayOfBits(t, 6),
			},
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
		{
			name: "failed to fetch updated ics node set as public keys of observer set not present in senatus",
			ts:   ts[0],
			clusterInfo: &common.ICSClusterInfo{
				RandomSet:   obj[2].Ids,
				ObserverSet: obj[5].Ids,
				Responses:   createRandomArrayOfBits(t, 6),
			},
			expectedError: errors.New("failed to fetch latest tesseract hash"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			icsNodes, err := sm.FetchICSNodeSet(test.ts, test.clusterInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for setType, kramaIDs := range test.expectedNodeSet {
				require.Equal(t, kramaIDs, icsNodes.Sets[setType].Ids)
			}

			for index, set := range icsNodes.Sets {
				if set != nil && test.clusterInfo.Responses[index] != nil {
					require.Equal(t, set.Responses, test.clusterInfo.Responses[index])
				}
			}
		})
	}
}

/*
func TestStateManager_FetchInteractionContext(t *testing.T) {
	db := mockDB()
	mocksenatus := mockSenatus(t)
	addrs := tests.GetAddresses(t, 2)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 8)
	obj, cHash := getContextObjects(t, kramaIDs, 2, 4)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(addrs[0], addrs[1]),
		1: tests.GetIxParamsWithAddress(identifiers.NilAddress, identifiers.NilAddress),
	}

	ixs := tests.CreateIxns(t, 2, ixParams)

	cache, err := lru.New(20)
	assert.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		AccType: common.SargaAccount,
	}, NilMetrics())

	_, err = so.createStorageTreeForLogic(common.SargaLogicID)
	assert.NoError(t, err)

	err = so.SetStorageEntry(common.SargaLogicID, ixs[0].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)
	err = so.SetStorageEntry(common.SargaLogicID, ixs[1].Receiver().Bytes(), []byte{0x01})
	assert.NoError(t, err)

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	assert.NoError(t, so.flush())

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithContextHash(ixs[0].Sender(), mHash[0]),
		1: getTesseractParamsWithContextHash(ixs[0].Receiver(), mHash[1]),
		2: getTesseractParamsWithStateHash(common.SargaAddress, stateHash),
	}

	ts := tests.CreateTesseracts(t, 3, tesseractParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			insertTesseractsAndIxnsInDB(t, db, ts...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts...)
			sm.senatus = mocksenatus
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		ix            *common.Interaction
		ics           *ICSNodes
		contextHashes map[identifiers.Address]common.Hash
		mockFn        func()
		expectedError error
	}{
		{
			name: "both sender and receiver addresses has context",
			ix:   ixs[0],
			ics: getICSNodes(
				common.NewNodeSet(obj[0].Ids, pk[:2], 0),
				common.NewNodeSet(obj[1].Ids, pk[2:4], 0),
				common.NewNodeSet(obj[2].Ids, pk[4:6], 0),
				common.NewNodeSet(obj[3].Ids, pk[6:8], 0),
			),
			contextHashes: map[identifiers.Address]common.Hash{
				ixs[0].Sender():   ts[0].LatestContextHash(ts[0].AnyAddress()),
				ixs[0].Receiver(): ts[1].LatestContextHash(ts[1].AnyAddress()),
			},
			mockFn: func() {
				mocksenatus.AddPublicKeys(kramaIDs, pk)
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
				require.Nil(t, nodeSet[common.SenderBehaviourSet])
				require.Nil(t, nodeSet[common.SenderRandomSet])
				require.Nil(t, nodeSet[common.ReceiverBehaviourSet])
				require.Nil(t, nodeSet[common.ReceiverRandomSet])

				return
			}
			checkIfNodesetEqual(
				t,
				test.ics.senderBeh,
				test.ics.senderRand,
				nodeSet[common.SenderBehaviourSet],
				nodeSet[common.SenderRandomSet],
			)
			checkIfNodesetEqual(
				t,
				test.ics.receiverBeh,
				test.ics.receiverRand,
				nodeSet[common.ReceiverBehaviourSet],
				nodeSet[common.ReceiverRandomSet],
			)
			for i := range test.contextHashes {
				require.Equal(t, test.contextHashes[i], contextHashes[i])
			}
		})
	}
}
*/

func TestStateManager_GetAccountInfo(t *testing.T) {
	hash := tests.RandomHash(t)
	acc := &common.Account{
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
		stateHash       common.Hash
		expectedAccount *common.Account
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
			expectedError:   common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			acc, err := sm.GetAccountState(identifiers.NilAddress, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccount, acc)
		})
	}
}

func TestStateManager_GetAccTypeUsingStateObject(t *testing.T) {
	soParams := &createStateObjectParams{
		account: &common.Account{AccType: common.LogicAccount},
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
		address       identifiers.Address
		sObj          *Object
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
			expectedError: common.ErrKeyNotFound,
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
			require.Equal(t, accType, common.LogicAccount)
		})
	}
}

func TestStateManager_SyncTree(t *testing.T) {
	keys, values := getEntries(t, 2)

	syncedMerkleTree := getMerkleTreeWithEntries(t, [][]byte{keys[0]}, [][]byte{values[0]})
	err := syncedMerkleTree.Commit()
	require.NoError(t, err)

	merkleTree := getMerkleTreeWithEntries(t, [][]byte{keys[0]}, [][]byte{values[1]})
	err = merkleTree.Commit()
	require.NoError(t, err)

	sm := createTestStateManager(t, nil)

	testcases := []struct {
		name          string
		tree          *MockMerkleTree
		newRoot       common.RootNode
		expectedError error
	}{
		{
			name:    "tree root and new root are same so tree is already synced",
			tree:    syncedMerkleTree,
			newRoot: syncedMerkleTree.Root(),
		},
		{
			name: "tree is synced successfully with the new root",
			tree: syncedMerkleTree,
			newRoot: common.RootNode{
				MerkleRoot: merkleTree.Root().MerkleRoot,
				HashTable:  map[string][]byte{common.BytesToHex(keys[0]): values[1]},
			},
		},
		{
			name: "tree is not synced properly with the new root",
			tree: syncedMerkleTree,
			newRoot: common.RootNode{
				MerkleRoot: tests.RandomHash(t),
				HashTable:  map[string][]byte{common.BytesToHex(keys[0]): values[1]},
			},
			expectedError: errors.New("updated root doesn't match"),
		},
		{
			name: "failed to set entry of the tree root with the new root",
			tree: getMerkleTreeWithHook(t, [][]byte{keys[0]}, [][]byte{values[0]}, func() error {
				return errors.New("failed to set entry")
			}),
			newRoot: common.RootNode{
				MerkleRoot: merkleTree.Root().MerkleRoot,
				HashTable:  map[string][]byte{common.BytesToHex(keys[0]): values[1]},
			},
			expectedError: errors.New("failed to set entry"),
		},
		{
			name: "tree synced with the new root but failed to commit",
			tree: getMerkleTreeWithCommitHook(t, [][]byte{keys[0]}, [][]byte{values[0]}, func() error {
				return errors.New("failed to commit")
			}),
			newRoot: common.RootNode{
				MerkleRoot: merkleTree.Root().MerkleRoot,
				HashTable:  map[string][]byte{common.BytesToHex(keys[0]): values[1]},
			},
			expectedError: errors.New("failed to commit"),
		},
		{
			name: "tree synced with the new root but failed to flush",
			tree: getMerkleTreeWithFlushHook(t, [][]byte{keys[0]}, [][]byte{values[0]}, func() error {
				return errors.New("failed to flush")
			}),
			newRoot: common.RootNode{
				MerkleRoot: merkleTree.Root().MerkleRoot,
				HashTable:  map[string][]byte{common.BytesToHex(keys[0]): values[1]},
			},
			expectedError: errors.New("failed to flush"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sm.syncTree(test.tree, &test.newRoot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			root := test.tree.Root()
			require.Equal(t, test.newRoot.MerkleRoot, root.MerkleRoot)

			for key, expectedValue := range root.HashTable {
				actualValue, err := test.tree.Get(common.Hex2Bytes(key))
				require.NoError(t, err)

				require.Equal(t, expectedValue, actualValue)
			}
		})
	}
}

func TestStateManager_FlushDirtyObject(t *testing.T) {
	db := mockDB()
	dirtyEntries := getDirtyEntries(t, 5)
	keys, values := getEntries(t, 4)

	merkle := mockMerkleTreeWithDB()
	soParams := map[int]*createStateObjectParams{
		0: {
			db: db,
			metaStorageTreeCallback: func(so *Object) {
				so.metaStorageTree = merkle
				setEntries(t, merkle, keys, values)
			},
			soCallback: func(so *Object) {
				so.dirtyEntries = dirtyEntries
			},
		},
		1: {
			db: db,
			metaStorageTreeCallback: func(so *Object) {
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
		address       identifiers.Address
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
			expectedError: common.ErrKeyNotFound,
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
						return common.ErrKeyNotFound
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
				val, err := merkle.dbStorage[common.BytesToHex(keys[i])]
				require.True(t, err)
				require.Equal(t, values[i], val)
			}

			// check if dirty entries flushed to db
			for k, v := range dirtyEntries {
				val, err := sm.db.ReadEntry(common.Hex2Bytes(k))
				require.NoError(t, err)
				require.Equal(t, v, val)
			}
		})
	}
}

func TestStateManager_IsAccountRegisteredAt(t *testing.T) {
	addresses := tests.GetAddresses(t, 3)
	db := mockDB()
	_, storageRoot := createMetaStorageTree(
		t,
		db,
		common.SargaAddress,
		common.SargaLogicID,
		[][]byte{addresses[2].Bytes()},
		[][]byte{common.NilHash.Bytes()},
	)

	balance, balanceHash := getTestBalance(t, getAssetMap(getAssetIDsAndBalances(t, 2)))

	cache, err := lru.New(100)
	require.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		StorageRoot: storageRoot,
	}, NilMetrics())

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithStateHash(common.SargaAddress, stateHash),
		1: getTesseractParamsWithStateHash(tests.RandomAddress(t), tests.RandomHash(t)),
	}

	tesseracts := tests.CreateTesseracts(t, 2, tesseractParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertTesseractsAndIxnsInDB(t, db, tesseracts...)
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
			insertBalancesInDB(t, db, []common.Hash{balanceHash}, balance)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, tesseracts...)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name                string
		tsHash              common.Hash
		address             identifiers.Address
		isAccountRegistered bool
		expectedError       error
	}{
		{
			name:          "should fail if tesseract not found",
			tsHash:        tests.RandomHash(t),
			address:       addresses[0],
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name:          "should fail if state object not found",
			tsHash:        tesseracts[1].Hash(),
			address:       addresses[0],
			expectedError: common.ErrStateNotFound,
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

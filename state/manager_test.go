package state

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/decred/dcrd/crypto/blake256"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/hashicorp/golang-lru"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	so := sm.CreateStateObject(address, accType, true)

	validateStateObject(t, so, accType, address, true)
}

func TestStateManager_HasParticipantStateAt(t *testing.T) {
	accounts, stateHashes := getTestAccounts(t, []common.Hash{tests.RandomHash(t), tests.RandomHash(t)}, 1)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
		},
	}

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name      string
		address   identifiers.Address
		stateHash common.Hash
		hasState  bool
	}{
		{
			name:      "participant state exist",
			address:   so[0].address,
			stateHash: stateHashes[0],
			hasState:  true,
		},
		{
			name:      "participant state doesn't exist",
			address:   so[0].address,
			stateHash: tests.RandomHash(t),
			hasState:  false,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hasState := sm.HasParticipantStateAt(test.address, test.stateHash)
			require.Equal(t, test.hasState, hasState)
		})
	}
}

func TestStateManager_GetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxnsAndReceipts(t, 4, 2)
	tesseracts := tests.CreateTesseracts(t, 5, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, tesseracts[:4]...)
		},
		smCallBack: func(sm *StateManager) {
			sm.cache.Add(tesseracts[4].Hash(), tesseracts[4])
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name             string
		hash             common.Hash
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
		withCommitInfo   bool
	}{
		// TODO: write a test to fetch ts with commit info
		{
			name:             "fetches tesseract from cache", // only tesseracts without interactions exists in cache
			hash:             tesseracts[4].Hash(),
			withInteractions: false,
			expectedTS:       tesseracts[4],
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
		{
			name:             "with commit info",
			hash:             tesseracts[2].Hash(),
			withInteractions: false,
			withCommitInfo:   true,
			expectedTS:       tesseracts[2],
		},
		{
			name:             "without commit info",
			hash:             tesseracts[2].Hash(),
			withInteractions: false,
			withCommitInfo:   false,
			expectedTS:       tesseracts[2],
		},
		{
			name:             "with interactions and commit info",
			hash:             tesseracts[3].Hash(),
			withInteractions: true,
			withCommitInfo:   true,
			expectedTS:       tesseracts[3],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := sm.getTesseractByHash(test.hash, test.withInteractions, test.withCommitInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			tests.ValidateTesseract(t, test.expectedTS, ts, test.withInteractions, test.withCommitInfo)
			checkForTesseractInSMCache(t, sm, ts, test.withInteractions, test.withCommitInfo)
		})
	}
}

func TestStateManager_GetStateObjectByHash(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)
	assetIDs, bal := getAssetIDsAndBalances(t, 1)
	_, assetRoot := createTestAssets(t, address, db, assetIDs, bal)
	accounts, stateHashes := getTestAccounts(t, []common.Hash{assetRoot}, 1)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
			db:      db,
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...) // insert account into db
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

func TestStateManager_GetLatestStateObject_WithStateCache(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)
	assetIDs, bal := getAssetIDsAndBalances(t, 2)
	_, assetRoot := createTestAssets(t, address, db, assetIDs, bal)
	accounts, stateHashes := getTestAccounts(t, []common.Hash{assetRoot, common.NilHash}, 2)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
			db:      db,
		},
		1: {
			account: accounts[1],
			db:      db,
		},
	}

	so := createTestStateObjects(t, 2, soParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes[0:1], accounts[0:1]...)
			for i := 0; i < 2; i++ {
				db.setAccountMetaInfo(&common.AccountMetaInfo{
					Address:   so[i].address,
					StateHash: stateHashes[i],
				})
			}
		},
	}
	sm := createTestStateManager(t, smParams)
	soInCache := createTestStateObject(t, nil)
	sm.objectCache.Add(soInCache.address, soInCache)
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
			name:    "state object fetched from object cache",
			address: soInCache.address,
			sObj:    soInCache,
		},
		{
			name:          "should fail if acc meta info not found",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch acc meta info"),
		},
		{
			name:          "should fail if state object not found",
			address:       so[1].Address(),
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

			if sm.objectCache != nil {
				data, ok := sm.objectCache.Get(test.address)
				require.True(t, ok)
				so, ok := data.(*Object)
				require.True(t, ok)
				checkForStateObject(t, so, latestStateObject)
			}
		})
	}
}

func TestStateManager_GetLatestStateObject_WithoutStateCache(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)
	assetIDs, bal := getAssetIDsAndBalances(t, 1)
	_, assetRoot := createTestAssets(t, address, db, assetIDs, bal)
	accounts, stateHashes := getTestAccounts(t, []common.Hash{assetRoot}, 1)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
			db:      db,
		},
	}

	so := createTestStateObjects(t, 1, soParams)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
			db.setAccountMetaInfo(&common.AccountMetaInfo{
				Address:   so[0].address,
				StateHash: stateHashes[0],
			})
		},
		smCallBack: func(sm *StateManager) {
			sm.objectCache = nil
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
			require.True(t, sm.objectCache == nil)
		})
	}
}

func TestStateManager_GetStateObject(t *testing.T) {
	account, stateHash := getTestAccounts(t, []common.Hash{common.NilHash, common.NilHash}, 2)

	so := NewStateObject(tests.RandomAddress(t), nil, nil, mockDB(), *account[0], NilMetrics(),
		false)
	so1 := NewStateObject(tests.RandomAddress(t), nil, nil, mockDB(), *account[1], NilMetrics(),
		false)
	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHash, account...)
		},
	}
	sm := createTestStateManager(t, smParams)
	sm.objectCache.Add(so1.address, so1)
	testcases := []struct {
		name      string
		address   identifiers.Address
		stateHash common.Hash
		sObj      *Object
	}{
		{
			name:      "fetch state object by state hash",
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
			behCtx, randCtx, err := sm.GetContext(identifiers.NilAddress, test.hash)

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
	address := tests.RandomAddress(t)
	ts := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{address},
		Participants: common.ParticipantsState{
			address: {
				LatestContext: mHash[1],
			},
		},
	})

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

func TestStateManager_GetCommittedContextHash(t *testing.T) {
	address := tests.RandomAddress(t)
	contextHash := tests.RandomHash(t)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			db.setAccountMetaInfo(&common.AccountMetaInfo{
				Address:     address,
				ContextHash: contextHash,
			})
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
			expectedError: errors.New("failed to fetch account meta info"),
		},
		{
			name:        "context exists",
			address:     address,
			contextHash: contextHash,
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

func TestStateManager_GetICSSeed(t *testing.T) {
	addresses := tests.GetRandomAddressList(t, 2)
	icsSeed := tests.RandomHash(t)
	tesseract := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{addresses[0]},
		TSDataCallback: func(ts *tests.TesseractData) {
			ts.ConsensusInfo.ICSSeed = icsSeed
		},
	})

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			db.setAccountMetaInfo(&common.AccountMetaInfo{
				Address:       addresses[0],
				TesseractHash: tesseract.Hash(),
			})

			db.setAccountMetaInfo(&common.AccountMetaInfo{
				Address:       addresses[1],
				TesseractHash: tests.RandomHash(t),
			})

			insertTesseractsInDB(t, db, tesseract)
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name        string
		address     identifiers.Address
		expectedErr error
		icsSeed     [32]byte
	}{
		{
			name:        "account meta info not found",
			address:     tests.RandomAddress(t),
			expectedErr: common.ErrKeyNotFound,
		},
		{
			name:        "tesseract not found",
			address:     addresses[1],
			expectedErr: common.ErrFetchingTesseract,
		},
		{
			name:    "valid ics seed exists",
			address: addresses[0],
			icsSeed: icsSeed,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			seed, err := sm.GetICSSeed(test.address)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.icsSeed, seed)
		})
	}
}

func TestStateManager_IsInitialTesseract_Without_Sarga(t *testing.T) {
	db := mockDB()
	addresses := tests.GetRandomAddressList(t, 2)

	so := NewStateObject(common.SargaAddress, mockCache(t), nil, db, common.Account{
		AccType: common.SargaAccount,
	}, NilMetrics(), false)

	so.storageTreeTxns[common.SargaLogicID] = iradix.New().Txn()

	err := so.SetStorageEntry(common.SargaLogicID, addresses[0].Bytes(), []byte{0x01})
	assert.NoError(t, err)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.objectCache.Add(common.SargaAddress, so)
		},
	}

	sm := createTestStateManager(t, smParams)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParamsWithStateHash(addresses[0], tests.RandomHash(t)),
		1: getTesseractParamsWithStateHash(addresses[1], tests.RandomHash(t)),
	}

	tesseracts := tests.CreateTesseracts(t, 2, tesseractParams)

	// Define test cases
	testcases := []struct {
		name          string
		address       identifiers.Address
		tesseract     *common.Tesseract
		expectedError error
		isInitialTS   bool
	}{
		{
			name:          "account not registered",
			address:       addresses[1],
			tesseract:     tesseracts[0],
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name:        "account already registered",
			address:     addresses[0],
			tesseract:   tesseracts[0],
			isInitialTS: false,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			isInitialTS, err := sm.IsInitialTesseract(test.tesseract, test.address)

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.isInitialTS, isInitialTS)
		})
	}
}

func TestStateManager_IsInitialTesseract_With_Sarga(t *testing.T) {
	addresses := tests.GetAddresses(t, 2)
	db := mockDB()

	_, storageRoot := createMetaStorageTree(
		t,
		db,
		common.SargaAddress,
		common.SargaLogicID,
		[][]byte{addresses[0].Bytes()},
		[][]byte{common.NilHash.Bytes()},
	)

	cache, err := lru.New(100)
	require.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		StorageRoot: storageRoot,
	}, NilMetrics(), false)

	stateHash, err := so.Commit()
	assert.NoError(t, err)

	initialTS := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{
			addresses[0],
			common.SargaAddress,
		},
		Participants: common.ParticipantsState{
			common.SargaAddress: {
				StateHash: stateHash,
			},
		},
	})

	tesseract := tests.CreateTesseract(t, &tests.CreateTesseractParams{
		Addresses: []identifiers.Address{common.SargaAddress},
		Participants: common.ParticipantsState{
			common.SargaAddress: {
				TransitiveLink: initialTS.Hash(),
			},
		},
	})

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, initialTS)
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, initialTS)
		},
	}

	sm := createTestStateManager(t, smParams)

	// Define test cases
	testcases := []struct {
		name          string
		address       identifiers.Address
		tesseract     *common.Tesseract
		expectedError error
		isInitialTS   bool
	}{
		{
			name:        "account not registered",
			address:     addresses[1],
			tesseract:   tesseract,
			isInitialTS: true,
		},
		{
			name:        "account already registered",
			address:     addresses[0],
			tesseract:   tesseract,
			isInitialTS: false,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			isInitialTS, err := sm.IsInitialTesseract(test.tesseract, test.address)

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.isInitialTS, isInitialTS)
		})
	}
}

func TestStateManager_FetchIxStateObjects(t *testing.T) {
	accounts, stateHashes := getTestAccounts(t, []common.Hash{tests.RandomHash(t)}, 1)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
		},
	}

	so := createTestStateObjects(t, 1, soParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...) // insert account into db
			db.setAccountMetaInfo(&common.AccountMetaInfo{
				Address:   so[0].Address(),
				StateHash: stateHashes[0],
			})
		},
	}

	sm := createTestStateManager(t, smParams)

	ixn := tests.CreateIX(t, &tests.CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.IxOps = []common.IxOpRaw{}
			ix.Sender = so[0].Address()
			ix.Participants = []common.IxParticipant{
				{
					Address: so[0].Address(),
				},
			}
		},
	})

	testcases := []struct {
		name          string
		ixns          common.Interactions
		stateHashes   map[identifiers.Address]common.Hash
		expectedError error
	}{
		{
			name: "fetch latest state objects",
			ixns: common.NewInteractionsWithLeaderCheck(false, ixn),
		},
		{
			name: "fetch state objects by hash",
			ixns: common.NewInteractionsWithLeaderCheck(false, ixn),
			stateHashes: map[identifiers.Address]common.Hash{
				so[0].Address(): stateHashes[0],
			},
		},
		{
			name: "state object doesn't exist",
			ixns: common.NewInteractionsWithLeaderCheck(false, ixn),
			stateHashes: map[identifiers.Address]common.Hash{
				so[0].Address(): tests.RandomHash(t),
			},
			expectedError: errors.New("state object fetch failed"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			transisition, err := sm.FetchIxStateObjects(test.ixns, test.stateHashes)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.NotNil(t, transisition)
			require.NotNil(t, transisition.GetObject(so[0].address))
		})
	}
}

/*

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

	testcases := []struct {
		name          string
		hash          common.Hash
		preTestFn     func(sm *StateManager)
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
			preTestFn: func(sm *StateManager) {
				createGuardianLogic(t, sm, kramaIDs[4:6], pk[4:6])
			},
		},
		{
			name: "valid hash and public keys",
			hash: mHash[0],
			preTestFn: func(sm *StateManager) {
				createGuardianLogic(t, sm, kramaIDs, pk)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := createTestStateManager(t, smParams)

			if test.preTestFn != nil {
				test.preTestFn(sm)
			}

			behCtx, randCtx, err := sm.fetchParticipantContextByHash(identifiers.NilAddress, test.hash)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfNodesetEqual(
				t,
				types.NewNodeSet(obj[0].Ids, pk[:2], uint32(len(obj[0].Ids))),
				types.NewNodeSet(obj[1].Ids, pk[2:4], uint32(len(obj[1].Ids))),
				behCtx,
				randCtx,
			)
		})
	}
}

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
		tess          *common.ts
		nodes         *ICSNodes
		preTestFn        func()
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
			if test.preTestFn != nil {
				test.preTestFn()
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

	so := NewStateObject(common.SargaAddress, mockCache(t), nil, db, common.Account{
		AccType: common.SargaAccount,
	}, NilMetrics(), false)

	so.storageTreeTxns[common.SargaLogicID] = iradix.New().Txn()

	err := so.SetStorageEntry(common.SargaLogicID, address.Bytes(), []byte{0x01})
	assert.NoError(t, err)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.objectCache.Add(common.SargaAddress, so)
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
			name:          "non-registered account",
			address:       tests.RandomAddress(t),
			smParams:      smParams,
			expectedError: common.ErrLogicStorageTreeNotFound,
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
	accounts0 := &common.Account{
		Nonce: 12,
	}

	stateHash0, err := accounts0.Hash()
	assert.NoError(t, err)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, []common.Hash{stateHash0}, accounts0)
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
	db := mockDB()
	address := tests.RandomAddress(t)
	assetIDs, balances := getAssetIDsAndBalances(t, 2)
	assets, assetRoot := createTestAssets(t, address, db, assetIDs, balances)
	accounts, stateHashes := getTestAccounts(t, []common.Hash{assetRoot}, 1)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		address       identifiers.Address
		stateHash     common.Hash
		assets        common.AssetMap
		expectedError error
	}{
		{
			name:      "fetch balances at particular state",
			address:   tests.RandomAddress(t),
			stateHash: stateHashes[0],
			assets:    assets,
		},
		{
			name:          "failed to fetch state object",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch state object"),
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
			require.Equal(t, test.assets, balance)
		})
	}
}

func TestStateManager_GetBalance(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)
	assetIDs, bal := getAssetIDsAndBalances(t, 1)
	assets, assetRoot := createTestAssets(t, address, db, assetIDs, bal)
	accounts, stateHashes := getTestAccounts(t, []common.Hash{assetRoot}, 1)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		address       identifiers.Address
		assetID       identifiers.AssetID
		stateHash     common.Hash
		assets        common.AssetMap
		expectedError error
	}{
		{
			name:      "fetch balance at particular state",
			address:   address,
			assetID:   assetIDs[0],
			stateHash: stateHashes[0],
			assets:    assets,
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
				require.Equal(t, big.NewInt(0), balance)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.assets[test.assetID], balance)
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
		common.Account{}, NilMetrics(), false)

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
	}

	sm := createTestStateManager(t, smParams)

	newRoot := storageTree.Root()

	testcases := []struct {
		name                  string
		newRoot               *common.RootNode
		logicID               identifiers.LogicID
		logicStorageTreeRoots map[string]*common.RootNode
		stateObject           *Object
		expectedError         error
	}{
		{
			name:    "tree is synced successfully with the new root",
			newRoot: &newRoot,
			logicID: logicIDs[0],
			logicStorageTreeRoots: map[string]*common.RootNode{
				string(logicIDs[0]): &newRoot,
			},
			stateObject: stateObject,
		},
		{
			name:    "tree is not synced properly",
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
			err := sm.SyncStorageTrees(context.Background(), test.newRoot, test.logicStorageTreeRoots, test.stateObject)
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

//nolint:dupl
func TestStateManager_SyncAssetTree(t *testing.T) {
	db := mockDB()
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	rawData := assetID.Bytes()

	assetTree, err := tree.NewKramaHashTree(assetID.Address(), common.NilHash, db, blake256.New(),
		storage.Asset, nil, tree.NilMetrics())
	require.NoError(t, err)

	err = assetTree.Set(assetID.Bytes(), rawData)
	require.NoError(t, err)

	soParams := map[int]*createStateObjectParams{
		0: stateObjectParamsWithAssetTree(t, assetID.Address(), db, assetTree, common.NilHash, nil),
	}

	so := createTestStateObjects(t, 1, soParams)

	smParams := &createStateManagerParams{
		db: db,
	}

	sm := createTestStateManager(t, smParams)

	newRoot := so[0].assetTree.Root()

	testcases := []struct {
		name          string
		newRoot       *common.RootNode
		assetTree     tree.MerkleTree
		expectedError error
	}{
		{
			name:      "asset tree synced successfully",
			newRoot:   &newRoot,
			assetTree: so[0].assetTree,
		},
		{
			name: "tree is not synced properly",
			newRoot: &common.RootNode{
				MerkleRoot: tests.RandomHash(t),
				HashTable:  map[string][]byte{tests.RandomHash(t).String(): tests.RandomHash(t).Bytes()},
			},
			assetTree:     so[0].assetTree,
			expectedError: errors.New("updated root doesn't match"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sm.SyncAssetTree(test.newRoot, so[0])
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			root := test.assetTree.Root()
			require.Equal(t, test.newRoot, &root)
		})
	}
}

//nolint:dupl
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
	}

	so := createTestStateObjects(t, 1, soParams)

	smParams := &createStateManagerParams{
		db: db,
	}

	sm := createTestStateManager(t, smParams)

	newRoot := so[0].logicTree.Root()

	testcases := []struct {
		name          string
		newRoot       *common.RootNode
		logicTree     tree.MerkleTree
		expectedError error
	}{
		{
			name:      "logic tree synced successfully",
			newRoot:   &newRoot,
			logicTree: so[0].logicTree,
		},
		{
			name: "tree is not synced properly",
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
			err := sm.SyncLogicTree(test.newRoot, so[0])
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


func TestStateManager_GetICSNodeSetFromRawContext(t *testing.T) {
	addrs := tests.GetAddresses(t, 2)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)

	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(t, addrs[0], identifiers.NilAddress),
		1: tests.GetIxParamsWithAddress(t, tests.RandomAddress(t), addrs[1]),
		2: tests.GetIxParamsWithAddress(t, tests.RandomAddress(t), common.SargaAddress),
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
			createGuardianLogic(t, sm, kramaIDs, pk)
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
				require.Equal(t, kramaIDs, nodeSet.Sets[setType].Nodes)
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

*/

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
			expectedError: common.ErrContextStateNotFound,
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

func TestStateManager_GetPersistentStorageEntry(t *testing.T) {
	db := mockDB()
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))

	so := NewStateObject(logicID.Address(), mockCache(t), nil, db, common.Account{}, NilMetrics(), false)

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
			storageEntry, err := sm.GetPersistentStorageEntry(test.logicID, test.slot, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedStorageEntry, storageEntry)
		})
	}
}

func TestStateManager_GetEphemeralStorageEntry(t *testing.T) {
	db := mockDB()
	address := tests.RandomAddress(t)
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))

	so := NewStateObject(address, mockCache(t), nil, db, common.Account{}, NilMetrics(), false)

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
		address              identifiers.Address
		logicID              identifiers.LogicID
		slot                 []byte
		stateHash            common.Hash
		expectedStorageEntry []byte
		expectedError        error
	}{
		{
			name:          "should return an error if state object not found",
			address:       tests.RandomAddress(t),
			logicID:       logicID,
			slot:          logicID.Bytes(),
			stateHash:     tests.RandomHash(t),
			expectedError: errors.New("failed to fetch state object"),
		},
		{
			name:          "should return an error if logic storage tree not found",
			address:       tests.RandomAddress(t),
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			slot:          logicID.Bytes(),
			stateHash:     stateHash,
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name:                 "should fetch storage data successfully",
			address:              address,
			logicID:              logicID,
			slot:                 logicID.Bytes(),
			stateHash:            stateHash,
			expectedStorageEntry: []byte{0x01},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			storageEntry, err := sm.GetEphemeralStorageEntry(test.address, test.logicID, test.slot, test.stateHash)
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

	so := NewStateObject(logicID.Address(), mockCache(t), nil, db, common.Account{}, NilMetrics(), false)

	err := so.InsertNewLogicObject(logicID, logicObject)
	require.NoError(t, err)

	_, err = so.commitLogics()
	require.NoError(t, err)

	err = so.flushLogicTree()
	require.NoError(t, err)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			sm.objectCache.Add(so.address, so)
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
	db := mockDB()

	assetAddrs := tests.RandomAddress(t)
	assetInfo := tests.GetRandomAssetInfo(t, assetAddrs)

	sObj := createTestStateObject(t, &createStateObjectParams{
		db:      db,
		address: assetAddrs,
	})

	assetID, assetRoot := createTestAssetInAssetAccount(t, sObj, assetInfo)

	accounts, stateHashes := getTestAccounts(t, []common.Hash{assetRoot}, 1)

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
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
			stateHash:         stateHashes[0],
			expectedAssetInfo: assetInfo,
		},
		{
			name:          "should return error as asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			stateHash:     stateHashes[0],
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

func TestStateManager_GetDeeds(t *testing.T) {
	db := mockDB()

	assetAddr := tests.RandomAddress(t)
	creatorAddrs := tests.GetRandomAddressList(t, 2)
	assetInfo := tests.GetRandomAssetInfo(t, assetAddr)

	sObj := createTestStateObjects(t, 2, map[int]*createStateObjectParams{
		0: {
			db:      db,
			address: assetAddr,
		},
		1: {
			db:      db,
			address: creatorAddrs[0],
		},
		2: {
			db:      db,
			address: creatorAddrs[1],
		},
	})

	assetID, assetRoot := createTestAssetInAssetAccount(t, sObj[0], assetInfo)
	assetRoot1 := createTestAssetInRegularAccount(t, sObj[1], assetID, assetInfo)

	deeds, deedsHash := getTestDeeds(
		t,
		map[string]struct{}{string(assetID): {}},
	)

	assetAcc, assetStateHash := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.AssetRoot = assetRoot
	})

	operatorAcc, operatorStateHash := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.AssetRoot = assetRoot1
		acc.AssetDeeds = deedsHash
	})

	operatorAcc1, operatorStateHash1 := tests.GetTestAccount(t, func(acc *common.Account) {})

	smParams := &createStateManagerParams{
		db: db,
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(
				t, db, []common.Hash{assetStateHash, operatorStateHash, operatorStateHash1},
				[]*common.Account{assetAcc, operatorAcc, operatorAcc1}...,
			)
			insertAssetDeedsInDB(t, db, []common.Hash{deedsHash}, deeds)
			db.setAccountMetaInfo(&common.AccountMetaInfo{
				Address:   assetAddr,
				StateHash: assetStateHash,
			})
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name                string
		address             identifiers.Address
		stateHash           common.Hash
		expectedDeedEntries map[string]*common.AssetDescriptor
		expectedError       error
	}{
		{
			name:          "failed to fetch state object",
			address:       tests.RandomAddress(t),
			stateHash:     tests.RandomHash(t),
			expectedError: errors.New("failed to fetch state object"),
		},
		{
			name:      "fetch deeds entries successfully",
			address:   creatorAddrs[0],
			stateHash: operatorStateHash,
			expectedDeedEntries: map[string]*common.AssetDescriptor{
				string(assetID): assetInfo,
			},
		},
		{
			name:                "fetch empty deeds object",
			address:             creatorAddrs[1],
			stateHash:           operatorStateHash1,
			expectedDeedEntries: map[string]*common.AssetDescriptor{},
		},
		{
			name:          "state object doesn't exist",
			address:       creatorAddrs[1],
			stateHash:     tests.RandomHash(t),
			expectedError: common.ErrStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			deeds, err := sm.GetDeeds(test.address, test.stateHash)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedDeedEntries, deeds)
		})
	}
}

func TestStateManager_GetLogicManifest(t *testing.T) {
	db := mockDB()

	engineio.RegisterEngine(pisa.NewEngine())

	manifest, err := engineio.NewManifestFromFile("../compute/exlogics/tokenledger/tokenledger.yaml")
	require.NoError(t, err)

	encodedManifest, err := manifest.Encode(common.POLO)
	require.NoError(t, err)

	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	logicObject.Manifest = manifest.Hash()

	so := NewStateObject(logicID.Address(), mockCache(t), nil, db, common.Account{}, NilMetrics(), false)

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
	so := NewStateObject(address, mockCache(t), nil, db, common.Account{}, NilMetrics(), false)

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
	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	addresses := tests.GetAddresses(t, 3)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			createGuardianLogic(t, sm, append(kramaIDs[:4], kramaIDs[8:10]...),
				append(pk[:4], pk[8:10]...))
		},
		dbCallback: func(db *MockDB) {
			insertMetaContextsInDB(t, db, mObj...)
			insertContextsInDB(t, db, obj...)
			for i := 0; i < 3; i++ {
				db.setAccountMetaInfo(&common.AccountMetaInfo{
					Address:     addresses[i],
					ContextHash: mHash[i],
				})
			}
		},
	}

	sm := createTestStateManager(t, smParams)
	testcases := []struct {
		name          string
		address       identifiers.Address
		ctxHash       common.Hash
		behSet        []id.KramaID
		randSet       []id.KramaID
		behKeys       [][]byte
		randKeys      [][]byte
		expectedError error
	}{
		{
			name:          "tesseract doesn't exist",
			address:       tests.RandomAddress(t),
			expectedError: errors.New("failed to fetch account meta info"),
		},
		{
			name:          "behavioural context Nodes doesn't have public keys",
			address:       addresses[1],
			expectedError: common.ErrPublicKeyNotFound,
		},
		{
			name:          "random context Nodes doesn't have public keys",
			address:       addresses[2],
			expectedError: common.ErrPublicKeyNotFound,
		},
		{
			name:     "valid hash and public keys",
			address:  addresses[0],
			ctxHash:  mHash[0],
			behSet:   obj[0].Ids,
			behKeys:  pk[:2],
			randSet:  obj[1].Ids,
			randKeys: pk[2:4],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, behSet, randSet, behKeys, randKeys, err := sm.GetLatestContextAndPublicKeys(test.address)

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
				test.behKeys,
				test.randKeys,
				behSet,
				randSet,
				behKeys,
				randKeys,
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
			insertTesseractsInDB(t, db, ts)
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
			insertTesseractsInDB(t, db, sargaTesseract)
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
		behSet        *common.committee
		randSet       *common.committee
		address       identifiers.Address
		contextHash   common.Hash
		preTestFn        func()
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
			nodeSet := make([]*common.committee, 4)
			contextHashes := make(map[identifiers.Address]common.Hash)

			if test.preTestFn != nil {
				test.preTestFn()
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
		behSet        *common.committee
		randSet       *common.committee
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
					insertTesseractsInDB(t, db, ts)
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
			nodeSet := make([]*common.committee, 4)
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

func TestStateManager_FetchICSNodeSet(t *testing.T) {
	addr := tests.RandomAddress(t)
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 12)

	obj, cHash := getContextObjects(t, kramaIDs, 2, 6)
	mObj, mHash := getMetaContextObjects(t, cHash)

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(t, addr, tests.RandomAddress(t)),
	}

	ixs := tests.CreateIxns(t, 1, ixParams)

	tesseractParams := map[int]*tests.CreateTesseractParams{
		0: getTesseractParams(addr, common.Interactions{ixs[0]}, mHash[0]),
	}

	ts := tests.CreateTesseracts(t, 1, tesseractParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertTesseractsInDB(t, db, ts...)
			insertContextsInDB(t, db, obj...)
			insertMetaContextsInDB(t, db, mObj...)
		},
		smCallBack: func(sm *StateManager) {
			storeTesseractHashInCache(t, sm.cache, ts...)
			createGuardianLogic(t, sm, kramaIDs[:8], pk[:8])
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
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "failed to fetch updated ics node set as public keys of observer set not present in senatus",
			ts:   ts[0],
			clusterInfo: &common.ICSClusterInfo{
				RandomSet:   obj[2].Ids,
				ObserverSet: obj[5].Ids,
				Responses:   createRandomArrayOfBits(t, 6),
			},
			expectedError: common.ErrLogicStorageTreeNotFound,
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
				require.Equal(t, kramaIDs, icsNodes.Sets[setType].Nodes)
			}

			for index, set := range icsNodes.Sets {
				if set != nil && test.clusterInfo.Responses[index] != nil {
					require.Equal(t, set.Responses, test.clusterInfo.Responses[index])
				}
			}
		})
	}
}

*/

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
			insertTesseractsInDB(t, db, ts...)
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
		preTestFn        func()
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
			preTestFn: func() {
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
			if test.preTestFn != nil {
				test.preTestFn()
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
		// Balance: tests.RandomHash(t),
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

	cache, err := lru.New(100)
	require.NoError(t, err)

	so := NewStateObject(common.SargaAddress, cache, nil, db, common.Account{
		StorageRoot: storageRoot,
	}, NilMetrics(), false)
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
			insertTesseractsInDB(t, db, tesseracts...)
			insertAccountsInDB(t, db, []common.Hash{stateHash}, so.Data())
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

func TestStateManager_LoadTransitionObjects(t *testing.T) {
	accounts, stateHashes := getTestAccounts(t, tests.GetHashes(t, 3), 3)

	soParams := map[int]*createStateObjectParams{
		0: {
			account: accounts[0],
		},
		1: {
			account: accounts[1],
		},
		2: {
			account: accounts[1],
		},
	}

	so := createTestStateObjects(t, 3, soParams)

	smParams := &createStateManagerParams{
		dbCallback: func(db *MockDB) {
			insertAccountsInDB(t, db, stateHashes, accounts...)
			for i := 0; i < 2; i++ {
				db.setAccountMetaInfo(&common.AccountMetaInfo{
					Address:   so[i].address,
					StateHash: stateHashes[i],
				})
			}
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name         string
		participants map[identifiers.Address]common.ParticipantInfo
		expectedErr  error
	}{
		{
			name: "account not found",
			participants: map[identifiers.Address]common.ParticipantInfo{
				tests.RandomAddress(t): {
					Address: tests.RandomAddress(t),
				},
			},
			expectedErr: common.ErrKeyNotFound,
		},
		{
			name: "state object not found",
			participants: map[identifiers.Address]common.ParticipantInfo{
				so[2].address: {
					Address: so[2].address,
				},
			},
			expectedErr: errors.New("state object fetch failed"),
		},
		{
			name: "transition object loaded",
			participants: map[identifiers.Address]common.ParticipantInfo{
				so[0].address: {
					Address: so[0].address,
				},
				so[1].address: {
					Address: so[1].address,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			transition, err := sm.LoadTransitionObjects(test.participants)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.NotNil(t, transition.objects.GetObject(so[0].Address()))
			require.NotNil(t, transition.objects.GetObject(so[1].Address()))
		})
	}
}

func TestStateManager_GetPublicKeys(t *testing.T) {
	kramaIDs, pk := tests.GetTestKramaIdsWithPublicKeys(t, 4)

	addresses := tests.GetAddresses(t, 3)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			createGuardianLogic(t, sm, kramaIDs, pk)
		},
		dbCallback: func(db *MockDB) {
			for i := 0; i < 3; i++ {
				db.setAccountMetaInfo(&common.AccountMetaInfo{
					Address: addresses[i],
				})
			}
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name          string
		kramaIDs      []id.KramaID
		sm            *StateManager
		expectedError error
	}{
		{
			name:          "empty krama ids",
			sm:            sm,
			expectedError: errors.New("Empty Ids"),
		},
		{
			name:          "guardian logic state doesn't exist",
			kramaIDs:      kramaIDs,
			sm:            createTestStateManager(t, nil),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:          "non existing krama ids",
			kramaIDs:      tests.RandomKramaIDs(t, 4),
			sm:            sm,
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name:     "valid krama ids",
			kramaIDs: kramaIDs,
			sm:       sm,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			publicKeys, err := test.sm.GetPublicKeys(context.Background(), test.kramaIDs...)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, publicKeys, pk)
		})
	}
}

func TestStateManager_IsSealValid(t *testing.T) {
	kramaIDs := tests.RandomKramaIDs(t, 2)
	addresses := tests.GetAddresses(t, 2)
	tesseracts, signedTS, pks := createAndSignTesseracts(t, 2)

	smParams := &createStateManagerParams{
		smCallBack: func(sm *StateManager) {
			createGuardianLogic(t, sm, kramaIDs, pks)
		},
		dbCallback: func(db *MockDB) {
			for i := 0; i < 1; i++ {
				db.setAccountMetaInfo(&common.AccountMetaInfo{
					Address: addresses[i],
				})
			}
		},
	}

	sm := createTestStateManager(t, smParams)

	testcases := []struct {
		name        string
		tesseract   *common.Tesseract
		preTestFn   func(ts *common.Tesseract)
		isValid     bool
		expectedErr error
	}{
		{
			name:      "seal is valid",
			tesseract: tesseracts[0],
			preTestFn: func(ts *common.Tesseract) {
				ts.SetSeal(signedTS[0])
				ts.SetSealBy(kramaIDs[0])
			},
			isValid: true,
		},
		{
			name: "sealer logic tree not found",
			tesseract: func() *common.Tesseract {
				ts := tests.CreateTesseract(t, nil)
				ts.SetSealBy(tests.RandomKramaID(t, 0))

				return ts
			}(),
			isValid:     false,
			expectedErr: common.ErrLogicStorageTreeNotFound,
		},
		{
			name:      "seal is invalid",
			tesseract: tesseracts[1],
			preTestFn: func(ts *common.Tesseract) {
				ts.SetSeal(signedTS[0])
				ts.SetSealBy(kramaIDs[1])
			},
			isValid: false,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.tesseract)
			}

			valid, err := sm.IsSealValid(test.tesseract)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.isValid, valid)
		})
	}
}

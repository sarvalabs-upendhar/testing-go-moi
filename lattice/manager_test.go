package lattice

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/storage"
)

func TestHasTesseract(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

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

func TestUpdateNodeInclusivity(t *testing.T) {
	senatus := mockSenatus(t) // initializing senatus here as we need to validate results by using this
	chainParams := &CreateChainParams{
		senatus: senatus,
	}
	c := createTestChainManager(t, chainParams)

	// create context delta
	deltaGroup := getDeltaGroup(t, 2, 2, 2)

	err := c.UpdateNodeInclusivity(deltaGroup)
	require.NoError(t, err)

	validateDeltaGroup(t, senatus, deltaGroup)
}

func TestGetTesseract(t *testing.T) {
	// set the interactions to nil for first tesseract
	ts := tests.CreateTesseracts(t, 1, nil)
	tsWithInteractions := tests.CreateTesseracts(t, 2, getTesseractParamsMapWithIxns(t, 3))
	ts = append(ts, tsWithInteractions...)

	chainParams := &CreateChainParams{
		chainManagerCallback: func(c *ChainManager) {
			insertTesseractsInCache(t, c, ts[0])
		},
		dbCallback: func(db *MockDB) {
			db.InsertTesseractsInDB(t, ts[1:]...)
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

	ts := tests.CreateTesseract(t, getTesseractParamsMapWithIxns(t, 1)[0])

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			insertTesseractByHeight(t, db, ts)
			db.InsertTesseractsInDB(t, ts)
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

func TestAddParticipantData(t *testing.T) {
	addresses := tests.GetAddresses(t, 2)
	objects := make(map[identifiers.Address]*state.Object)
	keys := getHexEntries(t, 2)
	values := getHexEntries(t, 2)

	tsParams := &tests.CreateTesseractParams{
		Addresses: addresses,
		Participants: common.ParticipantsState{
			addresses[0]: {
				Height: 11,
			},
			addresses[1]: {
				Height: 23,
			},
		},
	}

	testcases := []struct {
		name            string
		address         identifiers.Address
		allParticipants bool
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
			name:            "added only one participant data successfully",
			address:         addresses[0],
			allParticipants: false,
		},
		{
			name:            "added all participant data successfully",
			allParticipants: true,
		},
		{
			name: "failed to flush dirty object",
			dbCallback: func(db *MockDB) {
				db.createEntryHook = func() error {
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
			ts := tests.CreateTesseract(t, tsParams)
			db := mockDB()
			pool := mockIXPool()

			chainParams := &CreateChainParams{
				db:         db,
				dbCallback: test.dbCallback,
				ixPool:     pool,
			}

			c := createTestChainManager(t, chainParams)

			for i := 0; i < 2; i++ {
				obj := state.NewStateObject(addresses[i], mockCache(), nil, db,
					common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false)

				for i, k := range keys {
					obj.SetDirtyEntry(k, []byte(values[i]))
				}

				objects[addresses[i]] = obj
			}

			err := c.addParticipantsData(
				test.address,
				ts,
				state.NewTransition(objects),
				test.allParticipants,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			participants := make(common.ParticipantsState)

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
				_, ok := pool.removedObjects[addr]
				require.True(t, ok)

				checkForParticipantByHeight(t, c, addr, participant, true)

				// make sure transition object flushed to db
				for i, k := range keys {
					val, err := db.ReadEntry(common.FromHex(k))
					require.NoError(t, err)
					require.Equal(t, []byte(values[i]), val)
				}

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

func TestStoreReceipts(t *testing.T) {
	c := createTestChainManager(t, nil)
	_, receipts := getIxAndReceipts(t, 1)

	testcases := []struct {
		name               string
		batchWriterSetHook func() error
		params             *tests.CreateTesseractParams
		expectedError      error
	}{
		{
			name: "receipts stored successfully",
			params: &tests.CreateTesseractParams{
				Receipts: receipts,
			},
		},
		{
			name: "failed to store receipts",
			batchWriterSetHook: func() error {
				return errors.New("failed to write receipts")
			},
			params: &tests.CreateTesseractParams{
				Receipts: receipts,
			},
			expectedError: errors.New("failed to write receipts"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.params)

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
		params             *tests.CreateTesseractParams
		expectedError      error
	}{
		{
			name: "receipts stored successfully",
			params: &tests.CreateTesseractParams{
				Ixns: ixns,
			},
		},
		{
			name: "failed to store receipts",
			batchWriterSetHook: func() error {
				return errors.New("failed to write ixns")
			},
			params: &tests.CreateTesseractParams{
				Ixns: ixns,
			},
			expectedError: errors.New("failed to write ixns"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.params)

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
		params             *tests.CreateTesseractParams
		expectedError      error
	}{
		{
			name: "tesseract added successfully",
			params: &tests.CreateTesseractParams{
				Ixns:     ixns,
				Receipts: receipts,
			},
		},
		{
			name:   "failed to store tesseract",
			params: &tests.CreateTesseractParams{},
			batchWriterSetHook: func() error {
				return errors.New("failed to store tesseract")
			},
			expectedError: errors.New("failed to store tesseract"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.params)

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
	objects := make(map[identifiers.Address]*state.Object)
	keys := getHexEntries(t, 2)
	values := getHexEntries(t, 2)

	tsParams := &tests.CreateTesseractParams{
		Addresses: addresses,
		Participants: common.ParticipantsState{
			addresses[0]: {
				Height:       11,
				ContextDelta: getDeltaGroup(t, 1, 1, 1),
			},
			addresses[1]: {
				Height:       23,
				ContextDelta: getDeltaGroup(t, 1, 1, 1),
			},
		},
		Ixns:     ixns,
		Receipts: receipts,
	}

	testcases := []struct {
		name string
		// tsParams         *createTesseractParams
		shouldCache      bool
		dbCallback       func(db *MockDB)
		dbCallbackWithTS func(db *MockDB, ts *common.Tesseract)
		senatusCallback  func(senatus *MockSenatus)
		expectedError    error
	}{
		{
			name:        "added tesseract successfully with caching",
			shouldCache: true,
		},
		{
			name:        "added tesseract successfully without caching",
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
			dbCallback: func(db *MockDB) {
				db.setTesseractHeightEntryHook = func() error {
					return errors.New("failed to set tesseract height entry")
				}
			},
			expectedError: errors.New("failed to set tesseract height entry"),
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
			ts := tests.CreateTesseract(t, tsParams)
			db := mockDB()
			senatus := mockSenatus(t)
			ixpool := mockIXPool()

			if test.dbCallbackWithTS != nil {
				test.dbCallbackWithTS(db, ts)
			}

			chainParams := &CreateChainParams{
				db:              db,
				senatus:         senatus,
				ixPool:          ixpool,
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

			for i := 0; i < 2; i++ {
				obj := state.NewStateObject(addresses[i], mockCache(), nil, db,
					common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false)

				for i, k := range keys {
					obj.SetDirtyEntry(k, []byte(values[i]))
				}

				objects[addresses[i]] = obj
			}

			err := c.AddTesseract(
				test.shouldCache,
				identifiers.NilAddress,
				ts,
				state.NewTransition(objects),
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

	objects := make(map[identifiers.Address]*state.Object)
	keys := getHexEntries(t, 2)
	values := getHexEntries(t, 2)

	dirtyStorage := map[common.Hash][]byte{
		icsHash:             {1, 2, 3},
		tests.RandomHash(t): {4, 5, 6},
	}

	tsParams := &tests.CreateTesseractParams{
		Addresses: addresses,
		Participants: common.ParticipantsState{
			addresses[0]: {
				Height:       11,
				ContextDelta: getDeltaGroup(t, 1, 1, 1),
			},
			addresses[1]: {
				Height:       23,
				ContextDelta: getDeltaGroup(t, 1, 1, 1),
			},
		},
		TSDataCallback: func(ts *tests.TesseractData) {
			ts.ConsensusInfo.ICSHash = icsHash
		},
	}

	testcases := []struct {
		name            string
		address         identifiers.Address
		dirtyStorage    map[common.Hash][]byte
		allParticipants bool
		dbCallback      func(db *MockDB)
		expectedError   error
	}{
		{
			name:            "added tesseract with state successfully",
			address:         identifiers.NilAddress,
			dirtyStorage:    dirtyStorage,
			allParticipants: true,
		},
		{
			name:            "added tesseract's participant successfully",
			address:         addresses[0],
			dirtyStorage:    dirtyStorage,
			allParticipants: false,
		},
		{
			name: "failed to add tesseract",
			dbCallback: func(db *MockDB) {
				db.setTesseractHeightEntryHook = func() error {
					return errors.New("failed to set tesseract height entry")
				}
			},
			dirtyStorage:    dirtyStorage,
			allParticipants: true,
			expectedError:   errors.New("failed to set tesseract height entry"),
		},
		{
			name:            "empty dirty storage",
			allParticipants: true,
			expectedError:   errors.New("empty dirty storage"),
		},
		{
			name: "failed to write dirty keys",
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
			ts := tests.CreateTesseract(t, tsParams)

			db := mockDB()

			chainParams := &CreateChainParams{
				db:         db,
				dbCallback: test.dbCallback,
			}

			c := createTestChainManager(t, chainParams)

			participants := ts.Participants()

			for i := 0; i < 2; i++ {
				obj := state.NewStateObject(addresses[i], mockCache(), nil, db,
					common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false)

				for i, k := range keys {
					obj.SetDirtyEntry(k, []byte(values[i]))
				}

				objects[addresses[i]] = obj
			}

			err := c.AddTesseractWithState(
				test.address,
				test.dirtyStorage,
				ts,
				state.NewTransition(objects),
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
	ts := tests.CreateTesseracts(t, 1, getTesseractParamsMapWithIxns(t, 3))

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			db.InsertTesseractsInDB(t, ts...)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name                 string
		tsHash               common.Hash
		ixIndex              int
		expectedInteraction  *common.Interaction
		expectedParticipants common.ParticipantsState
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

	ts := tests.CreateTesseracts(t, 1, nil)
	tsWithInteractions := tests.CreateTesseracts(t, 1, getTesseractParamsMapWithIxns(t, 3))
	ts = append(ts, tsWithInteractions...)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			db.InsertTesseractsInDB(t, ts...)
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
		expectedParticipants common.ParticipantsState
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

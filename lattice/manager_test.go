package lattice

import (
	"context"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
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
	deltaGroup := getDeltaGroup(t, 2, 2)

	err := c.UpdateNodeInclusivity(deltaGroup)
	require.NoError(t, err)

	validateDeltaGroup(t, senatus, deltaGroup)
}

func TestGetTesseract(t *testing.T) {
	// set the interactions to nil for first tesseract
	ts := tests.CreateTesseracts(t, 1, map[int]*tests.CreateTesseractParams{
		0: {
			CommitInfo: &common.CommitInfo{
				Operator: tests.RandomKramaID(t, 0),
			},
		},
	})
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
		id               identifiers.Identifier
		tsHash           common.Hash
		withInteractions bool
		withCommitInfo   bool
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
			name:             "fetch tesseract without interactions and with commit info from cache",
			tsHash:           ts[0].Hash(),
			withInteractions: false,
			withCommitInfo:   true,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch tesseract without interactions and without commit info from db",
			tsHash:           ts[1].Hash(),
			withInteractions: false,
			expectedTS:       ts[1],
		},
		{
			name:             "fetch tesseract with interactions and without commit info from db",
			tsHash:           ts[2].Hash(),
			withInteractions: true,
			expectedTS:       ts[2],
		},
		{
			name:             "fetch tesseract with interactions and with commit info from db",
			tsHash:           ts[2].Hash(),
			withInteractions: true,
			expectedTS:       ts[2],
		},
		{
			name:             "fetch tesseract without interactions and with commit info from cache",
			tsHash:           ts[2].Hash(),
			withInteractions: false,
			withCommitInfo:   true,
			expectedTS:       ts[2],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualTS, err := c.GetTesseract(test.tsHash, test.withInteractions, test.withCommitInfo)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			tests.ValidateTesseract(t, test.expectedTS, actualTS, test.withInteractions, test.withCommitInfo)
			checkIfTesseractCachedInCM(t, c, test.withInteractions, test.tsHash)
		})
	}
}

func TestGetTesseractByHeight(t *testing.T) {
	type args struct {
		id               identifiers.Identifier
		height           uint64
		withInteractions bool
		withCommitInfo   bool
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
			name: "fetch tesseract with Interactions and with commit info for a valid height",
			args: args{
				id:               ts.AnyAccountID(),
				height:           ts.Height(ts.AnyAccountID()),
				withInteractions: true,
				withCommitInfo:   true,
			},
			expectedTS: ts,
		},
		{
			name: "fetch tesseract without Interactions and without commit info for a valid height",
			args: args{
				id:               ts.AnyAccountID(),
				height:           ts.Height(ts.AnyAccountID()),
				withInteractions: false,
			},
			expectedTS: ts,
		},
		{
			name: "should return error for invalid height",
			args: args{
				id:               ts.AnyAccountID(),
				height:           1,
				withInteractions: true,
			},
			expectedError: errors.New("failed to fetch tesseract height entry"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualTS, err := c.GetTesseractByHeight(
				test.args.id,
				test.args.height,
				test.args.withInteractions,
				test.args.withCommitInfo)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.ValidateTesseract(t, test.expectedTS, actualTS, test.args.withInteractions, test.args.withCommitInfo)
			checkIfTesseractCachedInCM(t, c, test.args.withInteractions, ts.Hash())
		})
	}
}

func TestGetTesseractHashByHeight(t *testing.T) {
	id := tests.RandomIdentifier(t)
	height := uint64(33)
	hash := tests.RandomHash(t)

	chainParams := &CreateChainParams{
		dbCallback: func(db *MockDB) {
			err := db.SetTesseractHeightEntry(id, height, hash)
			require.NoError(t, err)
		},
	}

	c := createTestChainManager(t, chainParams)

	testcases := []struct {
		name          string
		id            identifiers.Identifier
		height        uint64
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name:         "successfully fetched tesseract hash",
			id:           id,
			height:       height,
			expectedHash: hash,
		},
		{
			name:          "failed to fetch tesseract hash",
			id:            tests.RandomIdentifier(t),
			height:        height,
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := c.GetTesseractHeightEntry(test.id, test.height)

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
	ids := tests.GetIdentifiers(t, 3)
	objects := make(map[identifiers.Identifier]*state.Object)
	keys := getHexEntries(t, 2)
	values := getHexEntries(t, 2)

	tsParams := &tests.CreateTesseractParams{
		IDs: ids,
		Participants: common.ParticipantsState{
			ids[0]: {
				Height:    11,
				StateHash: tests.RandomHash(t),
			},
			ids[1]: {
				Height:    23,
				StateHash: tests.RandomHash(t),
			},
			ids[2]: {
				Height: 28,
			},
		},
	}

	testcases := []struct {
		name            string
		id              identifiers.Identifier
		allParticipants bool
		dbCallback      func(db *MockDB)
		expectedError   error
	}{
		{
			name:            "id is not specified when a specific participant need to be added",
			id:              identifiers.Nil,
			allParticipants: false,
			expectedError:   errors.New("id is not specified"),
		},
		{
			name:            "added only one participant data successfully",
			id:              ids[0],
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
				obj := state.NewStateObject(ids[i], mockCache(), nil, db,
					common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false)

				for i, k := range keys {
					obj.SetDirtyEntry(k, []byte(values[i]))
				}

				objects[ids[i]] = obj
			}

			err := c.addParticipantsData(
				test.id,
				ts,
				state.NewTransition(objects, nil),
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
				s, ok := ts.State(test.id)
				if !ok {
					panic(ok)
				}

				participants[test.id] = s
			}

			for id, participant := range participants {
				if participant.StateHash == common.NilHash {
					_, found := db.GetAccMetaInfo(id)
					require.False(t, found)

					continue
				}

				_, ok := pool.removedObjects[id]
				require.True(t, ok)

				checkForParticipantByHeight(t, c, id, participant, true)

				// make sure transition object flushed to db
				for i, k := range keys {
					val, err := db.ReadEntry(common.FromHex(k))
					require.NoError(t, err)
					require.Equal(t, []byte(values[i]), val)
				}

				// check if acc meta info inserted
				accMetaInfo, found := db.GetAccMetaInfo(id)
				require.True(t, found)

				checkIfAccMetaInfoMatches(
					t,
					accMetaInfo,
					id,
					participant.Height,
					ts.Hash(),
					common.RegularAccount,
				)
			}

			// make sure other accounts are not stored in this case
			if !test.allParticipants {
				for id, p := range ts.Participants() {
					if id != test.id {
						checkForParticipantByHeight(t, c, id, p, false)
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

func TestStoreCommitInfo(t *testing.T) {
	c := createTestChainManager(t, nil)

	testcases := []struct {
		name               string
		batchWriterSetHook func() error
		params             *tests.CreateTesseractParams
		expectedError      error
	}{
		{
			name: "commit info stored successfully",
			params: &tests.CreateTesseractParams{
				CommitInfo: &common.CommitInfo{
					Operator: tests.RandomKramaID(t, 1),
				},
			},
		},
		{
			name: "failed to store commit info",
			batchWriterSetHook: func() error {
				return errors.New("failed to write commit info")
			},
			params: &tests.CreateTesseractParams{
				CommitInfo: &common.CommitInfo{
					Operator: tests.RandomKramaID(t, 1),
				},
			},
			expectedError: errors.New("failed to write commit info"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts := tests.CreateTesseract(t, test.params)

			bw := newMockBatchWriter()
			bw.setHook = test.batchWriterSetHook

			err := c.storeCommitInfo(bw, ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if receipts are stored in batch writer
			val, found := bw.Get(storage.TesseractCommitInfoKey(ts.Hash()))
			require.True(t, found)

			info := new(common.CommitInfo)

			err = info.FromBytes(val)
			require.NoError(t, err)

			require.Equal(t, ts.CommitInfo(), info)
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
			name: "interactions stored successfully",
			params: &tests.CreateTesseractParams{
				Ixns: common.NewInteractionsWithLeaderCheck(false, ixns...),
			},
		},
		{
			name: "failed to store interactions",
			batchWriterSetHook: func() error {
				return errors.New("failed to write ixns")
			},
			params: &tests.CreateTesseractParams{
				Ixns: common.NewInteractionsWithLeaderCheck(false, ixns...),
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

			require.Equal(t, ts.Interactions().IxList(), actualIxns.IxList())

			// check if ixnHash ==> tsHash pair stored in batch writer
			for _, ixn := range ts.Interactions().IxList() {
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
				Ixns:     common.NewInteractionsWithLeaderCheck(false, ixns...),
				Receipts: receipts,
				CommitInfo: &common.CommitInfo{
					Operator: tests.RandomKramaID(t, 1),
				},
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

			actualTS := new(common.Tesseract)

			err = actualTS.FromBytes(val)
			require.NoError(t, err)

			tests.ValidateTesseract(t, ts, actualTS, false, false)

			// check if ixns are stored in batch writer
			_, found = bw.Get(storage.InteractionsKey(ts.Hash()))
			require.True(t, found)

			// check if receipts are stored in batch writer
			_, found = bw.Get(storage.ReceiptsKey(ts.Hash()))
			require.True(t, found)

			// check if commit info are stored in batch writer
			_, found = bw.Get(storage.TesseractCommitInfoKey(ts.Hash()))
			require.True(t, found)
		})
	}
}

func TestAddTesseract(t *testing.T) {
	ixns, receipts := getIxAndReceipts(t, 1)
	ids := tests.GetIdentifiers(t, 2)
	objects := make(map[identifiers.Identifier]*state.Object)
	keys := getHexEntries(t, 2)
	values := getHexEntries(t, 2)

	tsParams := &tests.CreateTesseractParams{
		IDs: ids,
		Participants: common.ParticipantsState{
			ids[0]: {
				Height:       11,
				StateHash:    tests.RandomHash(t),
				ContextDelta: getDeltaGroup(t, 1, 1),
			},
			ids[1]: {
				Height:       23,
				StateHash:    tests.RandomHash(t),
				ContextDelta: getDeltaGroup(t, 1, 1),
			},
		},
		Ixns:     common.NewInteractionsWithLeaderCheck(false, ixns...),
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
				obj := state.NewStateObject(ids[i], mockCache(), nil, db,
					common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false)

				for i, k := range keys {
					obj.SetDirtyEntry(k, []byte(values[i]))
				}

				objects[ids[i]] = obj
			}

			err := c.AddTesseract(
				test.shouldCache,
				identifiers.Nil,
				ts,
				state.NewTransition(objects, nil),
				true,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// check if participant data added
			for id, participant := range participants {
				checkForParticipantByHeight(t, c, id, participant, true)
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
	ids := tests.GetIdentifiers(t, 2)
	icsHash := tests.RandomHash(t)

	objects := make(map[identifiers.Identifier]*state.Object)
	keys := getHexEntries(t, 2)
	values := getHexEntries(t, 2)

	dirtyStorage := map[common.Hash][]byte{
		icsHash:             {1, 2, 3},
		tests.RandomHash(t): {4, 5, 6},
	}

	commitInfo := &common.CommitInfo{
		Operator: tests.RandomKramaID(t, 1),
	}

	tsParams := &tests.CreateTesseractParams{
		IDs: ids,
		Participants: common.ParticipantsState{
			ids[0]: {
				Height:       11,
				StateHash:    tests.RandomHash(t),
				ContextDelta: getDeltaGroup(t, 1, 1),
			},
			ids[1]: {
				Height:       23,
				StateHash:    tests.RandomHash(t),
				ContextDelta: getDeltaGroup(t, 1, 1),
			},
		},
		TSDataCallback: func(ts *tests.TesseractData) {
			// ts.ConsensusInfo.ICSHash = icsHash
		},
		CommitInfo: commitInfo,
	}

	testcases := []struct {
		name            string
		id              identifiers.Identifier
		tsParams        *tests.CreateTesseractParams
		dirtyStorage    map[common.Hash][]byte
		allParticipants bool
		dbCallback      func(db *MockDB)
		expectedError   error
	}{
		{
			name:            "added tesseract with state successfully",
			id:              identifiers.Nil,
			tsParams:        tsParams,
			dirtyStorage:    dirtyStorage,
			allParticipants: true,
		},
		{
			name:            "added tesseract's participant successfully",
			id:              ids[0],
			tsParams:        tsParams,
			dirtyStorage:    dirtyStorage,
			allParticipants: false,
		},
		{
			name:     "failed to add tesseract",
			tsParams: tsParams,
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
			name:            "commit info not found",
			allParticipants: true,
			expectedError:   common.ErrCommitInfoNotFound,
		},
		{
			name:     "failed to write dirty keys",
			tsParams: tsParams,
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
			ts := tests.CreateTesseract(t, test.tsParams)

			db := mockDB()

			chainParams := &CreateChainParams{
				db:         db,
				dbCallback: test.dbCallback,
			}

			c := createTestChainManager(t, chainParams)

			participants := ts.Participants()

			for i := 0; i < 2; i++ {
				obj := state.NewStateObject(ids[i], mockCache(), nil, db,
					common.Account{AccType: common.RegularAccount}, state.NilMetrics(), false)

				for i, k := range keys {
					obj.SetDirtyEntry(k, []byte(values[i]))
				}

				objects[ids[i]] = obj
			}

			err := c.AddTesseractWithState(
				test.id,
				test.dirtyStorage,
				ts,
				state.NewTransition(objects, nil),
				test.allParticipants,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			var expectedAddedAccounts []identifiers.Identifier

			if test.allParticipants {
				expectedAddedAccounts = ts.AccountIDs()
			} else {
				expectedAddedAccounts = []identifiers.Identifier{test.id}
			}

			// check if participants data added
			for _, id := range expectedAddedAccounts {
				checkForParticipantByHeight(t, c, id, participants[id], true)
			}

			// make sure other accounts are not stored in this case
			if !test.allParticipants {
				for _, expectedAddr := range expectedAddedAccounts {
					for id, p := range participants {
						if id != expectedAddr {
							checkForParticipantByHeight(t, c, id, p, false)
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
		paramsMap = tests.GetIxParamsMapWithIDs(
			t,
			[]identifiers.Identifier{tests.RandomIdentifier(t)},
			[]identifiers.Identifier{tests.RandomIdentifier(t)},
		)
		ixns = tests.CreateIxns(t, 2, paramsMap)
		c    = createTestChainManager(t, nil)
	)

	rawIxns, err := polo.Polorize(ixns)
	require.NoError(t, err)

	err = c.db.SetInteractions(tsHash, rawIxns)
	require.NoError(t, err)

	testcases := []struct {
		name                 string
		tsHash               common.Hash
		expectedInteractions []*common.Interaction
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
			expectedInteraction:  ts[0].Interactions().IxList()[1],
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

	err = c.db.SetIXLookup(ts[1].Interactions().IxList()[1].Hash(), ts[1].Hash())
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
			ixHash:               ts[1].Interactions().IxList()[1].Hash(),
			expectedParticipants: ts[1].Participants(),
			expectedInteraction:  ts[1].Interactions().IxList()[1],
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

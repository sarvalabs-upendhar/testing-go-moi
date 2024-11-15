package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestUpdateAccMetaInfo_CheckErrors(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	address := tests.RandomAddress(t)
	testcases := []struct {
		name          string
		accMetaInfo   *common.AccountMetaInfo
		args          *common.AccountMetaInfo
		expectedError error
	}{
		{
			name:        "nil address",
			accMetaInfo: nil,
			args: &common.AccountMetaInfo{
				Address:       identifiers.NilAddress,
				Type:          common.AccountType(1),
				Height:        7,
				TesseractHash: tests.RandomHash(t),
			},
			expectedError: common.ErrInvalidAddress,
		},
		{
			name:        "nil hash",
			accMetaInfo: nil,
			args: &common.AccountMetaInfo{
				Address:       tests.RandomAddress(t),
				Type:          common.AccountType(1),
				Height:        8,
				TesseractHash: common.NilHash,
			},
			expectedError: common.ErrEmptyHash,
		},
		{
			name: "hash mismatch",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       address,
				Type:          common.AccountType(1),
				Height:        8,
				TesseractHash: tests.RandomHash(t),
			},
			args: &common.AccountMetaInfo{
				Address:       address,
				Type:          common.AccountType(1),
				Height:        8,
				TesseractHash: tests.RandomHash(t),
			},
			expectedError: common.ErrHashMismatch,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.accMetaInfo != nil {
				insertAccMetaInfo(t, pm, *test.accMetaInfo)
			}

			_, _, err := pm.UpdateAccMetaInfo(
				test.args.Address,
				test.args.Height,
				test.args.TesseractHash,
				test.args.StateHash,
				test.args.ContextHash,
				test.args.CommitHash,
				test.args.Type,
				true,
				test.args.PositionInContextSet,
			)
			require.Error(t, err)
			require.Equal(t, test.expectedError, err)
		})
	}
}

func TestUpdateAccMetaInfo_AddNewAccount(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	args := tests.GetRandomAccMetaInfo(t, 1)

	bucketID, isInsert, err := pm.UpdateAccMetaInfo(
		args.Address,
		args.Height,
		args.TesseractHash,
		args.StateHash,
		args.ContextHash,
		args.CommitHash,
		args.Type,
		true,
		args.PositionInContextSet,
	)

	require.NoError(t, err)

	// check if inserted
	require.True(t, isInsert)

	// check BucketID
	_, bucket := BucketKeyAndID(args.Address)
	require.Equal(t, int32(bucket), bucketID)

	afterAccMetaInfo, err := pm.GetAccountMetaInfo(args.Address)
	require.NoError(t, err)

	// check account state
	require.Equal(t, args, afterAccMetaInfo)
}

func TestUpdateAccMetaInfo_CheckHeight(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	addresses := tests.GetAddresses(t, 3)
	height := uint64(30)
	hash := tests.RandomHash(t)

	testcases := []struct {
		name                        string
		accMetaInfo                 *common.AccountMetaInfo
		args                        *common.AccountMetaInfo
		shouldUpdateContextPosition bool
	}{
		{
			name: "should update with new height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:              addresses[0],
				Type:                 common.AccountType(1),
				Height:               height,
				TesseractHash:        tests.RandomHash(t),
				StateHash:            tests.RandomHash(t),
				ContextHash:          tests.RandomHash(t),
				CommitHash:           tests.RandomHash(t),
				PositionInContextSet: 1,
			},
			shouldUpdateContextPosition: true,
			args: &common.AccountMetaInfo{
				Address:              addresses[0],
				Type:                 common.AccountType(1),
				Height:               height + 1,
				TesseractHash:        tests.RandomHash(t),
				StateHash:            tests.RandomHash(t),
				ContextHash:          tests.RandomHash(t),
				CommitHash:           tests.RandomHash(t),
				PositionInContextSet: 2,
			},
		},
		{
			name: "should update with equal height ",
			accMetaInfo: &common.AccountMetaInfo{
				Address:              addresses[1],
				Type:                 common.AccountType(3),
				Height:               height,
				TesseractHash:        hash,
				StateHash:            tests.RandomHash(t),
				ContextHash:          tests.RandomHash(t),
				CommitHash:           tests.RandomHash(t),
				PositionInContextSet: 3,
			},
			shouldUpdateContextPosition: true,
			args: &common.AccountMetaInfo{
				Address:              addresses[1],
				Type:                 common.AccountType(3),
				Height:               height,
				TesseractHash:        hash,
				StateHash:            tests.RandomHash(t),
				ContextHash:          tests.RandomHash(t),
				CommitHash:           tests.RandomHash(t),
				PositionInContextSet: 4,
			},
		},
		{
			name: "shouldn't update with low height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:              addresses[2],
				Type:                 common.AccountType(1),
				Height:               height,
				TesseractHash:        tests.RandomHash(t),
				StateHash:            tests.RandomHash(t),
				ContextHash:          tests.RandomHash(t),
				CommitHash:           tests.RandomHash(t),
				PositionInContextSet: 5,
			},
			shouldUpdateContextPosition: false,
			args: &common.AccountMetaInfo{
				Address:              addresses[2],
				Type:                 common.AccountType(3),
				Height:               height - 1,
				TesseractHash:        tests.RandomHash(t),
				StateHash:            tests.RandomHash(t),
				ContextHash:          tests.RandomHash(t),
				CommitHash:           tests.RandomHash(t),
				PositionInContextSet: 6,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// insert test accMetaInfo , so that it can be updated
			insertAccMetaInfo(t, pm, *test.accMetaInfo)

			beforeAccMetaInfo, err := pm.GetAccountMetaInfo(test.args.Address)
			require.NoError(t, err)

			_, isInsert, err := pm.UpdateAccMetaInfo(
				test.args.Address,
				test.args.Height,
				test.args.TesseractHash,
				test.args.StateHash,
				test.args.ContextHash,
				test.args.CommitHash,
				test.args.Type,
				test.shouldUpdateContextPosition,
				test.args.PositionInContextSet,
			)
			require.NoError(t, err)

			// check if updated
			require.False(t, isInsert)

			afterAccMetaInfo, err := pm.GetAccountMetaInfo(test.args.Address)
			require.NoError(t, err)

			// changes should take place if new height is greater than equal to current height
			if test.args.Height >= beforeAccMetaInfo.Height {
				require.Equal(t, test.args.TesseractHash, afterAccMetaInfo.TesseractHash)
				require.Equal(t, test.args.StateHash, afterAccMetaInfo.StateHash)
				require.Equal(t, test.args.ContextHash, afterAccMetaInfo.ContextHash)
				require.Equal(t, test.args.Address, afterAccMetaInfo.Address)
				require.Equal(t, test.args.Height, afterAccMetaInfo.Height)
				require.Equal(t, test.args.Type, afterAccMetaInfo.Type)
				require.Equal(t, test.args.PositionInContextSet, afterAccMetaInfo.PositionInContextSet)
				require.Equal(t, test.args.CommitHash, afterAccMetaInfo.CommitHash)
			} else { // changes shouldn't take place if new height less than current height
				require.Equal(t, beforeAccMetaInfo.TesseractHash, afterAccMetaInfo.TesseractHash)
				require.Equal(t, beforeAccMetaInfo.StateHash, afterAccMetaInfo.StateHash)
				require.Equal(t, beforeAccMetaInfo.ContextHash, afterAccMetaInfo.ContextHash)
				require.Equal(t, beforeAccMetaInfo.Address, afterAccMetaInfo.Address)
				require.Equal(t, beforeAccMetaInfo.Height, afterAccMetaInfo.Height)
				require.Equal(t, beforeAccMetaInfo.Type, afterAccMetaInfo.Type)
				require.Equal(t, beforeAccMetaInfo.PositionInContextSet, afterAccMetaInfo.PositionInContextSet)
				require.Equal(t, beforeAccMetaInfo.CommitHash, afterAccMetaInfo.CommitHash)
			}
		})
	}
}

func TestUpdateAccMetaInfo_CheckBucketID(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	address := tests.RandomAddress(t)

	accMetaInfo := common.AccountMetaInfo{
		Address:       address,
		Type:          common.AccountType(1),
		Height:        1,
		TesseractHash: tests.RandomHash(t),
	}
	args := &common.AccountMetaInfo{
		Address:       address,
		Type:          common.AccountType(1),
		Height:        3,
		TesseractHash: tests.RandomHash(t),
	}

	// insert test accMetaInfo , so that it can be updated
	insertAccMetaInfo(t, pm, accMetaInfo)

	bucketID, _, err := pm.UpdateAccMetaInfo(
		args.Address,
		args.Height,
		args.TesseractHash,
		args.StateHash,
		args.ContextHash,
		args.CommitHash,
		args.Type,
		true,
		args.PositionInContextSet,
	)
	require.NoError(t, err)

	// check if BucketID matches
	_, bucket := BucketKeyAndID(args.Address)
	require.Equal(t, int32(bucket), bucketID)
}

func TestGetAccountMetaInfo(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// test data
	AccMetaInfo := tests.GetRandomAccMetaInfo(t, 1)

	// insert test data in to db
	insertAccMetaInfo(t, pm, *AccMetaInfo)

	testcases := []struct {
		name                string
		address             identifiers.Address
		expectedAccMetaInfo *common.AccountMetaInfo
		expectedError       error
	}{
		{
			name:                "account doesn't exist",
			address:             tests.RandomAddress(t),
			expectedAccMetaInfo: &common.AccountMetaInfo{},
			expectedError:       common.ErrAccountNotFound,
		},
		{
			name:                "account exists",
			address:             AccMetaInfo.Address,
			expectedAccMetaInfo: AccMetaInfo,
			expectedError:       nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			accMetaInfo, err := pm.GetAccountMetaInfo(test.address)

			if test.expectedError != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, AccMetaInfo, accMetaInfo)
			}
		})
	}
}

func TestHasAccMetaInfoAt(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// test data
	AccMetaInfo := tests.GetRandomAccMetaInfo(t, 6)

	// insert test data in to db
	insertAccMetaInfo(t, pm, *AccMetaInfo)

	testcases := []struct {
		name             string
		address          identifiers.Address
		height           uint64
		hasAccMetaInfoAt bool
	}{
		{
			name:    "account meta info doesn't exist",
			address: tests.RandomAddress(t),
		},
		{
			name:    "account meta info doesn't exist at given height",
			address: AccMetaInfo.Address,
			height:  7,
		},
		{
			name:             "account meta info exists at given equal height",
			address:          AccMetaInfo.Address,
			height:           6,
			hasAccMetaInfoAt: true,
		},
		{
			name:             "account meta info exists at given lesser height",
			address:          AccMetaInfo.Address,
			height:           5,
			hasAccMetaInfoAt: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hasAccMetaInfoAt := pm.HasAccMetaInfoAt(test.address, test.height)

			require.Equal(t, test.hasAccMetaInfoAt, hasAccMetaInfoAt)
		})
	}
}

func TestIncrementBucketCount(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	type args struct {
		address identifiers.Address
		count   uint64
	}

	address := tests.RandomAddress(t)

	testcases := []struct {
		name          string
		arg           args
		expectedCount uint64
	}{
		{
			name: "account doesn't exist",
			arg: args{
				address: address,
				count:   1,
			},
			expectedCount: 1,
		},
		{
			name: "account exists",
			arg: args{
				address: address,
				count:   1,
			},
			expectedCount: 2,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, bucket := BucketKeyAndID(test.arg.address)

			err := pm.incrementBucketCount(bucket, test.arg.count)
			require.NoError(t, err)

			actualCount, err := pm.GetBucketCount(bucket)
			require.NoError(t, err)

			require.Equal(t, test.expectedCount, actualCount)
		})
	}
}

// here we increment bucket count for 10000 addresses and check if number of addresses in each bucket are as expected
func TestGetBucketSizes(t *testing.T) {
	pm := NewTestPersistenceManager(t)
	incrementBucketCounts := incrementBuckets(t, pm)

	actualBucketSizes, err := pm.GetBucketSizes()

	require.NoError(t, err)

	for k, v := range actualBucketSizes {
		require.Equal(t, incrementBucketCounts[k], v)
	}
}

// here we insert 10000 random accounts and check if inserted accounts and fetched accounts match
func TestGetAccounts(t *testing.T) {
	t.Parallel()

	pm := NewTestPersistenceManager(t)

	insertedAccounts := insertTestAccMetaInfo(t, pm)

	// check if all accounts under every bucket number are stored properly
	for i := uint64(0); i < 1024; i++ {
		resp := make(chan []byte)
		responses := make([][]byte, 0)

		go func() {
			err := pm.StreamAccountMetaInfosRaw(context.Background(), i, resp)
			require.NoError(t, err)
		}()

		for rawData := range resp {
			responses = append(responses, rawData)
		}

		require.Equal(t, len(insertedAccounts[i]), len(responses), "inserted account count doesn't match")
	}
}

func TestPersistenceManager_GetTesseract(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxnsAndReceipts(t, 2, 2)

	// Set the clusterID to genesis identifier to avoid fetching interactions
	tesseractParams[0].TSDataCallback = func(ts *tests.TesseractData) {
		ts.ConsensusInfo.View = common.GenesisView
	}

	tesseracts := tests.CreateTesseracts(t, 3, tesseractParams)

	pm := NewTestPersistenceManager(t)

	insertTesseracts(t, pm, tesseracts...)
	insertIxns(t, pm, tesseracts[:2]...)
	insertReceiptsInDB(t, pm, tesseracts[:2]...)
	insertCommitInfosInDB(t, pm, tesseracts[:2]...)

	testcases := []struct {
		name             string
		hash             common.Hash
		withInteractions bool
		withCommitInfo   bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:       "genesis tesseract",
			hash:       tesseracts[0].Hash(),
			expectedTS: tesseracts[0],
		},
		{
			name:             "non-genesis tesseract with interactions with commit info",
			hash:             tesseracts[1].Hash(),
			withInteractions: true,
			withCommitInfo:   true,
			expectedTS:       tesseracts[1],
		},
		{
			name:             "without interactions and with commit info",
			hash:             tesseracts[1].Hash(),
			withInteractions: false,
			withCommitInfo:   true,
			expectedTS:       tesseracts[1],
		},
		{
			name:             "with interactions and without commit info",
			hash:             tesseracts[1].Hash(),
			withInteractions: true,
			withCommitInfo:   false,
			expectedTS:       tesseracts[1],
		},
		{
			name:             "should fail if tesseract not found",
			hash:             tests.RandomHash(t),
			withInteractions: false,
			expectedError:    common.ErrKeyNotFound,
		},
		{
			name:             "should fail if interactions not found",
			hash:             tesseracts[2].Hash(),
			withInteractions: true,
			expectedError:    common.ErrFetchingInteractions,
		},
		{
			name:           "should fail if commit info not found",
			hash:           tesseracts[2].Hash(),
			withCommitInfo: true,
			expectedError:  common.ErrCommitInfoNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ts, err := pm.GetTesseract(test.hash, test.withInteractions, test.withCommitInfo)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			validateTesseract(t, test.expectedTS, ts, test.withInteractions, test.withCommitInfo)
		})
	}
}

// here we insert 10000 entries and check if inserted entries and fetched entries match
func TestGetEntriesWithPrefix(t *testing.T) {
	t.Parallel()

	pm := NewTestPersistenceManager(t)
	insertedEntries, prefixes := insertTestEntries(t, pm)

	actualEntryCount := 0

	for _, prefix := range prefixes {
		actualEntries, err := pm.GetEntriesWithPrefix(context.Background(), []byte(prefix))
		require.NoError(t, err)

		for actualEntry := range actualEntries {
			actualEntryCount++

			actualVal := string(actualEntry.Value)

			insertedVal, ok := insertedEntries[string(actualEntry.Key)]
			require.True(t, ok)

			require.Equal(t, insertedVal, actualVal)
		}
	}

	require.Equal(t, len(insertedEntries), actualEntryCount)
}

func TestWritePreImages(t *testing.T) {
	pm := NewTestPersistenceManager(t)
	// create random entries
	address := tests.RandomAddress(t)
	testEntries := getRandomPreImageEntries(t, 10)
	// write preimages
	err := pm.WritePreImages(address, testEntries)
	require.NoError(t, err)

	for k, v := range testEntries {
		dbValue, err := pm.ReadEntry(PreImageKey(address, k))
		require.NoError(t, err)
		require.Equal(t, v, dbValue)
	}
}

func TestSetReceipts(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// create random receipts
	tsHash := tests.RandomHash(t)
	receipts := getRandomReceipts(t, tsHash, 2)

	testcases := []struct {
		name     string
		receipts common.Receipts
		tsHash   common.Hash
	}{
		{
			name:     "Create an entry in db for the given receipts",
			receipts: receipts,
			tsHash:   tsHash,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			receipts, err := testcase.receipts.Bytes()
			require.NoError(t, err)

			err = pm.SetReceipts(testcase.tsHash, receipts)
			require.NoError(t, err)

			rawData, err := pm.GetReceipts(testcase.tsHash)
			require.NoError(t, err)

			require.Equal(t, receipts, rawData)
		})
	}
}

func TestGetReceipts(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// create random receipts
	tsHash := tests.RandomHash(t)
	receipts := getRandomReceipts(t, tsHash, 2)

	insertReceipts(t, pm, tsHash, receipts)

	testcases := []struct {
		name          string
		receipts      common.Receipts
		tsHash        common.Hash
		expectedError error
	}{
		{
			name:          "failed to fetch receipts",
			tsHash:        tests.RandomHash(t),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:     "fetched receipts successfully",
			receipts: receipts,
			tsHash:   tsHash,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			receipts, err := pm.GetReceipts(testcase.tsHash)

			if testcase.expectedError != nil {
				require.ErrorContains(t, err, testcase.expectedError.Error())

				return
			}

			require.NoError(t, err)

			expectedReceipts, err := testcase.receipts.Bytes()
			require.NoError(t, err)

			require.Equal(t, expectedReceipts, receipts)
		})
	}
}

func keyWithPrefix(prefix identifiers.Address, k int) []byte {
	return append(prefix.Bytes(), []byte(fmt.Sprintf("%d", k))...)
}

func value(k int) []byte {
	return []byte(fmt.Sprintf("%08d", k))
}

func TestPersistenceManager_GetAccountSnapshot(t *testing.T) {
	dir1, err := os.MkdirTemp(os.TempDir(), "test1")
	require.NoError(t, err)

	dir2, err := os.MkdirTemp(os.TempDir(), "test2")
	require.NoError(t, err)

	defer func() {
		os.RemoveAll(dir1)
		os.RemoveAll(dir2)
	}()

	pm1 := NewTestPersistenceManagerWithBadger(t, dir1)
	pm2 := NewTestPersistenceManagerWithBadger(t, dir2)

	address := tests.GetRandomAddressList(t, 3)
	count := 10000

	for _, prefix := range address {
		bw := pm1.db.NewBatchWriter()
		for i := 1; i <= count; i++ {
			require.NoError(t, bw.Set(keyWithPrefix(prefix, i), value(i)))
		}

		require.NoError(t, bw.Flush())
	}

	for _, prefix := range address {
		var (
			exit              = make(chan bool)
			receivedData      = make([]byte, 0)
			resp              = make(chan common.SnapResponse)
			expectedSize      = uint64(0)
			totalReceivedSize = uint64(0)
		)

		go func() {
			for {
				select {
				case r := <-resp:
					if r.ChunkSize != 0 {
						receivedData = make([]byte, 0, r.ChunkSize)
						expectedSize = r.ChunkSize

						continue
					}

					if r.End {
						require.True(t, expectedSize == uint64(len(receivedData)))
						totalReceivedSize += expectedSize

						err = pm2.StoreAccountSnapShot(&common.Snapshot{
							Entries: receivedData,
						})
						require.NoError(t, err)

						continue
					}

					receivedData = append(receivedData, r.Data...)

				case <-exit:
					return
				}
			}
		}()

		sentSnapSize, err := pm1.StreamSnapshot(context.Background(), prefix, 0, resp)
		require.NoError(t, err)

		exit <- true

		require.True(t, sentSnapSize == totalReceivedSize)
	}

	for _, prefix := range address {
		for i := 1; i <= count; i++ {
			val, err := pm2.ReadEntry(keyWithPrefix(prefix, i))
			require.NoError(t, err)

			bytes.Equal(value(i), val)
		}

		require.NoError(t, err)
	}
}

func TestSetInteractions(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	testcases := []struct {
		name   string
		ixData []byte
		tsHash common.Hash
	}{
		{
			name:   "Create an entry in db for the given ixns",
			ixData: []byte{1, 2, 3},
			tsHash: tests.RandomHash(t),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := pm.SetInteractions(testcase.tsHash, testcase.ixData)
			require.NoError(t, err)

			rawData, err := pm.GetInteractions(testcase.tsHash)
			require.NoError(t, err)

			require.Equal(t, testcase.ixData, rawData)
		})
	}
}

func TestGetInteractions(t *testing.T) {
	pm := NewTestPersistenceManager(t)
	tsHash := tests.RandomHash(t)
	ixData := []byte{1, 2, 3}

	err := pm.SetInteractions(tsHash, ixData)
	require.NoError(t, err)

	testcases := []struct {
		name          string
		ixData        []byte
		tsHash        common.Hash
		expectedError error
	}{
		{
			name:          "failed to fetch interactions",
			tsHash:        tests.RandomHash(t),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:   "fetched interactions successfully",
			ixData: ixData,
			tsHash: tsHash,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixData, err := pm.GetInteractions(testcase.tsHash)

			if testcase.expectedError != nil {
				require.ErrorContains(t, err, testcase.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.ixData, ixData)
		})
	}
}

package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

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
				Address:       common.NilAddress,
				Type:          common.AccountType(1),
				Height:        7,
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
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
				LatticeExists: true,
				StateExists:   true,
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
				LatticeExists: true,
				StateExists:   true,
			},
			args: &common.AccountMetaInfo{
				Address:       address,
				Type:          common.AccountType(1),
				Height:        8,
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
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
				test.args.Type,
				test.args.LatticeExists,
				test.args.StateExists,
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
		args.Type,
		args.LatticeExists,
		args.StateExists,
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

	addresses := getAddresses(t, 3)
	height := uint64(30)
	hash := tests.RandomHash(t)

	testcases := []struct {
		name          string
		accMetaInfo   *common.AccountMetaInfo
		args          *common.AccountMetaInfo
		expectedError error
	}{
		{
			name: "should update with new height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       addresses[0],
				Type:          common.AccountType(1),
				Height:        height,
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			args: &common.AccountMetaInfo{
				Address:       addresses[0],
				Type:          common.AccountType(1),
				Height:        height + 1,
				TesseractHash: tests.RandomHash(t),
				LatticeExists: false,
				StateExists:   false,
			},
			expectedError: nil,
		},
		{
			name: "should update with equal height ",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       addresses[1],
				Type:          common.AccountType(3),
				Height:        height,
				TesseractHash: hash,
				LatticeExists: true,
				StateExists:   true,
			},
			args: &common.AccountMetaInfo{
				Address:       addresses[1],
				Type:          common.AccountType(3),
				Height:        height,
				TesseractHash: hash,
				LatticeExists: false,
				StateExists:   true,
			},
			expectedError: nil,
		},
		{
			name: "shouldn't update with low height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       addresses[2],
				Type:          common.AccountType(1),
				Height:        height,
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			args: &common.AccountMetaInfo{
				Address:       addresses[2],
				Type:          common.AccountType(3),
				Height:        height - 1,
				TesseractHash: tests.RandomHash(t),
				LatticeExists: false,
				StateExists:   true,
			},
			expectedError: nil,
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
				test.args.Type,
				test.args.LatticeExists,
				test.args.StateExists,
			)
			require.NoError(t, err)

			// check if updated
			require.False(t, isInsert)

			afterAccMetaInfo, err := pm.GetAccountMetaInfo(test.args.Address)
			require.NoError(t, err)

			// changes should take place if new height is greater than equal to current height
			if test.args.Height >= beforeAccMetaInfo.Height {
				require.Equal(t, test.args.StateExists, afterAccMetaInfo.StateExists)
				require.Equal(t, test.args.TesseractHash, afterAccMetaInfo.TesseractHash)
				require.Equal(t, test.args.Address, afterAccMetaInfo.Address)
				require.Equal(t, test.args.Height, afterAccMetaInfo.Height)
				require.Equal(t, beforeAccMetaInfo.Type, afterAccMetaInfo.Type)
			} else { // changes shouldn't take place if new height less than current height
				require.Equal(t, beforeAccMetaInfo.StateExists, afterAccMetaInfo.StateExists)
				require.Equal(t, beforeAccMetaInfo.TesseractHash, afterAccMetaInfo.TesseractHash)
				require.Equal(t, beforeAccMetaInfo.Address, afterAccMetaInfo.Address)
				require.Equal(t, beforeAccMetaInfo.Height, afterAccMetaInfo.Height)
				require.Equal(t, beforeAccMetaInfo.Type, afterAccMetaInfo.Type)
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
		LatticeExists: true,
		StateExists:   true,
	}
	args := &common.AccountMetaInfo{
		Address:       address,
		Type:          common.AccountType(1),
		Height:        3,
		TesseractHash: tests.RandomHash(t),
		LatticeExists: false,
		StateExists:   false,
	}

	// insert test accMetaInfo , so that it can be updated
	insertAccMetaInfo(t, pm, accMetaInfo)

	bucketID, _, err := pm.UpdateAccMetaInfo(
		args.Address,
		args.Height,
		args.TesseractHash,
		args.Type,
		args.LatticeExists,
		args.StateExists,
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
		address             common.Address
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

func TestIncrementBucketCount(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	type args struct {
		address common.Address
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

func TestUpdateTesseractStatus_CheckErrors(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	type args struct {
		address common.Address
		height  uint64
		hash    common.Hash
		status  bool
	}

	AccMetaInfo := tests.GetRandomAccMetaInfo(t, 30)
	insertAccMetaInfo(t, pm, *AccMetaInfo)

	testcases := []struct {
		name          string
		arg           args
		expectedError error
	}{
		{
			name: "account doesn't exist",
			arg: args{
				address: tests.RandomAddress(t),
				height:  1,
				hash:    AccMetaInfo.TesseractHash,
				status:  false,
			},
			expectedError: common.ErrKeyNotFound,
		},
		{
			name: "should return error if hash mismatch",
			arg: args{
				address: AccMetaInfo.Address,
				height:  AccMetaInfo.Height,
				hash:    tests.RandomHash(t),
				status:  false,
			},
			expectedError: common.ErrHashMismatch,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := pm.UpdateTesseractStatus(
				test.arg.address,
				test.arg.height,
				test.arg.hash,
				test.arg.status,
			)
			require.Error(t, err)

			require.Equal(t, test.expectedError, err)
		})
	}
}

func TestUpdateTesseractStatus_CheckHeight(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	type args struct {
		address common.Address
		height  uint64
		hash    common.Hash
		status  bool
	}

	addresses := getAddresses(t, 3)
	hashes := getHashes(t, 3)
	height := uint64(30)
	testcases := []struct {
		name          string
		accMetaInfo   *common.AccountMetaInfo
		arg           args
		expectedError error
	}{
		{
			name: "shouldn't update with lower height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       addresses[0],
				Type:          common.AccountType(1),
				Height:        height,
				TesseractHash: hashes[0],
				LatticeExists: true,
				StateExists:   true,
			},
			arg: args{
				address: addresses[0],
				height:  height - 1,
				hash:    hashes[0],
				status:  false,
			},
		},
		{
			name: "should update with equal height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       addresses[1],
				Type:          common.AccountType(1),
				Height:        height,
				TesseractHash: hashes[1],
				LatticeExists: true,
				StateExists:   true,
			},
			arg: args{
				address: addresses[1],
				height:  height,
				hash:    hashes[1],
				status:  false,
			},
		},
		{
			name: "should update with new height",
			accMetaInfo: &common.AccountMetaInfo{
				Address:       addresses[2],
				Type:          common.AccountType(1),
				Height:        height,
				TesseractHash: hashes[2],
				LatticeExists: true,
				StateExists:   true,
			},
			arg: args{
				address: addresses[2],
				height:  height + 1,
				hash:    hashes[2],
				status:  false,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			insertAccMetaInfo(t, pm, *test.accMetaInfo)

			err := pm.UpdateTesseractStatus(
				test.arg.address,
				test.arg.height,
				test.arg.hash,
				test.arg.status,
			)
			require.NoError(t, err)

			actualAccMetaInfo, err := pm.GetAccountMetaInfo(test.arg.address)
			require.NoError(t, err)

			// changes should take place if new height is greater than equal to current height
			if test.arg.height >= actualAccMetaInfo.Height {
				require.Equal(t, test.arg.status, actualAccMetaInfo.StateExists)
			} else { // changes shouldn't take place if new height less than current height
				require.Equal(t, test.accMetaInfo.StateExists, actualAccMetaInfo.StateExists)
			}
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

// here we insert 10000 entries and check if inserted entries and fetched entries match
func TestGetEntriesWithPrefix(t *testing.T) {
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

func TestSetGridLookup(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	testcases := []struct {
		name        string
		tsHash      common.Hash
		gridHash    common.Hash
		expectedErr bool
	}{
		{
			name:        "Create an entry in db with key as tesseract hash and grid hash as value",
			tsHash:      tests.RandomHash(t),
			gridHash:    tests.RandomHash(t),
			expectedErr: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := pm.SetTSGridLookup(testcase.tsHash, testcase.gridHash)
			require.NoError(t, err)

			rawData, err := pm.ReadEntry(DBKey(common.NilAddress, TSGridLookup, testcase.tsHash.Bytes()))
			require.NoError(t, err)

			require.Equal(t, testcase.gridHash.Bytes(), rawData)
		})
	}
}

func TestGetGridLookup(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	tsHash := tests.RandomHash(t)
	gridHash := tests.RandomHash(t)
	insertTSGridLookup(t, pm, tsHash, gridHash)

	testcases := []struct {
		name        string
		tsHash      common.Hash
		expectedErr bool
	}{
		{
			name:        "valid hash without state",
			tsHash:      tests.RandomHash(t),
			expectedErr: true,
		},
		{
			name:        "valid hash with state",
			tsHash:      tsHash,
			expectedErr: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			gridLookup, err := pm.GetTSGridLookup(testcase.tsHash)

			if testcase.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, gridHash.Bytes(), gridLookup)
		})
	}
}

func TestSetReceipts(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// create random receipts
	receiptHash := tests.RandomHash(t)
	receipts := getRandomReceipts(t, receiptHash, 2)

	testcases := []struct {
		name        string
		receipts    common.Receipts
		receiptHash common.Hash
	}{
		{
			name:        "Create an entry in db for the given receipts",
			receipts:    receipts,
			receiptHash: receiptHash,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			receipts, err := testcase.receipts.Bytes()
			require.NoError(t, err)

			err = pm.SetReceipts(testcase.receiptHash, receipts)
			require.NoError(t, err)

			rawData, err := pm.GetReceipts(testcase.receiptHash)
			require.NoError(t, err)

			require.Equal(t, receipts, rawData)
		})
	}
}

func TestGetReceipts(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// create random receipts
	receiptHash := tests.RandomHash(t)
	receipts := getRandomReceipts(t, receiptHash, 2)

	insertReceipts(t, pm, receiptHash, receipts)

	testcases := []struct {
		name          string
		receipts      common.Receipts
		receiptHash   common.Hash
		expectedError error
	}{
		{
			name:          "failed to fetch receipts",
			receiptHash:   tests.RandomHash(t),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:        "fetched receipts successfully",
			receipts:    receipts,
			receiptHash: receiptHash,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			receipts, err := pm.GetReceipts(testcase.receiptHash)

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

func keyWithPrefix(prefix common.Address, k int) []byte {
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

	for _, prefix := range address {
		bw := pm1.db.NewBatchWriter()
		for i := 1; i <= 100; i++ {
			require.NoError(t, bw.Set(keyWithPrefix(prefix, i), value(i)))
		}
		require.NoError(t, bw.Flush())
	}

	for _, prefix := range address {
		snap, err := pm1.GetAccountSnapshot(context.Background(), prefix, 0)
		require.NoError(t, err)

		err = pm2.StoreAccountSnapShot(snap)
		require.NoError(t, err)
	}

	for _, prefix := range address {
		for i := 1; i <= 100; i++ {
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
		name     string
		ixData   []byte
		gridHash common.Hash
	}{
		{
			name:     "Create an entry in db for the given receipts",
			ixData:   []byte{1, 2, 3},
			gridHash: tests.RandomHash(t),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := pm.SetInteractions(testcase.gridHash, testcase.ixData)
			require.NoError(t, err)

			rawData, err := pm.GetInteractions(testcase.gridHash)
			require.NoError(t, err)

			require.Equal(t, testcase.ixData, rawData)
		})
	}
}

func TestGetInteractions(t *testing.T) {
	pm := NewTestPersistenceManager(t)
	gridHash := tests.RandomHash(t)
	ixData := []byte{1, 2, 3}

	err := pm.SetInteractions(gridHash, ixData)
	require.NoError(t, err)

	testcases := []struct {
		name          string
		ixData        []byte
		gridHash      common.Hash
		expectedError error
	}{
		{
			name:          "failed to fetch interactions",
			gridHash:      tests.RandomHash(t),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:     "fetched interactions successfully",
			ixData:   ixData,
			gridHash: gridHash,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixData, err := pm.GetInteractions(testcase.gridHash)

			if testcase.expectedError != nil {
				require.ErrorContains(t, err, testcase.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.ixData, ixData)
		})
	}
}

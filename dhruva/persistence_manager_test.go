package dhruva

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
)

func TestUpdateAccMetaInfo_CheckErrors(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	address := tests.RandomAddress(t)
	testcases := []struct {
		name          string
		accMetaInfo   *types.AccountMetaInfo
		args          *types.AccountMetaInfo
		expectedError error
	}{
		{
			name:        "nil address",
			accMetaInfo: nil,
			args: &types.AccountMetaInfo{
				Address:       types.NilAddress,
				Type:          types.AccountType(1),
				Height:        big.NewInt(7),
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:        "nil hash",
			accMetaInfo: nil,
			args: &types.AccountMetaInfo{
				Address:       tests.RandomAddress(t),
				Type:          types.AccountType(1),
				Height:        big.NewInt(8),
				TesseractHash: types.NilHash,
				LatticeExists: true,
				StateExists:   true,
			},
			expectedError: types.ErrEmptyHash,
		},
		{
			name: "hash mismatch",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       address,
				Type:          types.AccountType(1),
				Height:        big.NewInt(8),
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			args: &types.AccountMetaInfo{
				Address:       address,
				Type:          types.AccountType(1),
				Height:        big.NewInt(8),
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			expectedError: types.ErrHashMismatch,
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
	_, bucket := BucketIDFromAddress(args.Address.Bytes())
	require.Equal(t, int32(bucket.getID()), bucketID)

	afterAccMetaInfo, err := pm.GetAccountMetaInfo(args.Address.Bytes())
	require.NoError(t, err)

	// check account state
	require.Equal(t, args, afterAccMetaInfo)
}

func TestUpdateAccMetaInfo_CheckHeight(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	addresses := getAddresses(t, 3)
	height := int64(30)
	hash := tests.RandomHash(t)

	testcases := []struct {
		name          string
		accMetaInfo   *types.AccountMetaInfo
		args          *types.AccountMetaInfo
		expectedError error
	}{
		{
			name: "should update with new height",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       addresses[0],
				Type:          types.AccountType(1),
				Height:        big.NewInt(height),
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			args: &types.AccountMetaInfo{
				Address:       addresses[0],
				Type:          types.AccountType(1),
				Height:        big.NewInt(height + 1),
				TesseractHash: tests.RandomHash(t),
				LatticeExists: false,
				StateExists:   false,
			},
			expectedError: nil,
		},
		{
			name: "should update with equal height ",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       addresses[1],
				Type:          types.AccountType(3),
				Height:        big.NewInt(height),
				TesseractHash: hash,
				LatticeExists: true,
				StateExists:   true,
			},
			args: &types.AccountMetaInfo{
				Address:       addresses[1],
				Type:          types.AccountType(3),
				Height:        big.NewInt(height),
				TesseractHash: hash,
				LatticeExists: false,
				StateExists:   true,
			},
			expectedError: nil,
		},
		{
			name: "shouldn't update with low height",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       addresses[2],
				Type:          types.AccountType(1),
				Height:        big.NewInt(height),
				TesseractHash: tests.RandomHash(t),
				LatticeExists: true,
				StateExists:   true,
			},
			args: &types.AccountMetaInfo{
				Address:       addresses[2],
				Type:          types.AccountType(3),
				Height:        big.NewInt(height - 1),
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

			beforeAccMetaInfo, err := pm.GetAccountMetaInfo(test.args.Address.Bytes())
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

			afterAccMetaInfo, err := pm.GetAccountMetaInfo(test.args.Address.Bytes())
			require.NoError(t, err)

			// changes should take place if new height is greater than equal to current height
			if test.args.Height.Cmp(beforeAccMetaInfo.Height) >= 0 {
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

	accMetaInfo := types.AccountMetaInfo{
		Address:       address,
		Type:          types.AccountType(1),
		Height:        big.NewInt(1),
		TesseractHash: tests.RandomHash(t),
		LatticeExists: true,
		StateExists:   true,
	}
	args := &types.AccountMetaInfo{
		Address:       address,
		Type:          types.AccountType(1),
		Height:        big.NewInt(3),
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
	_, bucket := BucketIDFromAddress(args.Address.Bytes())
	require.Equal(t, int32(bucket.getID()), bucketID)
}

func TestGetAccountMetaInfo(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	// test data
	AccMetaInfo := tests.GetRandomAccMetaInfo(t, 1)

	// insert test data in to db
	insertAccMetaInfo(t, pm, *AccMetaInfo)

	testcases := []struct {
		name                string
		address             types.Address
		expectedAccMetaInfo *types.AccountMetaInfo
		expectedError       error
	}{
		{
			name:                "account doesn't exist",
			address:             tests.RandomAddress(t),
			expectedAccMetaInfo: &types.AccountMetaInfo{},
			expectedError:       types.ErrAccountNotFound,
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
			accMetaInfo, err := pm.GetAccountMetaInfo(test.address.Bytes())

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
		address types.Address
		count   int64
	}

	address := tests.RandomAddress(t)

	testcases := []struct {
		name          string
		arg           args
		expectedCount int64
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
			_, bucket := BucketIDFromAddress(test.arg.address.Bytes())

			err := pm.incrementBucketCount(bucket.getIDBytes(), test.arg.count)
			require.NoError(t, err)

			actualCount, err := pm.getBucketCountByBucketNumber(bucket.getIDBytes())
			require.NoError(t, err)

			require.Equal(t, test.expectedCount, actualCount.Int64())
		})
	}
}

func TestUpdateTesseractStatus_CheckErrors(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	type args struct {
		address types.Address
		height  uint64
		hash    types.Hash
		status  bool
	}

	AccMetaInfo := tests.GetRandomAccMetaInfo(t, int64(30))
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
			expectedError: types.ErrKeyNotFound,
		},
		{
			name: "should return error if hash mismatch",
			arg: args{
				address: AccMetaInfo.Address,
				height:  AccMetaInfo.Height.Uint64(),
				hash:    tests.RandomHash(t),
				status:  false,
			},
			expectedError: types.ErrHashMismatch,
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
		address types.Address
		height  uint64
		hash    types.Hash
		status  bool
	}

	addresses := getAddresses(t, 3)
	hashes := getHashes(t, 3)
	height := int64(30)
	testcases := []struct {
		name          string
		accMetaInfo   *types.AccountMetaInfo
		arg           args
		expectedError error
	}{
		{
			name: "shouldn't update with lower height",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       addresses[0],
				Type:          types.AccountType(1),
				Height:        big.NewInt(height),
				TesseractHash: hashes[0],
				LatticeExists: true,
				StateExists:   true,
			},
			arg: args{
				address: addresses[0],
				height:  uint64(height - 1),
				hash:    hashes[0],
				status:  false,
			},
		},
		{
			name: "should update with equal height",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       addresses[1],
				Type:          types.AccountType(1),
				Height:        big.NewInt(height),
				TesseractHash: hashes[1],
				LatticeExists: true,
				StateExists:   true,
			},
			arg: args{
				address: addresses[1],
				height:  uint64(height),
				hash:    hashes[1],
				status:  false,
			},
		},
		{
			name: "should update with new height",
			accMetaInfo: &types.AccountMetaInfo{
				Address:       addresses[2],
				Type:          types.AccountType(1),
				Height:        big.NewInt(height),
				TesseractHash: hashes[2],
				LatticeExists: true,
				StateExists:   true,
			},
			arg: args{
				address: addresses[2],
				height:  uint64(height + 1),
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

			actualAccMetaInfo, err := pm.GetAccountMetaInfo(test.arg.address.Bytes())
			require.NoError(t, err)

			// changes should take place if new height is greater than equal to current height
			if test.arg.height >= actualAccMetaInfo.Height.Uint64() {
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
		require.Equal(t, incrementBucketCounts[k], v.Int64())
	}
}

// here we insert 10000 random accounts and check if inserted accounts and fetched accounts match
func TestGetAccounts(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	insertedAccounts := insertTestAccMetaInfo(t, pm)

	// check if all accounts under every bucket number are stored properly
	for i := 0; i < 1024; i++ {
		actualAccounts, err := pm.GetAccounts(int32(i))
		require.NoError(t, err)

		insertedAccounts := insertedAccounts[int64(i)]
		require.Equal(t, len(insertedAccounts), len(actualAccounts),
			"inserted account count doesn't match")

		// traverse inserted accounts and check if it is present in actual accounts
		for _, insertedAccount := range insertedAccounts {
			isExists := checkIfAccountExists(insertedAccount, actualAccounts)

			require.True(t, isExists, "inserted account not found")
		}
	}
}

// here we insert 10000 entries and check if inserted entries and fetched entries match
func TestGetEntries(t *testing.T) {
	pm := NewTestPersistenceManager(t)
	insertedEntries, prefixes := insertTestEntries(t, pm)

	actualEntryCount := 0

	for _, prefix := range prefixes {
		actualEntries := pm.GetEntries([]byte(prefix))

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

func TestSetIxLookup(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	testcases := []struct {
		name        string
		ixHash      types.Hash
		receiptHash types.Hash
		expectedErr bool
	}{
		{
			name:        "Create an entry in db for the given tesseract hash",
			ixHash:      tests.RandomHash(t),
			receiptHash: tests.RandomHash(t),
			expectedErr: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := pm.SetIxLookup(testcase.ixHash, testcase.receiptHash.Bytes())
			require.NoError(t, err)

			rawData, err := pm.ReadEntry(DBKey(types.NilAddress, IxLookup, testcase.ixHash.Bytes()))
			require.NoError(t, err)

			require.Equal(t, testcase.receiptHash.Bytes(), rawData)
		})
	}
}

func TestGetIxLookup(t *testing.T) {
	pm := NewTestPersistenceManager(t)

	ixHash := tests.RandomHash(t)
	receiptHash := tests.RandomHash(t)
	insertIxLookup(t, pm, ixHash, receiptHash)

	testcases := []struct {
		name        string
		ixHash      types.Hash
		expectedErr bool
	}{
		{
			name:        "valid hash without state",
			ixHash:      tests.RandomHash(t),
			expectedErr: true,
		},
		{
			name:        "valid hash with state",
			ixHash:      ixHash,
			expectedErr: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixLookup, err := pm.GetIxLookup(testcase.ixHash)

			if testcase.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, receiptHash.Bytes(), ixLookup)
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
		receipts    types.Receipts
		receiptHash types.Hash
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
		name        string
		receipts    types.Receipts
		receiptHash types.Hash
		expectedErr bool
	}{
		{
			name:        "valid hash without state",
			receiptHash: tests.RandomHash(t),
			expectedErr: true,
		},
		{
			name:        "valid hash with state",
			receipts:    receipts,
			receiptHash: receiptHash,
			expectedErr: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			receipts, err := pm.GetReceipts(testcase.receiptHash)

			if testcase.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			expectedReceipts, err := testcase.receipts.Bytes()
			require.NoError(t, err)

			require.Equal(t, expectedReceipts, receipts)
		})
	}
}

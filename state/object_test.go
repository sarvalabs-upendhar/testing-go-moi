package state

import (
	"math/big"
	"testing"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
)

func TestBalanceOf(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	balance := big.NewInt(123)

	setAssetBalance(t, sObj, assetID, balance)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		expectedError error
	}{
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			expectedError: common.ErrAssetNotFound,
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
	setAssetBalance(t, sObj, assetID, big.NewInt(123))

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		amount          *big.Int
		expectedBalance *big.Int
		expectError     error
	}{
		{
			name:            "balance gets incremented if asset already exists",
			assetID:         assetID,
			amount:          big.NewInt(123),
			expectedBalance: big.NewInt(246),
		},
		{
			name:            "should return error if asset doesn't exist",
			assetID:         tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			amount:          big.NewInt(50),
			expectedBalance: big.NewInt(0), // no balance set
			expectError:     common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.AddBalance(test.assetID, test.amount)
			if test.expectError != nil {
				require.Error(t, err)
				require.Equal(t, err, test.expectError)

				return
			}

			require.NoError(t, err)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID)
		})
	}
}

func TestSubBalance(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	setAssetBalance(t, sObj, assetID, big.NewInt(124))

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		amount          *big.Int
		expectedBalance *big.Int
		expectError     error
	}{
		{
			name:            "balance gets decremented if asset already exists",
			assetID:         assetID,
			amount:          big.NewInt(123),
			expectedBalance: big.NewInt(1),
			expectError:     nil,
		},
		{
			name:            "should return error if asset doesn't exist",
			assetID:         tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			amount:          big.NewInt(50),
			expectedBalance: big.NewInt(0), // no balance set
			expectError:     common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.SubBalance(test.assetID, test.amount)

			if test.expectError != nil {
				require.Error(t, err)
				require.Equal(t, err, test.expectError)

				return
			}

			require.NoError(t, err)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID)
		})
	}
}

func TestCreateDeposit(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 3)

	setAssetDeposits(
		t, sObj, assetIDs, []*big.Int{big.NewInt(50000), big.NewInt(50000), big.NewInt(500)},
		[]identifiers.LogicID{}, []*big.Int{},
	)

	testcases := []struct {
		name                  string
		assetID               identifiers.AssetID
		logicID               identifiers.LogicID
		amount                *big.Int
		preTestFn             func(logicID identifiers.LogicID)
		expectedBalance       *big.Int
		expectedDepositAmount *big.Int
		expectedError         error
	}{
		{
			name:                  "creates a new deposit and updates balance",
			assetID:               assetIDs[0],
			logicID:               tests.GetLogicID(t, tests.RandomAddress(t)),
			amount:                big.NewInt(5000),
			expectedBalance:       big.NewInt(45000),
			expectedDepositAmount: big.NewInt(5000),
		},
		{
			name:    "increments existing deposit amount",
			assetID: assetIDs[1],
			logicID: tests.GetLogicID(t, tests.RandomAddress(t)),
			amount:  big.NewInt(5000),
			preTestFn: func(logicID identifiers.LogicID) {
				assert.NoError(t, sObj.CreateDeposit(assetIDs[1], logicID, big.NewInt(500)))
			},
			expectedBalance:       big.NewInt(44500),
			expectedDepositAmount: big.NewInt(5500),
		},
		{
			name:          "should return error if balance is insufficient",
			assetID:       assetIDs[2],
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			amount:        big.NewInt(5000),
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			amount:        big.NewInt(5000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.logicID)
			}

			err := sObj.CreateDeposit(test.assetID, test.logicID, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)

			assetObject, err := sObj.getAssetObject(test.assetID, true)
			require.NoError(t, err)
			require.Equal(t, test.expectedBalance, assetObject.Balance)

			amount, err := sObj.GetDeposit(test.assetID, test.logicID)
			require.NoError(t, err)
			require.Equal(t, test.expectedDepositAmount, amount)
		})
	}
}

func TestGetDeposit(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	logicIDs := tests.GetLogicIDs(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 1)

	setAssetDeposits(
		t, sObj, assetIDs, []*big.Int{big.NewInt(50000)},
		logicIDs, []*big.Int{big.NewInt(500)},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		logicID        identifiers.LogicID
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "retrieves existing deposit amount",
			assetID:        assetIDs[0],
			logicID:        logicIDs[0],
			expectedAmount: big.NewInt(500),
		},
		{
			name:          "should return error if asset doesn't exist",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if deposit doesn't exist",
			assetID:       assetIDs[0],
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			expectedError: errors.New("deposit doesn't exist"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			amount, err := sObj.GetDeposit(test.assetID, test.logicID)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, amount, test.expectedAmount)
		})
	}
}

func TestCreateMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	setAssetMandates(
		t, sObj, assetIDs, []*big.Int{big.NewInt(50000), big.NewInt(50000)},
		[]identifiers.Address{}, []*big.Int{},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		address        identifiers.Address
		amount         *big.Int
		preTestFn      func(address identifiers.Address)
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "creates a new mandate successfully",
			assetID:        assetIDs[0],
			address:        tests.RandomAddress(t),
			amount:         big.NewInt(5000),
			expectedAmount: big.NewInt(5000),
		},
		{
			name:    "increments existing mandate amount",
			assetID: assetIDs[1],
			address: tests.RandomAddress(t),
			amount:  big.NewInt(3000),
			preTestFn: func(address identifiers.Address) {
				assert.NoError(
					t,
					sObj.CreateMandate(
						assetIDs[1],
						address,
						big.NewInt(1000),
						time.Now().Add(1*time.Hour).Unix(),
					),
				)
			},
			expectedAmount: big.NewInt(4000),
		},
		{
			name:          "should return error if asset does not exist",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			address:       tests.RandomAddress(t),
			amount:        big.NewInt(3000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.address)
			}

			err := sObj.CreateMandate(test.assetID, test.address, test.amount, time.Now().Unix())

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.address, test.expectedAmount)
		})
	}
}

func TestSubMandateAmount(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	addresses := tests.GetRandomAddressList(t, 2)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	setAssetMandates(
		t, sObj, assetIDs, []*big.Int{big.NewInt(50000), big.NewInt(1000)},
		addresses, []*big.Int{big.NewInt(500), big.NewInt(1000)},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		address        identifiers.Address
		amount         *big.Int
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "deducts mandate amount successfully",
			assetID:        assetIDs[0],
			address:        addresses[0],
			amount:         big.NewInt(200),
			expectedAmount: big.NewInt(300),
		},
		{
			name:           "remove mandate when balance reaches zero",
			assetID:        assetIDs[1],
			address:        addresses[1],
			amount:         big.NewInt(1000),
			expectedAmount: big.NewInt(0),
		},
		{
			name:          "fails to deduct from nonexistent mandate",
			assetID:       assetIDs[0],
			address:       tests.RandomAddress(t),
			amount:        big.NewInt(100),
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:          "fails to deduct from nonexistent asset",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			address:       addresses[0],
			amount:        big.NewInt(100),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.SubMandateBalance(test.assetID, test.address, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.address, test.expectedAmount)
		})
	}
}

func TestConsumeMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	addresses := tests.GetRandomAddressList(t, 2)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	setAssetMandates(
		t, sObj, assetIDs, []*big.Int{big.NewInt(2000), big.NewInt(1000)},
		addresses, []*big.Int{big.NewInt(500), big.NewInt(1000)},
	)

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		address         identifiers.Address
		amount          *big.Int
		expectedMandate *big.Int
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:            "consumes mandate and balance successfully",
			assetID:         assetIDs[0],
			address:         addresses[0],
			amount:          big.NewInt(200),
			expectedMandate: big.NewInt(300),
			expectedBalance: big.NewInt(1800),
		},
		{
			name:          "fails to deduct mandate balance",
			assetID:       assetIDs[0],
			address:       tests.RandomAddress(t),
			amount:        big.NewInt(100),
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:          "fails to deduct asset balance",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			address:       addresses[0],
			amount:        big.NewInt(100),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.ConsumeMandate(test.assetID, test.address, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.address, test.expectedMandate)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID)
		})
	}
}

func TestDeleteMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	addresses := tests.GetRandomAddressList(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 1)

	setAssetMandates(
		t, sObj, assetIDs, []*big.Int{big.NewInt(50000)},
		addresses, []*big.Int{big.NewInt(500)},
	)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		address       identifiers.Address
		expectedError error
	}{
		{
			name:    "deletes existing mandate successfully",
			assetID: assetIDs[0],
			address: addresses[0],
		},
		{
			name:          "fails to delete mandate for nonexistent asset",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			address:       tests.RandomAddress(t),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.DeleteMandate(test.assetID, test.address)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.address, big.NewInt(0))
		})
	}
}

func TestGetMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	addresses := tests.GetRandomAddressList(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 1)

	setAssetMandates(
		t, sObj, assetIDs, []*big.Int{big.NewInt(50000)},
		addresses, []*big.Int{big.NewInt(500)},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		address        identifiers.Address
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "valid mandate returns correct amount",
			assetID:        assetIDs[0],
			address:        addresses[0],
			expectedAmount: big.NewInt(500),
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			address:       tests.RandomAddress(t),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if mandate not found",
			assetID:       assetIDs[0],
			address:       tests.RandomAddress(t),
			expectedError: common.ErrMandateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			mandate, err := sObj.GetMandate(test.assetID, test.address)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAmount, mandate.Amount)
		})
	}
}

func TestMandates(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	addresses := tests.GetRandomAddressList(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	// Set mandates for the test assets and addresses
	setAssetMandates(
		t, sObj, assetIDs, []*big.Int{big.NewInt(10000), big.NewInt(20000)},
		addresses, []*big.Int{big.NewInt(1000)},
	)

	testcases := []struct {
		name           string
		stateObject    *Object
		preTestFn      func(stateObject *Object)
		expectedResult []common.AssetMandate
		expectedError  error
	}{
		{
			name:        "retrieves existing mandates successfully",
			stateObject: sObj,
			expectedResult: []common.AssetMandate{
				{
					AssetID: assetIDs[0],
					Address: addresses[0],
					Amount:  big.NewInt(1000),
				},
				{
					AssetID: assetIDs[1],
					Address: addresses[0],
					Amount:  big.NewInt(1000),
				},
			},
		},
		{
			name:           "returns empty list when mandates doesn't exist",
			stateObject:    createTestStateObject(t, nil),
			expectedResult: []common.AssetMandate{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			mandates, err := test.stateObject.Mandates()

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, test.expectedResult, mandates)
		})
	}
}

func TestCopy(t *testing.T) {
	testcases := []struct {
		name     string
		soParams *createStateObjectParams
	}{
		{
			name:     "copied whole state object",
			soParams: stateObjectParamsWithTestData(t, false),
		},
		{
			name:     "logic tree, asset tree and meta storage tree are not copied",
			soParams: stateObjectParamsWithTestData(t, true),
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)
			copiedSO := sObj.Copy()
			checkIfStateObjectAreEqual(t, sObj, copiedSO)
			checkForReferences(t, sObj, copiedSO)
		})
	}
}

func TestCommitDeeds(t *testing.T) {
	deeds, _ := getTestDeeds(
		t,
		map[string]struct{}{tests.RandomHash(t).String(): {}},
	)
	sObj := createTestStateObject(t, stateObjectParamsWithDeeds(t, common.NilHash, deeds))

	actualDeedsHash, err := sObj.commitDeeds()
	require.NoError(t, err)

	checkForDeeds(t, sObj, deeds, actualDeedsHash)
}

func TestCommitAccount(t *testing.T) {
	inputAcc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.ContextHash = tests.RandomHash(t)
	})

	sObj := createTestStateObject(t, &createStateObjectParams{
		account: inputAcc,
	})

	actualAccHash, err := sObj.commitAccount()
	require.NoError(t, err)

	checkForAccount(t, sObj, inputAcc, actualAccHash, 0)
}

func TestCommitContext(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ctxObject, _ := getContextObjects(t, tests.RandomKramaIDs(t, 2), 2, 1)

	actualCtxHash, err := sObj.commitContextObject(ctxObject[0])
	require.NoError(t, err)

	checkForContextObject(t, sObj, *ctxObject[0], actualCtxHash)
}

func TestCommitActiveStorageTrees(t *testing.T) {
	logicIDs := tests.GetLogicIDs(t, 1)

	keys, values := getEntries(t, 1)
	newKeys, newValues := getEntries(t, 2)

	testcases := []struct {
		name            string
		storageTxns     map[identifiers.LogicID]*iradix.Txn
		storageTrees    map[identifiers.LogicID]tree.MerkleTree
		metaStorageTree *MockMerkleTree
		expectedError   error
	}{
		{
			name: "commit active storage trees successfully",
			storageTxns: map[identifiers.LogicID]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			storageTrees: map[identifiers.LogicID]tree.MerkleTree{
				logicIDs[0]: getMerkleTreeWithEntries(t, keys, values),
			},
			metaStorageTree: mockMerkleTreeWithDB(),
		},
		{
			name: "should return error if storage tree doesn't exists",
			storageTxns: map[identifiers.LogicID]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			metaStorageTree: mockMerkleTreeWithDB(),
			expectedError:   common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "should return error if fail to commit storage tree",
			storageTxns: map[identifiers.LogicID]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			storageTrees: map[identifiers.LogicID]tree.MerkleTree{
				logicIDs[0]: getMerkleTreeWithCommitHook(t,
					nil,
					nil,
					func() error {
						return errors.New("failed to commit storage tree")
					},
				),
			},
			metaStorageTree: mockMerkleTreeWithDB(),
			expectedError:   errors.New("failed to commit storage tree"),
		},
		{
			name: "should return error if fail to update meta storage tree",
			storageTxns: map[identifiers.LogicID]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			storageTrees: map[identifiers.LogicID]tree.MerkleTree{
				logicIDs[0]: getMerkleTreeWithHook(t,
					nil,
					nil,
					func() error {
						return errors.New("failed to set entries")
					},
				),
			},
			metaStorageTree: getMerkleTreeWithHook(t,
				nil,
				nil,
				func() error {
					return errors.New("failed to set entries")
				},
			),
			expectedError: errors.New("failed to set entries"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			storageTrees := copyStorageTrees(test.storageTrees)
			sObj := createTestStateObject(t, &createStateObjectParams{
				soCallback: func(so *Object) {
					so.storageTreeTxns = test.storageTxns
					so.storageTrees = test.storageTrees
					so.metaStorageTree = test.metaStorageTree
				},
			})

			err := sObj.commitActiveStorageTrees()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfStorageTreesAreCommitted(t, sObj, storageTrees)
		})
	}
}

func TestCommitMetaStorageTree(t *testing.T) {
	keys, values := getEntries(t, 1)

	mst := getMerkleTreeWithEntries(t, keys, values)

	testcases := []struct {
		name          string
		mst           tree.MerkleTree
		storageRoot   common.Hash
		expectedError error
	}{
		{
			name:        "mst doesn't get committed as it doesn't have dirty entries",
			mst:         getMerkleTreeWithEntries(t, keys, values),
			storageRoot: tests.RandomHash(t),
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
				func() (common.Hash, error) {
					return common.NilHash, errors.New("failed to calculate mst root")
				},
			),
			expectedError: errors.New("failed to calculate mst root"),
		},
		{
			name:        "committed meta storage tree successfully",
			mst:         mst,
			storageRoot: tests.RandomHash(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithMST(t, identifiers.NilAddress, nil, test.mst, test.storageRoot),
			)

			generatedStorageRoot, err := sObj.commitMetaStorageTree()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkIfMetaStorageTreeCommitted(
				t,
				generatedStorageRoot,
				getRoot(t, test.mst.(*MockMerkleTree)), //nolint
				sObj,
			)
		})
	}
}

func TestCommitStorage(t *testing.T) {
	logicIds := tests.GetLogicIDs(t, 1)
	keys, values := getEntries(t, 5)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		isMSTNil      bool
		expectedError error
	}{
		{
			name:     "meta storage tree and txns are nil",
			soParams: stateObjectParamsWithASTAndMST(t, nil, nil),
			isMSTNil: true,
		},
		{
			name: "should return error if failed to commit active storage trees",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.storageTreeTxns = map[identifiers.LogicID]*iradix.Txn{
						logicIds[0]: getTxnsWithEntries(t, keys, values),
					}
					// create a meta storage tree with no entry for logicID[0].
					so.metaStorageTree = mockMerkleTreeWithDB()
				},
			},
			// Commit should fail since getStorageTree return an error
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "should return error if failed to commit meta storage trees",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					// We set storage tree and txn, so that commitActiveStorageTress will not fail
					so.storageTreeTxns = map[identifiers.LogicID]*iradix.Txn{
						logicIds[0]: getTxnsWithEntries(t, keys, values),
					}
					so.storageTrees = map[identifiers.LogicID]tree.MerkleTree{
						logicIds[0]: mockMerkleTreeWithDB(),
					}
					// set a commit hook
					so.metaStorageTree = getMerkleTreeWithCommitHook(t, keys, values, func() error {
						return errors.New("failed to commit")
					})
				},
			},
			expectedError: errors.New("failed to commit"),
		},
		{
			name: "should commit active storage trees and meta storage tree",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					// We set storage tree and txn, so that commitActiveStorageTress will not fail
					so.storageTreeTxns = getStorageTxnsWithEntries(t, logicIds, keys, values)
					so.storageTrees = map[identifiers.LogicID]tree.MerkleTree{
						logicIds[0]: mockMerkleTreeWithDB(),
					}
					so.metaStorageTree = mockMerkleTreeWithDB()
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			storageTrees := copyStorageTrees(sObj.storageTrees)
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

			// check if storage trees are committed
			checkIfStorageTreesAreCommitted(t, sObj, storageTrees)
			// check if meta storage tree is committed
			require.True(t, sObj.metaStorageTree.(*MockMerkleTree).isCommitted) //nolint
		})
	}
}

func TestCommitAssets(t *testing.T) {
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	assetObject := createAssetObject(t)
	assetTree := getMerkleTreeWithEntries(t, nil, nil)
	inputAssetTree := assetTree.Copy()

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		assetTxn      *iradix.Txn
		assetTree     tree.MerkleTree
		assetRoot     common.Hash
		expectedError error
	}{
		{
			name:      "asset tree is nil",
			assetRoot: tests.RandomHash(t),
		},
		{
			name:      "committed asset tree successfully",
			assetTxn:  getTxnWithAssetObjects(t, []identifiers.AssetID{assetID}, assetObject),
			assetTree: assetTree,
		},
		{
			name:     "should return error if failed to commit asset tree",
			assetTxn: getTxnWithAssetObjects(t, []identifiers.AssetID{assetID}, assetObject),
			assetTree: getMerkleTreeWithCommitHook(
				t,
				emptyKeys,
				emptyValues,
				func() error {
					return errors.New("failed to commit asset tree")
				},
			),
			expectedError: errors.New("failed to commit asset tree"),
		},
		{
			name:     "should return error if failed to calculate root hash",
			assetTxn: getTxnWithAssetObjects(t, []identifiers.AssetID{assetID}, assetObject),
			assetTree: getMerkleTreeWithRootHashHook(
				t,
				emptyKeys,
				emptyValues,
				func() (common.Hash, error) {
					return common.NilHash, errors.New("failed to calculate asset root")
				},
			),
			expectedError: errors.New("failed to calculate asset root"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithAssetTree(
					t,
					identifiers.NilAddress,
					nil,
					test.assetTree,
					test.assetRoot,
					test.assetTxn,
				),
			)

			actualRootHash, err := sObj.commitAssets()
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			if test.assetTree == nil {
				require.Equal(t, sObj.data.AssetRoot, actualRootHash)

				return
			}

			require.NoError(t, err)
			checkIfAssetTreeCommitted(t, inputAssetTree, sObj, actualRootHash)
		})
	}
}

func TestCommitLogics(t *testing.T) {
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	logicTree := getMerkleTreeWithEntries(t, nil, nil)
	inputLogicTree := logicTree.Copy()

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		logicTxn      *iradix.Txn
		logicTree     tree.MerkleTree
		logicRoot     common.Hash
		expectedError error
	}{
		{
			name:      "logic tree is nil",
			logicRoot: tests.RandomHash(t),
		},
		{
			name:      "committed logic tree successfully",
			logicTxn:  getTxnWithLogicObjects(t, logicObject),
			logicTree: logicTree,
		},
		{
			name:     "should return error if failed to commit logic tree",
			logicTxn: getTxnWithLogicObjects(t, logicObject),
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
			name:     "should return error if failed to calculate root hash",
			logicTxn: getTxnWithLogicObjects(t, logicObject),
			logicTree: getMerkleTreeWithRootHashHook(
				t,
				emptyKeys,
				emptyValues,
				func() (common.Hash, error) {
					return common.NilHash, errors.New("failed to calculate logic root")
				},
			),
			expectedError: errors.New("failed to calculate logic root"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(
					t,
					identifiers.NilAddress,
					nil,
					test.logicTree,
					test.logicRoot,
					test.logicTxn,
				),
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
	logicIds := tests.GetLogicIDs(t, 1)
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicIds[0]))

	keys, values := getEntries(t, 1)

	astWithDirtyEntries := getStorageTreesWithDefaultEntries(t, 2, 1)

	mstWithDirtyEntries := getMerkleTreeWithDefaultEntries(t, 1)

	logicTree := getMerkleTreeWithEntries(t, keys, values)

	assetTree := getMerkleTreeWithEntries(t, keys, values)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name: "should return error if failed to commit logic tree",
			soParams: stateObjectParamsWithLogicTree(
				t,
				identifiers.NilAddress,
				nil,
				getMerkleTreeWithCommitHook(
					t,
					emptyKeys,
					emptyValues,
					func() error {
						return errors.New("failed to commit logic tree")
					},
				),
				common.NilHash,
				getTxnWithLogicObjects(t, logicObject),
			),
			expectedError: errors.New("failed to commit logic tree"),
		},
		{
			name: "should return error if failed to commit storage tree",
			soParams: soParamsWithStorageTreesAndTxns(
				t,
				getStorageTreesWithCommitHook(
					t,
					logicIds,
					keys,
					values,
					func() error {
						return errors.New("failed to commit ast")
					},
				),
				getStorageTxnsWithEntries(t, logicIds, keys, values),
			),
			expectedError: errors.New("failed to commit ast"),
		},
		{
			name: "committed successfully",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.storageTrees = astWithDirtyEntries
					so.logicTree = logicTree
					so.assetTree = assetTree
					so.metaStorageTree = mstWithDirtyEntries
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

			inputAcc := getAccount(t, nil, mstWithDirtyEntries, assetTree, logicTree)

			checkForAccount(t, sObj, inputAcc, actualAccHash, 2)
		})
	}
}

func getAccount(
	t *testing.T,
	reg *Deeds,
	metaStorage, assetTree, logicTree tree.MerkleTree,
) *common.Account {
	t.Helper()

	acc := new(common.Account)

	rawReg, err := reg.Bytes()
	require.NoError(t, err)

	assetRoot, err := assetTree.RootHash()
	require.NoError(t, err)

	logicRoot, err := logicTree.RootHash()
	require.NoError(t, err)
	storageRoot, err := metaStorage.RootHash()
	require.NoError(t, err)

	if reg != nil {
		acc.AssetDeeds = common.GetHash(rawReg)
	}

	acc.AssetRoot = assetRoot
	acc.LogicRoot = logicRoot
	acc.StorageRoot = storageRoot

	return acc
}

func TestFlushAssetTree(t *testing.T) {
	testcases := []struct {
		name      string
		assetTree tree.MerkleTree
	}{
		{
			name:      "asset tree is nil",
			assetTree: nil,
		},
		{
			name:      "asset tree flushed to db successfully",
			assetTree: mockMerkleTreeWithDB(),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithAssetTree(t, identifiers.NilAddress, nil, test.assetTree, common.NilHash, nil),
			)

			err := sObj.flushAssetTree()
			require.NoError(t, err)

			if test.assetTree != nil {
				checkIfMerkleTreeFlushed(t, sObj.assetTree, true)

				return
			}
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
				stateObjectParamsWithLogicTree(t, identifiers.NilAddress, nil, test.logicTree, common.NilHash, nil),
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

func TestStateManager_FlushDirtyObject(t *testing.T) {
	db := mockDB()
	dbWithCreateEntryHook := mockDB()
	dbWithCreateEntryHook.createEntryHook = func() error {
		return errors.New("failed to write dirty entries")
	}

	dirtyEntries := getDirtyEntries(t, 5)
	keys, values := getEntries(t, 4)

	merkle := mockMerkleTreeWithDB()

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name: "state object exists",
			soParams: &createStateObjectParams{
				db: db,
				metaStorageTreeCallback: func(so *Object) {
					so.metaStorageTree = merkle
					setEntries(t, merkle, keys, values)
				},
				soCallback: func(so *Object) {
					so.dirtyEntries = dirtyEntries
				},
			},
		},
		{
			name: "failed to flush storage trees",
			soParams: &createStateObjectParams{
				db: db,
				metaStorageTreeCallback: func(so *Object) {
					m := mockMerkleTreeWithDB()
					m.flushHook = func() error {
						return errors.New("flush failed")
					}
					so.metaStorageTree = m
				},
			},
			expectedError: errors.New("failed to flush active storage trees"),
		},
		{
			name: "failed to flush dirty entries",
			soParams: &createStateObjectParams{
				db: dbWithCreateEntryHook,
				metaStorageTreeCallback: func(so *Object) {
					so.metaStorageTree = merkle
					setEntries(t, merkle, keys, values)
				},
				soCallback: func(so *Object) {
					so.dirtyEntries = dirtyEntries
				},
			},
			expectedError: errors.New("failed to write dirty entries"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			obj := createTestStateObject(t, test.soParams)
			err := obj.flush()

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
				val, err := db.ReadEntry(common.Hex2Bytes(k))
				require.NoError(t, err)
				require.Equal(t, v, val)
			}
		})
	}
}

func TestUpdateAssetTree(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	assetObject := NewAssetObject(big.NewInt(500), nil)

	setAssetObject(t, sObj, assetID, assetObject)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		assetObject   *AssetObject
		expectedValue *AssetObject
	}{
		{
			name:          "insert new asset",
			assetID:       assetID,
			assetObject:   assetObject,
			expectedValue: assetObject,
		},
		{
			name:          "insert with non-existent asset ID",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			assetObject:   assetObject,
			expectedValue: assetObject,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj.updateAssetTree(test.assetID, test.assetObject)

			value, ok := sObj.assetTreeTxn.Get(test.assetID.Bytes())
			require.True(t, ok)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestUpdateLogicTree(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := NewLogicObject(tests.RandomAddress(t), engineio.LogicDescriptor{Engine: engineio.PISA})

	setLogicObject(t, sObj, logicID, logicObject)

	testcases := []struct {
		name          string
		logicID       identifiers.LogicID
		logicObject   *LogicObject
		expectedValue *LogicObject
	}{
		{
			name:          "insert new logic object",
			logicID:       logicID,
			logicObject:   logicObject,
			expectedValue: logicObject,
		},
		{
			name:          "insert with non-existent logic ID",
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			logicObject:   logicObject,
			expectedValue: logicObject,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj.updateLogicTree(test.logicID, test.logicObject)

			value, ok := sObj.logicTreeTxn.Get(test.logicID.Bytes())
			require.True(t, ok)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestStorageTrees(t *testing.T) {
	logicIds := tests.GetLogicIDs(t, 2)
	ast := getStorageTrees(t, logicIds, emptyKeys, emptyValues)
	mst := mockMerkleTreeWithDB()
	testcases := []struct {
		name          string
		mst           tree.MerkleTree
		ast           map[identifiers.LogicID]tree.MerkleTree
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

			err := sObj.flushStorageTrees()
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
	assetAddress1 := tests.RandomAddress(t)
	assetAddress2 := tests.RandomAddress(t)

	sObj := createTestStateObject(t, nil)

	assetDescriptor := getTestAssetDescriptor(t, sObj.address, "btc")

	assetID := identifiers.NewAssetIDv0(
		assetDescriptor.IsLogical,
		assetDescriptor.IsStateFul,
		assetDescriptor.Dimension,
		uint16(assetDescriptor.Standard),
		assetAddress2,
	)

	setAssetState(t, sObj, assetID, assetDescriptor)

	testcases := []struct {
		name            string
		assetAddress    identifiers.Address
		assetDescriptor *common.AssetDescriptor
		expectedError   error
	}{
		{
			name:            "asset created successfully",
			assetAddress:    assetAddress1,
			assetDescriptor: getTestAssetDescriptor(t, sObj.address, "moi"),
		},
		{
			name:            "should return error if asset already exists",
			assetAddress:    assetAddress2,
			assetDescriptor: assetDescriptor,
			expectedError:   common.ErrAssetAlreadyRegistered,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualAssetID, err := sObj.CreateAsset(test.assetAddress, test.assetDescriptor)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			expectedAssetID := getTestAssetID(test.assetAddress, test.assetDescriptor)

			require.NoError(t, err)
			require.Equal(t, expectedAssetID, actualAssetID)

			CheckAssetCreation(t, sObj, test.assetDescriptor, expectedAssetID)
		})
	}
}

//nolint:dupl
func TestMintAsset(t *testing.T) {
	sObj := createTestStateObject(t, nil)

	assetDescriptor := getTestAssetDescriptor(t, sObj.address, "btc")

	assetID := identifiers.NewAssetIDv0(
		assetDescriptor.IsLogical,
		assetDescriptor.IsStateFul,
		assetDescriptor.Dimension,
		uint16(assetDescriptor.Standard),
		tests.RandomAddress(t),
	)

	setAssetState(t, sObj, assetID, assetDescriptor)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name:           "asset minted successfully",
			assetID:        assetID,
			amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(11000),
		},
		{
			name:          "should return error if asset doesn't exists",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			amount:        big.NewInt(2000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			supply, err := sObj.MintAsset(test.assetID, test.amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, &supply)
		})
	}
}

//nolint:dupl
func TestBurnAsset(t *testing.T) {
	sObj := createTestStateObject(t, nil)

	assetDescriptor := getTestAssetDescriptor(t, sObj.address, "eth")

	assetID := identifiers.NewAssetIDv0(
		assetDescriptor.IsLogical,
		assetDescriptor.IsStateFul,
		assetDescriptor.Dimension,
		uint16(assetDescriptor.Standard),
		tests.RandomAddress(t),
	)

	setAssetState(t, sObj, assetID, assetDescriptor)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name:           "asset burned successfully",
			assetID:        assetID,
			amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(9000),
		},
		{
			name:          "should return error if asset doesn't exists",
			assetID:       tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			amount:        big.NewInt(2000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			supply, err := sObj.BurnAsset(test.assetID, test.amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, &supply)
		})
	}
}

func TestGetMetaContextObjectCopy(t *testing.T) {
	hashes := tests.GetHashes(t, 4)
	mObj, mHash := getMetaContextObjects(t, hashes)

	testcases := []struct {
		name           string
		soParams       *createStateObjectParams
		expectedObject *MetaContextObject
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
	kramaIDs := tests.RandomKramaIDs(t, 4)
	cObj, cHash := getContextObjects(t, kramaIDs, 2, 2)

	testcases := []struct {
		name           string
		hash           common.Hash
		soParams       *createStateObjectParams
		expectedObject *ContextObject
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
		behaviouralNodes []kramaid.KramaID
		randomNodes      []kramaid.KramaID
		expectedError    error
	}{
		{
			name:             "created context successfully",
			behaviouralNodes: tests.RandomKramaIDs(t, 2),
			randomNodes:      tests.RandomKramaIDs(t, 2),
			expectedError:    nil,
		},
		{
			name:             "should return error if failed to create context",
			behaviouralNodes: tests.RandomKramaIDs(t, 0),
			randomNodes:      tests.RandomKramaIDs(t, 0),
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
	kramaIDs := tests.RandomKramaIDs(t, 4)
	cObj, cHash := getContextObjects(t, kramaIDs, 2, 2)
	mObj, mHash := getMetaContextObjects(t, cHash)

	behaviouralNodes := tests.RandomKramaIDs(t, 2)
	randomNodes := tests.RandomKramaIDs(t, 2)

	testcases := []struct {
		name             string
		behaviouralNodes []kramaid.KramaID
		randomNodes      []kramaid.KramaID
		metaHash         common.Hash
		soParams         *createStateObjectParams
		mCtx             *MetaContextObject
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
				soCallback: func(so *Object) {
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

func TestLoadDeedsObject(t *testing.T) {
	deeds, deedsHash := getTestDeeds(t, map[string]struct{}{
		"MOI": {},
		"BTR": {},
	})

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedDeeds *Deeds
		expectedError error
	}{
		{
			name: "should successfully load deeds using deeds hash",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					insertAssetDeedsInDB(t, so.db, []common.Hash{deedsHash}, deeds)
					so.data.AssetDeeds = deedsHash
				},
			},
			expectedDeeds: deeds,
		},
		{
			name: "should return error if failed to load deeds",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.data.AssetDeeds = tests.RandomHash(t)
				},
			},
			expectedError: common.ErrKeyNotFound,
		},
		{
			name: "should return empty deeds object for nil deeds hash",
			expectedDeeds: &Deeds{
				Entries: make(map[string]struct{}),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)
			err := sObj.loadDeeds()

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedDeeds, sObj.deeds)
		})
	}
}

func TestHasFuel(t *testing.T) {
	testcases := []struct {
		name          string
		amount        *big.Int
		soParams      *createStateObjectParams
		hasFuel       bool
		expectedError error
	}{
		{
			name:   "has enough fuel",
			amount: big.NewInt(200),
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					setAssetBalance(t, so, common.KMOITokenAssetID, big.NewInt(1000))
				},
			},
			hasFuel: true,
		},
		{
			name:   "insufficient fuel",
			amount: big.NewInt(200),
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					setAssetBalance(t, so, common.KMOITokenAssetID, big.NewInt(100))
				},
			},
			hasFuel: false,
		},
		{
			name:          "invalid transfer amount",
			amount:        big.NewInt(-122),
			expectedError: errors.New("invalid transfer amount"),
		},
		{
			name:    "failed to load balance",
			amount:  big.NewInt(200),
			hasFuel: false,
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			hasFuel, err := sObj.HasSufficientFuel(test.amount)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.hasFuel, hasFuel)
		})
	}
}

func TestGetMetaStorageTree(t *testing.T) {
	keys, values := getEntries(t, 2)

	db := mockDB()
	address := tests.RandomAddress(t)
	storageTree, storageRoot := createTestKramaHashTree(t,
		db,
		address,
		storage.Storage,
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
			soParams: stateObjectParamsWithMST(t, address, db, storageTree, common.NilHash),
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
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	keys, values := getEntries(t, 2)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		logicID       identifiers.LogicID
		expectedError error
	}{
		{
			name:     "fetched storage tree from active storage trees",
			soParams: stateObjectParamsWithStorageTree(t, getStorageTrees(t, []identifiers.LogicID{logicID}, keys, values)),
			logicID:  logicID,
		},
		{
			name:          "should return error if failed to get meta storage tree",
			soParams:      stateObjectParamsWithInvalidMST(t),
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			expectedError: errors.New("failed to initiate storage tree"),
		},
		{
			name:          "should return error if logic storage tree not found",
			logicID:       tests.GetLogicID(t, tests.RandomAddress(t)),
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "fetched storage tree from db",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.metaStorageTree, _ = createMetaStorageTree(
						t,
						so.db,
						so.address,
						common.SargaLogicID,
						keys,
						values,
					)
				},
			},
			logicID: common.SargaLogicID,
		},
		{
			name: "should return error if failed to initiate logic storage tree",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.metaStorageTree, _ = createTestKramaHashTree(
						t,
						so.db,
						so.address,
						storage.Storage,
						[][]byte{common.SargaLogicID.Bytes()},
						[][]byte{tests.RandomHash(t).Bytes()},
					)
				},
			},
			logicID:       common.SargaLogicID,
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
	logicID := tests.GetLogicIDs(t, 1)
	keys, values := getEntries(t, 2)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		logicID       identifiers.LogicID
		expectedError error
	}{
		{
			name:          "should return error if storage tree is not initiated",
			soParams:      stateObjectParamsWithASTAndMST(t, nil, mockMerkleTreeWithDB()),
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "should add storage entry",
			soParams: stateObjectParamsWithASTAndMST(
				t,
				map[identifiers.LogicID]tree.MerkleTree{
					logicID[0]: mockMerkleTreeWithDB(),
				},
				mockMerkleTreeWithDB(),
			),
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)
			err := sObj.SetStorageEntry(logicID[0], keys[1], values[1])

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			txn, ok := sObj.storageTreeTxns[(logicID)[0]]
			require.True(t, ok)

			checkForEntryInTxn(t, txn, keys[1], values[1])
		})
	}
}

func TestAddAccountGenesisInfo(t *testing.T) {
	keys, values := getEntries(t, 1)

	testcases := []struct {
		name          string
		address       identifiers.Address
		ixHash        common.Hash
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:    "should succeed if account genesis info added",
			address: tests.RandomAddress(t),
			ixHash:  tests.RandomHash(t),
			soParams: stateObjectParamsWithStorageTree(t, getStorageTrees(
				t, []identifiers.LogicID{common.SargaLogicID}, keys, values,
			)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			err := sObj.AddAccountGenesisInfo(test.address, test.ixHash)
			require.NoError(t, err)

			accInfo := common.AccountGenesisInfo{
				IxHash: test.ixHash,
			}

			expectedValue, err := accInfo.Bytes()
			require.NoError(t, err)

			actualValue, err := sObj.GetStorageEntry(common.SargaLogicID, test.address.Bytes())
			require.NoError(t, err)

			require.Equal(t, expectedValue, actualValue)
		})
	}
}

func TestGetStorageEntry(t *testing.T) {
	logicIDs := tests.GetLogicIDs(t, 2)
	keys, values := getEntries(t, 1)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:     "successfully fetched entry in storage tree",
			soParams: stateObjectParamsWithStorageTree(t, getStorageTrees(t, logicIDs, keys, values)),
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

func TestGetMetaLogicTree(t *testing.T) {
	keys, values := getEntries(t, 2)
	db := mockDB()
	address := tests.RandomAddress(t)
	logicTree, logicRoot := createTestKramaHashTree(t,
		db,
		address,
		storage.Storage,
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
			soParams: stateObjectParamsWithLogicTree(t, address, db, nil, logicRoot, nil),
		},
		{
			name:     "fetch logic tree from cache",
			soParams: stateObjectParamsWithLogicTree(t, address, db, logicTree, common.NilHash, nil),
		},
		{
			name:          "should return error if failed to initiate logic tree",
			soParams:      stateObjectParamsWithLogicTree(t, address, db, nil, tests.RandomHash(t), nil),
			expectedError: errors.New("failed to initiate logic tree"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualLogicTree, err := sObj.getLogicTree()
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

func TestGetAssetObject(t *testing.T) {
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	assetObject := createAssetObject(t)
	rawData, err := assetObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name: "should return error if asset tree not found",
			soParams: stateObjectParamsWithAssetTree(
				t,
				identifiers.NilAddress,
				nil,
				nil,
				tests.RandomHash(t),
				nil,
			),
			expectedError: errors.New("failed to initiate asset tree"),
		},
		{
			name: "should return error if asset object not found",
			soParams: stateObjectParamsWithAssetTree(
				t,
				identifiers.NilAddress,
				nil,
				nil,
				common.NilHash,
				nil,
			),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name: "fetched asset object from asset tree successfully",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.assetTree = nil

					_, so.data.AssetRoot = createTestKramaHashTree(t,
						so.db,
						so.address,
						storage.Storage,
						[][]byte{assetID.Bytes()},
						[][]byte{rawData},
					)
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			actualAssetObject, err := sObj.getAssetObject(assetID, true)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, assetObject, actualAssetObject)
		})
	}
}

func TestGetLogicObject(t *testing.T) {
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	rawData, err := logicObject.Bytes()
	require.NoError(t, err)

	// seed the engine runtimes
	engineio.RegisterEngine(pisa.NewEngine())

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name: "should return error if logic tree not found",
			soParams: stateObjectParamsWithLogicTree(
				t,
				identifiers.NilAddress,
				nil,
				nil,
				tests.RandomHash(t),
				nil,
			),
			expectedError: errors.New("failed to initiate logic tree"),
		},
		{
			name: "should return error if logic object not found",
			soParams: stateObjectParamsWithLogicTree(
				t,
				identifiers.NilAddress,
				nil,
				nil,
				common.NilHash,
				nil,
			),
			expectedError: common.ErrKeyNotFound,
		},
		{
			name: "fetched logic object from logic tree successfully",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.logicTree = nil

					_, so.data.LogicRoot = createTestKramaHashTree(t,
						so.db,
						so.address,
						storage.Storage,
						[][]byte{logicID.Bytes()},
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

func TestIsAssetRegistered(t *testing.T) {
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	assetObject := createAssetObject(t)
	rawData, err := assetObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		assetTree     tree.MerkleTree
		expectedError error
	}{
		{
			name:          "should return error if asset not registered",
			assetTree:     nil,
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:      "asset registered",
			assetTree: getMerkleTreeWithEntries(t, [][]byte{assetID.Bytes()}, [][]byte{rawData}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithAssetTree(
					t,
					identifiers.NilAddress,
					nil,
					test.assetTree,
					common.NilHash,
					nil,
				),
			)

			err = sObj.isAssetRegistered(assetID)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIsLogicRegistered(t *testing.T) {
	// seed the engine runtimes
	engineio.RegisterEngine(pisa.NewEngine())

	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
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
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:      "logic registered",
			logicTree: getMerkleTreeWithEntries(t, [][]byte{logicID.Bytes()}, [][]byte{rawData}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(
					t,
					identifiers.NilAddress,
					nil,
					test.logicTree,
					common.NilHash,
					nil,
				),
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

func TestInsertNewAssetObject(t *testing.T) {
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	assetObject := createAssetObject(t)
	rawData, err := assetObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		assetTree     tree.MerkleTree
		assetRoot     common.Hash
		assetID       identifiers.AssetID
		expectedError error
	}{
		{
			name:          "should return error if asset already registered",
			assetTree:     getMerkleTreeWithEntries(t, [][]byte{assetID.Bytes()}, [][]byte{rawData}),
			assetID:       assetID,
			expectedError: errors.New("asset already registered"),
		},
		{
			name:      "asset object inserted successfully",
			assetTree: getMerkleTreeWithEntries(t, [][]byte{assetID.Bytes()}, [][]byte{rawData}),
			assetID:   tests.GetRandomAssetID(t, tests.RandomAddress(t)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithAssetTree(
					t,
					identifiers.NilAddress,
					nil,
					test.assetTree,
					test.assetRoot,
					nil,
				),
			)

			err = sObj.InsertNewAssetObject(test.assetID, assetObject)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			actualAssetObject, err := sObj.getAssetObject(assetID, true)

			require.NoError(t, err)
			require.Equal(t, assetObject, actualAssetObject)
		})
	}
}

func TestInsertNewLogicObject(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())

	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	rawData, err := logicObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		logicTree     tree.MerkleTree
		logicRoot     common.Hash
		logicID       identifiers.LogicID
		expectedError error
	}{
		{
			name:          "should return error if logic already registered",
			logicTree:     getMerkleTreeWithEntries(t, [][]byte{logicID.Bytes()}, [][]byte{rawData}),
			logicID:       logicID,
			expectedError: errors.New("logic already registered"),
		},
		{
			name:      "logic object inserted successfully",
			logicTree: getMerkleTreeWithEntries(t, [][]byte{logicID.Bytes()}, [][]byte{rawData}),
			logicID:   tests.GetLogicID(t, tests.RandomAddress(t)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(
					t,
					identifiers.NilAddress,
					nil,
					test.logicTree,
					test.logicRoot,
					nil,
				),
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
	logicID := tests.GetLogicID(t, tests.RandomAddress(t))

	testcases := []struct {
		name          string
		logicID       identifiers.LogicID
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
			expectedLogicTree, ok := sObj.storageTrees[logicID]
			require.True(t, ok)

			checkForKramaHashTree(t, expectedLogicTree, actualStorageTree)
			checkForEntryInMST(t, sObj, logicID.Bytes(), common.NilHash.Bytes())
		})
	}
}

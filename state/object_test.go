package state

import (
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/pkg/errors"
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	balance := big.NewInt(123)

	WithAssetBalance(t, sObj, assetID, common.DefaultTokenID, balance, nil, nil)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		tokenID       common.TokenID
		expectedError error
	}{
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if token not found",
			assetID:       assetID,
			tokenID:       tests.GetRandomTokenID(t),
			expectedError: common.ErrTokenNotFound,
		},
		{
			name:    "fetched balance successfully",
			assetID: assetID,
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actualBalance, err := sObj.BalanceOf(test.assetID, test.tokenID)
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	tokenID := tests.GetRandomTokenID(t)
	tokenMetaData := &MetaData{static: map[string][]byte{"URL": []byte("https://moi.technology")}}
	WithAssetBalance(
		t, sObj,
		assetID, tokenID, big.NewInt(123),
		nil,
		tokenMetaData,
	)

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		tokenID         common.TokenID
		amount          *big.Int
		expectedBalance *big.Int
		expectError     error
		metaData        *MetaData
	}{
		{
			name:            "balance gets incremented if asset already exists",
			assetID:         assetID,
			tokenID:         tokenID,
			amount:          big.NewInt(123),
			expectedBalance: big.NewInt(246),
		},
		{
			name:            "balance and metadata for the given tokenID should be updated",
			assetID:         assetID,
			tokenID:         1,
			amount:          big.NewInt(123),
			expectedBalance: big.NewInt(123),
			metaData:        &MetaData{static: map[string][]byte{"URL": []byte("https://sarva.ai")}},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.AddBalance(test.assetID, test.tokenID, test.amount, test.metaData)
			if test.expectError != nil {
				require.Error(t, err)
				require.Equal(t, err, test.expectError)

				return
			}

			require.NoError(t, err)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID, test.tokenID)

			if test.metaData != nil {
				checkForTokenMetaData(t, sObj, test.assetID, test.tokenID, test.metaData)
			}
		})
	}
}

func TestSubBalance(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 2)
	tokenID := tests.GetRandomTokenID(t)
	tokenMetaData := &MetaData{static: map[string][]byte{"URL": []byte("https://moi.technology")}}
	WithAssetBalance(t, sObj, assetIDs[0], tokenID, big.NewInt(124), nil, tokenMetaData)
	WithAssetBalance(t, sObj, assetIDs[1], tokenID, big.NewInt(124), nil, tokenMetaData)

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		tokenID         common.TokenID
		amount          *big.Int
		expectedBalance *big.Int
		expectError     error
	}{
		{
			name:            "balance gets decremented if asset already exists",
			assetID:         assetIDs[0],
			tokenID:         tokenID,
			amount:          big.NewInt(123),
			expectedBalance: big.NewInt(1),
			expectError:     nil,
		},
		{
			name:            "should return error if asset doesn't exist",
			assetID:         tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:         tokenID,
			amount:          big.NewInt(50),
			expectedBalance: big.NewInt(0), // no balance set
			expectError:     common.ErrAssetNotFound,
		},
		{
			name:            "should return error if token doesn't exist",
			assetID:         assetIDs[1],
			tokenID:         tests.GetRandomTokenID(t),
			amount:          big.NewInt(50),
			expectedBalance: big.NewInt(0), // no balance set
			expectError:     common.ErrTokenNotFound,
		},
		{
			name:            "should delete token meta data if balance is zero",
			assetID:         tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:         tokenID,
			amount:          big.NewInt(124),
			expectedBalance: big.NewInt(0), // no balance set
			expectError:     common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			metaData, err := sObj.SubBalance(test.assetID, test.tokenID, test.amount)

			if test.expectError != nil {
				require.Error(t, err)
				require.Equal(t, err, test.expectError)

				return
			}

			require.NoError(t, err)
			require.Equal(t, metaData, tokenMetaData)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID, test.tokenID)
		})
	}
}

func TestIsAccountRegistered(t *testing.T) {
	id := tests.RandomIdentifier(t)
	db := mockDB()

	logicTree, _ := createTestKramaHashTree(t,
		db,
		common.SargaAccountID,
		storage.Storage,
		nil,
		nil,
	)

	sObj := createTestStateObject(t, stateObjectParamsWithStorageTree(
		t,
		map[identifiers.Identifier]tree.MerkleTree{
			common.SargaLogicID.AsIdentifier(): logicTree,
		},
	))

	// Test case: Account not registered
	isRegistered, err := sObj.IsAccountRegistered(id)
	require.NoError(t, err)
	require.False(t, isRegistered)

	// Register the account
	err = sObj.AddAccountGenesisInfo(id, common.NilHash)
	require.NoError(t, err)

	// Test case: Account registered
	isRegistered, err = sObj.IsAccountRegistered(id)
	require.NoError(t, err)
	require.True(t, isRegistered)
}

func TestCreateLockup(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 3)

	setAssetLockups(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(50000)),
			tests.TokenWithoutExpiry(t, common.TokenID(1), big.NewInt(50000)),
			tests.TokenWithoutExpiry(t, common.TokenID(2), big.NewInt(50000)),
		},
		nil, nil,
	)

	testcases := []struct {
		name                 string
		assetID              identifiers.AssetID
		tokenID              common.TokenID
		beneficiary          identifiers.Identifier
		amount               *big.Int
		preTestFn            func(id identifiers.Identifier)
		expectedBalance      *big.Int
		expectedLockupAmount *big.Int
		expectedError        error
	}{
		{
			name:                 "creates a new lockup",
			assetID:              assetIDs[0],
			tokenID:              common.DefaultTokenID,
			beneficiary:          tests.RandomIdentifier(t),
			amount:               big.NewInt(5000),
			expectedBalance:      big.NewInt(45000),
			expectedLockupAmount: big.NewInt(5000),
		},
		{
			name:        "increments existing lockup balance",
			assetID:     assetIDs[1],
			tokenID:     common.TokenID(1),
			beneficiary: tests.RandomIdentifier(t),
			amount:      big.NewInt(5000),
			preTestFn: func(id identifiers.Identifier) {
				assert.NoError(
					t,
					sObj.CreateLockup(
						assetIDs[1],
						common.TokenID(1),
						id,
						big.NewInt(3000),
					),
				)
			},
			expectedBalance:      big.NewInt(42000),
			expectedLockupAmount: big.NewInt(8000),
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			beneficiary:   tests.RandomIdentifier(t),
			amount:        big.NewInt(5000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.beneficiary)
			}

			err := sObj.CreateLockup(test.assetID, test.tokenID, test.beneficiary, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)

			// Verify if the balance is debited
			checkForBalances(t, sObj, test.expectedBalance, test.assetID, test.tokenID)
			// Verify the updated lockup amount
			checkForLockups(t, sObj, test.assetID, test.tokenID, test.beneficiary, test.expectedLockupAmount)
		})
	}
}

func TestReleaseLockup(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 2)

	ids := tests.GetIdentifiers(t, 2)
	setAssetLockups(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(5000)),
			tests.TokenWithoutExpiry(t, common.TokenID(1), big.NewInt(1000)),
		},

		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(5000), 0),
			tests.TokenWithExpiry(t, common.TokenID(1), big.NewInt(1000), 0),
		},
	)

	testcases := []struct {
		name                 string
		assetID              identifiers.AssetID
		tokenID              common.TokenID
		operator             identifiers.Identifier
		amount               *big.Int
		expectedLockupAmount *big.Int
		expectedError        error
	}{
		{
			name:                 "successfully releases lockup",
			assetID:              assetIDs[0],
			tokenID:              common.DefaultTokenID,
			operator:             ids[0],
			amount:               big.NewInt(2000),
			expectedLockupAmount: big.NewInt(3000), // 5000 - 2000
		},
		{
			name:                 "remove lockup when balance reaches zero",
			assetID:              assetIDs[1],
			tokenID:              common.TokenID(1),
			operator:             ids[1],
			amount:               big.NewInt(1000),
			expectedLockupAmount: big.NewInt(0),
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			operator:      ids[0],
			amount:        big.NewInt(2000),
			expectedError: common.ErrAssetNotFound,
		},
		// TODO: Add test case for checking meta data
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err := sObj.ReleaseLockup(test.assetID, test.tokenID, test.operator, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)

			// Verify the updated lockup amount
			checkForLockups(t, sObj, test.assetID, test.tokenID, test.operator, test.expectedLockupAmount)
		})
	}
}

func TestGetLockup(t *testing.T) { //nolint
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 1)

	setAssetLockups(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(5000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(500), 0),
		},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		tokenID        common.TokenID
		id             identifiers.Identifier
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "retrieves existing lockup amount",
			assetID:        assetIDs[0],
			tokenID:        common.DefaultTokenID,
			id:             ids[0],
			expectedAmount: big.NewInt(500),
		},
		{
			name:          "should return error if asset doesn't exist",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if lockup doesn't exist",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			expectedError: common.ErrLockupNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			lockup, err := sObj.GetLockup(test.assetID, test.tokenID, test.id)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, lockup.Amount, test.expectedAmount)
		})
	}
}

func TestCreateMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	setAssetMandates(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(5000)),
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(1000)),
		},
		nil, nil,
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		tokenID        common.TokenID
		id             identifiers.Identifier
		amount         *big.Int
		preTestFn      func(id identifiers.Identifier)
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "creates a new mandate successfully",
			assetID:        assetIDs[0],
			tokenID:        common.DefaultTokenID,
			id:             tests.RandomIdentifier(t),
			amount:         big.NewInt(5000),
			expectedAmount: big.NewInt(5000),
		},
		{
			name:    "increments existing mandate amount",
			assetID: assetIDs[1],
			tokenID: common.DefaultTokenID,
			id:      tests.RandomIdentifier(t),
			amount:  big.NewInt(3000),
			preTestFn: func(id identifiers.Identifier) {
				assert.NoError(
					t,
					sObj.CreateMandate(
						assetIDs[1],
						common.DefaultTokenID,
						id,
						big.NewInt(1000),
						uint64(time.Now().Add(1*time.Hour).Unix()),
					),
				)
			},
			expectedAmount: big.NewInt(4000),
		},
		{
			name:          "should return error if asset does not exist",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			id:            tests.RandomIdentifier(t),
			amount:        big.NewInt(3000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.id)
			}

			err := sObj.CreateMandate(test.assetID, test.tokenID, test.id, test.amount, uint64(time.Now().Unix()))

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedError, err)

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.tokenID, test.id, test.expectedAmount)
		})
	}
}

func TestSubMandateAmount(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 2)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	setAssetMandates(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(50000)),
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(1000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(500), 0),
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(1000), 0),
		},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		tokenID        common.TokenID
		id             identifiers.Identifier
		amount         *big.Int
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "deducts mandate amount successfully",
			assetID:        assetIDs[0],
			tokenID:        common.DefaultTokenID,
			id:             ids[0],
			amount:         big.NewInt(200),
			expectedAmount: big.NewInt(300),
		},
		{
			name:           "remove mandate when balance reaches zero",
			assetID:        assetIDs[1],
			tokenID:        common.DefaultTokenID,
			id:             ids[1],
			amount:         big.NewInt(1000),
			expectedAmount: big.NewInt(0),
		},
		{
			name:          "fails to deduct from nonexistent mandate",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			amount:        big.NewInt(100),
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:          "fails to deduct from nonexistent asset",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			id:            ids[0],
			tokenID:       common.DefaultTokenID,
			amount:        big.NewInt(100),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.SubMandateBalance(test.assetID, test.tokenID, test.id, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.tokenID, test.id, test.expectedAmount)
		})
	}
}

func TestConsumeMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 2)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)

	setAssetMandates(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(2000)),
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(1000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(500), 0),
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(1000), 0),
		},
	)

	testcases := []struct {
		name            string
		assetID         identifiers.AssetID
		tokenID         common.TokenID
		id              identifiers.Identifier
		amount          *big.Int
		expectedMandate *big.Int
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:            "consumes mandate and balance successfully",
			assetID:         assetIDs[0],
			tokenID:         common.DefaultTokenID,
			id:              ids[0],
			amount:          big.NewInt(200),
			expectedMandate: big.NewInt(300),
			expectedBalance: big.NewInt(1800),
		},
		{
			name:          "fails to deduct mandate balance",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			amount:        big.NewInt(100),
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:          "fails to deduct asset balance",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:       common.DefaultTokenID,
			id:            ids[0],
			amount:        big.NewInt(100),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, err := sObj.ConsumeMandate(test.assetID, test.tokenID, test.id, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.tokenID, test.id, test.expectedMandate)
			checkForBalances(t, sObj, test.expectedBalance, test.assetID, test.tokenID)
		})
	}
}

func TestDeleteMandate(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 1)

	setAssetMandates(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(50000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(500), 0),
		},
	)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		tokenID       common.TokenID
		id            identifiers.Identifier
		expectedError error
	}{
		{
			name:    "deletes existing mandate successfully",
			assetID: assetIDs[0],
			tokenID: common.DefaultTokenID,
			id:      ids[0],
		},
		{
			name:          "fails to delete mandate for nonexistent asset",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.DeleteMandate(test.assetID, test.tokenID, test.id)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)
			checkForMandates(t, sObj, test.assetID, test.tokenID, test.id, big.NewInt(0))
		})
	}
}

func TestGetMandate(t *testing.T) { //nolint
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 1)

	setAssetMandates(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(50000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(500), 0),
		},
	)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		tokenID        common.TokenID
		id             identifiers.Identifier
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:           "valid mandate returns correct amount",
			assetID:        assetIDs[0],
			tokenID:        common.DefaultTokenID,
			id:             ids[0],
			expectedAmount: big.NewInt(500),
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if mandate not found",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			id:            tests.RandomIdentifier(t),
			expectedError: common.ErrMandateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			mandate, err := sObj.GetMandate(test.assetID, test.tokenID, test.id)

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

//nolint:dupl
func TestMandates(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)
	tokens := tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(1000), 0)

	// Set mandates for the test assets and ids
	setAssetMandates(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(10000)),
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(20000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tokens,
		},
	)

	testcases := []struct {
		name           string
		stateObject    *Object
		preTestFn      func(stateObject *Object)
		expectedResult []common.AssetMandateOrLockup
		expectedError  error
	}{
		{
			name:        "retrieves existing mandates successfully",
			stateObject: sObj,
			expectedResult: []common.AssetMandateOrLockup{
				{
					AssetID: assetIDs[0],
					ID:      ids[0],
					Amount:  tokens,
				},
				{
					AssetID: assetIDs[1],
					ID:      ids[0],
					Amount:  tokens,
				},
			},
		},
		{
			name:           "returns empty list when mandates doesn't exist",
			stateObject:    createTestStateObject(t, nil),
			expectedResult: []common.AssetMandateOrLockup{},
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

//nolint:dupl
func TestLockups(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	ids := tests.GetIdentifiers(t, 1)
	assetIDs, _ := getAssetIDsAndBalances(t, 2)
	tokens := tests.TokenWithExpiry(t, common.DefaultTokenID, big.NewInt(1500), 0)

	// Set lockups for the test assets and ids
	setAssetLockups(
		t, sObj, assetIDs, []map[common.TokenID]*big.Int{
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(14000)),
			tests.TokenWithoutExpiry(t, common.DefaultTokenID, big.NewInt(22000)),
		},
		ids, []map[common.TokenID]*common.AmountWithExpiry{
			tokens,
		},
	)

	testcases := []struct {
		name           string
		stateObject    *Object
		preTestFn      func(stateObject *Object)
		expectedResult []common.AssetMandateOrLockup
		expectedError  error
	}{
		{
			name:        "retrieves existing lockups successfully",
			stateObject: sObj,
			expectedResult: []common.AssetMandateOrLockup{
				{
					AssetID: assetIDs[0],
					ID:      ids[0],
					Amount:  tokens,
				},
				{
					AssetID: assetIDs[1],
					ID:      ids[0],
					Amount:  tokens,
				},
			},
		},
		{
			name:           "returns empty list when lockups doesn't exist",
			stateObject:    createTestStateObject(t, nil),
			expectedResult: []common.AssetMandateOrLockup{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			lockups, err := test.stateObject.Lockups()

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, test.expectedResult, lockups)
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
		map[identifiers.Identifier]struct{}{tests.RandomIdentifier(t): {}},
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
	obj, _ := getMetaContextObject(t)

	sObj.metaContext = obj

	_, err := sObj.commitContextObject()
	require.NoError(t, err)

	checkForContextObject(t, sObj, obj)
}

func TestCommitActiveStorageTrees(t *testing.T) {
	logicIDs := tests.GetLogicIDs(t, 1)

	keys, values := getEntries(t, 1)
	newKeys, newValues := getEntries(t, 2)

	testcases := []struct {
		name            string
		storageTxns     map[identifiers.Identifier]*iradix.Txn
		storageTrees    map[identifiers.Identifier]tree.MerkleTree
		metaStorageTree *MockMerkleTree
		expectedError   error
	}{
		{
			name: "commit active storage trees successfully",
			storageTxns: map[identifiers.Identifier]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			storageTrees: map[identifiers.Identifier]tree.MerkleTree{
				logicIDs[0]: getMerkleTreeWithEntries(t, keys, values),
			},
			metaStorageTree: mockMerkleTreeWithDB(),
		},
		{
			name: "should return error if storage tree doesn't exists",
			storageTxns: map[identifiers.Identifier]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			metaStorageTree: mockMerkleTreeWithDB(),
			expectedError:   common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "should return error if fail to commit storage tree",
			storageTxns: map[identifiers.Identifier]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			storageTrees: map[identifiers.Identifier]tree.MerkleTree{
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
			storageTxns: map[identifiers.Identifier]*iradix.Txn{
				logicIDs[0]: getTxnsWithEntries(t, newKeys, newValues),
			},
			storageTrees: map[identifiers.Identifier]tree.MerkleTree{
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
				stateObjectParamsWithMST(t, identifiers.Nil, nil, test.mst, test.storageRoot),
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
					so.storageTreeTxns = map[identifiers.Identifier]*iradix.Txn{
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
					so.storageTreeTxns = map[identifiers.Identifier]*iradix.Txn{
						logicIds[0]: getTxnsWithEntries(t, keys, values),
					}
					so.storageTrees = map[identifiers.Identifier]tree.MerkleTree{
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
					so.storageTrees = map[identifiers.Identifier]tree.MerkleTree{
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
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
					identifiers.Nil,
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
	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
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
					identifiers.Nil,
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

	mObj, mHash := getMetaContextObjects(t, 1)

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
				identifiers.Nil,
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
					// we set this to check if the context object is committed
					so.metaContext = mObj[0]
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

			inputAcc := getAccount(t, nil, mHash[0], mstWithDirtyEntries, assetTree, logicTree)

			checkForAccount(t, sObj, inputAcc, actualAccHash, 2)
		})
	}
}

func getAccount(
	t *testing.T,
	reg *Deeds,
	metaContextHash common.Hash,
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
	acc.ContextHash = metaContextHash

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
				stateObjectParamsWithAssetTree(t, identifiers.Nil, nil, test.assetTree, common.NilHash, nil),
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
				stateObjectParamsWithLogicTree(t, identifiers.Nil, nil, test.logicTree, common.NilHash, nil),
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	assetObject := NewAssetObject(&common.AssetDescriptor{AssetID: assetID})

	SetAssetObject(t, sObj, assetID, assetObject)

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
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
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
	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
	logicObject := NewLogicObject(logicID, &engineio.LogicDescriptor{Engine: engineio.PISA})

	setLogicObject(t, sObj, logicID, logicObject)

	testcases := []struct {
		name          string
		logicID       identifiers.Identifier
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
			logicID:       tests.GetLogicID(t, tests.RandomIdentifier(t)),
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
		ast           map[identifiers.Identifier]tree.MerkleTree
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
	assetAddress1 := tests.RandomIdentifier(t)
	assetAddress2 := tests.RandomIdentifier(t)

	sObj := createTestStateObject(t, nil)

	assetDescriptor := getTestAssetDescriptor(t, sObj.id, 1000, "btc", 0)

	// assetID := tests.GetTestAssetIDFromAssetDescriptor(t, assetAddress2, assetDescriptor)

	setAssetState(t, sObj, assetDescriptor.AssetID, assetDescriptor)

	testcases := []struct {
		name            string
		assetAddress    identifiers.Identifier
		assetDescriptor *common.AssetDescriptor
		expectedError   error
	}{
		{
			name:            "asset created successfully",
			assetAddress:    assetAddress1,
			assetDescriptor: getTestAssetDescriptor(t, sObj.id, 1000, "moi", 0),
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
			err := sObj.CreateAsset(test.assetAddress, test.assetDescriptor)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			CheckAssetCreation(t, sObj, test.assetDescriptor)
		})
	}
}

//nolint:dupl
func TestMintAsset(t *testing.T) {
	sObj := createTestStateObject(t, nil)

	assetDescriptor := getTestAssetDescriptor(t, sObj.id, 10000, "btc", 0)

	setAssetState(t, sObj, assetDescriptor.AssetID, assetDescriptor)

	testcases := []struct {
		name                      string
		assetID                   identifiers.AssetID
		amount                    *big.Int
		expectedCirculatingSupply *big.Int
		expectedError             error
	}{
		{
			name:                      "asset minted successfully",
			assetID:                   assetDescriptor.AssetID,
			amount:                    big.NewInt(1000),
			expectedCirculatingSupply: big.NewInt(11000),
		},
		{
			name:          "should return error if asset doesn't exists",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(2000),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			circulatingSupply, err := sObj.MintAsset(test.assetID, test.amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, test.expectedError, err.Error())

				return
			}

			require.NoError(t, err)

			require.Equal(t, test.expectedCirculatingSupply, circulatingSupply)
		})
	}
}

//nolint:dupl
func TestBurnAsset(t *testing.T) {
	sObj := createTestStateObject(t, nil)

	assetDescriptor := getTestAssetDescriptor(t, sObj.id, 10000, "eth", 0)

	setAssetState(t, sObj, assetDescriptor.AssetID, assetDescriptor)

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name:           "asset burned successfully",
			assetID:        assetDescriptor.AssetID,
			amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(9000),
		},
		{
			name:          "should return error if asset doesn't exists",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
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
			require.Equal(t, test.expectedSupply, supply)
		})
	}
}

func TestSetMetadata(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 2)

	// Set up assets with initial balances
	WithAssetBalance(t, sObj, assetIDs[0], common.DefaultTokenID, big.NewInt(1000), &MetaData{static: map[string][]byte{
		"symbol": []byte("BTC"),
	}, dynamic: nil}, nil)
	WithAssetBalance(t, sObj, assetIDs[1], common.DefaultTokenID, big.NewInt(2000), nil, nil)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		isStatic      bool
		key           string
		value         []byte
		expectedError error
	}{
		{
			name:     "successfully set static metadata",
			assetID:  assetIDs[0],
			isStatic: true,
			key:      "name",
			value:    []byte("Bitcoin"),
		},
		{
			name:     "successfully set dynamic metadata",
			assetID:  assetIDs[0],
			isStatic: false,
			key:      "price",
			value:    []byte("50000"),
		},
		{
			name:          "should return error for updating static metadata",
			assetID:       assetIDs[0],
			isStatic:      true,
			key:           "symbol",
			value:         []byte("ETH"),
			expectedError: common.ErrKeyExists,
		},
		{
			name:     "successfully update existing dynamic metadata",
			assetID:  assetIDs[1],
			isStatic: false,
			key:      "volume",
			value:    []byte("1000000"),
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			isStatic:      true,
			key:           "name",
			value:         []byte("Unknown"),
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.SetAssetMetadata(test.assetID, test.isStatic, test.key, test.value)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// Verify metadata was set correctly
			assetObject, err := sObj.getAssetObject(test.assetID, true)
			require.NoError(t, err)

			if test.isStatic {
				require.NotNil(t, assetObject.Properties.StaticMetaData)
				val, ok := assetObject.Properties.StaticMetaData[test.key]
				require.True(t, ok)
				require.Equal(t, test.value, val)
			} else {
				require.NotNil(t, assetObject.Properties.DynamicMetaData)
				val, ok := assetObject.Properties.DynamicMetaData[test.key]
				require.True(t, ok)
				require.Equal(t, test.value, val)
			}
		})
	}
}

func TestGetAssetMetadata(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 2)

	// Set up assets with metadata
	WithAssetBalance(t, sObj, assetIDs[0], common.DefaultTokenID, big.NewInt(1000), &MetaData{
		static: map[string][]byte{
			"symbol": []byte("BTC"),
			"name":   []byte("Bitcoin"),
		},
		dynamic: map[string][]byte{
			"price":  []byte("50000"),
			"volume": []byte("1000000"),
		},
	}, nil)
	WithAssetBalance(t, sObj, assetIDs[1], common.DefaultTokenID, big.NewInt(2000), nil, nil)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		isStatic      bool
		key           string
		expectedValue []byte
		expectedError error
	}{
		{
			name:          "successfully get static metadata",
			assetID:       assetIDs[0],
			isStatic:      true,
			key:           "symbol",
			expectedValue: []byte("BTC"),
		},
		{
			name:          "successfully get dynamic metadata",
			assetID:       assetIDs[0],
			isStatic:      false,
			key:           "price",
			expectedValue: []byte("50000"),
		},
		{
			name:          "should return error if static key not found",
			assetID:       assetIDs[0],
			isStatic:      true,
			key:           "nonexistent",
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:          "should return error if dynamic key not found",
			assetID:       assetIDs[0],
			isStatic:      false,
			key:           "nonexistent",
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			isStatic:      true,
			key:           "symbol",
			expectedError: common.ErrAssetNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := sObj.GetAssetMetadata(test.assetID, test.isStatic, test.key)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestSetTokenMetadata(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 4)

	// Set up assets with initial balances and tokens
	WithAssetBalance(t, sObj, assetIDs[0], common.DefaultTokenID, big.NewInt(1000), nil, nil)
	WithAssetBalance(t, sObj, assetIDs[1], common.TokenID(1), big.NewInt(500), nil, nil)
	WithAssetBalance(t, sObj, assetIDs[2], common.DefaultTokenID, big.NewInt(2000), nil, &MetaData{
		static: map[string][]byte{
			"symbol": []byte("ETH"),
		},
	})
	WithAssetBalance(t, sObj, assetIDs[3], common.DefaultTokenID, big.NewInt(3000), nil, nil)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		tokenID       common.TokenID
		isStatic      bool
		key           string
		value         []byte
		expectedError error
	}{
		{
			name:     "successfully set static token metadata",
			assetID:  assetIDs[0],
			tokenID:  common.DefaultTokenID,
			isStatic: true,
			key:      "name",
			value:    []byte("Bitcoin Token"),
		},
		{
			name:     "successfully set dynamic token metadata",
			assetID:  assetIDs[0],
			tokenID:  common.DefaultTokenID,
			isStatic: false,
			key:      "price",
			value:    []byte("60000"),
		},
		{
			name:     "successfully set metadata for non-default token",
			assetID:  assetIDs[1],
			tokenID:  common.TokenID(1),
			isStatic: true,
			key:      "rarity",
			value:    []byte("rare"),
		},
		{
			name:          "successfully update existing static token metadata",
			assetID:       assetIDs[2],
			tokenID:       common.DefaultTokenID,
			isStatic:      true,
			key:           "symbol",
			value:         []byte("ETH"),
			expectedError: common.ErrKeyExists,
		},
		{
			name:     "successfully update existing dynamic token metadata",
			assetID:  assetIDs[3],
			tokenID:  common.DefaultTokenID,
			isStatic: false,
			key:      "volume",
			value:    []byte("2000000"),
		},
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:       common.DefaultTokenID,
			isStatic:      true,
			key:           "name",
			value:         []byte("Unknown"),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if token not found",
			assetID:       assetIDs[0],
			tokenID:       tests.GetRandomTokenID(t),
			isStatic:      true,
			key:           "name",
			value:         []byte("Unknown Token"),
			expectedError: common.ErrTokenNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.SetTokenMetadata(test.assetID, test.tokenID, test.isStatic, test.key, test.value)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			// Verify token metadata was set correctly
			assetObject, err := sObj.getAssetObject(test.assetID, true)
			require.NoError(t, err)

			tokenMetadata, ok := assetObject.TokenMetaData[test.tokenID]
			require.True(t, ok, "token metadata should exist for token ID %d", test.tokenID)
			require.NotNil(t, tokenMetadata)

			if test.isStatic {
				val, ok := tokenMetadata.static[test.key]
				require.True(t, ok, "static metadata key '%s' should exist", test.key)
				require.Equal(t, test.value, val)
			} else {
				val, ok := tokenMetadata.dynamic[test.key]
				require.True(t, ok, "dynamic metadata key '%s' should exist", test.key)
				require.Equal(t, test.value, val)
			}
		})
	}
}

func TestGetTokenMetadata(t *testing.T) {
	sObj := createTestStateObject(t, nil)
	assetIDs, _ := tests.CreateTestAssets(t, 3)

	// Set up test assets with various metadata configurations
	// Asset 0: Has both static and dynamic metadata
	dynamicMetadata := &MetaData{
		static: map[string][]byte{
			"name":   []byte("Bitcoin Token"),
			"symbol": []byte("BTC"),
		},
		dynamic: map[string][]byte{
			"price":  []byte("60000"),
			"volume": []byte("2000000"),
		},
	}
	WithAssetBalance(t, sObj, assetIDs[0], common.DefaultTokenID, big.NewInt(1000), nil, dynamicMetadata)

	// Asset 1: Has only static metadata
	staticOnlyMetadata := &MetaData{
		static: map[string][]byte{
			"rarity": []byte("legendary"),
			"tier":   []byte("gold"),
		},
		dynamic: map[string][]byte{},
	}
	WithAssetBalance(t, sObj, assetIDs[1], common.TokenID(1), big.NewInt(500), nil, staticOnlyMetadata)

	// Asset 2: Has balance but no metadata keys
	emptyMetadata := &MetaData{
		static:  map[string][]byte{},
		dynamic: map[string][]byte{},
	}
	WithAssetBalance(t, sObj, assetIDs[2], common.DefaultTokenID, big.NewInt(2000), nil, emptyMetadata)

	testcases := []struct {
		name          string
		assetID       identifiers.AssetID
		tokenID       common.TokenID
		isStatic      bool
		key           string
		expectedValue []byte
		expectedError error
	}{
		{
			name:          "should return error if asset not found",
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			tokenID:       common.DefaultTokenID,
			isStatic:      true,
			key:           "name",
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "should return error if token not found",
			assetID:       assetIDs[0],
			tokenID:       tests.GetRandomTokenID(t),
			isStatic:      true,
			key:           "name",
			expectedError: common.ErrTokenNotFound,
		},
		{
			name:          "successfully retrieve static metadata",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			isStatic:      true,
			key:           "name",
			expectedValue: []byte("Bitcoin Token"),
		},
		{
			name:          "successfully retrieve another static metadata key",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			isStatic:      true,
			key:           "symbol",
			expectedValue: []byte("BTC"),
		},
		{
			name:          "successfully retrieve dynamic metadata",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			isStatic:      false,
			key:           "price",
			expectedValue: []byte("60000"),
		},
		{
			name:          "successfully retrieve static metadata from non-default token",
			assetID:       assetIDs[1],
			tokenID:       common.TokenID(1),
			isStatic:      true,
			key:           "rarity",
			expectedValue: []byte("legendary"),
		},
		{
			name:          "should return error if static metadata key not found",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			isStatic:      true,
			key:           "nonexistent",
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:          "should return error if dynamic metadata key not found",
			assetID:       assetIDs[0],
			tokenID:       common.DefaultTokenID,
			isStatic:      false,
			key:           "nonexistent",
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := sObj.GetTokenMetadata(test.assetID, test.tokenID, test.isStatic, test.key)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, test.expectedError)
				require.Nil(t, value)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, value)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestGetMetaContextObjectCopy(t *testing.T) {
	mObj, mHash := getMetaContextObjects(t, 2)

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
			require.Equal(t, test.expectedObject.ComputeContext, actualObj.ComputeContext)
			require.Equal(t, test.expectedObject.DefaultMTQ, actualObj.DefaultMTQ)
		})
	}
}

func TestCreateContext(t *testing.T) {
	sObj := createTestStateObject(t, nil)

	testcases := []struct {
		name           string
		consensusNodes []identifiers.KramaID
		expectedError  error
	}{
		{
			name:           "created context successfully",
			consensusNodes: tests.RandomKramaIDs(t, 2),
			expectedError:  nil,
		},
		{
			name:           "should return error if failed to create context",
			consensusNodes: tests.RandomKramaIDs(t, 0),
			expectedError:  errors.New("liveliness size not met"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := sObj.CreateContext(test.consensusNodes)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForConsensusNodesUpdate(t, sObj, nil, test.consensusNodes)
		})
	}
}

func TestUpdateContext(t *testing.T) {
	mObj, mHash := getMetaContextObjects(t, 1)
	consensusNodes := tests.RandomKramaIDs(t, 2)

	testcases := []struct {
		name           string
		consensusNodes []identifiers.KramaID
		metaHash       common.Hash
		soParams       *createStateObjectParams
		mCtx           *MetaContextObject
		shouldUpdate   bool
		expectedError  error
	}{
		{
			name:           "should return error if meta context object doesn't exist",
			consensusNodes: consensusNodes,
			metaHash:       tests.RandomHash(t),
			expectedError:  errors.New("failed to fetch meta context object"),
		},
		{
			name:           "context updated successfully",
			consensusNodes: consensusNodes,
			metaHash:       mHash[0],
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					insertMetaContextsInDB(t, so.db, mObj...)
					insertContextHash(so, mHash[0])
				},
			},
			mCtx:         mObj[0],
			shouldUpdate: true,
		},
		{
			name:           "context should not be updated for sub account",
			consensusNodes: consensusNodes,
			soParams: &createStateObjectParams{
				id: tests.RandomSubAccountIdentifier(t, 1),
			},
		},
		{
			name:           "empty consensus nodes",
			consensusNodes: consensusNodes[0:0],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			insertContextHash(sObj, test.metaHash)

			err := sObj.UpdateContext(test.consensusNodes)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if !test.shouldUpdate {
				require.Nil(t, sObj.metaContext)

				return
			}

			checkForConsensusNodesUpdate(t,
				sObj,
				mObj[0],
				test.consensusNodes,
			)
		})
	}
}

func TestLoadKeys(t *testing.T) {
	accountKeys, accountKeysHash := getTestAccountKeys(t, 2)

	testcases := []struct {
		name            string
		soParams        *createStateObjectParams
		expectedAccKeys common.AccountKeys
		expectedError   error
	}{
		{
			name: "loaded acc keys successfully",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					insertAccKeysInDB(t, so.db, accountKeysHash, accountKeys)
					so.data.KeysHash = accountKeysHash
				},
			},
			expectedAccKeys: accountKeys,
		},
		{
			name: "should return error if failed to load acc keys",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.data.KeysHash = tests.RandomHash(t)
				},
			},
			expectedError: common.ErrKeyNotFound,
		},
		{
			name:            "should load empty acc keys object for nil acc keys hash",
			expectedAccKeys: make(common.AccountKeys, 0),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)
			err := sObj.loadKeys()

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccKeys, sObj.keys)
		})
	}
}

func TestRevokeAccountKeys(t *testing.T) {
	accountKeys, accountKeysHash := getTestAccountKeys(t, 3)

	testcases := []struct {
		name           string
		soParams       *createStateObjectParams
		revokePayload  []common.KeyRevokePayload
		revokedKeys    []int
		nonRevokedKeys []int
	}{
		{
			name: "revoke account keys",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					insertAccKeysInDB(t, so.db, accountKeysHash, accountKeys)
					so.data.KeysHash = accountKeysHash
				},
			},
			revokePayload: []common.KeyRevokePayload{
				{
					KeyID: accountKeys[0].ID,
				},
				{
					KeyID: accountKeys[2].ID,
				},
			},
			revokedKeys:    []int{0, 2},
			nonRevokedKeys: []int{1},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)
			err := sObj.RevokeAccountKeys(test.revokePayload)
			require.NoError(t, err)

			for _, id := range test.revokedKeys {
				require.True(t, sObj.keys[id].Revoked)
			}

			for _, id := range test.nonRevokedKeys {
				require.False(t, sObj.keys[id].Revoked)
			}
		})
	}
}

func TestLoadDeedsObject(t *testing.T) {
	deeds, deedsHash := getTestDeeds(t, map[identifiers.Identifier]struct{}{
		tests.RandomIdentifier(t): {},
		tests.RandomIdentifier(t): {},
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
				Entries: make(map[identifiers.Identifier]struct{}),
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
					WithAssetBalance(t,
						so, common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(1000), nil, nil)
				},
			},
			hasFuel: true,
		},
		{
			name:   "insufficient fuel",
			amount: big.NewInt(200),
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					WithAssetBalance(t, so, common.KMOITokenAssetID, common.DefaultTokenID, big.NewInt(100), nil, nil)
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
			name:          "failed to load balance",
			amount:        big.NewInt(200),
			hasFuel:       false,
			expectedError: common.ErrAssetNotFound,
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
	id := tests.RandomIdentifier(t)
	storageTree, storageRoot := createTestKramaHashTree(t,
		db,
		id,
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
			soParams: stateObjectParamsWithMST(t, id, db, nil, storageRoot),
		},
		{
			name:     "fetch meta storage tree from cache",
			soParams: stateObjectParamsWithMST(t, id, db, storageTree, common.NilHash),
		},
		{
			name:          "should return error if failed to initiate storage tree",
			soParams:      stateObjectParamsWithMST(t, id, db, nil, tests.RandomHash(t)),
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
	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
	keys, values := getEntries(t, 2)

	testcases := []struct {
		name          string
		soParams      *createStateObjectParams
		logicID       identifiers.Identifier
		expectedError error
	}{
		{
			name:     "fetched storage tree from active storage trees",
			soParams: stateObjectParamsWithStorageTree(t, getStorageTrees(t, []identifiers.Identifier{logicID}, keys, values)),
			logicID:  logicID,
		},
		{
			name:          "should return error if failed to get meta storage tree",
			soParams:      stateObjectParamsWithInvalidMST(t),
			logicID:       tests.GetLogicID(t, tests.RandomIdentifier(t)),
			expectedError: errors.New("failed to initiate storage tree"),
		},
		{
			name:          "should return error if logic storage tree not found",
			logicID:       tests.GetLogicID(t, tests.RandomIdentifier(t)),
			expectedError: common.ErrLogicStorageTreeNotFound,
		},
		{
			name: "fetched storage tree from db",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.metaStorageTree, _ = createMetaStorageTree(
						t,
						so.db,
						so.id,
						common.SargaLogicID,
						keys,
						values,
					)
				},
			},
			logicID: common.SargaLogicID.AsIdentifier(),
		},
		{
			name: "should return error if failed to initiate logic storage tree",
			soParams: &createStateObjectParams{
				soCallback: func(so *Object) {
					so.metaStorageTree, _ = createTestKramaHashTree(
						t,
						so.db,
						so.id,
						storage.Storage,
						[][]byte{common.SargaLogicID.Bytes()},
						[][]byte{tests.RandomHash(t).Bytes()},
					)
				},
			},
			logicID:       common.SargaLogicID.AsIdentifier(),
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
			name:     "should create tree if storage tree is not initiated",
			soParams: stateObjectParamsWithASTAndMST(t, nil, mockMerkleTreeWithDB()),
		},
		{
			name: "should add storage entry",
			soParams: stateObjectParamsWithASTAndMST(
				t,
				map[identifiers.Identifier]tree.MerkleTree{
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
		id            identifiers.Identifier
		ixHash        common.Hash
		soParams      *createStateObjectParams
		expectedError error
	}{
		{
			name:   "should succeed if account genesis info added",
			id:     tests.RandomIdentifier(t),
			ixHash: tests.RandomHash(t),
			soParams: stateObjectParamsWithStorageTree(t, getStorageTrees(
				t, []identifiers.Identifier{common.SargaLogicID.AsIdentifier()}, keys, values,
			)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(t, test.soParams)

			err := sObj.AddAccountGenesisInfo(test.id, test.ixHash)
			require.NoError(t, err)

			accInfo := common.AccountGenesisInfo{
				IxHash: test.ixHash,
			}

			expectedValue, err := accInfo.Bytes()
			require.NoError(t, err)

			actualValue, err := sObj.GetStorageEntry(common.SargaLogicID.AsIdentifier(), test.id.Bytes())
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
	id := tests.RandomIdentifier(t)
	logicTree, logicRoot := createTestKramaHashTree(t,
		db,
		id,
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
			soParams: stateObjectParamsWithLogicTree(t, id, db, nil, logicRoot, nil),
		},
		{
			name:     "fetch logic tree from cache",
			soParams: stateObjectParamsWithLogicTree(t, id, db, logicTree, common.NilHash, nil),
		},
		{
			name:          "should return error if failed to initiate logic tree",
			soParams:      stateObjectParamsWithLogicTree(t, id, db, nil, tests.RandomHash(t), nil),
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
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
				identifiers.Nil,
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
				identifiers.Nil,
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
						so.id,
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
	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
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
				identifiers.Nil,
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
				identifiers.Nil,
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
						so.id,
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
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
					identifiers.Nil,
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

	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
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
					identifiers.Nil,
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
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
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
			assetID:   tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithAssetTree(
					t,
					identifiers.Nil,
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

	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))
	logicObject := createLogicObject(t, getLogicObjectParamsWithLogicID(logicID))
	rawData, err := logicObject.Bytes()
	require.NoError(t, err)

	testcases := []struct {
		name          string
		logicTree     tree.MerkleTree
		logicRoot     common.Hash
		logicID       identifiers.Identifier
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
			logicID:   tests.GetLogicID(t, tests.RandomIdentifier(t)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sObj := createTestStateObject(
				t,
				stateObjectParamsWithLogicTree(
					t,
					identifiers.Nil,
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
	logicID := tests.GetLogicID(t, tests.RandomIdentifier(t))

	testcases := []struct {
		name          string
		logicID       identifiers.Identifier
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

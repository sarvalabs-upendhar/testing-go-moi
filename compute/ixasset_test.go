package compute

import (
	"math/big"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_CreateAsset(t *testing.T) {
	testcases := []struct {
		name          string
		payload       *common.AssetCreatePayload
		preTestFn     func(assetID identifiers.AssetID, creatorAcc *state.Object, payload *common.AssetCreatePayload)
		expectedError error
	}{
		{
			name: "asset created successfully",
			payload: &common.AssetCreatePayload{
				Symbol:   "MOI",
				Supply:   big.NewInt(5000),
				Standard: common.MAS0,
			},
		},
		{
			name: "asset already exists in asset account",
			payload: &common.AssetCreatePayload{
				Symbol:   "ETH",
				Supply:   big.NewInt(500),
				Standard: common.MAS0,
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc *state.Object, payload *common.AssetCreatePayload) {
				insertTestAssetObject(t, assetID, creatorAcc, state.NewAssetObject(payload.Supply, nil))
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name: "asset already exists in creator account",
			payload: &common.AssetCreatePayload{
				Symbol:   "ETH",
				Supply:   big.NewInt(500),
				Standard: common.MAS0,
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc *state.Object, payload *common.AssetCreatePayload) {
				insertTestAssetObject(t, assetID, creatorAcc, state.NewAssetObject(payload.Supply, nil))
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name: "asset already exists in deeds registry",
			payload: &common.AssetCreatePayload{
				Symbol:   "BTC",
				Supply:   big.NewInt(1000),
				Standard: common.MAS0,
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc *state.Object, payload *common.AssetCreatePayload) {
				createTestDeedsEntry(t, assetID.Address(), creatorAcc, payload)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			assetObject := createTestStateObject(t)
			creatorObject := createTestStateObject(t)
			assetID := createTestAssetID(t, assetObject.Address(), test.payload)

			if test.preTestFn != nil {
				test.preTestFn(assetID, creatorObject, test.payload)
			}

			assetID, err := createAsset(creatorObject, assetObject, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetCreate(t, assetID, creatorObject, assetObject, test.payload)
		})
	}
}

func Test_TransferAsset(t *testing.T) {
	creator0, _, assetID0 := createTestAsset(t, big.NewInt(3000))
	creator1, _, assetID1 := createTestAsset(t, big.NewInt(5000))

	testcases := []struct {
		name                  string
		sender                *state.Object
		payload               *common.AssetActionPayload
		preTestFn             func(assetID identifiers.AssetID, target *state.Object)
		expectedSenderBalance *big.Int
		expectedTargetBalance *big.Int
		expectedError         error
	}{
		{
			name:   "asset not found",
			sender: creator1,
			payload: &common.AssetActionPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "initialize asset balance if asset doesn't exist",
			sender: creator0,
			payload: &common.AssetActionPayload{
				AssetID: assetID0,
				Amount:  big.NewInt(3000),
			},
			expectedSenderBalance: big.NewInt(0),
			expectedTargetBalance: big.NewInt(3000),
		},
		{
			name:   "asset balance incremented successfully",
			sender: creator1,
			payload: &common.AssetActionPayload{
				AssetID: assetID1,
				Amount:  big.NewInt(1000),
			},
			preTestFn: func(assetID identifiers.AssetID, target *state.Object) {
				insertTestAssetObject(t, assetID, target, state.NewAssetObject(big.NewInt(500), nil))
			},
			expectedSenderBalance: big.NewInt(4000),
			expectedTargetBalance: big.NewInt(1500),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := createTestStateObject(t)

			if test.preTestFn != nil {
				test.preTestFn(test.payload.AssetID, target)
			}

			err := transferAsset(test.sender, target, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetTransfer(
				t, test.payload.AssetID, test.sender, target,
				test.expectedSenderBalance, test.expectedTargetBalance,
			)
		})
	}
}

//nolint:dupl
func Test_MintAsset(t *testing.T) {
	creator, asset, assetID := createTestAsset(t, big.NewInt(5000))

	testcases := []struct {
		name           string
		payload        *common.AssetSupplyPayload
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name: "asset not found",
			payload: &common.AssetSupplyPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name: "asset minted successfully",
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(1000),
			},
			expectedSupply: big.NewInt(6000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			supply, err := mintAsset(creator, asset, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, &supply)
		})
	}
}

//nolint:dupl
func Test_BurnAsset(t *testing.T) {
	creator, asset, assetID := createTestAsset(t, big.NewInt(5000))

	testcases := []struct {
		name           string
		payload        *common.AssetSupplyPayload
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name: "asset not found",
			payload: &common.AssetSupplyPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name: "asset burned successfully",
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(1000),
			},
			expectedSupply: big.NewInt(4000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			supply, err := burnAsset(creator, asset, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, &supply)
		})
	}
}

package compute

import (
	"math/big"
	"testing"
	"time"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_ValidateAssetCreate(t *testing.T) {
	operator := createTestStateObject(t)
	assetAcc := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetAcc.Address())

	insertTestAssetObject(
		t, assetID, operator, state.NewAssetObject(big.NewInt(5000), nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetCreatePayload
		expectedError error
	}{
		{
			name:   "asset already registered",
			sender: operator,
			payload: &common.AssetCreatePayload{
				Standard:   common.MAS0,
				Dimension:  1,
				IsStateFul: true,
				IsLogical:  false,
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name:   "valid asset create operation",
			sender: operator,
			payload: &common.AssetCreatePayload{
				Standard:   common.MAS1,
				Dimension:  1,
				IsStateFul: true,
				IsLogical:  false,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetCreate(test.sender, assetAcc, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

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

func Test_ValidateAssetTransfer(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	sarga := createTestSargaStateObject(t)

	insertTestAssetObject(
		t, assetID, sender, state.NewAssetObject(big.NewInt(5000), nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetActionPayload
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:   "beneficiary not registered",
			sender: createTestStateObject(t),
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
			},
			expectedError: common.ErrBeneficiaryNotRegistered,
		},
		{
			name:   "asset not found",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:      big.NewInt(1000),
			},
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Address())
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient balance",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(7000),
			},
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Address())
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid asset transfer operation",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(4000),
			},
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Address())
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.payload.Beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetTransfer(test.sender, target, sarga, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_TransferAsset(t *testing.T) {
	sender0, _, assetID0 := createTestAsset(t, big.NewInt(3000))
	sender1, _, assetID1 := createTestAsset(t, big.NewInt(5000))

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
			sender: sender1,
			payload: &common.AssetActionPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "initialize asset balance if asset doesn't exist",
			sender: sender0,
			payload: &common.AssetActionPayload{
				AssetID: assetID0,
				Amount:  big.NewInt(3000),
			},
			expectedSenderBalance: big.NewInt(0),
			expectedTargetBalance: big.NewInt(3000),
		},
		{
			name:   "asset balance incremented successfully",
			sender: sender1,
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

func Test_ValidateMandateConsume(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	sarga := createTestSargaStateObject(t)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetActionPayload
		preTestFn     func(target, benefactor *state.Object)
		expectedError error
	}{
		{
			name:   "beneficiary not registered",
			sender: createTestStateObject(t),
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
			},
			expectedError: common.ErrBeneficiaryNotRegistered,
		},
		{
			name:   "asset not found",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:      big.NewInt(1000),
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Address())
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient balance",
			sender: sender,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(7000),
			},
			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, assetID, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
				registerParticipant(t, sarga, target.Address())
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "mandate not found",
			sender: sender,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(2000),
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Address())
				insertTestAssetObject(
					t, assetID, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
			},
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:   "mandate expired",
			sender: sender,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(2000),
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Address())
				insertTestAssetObject(
					t, assetID, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
				createTestMandate(t, benefactor, sender, assetID, big.NewInt(2500), time.Now().Add(-1*time.Hour).Unix())
			},
			expectedError: common.ErrMandateExpired,
		},
		{
			name:   "insufficient mandate funds",
			sender: sender,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(4000),
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Address())
				insertTestAssetObject(
					t, assetID, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
				createTestMandate(t, benefactor, sender, assetID, big.NewInt(1000), time.Now().Add(1*time.Hour).Unix())
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid asset transfer operation",
			sender: sender,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(4000),
			},
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Address())
				insertTestAssetObject(
					t, assetID, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
				createTestMandate(t, benefactor, sender, assetID, big.NewInt(4000), time.Now().Add(1*time.Hour).Unix())
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.payload.Beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)
			benefactor := state.NewStateObject(
				test.payload.Benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := validateAssetConsume(test.sender, target, sarga, benefactor, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_ConsumeMandate(t *testing.T) {
	sender0, _, assetID0 := createTestAsset(t, big.NewInt(3000))
	sender1, _, assetID1 := createTestAsset(t, big.NewInt(5000))

	testcases := []struct {
		name                      string
		sender                    *state.Object
		payload                   *common.AssetActionPayload
		preTestFn                 func(target, benefactor *state.Object)
		expectedBenefactorBalance *big.Int
		expectedTargetBalance     *big.Int
		expectedMandateBalance    *big.Int
		expectedError             error
	}{
		{
			name:   "asset not found",
			sender: sender0,
			payload: &common.AssetActionPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "initialize asset balance if asset doesn't exist",
			sender: sender1,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID1,
				Amount:      big.NewInt(1500),
			},
			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, assetID1, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
				createTestMandate(t, benefactor, sender1, assetID1, big.NewInt(2000), time.Now().Unix())
			},
			expectedBenefactorBalance: big.NewInt(3500),
			expectedTargetBalance:     big.NewInt(1500),
			expectedMandateBalance:    big.NewInt(500),
		},
		{
			name:   "asset mandate consumed successfully",
			sender: sender0,
			payload: &common.AssetActionPayload{
				Benefactor:  tests.RandomAddress(t),
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID0,
				Amount:      big.NewInt(1500),
			},
			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, assetID0, benefactor, state.NewAssetObject(big.NewInt(5000), nil),
				)
				insertTestAssetObject(
					t, assetID0, target, state.NewAssetObject(big.NewInt(3000), nil),
				)
				createTestMandate(t, benefactor, sender0, assetID0, big.NewInt(2000), time.Now().Unix())
			},
			expectedBenefactorBalance: big.NewInt(3500),
			expectedTargetBalance:     big.NewInt(4500),
			expectedMandateBalance:    big.NewInt(500),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := createTestStateObject(t)
			benefactor := createTestStateObject(t)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := consumeMandate(test.sender, target, benefactor, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkMandateConsumption(
				t, test.payload.AssetID, test.sender, target, benefactor, test.expectedBenefactorBalance,
				test.expectedTargetBalance, test.expectedMandateBalance,
			)
		})
	}
}

func Test_ValidateAssetMint(t *testing.T) {
	operator := createTestStateObject(t)
	assetAcc := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))

	insertTestAssetObject(
		t, assetID, operator, state.NewAssetObject(big.NewInt(5000), nil),
	)
	setupAssetAccount(t, operator, assetAcc, assetID)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetSupplyPayload
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: createTestStateObject(t),
			payload: &common.AssetSupplyPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "operator mismatch",
			sender: createTestStateObject(t),
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(7000),
			},
			expectedError: common.ErrOperatorMismatch,
		},
		{
			name:   "valid asset mint operation",
			sender: operator,
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(5000),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetMint(test.sender, assetAcc, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
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

func Test_ValidateAssetBurn(t *testing.T) {
	operator := createTestStateObject(t)
	assetAcc := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))

	insertTestAssetObject(
		t, assetID, operator, state.NewAssetObject(big.NewInt(5000), nil),
	)
	setupAssetAccount(t, operator, assetAcc, assetID)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetSupplyPayload
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: createTestStateObject(t),
			payload: &common.AssetSupplyPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "operator mismatch",
			sender: createTestStateObject(t),
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(7000),
			},
			expectedError: common.ErrOperatorMismatch,
		},
		{
			name:   "asset not found",
			sender: operator,
			payload: &common.AssetSupplyPayload{
				AssetID: tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:  big.NewInt(5000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient funds",
			sender: operator,
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(50000),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid asset burn operation",
			sender: operator,
			payload: &common.AssetSupplyPayload{
				AssetID: assetID,
				Amount:  big.NewInt(5000),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetBurn(test.sender, assetAcc, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
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

func Test_ValidateAssetApprove(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))

	insertTestAssetObject(
		t, assetID, sender, state.NewAssetObject(big.NewInt(5000), nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetActionPayload
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:      big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient balance",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(7000),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid asset approve operation",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(4000),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetApprove(test.sender, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetApprove(t *testing.T) {
	creator, _, assetID := createTestAsset(t, big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetActionPayload
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: creator,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:      big.NewInt(5000),
				Timestamp:   time.Now().Add(1 * time.Hour).Unix(),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "asset approved successfully",
			sender: creator,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
				Amount:      big.NewInt(5000),
				Timestamp:   time.Now().Add(1 * time.Hour).Unix(),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := approveAsset(test.sender, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetApprove(t, test.sender, test.payload)
		})
	}
}

func Test_ValidateAssetRevoke(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomAddress(t))
	sarga := createTestSargaStateObject(t)
	insertTestAssetObject(
		t, assetID, sender, state.NewAssetObject(big.NewInt(5000), nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetActionPayload
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:   "mandate not found",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
			},
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Address())
			},
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:   "valid asset revoke operation",
			sender: sender,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
			},
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Address())
				createTestMandate(t, sender, target, assetID, big.NewInt(2000), time.Now().Unix())
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.payload.Beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetRevoke(test.sender, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetRevoke(t *testing.T) {
	creator, _, assetID := createTestAsset(t, big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetActionPayload
		preTestFn     func(sender *state.Object, payload *common.AssetActionPayload)
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: creator,
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     tests.GetRandomAssetID(t, tests.RandomAddress(t)),
				Amount:      big.NewInt(5000),
				Timestamp:   time.Now().Add(1 * time.Hour).Unix(),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "asset revoked successfully",
			sender: creator,
			preTestFn: func(sender *state.Object, payload *common.AssetActionPayload) {
				createMandate(t, sender, &common.AssetActionPayload{
					Beneficiary: payload.Beneficiary,
					AssetID:     payload.AssetID,
					Amount:      big.NewInt(5000),
					Timestamp:   time.Now().Add(1 * time.Hour).Unix(),
				})
			},
			payload: &common.AssetActionPayload{
				Beneficiary: tests.RandomAddress(t),
				AssetID:     assetID,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.sender, test.payload)
			}

			err := revokeAsset(test.sender, test.payload)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetRevoke(t, test.sender, test.payload)
		})
	}
}

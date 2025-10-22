package compute

import (
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_ValidateAssetCreate(t *testing.T) {
	operator := createTestStateObject(t)
	assetAcc := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, assetAcc.Identifier())
	assetID2 := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	assetID3 := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	insertTestAssetObject(
		t, operator, assetID, state.NewAssetObject(nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.AssetCreatePayload
		assetID       identifiers.AssetID
		expectedError error
	}{
		{
			name:          "asset already registered",
			sender:        operator,
			assetID:       assetID,
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name:    "invalid asset standard",
			sender:  operator,
			assetID: assetID2,
			payload: &common.AssetCreatePayload{
				Standard: common.AssetStandard(500),
			},
			expectedError: common.ErrInvalidAssetStandard,
		},
		{
			name:    "invalid asset logic",
			sender:  operator,
			assetID: assetID3,
			payload: &common.AssetCreatePayload{
				Standard: common.MASX,
				Logic:    nil, // empty logic
			},
			expectedError: common.ErrEmptyManifest,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetCreate(test.sender, test.assetID, test.payload)
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
		descriptor    *common.AssetDescriptor
		preTestFn     func(assetID identifiers.AssetID, creatorAcc, assetAcc *state.Object, payload *common.AssetDescriptor)
		expectedError error
	}{
		{
			name: "asset created successfully",
			descriptor: &common.AssetDescriptor{
				AssetID:           tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Symbol:            "MOI",
				CirculatingSupply: big.NewInt(5000),
			},
		},
		{
			name: "asset already exists in asset account",
			descriptor: &common.AssetDescriptor{
				AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Symbol:  "ETH",
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc, assetAcc *state.Object, payload *common.AssetDescriptor) {
				insertTestAssetObject(t, assetAcc, assetID, state.NewAssetObject(payload))
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name: "asset already exists in deeds registry",
			descriptor: &common.AssetDescriptor{
				AssetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
				Symbol:  "ETH",
			},
			preTestFn: func(assetID identifiers.AssetID, creatorAcc, assetAcc *state.Object, payload *common.AssetDescriptor) {
				createTestDeedsEntry(t, creatorAcc, payload)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			assetObject := createTestStateObject(t)
			creatorObject := createTestStateObject(t)

			if test.preTestFn != nil {
				test.preTestFn(test.descriptor.AssetID, creatorObject, assetObject, test.descriptor)
			}

			assetID, err := createAsset(creatorObject, assetObject, test.descriptor)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetCreate(t, assetID, creatorObject, assetObject, test.descriptor)
		})
	}
}

func Test_ValidateAssetTransfer(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	insertTestAssetObject(
		t,
		sender,
		assetID,
		assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(50000)),
	)

	testcases := []struct {
		name          string
		benefactor    *state.Object
		assetID       identifiers.AssetID
		tokenID       common.TokenID
		amount        *big.Int
		beneficiary   identifiers.Identifier
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:          "beneficiary not registered",
			benefactor:    createTestStateObject(t),
			beneficiary:   tests.RandomIdentifier(t),
			amount:        big.NewInt(5000),
			expectedError: common.ErrBeneficiaryNotRegistered,
		},
		{
			name:        "asset not found",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(1000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "insufficient balance",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(70000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "valid asset transfer operation",
			benefactor:  sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetTransfer(test.benefactor, target, sarga, test.assetID, common.DefaultTokenID, test.amount)
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
	sender0, _, assetID0 := createTestAsset(t, big.NewInt(1000000), big.NewInt(3000))
	sender1, _, assetID1 := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name                  string
		sender                *state.Object
		assetID               identifiers.AssetID
		amount                *big.Int
		preTestFn             func(assetID identifiers.AssetID, target *state.Object)
		expectedSenderBalance *big.Int
		expectedTargetBalance *big.Int
		expectedError         error
	}{
		{
			name:          "asset not found",
			sender:        sender1,
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(1000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:                  "initialize asset balance if asset doesn't exist",
			sender:                sender0,
			amount:                big.NewInt(3000),
			assetID:               assetID0,
			expectedSenderBalance: big.NewInt(0),
			expectedTargetBalance: big.NewInt(3000),
		},
		{
			name:                  "asset balance incremented successfully",
			sender:                sender1,
			assetID:               assetID1,
			amount:                big.NewInt(1000),
			expectedSenderBalance: big.NewInt(4000),
			expectedTargetBalance: big.NewInt(1000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := createTestStateObject(t)

			err := transferAsset(test.sender, target, test.assetID, common.DefaultTokenID, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetTransfer(
				t, test.assetID, common.DefaultTokenID, test.sender, target,
				test.expectedSenderBalance, test.expectedTargetBalance,
			)
		})
	}
}

func Test_ValidateMandateConsume(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	testcases := []struct {
		name          string
		sender        *state.Object
		Beneficiary   identifiers.Identifier
		Benefactor    identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		preTestFn     func(target, benefactor *state.Object)
		expectedError error
	}{
		{
			name:          "beneficiary not registered",
			sender:        createTestStateObject(t),
			Beneficiary:   tests.RandomIdentifier(t),
			expectedError: common.ErrBeneficiaryNotRegistered,
		},
		{
			name:        "asset not found",
			sender:      sender,
			Beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(1000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "insufficient balance",
			sender:      sender,
			Benefactor:  tests.RandomIdentifier(t),
			Beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(7000),
			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "mandate not found",
			sender:      sender,
			Benefactor:  tests.RandomIdentifier(t),
			Beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),
			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
			},
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:        "mandate expired",
			sender:      sender,
			Benefactor:  tests.RandomIdentifier(t),
			Beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(2000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(2500), uint64(time.Now().Add(-1*time.Hour).Unix()),
				)
			},
			expectedError: common.ErrMandateExpired,
		},
		{
			name:   "insufficient mandate funds",
			sender: sender,

			Benefactor:  tests.RandomIdentifier(t),
			Beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender,
					assetID, common.DefaultTokenID,
					big.NewInt(1000), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "valid asset transfer operation",
			sender:      sender,
			Benefactor:  tests.RandomIdentifier(t),
			Beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor,
					sender, assetID, common.DefaultTokenID, big.NewInt(4000), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.Beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)
			benefactor := state.NewStateObject(
				test.Benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := validateAssetConsume(
				test.sender.Identifier(),
				target, benefactor, sarga,
				test.assetID, common.DefaultTokenID,
				test.amount)
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
	sender0, _, assetID0 := createTestAsset(t, big.NewInt(1000000), big.NewInt(3000))
	sender1, _, assetID1 := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name                      string
		sender                    *state.Object
		assetID                   identifiers.AssetID
		benefactor                identifiers.Identifier
		beneficiary               identifiers.Identifier
		amount                    *big.Int
		preTestFn                 func(target, benefactor *state.Object)
		expectedBenefactorBalance *big.Int
		expectedTargetBalance     *big.Int
		expectedMandateBalance    *big.Int
		expectedError             error
	}{
		{
			name:   "asset not found",
			sender: sender0,

			assetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:  big.NewInt(1000),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "initialize asset balance if asset doesn't exist",
			sender: sender1,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID1,
			amount:      big.NewInt(1500),

			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, benefactor, assetID1, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createTestMandate(
					t,
					benefactor, sender1,
					assetID1, common.DefaultTokenID,
					big.NewInt(2000),
					uint64(time.Now().Unix()))
			},
			expectedBenefactorBalance: big.NewInt(3500),
			expectedTargetBalance:     big.NewInt(1500),
			expectedMandateBalance:    big.NewInt(500),
		},
		{
			name:   "asset mandate consumed successfully",
			sender: sender0,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID0,
			amount:      big.NewInt(1500),

			preTestFn: func(target, benefactor *state.Object) {
				insertTestAssetObject(
					t, benefactor, assetID0, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				insertTestAssetObject(
					t, target, assetID0, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(3000)),
				)
				createTestMandate(
					t,
					benefactor, sender0,
					assetID0, common.DefaultTokenID,
					big.NewInt(2000), uint64(time.Now().Unix()))
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

			err := consumeMandate(
				test.sender.Identifier(),
				benefactor, target,
				test.assetID, common.DefaultTokenID,
				test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkMandateConsumption(
				t,
				test.assetID, common.DefaultTokenID,
				test.sender, target, benefactor, test.expectedBenefactorBalance,
				test.expectedTargetBalance, test.expectedMandateBalance,
			)
		})
	}
}

func Test_ValidateAssetMint(t *testing.T) {
	operator, assetAcc, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		assetID       identifiers.AssetID
		amount        *big.Int
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        createTestStateObject(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "manager mismatch",
			sender:        createTestStateObject(t),
			assetID:       assetID,
			amount:        big.NewInt(7000),
			expectedError: common.ErrManagerMismatch,
		},
		{
			name:    "valid asset mint operation",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(5000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetMint(test.sender.Identifier(), assetAcc, test.assetID, test.amount)
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
	creator, asset, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name           string
		AssetID        identifiers.AssetID
		Amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name: "asset not found",

			AssetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			Amount:        big.NewInt(1000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:           "asset minted successfully",
			AssetID:        assetID,
			Amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(6000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := mintAsset(creator, asset, test.AssetID, common.DefaultTokenID, test.Amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, getCirculatingSupply(t, asset, assetID))
		})
	}
}

func Test_ValidateAssetBurn(t *testing.T) {
	operator, assetAcc, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		assetID       identifiers.AssetID
		amount        *big.Int
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        createTestStateObject(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:          "operator mismatch",
			sender:        createTestStateObject(t),
			assetID:       assetID,
			amount:        big.NewInt(7000),
			expectedError: common.ErrManagerMismatch,
		},
		{
			name:          "asset not found",
			sender:        operator,
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(5000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient funds",
			sender: operator,

			assetID:       assetID,
			amount:        big.NewInt(50000),
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:    "valid asset burn operation",
			sender:  operator,
			assetID: assetID,
			amount:  big.NewInt(5000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetBurn(
				test.sender, assetAcc,
				test.assetID, common.DefaultTokenID,
				test.amount)
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
	creator, asset, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name           string
		assetID        identifiers.AssetID
		amount         *big.Int
		expectedSupply *big.Int
		expectedError  error
	}{
		{
			name: "asset not found",

			assetID: tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:  big.NewInt(1000),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:           "asset burned successfully",
			assetID:        assetID,
			amount:         big.NewInt(1000),
			expectedSupply: big.NewInt(4000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := burnAsset(creator, asset, test.assetID, common.DefaultTokenID, test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSupply, getCirculatingSupply(t, asset, assetID))
		})
	}
}

func Test_ValidateAssetApprove(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	insertTestAssetObject(
		t, sender, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
	)

	testcases := []struct {
		name        string
		sender      *state.Object
		beneficiary identifiers.Identifier
		amount      *big.Int
		assetID     identifiers.AssetID

		expectedError error
	}{
		{
			name:   "asset not found",
			sender: sender,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(1000),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient balance",
			sender: sender,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(7000),

			expectedError: common.ErrInsufficientFunds,
		},
		// TODO: Add more tests
		{
			name:   "valid asset approve operation",
			sender: sender,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(4000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAssetApprove(test.sender, test.assetID, common.DefaultTokenID, test.amount)
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
	creator, _, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		Timestamp     uint64
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: creator,

			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:      big.NewInt(5000),
			Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "asset approved successfully",
			sender:      creator,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(5000),
			Timestamp:   uint64(time.Now().Add(1 * time.Hour).Unix()),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := approveAsset(
				test.sender,
				test.assetID, common.DefaultTokenID,
				test.beneficiary,
				test.amount, 0)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetApprove(t, test.sender, test.assetID, common.DefaultTokenID, test.beneficiary, test.amount)
		})
	}
}

func Test_ValidateAssetRevoke(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)
	insertTestAssetObject(
		t, sender, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:        "mandate not found",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrMandateNotFound,
		},
		{
			name:        "valid asset revoke operation",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				createTestMandate(t,
					sender, target,
					assetID, common.DefaultTokenID,
					big.NewInt(2000), uint64(time.Now().Unix()),
				)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetRevoke(test.sender, test.beneficiary, test.assetID, common.DefaultTokenID)
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
	creator, _, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		Timestamp     uint64
		preTestFn     func(sender *state.Object, assetID identifiers.AssetID, beneficiary identifiers.Identifier)
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        creator,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(5000),
			Timestamp:     uint64(time.Now().Add(1 * time.Hour).Unix()),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "asset revoked successfully",
			sender: creator,
			preTestFn: func(sender *state.Object, assetID identifiers.AssetID, beneficiary identifiers.Identifier) {
				createMandate(
					t,
					sender, beneficiary,
					assetID, common.DefaultTokenID,
					big.NewInt(500), uint64(time.Now().Add(1*time.Hour).Unix()),
				)
			},

			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.sender, test.assetID, test.beneficiary)
			}

			err := revokeAsset(test.sender, test.beneficiary, test.assetID, common.DefaultTokenID)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetRevoke(t, test.sender, test.assetID, common.DefaultTokenID, test.beneficiary)
		})
	}
}

func Test_ValidateAssetLockup(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))

	insertTestAssetObject(
		t, sender, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(10000)),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		Amount        *big.Int
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:        "asset not found",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			Amount:      big.NewInt(1000),

			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "insufficient balance",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(12000),

			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:        "valid asset lockup operation",
			sender:      sender,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(4000),
		},
		{
			name:          "nil beneficiary",
			sender:        sender,
			beneficiary:   identifiers.Nil,
			assetID:       assetID,
			Amount:        big.NewInt(4000),
			expectedError: common.ErrInvalidBeneficiary,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateAssetLockup(test.sender, test.beneficiary, test.assetID, common.DefaultTokenID, test.Amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetLockup(t *testing.T) {
	creator, _, assetID := createTestAsset(t, big.NewInt(1000000), big.NewInt(5000))

	testcases := []struct {
		name          string
		sender        *state.Object
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		amount        *big.Int
		expectedError error
	}{
		{
			name:          "asset not found",
			sender:        creator,
			beneficiary:   tests.RandomIdentifier(t),
			assetID:       tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),
			amount:        big.NewInt(5000),
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "asset locked up successfully",
			sender:      creator,
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(5000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := lockupAsset(
				test.sender,
				test.beneficiary,
				test.assetID, common.DefaultTokenID,
				test.amount)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetLockup(t, test.sender, test.assetID, common.DefaultTokenID, test.beneficiary, test.amount)
		})
	}
}

func Test_ValidateAssetRelease(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	testcases := []struct {
		name          string
		sender        *state.Object
		benefactor    identifiers.Identifier
		beneficiary   identifiers.Identifier
		assetID       identifiers.AssetID
		Amount        *big.Int
		preTestFn     func(target, benefactor *state.Object)
		expectedError error
	}{
		{
			name:   "lockup not found",
			sender: sender,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrLockupNotFound,
		},
		{
			name:   "valid asset release operation",
			sender: sender,

			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			Amount:      big.NewInt(1000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2000))
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			benefactor := state.NewStateObject(
				test.benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := validateAssetRelease(
				test.sender.Identifier(),
				benefactor,
				test.assetID, common.DefaultTokenID,
				test.Amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_AssetRelease(t *testing.T) {
	sender := createTestStateObject(t)
	assetID := tests.GetRandomAssetID(t, tests.RandomIdentifier(t))
	sarga := createTestSargaStateObject(t)

	testcases := []struct {
		name           string
		sender         *state.Object
		benefactor     identifiers.Identifier
		beneficiary    identifiers.Identifier
		assetID        identifiers.AssetID
		amount         *big.Int
		preTestFn      func(target, benefactor *state.Object)
		expectedAmount *big.Int
		expectedError  error
	}{
		{
			name:        "asset not found",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     tests.GetRandomAssetID(t, tests.RandomIdentifier(t)),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:        "asset released successfully",
			sender:      sender,
			benefactor:  tests.RandomIdentifier(t),
			beneficiary: tests.RandomIdentifier(t),
			assetID:     assetID,
			amount:      big.NewInt(1000),

			preTestFn: func(target, benefactor *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
				insertTestAssetObject(
					t, benefactor, assetID, assetObjectWithToken(t, common.DefaultTokenID, big.NewInt(5000)),
				)
				createLockup(t, benefactor, sender.Identifier(), assetID, common.DefaultTokenID, big.NewInt(2500))
			},
			expectedAmount: big.NewInt(1500),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.beneficiary, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			benefactor := state.NewStateObject(
				test.benefactor, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target, benefactor)
			}

			err := releaseAsset(sender.Identifier(), benefactor, target, test.assetID, common.DefaultTokenID, test.amount)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetRelease(
				t,
				sender, target,
				benefactor,
				test.amount, test.assetID,
				common.DefaultTokenID, test.expectedAmount,
			)
		})
	}
}

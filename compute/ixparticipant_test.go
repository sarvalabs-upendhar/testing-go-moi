package compute

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_ValidateParticipantCreate(t *testing.T) {
	sender := createTestStateObject(t)
	sarga := createTestSargaStateObject(t)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(5000), nil),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.ParticipantCreatePayload
		preTestFn     func(target *state.Object)
		expectedError error
	}{
		{
			name:   "asset not found",
			sender: createTestStateObject(t),
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "participant already registered",
			sender: sender,
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(1000),
			},
			preTestFn: func(target *state.Object) {
				registerParticipant(t, sarga, target.Identifier())
			},
			expectedError: common.ErrAlreadyRegistered,
		},
		{
			name:   "insufficient funds",
			sender: sender,
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(7000),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid participant create operation",
			sender: sender,
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(2000),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.payload.ID, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := validateParticipantCreate(test.sender, target, sarga, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_ParticipantCreate(t *testing.T) {
	sender0 := createTestStateObject(t)
	sender1 := createTestStateObject(t)

	insertTestAssetObject(
		t, sender0, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(2000), nil),
	)
	insertTestAssetObject(
		t, sender1, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(5000), nil),
	)

	testcases := []struct {
		name                  string
		sender                *state.Object
		payload               *common.ParticipantCreatePayload
		preTestFn             func(target *state.Object)
		expectedSenderBalance *big.Int
		expectedTargetBalance *big.Int
		expectedError         error
	}{
		{
			name:   "asset not found",
			sender: createTestStateObject(t),
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "asset already registered",
			sender: sender0,
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(1000),
			},
			preTestFn: func(target *state.Object) {
				insertTestAssetObject(
					t, target, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(5000), nil),
				)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name:   "participant created successfully",
			sender: sender1,
			payload: &common.ParticipantCreatePayload{
				ID:     tests.RandomIdentifier(t),
				Amount: big.NewInt(2000),
			},
			expectedSenderBalance: big.NewInt(3000),
			expectedTargetBalance: big.NewInt(2000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.payload.ID, nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			)

			if test.preTestFn != nil {
				test.preTestFn(target)
			}

			err := createParticipant(test.sender, target, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkAssetTransfer(
				t, common.KMOITokenAssetID, test.sender, target,
				test.expectedSenderBalance, test.expectedTargetBalance,
			)
		})
	}
}

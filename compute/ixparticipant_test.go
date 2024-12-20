package compute

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_ParticipantCreate(t *testing.T) {
	sender0 := state.NewStateObject(
		tests.RandomAddress(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)
	sender1 := state.NewStateObject(
		tests.RandomAddress(t), nil, tests.NewTestTreeCache(),
		nil, common.Account{}, state.NilMetrics(), false,
	)

	insertTestAssetObject(
		t, common.KMOITokenAssetID, sender0, state.NewAssetObject(big.NewInt(2000), nil),
	)
	insertTestAssetObject(
		t, common.KMOITokenAssetID, sender1, state.NewAssetObject(big.NewInt(5000), nil),
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
			name: "asset not found",
			sender: state.NewStateObject(
				tests.RandomAddress(t), nil, tests.NewTestTreeCache(),
				nil, common.Account{}, state.NilMetrics(), false,
			),
			payload: &common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				Amount:  big.NewInt(1000),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient funds",
			sender: sender0,
			payload: &common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				Amount:  big.NewInt(6000),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "asset already registered",
			sender: sender0,
			payload: &common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				Amount:  big.NewInt(1000),
			},
			preTestFn: func(target *state.Object) {
				insertTestAssetObject(
					t, common.KMOITokenAssetID, target, state.NewAssetObject(big.NewInt(5000), nil),
				)
			},
			expectedError: common.ErrAssetAlreadyRegistered,
		},
		{
			name:   "participant created successfully",
			sender: sender1,
			payload: &common.ParticipantCreatePayload{
				Address: tests.RandomAddress(t),
				Amount:  big.NewInt(2000),
			},
			expectedSenderBalance: big.NewInt(3000),
			expectedTargetBalance: big.NewInt(2000),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			target := state.NewStateObject(
				test.payload.Address, nil, tests.NewTestTreeCache(),
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

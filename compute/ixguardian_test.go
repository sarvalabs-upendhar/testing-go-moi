package compute

import (
	"errors"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/state"
	"github.com/stretchr/testify/require"
)

func Test_ValidateGuardianRegister(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	availableBalance := int64(5000)
	registeredKramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID,
		state.NewAssetObject(big.NewInt(availableBalance), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID: registeredKramaID,
	})

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.GuardianRegisterPayload
		expectedError error
	}{
		{
			name:   "guardian already exists",
			sender: sender,
			payload: &common.GuardianRegisterPayload{
				KramaID: registeredKramaID,
			},
			expectedError: common.ErrGuardianExists,
		},
		{
			name:   "kmoi asset not found",
			sender: createTestStateObject(t),
			payload: &common.GuardianRegisterPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(availableBalance),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient kmoi balance",
			sender: sender,
			payload: &common.GuardianRegisterPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(availableBalance + 1),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid guardian register",
			sender: sender,
			payload: &common.GuardianRegisterPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(availableBalance),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateGuardianRegister(test.sender, system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_RegisterGuardian(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	availableBalance := int64(5000)
	registeredKramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(availableBalance), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID: registeredKramaID,
	})

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.GuardianRegisterPayload
		expectedError error
	}{
		{
			name:   "kmoi asset not found",
			sender: createTestStateObject(t),
			payload: &common.GuardianRegisterPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(availableBalance + 1),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "valid guardian register",
			sender: sender,
			payload: &common.GuardianRegisterPayload{
				WalletID:     tests.RandomIdentifier(t),
				KramaID:      tests.RandomKramaID(t, 0),
				ConsensusKey: tests.RandomHash(t).Bytes(),
				KYCProof:     tests.RandomHash(t).Bytes(),
				Amount:       big.NewInt(availableBalance),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := registerGuardian(test.sender, system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkGuardianRegister(t, system, test.payload)
		})
	}
}

func Test_ValidateGuardianStake(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	availableBalance := int64(5000)
	registeredKramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID,
		state.NewAssetObject(big.NewInt(availableBalance), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID: registeredKramaID,
	})

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.GuardianActionPayload
		expectedError error
	}{
		{
			name:   "guardian doesn't exists",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "kmoi asset not found",
			sender: createTestStateObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(availableBalance),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "insufficient kmoi balance",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(availableBalance + 1),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid guardian stake",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(availableBalance),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateGuardianStake(test.sender, system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_StakeGuardian(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	availableBalance := int64(5000)
	registeredKramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID,
		state.NewAssetObject(big.NewInt(availableBalance), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID:               registeredKramaID,
		PendingStakeAdditions: big.NewInt(0),
	})

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.GuardianActionPayload
		expectedError error
	}{
		{
			name:   "kmoi asset not found",
			sender: createTestStateObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(availableBalance),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "guardian doesn't exists",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(availableBalance),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "valid guardian stake",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(availableBalance),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := stakeGuardian(test.sender, system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkStakeGuardian(t, system, test.payload)
		})
	}
}

func Test_ValidateGuardianUnstake(t *testing.T) {
	system := createTestSystemObject(t)
	activeStake := int64(5000)
	registeredKramaID := tests.RandomKramaID(t, 0)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID:     registeredKramaID,
		ActiveStake: big.NewInt(activeStake),
	})

	testcases := []struct {
		name          string
		payload       *common.GuardianActionPayload
		expectedError error
	}{
		{
			name: "guardian doesn't exists",
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name: "amount greater than active stake",
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(activeStake + 1),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name: "valid guardian unstake",
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(activeStake),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := validateGuardianUnstake(system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_UnstakeGuardian(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	activeStake := int64(5000)
	registeredKramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(activeStake), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID:              registeredKramaID,
		ActiveStake:          big.NewInt(activeStake),
		PendingStakeRemovals: make(map[common.Epoch]*big.Int),
	})

	testcases := []struct {
		name          string
		sender        *state.Object
		payload       *common.GuardianActionPayload
		expectedError error
	}{
		{
			name:   "guardian doesn't exists",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "valid guardian unstake",
			sender: sender,
			payload: &common.GuardianActionPayload{
				KramaID: registeredKramaID,
				Amount:  big.NewInt(activeStake),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := unstakeGuardian(system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkUnstakeGuardian(t, system, test.payload)
		})
	}
}

func Test_ValidateGuardianWithdraw(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	inactiveStake := int64(8000)
	kramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID, state.NewAssetObject(big.NewInt(inactiveStake), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID:       kramaID,
		InactiveStake: big.NewInt(inactiveStake),
		WalletAddress: sender.Identifier(),
	})

	createLockup(t, sender, createTestGuardianStateObject(t),
		common.KMOITokenAssetID, big.NewInt(inactiveStake-3000),
	)

	testcases := []struct {
		name          string
		sender        *state.Object
		system        *state.SystemObject
		payload       *common.GuardianActionPayload
		preTestFn     func(system *state.SystemObject, sender *state.Object)
		expectedError error
	}{
		{
			name:   "guardian doesn't exists",
			sender: sender,
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "guardian wallet address mismatch",
			sender: createTestStateObject(t),
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake),
			},
			expectedError: common.ErrInvalidIdentifier,
		},
		{
			name:   "amount lesser than inactive stake",
			sender: sender,
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake + 1),
			},
			expectedError: errors.New("insufficient inactive stake"),
		},
		{
			name:   "lockup not found",
			sender: createTestStateObject(t),
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake),
			},
			preTestFn: func(system *state.SystemObject, sender *state.Object) {
				insertTestAssetObject(
					t, sender, common.KMOITokenAssetID,
					state.NewAssetObject(big.NewInt(5000), nil),
				)

				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					InactiveStake: big.NewInt(8000),
					WalletAddress: sender.Identifier(),
				})
			},
			expectedError: common.ErrLockupNotFound,
		},
		{
			name:   "amount lesser than the withdraw balance",
			sender: sender,
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake),
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid guardian withdraw",
			sender: sender,
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake - 3000),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.system, test.sender)
			}

			err := validateGuardianWithdraw(test.sender, test.system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_GuardianWithdraw(t *testing.T) {
	system := createTestSystemObject(t)
	inactiveStake := int64(5000)
	kramaID := tests.RandomKramaID(t, 0)

	testcases := []struct {
		name            string
		sender          *state.Object
		payload         *common.GuardianActionPayload
		preTestFn       func(system *state.SystemObject, sender *state.Object)
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:   "lockup not found",
			sender: createTestStateObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "guardian doesn't exists",
			sender: createTestStateObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(inactiveStake),
			},
			preTestFn: func(system *state.SystemObject, sender *state.Object) {
				insertTestAssetObject(
					t, sender, common.KMOITokenAssetID,
					state.NewAssetObject(big.NewInt(5000), nil),
				)

				createLockup(
					t, sender, createTestGuardianStateObject(t),
					common.KMOITokenAssetID, big.NewInt(inactiveStake),
				)
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "valid guardian withdraw",
			sender: createTestStateObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(inactiveStake),
			},
			preTestFn: func(system *state.SystemObject, sender *state.Object) {
				insertTestAssetObject(
					t, sender, common.KMOITokenAssetID,
					state.NewAssetObject(big.NewInt(inactiveStake), nil),
				)

				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					InactiveStake: big.NewInt(inactiveStake),
					WalletAddress: sender.Identifier(),
				})

				createLockup(
					t, sender, createTestGuardianStateObject(t),
					common.KMOITokenAssetID, big.NewInt(inactiveStake),
				)
			},
			expectedBalance: big.NewInt(inactiveStake),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(system, test.sender)
			}

			err := withdrawStake(test.sender, system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkGuardianWithdraw(
				t, test.sender, system, test.payload,
				big.NewInt(inactiveStake-test.payload.Amount.Int64()), test.expectedBalance,
			)
		})
	}
}

func Test_ValidateGuardianClaim(t *testing.T) {
	sender := createTestStateObject(t)
	activeRewards := int64(5000)
	kramaID := tests.RandomKramaID(t, 0)

	testcases := []struct {
		name          string
		sender        *state.Object
		system        *state.SystemObject
		payload       *common.GuardianActionPayload
		preTestFn     func(system *state.SystemObject)
		expectedError error
	}{
		{
			name:   "guardian doesn't exists",
			sender: sender,
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "guardian wallet address mismatch",
			sender: sender,
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(activeRewards),
			},
			preTestFn: func(system *state.SystemObject) {
				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					Rewards:       big.NewInt(activeRewards),
					WalletAddress: tests.RandomIdentifier(t),
				})
			},
			expectedError: common.ErrInvalidIdentifier,
		},
		{
			name:   "amount greater than available rewards",
			sender: sender,
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(activeRewards + 1),
			},
			preTestFn: func(system *state.SystemObject) {
				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					Rewards:       big.NewInt(activeRewards),
					WalletAddress: sender.Identifier(),
				})
			},
			expectedError: errors.New("insufficient rewards"),
		},
		{
			name:   "kmoi asset not found",
			sender: sender,
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(activeRewards),
			},
			preTestFn: func(system *state.SystemObject) {
				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					Rewards:       big.NewInt(activeRewards),
					WalletAddress: sender.Identifier(),
				})
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "amount lesser than the withdraw balance",
			sender: sender,
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(activeRewards),
			},
			preTestFn: func(system *state.SystemObject) {
				insertTestAssetObject(
					t, system.Object, common.KMOITokenAssetID,
					state.NewAssetObject(big.NewInt(activeRewards-1), nil),
				)

				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					Rewards:       big.NewInt(activeRewards),
					WalletAddress: sender.Identifier(),
				})
			},
			expectedError: common.ErrInsufficientFunds,
		},
		{
			name:   "valid guardian withdraw",
			sender: sender,
			system: createTestSystemObject(t),
			preTestFn: func(system *state.SystemObject) {
				insertTestAssetObject(
					t, system.Object, common.KMOITokenAssetID,
					state.NewAssetObject(big.NewInt(activeRewards), nil),
				)

				insertGuardianEntry(t, system, &common.Validator{
					KramaID:       kramaID,
					Rewards:       big.NewInt(activeRewards),
					WalletAddress: sender.Identifier(),
				})
			},
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(activeRewards),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn(test.system)
			}

			err := validateGuardianClaim(test.sender, test.system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_GuardianClaim(t *testing.T) {
	sender := createTestStateObject(t)
	system := createTestSystemObject(t)
	activeRewards := int64(5000)
	kramaID := tests.RandomKramaID(t, 0)

	insertTestAssetObject(
		t, sender, common.KMOITokenAssetID,
		state.NewAssetObject(big.NewInt(100), nil),
	)

	insertTestAssetObject(
		t, system.Object, common.KMOITokenAssetID,
		state.NewAssetObject(big.NewInt(activeRewards), nil),
	)

	insertGuardianEntry(t, system, &common.Validator{
		KramaID:       kramaID,
		Rewards:       big.NewInt(activeRewards),
		WalletAddress: sender.Identifier(),
	})

	testcases := []struct {
		name            string
		system          *state.SystemObject
		payload         *common.GuardianActionPayload
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name:   "kmoi asset not found",
			system: createTestSystemObject(t),
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(activeRewards),
			},
			expectedError: common.ErrAssetNotFound,
		},
		{
			name:   "guardian doesn't exists",
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: tests.RandomKramaID(t, 0),
				Amount:  big.NewInt(activeRewards),
			},
			expectedError: common.ErrKramaIDNotFound,
		},
		{
			name:   "valid guardian claim",
			system: system,
			payload: &common.GuardianActionPayload{
				KramaID: kramaID,
				Amount:  big.NewInt(activeRewards),
			},
			expectedBalance: big.NewInt(activeRewards + 100),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := claimRewards(sender, test.system, test.payload)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkGuardianClaim(t, sender, system, test.payload, test.expectedBalance)
		})
	}
}

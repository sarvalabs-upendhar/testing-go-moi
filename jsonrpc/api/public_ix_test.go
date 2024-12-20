package api

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
)

// Interaction API Testcases
func TestPublicCoreAPI_SendInteraction(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)
	assetPayload := common.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(2)
	})

	validIXArgs := common.IxData{
		Sender:    address,
		Nonce:     2,
		FuelPrice: big.NewInt(1),
		FuelLimit: 1,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: rawAssetPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				Address:  address,
				LockType: common.MutateLock,
			},
		},
	}

	expectedIxn, err := common.NewInteraction(
		common.IxData{
			Sender:    address,
			Nonce:     acc.Nonce,
			FuelPrice: big.NewInt(1),
			FuelLimit: 1,
			IxOps: []common.IxOpRaw{
				{
					Type:    common.IxAssetCreate,
					Payload: rawAssetPayload,
				},
			},
			Participants: []common.IxParticipant{
				{
					Address:  address,
					LockType: common.MutateLock,
				},
			},
		},
		getSignatureBytes(t, &validIXArgs, mnemonic),
	)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ixData      common.IxData
		expected    *common.Interaction
		preTestFn   func(ixPool *MockIxPool, sm *MockStateManager)
		expectedErr error
	}{
		{
			name: "invalid send ix args",
			ixData: common.IxData{
				Sender:    common.SargaAddress,
				FuelPrice: big.NewInt(1),
				FuelLimit: 1,
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
					},
				},
			},
			expectedErr: common.ErrGenesisAccount,
		},
		{
			name: "failed to construct interaction",
			ixData: common.IxData{
				Nonce:     3,
				FuelPrice: big.NewInt(1),
				FuelLimit: 1,
				Sender:    address,
				IxOps: []common.IxOpRaw{
					{
						Type: common.IxInvalid,
					},
				},
				Participants: []common.IxParticipant{
					{
						Address:  address,
						LockType: common.MutateLock,
					},
				},
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name: "failed to add interaction in ixpool",
			ixData: common.IxData{
				Nonce:     3,
				Sender:    address,
				FuelPrice: big.NewInt(1),
				FuelLimit: 1,
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				},
				Participants: []common.IxParticipant{
					{
						Address:  address,
						LockType: common.MutateLock,
					},
				},
			},
			preTestFn: func(ixPool *MockIxPool, sm *MockStateManager) {
				ixPool.addInteractionHook = func() []error {
					return []error{
						errors.New("failed add ix in ixpool"),
					}
				}
			},
			expectedErr: errors.New("failed add ix in ixpool"),
		},
		{
			name:     "Valid ix args with zero nonce",
			ixData:   validIXArgs,
			expected: expectedIxn,
			preTestFn: func(ixPool *MockIxPool, sm *MockStateManager) {
				ixPool.setNonce(address, 2)
				sm.setAccount(address, *acc)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sm := NewMockStateManager(t)
			ixpool := NewMockIxPool(t)
			coreAPI := NewPublicCoreAPI(ixpool, nil, sm, nil, nil, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixpool, sm)
			}

			bz, err := polo.Polorize(test.ixData)
			require.NoError(t, err)

			sendIx := rpcargs.SendIX{
				IXArgs:    hex.EncodeToString(bz),
				Signature: getSignatureString(t, &test.ixData, mnemonic),
			}

			ixnHash, err := coreAPI.SendInteractions(&sendIx)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected.Hash(), ixnHash)
			require.Equal(t, len(ixpool.interactions), 1)
		})
	}
}

func TestPublicCoreAPI_ValidateArgumentsWithSign(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	ixWithNilSender := common.IxData{
		Sender: identifiers.NilAddress,
	}

	ixWithSargaSender := common.IxData{
		Sender: common.SargaAddress,
	}

	ix := &common.IxData{
		Sender:    address,
		FuelPrice: big.NewInt(1),
		FuelLimit: 23,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetTransfer,
				Payload: tests.CreateRawAssetActionPayload(t, common.SargaAddress),
			},
		},
	}

	rawIXWithNilSender, err := ixWithNilSender.Bytes()
	require.NoError(t, err)

	rawIXWithSargaSender, err := ixWithSargaSender.Bytes()
	require.NoError(t, err)

	rawIX, err := ix.Bytes()
	require.NoError(t, err)

	sign, err := crypto.GetSignature(rawIX, mnemonic)
	require.NoError(t, err)

	rawSign, err := hex.DecodeString(sign)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *rpcargs.SendIX
		ixArgs      *common.IxData
		sign        []byte
		expectedErr error
	}{
		{
			name: "sender address is nil",
			ix: &rpcargs.SendIX{
				IXArgs: hex.EncodeToString(rawIXWithNilSender),
			},
			expectedErr: common.ErrInvalidAddress,
		},
		{
			name: "sender is sarga account",
			ix: &rpcargs.SendIX{
				IXArgs: hex.EncodeToString(rawIXWithSargaSender),
			},
			expectedErr: common.ErrGenesisAccount,
		},
		{
			name: "valid args",
			ix: &rpcargs.SendIX{
				IXArgs: hex.EncodeToString(rawIX),
			},
			ixArgs: ix,
			sign:   rawSign,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			ixArgs, err := validateArgumentsWithSign(test.ix)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.ixArgs, ixArgs)
		})
	}
}

func TestPublicCoreAPI_ValidateIxData(t *testing.T) {
	testcases := []struct {
		name         string
		ixArgs       *common.IxData
		requiresFuel bool
		expectedErr  error
	}{
		{
			name: "sender address is nil",
			ixArgs: &common.IxData{
				FuelPrice: big.NewInt(1),
				FuelLimit: 23,
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
					},
				},
			},
			requiresFuel: true,
			expectedErr:  common.ErrInvalidAddress,
		},
		{
			name: "sender is sarga account",
			ixArgs: &common.IxData{
				Sender:    common.SargaAddress,
				FuelPrice: big.NewInt(1),
				FuelLimit: 23,
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, common.SargaAddress),
					},
				},
			},
			requiresFuel: true,
			expectedErr:  common.ErrGenesisAccount,
		},
		{
			name: "empty ix ops",
			ixArgs: &common.IxData{
				Sender:    tests.RandomAddress(t),
				FuelPrice: big.NewInt(1),
				FuelLimit: 23,
			},
			requiresFuel: true,
			expectedErr:  ErrEmptyIxOps,
		},
		{
			name: "fuel price and limit required",
			ixArgs: &common.IxData{
				Sender: tests.RandomAddress(t),
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
					},
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				},
			},
			requiresFuel: true,
			expectedErr:  common.ErrFuelPriceNotFound,
		},
		{
			name: "fuel price and limit not required",
			ixArgs: &common.IxData{
				Sender: tests.RandomAddress(t),
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
					},
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				},
			},
			requiresFuel: false,
		},
		{
			name: "valid ix data",
			ixArgs: &common.IxData{
				Sender:    tests.RandomAddress(t),
				FuelPrice: big.NewInt(1),
				FuelLimit: 23,
				IxOps: []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
					},
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				},
			},
			requiresFuel: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			err := validateIxData(test.ixArgs, test.requiresFuel)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestPublicCoreAPI_ValidateFuel(t *testing.T) {
	testcases := []struct {
		name        string
		ixArgs      *common.IxData
		expectedErr error
	}{
		{
			name: "fuel price not found",
			ixArgs: &common.IxData{
				FuelLimit: 23,
			},
			expectedErr: common.ErrFuelPriceNotFound,
		},
		{
			name: "fuel limit not found",
			ixArgs: &common.IxData{
				FuelPrice: big.NewInt(1),
			},
			expectedErr: common.ErrFuelLimitNotFound,
		},
		{
			name: "valid fuel price and limit",
			ixArgs: &common.IxData{
				FuelPrice: big.NewInt(1),
				FuelLimit: 23,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			err := validateFuel(test.ixArgs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestPublicCoreAPI_ValidateIxOps(t *testing.T) {
	testcases := []struct {
		name        string
		txs         []common.IxOpRaw
		expectedErr error
	}{
		{
			name:        "empty ix operations",
			expectedErr: ErrEmptyIxOps,
		},
		{
			name: "too many ix operations",
			txs: []common.IxOpRaw{
				{
					Type:    common.IxAssetCreate,
					Payload: tests.CreateRawAssetCreatePayload(t),
				},
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
				},
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
				},
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
				},
			},
			expectedErr: ErrTooManyIxOps,
		},
		{
			name: "exceeds asset creation limit",
			txs: []common.IxOpRaw{
				{
					Type:    common.IxAssetCreate,
					Payload: tests.CreateRawAssetCreatePayload(t),
				},
				{
					Type:    common.IxAssetCreate,
					Payload: tests.CreateRawAssetCreatePayload(t),
				},
			},
			expectedErr: ErrAssetCreationLimit,
		},
		{
			name: "exceeds logic deploy limit",
			txs: []common.IxOpRaw{
				{
					Type:    common.IxLogicDeploy,
					Payload: tests.CreateRawLogicPayload(t, tests.RandomAddress(t)),
				},
				{
					Type:    common.IxLogicDeploy,
					Payload: tests.CreateRawLogicPayload(t, tests.RandomAddress(t)),
				},
			},
			expectedErr: ErrLogicDeploymentLimit,
		},
		{
			name: "valid ix operations",
			txs: []common.IxOpRaw{
				{
					Type:    common.IxAssetTransfer,
					Payload: tests.CreateRawAssetActionPayload(t, tests.RandomAddress(t)),
				},
				{
					Type:    common.IxAssetCreate,
					Payload: tests.CreateRawAssetCreatePayload(t),
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			err := validateIxOps(test.txs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

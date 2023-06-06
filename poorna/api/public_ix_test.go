package api

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/mudra"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Interaction Api Testcases
func TestIx_SendInteraction(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)
	assetPayload := types.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	acc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
		acc.Nonce = uint64(2)
	})

	validIXArgs := types.SendIXArgs{
		Type:      types.IxAssetCreate,
		Sender:    address,
		Nonce:     2,
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(1),
		Payload:   rawAssetPayload,
	}

	expectedIxn, err := constructInteraction(
		&types.SendIXArgs{
			Type:      types.IxAssetCreate,
			Sender:    address,
			Nonce:     acc.Nonce,
			FuelPrice: big.NewInt(1),
			FuelLimit: big.NewInt(1),
			Payload:   rawAssetPayload,
		},
		getSignatureBytes(t, &validIXArgs, mnemonic),
	)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		sendIXArgs  types.SendIXArgs
		expected    *types.Interaction
		preTestFn   func(ixPool *MockIxPool, sm *MockStateManager)
		expectedErr error
	}{
		{
			name: "invalid send ix args",
			sendIXArgs: types.SendIXArgs{
				Type:      types.IxValueTransfer,
				Sender:    types.SargaAddress,
				FuelPrice: big.NewInt(1),
				FuelLimit: big.NewInt(1),
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "failed to construct interaction",
			sendIXArgs: types.SendIXArgs{
				Type:      types.IxInvalid,
				Nonce:     3,
				FuelPrice: big.NewInt(1),
				FuelLimit: big.NewInt(1),
				Sender:    address,
			},
			expectedErr: errors.New("invalid interaction type"),
		},
		{
			name: "failed to add interaction in ixpool",
			sendIXArgs: types.SendIXArgs{
				Type:      types.IxValueTransfer,
				Nonce:     3,
				Sender:    address,
				FuelPrice: big.NewInt(1),
				FuelLimit: big.NewInt(1),
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
			name:       "Valid ix args with zero nonce",
			sendIXArgs: validIXArgs,
			expected:   expectedIxn,
			preTestFn: func(ixPool *MockIxPool, sm *MockStateManager) {
				ixPool.setNonce(address, 2)
				sm.setAccount(address, *acc)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixpool := NewMockIxPool(t)
			stateManager := NewMockStateManager(t)
			ixAPI := NewPublicIXAPI(ixpool, stateManager)

			if test.preTestFn != nil {
				test.preTestFn(ixpool, stateManager)
			}

			bz, err := polo.Polorize(test.sendIXArgs)
			require.NoError(t, err)

			sendIx := ptypes.SendIX{
				IXArgs:    hex.EncodeToString(bz),
				Signature: getSignatureString(t, &test.sendIXArgs, mnemonic),
			}

			ixn, err := ixAPI.SendInteraction(&sendIx)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected, ixn)
			require.Equal(t, len(ixpool.interactions), 1)
		})
	}
}

func TestIx_ConstructInteraction(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	testcases := []struct {
		name        string
		sendIXArgs  *types.SendIXArgs
		expectedIX  *types.Interaction
		expectedErr error
	}{
		{
			name: "send ix args",
			sendIXArgs: &types.SendIXArgs{
				Type:     types.IxValueTransfer,
				Nonce:    4,
				Sender:   address,
				Receiver: tests.RandomAddress(t),
				Payer:    tests.RandomAddress(t),
				TransferValues: map[types.AssetID]*big.Int{
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
				},
				PerceivedValues: map[types.AssetID]*big.Int{
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(99),
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(111),
				},
				FuelPrice: big.NewInt(12),
				FuelLimit: big.NewInt(23),
				Payload:   []byte{2, 3, 3},
			},
		},
		{
			name: "fuel price not found",
			sendIXArgs: &types.SendIXArgs{
				Type:     types.IxValueTransfer,
				Nonce:    4,
				Sender:   types.SargaAddress,
				Receiver: tests.RandomAddress(t),
				Payer:    tests.RandomAddress(t),
			},
			expectedErr: types.ErrFuelPriceNotFound,
		},
		{
			name: "fuel limit not found",
			sendIXArgs: &types.SendIXArgs{
				Type:      types.IxValueTransfer,
				Nonce:     4,
				Sender:    types.SargaAddress,
				Receiver:  tests.RandomAddress(t),
				FuelPrice: big.NewInt(22),
				Payer:     tests.RandomAddress(t),
			},
			expectedErr: types.ErrFuelLimitNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			sign := tests.GetIXSignature(t, test.sendIXArgs, mnemonic)
			ix, err := constructInteraction(test.sendIXArgs, sign)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.sendIXArgs.Type, ix.Type())
			require.Equal(t, test.sendIXArgs.Nonce, ix.Nonce())
			require.Equal(t, test.sendIXArgs.Sender, ix.Sender())
			require.Equal(t, test.sendIXArgs.Receiver, ix.Receiver())
			require.Equal(t, test.sendIXArgs.Payer, ix.Payer())
			require.Equal(t, test.sendIXArgs.TransferValues, ix.TransferValues())
			require.Equal(t, test.sendIXArgs.PerceivedValues, ix.PerceivedValues())
			require.Equal(t, test.sendIXArgs.FuelPrice, ix.FuelPrice())
			require.Equal(t, test.sendIXArgs.FuelLimit, ix.FuelLimit())
			require.Equal(t, test.sendIXArgs.Payload, ix.Payload())
			require.Equal(t, sign, ix.Signature())
		})
	}
}

func TestIx_ValidateArgumentsWithSign(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	ixWithNilSender := types.SendIXArgs{
		Sender: types.NilAddress,
	}

	ixWithSargaSender := types.SendIXArgs{
		Sender: types.SargaAddress,
	}

	ixWithSargaReceiver := types.SendIXArgs{
		Sender:   tests.RandomAddress(t),
		Receiver: types.SargaAddress,
	}

	ix := &types.SendIXArgs{
		Sender: address,
	}

	rawIXWithNilSender, err := ixWithNilSender.Bytes()
	require.NoError(t, err)

	rawIXWithSargaSender, err := ixWithSargaSender.Bytes()
	require.NoError(t, err)

	rawIXWithSargaReceiver, err := ixWithSargaReceiver.Bytes()
	require.NoError(t, err)

	rawIX, err := ix.Bytes()
	require.NoError(t, err)

	sign, err := mudra.GetSignature(rawIX, mnemonic)
	require.NoError(t, err)

	rawSign, err := hex.DecodeString(sign)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *ptypes.SendIX
		ixArgs      *types.SendIXArgs
		sign        []byte
		expectedErr error
	}{
		{
			name: "sender address is nil",
			ix: &ptypes.SendIX{
				IXArgs: hex.EncodeToString(rawIXWithNilSender),
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "sender is sarga account",
			ix: &ptypes.SendIX{
				IXArgs: hex.EncodeToString(rawIXWithSargaSender),
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "receiver is sarga account",
			ix: &ptypes.SendIX{
				IXArgs: hex.EncodeToString(rawIXWithSargaReceiver),
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "valid args",
			ix: &ptypes.SendIX{
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

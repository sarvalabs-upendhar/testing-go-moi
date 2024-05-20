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

	validIXArgs := common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Sender:    address,
		Nonce:     2,
		FuelPrice: big.NewInt(1),
		FuelLimit: 1,
		Payload:   rawAssetPayload,
	}

	expectedIxn, err := constructInteraction(
		&common.SendIXArgs{
			Type:      common.IxAssetCreate,
			Sender:    address,
			Nonce:     acc.Nonce,
			FuelPrice: big.NewInt(1),
			FuelLimit: 1,
			Payload:   rawAssetPayload,
		},
		getSignatureBytes(t, &validIXArgs, mnemonic),
	)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		sendIXArgs  common.SendIXArgs
		expected    *common.Interaction
		preTestFn   func(ixPool *MockIxPool, sm *MockStateManager)
		expectedErr error
	}{
		{
			name: "invalid send ix args",
			sendIXArgs: common.SendIXArgs{
				Type:      common.IxValueTransfer,
				Sender:    common.SargaAddress,
				FuelPrice: big.NewInt(1),
				FuelLimit: 1,
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "failed to construct interaction",
			sendIXArgs: common.SendIXArgs{
				Type:      common.IxInvalid,
				Nonce:     3,
				FuelPrice: big.NewInt(1),
				FuelLimit: 1,
				Sender:    address,
			},
			expectedErr: common.ErrInvalidInteractionType,
		},
		{
			name: "failed to add interaction in ixpool",
			sendIXArgs: common.SendIXArgs{
				Type:      common.IxValueTransfer,
				Nonce:     3,
				Sender:    address,
				FuelPrice: big.NewInt(1),
				FuelLimit: 1,
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
			sm := NewMockStateManager(t)
			ixpool := NewMockIxPool(t)
			coreAPI := NewPublicCoreAPI(ixpool, nil, sm, nil, nil, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixpool, sm)
			}

			bz, err := polo.Polorize(test.sendIXArgs)
			require.NoError(t, err)

			sendIx := rpcargs.SendIX{
				IXArgs:    hex.EncodeToString(bz),
				Signature: getSignatureString(t, &test.sendIXArgs, mnemonic),
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

func TestPublicCoreAPI_ConstructInteraction(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	testcases := []struct {
		name        string
		sendIXArgs  *common.SendIXArgs
		expectedIX  *common.Interaction
		expectedErr error
	}{
		{
			name: "send ix args",
			sendIXArgs: &common.SendIXArgs{
				Type:     common.IxValueTransfer,
				Nonce:    4,
				Sender:   address,
				Receiver: tests.RandomAddress(t),
				Payer:    tests.RandomAddress(t),
				TransferValues: map[identifiers.AssetID]*big.Int{
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
				},
				PerceivedValues: map[identifiers.AssetID]*big.Int{
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(99),
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(111),
				},
				FuelPrice: big.NewInt(1),
				FuelLimit: 23,
				Payload:   []byte{2, 3, 3},
			},
		},
		{
			name: "fuel price not found",
			sendIXArgs: &common.SendIXArgs{
				Type:     common.IxValueTransfer,
				Nonce:    4,
				Sender:   common.SargaAddress,
				Receiver: tests.RandomAddress(t),
				Payer:    tests.RandomAddress(t),
			},
			expectedErr: common.ErrFuelPriceNotFound,
		},
		{
			name: "fuel limit not found",
			sendIXArgs: &common.SendIXArgs{
				Type:      common.IxValueTransfer,
				Nonce:     4,
				Sender:    common.SargaAddress,
				Receiver:  tests.RandomAddress(t),
				FuelPrice: big.NewInt(1),
				Payer:     tests.RandomAddress(t),
			},
			expectedErr: common.ErrFuelLimitNotFound,
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

func TestPublicCoreAPI_ValidateArgumentsWithSign(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	ixWithNilSender := common.SendIXArgs{
		Sender: identifiers.NilAddress,
	}

	ixWithSargaSender := common.SendIXArgs{
		Sender: common.SargaAddress,
	}

	ixWithSargaReceiver := common.SendIXArgs{
		Sender:   tests.RandomAddress(t),
		Receiver: common.SargaAddress,
	}

	ix := &common.SendIXArgs{
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

	sign, err := crypto.GetSignature(rawIX, mnemonic)
	require.NoError(t, err)

	rawSign, err := hex.DecodeString(sign)
	require.NoError(t, err)

	testcases := []struct {
		name        string
		ix          *rpcargs.SendIX
		ixArgs      *common.SendIXArgs
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
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "receiver is sarga account",
			ix: &rpcargs.SendIX{
				IXArgs: hex.EncodeToString(rawIXWithSargaReceiver),
			},
			expectedErr: ErrGenesisAccount,
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

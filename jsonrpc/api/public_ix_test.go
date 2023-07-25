package api

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Interaction Api Testcases
func TestIx_SendInteraction(t *testing.T) {
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
		FuelLimit: big.NewInt(1),
		Payload:   rawAssetPayload,
	}

	expectedIxn, err := constructInteraction(
		&common.SendIXArgs{
			Type:      common.IxAssetCreate,
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
				FuelLimit: big.NewInt(1),
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "failed to construct interaction",
			sendIXArgs: common.SendIXArgs{
				Type:      common.IxInvalid,
				Nonce:     3,
				FuelPrice: big.NewInt(1),
				FuelLimit: big.NewInt(1),
				Sender:    address,
			},
			expectedErr: errors.New("invalid interaction type"),
		},
		{
			name: "failed to add interaction in ixpool",
			sendIXArgs: common.SendIXArgs{
				Type:      common.IxValueTransfer,
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
			ixAPI := NewPublicIXAPI(ixpool, stateManager, nil)

			if test.preTestFn != nil {
				test.preTestFn(ixpool, stateManager)
			}

			bz, err := polo.Polorize(test.sendIXArgs)
			require.NoError(t, err)

			sendIx := rpcargs.SendIX{
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

func TestIx_Call(t *testing.T) {
	addr := tests.RandomAddress(t)
	assetPayload := common.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	exec := NewMockExecutionManager(t)
	ixAPI := NewPublicIXAPI(nil, nil, exec)

	IxArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Sender:    addr,
		Nonce:     4,
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(100),
		Payload:   rawAssetPayload,
	}

	ix, err := constructInteraction(IxArgs, nil)
	assert.NoError(t, err)

	receipt := &common.Receipt{
		FuelUsed:  big.NewInt(100),
		ExtraData: rawAssetPayload,
	}

	exec.setInteractionCall(ix, receipt)

	testcases := []struct {
		name            string
		sendIXArgs      *rpcargs.IxArgs
		expectedReceipt *rpcargs.RPCReceipt
		expectedErr     error
	}{
		{
			name: "failed to construct interaction",
			sendIXArgs: &rpcargs.IxArgs{
				Type:      common.IxInvalid,
				Sender:    addr,
				Nonce:     hexutil.Uint64(3),
				FuelPrice: (*hexutil.Big)(big.NewInt(1)),
				FuelLimit: (*hexutil.Big)(big.NewInt(100)),
				Payload:   (hexutil.Bytes)(rawAssetPayload),
			},
			expectedErr: errors.New("invalid interaction type"),
		},
		{
			name: "rpc receipt fetched successfully",
			sendIXArgs: &rpcargs.IxArgs{
				Type:      common.IxAssetCreate,
				Sender:    addr,
				Nonce:     hexutil.Uint64(4),
				FuelPrice: (*hexutil.Big)(big.NewInt(1)),
				FuelLimit: (*hexutil.Big)(big.NewInt(100)),
				Payload:   (hexutil.Bytes)(rawAssetPayload),
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				FuelUsed:  hexutil.Big(*big.NewInt(100)),
				ExtraData: rawAssetPayload,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixn, err := ixAPI.Call(test.sendIXArgs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedReceipt, ixn)
		})
	}
}

func TestIx_ConstructInteraction(t *testing.T) {
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
				TransferValues: map[common.AssetID]*big.Int{
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(22),
				},
				PerceivedValues: map[common.AssetID]*big.Int{
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(99),
					tests.GetRandomAssetID(t, tests.RandomAddress(t)): big.NewInt(111),
				},
				FuelPrice: big.NewInt(1),
				FuelLimit: big.NewInt(23),
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

func TestIx_ValidateArgumentsWithSign(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	ixWithNilSender := common.SendIXArgs{
		Sender: common.NilAddress,
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

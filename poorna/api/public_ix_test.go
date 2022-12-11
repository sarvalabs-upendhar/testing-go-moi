package api

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/guna"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"

	"github.com/sarvalabs/moichain/types"
)

// Interaction Api Testcases
func TestIx_SendInteraction(t *testing.T) {
	t.Helper()

	address := tests.RandomAddress(t)
	genesisAddress := guna.SargaAddress
	ixpool := NewIxPool()
	stateManager := NewMockStateManager(t)
	cfg := new(common.IxPoolConfig)
	cfg.Mode = 0
	cfg.PriceLimit = big.NewInt(100)

	ixpool.setNonce(address, 5)
	stateManager.setAccounts(address, 5)

	ixAPI := NewPublicIXAPI(ixpool, stateManager, cfg)

	assetPayload, err := json.Marshal(AssetCreationArgs{
		Type:           types.AssetKindValue,
		Symbol:         "GR",
		Supply:         hex.EncodeToString(big.NewInt(100).Bytes()),
		Dimension:      1,
		IsFungible:     true,
		IsMintable:     false,
		IsTransferable: true,
	})
	if err != nil {
		log.Panic(err)
	}

	expectedIxnArgs := SendIXArgs{
		Type:      types.IxAssetCreate,
		Sender:    address.String(),
		FuelPrice: hex.EncodeToString(big.NewInt(100).Bytes()),
		Payload:   assetPayload,
	}

	expectedIxns, err := constructInteraction(&expectedIxnArgs, 5)
	if err != nil {
		log.Panic(err)
	}

	testcases := []struct {
		name        string
		args        SendIXArgs
		expected    *types.Interaction
		expectedErr error
	}{
		{
			name: "Invalid account",
			args: SendIXArgs{
				Type:      1,
				Sender:    "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				FuelPrice: hex.EncodeToString(big.NewInt(100).Bytes()),
				Payload:   assetPayload,
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Genesis account",
			args: SendIXArgs{
				Type:      1,
				Sender:    genesisAddress.String(),
				FuelPrice: hex.EncodeToString(big.NewInt(100).Bytes()),
				Payload:   assetPayload,
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name:     "Valid account",
			args:     expectedIxnArgs,
			expected: expectedIxns,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(testing *testing.T) {
			ixn, err := ixAPI.SendInteraction(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected, ixn)
			}
		})
	}
}

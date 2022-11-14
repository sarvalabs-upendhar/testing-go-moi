package api

import (
	"log"
	"testing"

	"gitlab.com/sarvalabs/moichain/guna"

	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/tests"

	"gitlab.com/sarvalabs/moichain/types"
)

// Interaction Api Testcases
func TestIx_SendInteraction(t *testing.T) {
	t.Helper()

	address := tests.RandomAddress(t)
	genesisAddress := guna.GenesisAddress
	ixpool := NewIxPool()
	stateManager := NewMockStateManager(t)
	cfg := new(common.IxPoolConfig)
	cfg.Mode = 0
	cfg.PriceLimit = 100

	ixpool.setNonce(address, 5)
	stateManager.setAccounts(address, 5)

	ixAPI := NewPublicIXAPI(ixpool, stateManager, cfg)

	expectedIxnArgs := SendIXArgs{
		IxType:   1,
		From:     address.String(),
		AnuPrice: 100,
		AssetCreation: AssetCreation{
			Symbol:      "GR",
			TotalSupply: 100,
			IsFungible:  true,
			IsMintable:  false,
			Dimension:   1,
		},
	}

	expectedIxns, err := constructInteraction(&expectedIxnArgs, 5)
	if err != nil {
		log.Panic(err)
	}

	testcases := []struct {
		name        string
		args        SendIXArgs
		expected    types.Interactions
		expectedErr error
	}{
		{
			name: "Invalid account",
			args: SendIXArgs{
				IxType:   1,
				From:     "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				AnuPrice: 100,
				AssetCreation: AssetCreation{
					Symbol:      "GR",
					TotalSupply: 100,
					IsFungible:  true,
					IsMintable:  false,
					Dimension:   1,
				},
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Genesis account",
			args: SendIXArgs{
				IxType:   1,
				From:     genesisAddress.String(),
				AnuPrice: 100,
				AssetCreation: AssetCreation{
					Symbol:      "GR",
					TotalSupply: 100,
					IsFungible:  true,
					IsMintable:  false,
					Dimension:   1,
				},
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
			ixns, err := ixAPI.SendInteraction(&testcase.args)
			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testcase.expected[0], ixns[0])
			}
		})
	}
}

package api

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/guna"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Interaction Api Testcases
func TestIx_SendInteraction(t *testing.T) {
	t.Helper()

	address := tests.RandomAddress(t)
	genesisAddress := guna.SargaAddress
	ixpool := NewMockIxPool(t)
	stateManager := NewMockStateManager(t)
	cfg := new(common.IxPoolConfig)
	cfg.Mode = 0
	cfg.PriceLimit = big.NewInt(100)

	ixpool.setNonce(address, 5)

	acc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
		acc.Nonce = uint64(5)
	})
	stateManager.setAccount(address, *acc)

	ixAPI := NewPublicIXAPI(ixpool, stateManager)

	assetPayload, err := json.Marshal(ptypes.AssetCreationArgs{
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

	expectedIxnArgs := ptypes.SendIXArgs{
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
		args        ptypes.SendIXArgs
		expected    *types.Interaction
		expectedErr error
	}{
		{
			name: "Invalid account",
			args: ptypes.SendIXArgs{
				Type:      1,
				Sender:    "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
				FuelPrice: hex.EncodeToString(big.NewInt(100).Bytes()),
				Payload:   nil,
			},
			expectedErr: types.ErrInvalidAddress,
		},
		{
			name: "Genesis account",
			args: ptypes.SendIXArgs{
				Type:      1,
				Sender:    genesisAddress.String(),
				FuelPrice: hex.EncodeToString(big.NewInt(100).Bytes()),
				Payload:   nil,
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

func TestGetRawIXPayloadForLogicDeploy(t *testing.T) {
	nonce := uint64(0)
	address := tests.RandomAddress(t)

	tableTests := []struct {
		name               string
		deployArgsCallback func(args *ptypes.LogicDeployArgs)
		error              error
	}{
		{
			name: "should fail for empty manifest",
			deployArgsCallback: func(args *ptypes.LogicDeployArgs) {
				args.Manifest = ""
			},
			error: types.ErrEmptyManifest,
		},
		{
			name:               "should pass for valid data",
			deployArgsCallback: nil,
			error:              nil,
		},
	}

	for _, test := range tableTests {
		t.Run(test.name, func(t *testing.T) {
			// Create test payload
			jsonPayload, poloPayload := GetTestLogicDeployPayload(t, nonce, address, test.deployArgsCallback)
			obtainedPayload, err := GetRawIXPayloadForLogicDeploy(jsonPayload, nonce, address)
			if test.error != nil {
				require.ErrorContains(t, err, test.error.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, poloPayload, obtainedPayload)
			}
		})
	}
}

func TestGetRawIXPayloadForAssetCreation(t *testing.T) {
	tableTests := []struct {
		name                  string
		assetCreationCallback func(args *ptypes.AssetCreationArgs)
		error                 error
	}{
		{
			name: "should fail for invalid supply",
			assetCreationCallback: func(args *ptypes.AssetCreationArgs) {
				args.Supply = "h123"
			},
			error: errors.New("failed to decode supply"),
		},
		{
			name:                  "should pass for valid data",
			assetCreationCallback: nil,
			error:                 nil,
		},
	}

	for _, test := range tableTests {
		t.Run(test.name, func(t *testing.T) {
			// Create test payload
			jsonPayload, poloPayload := GetTestIxCreationPayload(t, test.assetCreationCallback)
			obtainedPayload, err := GetRawIXPayloadForAssetCreation(jsonPayload)
			if err != nil {
				require.ErrorContains(t, err, test.error.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, poloPayload, obtainedPayload)
			}
		})
	}
}

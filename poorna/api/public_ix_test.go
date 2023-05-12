package api

import (
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/utils"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/common/tests"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Interaction Api Testcases
func TestIx_SendInteraction(t *testing.T) {
	t.Helper()

	address := tests.RandomAddress(t)
	genesisAddress := types.SargaAddress
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

	dimension := uint8(1)
	ixAPI := NewPublicIXAPI(ixpool, stateManager)

	assetPayload, err := json.Marshal(ptypes.RPCAssetCreation{
		Type:           types.AssetKindValue,
		Symbol:         "GR",
		Supply:         (*hexutil.Big)(big.NewInt(100)),
		Dimension:      (*hexutil.Uint8)(&dimension),
		IsFungible:     true,
		IsMintable:     false,
		IsTransferable: true,
	})
	if err != nil {
		log.Panic(err)
	}

	expectedIxnArgs := ptypes.SendIXArgs{
		Type:      types.IxAssetCreate,
		Sender:    address,
		Nonce:     (*hexutil.Uint64)(utils.NewUint64(uint64(5))),
		FuelPrice: (*hexutil.Big)(big.NewInt(100)),
		Payload:   assetPayload,
	}

	expectedIxns, err := constructInteraction(&expectedIxnArgs)
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
				Sender:    types.SargaAddress,
				FuelPrice: (*hexutil.Big)(big.NewInt(100)),
				Payload:   nil,
			},
			expectedErr: ErrGenesisAccount,
		},
		{
			name: "Genesis account",
			args: ptypes.SendIXArgs{
				Type:      1,
				Sender:    genesisAddress,
				FuelPrice: (*hexutil.Big)(big.NewInt(100)),
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
				require.EqualError(t, testcase.expectedErr, err.Error())
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
		deployArgsCallback func(args *ptypes.RPCLogicPayload)
		error              error
	}{
		{
			name: "should fail for empty manifest",
			deployArgsCallback: func(args *ptypes.RPCLogicPayload) {
				args.Manifest = hexutil.Bytes{}
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
		assetCreationCallback func(args *ptypes.RPCAssetCreation)
		error                 error
	}{
		{
			name: "should fail for invalid supply",
			assetCreationCallback: func(args *ptypes.RPCAssetCreation) {
				supply, _ := new(big.Int).SetString("h1234", 16)
				args.Supply = (*hexutil.Big)(supply)
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

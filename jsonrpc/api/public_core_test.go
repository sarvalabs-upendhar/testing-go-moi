package api

import (
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/jsonrpc/websocket"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Core Api Testcases

func TestPublicCoreAPI_GetRPCTesseract(t *testing.T) {
	height := uint64(8)

	assetPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	ix := tests.CreateIX(t, rpcargs.GetIxParamsWithInputComputeTrust(common.IxAssetCreate, assetPayloadBytes, 2, 3))

	tesseractParams := &tests.CreateTesseractParams{
		Address: tests.RandomAddress(t),
		Height:  height,
		Ixns:    []*common.Interaction{ix},
		Receipts: map[common.Hash]*common.Receipt{
			tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
		},
		Seal:           []byte{1, 2},
		HeaderCallback: tests.HeaderCallbackWithGridHash(t),
		BodyCallback: func(body *common.TesseractBody) {
			body.StateHash = tests.RandomHash(t)
		},
	}

	ts := tests.CreateTesseract(t, tesseractParams)

	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)
	hash := getTesseractHash(t, ts)

	testcases := []struct {
		name          string
		args          rpcargs.TesseractArgs
		expectedError error
	}{
		{
			name: "get rpc tesseract returns error",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{},
			},
			expectedError: common.ErrEmptyOptions,
		},
		{
			name: "get rpc tesseract by height with interactions",
			args: rpcargs.TesseractArgs{
				WithInteractions: true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &hash,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.GetRPCTesseract(&test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			rpcargs.CheckForRPCTesseract(t, ts, fetchedTesseract)
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 1, 2)
	ts := tests.CreateTesseracts(t, 1, tesseractParams)

	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[0])

	testcases := []struct {
		name             string
		hash             common.Hash
		withInteractions bool
		expectedTS       *common.Tesseract
		expectedError    error
	}{
		{
			name:             "fetch tesseract with interactions",
			hash:             tsHash,
			withInteractions: true,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch tesseract without interactions",
			hash:             tsHash,
			withInteractions: false,
			expectedTS:       ts[0],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.getTesseractByHash(test.hash, test.withInteractions)

			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.CheckForTesseract(t, test.expectedTS, fetchedTesseract, test.withInteractions)
		})
	}
}

func TestPublicCoreAPI_GetTesseractHashByHeight(t *testing.T) {
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)
	acc := tests.GetRandomAccMetaInfo(t, 8)

	stateManager.setAccountMetaInfo(t, acc.Address, acc)

	address := tests.RandomAddress(t)
	height := uint64(66)
	hash := tests.RandomHash(t)

	chainManager.SetTesseractHeightEntry(address, height, hash)

	testcases := []struct {
		name          string
		address       common.Address
		height        int64
		expectedHash  common.Hash
		expectedError error
	}{
		{
			name:          "invalid address",
			address:       common.NilAddress,
			expectedError: common.ErrInvalidAddress,
		},
		{
			name:          "failed fetch tesseract hash for latest tesseract height",
			address:       tests.RandomAddress(t),
			height:        -1,
			expectedError: common.ErrAccountNotFound,
		},
		{
			name:         "fetch tesseract hash for latest tesseract height",
			address:      acc.Address,
			height:       -1,
			expectedHash: acc.TesseractHash,
		},
		{
			name:         "fetch tesseract hash for given tesseract height",
			address:      address,
			height:       int64(height),
			expectedHash: hash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hash, err := coreAPI.getTesseractHashByHeight(test.address, test.height)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedHash, hash)
		})
	}
}

func TestPublicCoreAPI_FuelEstimate(t *testing.T) {
	assetPayload := common.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	exec := NewMockExecutionManager(t)
	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, exec, nil, nil)

	ts := tests.CreateTesseracts(t, 1, nil)
	chainManager.setTesseractByHash(t, ts[0])

	tsHash := getTesseractHash(t, ts[0])

	IxArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Sender:    ts[0].Address(),
		Nonce:     4,
		FuelPrice: big.NewInt(1),
		FuelLimit: 100,
		Payload:   rawAssetPayload,
	}

	ix, err := constructInteraction(IxArgs, nil)
	assert.NoError(t, err)

	receipt := &common.Receipt{
		FuelUsed: 100,
	}

	exec.setInteractionCall(ix, receipt)

	testcases := []struct {
		name                 string
		callArgs             *rpcargs.CallArgs
		expectedFuelConsumed *hexutil.Big
		expectedErr          error
	}{
		{
			name: "failed to construct interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxInvalid,
					Sender:    tests.RandomAddress(t),
					Nonce:     hexutil.Uint64(3),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
			},
			expectedErr: errors.New("invalid interaction type"),
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    tests.RandomAddress(t),
					Nonce:     hexutil.Uint64(3),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts[0].Address(): {
						TesseractNumber: nil,
					},
				},
			},
			expectedErr: common.ErrEmptyOptions,
		},
		{
			name: "should return error as rpc receipt fetch failed",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    tests.RandomAddress(t),
					Nonce:     hexutil.Uint64(4),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts[0].Address(): {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "rpc receipt fetched successfully",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    ts[0].Address(),
					Nonce:     hexutil.Uint64(4),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts[0].Address(): {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedFuelConsumed: (*hexutil.Big)(big.NewInt(100)),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fuelConsumed, err := coreAPI.FuelEstimate(test.callArgs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedFuelConsumed, fuelConsumed)
		})
	}
}

func TestIx_Call(t *testing.T) {
	assetPayload := common.AssetCreatePayload{}
	rawAssetPayload, err := assetPayload.Bytes()
	require.NoError(t, err)

	exec := NewMockExecutionManager(t)
	chainManager := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, exec, nil, nil)

	ts := tests.CreateTesseracts(t, 1, nil)
	chainManager.setTesseractByHash(t, ts[0])

	tsHash := getTesseractHash(t, ts[0])

	IxArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Sender:    ts[0].Address(),
		Nonce:     4,
		FuelPrice: big.NewInt(1),
		FuelLimit: 100,
		Payload:   rawAssetPayload,
	}

	ix, err := constructInteraction(IxArgs, nil)
	assert.NoError(t, err)

	receipt := &common.Receipt{
		IxType:    common.IxAssetCreate,
		IxHash:    ix.Hash(),
		FuelUsed:  100,
		ExtraData: rawAssetPayload,
	}

	exec.setInteractionCall(ix, receipt)

	testcases := []struct {
		name            string
		callArgs        *rpcargs.CallArgs
		expectedReceipt *rpcargs.RPCReceipt
		expectedErr     error
	}{
		{
			name: "failed to construct interaction",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxInvalid,
					Sender:    tests.RandomAddress(t),
					Nonce:     hexutil.Uint64(3),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
			},
			expectedErr: errors.New("invalid interaction type"),
		},
		{
			name: "failed to retrieve stateHashes as options are empty",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    tests.RandomAddress(t),
					Nonce:     hexutil.Uint64(4),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts[0].Address(): {
						TesseractNumber: nil,
					},
				},
			},
			expectedErr: common.ErrEmptyOptions,
		},
		{
			name: "should return error as rpc receipt fetch failed",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    tests.RandomAddress(t),
					Nonce:     hexutil.Uint64(4),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts[0].Address(): {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedErr: common.ErrAccountNotFound,
		},
		{
			name: "rpc receipt fetched successfully",
			callArgs: &rpcargs.CallArgs{
				IxArgs: &rpcargs.IxArgs{
					Type:      common.IxAssetCreate,
					Sender:    ts[0].Address(),
					Nonce:     hexutil.Uint64(4),
					FuelPrice: (*hexutil.Big)(big.NewInt(1)),
					FuelLimit: hexutil.Uint64(100),
					Payload:   (hexutil.Bytes)(rawAssetPayload),
				},
				Options: map[common.Address]*rpcargs.TesseractNumberOrHash{
					ts[0].Address(): {
						TesseractHash: &tsHash,
					},
				},
			},
			expectedReceipt: &rpcargs.RPCReceipt{
				IxType:    hexutil.Uint64(common.IxAssetCreate),
				IxHash:    ix.Hash(),
				FuelUsed:  hexutil.Uint64(100),
				ExtraData: rawAssetPayload,
				From:      ix.Sender(),
				To:        ix.Receiver(),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ixn, err := coreAPI.Call(test.callArgs)
			if test.expectedErr != nil {
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedReceipt, ixn)
		})
	}
}

func TestPublicCoreAPI_GetTesseract(t *testing.T) {
	height := int64(8)
	invalidHeight := int64(-2)
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseractParams[0].Height = uint64(height)

	ts := tests.CreateTesseracts(t, 2, tesseractParams)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].Address(), ts[0].Height(), getTesseractHash(t, ts[0]))

	chainManager.setTesseractByHash(t, ts[1])
	chainManager.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[1])

	testcases := []struct {
		name          string
		args          rpcargs.TesseractArgs
		expectedTS    *common.Tesseract
		expectedError error
	}{
		{
			name: "should return error if both options are provided",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
					TesseractHash:   &tsHash,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
		{
			name: "should return error if options are empty",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{},
			},
			expectedError: common.ErrEmptyOptions,
		},
		{
			name: "should return error if address is nil",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedError: common.ErrInvalidAddress,
		},
		{
			name: "should return error if height is invalid",
			args: rpcargs.TesseractArgs{
				Address: ts[0].Address(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "should return error if address is invalid",
			args: rpcargs.TesseractArgs{
				Address: tests.RandomAddress(t),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &rpcargs.LatestTesseractHeight,
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "get tesseract by height with interactions",
			args: rpcargs.TesseractArgs{
				Address:          ts[0].Address(),
				WithInteractions: true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTS: ts[0],
		},
		{
			name: "get tesseract by height without interactions",
			args: rpcargs.TesseractArgs{
				Address:          ts[0].Address(),
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTS: ts[0],
		},
		{
			name: "get tesseract by hash with interactions",
			args: rpcargs.TesseractArgs{
				Address:          ts[1].Address(),
				WithInteractions: true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedTS: ts[1],
		},
		{
			name: "get tesseract by hash tesseract without interactions",
			args: rpcargs.TesseractArgs{
				Address:          ts[1].Address(),
				WithInteractions: false,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedTS: ts[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.getTesseract(&test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.CheckForTesseract(t, test.expectedTS, fetchedTesseract, test.args.WithInteractions)
		})
	}
}

func TestPublicCoreAPI_GetBalance(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)
	assetID, _ := tests.CreateTestAsset(t, tests.RandomAddress(t))

	stateManager.setBalance(ts.Address(), assetID, big.NewInt(300))

	testcases := []struct {
		name            string
		args            rpcargs.BalArgs
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name: "should return error if failed to fetch balance",
			args: rpcargs.BalArgs{
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "fetched balance successfully",
			args: rpcargs.BalArgs{
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedBalance: big.NewInt(300),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			balance, err := coreAPI.GetBalance(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedBalance, balance.ToInt())
		})
	}
}

func TestPublicCoreAPI_GetContextInfo(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	context := getContext(t, 2)
	stateManager.setContext(t, ts[0].Address(), context)

	tsHash := getTesseractsHashes(t, ts)
	randomHash := tests.RandomHash(t)

	testcases := []struct {
		name            string
		args            rpcargs.ContextInfoArgs
		expectedContext *Context
		expectedError   error
	}{
		{
			name: "fetched context info successfully",
			args: rpcargs.ContextInfoArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedContext: context,
		},
		{
			name: "should return error if tesseract not found",
			args: rpcargs.ContextInfoArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if context not found",
			args: rpcargs.ContextInfoArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: common.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			behaviouralNodes, randomNodes, err := coreAPI.GetContextInfo(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			checkForContext(t, test.expectedContext, behaviouralNodes, randomNodes)
		})
	}
}

func TestPublicCoreAPI_GetTDU(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)
	assetID, _ := tests.CreateTestAsset(t, tests.RandomAddress(t))

	stateManager.setBalance(ts[0].Address(), assetID, big.NewInt(300))

	testcases := []struct {
		name          string
		args          rpcargs.QueryArgs
		expectedTDU   common.AssetMap
		expectedError error
	}{
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: rpcargs.QueryArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: rpcargs.QueryArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedTDU: stateManager.getTDU(ts[0].Address(), common.NilHash),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tdu, err := coreAPI.GetTDU(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			for _, expectedAmount := range test.expectedTDU {
				amount := tdu[0].Amount
				require.Equal(t, expectedAmount, amount.ToInt())
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionCount(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	latestNonce := uint64(5)
	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(5)
	})

	stateManager.setAccount(ts.Address(), *acc)

	testcases := []struct {
		name          string
		args          *rpcargs.InteractionCountArgs
		expectedNonce uint64
		expectedError error
	}{
		{
			name: "interaction count fetched successfully",
			args: &rpcargs.InteractionCountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedNonce: latestNonce,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &rpcargs.InteractionCountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedNonce, err := coreAPI.GetInteractionCount(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedNonce, fetchedNonce.ToUint64())
		})
	}
}

func TestPublicIXPoolAPI_GetPendingInteractionCount(t *testing.T) {
	address := tests.RandomAddress(t)

	ixpool := NewMockIxPool(t)
	ixpool.setNonce(address, 5)

	coreAPI := NewPublicCoreAPI(ixpool, nil, nil, nil, nil, nil)

	testcases := []struct {
		name            string
		args            *rpcargs.InteractionCountArgs
		expectedIxCount uint64
		expectedErr     error
	}{
		{
			name: "valid address with state",
			args: &rpcargs.InteractionCountArgs{
				Address: address,
			},
			expectedIxCount: 5,
		},
		{
			name: "valid address without state",
			args: &rpcargs.InteractionCountArgs{
				Address: tests.RandomAddress(t),
			},
			expectedIxCount: 0,
			expectedErr:     common.ErrAccountNotFound,
		},
		{
			name: "nil address",
			args: &rpcargs.InteractionCountArgs{
				Address: common.NilAddress,
			},
			expectedIxCount: 0,
			expectedErr:     common.ErrInvalidAddress,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixCount, err := coreAPI.GetPendingInteractionCount(testcase.args)

			if testcase.expectedErr != nil {
				require.EqualError(t, testcase.expectedErr, err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.expectedIxCount, ixCount.ToUint64())
		})
	}
}

func TestPublicCoreAPI_GetAccountState(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(5)
		acc.AccType = common.RegularAccount
		acc.Balance = tests.RandomHash(t)
		acc.AssetRegistry = tests.RandomHash(t)
		acc.AssetApprovals = tests.RandomHash(t)
		acc.ContextHash = tests.RandomHash(t)
		acc.StorageRoot = tests.RandomHash(t)
		acc.LogicRoot = tests.RandomHash(t)
		acc.FileRoot = tests.RandomHash(t)
	})

	stateManager.setAccount(ts.Address(), *acc)

	testcases := []struct {
		name          string
		args          *rpcargs.GetAccountArgs
		expectedAcc   *common.Account
		expectedError error
	}{
		{
			name: "account state fetched successfully",
			args: &rpcargs.GetAccountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAcc: acc,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &rpcargs.GetAccountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAcc, err := coreAPI.GetAccountState(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, hexutil.Uint64(test.expectedAcc.Nonce), fetchedAcc["nonce"])
			require.Equal(t, test.expectedAcc.AccType, fetchedAcc["acc_type"])
			require.Equal(t, test.expectedAcc.Balance, fetchedAcc["balance"])
			require.Equal(t, test.expectedAcc.AssetApprovals, fetchedAcc["asset_approvals"])
			require.Equal(t, test.expectedAcc.AssetRegistry, fetchedAcc["asset_registry"])
			require.Equal(t, test.expectedAcc.ContextHash, fetchedAcc["context_hash"])
			require.Equal(t, test.expectedAcc.StorageRoot, fetchedAcc["storage_root"])
			require.Equal(t, test.expectedAcc.LogicRoot, fetchedAcc["logic_root"])
			require.Equal(t, test.expectedAcc.FileRoot, fetchedAcc["file_root"])
		})
	}
}

func TestPublicCoreAPI_GetLogicIDs(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts[0])
	chainManager.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	logicIDs := make([]common.LogicID, 0, 3)

	for i := 0; i < 3; i++ {
		logicID := common.NewLogicIDv0(true, false, false, false, uint16(i), tests.RandomAddress(t))

		logicIDs = append(logicIDs, logicID)
	}

	stateManager.setLogicIDs(t, ts[0].Address(), logicIDs)

	testcases := []struct {
		name             string
		args             rpcargs.GetAccountArgs
		expectedLogicIDs []common.LogicID
		expectedError    error
	}{
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAccountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if logicIDs not found",
			args: rpcargs.GetAccountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("logic IDs not found"),
		},
		{
			name: "fetched logicIDs successfully",
			args: rpcargs.GetAccountArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedLogicIDs: logicIDs,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicIDs, err := coreAPI.GetLogicIDs(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedLogicIDs, logicIDs)
		})
	}
}

func TestPublicCoreAPI_GetRegistry(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil, nil, nil)

	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	assetIDs, assetDescriptors := tests.CreateTestAssets(t, 3)

	registryMap, registryEntries := getRegistry(t, assetIDs, assetDescriptors)
	sortRegistry(registryEntries)

	s.setRegistry(t, ts[0].Address(), registryMap)

	testcases := []struct {
		name             string
		args             rpcargs.QueryArgs
		expectedRegistry []rpcargs.RPCRegistry
		expectedError    error
	}{
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if registry not found",
			args: rpcargs.QueryArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("registry not found"),
		},
		{
			name: "should fetch registry successfully",
			args: rpcargs.QueryArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedRegistry: registryEntries,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			registry, err := coreAPI.GetRegistry(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			sortRegistry(registry)
			require.Equal(t, test.expectedRegistry, registry)
		})
	}
}

func TestPublicCoreAPI_GetLogicManifest(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	logicID := tests.GetLogicID(t, ts.Address())
	logicIDWithoutState := tests.GetLogicID(t, tests.RandomAddress(t))

	poloManifest, jsonManifest, yamlManifest := tests.GetManifests(t, "./../../compute/manifests/ledger.yaml")

	stateManager.setLogicManifest(logicID.String(), poloManifest)
	chainManager.setTesseractByHash(t, ts)

	testcases := []struct {
		name                  string
		args                  *rpcargs.LogicManifestArgs
		expectedLogicManifest []byte
		expectedError         error
	}{
		{
			name: "returns error if failed to fetch logic manifest",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicIDWithoutState,
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "fetched json encoded logic manifest successfully",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "JSON",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: jsonManifest,
		},
		{
			name: "fetched polo encoded logic manifest successfully",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "POLO",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: poloManifest,
		},
		{
			name: "fetched yaml encoded logic manifest successfully",
			args: &rpcargs.LogicManifestArgs{
				LogicID:  logicID,
				Encoding: "YAML",
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: yamlManifest,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := coreAPI.GetLogicManifest(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedLogicManifest, manifest.Bytes())
		})
	}
}

func TestPublicCoreAPI_GetLogicStorage(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	logicID := tests.GetLogicID(t, tests.RandomAddress(t))
	logicIDWithoutState := tests.GetLogicID(t, tests.RandomAddress(t))

	chainManager.setTesseractByHash(t, ts)

	keys := getHexEntries(t, 1)
	values := getHexEntries(t, 1)

	stateManager.SetStorageEntry(logicID, getStorageMap(keys, values))

	testcases := []struct {
		name          string
		args          *rpcargs.GetLogicStorageArgs
		expectedValue []byte
		expectedError error
	}{
		{
			name: "returns error if failed to fetch logic manifest",
			args: &rpcargs.GetLogicStorageArgs{
				LogicID: logicIDWithoutState,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "fetched logic manifest successfully",
			args: &rpcargs.GetLogicStorageArgs{
				LogicID:    logicID,
				StorageKey: common.Hex2Bytes(keys[0]),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedValue: []byte(values[0]),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := coreAPI.GetLogicStorage(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedValue, value.Bytes())
		})
	}
}

func TestPublicCoreAPI_GetInteractionByTSHash(t *testing.T) {
	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	ixParams := tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t))
	ix := tests.CreateIX(t, ixParams)
	tsHash := tests.RandomHash(t)
	parts := tests.CreateTesseractPartsWithTestData(t)
	chainManager.SetInteractionDataByTSHash(tsHash, ix, parts)

	address := tests.RandomAddress(t)
	height := int64(88)
	chainManager.SetTesseractHeightEntry(address, uint64(height), tsHash)

	randomHeight := int64(6)
	negativeHeight := int64(-99)
	randomHash := tests.RandomHash(t)
	ixIndex := uint64(8)

	testcases := []struct {
		name          string
		args          rpcargs.InteractionByTesseract
		expectedIX    *common.Interaction
		expectedParts *common.TesseractParts
		expectedError error
	}{
		{
			name: "can not use both tesseract number and tesseract hash",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
					TesseractHash:   &randomHash,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
		{
			name: "empty options",
			args: rpcargs.InteractionByTesseract{
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrEmptyOptions,
		},
		{
			name: "interaction not found",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name: "hash not available for given tesseract number and address",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: common.ErrInvalidAddress,
		},
		{
			name: "ix index is nil",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
			},
			expectedError: common.ErrIXIndex,
		},
		{
			name: "invalid options",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &negativeHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "fetch interaction by tesseract hash successfully",
			args: rpcargs.InteractionByTesseract{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedIX:    ix,
			expectedParts: parts,
		},
		{
			name: "fetch interaction by tesseract number successfully",
			args: rpcargs.InteractionByTesseract{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedIX:    ix,
			expectedParts: parts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIX, err := coreAPI.GetInteractionByTesseract(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			rpcargs.CheckForRPCIxn(t, test.expectedIX, rpcIX, test.expectedParts.Grid)
			require.Equal(t, *test.args.IxIndex, rpcIX.IxIndex)
		})
	}
}

func TestPublicCoreAPI_GetInteractionByHash(t *testing.T) {
	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t)),
		1: tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t)),
	}
	ixns := tests.CreateIxns(t, 2, ixParams)
	parts := tests.CreateTesseractPartsWithTestData(t)
	ixIndex := 8

	ixpool := NewMockIxPool(t)
	ixpool.SetPendingIx(ixns[1])

	testcases := []struct {
		name           string
		args           rpcargs.InteractionByHashArgs
		setHook        func(c *MockChainManager)
		expectedIXData ixData
		expectedError  error
	}{
		{
			name: "invalid tesseract hash",
			args: rpcargs.InteractionByHashArgs{
				Hash: common.NilHash,
			},
			expectedError: common.ErrInvalidHash,
		},
		{
			name: "fetch pending interaction successfully",
			args: rpcargs.InteractionByHashArgs{
				Hash: ixns[1].Hash(),
			},
			setHook: func(c *MockChainManager) {
				c.GetInteractionByIxHashHook = func() error {
					return common.ErrGridHashNotFound
				}
			},
			expectedIXData: ixData{
				ix:      ixns[1],
				parts:   nil,
				ixIndex: 0,
			},
		},
		{
			name: "fetch finalized interaction successfully",
			args: rpcargs.InteractionByHashArgs{
				Hash: ixns[0].Hash(),
			},
			expectedIXData: ixData{
				ix:      ixns[0],
				parts:   parts,
				ixIndex: ixIndex,
			},
		},
		{
			name: "should return error if failed to fetch finalized interaction",
			args: rpcargs.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name: "should return error if failed to fetch pending interaction",
			args: rpcargs.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			setHook: func(c *MockChainManager) {
				c.GetInteractionByIxHashHook = func() error {
					return common.ErrGridHashNotFound
				}
			},
			expectedError: common.ErrFetchingInteraction,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			chainManager := NewMockChainManager(t)
			if test.setHook != nil {
				test.setHook(chainManager)
			}

			chainManager.SetInteractionDataByIxHash(ixns[0], parts, ixIndex)

			coreAPI := NewPublicCoreAPI(ixpool, chainManager, nil, nil, nil, nil)

			rpcIX, err := coreAPI.GetInteractionByHash(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if test.expectedIXData.parts != nil {
				rpcargs.CheckForRPCIxn(t, test.expectedIXData.ix, rpcIX, test.expectedIXData.parts.Grid)
			}

			require.Equal(t, uint64(test.expectedIXData.ixIndex), rpcIX.IxIndex.ToUint64())
		})
	}
}

func TestPublicCoreAPI_GetInteractionReceipt(t *testing.T) {
	chainManager := NewMockChainManager(t)

	ixHashWithoutParts := tests.RandomHash(t)
	ixParams := tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t))
	ix := tests.CreateIX(t, ixParams)
	parts := tests.CreateTesseractPartsWithTestData(t)
	ixIndex := 8

	receipt := tests.CreateReceiptWithTestData(t)
	receipt.IxHash = ix.Hash()

	chainManager.SetInteractionDataByIxHash(ix, parts, ixIndex)
	chainManager.setReceiptByIXHash(receipt.IxHash, receipt)
	chainManager.setReceiptByIXHash(ixHashWithoutParts, receipt)

	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil, nil, nil)

	testcases := []struct {
		name            string
		args            rpcargs.ReceiptArgs
		expectedReceipt *common.Receipt
		ix              *common.Interaction
		grid            map[common.Address]common.TesseractHeightAndHash
		ixIndex         int
		expectedError   error
	}{
		{
			name: "nil hash",
			args: rpcargs.ReceiptArgs{
				Hash: common.NilHash,
			},
			expectedError: common.ErrInvalidHash,
		},
		{
			name: "failed to fetch receipt",
			args: rpcargs.ReceiptArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: common.ErrReceiptNotFound,
		},
		{
			name: "failed to fetch interaction",
			args: rpcargs.ReceiptArgs{
				Hash: ixHashWithoutParts,
			},
			expectedError: common.ErrFetchingInteraction,
		},
		{
			name: "fetched receipt successfully",
			args: rpcargs.ReceiptArgs{
				Hash: ix.Hash(),
			},
			expectedReceipt: receipt,
			ix:              ix,
			grid:            parts.Grid,
			ixIndex:         ixIndex,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := coreAPI.GetInteractionReceipt(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			rpcargs.CheckForRPCReceipt(t, test.grid, test.ix, test.expectedReceipt, receipt, test.ixIndex)
		})
	}
}

func TestPublicCoreAPI_GetAssetInfoByAssetID(t *testing.T) {
	address := tests.RandomAddress(t)
	stateManager := NewMockStateManager(t)
	chainManager := NewMockChainManager(t)

	ts := tests.CreateTesseract(t, nil)
	tsHash := tests.GetTesseractHash(t, ts)
	chainManager.setTesseractByHash(t, ts)

	assetID, assetInfo := tests.CreateTestAsset(t, address)
	stateManager.addAsset(assetID, assetInfo)

	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	testcases := []struct {
		name                    string
		args                    *rpcargs.GetAssetInfoArgs
		expectedAssetDescriptor *common.AssetDescriptor
		isErrorExpected         bool
	}{
		{
			name: "Valid asset id",
			args: &rpcargs.GetAssetInfoArgs{
				AssetID: assetID,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAssetDescriptor: assetInfo,
		},
		{
			name: "Valid asset id without state",
			args: &rpcargs.GetAssetInfoArgs{
				AssetID: tests.GetRandomAssetID(t, common.NilAddress),
			},
			isErrorExpected: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAssetInfo, err := coreAPI.GetAssetInfoByAssetID(test.args)
			if test.isErrorExpected {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAssetDescriptor.Symbol, fetchedAssetInfo["symbol"])
			require.Equal(t, test.expectedAssetDescriptor.Operator, fetchedAssetInfo["operator"])
			require.Equal(t, (*hexutil.Big)(test.expectedAssetDescriptor.Supply), fetchedAssetInfo["supply"])
			require.Equal(t, hexutil.Uint8(test.expectedAssetDescriptor.Dimension), fetchedAssetInfo["dimension"])
			require.Equal(t, hexutil.Uint16(test.expectedAssetDescriptor.Standard), fetchedAssetInfo["standard"])
			require.Equal(t, test.expectedAssetDescriptor.IsLogical, fetchedAssetInfo["is_logical"])
			require.Equal(t, test.expectedAssetDescriptor.IsStateFul, fetchedAssetInfo["is_stateful"])
			require.Equal(t, test.expectedAssetDescriptor.LogicID, fetchedAssetInfo["logic_id"])
		})
	}
}

func TestPublicCoreAPI_GetAccountMetaInfo(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.setTesseractByHash(t, ts)

	tsHash := tests.GetTesseractHash(t, ts)
	acc := tests.GetRandomAccMetaInfo(t, 1)

	stateManager.setAccountMetaInfo(t, ts.Address(), acc)

	testcases := []struct {
		name                string
		args                *rpcargs.GetAccountArgs
		expectedAccMetaInfo *common.AccountMetaInfo
		expectedError       error
	}{
		{
			name: "account meta info fetched successfully",
			args: &rpcargs.GetAccountArgs{
				Address: ts.Address(),
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAccMetaInfo: acc,
		},
		{
			name: "should return error if failed to fetch account meta info",
			args: &rpcargs.GetAccountArgs{
				Address: common.NilAddress,
			},
			expectedError: common.ErrInvalidAddress,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAccMetaInfo, err := coreAPI.AccountMetaInfo(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccMetaInfo.Type, fetchedAccMetaInfo["type"])
			require.Equal(t, test.expectedAccMetaInfo.Address, fetchedAccMetaInfo["address"])
			require.Equal(t, hexutil.Uint64(test.expectedAccMetaInfo.Height), fetchedAccMetaInfo["height"])
			require.Equal(t, test.expectedAccMetaInfo.TesseractHash, fetchedAccMetaInfo["tesseract_hash"])
			require.Equal(t, test.expectedAccMetaInfo.LatticeExists, fetchedAccMetaInfo["lattice_exists"])
			require.Equal(t, test.expectedAccMetaInfo.StateExists, fetchedAccMetaInfo["state_exists"])
		})
	}
}

func TestPublicCoreAPI_Syncing(t *testing.T) {
	addrs := tests.GetAddresses(t, 5)

	syncer := NewMockSyncer(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, syncer, nil)

	accSyncStatus := &rpcargs.AccSyncStatus{
		CurrentHeight:     2,
		ExpectedHeight:    4,
		IsPrimarySyncDone: true,
	}

	nodeSyncStatus := &rpcargs.NodeSyncStatus{
		TotalPendingAccounts:  2,
		PrincipalSyncDoneTime: hexutil.Uint64(time.Now().UnixNano()),
		IsPrincipalSyncDone:   true,
		IsInitialSyncDone:     false,
	}

	pendingNodeSyncStatus := &rpcargs.NodeSyncStatus{
		TotalPendingAccounts:  5,
		PendingAccounts:       addrs,
		PrincipalSyncDoneTime: hexutil.Uint64(time.Now().UnixNano()),
		IsPrincipalSyncDone:   true,
		IsInitialSyncDone:     false,
	}

	testcases := []struct {
		name                       string
		args                       *rpcargs.SyncStatusRequest
		preTestFn                  func()
		expectedSyncStatusResponse *rpcargs.SyncStatusResponse
		expectedError              error
	}{
		{
			name: "account sync status fetched successfully",
			args: &rpcargs.SyncStatusRequest{
				Address: addrs[0],
			},
			preTestFn: func() {
				syncer.setAccountSyncStatus(addrs[0], accSyncStatus)
			},
			expectedSyncStatusResponse: &rpcargs.SyncStatusResponse{
				AccSyncResp: accSyncStatus,
			},
		},
		{
			name: "node sync status with pending accounts fetched successfully",
			args: &rpcargs.SyncStatusRequest{
				PendingAccounts: true,
			},
			preTestFn: func() {
				syncer.setPendingNodeSyncStatus(pendingNodeSyncStatus)
			},
			expectedSyncStatusResponse: &rpcargs.SyncStatusResponse{
				NodeSyncResp: pendingNodeSyncStatus,
			},
		},
		{
			name: "node sync status without pending accounts fetched successfully",
			args: &rpcargs.SyncStatusRequest{
				PendingAccounts: false,
			},
			preTestFn: func() {
				syncer.setNodeSyncStatus(nodeSyncStatus)
			},
			expectedSyncStatusResponse: &rpcargs.SyncStatusResponse{
				NodeSyncResp: nodeSyncStatus,
			},
		},
		{
			name: "should return error if failed to fetch account sync status",
			args: &rpcargs.SyncStatusRequest{
				Address: tests.RandomAddress(t),
			},
			expectedError: common.ErrAccSyncStatusNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.preTestFn != nil {
				test.preTestFn()
			}

			syncStatus, err := coreAPI.Syncing(test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedSyncStatusResponse, syncStatus)
		})
	}
}

func TestPublicCoreAPI_normalizeOptions(t *testing.T) {
	invalidHeight := int64(-2)
	validHeight := int64(8)

	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseractParams[0].Height = uint64(validHeight)

	ts := tests.CreateTesseracts(t, 2, tesseractParams)

	chainManager := NewMockChainManager(t)
	stateManager := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, stateManager, nil, nil, nil)

	chainManager.SetTesseractHeightEntry(ts[0].Address(), ts[0].Height(), getTesseractHash(t, ts[0]))

	chainManager.setTesseractByHash(t, ts[1])
	chainManager.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[1])

	testcases := []struct {
		name                string
		options             map[common.Address]*rpcargs.TesseractNumberOrHash
		expectedStateHashes map[common.Address]common.Hash
		expectedError       error
	}{
		{
			name: "state hashes fetched successfully from tesseract hashes",
			options: map[common.Address]*rpcargs.TesseractNumberOrHash{
				ts[0].Address(): {
					TesseractNumber: &validHeight,
				},
				ts[1].Address(): {
					TesseractHash: &tsHash,
				},
			},
			expectedStateHashes: map[common.Address]common.Hash{
				ts[0].Address(): ts[0].StateHash(),
				ts[1].Address(): ts[1].StateHash(),
			},
		},
		{
			name: "should return error as tesseract height is invalid",
			options: map[common.Address]*rpcargs.TesseractNumberOrHash{
				ts[0].Address(): {
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			stateHashes, err := coreAPI.normalizeOptions(test.options)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedStateHashes, stateHashes)
		})
	}
}

func TestPublicCoreAPI_NewTesseractFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	filterResponse := coreAPI.NewTesseractFilter()
	require.NotEmpty(t, filterResponse.FilterID)

	res := filterManager.getTSFilter(filterResponse.FilterID)
	require.True(t, res)
}

func TestPublicCoreAPI_NewTesseractsByAccountFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	testcases := []struct {
		name           string
		args           *rpcargs.TesseractByAccountFilterArgs
		expectedResult bool
		expectedError  error
	}{
		{
			name: "setup ts by account filter successfully",
			args: &rpcargs.TesseractByAccountFilterArgs{
				Addr: tests.RandomAddress(t),
			},
			expectedResult: true,
		},
		{
			name: "setup ts by account filter with nil address",
			args: &rpcargs.TesseractByAccountFilterArgs{
				Addr: common.NilAddress,
			},
			expectedError: common.ErrInvalidAddress,
		},
	}

	for _, testcase := range testcases {
		filterResponse, err := coreAPI.NewTesseractsByAccountFilter(testcase.args)
		if !testcase.expectedResult {
			require.Equal(t, testcase.expectedError, err)

			return
		}

		require.NotEmpty(t, filterResponse.FilterID)
		res, ok := filterManager.getTSByAccFilter(filterResponse.FilterID)
		require.True(t, ok)
		require.Equal(t, testcase.args.Addr, res.tsByAccFilterParams)
	}
}

func TestPublicCoreAPI_NewLogFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	args := &websocket.LogQuery{}

	hash, err := common.PoloHash(args)
	if err != nil {
		log.Fatal(err)
	}

	filterResponse := coreAPI.NewLogFilter(args)
	require.NotEmpty(t, filterResponse.FilterID)

	res, ok := filterManager.getLogFilter(filterResponse.FilterID)
	require.True(t, ok)
	require.Equal(t, hash, res)
}

func TestPublicCoreAPI_PendingIxnsFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	filterResponse := coreAPI.PendingIxnsFilter()
	require.NotEmpty(t, filterResponse.FilterID)

	res := filterManager.getIxnsFilter(filterResponse.FilterID)
	require.True(t, res)
}

func TestPublicCoreAPI_RemoveFilter(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	reqArgs := &rpcargs.TesseractByAccountFilterArgs{
		Addr: tests.RandomAddress(t),
	}

	// add new tesseract by account filter
	filterResponse, err := coreAPI.NewTesseractsByAccountFilter(reqArgs)
	require.NoError(t, err)

	respArgs := &rpcargs.FilterArgs{
		FilterID: filterResponse.FilterID,
	}

	// remove new tesseract by account filter
	response := coreAPI.RemoveFilter(respArgs)
	require.True(t, response.Status)

	// removing already removed filter
	response = coreAPI.RemoveFilter(respArgs)
	require.False(t, response.Status)
}

func TestPublicCoreAPI_GetFilterChanges(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	reqArgs := &rpcargs.TesseractByAccountFilterArgs{
		Addr: tests.RandomAddress(t),
	}

	// add new tesseract by account filter
	filterResponse, err := coreAPI.NewTesseractsByAccountFilter(reqArgs)
	require.NoError(t, err)

	ts := tests.CreateTesseract(t, nil)
	rpcTS, err := rpcargs.CreateRPCTesseract(ts)
	require.NoError(t, err)

	filterManager.setTSByAccFilterChanges(filterResponse.FilterID, []*rpcargs.RPCTesseract{rpcTS})

	args := &rpcargs.FilterArgs{
		FilterID: filterResponse.FilterID,
	}

	// get filter changes for new tesseract by account filter
	respArgs, err := coreAPI.GetFilterChanges(args)
	require.NoError(t, err)

	resp, ok := respArgs.([]*rpcargs.RPCTesseract)
	require.True(t, ok)
	require.Equal(t, []*rpcargs.RPCTesseract{rpcTS}, resp)
}

func TestPublicCoreAPI_GetLogs(t *testing.T) {
	filterManager := NewMockFilterManager(t)
	coreAPI := NewPublicCoreAPI(nil, nil, nil, nil, nil, filterManager)

	addr := tests.RandomAddress(t)
	args := &websocket.LogQuery{
		StartHeight: 1,
		EndHeight:   10,
		Address:     addr,
		Topics:      nil,
	}

	filterResponse := coreAPI.NewLogFilter(args)
	require.NotEmpty(t, filterResponse.FilterID)

	dummyLogs := []*rpcargs.RPCLog{
		{
			Addresses: []common.Address{addr},
		},
	}

	filterManager.setLogs(filterResponse.FilterID, dummyLogs)

	query := &rpcargs.FilterQueryArgs{
		StartHeight: NumPointer(t, 1),
		EndHeight:   NumPointer(t, 10),
		Address:     addr,
		Topics:      nil,
	}
	logs, err := coreAPI.GetLogs(query)
	require.NoError(t, err)
	require.Equal(t, dummyLogs, logs)
}

package api

import (
	"math/big"
	"testing"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Core Api Testcases

func TestPublicCoreAPI_CreateRPCInteraction(t *testing.T) {
	invalidPayload := []byte{1}
	assetPayload := types.AssetPayload{
		Create: &types.AssetCreatePayload{
			Symbol: "MOI",
		},
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	logicPayload := &types.LogicPayload{
		Callsite: "call site",
	}

	logicPayloadBytes, err := polo.Polorize(logicPayload)
	require.NoError(t, err)

	testcases := []struct {
		name          string
		ixParams      *tests.CreateIxParams
		expectedError error
	}{
		{
			name:     "create rpc interaction for value transfer interaction",
			ixParams: getIxParamsWithInputComputeTrust(types.IxValueTransfer, nil, 2, 3),
		},
		{
			name:     "create rpc interaction for asset creation interaction",
			ixParams: getIxParamsWithInputComputeTrust(types.IxAssetCreate, assetPayloadBytes, 2, 3),
		},
		{
			name:     "create rpc interaction for logic deploy interaction",
			ixParams: getIxParamsWithInputComputeTrust(types.IxLogicDeploy, logicPayloadBytes, 4, 6),
		},
		{
			name:     "create rpc interaction for logic execute interaction",
			ixParams: getIxParamsWithInputComputeTrust(types.IxLogicInvoke, logicPayloadBytes, 6, 8),
		},
		{
			name:          "invalid interaction type",
			ixParams:      getIxParamsWithInputComputeTrust(types.IxAssetMint, logicPayloadBytes, 6, 8),
			expectedError: errors.New("invalid interaction type"),
		},
		{
			name:          "invalid payload",
			ixParams:      getIxParamsWithInputComputeTrust(types.IxAssetCreate, invalidPayload, 6, 8),
			expectedError: errors.New("failed to depolorize asset payload"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix := tests.CreateIX(t, test.ixParams)
			rpcIxn, err := createRPCInteraction(ix)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForRPCIxn(t, rpcIxn, ix)
		})
	}
}

func TestPublicCoreAPI_CreateRPCTesseract(t *testing.T) {
	invalidPayload := []byte{1}
	assetPayload := types.AssetPayload{
		Create: &types.AssetCreatePayload{
			Symbol: "MOI",
		},
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	// make sure to fill at least one field of every field of tesseract so that we can verify that every field is copied
	tesseractParams := &tests.CreateTesseractParams{
		Address: tests.RandomAddress(t),
		TesseractCallback: func(ts *types.Tesseract) {
			ts.Seal = []byte{1, 2}
			ts.Receipts = map[types.Hash]*types.Receipt{
				tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
			}
			ts.Body = types.TesseractBody{
				StateHash: tests.RandomHash(t),
			}
		},
	}

	testcases := []struct {
		name          string
		ixParams      *tests.CreateIxParams
		expectedError error
	}{
		{
			name:          "failed to create rpc tesseract",
			ixParams:      getIxParamsWithInputComputeTrust(types.IxAssetCreate, invalidPayload, 2, 3),
			expectedError: errors.New("failed to depolorize asset payload"),
		},
		{
			name:     "created rpc tesseract successfully",
			ixParams: getIxParamsWithInputComputeTrust(types.IxAssetCreate, assetPayloadBytes, 2, 3),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ix := tests.CreateIX(t, test.ixParams)
			tesseractParams.Ixns = []*types.Interaction{ix}
			ts := tests.CreateTesseract(t, tesseractParams)

			rpcTS, err := createRPCTesseract(ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForRPCTesseract(t, rpcTS, ts)
		})
	}
}

func TestPublicCoreAPI_GetRPCTesseract(t *testing.T) {
	height := uint64(8)
	argsHeight := int64(8)

	assetPayload := types.AssetPayload{
		Create: &types.AssetCreatePayload{
			Symbol: "MOI",
		},
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	ix := tests.CreateIX(t, getIxParamsWithInputComputeTrust(types.IxAssetCreate, assetPayloadBytes, 2, 3))

	tesseractParams := &tests.CreateTesseractParams{
		Address: tests.RandomAddress(t),
		Height:  height,
		Ixns:    []*types.Interaction{ix},
		TesseractCallback: func(ts *types.Tesseract) {
			ts.Seal = []byte{1, 2}
			ts.Receipts = map[types.Hash]*types.Receipt{
				tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
			}
			ts.Body = types.TesseractBody{
				StateHash: tests.RandomHash(t),
			}
		},
	}

	ts := tests.CreateTesseract(t, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil)

	c.setTesseractByHeight(t, ts)

	testcases := []struct {
		name          string
		args          ptypes.TesseractArgs
		expectedError error
	}{
		{
			name: "get rpc tesseract returns error",
			args: ptypes.TesseractArgs{
				Options: ptypes.TesseractNumberOrHash{},
			},
			expectedError: types.ErrEmptyOptions,
		},
		{
			name: "get rpc tesseract by height with interactions",
			args: ptypes.TesseractArgs{
				From:             ts.Address().String(),
				WithInteractions: true,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &argsHeight,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			fetchedTesseract, err := coreAPI.GetRPCTesseract(&test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForRPCTesseract(t, fetchedTesseract, ts)
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 1, 2)
	ts := tests.CreateTesseracts(t, 1, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil)

	c.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[0]).String()

	testcases := []struct {
		name             string
		hash             string
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "should return error if valid hash without state",
			hash:             tests.RandomHash(t).String(),
			withInteractions: false,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "should return error if hash is invalid",
			hash:             "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			withInteractions: false,
			expectedError:    types.ErrInvalidHash,
		},
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

func TestPublicCoreAPI_GetTesseractByHeight(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseractParams[0].Height = 8

	ts := tests.CreateTesseracts(t, 2, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil)

	c.setTesseractByHeight(t, ts[0])
	c.setLatestTesseract(t, ts[1])

	testcases := []struct {
		name             string
		from             string
		height           int64
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "should return error if address is invalid",
			from:             "68510188a8yff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			height:           8,
			withInteractions: false,
			expectedError:    types.ErrInvalidAddress,
		},
		{
			name:             "should return error if height doesn't exist",
			from:             ts[0].Address().String(),
			height:           9,
			withInteractions: true,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "fetch tesseract with interactions",
			from:             ts[0].Address().String(),
			height:           8,
			withInteractions: true,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch tesseract without interactions",
			from:             ts[0].Address().String(),
			height:           8,
			withInteractions: false,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch latest tesseract with interactions",
			from:             ts[1].Address().String(),
			height:           ptypes.LatestTesseractHeight,
			withInteractions: true,
			expectedTS:       ts[1],
		},
		{
			name:             "fetch latest tesseract without interactions",
			from:             ts[1].Address().String(),
			height:           ptypes.LatestTesseractHeight,
			withInteractions: false,
			expectedTS:       ts[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedTesseract, err := coreAPI.getTesseractByHeight(
				test.from,
				test.height,
				test.withInteractions,
			)

			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			tests.CheckForTesseract(t, test.expectedTS, fetchedTesseract, test.withInteractions)
		})
	}
}

func TestPublicCoreAPI_GetTesseract(t *testing.T) {
	height := int64(8)
	invalidHeight := int64(-2)
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseractParams[0].Height = uint64(height)

	ts := tests.CreateTesseracts(t, 2, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil)

	c.setTesseractByHeight(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	tsHash := tests.GetTesseractHash(t, ts[1]).String()

	testcases := []struct {
		name          string
		args          ptypes.TesseractArgs
		expectedTS    *types.Tesseract
		expectedError error
	}{
		{
			name: "should return error if both options are provided",
			args: ptypes.TesseractArgs{
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &height,
					TesseractHash:   &tsHash,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
		{
			name: "should return error if options are empty",
			args: ptypes.TesseractArgs{
				Options: ptypes.TesseractNumberOrHash{},
			},
			expectedError: types.ErrEmptyOptions,
		},
		{
			name: "should return error if height is invalid",
			args: ptypes.TesseractArgs{
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "get Tesseract by height with interactions",
			args: ptypes.TesseractArgs{
				From:             ts[0].Address().String(),
				WithInteractions: true,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTS: ts[0],
		},
		{
			name: "get tesseract by height tesseract without interactions",
			args: ptypes.TesseractArgs{
				From:             ts[0].Address().String(),
				WithInteractions: false,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedTS: ts[0],
		},
		{
			name: "get tesseract by hash with interactions",
			args: ptypes.TesseractArgs{
				From:             ts[1].Address().String(),
				WithInteractions: true,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedTS: ts[1],
		},
		{
			name: "get tesseract by hash tesseract without interactions",
			args: ptypes.TesseractArgs{
				From:             ts[1].Address().String(),
				WithInteractions: false,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedTS: ts[1],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t).String()
	tsHash := tests.GetTesseractHash(t, ts).String()
	assetID, _ := tests.CreateTestAsset(t, ts.Address())

	s.setBalance(ts.Address(), assetID, big.NewInt(300))
	address := ts.Address().String()

	testcases := []struct {
		name            string
		args            ptypes.BalArgs
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name: "should return error if failed to fetch balance",
			args: ptypes.BalArgs{
				From:    address,
				AssetID: string(assetID),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "should return error if asset Id is invalid",
			args: ptypes.BalArgs{
				From:    address,
				AssetID: "01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedError: types.ErrInvalidAssetID,
		},
		{
			name: "fetched balance successfully",
			args: ptypes.BalArgs{
				From:    address,
				AssetID: string(assetID),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedBalance: big.NewInt(300),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			balance, err := coreAPI.GetBalance(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedBalance, balance)
		})
	}
}

func TestPublicCoreAPI_GetContextInfo(t *testing.T) {
	ts := tests.CreateTesseracts(t, 2, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	context := getContext(t, 2)
	s.setContext(t, ts[0].Address(), context)
	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	address := ts[0].Address().String()
	tsHash := getTesseractsHashes(t, ts)
	randomHash := tests.RandomHash(t).String()

	testcases := []struct {
		name            string
		args            ptypes.ContextInfoArgs
		expectedContext *Context
		expectedError   error
	}{
		{
			name: "fetched context info successfully",
			args: ptypes.ContextInfoArgs{
				From: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedContext: context,
		},
		{
			name: "should return error if tesseract not found",
			args: ptypes.ContextInfoArgs{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "should return error if context not found",
			args: ptypes.ContextInfoArgs{
				From: ts[1].Address().String(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: types.ErrContextStateNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t).String()
	tsHash := getTesseractsHashes(t, ts)
	assetID, _ := tests.CreateTestAsset(t, ts[0].Address())

	s.setBalance(ts[0].Address(), assetID, big.NewInt(300))
	address := ts[0].Address().String()

	testcases := []struct {
		name          string
		args          ptypes.TesseractArgs
		expectedTDU   types.AssetMap
		expectedError error
	}{
		{
			name: "should return error if tesseract not found",
			args: ptypes.TesseractArgs{
				From: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: ptypes.TesseractArgs{
				From: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: types.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: ptypes.TesseractArgs{
				From: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedTDU: s.getTDU(ts[0].Address(), types.NilHash),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			data, err := coreAPI.GetTDU(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedTDU, data)
		})
	}
}

func TestPublicCoreAPI_GetInteractionCount(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t).String()
	tsHash := tests.GetTesseractHash(t, ts).String()
	latestNonce := uint64(5)
	acc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
		acc.Nonce = uint64(5)
	})

	s.setAccount(ts.Address(), *acc)

	testcases := []struct {
		name          string
		args          *ptypes.InteractionCountArgs
		expectedNonce uint64
		expectedError error
	}{
		{
			name: "interaction count fetched successfully",
			args: &ptypes.InteractionCountArgs{
				From: ts.Address().String(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedNonce: latestNonce,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &ptypes.InteractionCountArgs{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
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
			require.Equal(t, test.expectedNonce, fetchedNonce)
		})
	}
}

func TestPublicIXPoolAPI_GetPendingInteractionCount(t *testing.T) {
	address := tests.RandomAddress(t)
	ixpool := NewMockIxPool(t)

	ixpool.setNonce(address, 5)

	coreAPI := NewPublicCoreAPI(ixpool, nil, nil)

	testcases := []struct {
		name            string
		args            *ptypes.InteractionCountArgs
		expectedIxCount uint64
		expectedErr     error
	}{
		{
			name: "Valid address with state",
			args: &ptypes.InteractionCountArgs{
				From: address.String(),
			},
			expectedIxCount: 5,
			expectedErr:     nil,
		},
		{
			name: "Valid address without state",
			args: &ptypes.InteractionCountArgs{
				From: tests.RandomAddress(t).String(),
			},
			expectedIxCount: 0,
			expectedErr:     types.ErrAccountNotFound,
		},
		{
			name: "Invalid address",
			args: &ptypes.InteractionCountArgs{
				From: "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedIxCount: 0,
			expectedErr:     types.ErrInvalidAddress,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixCount, err := coreAPI.GetPendingInteractionCount(testcase.args)

			if testcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, testcase.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, testcase.expectedIxCount, ixCount)
		})
	}
}

func TestPublicCoreAPI_GetAccountState(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t).String()
	tsHash := tests.GetTesseractHash(t, ts).String()
	acc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
		acc.Nonce = uint64(5)
	})

	s.setAccount(ts.Address(), *acc)

	testcases := []struct {
		name          string
		args          *ptypes.GetAccountArgs
		expectedAcc   *types.Account
		expectedError error
	}{
		{
			name: "account state fetched successfully",
			args: &ptypes.GetAccountArgs{
				Address: ts.Address().String(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAcc: acc,
		},
		{
			name: "should return error if failed to fetch interaction count",
			args: &ptypes.GetAccountArgs{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
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
			require.Equal(t, test.expectedAcc, fetchedAcc)
		})
	}
}

func TestPublicCoreAPI_GetLogicManifest(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	randomHash := tests.RandomHash(t).String()
	tsHash := tests.GetTesseractHash(t, ts).String()

	logicID := getLogicID(t, ts.Address())
	logicIDWithoutState := getLogicID(t, tests.RandomAddress(t))

	s.setLogicManifest(logicID.Hex(), []byte{0x00, 0x01})
	c.setTesseractByHash(t, ts)

	testcases := []struct {
		name                  string
		args                  *ptypes.LogicManifestArgs
		expectedLogicManifest []byte
		expectedError         error
	}{
		{
			name: "returns error if logic id is invalid",
			args: &ptypes.LogicManifestArgs{
				LogicID: types.LogicID(tests.RandomHash(t).String()).Hex(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrInvalidLogicID,
		},
		{
			name: "returns error if failed to fetch logic manifest",
			args: &ptypes.LogicManifestArgs{
				LogicID: logicIDWithoutState.Hex(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "fetched logic manifest successfully",
			args: &ptypes.LogicManifestArgs{
				LogicID: logicID.Hex(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: []byte{0x00, 0x01},
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
			require.Equal(t, test.expectedLogicManifest, manifest)
		})
	}
}

func TestPublicCoreAPI_GetStorageAt(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	randomHash := tests.RandomHash(t).String()
	tsHash := tests.GetTesseractHash(t, ts).String()

	logicID := getLogicID(t, ts.Address())
	logicIDWithoutState := getLogicID(t, tests.RandomAddress(t))

	c.setTesseractByHash(t, ts)

	keys := getHexEntries(t, 1)
	values := getHexEntries(t, 1)

	s.SetStorageEntry(logicID, getStorageMap(keys, values))

	testcases := []struct {
		name          string
		args          *ptypes.GetStorageArgs
		expectedValue []byte
		expectedError error
	}{
		{
			name: "returns error if logic id is invalid",
			args: &ptypes.GetStorageArgs{
				LogicID: types.LogicID(tests.RandomHash(t).String()).Hex(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrInvalidLogicID,
		},
		{
			name: "returns error if failed to fetch logic manifest",
			args: &ptypes.GetStorageArgs{
				LogicID: logicIDWithoutState.Hex(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "fetched logic manifest successfully",
			args: &ptypes.GetStorageArgs{
				LogicID:    logicID.Hex(),
				StorageKey: keys[0],
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedValue: []byte(values[0]),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := coreAPI.GetStorageAt(test.args)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestPublicCoreAPI_GetInteractionReceipt(t *testing.T) {
	chainManager := NewMockChainManager(t)
	receiptHash, receipt := getReceipt(t)

	chainManager.setReceipt(receiptHash, receipt)

	coreAPI := NewPublicCoreAPI(nil, chainManager, nil)

	testcases := []struct {
		name            string
		args            ptypes.ReceiptArgs
		expectedReceipt *types.Receipt
		expectedError   error
	}{
		{
			name: "Invalid hash",
			args: ptypes.ReceiptArgs{
				Hash: "68510188a88ff3bc0f4bd4f7a1b0100cc7a15aacc8fxa0adf7c539054c93151c",
			},
			expectedError: types.ErrInvalidHash,
		},
		{
			name: "Valid hash without state",
			args: ptypes.ReceiptArgs{
				Hash: tests.RandomHash(t).String(),
			},
			expectedError: types.ErrReceiptNotFound,
		},
		{
			name: "Valid hash with state",
			args: ptypes.ReceiptArgs{
				Hash: receiptHash.String(),
			},
			expectedReceipt: receipt,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			receipt, err := coreAPI.GetInteractionReceipt(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, receipt, test.expectedReceipt)
		})
	}
}

func TestPublicCoreAPI_GetAssetInfoByAssetID(t *testing.T) {
	address := tests.RandomAddress(t)
	chainManager := NewMockChainManager(t)
	assetID, assetInfo := tests.CreateTestAsset(t, address)

	chainManager.setAssets(assetID, assetInfo)

	coreAPI := NewPublicCoreAPI(nil, chainManager, nil)

	testcases := []struct {
		name                    string
		args                    *ptypes.AssetDescriptorArgs
		expectedAssetDescriptor *types.AssetDescriptor
		isErrorExpected         bool
	}{
		{
			name: "Valid asset id",
			args: &ptypes.AssetDescriptorArgs{
				AssetID: string(assetID),
			},
			expectedAssetDescriptor: assetInfo,
		},
		{
			name: "Valid asset id without state",
			args: &ptypes.AssetDescriptorArgs{
				AssetID: "01801995a34ceda4db744a5b1363be9a0f2019e7481699c861ad7d1263c95473a2d9",
			},
			isErrorExpected: true,
		},
		{
			name: "Invalid asset id",
			args: &ptypes.AssetDescriptorArgs{
				AssetID: "01801995a34ceda4db744a5b1363bega0f2019e7481699c861ad7d1263c95473a2d9",
			},
			isErrorExpected: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAssetInfo, err := coreAPI.GetAssetInfoByAssetID(test.args.AssetID)
			if test.isErrorExpected {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAssetDescriptor, fetchedAssetInfo)
		})
	}
}

func TestPublicCoreAPI_GetAccountMetaInfo(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t).String()
	tsHash := tests.GetTesseractHash(t, ts).String()
	acc := tests.GetRandomAccMetaInfo(t, 1)

	s.setAccountMetaInfo(t, ts.Address(), acc)

	testcases := []struct {
		name                string
		args                *ptypes.GetAccountArgs
		expectedAccMetaInfo *types.AccountMetaInfo
		expectedError       error
	}{
		{
			name: "account meta info fetched successfully",
			args: &ptypes.GetAccountArgs{
				Address: ts.Address().String(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAccMetaInfo: acc,
		},
		{
			name: "should return error if failed to fetch account meta info",
			args: &ptypes.GetAccountArgs{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			fetchedAccMetaInfo, err := coreAPI.AccountMetaInfo(test.args)

			if test.expectedError != nil {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedAccMetaInfo, fetchedAccMetaInfo)
		})
	}
}

package api

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/common/hexutil"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Core Api Testcases

func TestPublicCoreAPI_CreateRPCInteraction(t *testing.T) {
	t.Helper()

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

	input := tests.CreateIXInputWithTestData(t, types.IxAssetCreate, assetPayloadBytes, nil)
	input.PerceivedValues = nil
	input.TransferValues = nil

	ixData := types.IxData{
		Input:   input,
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t).Bytes(), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ixWithNilFields := types.NewInteraction(ixData, tests.RandomHash(t).Bytes())

	testcases := []struct {
		name          string
		ix            *types.Interaction
		expectedError error
	}{
		{
			name: "create rpc interaction for value transfer interaction",
			ix:   createInteractionWithTestData(t, types.IxValueTransfer, json.RawMessage{}),
		},
		{
			name: "create rpc interaction for asset creation interaction",
			ix:   createInteractionWithTestData(t, types.IxAssetCreate, assetPayloadBytes),
		},
		{
			name: "create rpc interaction for logic deploy interaction",
			ix:   createInteractionWithTestData(t, types.IxLogicDeploy, logicPayloadBytes),
		},
		{
			name: "create rpc interaction for logic execute interaction",
			ix:   createInteractionWithTestData(t, types.IxLogicInvoke, logicPayloadBytes),
		},
		{
			name: "create rpc interaction with nil transfer values,perceived values, perceived proofs, ",
			ix:   ixWithNilFields,
		},
		{
			name:          "invalid interaction type",
			ix:            createInteractionWithTestData(t, types.IxAssetMint, logicPayloadBytes),
			expectedError: errors.New("invalid interaction type"),
		},
		{
			name:          "invalid payload",
			ix:            createInteractionWithTestData(t, types.IxAssetCreate, invalidPayload),
			expectedError: errors.New("failed to depolorize asset payload"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := createRPCInteraction(test.ix)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForRPCIxn(t, test.ix, rpcIxn)
		})
	}
}

func TestPublicCoreAPI_CreateRPCTesseractGridID(t *testing.T) {
	testcases := []struct {
		name   string
		gridID *types.TesseractGridID
	}{
		{
			name: "create rpc tesseract grid id from tesseract grid id with nil tesseract parts",
			gridID: &types.TesseractGridID{
				Hash: tests.RandomHash(t),
			},
		},
		{
			name: "create rpc tesseract grid id from tesseract grid id",
			gridID: &types.TesseractGridID{
				Hash:  tests.RandomHash(t),
				Parts: tests.CreateTesseractPartsWithTestData(t),
			},
		},
		{
			name:   "nil grid",
			gridID: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcTesseractGridID := createRPCTesseractGridID(test.gridID)

			checkForRPCTesseractGridID(t, test.gridID, rpcTesseractGridID)
		})
	}
}

func TestPublicCoreAPI_CreateRPCContextLockInfos(t *testing.T) {
	contextLockInfos := make(map[types.Address]types.ContextLockInfo)

	for i := 0; i < 3; i++ {
		contextLockInfos[tests.RandomAddress(t)] = types.ContextLockInfo{
			ContextHash:   tests.RandomHash(t),
			Height:        8,
			TesseractHash: tests.RandomHash(t),
		}
	}

	testcases := []struct {
		name             string
		contextLockInfos map[types.Address]types.ContextLockInfo
	}{
		{
			name:             "create rpc context lock infos",
			contextLockInfos: contextLockInfos,
		},
		{
			name:             "nil context lock infos",
			contextLockInfos: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcContextLockInfos := createRPCContextLockInfos(test.contextLockInfos)

			checkForRPCContextLockInfos(t, test.contextLockInfos, rpcContextLockInfos)
		})
	}
}

func TestPublicCoreAPI_CreateRPCDeltaGroups(t *testing.T) {
	deltaGroups := make(map[types.Address]*types.DeltaGroup)

	for i := 0; i < 3; i++ {
		deltaGroups[tests.RandomAddress(t)] = &types.DeltaGroup{
			Role:             4,
			BehaviouralNodes: tests.GetTestKramaIDs(t, 2),
			RandomNodes:      tests.GetTestKramaIDs(t, 2),
			ReplacedNodes:    tests.GetTestKramaIDs(t, 2),
		}
	}

	testcases := []struct {
		name             string
		contextLockInfos map[types.Address]*types.DeltaGroup
	}{
		{
			name:             "create rpc context lock infos",
			contextLockInfos: deltaGroups,
		},
		{
			name:             "nil context lock infos",
			contextLockInfos: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcContextLockInfos := createRPCDeltaGroups(test.contextLockInfos)

			checkForRPCDeltaGroups(t, test.contextLockInfos, rpcContextLockInfos)
		})
	}
}

func TestPublicCoreAPI_CreateRPCHeader(t *testing.T) {
	headerWithNilGrid := tests.CreateHeaderWithTestData(t)
	headerWithNilGrid.Extra.GridID = nil

	testcases := []struct {
		name   string
		header types.TesseractHeader
	}{
		{
			name:   "create rpc header from tesseract header with nil tesseract grid id",
			header: headerWithNilGrid,
		},
		{
			name:   "create rpc header from tesseract header",
			header: tests.CreateHeaderWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			rpcHeader := createRPCHeader(test.header)

			checkForRPCHeader(t, test.header, rpcHeader)
		})
	}
}

func TestPublicCoreAPI_CreateRPCBody(t *testing.T) {
	headerWithNilGrid := tests.CreateHeaderWithTestData(t)
	headerWithNilGrid.Extra.GridID = nil

	testcases := []struct {
		name string
		body types.TesseractBody
	}{
		{
			name: "create rpc body from tesseract body",
			body: tests.CreateBodyWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcBody := createRPCBody(test.body)

			checkForRPCBody(t, test.body, rpcBody)
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
		Receipts: map[types.Hash]*types.Receipt{
			tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
		},
		Seal:           []byte{1, 2},
		HeaderCallback: createHeaderCallbackWithTestData(t),
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = tests.RandomHash(t)
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
		{
			name:     "nil interactions",
			ixParams: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.ixParams != nil {
				ix := tests.CreateIX(t, test.ixParams)
				tesseractParams.Ixns = []*types.Interaction{ix}
			}

			ts := tests.CreateTesseract(t, tesseractParams)

			rpcTS, err := CreateRPCTesseract(ts)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForRPCTesseract(t, ts, rpcTS)
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
		Receipts: map[types.Hash]*types.Receipt{
			tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
		},
		Seal: []byte{1, 2},
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = tests.RandomHash(t)
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
				Address:          ts.Address(),
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

			checkForRPCTesseract(t, ts, fetchedTesseract)
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 1, 2)
	ts := tests.CreateTesseracts(t, 1, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil)

	c.setTesseractByHash(t, ts[0])

	tsHash := tests.GetTesseractHash(t, ts[0])

	testcases := []struct {
		name             string
		hash             types.Hash
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "should return error if valid hash without state",
			hash:             tests.RandomHash(t),
			withInteractions: false,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "nil hash",
			hash:             types.NilHash,
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
		from             types.Address
		height           int64
		withInteractions bool
		expectedTS       *types.Tesseract
		expectedError    error
	}{
		{
			name:             "nil address",
			from:             types.NilAddress,
			height:           8,
			withInteractions: false,
			expectedError:    types.ErrInvalidAddress,
		},
		{
			name:             "should return error if height doesn't exist",
			from:             ts[0].Address(),
			height:           9,
			withInteractions: true,
			expectedError:    types.ErrFetchingTesseract,
		},
		{
			name:             "fetch tesseract with interactions",
			from:             ts[0].Address(),
			height:           8,
			withInteractions: true,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch tesseract without interactions",
			from:             ts[0].Address(),
			height:           8,
			withInteractions: false,
			expectedTS:       ts[0],
		},
		{
			name:             "fetch latest tesseract with interactions",
			from:             ts[1].Address(),
			height:           ptypes.LatestTesseractHeight,
			withInteractions: true,
			expectedTS:       ts[1],
		},
		{
			name:             "fetch latest tesseract without interactions",
			from:             ts[1].Address(),
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

	tsHash := tests.GetTesseractHash(t, ts[1])

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
				Address:          ts[0].Address(),
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
				Address:          ts[0].Address(),
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
				Address:          ts[1].Address(),
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
				Address:          ts[1].Address(),
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

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)
	assetID, _ := tests.CreateTestAsset(t, ts.Address())

	s.setBalance(ts.Address(), assetID, big.NewInt(300))
	address := ts.Address()

	testcases := []struct {
		name            string
		args            ptypes.BalArgs
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name: "should return error if failed to fetch balance",
			args: ptypes.BalArgs{
				Address: address,
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
				Address: address,
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
				Address: address,
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
			require.Equal(t, test.expectedBalance, balance.ToInt())
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

	address := ts[0].Address()
	tsHash := getTesseractsHashes(t, ts)
	randomHash := tests.RandomHash(t)

	testcases := []struct {
		name            string
		args            ptypes.ContextInfoArgs
		expectedContext *Context
		expectedError   error
	}{
		{
			name: "fetched context info successfully",
			args: ptypes.ContextInfoArgs{
				Address: address,
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
				Address: ts[1].Address(),
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

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)
	assetID, _ := tests.CreateTestAsset(t, ts[0].Address())

	s.setBalance(ts[0].Address(), assetID, big.NewInt(300))
	address := ts[0].Address()

	testcases := []struct {
		name          string
		args          ptypes.TesseractArgs
		expectedTDU   types.AssetMap
		expectedError error
	}{
		{
			name: "should return error if tesseract not found",
			args: ptypes.TesseractArgs{
				Address: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: ptypes.TesseractArgs{
				Address: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: types.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: ptypes.TesseractArgs{
				Address: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedTDU: s.getTDU(ts[0].Address(), types.NilHash),
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

			for expectedAsset, expectedAmount := range test.expectedTDU {
				amount := tdu[expectedAsset]
				require.Equal(t, expectedAmount.Text(16), amount)
			}
		})
	}
}

func TestPublicCoreAPI_GetInteractionCount(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)
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
				Address: ts.Address(),
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
			require.Equal(t, test.expectedNonce, fetchedNonce.ToInt())
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
			name: "valid address with state",
			args: &ptypes.InteractionCountArgs{
				Address: address,
			},
			expectedIxCount: 5,
			expectedErr:     nil,
		},
		{
			name: "valid address without state",
			args: &ptypes.InteractionCountArgs{
				Address: tests.RandomAddress(t),
			},
			expectedIxCount: 0,
			expectedErr:     types.ErrAccountNotFound,
		},
		{
			name: "nil address",
			args: &ptypes.InteractionCountArgs{
				Address: types.NilAddress,
			},
			expectedIxCount: 0,
			expectedErr:     types.ErrInvalidAddress,
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
			require.Equal(t, testcase.expectedIxCount, ixCount.ToInt())
		})
	}
}

func TestPublicCoreAPI_GetAccountState(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)
	acc, _ := tests.GetTestAccount(t, func(acc *types.Account) {
		acc.Nonce = uint64(5)
		acc.AccType = types.RegularAccount
		acc.Balance = tests.RandomHash(t)
		acc.AssetApprovals = tests.RandomHash(t)
		acc.ContextHash = tests.RandomHash(t)
		acc.StorageRoot = tests.RandomHash(t)
		acc.LogicRoot = tests.RandomHash(t)
		acc.FileRoot = tests.RandomHash(t)
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
				Address: ts.Address(),
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
			require.Equal(t, hexutil.Uint64(test.expectedAcc.Nonce), fetchedAcc["nonce"])
			require.Equal(t, test.expectedAcc.AccType, fetchedAcc["acc_type"])
			require.Equal(t, test.expectedAcc.Balance, fetchedAcc["balance"])
			require.Equal(t, test.expectedAcc.AssetApprovals, fetchedAcc["asset_approvals"])
			require.Equal(t, test.expectedAcc.ContextHash, fetchedAcc["context_hash"])
			require.Equal(t, test.expectedAcc.StorageRoot, fetchedAcc["storage_root"])
			require.Equal(t, test.expectedAcc.LogicRoot, fetchedAcc["logic_root"])
			require.Equal(t, test.expectedAcc.FileRoot, fetchedAcc["file_root"])
		})
	}
}

func TestPublicCoreAPI_GetLogicManifest(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	logicID := getLogicID(t, ts.Address())
	logicIDWithoutState := getLogicID(t, tests.RandomAddress(t))

	poloManifest, jsonManifest, yamlManifest := tests.GetManifests(t, "./../../jug/manifests/erc20.json")

	s.setLogicManifest(logicID.Hex(), poloManifest)
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
				LogicID:  types.LogicID(tests.RandomHash(t).String()).Hex(),
				Encoding: "JSON",
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrInvalidLogicID,
		},
		{
			name: "returns error if failed to fetch logic manifest",
			args: &ptypes.LogicManifestArgs{
				LogicID:  logicIDWithoutState.Hex(),
				Encoding: "JSON",
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "fetched json encoded logic manifest successfully",
			args: &ptypes.LogicManifestArgs{
				LogicID:  logicID.Hex(),
				Encoding: "JSON",
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: jsonManifest,
		},
		{
			name: "fetched polo encoded logic manifest successfully",
			args: &ptypes.LogicManifestArgs{
				LogicID:  logicID.Hex(),
				Encoding: "POLO",
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedLogicManifest: poloManifest,
		},
		{
			name: "fetched yaml encoded logic manifest successfully",
			args: &ptypes.LogicManifestArgs{
				LogicID:  logicID.Hex(),
				Encoding: "YAML",
				Options: ptypes.TesseractNumberOrHash{
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

func TestPublicCoreAPI_GetStorageAt(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

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
			require.Equal(t, test.expectedValue, value.Bytes())
		})
	}
}

func TestPublicCoreAPI_GetInteractionReceipt(t *testing.T) {
	chainManager := NewMockChainManager(t)
	receipt := tests.CreateReceiptWithTestData(t)
	receiptHash := tests.RandomHash(t)
	chainManager.setReceipt(receiptHash, receipt)

	coreAPI := NewPublicCoreAPI(nil, chainManager, nil)

	testcases := []struct {
		name            string
		args            ptypes.ReceiptArgs
		expectedReceipt *types.Receipt
		expectedError   error
	}{
		{
			name: "nil hash",
			args: ptypes.ReceiptArgs{
				Hash: types.NilHash,
			},
			expectedError: types.ErrInvalidHash,
		},
		{
			name: "Valid hash without state",
			args: ptypes.ReceiptArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: types.ErrReceiptNotFound,
		},
		{
			name: "Valid hash with state",
			args: ptypes.ReceiptArgs{
				Hash: receiptHash,
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
			checkForRPCReceipt(t, test.expectedReceipt, receipt)
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
			require.Equal(t, test.expectedAssetDescriptor.Type, fetchedAssetInfo["type"])
			require.Equal(t, test.expectedAssetDescriptor.Symbol, fetchedAssetInfo["symbol"])
			require.Equal(t, test.expectedAssetDescriptor.Owner, fetchedAssetInfo["owner"])
			require.Equal(t, (*hexutil.Big)(test.expectedAssetDescriptor.Supply), fetchedAssetInfo["supply"])
			require.Equal(t, hexutil.Uint8(test.expectedAssetDescriptor.Dimension), fetchedAssetInfo["dimension"])
			require.Equal(t, hexutil.Uint8(test.expectedAssetDescriptor.Decimals), fetchedAssetInfo["decimals"])
			require.Equal(t, test.expectedAssetDescriptor.IsFungible, fetchedAssetInfo["is_fungible"])
			require.Equal(t, test.expectedAssetDescriptor.IsMintable, fetchedAssetInfo["is_mintable"])
			require.Equal(t, test.expectedAssetDescriptor.IsTransferable, fetchedAssetInfo["is_transferable"])
			require.Equal(t, test.expectedAssetDescriptor.LogicID, fetchedAssetInfo["logic_id"])
		})
	}
}

func TestPublicCoreAPI_GetAccountMetaInfo(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)

	c.setTesseractByHash(t, ts)

	tsHash := tests.GetTesseractHash(t, ts)
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
				Address: ts.Address(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAccMetaInfo: acc,
		},
		{
			name: "should return error if failed to fetch account meta info",
			args: &ptypes.GetAccountArgs{
				Address: types.NilAddress,
			},
			expectedError: types.ErrInvalidAddress,
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

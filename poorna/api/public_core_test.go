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
	"github.com/sarvalabs/moichain/lattice"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Core Api Testcases

func TestPublicCoreAPI_CreateRPCInteraction(t *testing.T) {
	t.Helper()

	assetPayload := &types.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetCreatePayloadBytes, err := assetPayload.Bytes()
	require.NoError(t, err)

	logicPayload := &types.LogicPayload{
		Callsite: "call site",
	}

	logicPayloadBytes, err := polo.Polorize(logicPayload)
	require.NoError(t, err)

	input := tests.CreateIXInputWithTestData(t, types.IxAssetCreate, assetCreatePayloadBytes, nil)
	input.PerceivedValues = nil
	input.TransferValues = nil

	ixData := types.IxData{
		Input:   input,
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ixWithNilFields, err := types.NewInteraction(ixData, tests.RandomHash(t).Bytes())
	require.NoError(t, err)

	testcases := []struct {
		name          string
		grid          map[types.Address]types.TesseractHeightAndHash
		ix            *types.Interaction
		expectedError error
	}{
		{
			name: "create rpc interaction for value transfer interaction",
			ix:   createInteractionWithTestData(t, types.IxValueTransfer, json.RawMessage{}),
		},
		{
			name: "create rpc interaction for asset creation interaction",
			ix:   createInteractionWithTestData(t, types.IxAssetCreate, assetCreatePayloadBytes),
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
			name: "create rpc interaction with grid data",
			ix:   createInteractionWithTestData(t, types.IxValueTransfer, json.RawMessage{}),
			grid: map[types.Address]types.TesseractHeightAndHash{
				tests.RandomAddress(t): {
					Height: 5,
					Hash:   tests.RandomHash(t),
				},
				tests.RandomAddress(t): {
					Height: 6,
					Hash:   tests.RandomHash(t),
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := createRPCInteraction(test.ix, test.grid, 0)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			checkForRPCIxn(t, test.ix, rpcIxn, test.grid)
			checkForRPCTesseractParts(t, test.grid, rpcIxn.Parts)
		})
	}
}

func TestGetRPCTesseractPartsFromGrid(t *testing.T) {
	testcases := []struct {
		name string
		grid map[types.Address]types.TesseractHeightAndHash
	}{
		{
			name: "create rpc tesseract parts from grid",
			grid: tests.CreateTesseractPartsWithTestData(t).Grid,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			parts := getRPCTesseractPartsFromGrid(test.grid)

			checkForRPCTesseractParts(t, test.grid, parts)
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
		t.Run(test.name, func(t *testing.T) {
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
	assetPayload := &types.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	// make sure to fill at least one field of every field of tesseract so that we can verify that every field is copied
	createTesseractParams := func(headerCallback func(header *types.TesseractHeader)) *tests.CreateTesseractParams {
		return &tests.CreateTesseractParams{
			Address: tests.RandomAddress(t),
			Receipts: map[types.Hash]*types.Receipt{
				tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
			},
			Seal:           []byte{1, 2},
			HeaderCallback: headerCallback,
			BodyCallback: func(body *types.TesseractBody) {
				body.StateHash = tests.RandomHash(t)
			},
		}
	}

	testcases := []struct {
		name          string
		ixParams      *tests.CreateIxParams
		tsParams      *tests.CreateTesseractParams
		expectedError error
	}{
		{
			name:     "created rpc tesseract for non-genesis tesseract",
			ixParams: getIxParamsWithInputComputeTrust(types.IxAssetCreate, assetPayloadBytes, 2, 3),
			tsParams: createTesseractParams(createHeaderCallbackWithTestData(t)),
		},
		{
			name: "create rpc tesseract for genesis tesseract",
			tsParams: createTesseractParams(func(header *types.TesseractHeader) {
				header.ClusterID = lattice.GenesisIdentifier
				header.FuelLimit = big.NewInt(0)
				header.FuelUsed = big.NewInt(0)
			}),
		},
		{
			name:     "nil interactions",
			ixParams: nil,
			tsParams: createTesseractParams(func(header *types.TesseractHeader) {
				header.FuelLimit = big.NewInt(0)
				header.FuelUsed = big.NewInt(0)
			}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.ixParams != nil {
				test.tsParams.Ixns = []*types.Interaction{tests.CreateIX(t, test.ixParams)}
			}

			ts := tests.CreateTesseract(t, test.tsParams)

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

	assetPayload := &types.AssetCreatePayload{
		Symbol: "MOI",
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
		Seal:           []byte{1, 2},
		HeaderCallback: tests.HeaderCallbackWithGridHash(t),
		BodyCallback: func(body *types.TesseractBody) {
			body.StateHash = tests.RandomHash(t)
		},
	}

	ts := tests.CreateTesseract(t, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil)

	c.setTesseractByHash(t, ts)
	hash := getTesseractHash(t, ts)

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
				WithInteractions: true,
				Options: ptypes.TesseractNumberOrHash{
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
	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s)
	acc := tests.GetRandomAccMetaInfo(t, 8)

	s.setAccountMetaInfo(t, acc.Address, acc)

	address := tests.RandomAddress(t)
	height := uint64(66)
	hash := tests.RandomHash(t)

	c.SetTesseractHeightEntry(address, height, hash)

	testcases := []struct {
		name          string
		address       types.Address
		height        int64
		expectedHash  types.Hash
		expectedError error
	}{
		{
			name:          "invalid address",
			address:       types.NilAddress,
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:          "failed fetch tesseract hash for latest tesseract height",
			address:       tests.RandomAddress(t),
			height:        -1,
			expectedError: types.ErrKeyNotFound,
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

func TestPublicCoreAPI_GetTesseract(t *testing.T) {
	height := int64(8)
	invalidHeight := int64(-2)
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 2, 2)
	tesseractParams[0].Height = uint64(height)

	ts := tests.CreateTesseracts(t, 2, tesseractParams)

	c := NewMockChainManager(t)
	sm := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, sm)

	c.SetTesseractHeightEntry(ts[0].Address(), ts[0].Height(), getTesseractHash(t, ts[0]))

	c.setTesseractByHash(t, ts[1])
	c.setTesseractByHash(t, ts[0])

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
			name: "hash not found for given tesseract and height",
			args: ptypes.TesseractArgs{
				WithInteractions: true,
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedError: types.ErrInvalidAddress,
		},
		{
			name: "get tesseract by height without interactions",
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
				AssetID: assetID,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "fetched balance successfully",
			args: ptypes.BalArgs{
				Address: address,
				AssetID: assetID,
				Options: ptypes.TesseractNumberOrHash{
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
		args          ptypes.QueryArgs
		expectedTDU   types.AssetMap
		expectedError error
	}{
		{
			name: "should return error if tesseract not found",
			args: ptypes.QueryArgs{
				Address: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: ptypes.QueryArgs{
				Address: address,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: types.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: ptypes.QueryArgs{
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

			for _, expectedAmount := range test.expectedTDU {
				amount := tdu[0].Amount
				require.Equal(t, expectedAmount, amount.ToInt())
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
			require.Equal(t, test.expectedNonce, fetchedNonce.ToUint64())
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
			require.Equal(t, testcase.expectedIxCount, ixCount.ToUint64())
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
		acc.AssetRegistry = tests.RandomHash(t)
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
			require.Equal(t, test.expectedAcc.AssetRegistry, fetchedAcc["asset_registry"])
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

	s.setLogicManifest(logicID.String(), poloManifest)
	c.setTesseractByHash(t, ts)

	testcases := []struct {
		name                  string
		args                  *ptypes.LogicManifestArgs
		expectedLogicManifest []byte
		expectedError         error
	}{
		{
			name: "returns error if failed to fetch logic manifest",
			args: &ptypes.LogicManifestArgs{
				LogicID:  logicIDWithoutState,
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
				LogicID:  logicID,
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
				LogicID:  logicID,
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
				LogicID:  logicID,
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
				LogicID: types.LogicID(tests.RandomHash(t).String()).String(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrInvalidLogicID,
		},
		{
			name: "returns error if failed to fetch logic manifest",
			args: &ptypes.GetStorageArgs{
				LogicID: logicIDWithoutState.String(),
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: types.ErrFetchingTesseract,
		},
		{
			name: "fetched logic manifest successfully",
			args: &ptypes.GetStorageArgs{
				LogicID:    logicID.String(),
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

func TestPublicCoreAPI_GetInteractionByTSHash(t *testing.T) {
	chainManager := NewMockChainManager(t)
	sm := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, sm)

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
		args          ptypes.InteractionByTesseract
		expectedIX    *types.Interaction
		expectedParts *types.TesseractParts
		expectedError error
	}{
		{
			name: "can not use both tesseract number and tesseract hash",
			args: ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
					TesseractHash:   &randomHash,
				},
			},
			expectedError: errors.New("can not use both tesseract number and tesseract hash"),
		},
		{
			name: "empty options",
			args: ptypes.InteractionByTesseract{
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: types.ErrEmptyOptions,
		},
		{
			name: "interaction not found",
			args: ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: types.ErrFetchingInteraction,
		},
		{
			name: "hash not available for given tesseract number and address",
			args: ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: types.ErrInvalidAddress,
		},
		{
			name: "ix index is nil",
			args: ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &randomHeight,
				},
			},
			expectedError: types.ErrIXIndex,
		},
		{
			name: "invalid options",
			args: ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractNumber: &negativeHeight,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "fetch interaction by tesseract hash successfully",
			args: ptypes.InteractionByTesseract{
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
				IxIndex: (*hexutil.Uint64)(&ixIndex),
			},
			expectedIX:    ix,
			expectedParts: parts,
		},
		{
			name: "fetch interaction by tesseract number successfully",
			args: ptypes.InteractionByTesseract{
				Address: address,
				Options: ptypes.TesseractNumberOrHash{
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

			checkForRPCIxn(t, test.expectedIX, rpcIX, test.expectedParts.Grid)
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
		args           ptypes.InteractionByHashArgs
		setHook        func(c *MockChainManager)
		expectedIXData ixData
		expectedError  error
	}{
		{
			name: "invalid tesseract hash",
			args: ptypes.InteractionByHashArgs{
				Hash: types.NilHash,
			},
			expectedError: types.ErrInvalidHash,
		},
		{
			name: "fetch pending interaction successfully",
			args: ptypes.InteractionByHashArgs{
				Hash: ixns[1].Hash(),
			},
			setHook: func(c *MockChainManager) {
				c.GetInteractionByIxHashHook = func() error {
					return types.ErrGridHashNotFound
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
			args: ptypes.InteractionByHashArgs{
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
			args: ptypes.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: types.ErrFetchingInteraction,
		},
		{
			name: "should return error if failed to fetch pending interaction",
			args: ptypes.InteractionByHashArgs{
				Hash: tests.RandomHash(t),
			},
			setHook: func(c *MockChainManager) {
				c.GetInteractionByIxHashHook = func() error {
					return types.ErrGridHashNotFound
				}
			},
			expectedError: types.ErrFetchingInteraction,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			chainManager := NewMockChainManager(t)
			if test.setHook != nil {
				test.setHook(chainManager)
			}

			chainManager.SetInteractionDataByIxHash(ixns[0], parts, ixIndex)

			coreAPI := NewPublicCoreAPI(ixpool, chainManager, nil)

			rpcIX, err := coreAPI.GetInteractionByHash(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if test.expectedIXData.parts != nil {
				checkForRPCIxn(t, test.expectedIXData.ix, rpcIX, test.expectedIXData.parts.Grid)
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

	chainManager.SetInteractionDataByIxHash(ix, parts, ixIndex)

	receipt := tests.CreateReceiptWithTestData(t)
	receipt.IxHash = ix.Hash()
	chainManager.setReceiptByIXHash(receipt.IxHash, receipt)
	chainManager.setReceiptByIXHash(ixHashWithoutParts, receipt)
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil)

	testcases := []struct {
		name            string
		args            ptypes.ReceiptArgs
		expectedReceipt *types.Receipt
		ix              *types.Interaction
		grid            map[types.Address]types.TesseractHeightAndHash
		ixIndex         int
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
			name: "failed to fetch receipt",
			args: ptypes.ReceiptArgs{
				Hash: tests.RandomHash(t),
			},
			expectedError: types.ErrReceiptNotFound,
		},
		{
			name: "failed to fetch interaction",
			args: ptypes.ReceiptArgs{
				Hash: ixHashWithoutParts,
			},
			expectedError: types.ErrFetchingInteraction,
		},
		{
			name: "fetched receipt successfully",
			args: ptypes.ReceiptArgs{
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
			checkForRPCReceipt(t, test.grid, test.ix, test.expectedReceipt, receipt, test.ixIndex)
		})
	}
}

func TestPublicCoreAPI_GetAssetInfoByAssetID(t *testing.T) {
	address := tests.RandomAddress(t)
	sm := NewMockStateManager(t)
	c := NewMockChainManager(t)
	assetID, assetInfo := tests.CreateTestAsset(t, address)

	ts := tests.CreateTesseract(t, nil)
	tsHash := tests.GetTesseractHash(t, ts)
	c.setTesseractByHash(t, ts)
	sm.addAsset(assetID, assetInfo)

	coreAPI := NewPublicCoreAPI(nil, c, sm)

	testcases := []struct {
		name                    string
		args                    *ptypes.GetAssetInfoArgs
		expectedAssetDescriptor *types.AssetDescriptor
		isErrorExpected         bool
	}{
		{
			name: "Valid asset id",
			args: &ptypes.GetAssetInfoArgs{
				AssetID: assetID,
				Options: ptypes.TesseractNumberOrHash{
					TesseractHash: &tsHash,
				},
			},
			expectedAssetDescriptor: assetInfo,
		},
		{
			name: "Valid asset id without state",
			args: &ptypes.GetAssetInfoArgs{
				AssetID: tests.GetRandomAssetID(t, types.NilAddress),
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
			require.Equal(t, test.expectedAssetDescriptor.Type, fetchedAssetInfo["type"])
			require.Equal(t, test.expectedAssetDescriptor.Symbol, fetchedAssetInfo["symbol"])
			require.Equal(t, test.expectedAssetDescriptor.Owner, fetchedAssetInfo["owner"])
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

func TestPublicCoreAPI_CreateRPCReceipt(t *testing.T) {
	ixParams := tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t))
	testcases := []struct {
		name    string
		receipt *types.Receipt
		ix      *types.Interaction
		grid    map[types.Address]types.TesseractHeightAndHash
		ixIndex int
	}{
		{
			name:    "create rpc receipt",
			receipt: tests.CreateReceiptWithTestData(t),
			ix:      tests.CreateIX(t, ixParams),
			grid:    tests.CreateTesseractPartsWithTestData(t).Grid,
			ixIndex: 8,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt := createRPCReceipt(test.receipt, test.ix, test.grid, test.ixIndex)

			checkForRPCReceipt(t, test.grid, test.ix, test.receipt, receipt, test.ixIndex)
		})
	}
}

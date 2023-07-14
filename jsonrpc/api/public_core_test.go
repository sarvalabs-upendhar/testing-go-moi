package api

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Core Api Testcases

func TestPublicCoreAPI_CreateRPCInteraction(t *testing.T) {
	t.Helper()

	assetPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetCreatePayloadBytes, err := assetPayload.Bytes()
	require.NoError(t, err)

	logicPayload := &common.LogicPayload{
		Callsite: "call site",
	}

	logicPayloadBytes, err := polo.Polorize(logicPayload)
	require.NoError(t, err)

	input := tests.CreateIXInputWithTestData(t, common.IxAssetCreate, assetCreatePayloadBytes, nil)
	input.PerceivedValues = nil
	input.TransferValues = nil

	ixData := common.IxData{
		Input:   input,
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ixWithNilFields, err := common.NewInteraction(ixData, tests.RandomHash(t).Bytes())
	require.NoError(t, err)

	testcases := []struct {
		name          string
		grid          map[common.Address]common.TesseractHeightAndHash
		ix            *common.Interaction
		expectedError error
	}{
		{
			name: "create rpc interaction for value transfer interaction",
			ix:   createInteractionWithTestData(t, common.IxValueTransfer, json.RawMessage{}),
		},
		{
			name: "create rpc interaction for asset creation interaction",
			ix:   createInteractionWithTestData(t, common.IxAssetCreate, assetCreatePayloadBytes),
		},
		{
			name: "create rpc interaction for logic deploy interaction",
			ix:   createInteractionWithTestData(t, common.IxLogicDeploy, logicPayloadBytes),
		},
		{
			name: "create rpc interaction for logic execute interaction",
			ix:   createInteractionWithTestData(t, common.IxLogicInvoke, logicPayloadBytes),
		},
		{
			name: "create rpc interaction with nil transfer values,perceived values, perceived proofs, ",
			ix:   ixWithNilFields,
		},
		{
			name: "create rpc interaction with grid data",
			ix:   createInteractionWithTestData(t, common.IxValueTransfer, json.RawMessage{}),
			grid: map[common.Address]common.TesseractHeightAndHash{
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
		grid map[common.Address]common.TesseractHeightAndHash
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
		gridID *common.TesseractGridID
	}{
		{
			name: "create rpc tesseract grid id from tesseract grid id with nil tesseract parts",
			gridID: &common.TesseractGridID{
				Hash: tests.RandomHash(t),
			},
		},
		{
			name: "create rpc tesseract grid id from tesseract grid id",
			gridID: &common.TesseractGridID{
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
	contextLockInfos := make(map[common.Address]common.ContextLockInfo)

	for i := 0; i < 3; i++ {
		contextLockInfos[tests.RandomAddress(t)] = common.ContextLockInfo{
			ContextHash:   tests.RandomHash(t),
			Height:        8,
			TesseractHash: tests.RandomHash(t),
		}
	}

	testcases := []struct {
		name             string
		contextLockInfos map[common.Address]common.ContextLockInfo
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
	deltaGroups := make(map[common.Address]*common.DeltaGroup)

	for i := 0; i < 3; i++ {
		deltaGroups[tests.RandomAddress(t)] = &common.DeltaGroup{
			Role:             4,
			BehaviouralNodes: tests.GetTestKramaIDs(t, 2),
			RandomNodes:      tests.GetTestKramaIDs(t, 2),
			ReplacedNodes:    tests.GetTestKramaIDs(t, 2),
		}
	}

	testcases := []struct {
		name             string
		contextLockInfos map[common.Address]*common.DeltaGroup
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
		header common.TesseractHeader
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
		body common.TesseractBody
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
	assetPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	// make sure to fill at least one field of every field of tesseract so that we can verify that every field is copied
	createTesseractParams := func(headerCallback func(header *common.TesseractHeader)) *tests.CreateTesseractParams {
		return &tests.CreateTesseractParams{
			Address: tests.RandomAddress(t),
			Receipts: map[common.Hash]*common.Receipt{
				tests.RandomHash(t): {IxHash: tests.RandomHash(t)},
			},
			Seal:           []byte{1, 2},
			HeaderCallback: headerCallback,
			BodyCallback: func(body *common.TesseractBody) {
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
			ixParams: getIxParamsWithInputComputeTrust(common.IxAssetCreate, assetPayloadBytes, 2, 3),
			tsParams: createTesseractParams(createHeaderCallbackWithTestData(t)),
		},
		{
			name: "create rpc tesseract for genesis tesseract",
			tsParams: createTesseractParams(func(header *common.TesseractHeader) {
				header.ClusterID = common.GenesisIdentifier
				header.FuelLimit = big.NewInt(0)
				header.FuelUsed = big.NewInt(0)
			}),
		},
		{
			name:     "nil interactions",
			ixParams: nil,
			tsParams: createTesseractParams(func(header *common.TesseractHeader) {
				header.FuelLimit = big.NewInt(0)
				header.FuelUsed = big.NewInt(0)
			}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.ixParams != nil {
				test.tsParams.Ixns = []*common.Interaction{tests.CreateIX(t, test.ixParams)}
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

	assetPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	ix := tests.CreateIX(t, getIxParamsWithInputComputeTrust(common.IxAssetCreate, assetPayloadBytes, 2, 3))

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

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil, nil)

	c.setTesseractByHash(t, ts)
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

			checkForRPCTesseract(t, ts, fetchedTesseract)
		})
	}
}

func TestPublicCoreAPI_GetTesseractByHash(t *testing.T) {
	tesseractParams := tests.GetTesseractParamsMapWithIxns(t, 1, 2)
	ts := tests.CreateTesseracts(t, 1, tesseractParams)

	c := NewMockChainManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, nil, nil)

	c.setTesseractByHash(t, ts[0])

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
	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)
	acc := tests.GetRandomAccMetaInfo(t, 8)

	s.setAccountMetaInfo(t, acc.Address, acc)

	address := tests.RandomAddress(t)
	height := uint64(66)
	hash := tests.RandomHash(t)

	c.SetTesseractHeightEntry(address, height, hash)

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
			expectedError: common.ErrKeyNotFound,
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
	coreAPI := NewPublicCoreAPI(nil, c, sm, nil)

	c.SetTesseractHeightEntry(ts[0].Address(), ts[0].Height(), getTesseractHash(t, ts[0]))

	c.setTesseractByHash(t, ts[1])
	c.setTesseractByHash(t, ts[0])

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
			name: "should return error if height is invalid",
			args: rpcargs.TesseractArgs{
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &invalidHeight,
				},
			},
			expectedError: errors.New("invalid options"),
		},
		{
			name: "get Tesseract by height with interactions",
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
			name: "hash not found for given tesseract and height",
			args: rpcargs.TesseractArgs{
				WithInteractions: true,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractNumber: &height,
				},
			},
			expectedError: common.ErrInvalidAddress,
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)
	assetID, _ := tests.CreateTestAsset(t, ts.Address())

	s.setBalance(ts.Address(), assetID, big.NewInt(300))
	address := ts.Address()

	testcases := []struct {
		name            string
		args            rpcargs.BalArgs
		expectedBalance *big.Int
		expectedError   error
	}{
		{
			name: "should return error if failed to fetch balance",
			args: rpcargs.BalArgs{
				Address: address,
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
				Address: address,
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	context := getContext(t, 2)
	s.setContext(t, ts[0].Address(), context)
	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	address := ts[0].Address()
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
				Address: address,
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
				Address: ts[1].Address(),
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)
	assetID, _ := tests.CreateTestAsset(t, ts[0].Address())

	s.setBalance(ts[0].Address(), assetID, big.NewInt(300))
	address := ts[0].Address()

	testcases := []struct {
		name          string
		args          rpcargs.QueryArgs
		expectedTDU   common.AssetMap
		expectedError error
	}{
		{
			name: "should return error if tesseract not found",
			args: rpcargs.QueryArgs{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if TDU not found",
			args: rpcargs.QueryArgs{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: common.ErrAccountNotFound,
		},
		{
			name: "fetched TDU successfully",
			args: rpcargs.QueryArgs{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[0],
				},
			},
			expectedTDU: s.getTDU(ts[0].Address(), common.NilHash),
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
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	c.setTesseractByHash(t, ts)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)
	latestNonce := uint64(5)
	acc, _ := tests.GetTestAccount(t, func(acc *common.Account) {
		acc.Nonce = uint64(5)
	})

	s.setAccount(ts.Address(), *acc)

	testcases := []struct {
		name          string
		args          *rpcargs.InteractionCountArgs
		expectedNonce uint64
		expectedError error
	}{
		{
			name: "interaction count fetched successfully",
			args: &rpcargs.InteractionCountArgs{
				Address: ts.Address(),
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

	coreAPI := NewPublicCoreAPI(ixpool, nil, nil, nil)

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
			expectedErr:     nil,
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	c.setTesseractByHash(t, ts)

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

	s.setAccount(ts.Address(), *acc)

	testcases := []struct {
		name          string
		args          *rpcargs.GetAccountArgs
		expectedAcc   *common.Account
		expectedError error
	}{
		{
			name: "account state fetched successfully",
			args: &rpcargs.GetAccountArgs{
				Address: ts.Address(),
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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	c.setTesseractByHash(t, ts[0])
	c.setTesseractByHash(t, ts[1])

	address := ts[0].Address()
	randomHash := tests.RandomHash(t)
	tsHash := getTesseractsHashes(t, ts)

	logicIDs := make([]common.LogicID, 0, 3)

	for i := 0; i < 3; i++ {
		logicID := common.NewLogicIDv0(true, false, false, false, uint16(i), address)

		logicIDs = append(logicIDs, logicID)
	}

	s.setLogicIDs(t, ts[0].Address(), logicIDs)

	testcases := []struct {
		name             string
		args             rpcargs.GetAccountArgs
		expectedLogicIDs []common.LogicID
		expectedError    error
	}{
		{
			name: "should return error if tesseract not found",
			args: rpcargs.GetAccountArgs{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &randomHash,
				},
			},
			expectedError: common.ErrFetchingTesseract,
		},
		{
			name: "should return error if logicIDs not found",
			args: rpcargs.GetAccountArgs{
				Address: address,
				Options: rpcargs.TesseractNumberOrHash{
					TesseractHash: &tsHash[1],
				},
			},
			expectedError: errors.New("logic IDs not found"),
		},
		{
			name: "fetched logicIDs successfully",
			args: rpcargs.GetAccountArgs{
				Address: address,
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

func TestPublicCoreAPI_LogicCall(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	exec := NewMockExecutionManager(t)

	coreAPI := NewPublicCoreAPI(nil, c, s, exec)

	c.setTesseractByHash(t, ts)

	logicID := getLogicID(t, ts.Address())
	calldata := "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29e" +
		"c442dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"

	logicCallResult := &rpcargs.LogicCallResult{
		Consumed: (hexutil.Big)(*big.NewInt(100)),
		Outputs:  (hexutil.Bytes)(common.Hex2Bytes("0x0d2f06256f6b02")),
		Error:    (hexutil.Bytes)(common.Hex2Bytes("0x01")),
	}

	exec.setLogicCall(ts.Address(), logicCallResult)

	testcases := []struct {
		name              string
		args              *rpcargs.LogicCallArgs
		expectedLogicCall *rpcargs.LogicCallResult
		expectedError     error
	}{
		{
			name: "should return the logic call result",
			args: &rpcargs.LogicCallArgs{
				Invoker:  ts.Address(),
				LogicID:  logicID,
				Callsite: "Transfer!",
				Calldata: hexutil.Bytes(common.Hex2Bytes(calldata)),
			},
			expectedLogicCall: logicCallResult,
		},
		{
			name: "should return an error",
			args: &rpcargs.LogicCallArgs{
				Invoker:  tests.RandomAddress(t),
				LogicID:  logicID,
				Callsite: "Transfer!",
				Calldata: hexutil.Bytes(common.Hex2Bytes(calldata)),
			},
			expectedError: common.ErrAccountNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			logicCall, err := coreAPI.LogicCall(test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedLogicCall.Consumed, logicCall.Consumed)
			require.Equal(t, test.expectedLogicCall.Outputs, logicCall.Outputs)
			require.Equal(t, test.expectedLogicCall.Error, logicCall.Error)
		})
	}
}

func TestPublicCoreAPI_GetLogicManifest(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	randomHash := tests.RandomHash(t)
	tsHash := tests.GetTesseractHash(t, ts)

	logicID := getLogicID(t, ts.Address())
	logicIDWithoutState := getLogicID(t, tests.RandomAddress(t))

	poloManifest, jsonManifest, yamlManifest := tests.GetManifests(t, "./../../compute/manifests/erc20.json")

	s.setLogicManifest(logicID.String(), poloManifest)
	c.setTesseractByHash(t, ts)

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

func TestPublicCoreAPI_GetLogicStorageAt(t *testing.T) {
	ts := tests.CreateTesseract(t, nil)

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

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
	sm := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, chainManager, sm, nil)

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

			coreAPI := NewPublicCoreAPI(ixpool, chainManager, nil, nil)

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
	coreAPI := NewPublicCoreAPI(nil, chainManager, nil, nil)

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

	coreAPI := NewPublicCoreAPI(nil, c, sm, nil)

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

	c := NewMockChainManager(t)
	s := NewMockStateManager(t)
	coreAPI := NewPublicCoreAPI(nil, c, s, nil)

	c.setTesseractByHash(t, ts)

	tsHash := tests.GetTesseractHash(t, ts)
	acc := tests.GetRandomAccMetaInfo(t, 1)

	s.setAccountMetaInfo(t, ts.Address(), acc)

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

func TestPublicCoreAPI_CreateRPCReceipt(t *testing.T) {
	ixParams := tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t))
	testcases := []struct {
		name    string
		receipt *common.Receipt
		ix      *common.Interaction
		grid    map[common.Address]common.TesseractHeightAndHash
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

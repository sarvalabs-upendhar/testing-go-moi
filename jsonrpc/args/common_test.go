package args

import (
	"encoding/json"
	"testing"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestCreateRPCInteraction(t *testing.T) {
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
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t), tests.RandomKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	ixWithNilFields, err := common.NewInteraction(ixData, tests.RandomHash(t).Bytes())
	require.NoError(t, err)

	testcases := []struct {
		name          string
		grid          map[identifiers.Address]common.TesseractHeightAndHash
		ix            *common.Interaction
		expectedError error
	}{
		{
			name: "create rpc interaction for value transfer interaction",
			ix:   CreateInteractionWithTestData(t, common.IxValueTransfer, json.RawMessage{}),
		},
		{
			name: "create rpc interaction for asset creation interaction",
			ix:   CreateInteractionWithTestData(t, common.IxAssetCreate, assetCreatePayloadBytes),
		},
		{
			name: "create rpc interaction for logic deploy interaction",
			ix:   CreateInteractionWithTestData(t, common.IxLogicDeploy, logicPayloadBytes),
		},
		{
			name: "create rpc interaction for logic execute interaction",
			ix:   CreateInteractionWithTestData(t, common.IxLogicInvoke, logicPayloadBytes),
		},
		{
			name: "create rpc interaction with nil transfer values,perceived values, perceived proofs, ",
			ix:   ixWithNilFields,
		},
		{
			name: "create rpc interaction with grid data",
			ix:   CreateInteractionWithTestData(t, common.IxValueTransfer, json.RawMessage{}),
			grid: map[identifiers.Address]common.TesseractHeightAndHash{
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
			rpcIxn, err := CreateRPCInteraction(test.ix, test.grid, 0)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			CheckForRPCIxn(t, test.ix, rpcIxn, test.grid)
			CheckForRPCTesseractParts(t, test.grid, rpcIxn.Parts)
		})
	}
}

func TestGetRPCTesseractPartsFromGrid(t *testing.T) {
	testcases := []struct {
		name string
		grid map[identifiers.Address]common.TesseractHeightAndHash
	}{
		{
			name: "create rpc tesseract parts from grid",
			grid: tests.CreateTesseractPartsWithTestData(t).Grid,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			parts := GetRPCTesseractPartsFromGrid(test.grid)

			CheckForRPCTesseractParts(t, test.grid, parts)
		})
	}
}

func TestCreateRPCTesseractGridID(t *testing.T) {
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
			rpcTesseractGridID := CreateRPCTesseractGridID(test.gridID)

			CheckForRPCTesseractGridID(t, test.gridID, rpcTesseractGridID)
		})
	}
}

func TestCreateRPCContextLockInfos(t *testing.T) {
	contextLockInfos := make(map[identifiers.Address]common.ContextLockInfo)

	for i := 0; i < 3; i++ {
		contextLockInfos[tests.RandomAddress(t)] = common.ContextLockInfo{
			ContextHash:   tests.RandomHash(t),
			Height:        8,
			TesseractHash: tests.RandomHash(t),
		}
	}

	testcases := []struct {
		name             string
		contextLockInfos map[identifiers.Address]common.ContextLockInfo
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
			rpcContextLockInfos := CreateRPCContextLockInfos(test.contextLockInfos)

			CheckForRPCContextLockInfos(t, test.contextLockInfos, rpcContextLockInfos)
		})
	}
}

func TestCreateRPCHeader(t *testing.T) {
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
			rpcHeader := CreateRPCHeader(test.header)

			CheckForRPCHeader(t, test.header, rpcHeader)
		})
	}
}

func TestCreateRPCDeltaGroups(t *testing.T) {
	deltaGroups := make(map[identifiers.Address]*common.DeltaGroup)

	for i := 0; i < 3; i++ {
		deltaGroups[tests.RandomAddress(t)] = &common.DeltaGroup{
			Role:             4,
			BehaviouralNodes: tests.RandomKramaIDs(t, 2),
			RandomNodes:      tests.RandomKramaIDs(t, 2),
			ReplacedNodes:    tests.RandomKramaIDs(t, 2),
		}
	}

	testcases := []struct {
		name             string
		contextLockInfos map[identifiers.Address]*common.DeltaGroup
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
			rpcContextLockInfos := CreateRPCDeltaGroups(test.contextLockInfos)

			CheckForRPCDeltaGroups(t, test.contextLockInfos, rpcContextLockInfos)
		})
	}
}

func TestCreateRPCBody(t *testing.T) {
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
			rpcBody := CreateRPCBody(test.body)

			CheckForRPCBody(t, test.body, rpcBody)
		})
	}
}

func TestCreateRPCTesseract(t *testing.T) {
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
			ixParams: GetIxParamsWithInputComputeTrust(common.IxAssetCreate, assetPayloadBytes, 2, 3),
			tsParams: createTesseractParams(CreateHeaderCallbackWithTestData(t)),
		},
		{
			name: "create rpc tesseract for genesis tesseract",
			tsParams: createTesseractParams(func(header *common.TesseractHeader) {
				header.ClusterID = common.GenesisIdentifier
				header.FuelLimit = 0
				header.FuelUsed = 0
			}),
		},
		{
			name:     "nil interactions",
			ixParams: nil,
			tsParams: createTesseractParams(func(header *common.TesseractHeader) {
				header.FuelLimit = 0
				header.FuelUsed = 0
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

			CheckForRPCTesseract(t, ts, rpcTS)
		})
	}
}

func TestCreateRPCReceipt(t *testing.T) {
	ixParams := tests.GetIxParamsWithAddress(tests.RandomAddress(t), tests.RandomAddress(t))
	testcases := []struct {
		name    string
		receipt *common.Receipt
		ix      *common.Interaction
		grid    map[identifiers.Address]common.TesseractHeightAndHash
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
			receipt := CreateRPCReceipt(test.receipt, test.ix, test.grid, test.ixIndex)

			CheckForRPCReceipt(t, test.grid, test.ix, test.receipt, receipt, test.ixIndex)
		})
	}
}

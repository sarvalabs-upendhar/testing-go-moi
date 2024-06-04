package args

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
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

	tsHash := tests.RandomHash(t)
	participants := tests.CreateParticipantWithTestData(t, 1)

	testcases := []struct {
		name          string
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
			name: "create rpc interaction with particpants data",
			ix:   CreateInteractionWithTestData(t, common.IxValueTransfer, json.RawMessage{}),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcIxn, err := CreateRPCInteraction(test.ix, tsHash, participants, 0)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			CheckForRPCIxn(t, test.ix, tsHash, participants, rpcIxn)
		})
	}
}

func TestCreateRPCParticipants(t *testing.T) {
	testcases := []struct {
		name         string
		participants common.ParticipantsState
	}{
		{
			name:         "create rpc participants",
			participants: tests.CreateParticipantWithTestData(t, 3),
		},
		{
			name:         "empty participants",
			participants: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcParticipants := CreateRPCParticipants(test.participants)

			CheckForRPCParticipants(t, test.participants, rpcParticipants)
		})
	}
}

func TestCreateRPCPoXtData(t *testing.T) {
	testcases := []struct {
		name     string
		poxtData common.PoXtData
	}{
		{
			name:     "create rpc participants",
			poxtData: tests.CreatePoXtWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			rpcPoXt := CreateRPCPoXtData(test.poxtData)

			CheckForRPCPoxtData(t, test.poxtData, rpcPoXt)
		})
	}
}

func TestCreateRPCTesseract(t *testing.T) {
	assetPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetPayloadBytes, err := polo.Polorize(assetPayload)
	require.NoError(t, err)

	participants := tests.CreateParticipantWithTestData(t, 2)

	// make sure to fill at least one field of every field of tesseract so that we can verify that every field is copied
	createTesseractParams := func(clusterID common.ClusterID) *tests.CreateTesseractParams {
		return &tests.CreateTesseractParams{
			Participants: participants,
			TSDataCallback: func(ts *tests.TesseractData) {
				ts.InteractionsHash = tests.RandomHash(t)
				ts.ReceiptsHash = tests.RandomHash(t)
				ts.Epoch = big.NewInt(33)
				ts.Timestamp = 443
				ts.Operator = "guardian"
				ts.FuelUsed = 55
				ts.FuelLimit = 88
				ts.ConsensusInfo = tests.CreatePoXtWithTestData(t)
				ts.Seal = []byte{2, 3, 4}
				ts.SealBy = tests.RandomKramaIDs(t, 1)[0]
				ts.ConsensusInfo.ClusterID = clusterID
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
			tsParams: createTesseractParams("non-genesis"),
		},
		{
			name:     "create rpc tesseract for genesis tesseract",
			tsParams: createTesseractParams(common.GenesisIdentifier),
		},
		{
			name:     "nil interactions",
			ixParams: nil,
			tsParams: createTesseractParams("non-genesis"),
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
		name         string
		tsHash       common.Hash
		receipt      *common.Receipt
		ix           *common.Interaction
		participants common.ParticipantsState
		ixIndex      int
	}{
		{
			name:         "create rpc receipt",
			tsHash:       tests.RandomHash(t),
			receipt:      tests.CreateReceiptWithTestData(t),
			ix:           tests.CreateIX(t, ixParams),
			participants: tests.CreateParticipantWithTestData(t, 1),
			ixIndex:      8,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			receipt := CreateRPCReceipt(test.receipt, test.ix, test.tsHash, test.participants, test.ixIndex)

			CheckForRPCReceipt(t, test.tsHash, test.participants, test.ix, test.receipt, receipt, test.ixIndex)
		})
	}
}

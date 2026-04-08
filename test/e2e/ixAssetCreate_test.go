package e2e

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

// TODO Decide how to throw errors , use suite or require package
func (te *TestEnvironment) createAsset(
	acc tests.AccountWithMnemonic,
	assetCreatePayload *common.AssetCreatePayload,
) (common.Hash, error) {
	te.logger.Debug("create asset ",
		"sender", acc.ID, "symbol", assetCreatePayload.Symbol, "supply", assetCreatePayload.MaxSupply)

	payload, err := assetCreatePayload.Bytes()
	te.Suite.NoError(err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         acc.ID,
			SequenceID: moiclient.GetLatestSequenceID(te.T(), te.moiClient, acc.ID, 0),
		},
		FuelPrice: DefaultFuelPrice,
		FuelLimit: DefaultFuelLimit,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       acc.ID,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := moiclient.CreateSendIXFromIxData(te.T(), ixData, []moiclient.AccountKeyWithMnemonic{
		{
			ID:       acc.ID,
			KeyID:    0,
			Mnemonic: acc.Mnemonic,
		},
	})

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. check if asset descriptor matches with asset create payload
// 3. check if asset id account got created
// 4. check if senders balance updated
func validateAssetCreation(
	te *TestEnvironment,
	sender identifiers.Identifier,
	ixHash common.Hash,
	txnID int,
	isFailure bool,
	assetCreatePayload *common.AssetCreatePayload,
) {
	if isFailure {
		checkForReceiptFailure(te.T(), te.moiClient, ixHash)

		return
	}

	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var assetReceipt common.AssetCreationResult

	err := json.Unmarshal(receipt.IxOps[txnID].Data, &assetReceipt)
	require.NoError(te.T(), err)

	assetDescriptor, err := te.moiClient.AssetInfoByAssetID(context.Background(), &args.GetAssetInfoArgs{
		AssetID: assetReceipt.AssetID,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})

	te.Suite.NoError(err)
	require.Equal(te.T(), sender, assetDescriptor.Creator)
	require.Equal(te.T(), assetCreatePayload.Symbol, assetDescriptor.Symbol)
	require.Equal(te.T(), assetCreatePayload.MaxSupply.Uint64(), assetDescriptor.MaxSupply.ToInt().Uint64())
	require.Equal(te.T(), uint16(assetCreatePayload.Standard), assetDescriptor.AssetID.Standard())
	require.Equal(te.T(), assetCreatePayload.Dimension, assetDescriptor.Dimension.ToInt())
	require.Equal(te.T(), assetCreatePayload.Decimals, assetDescriptor.Decimals.ToInt())
	require.Equal(te.T(), assetCreatePayload.Manager, assetDescriptor.Manager)

	for k, v := range assetCreatePayload.StaticMetadata {
		require.Equal(te.T(), v, assetDescriptor.StaticMetadata[k].Bytes())
	}

	for k, v := range assetCreatePayload.DynamicMetaData {
		require.Equal(te.T(), v, assetDescriptor.DynamicMetadata[k].Bytes())
	}

	manifestBytes, err := te.moiClient.LogicManifest(context.Background(), &args.LogicManifestArgs{
		LogicID:  assetReceipt.AssetID.AsIdentifier(),
		Encoding: "JSON",
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})

	require.NoError(te.T(), err)
	require.Greater(te.T(), len(manifestBytes), 0)

	// TODO compare logic payload

	ts, err := te.moiClient.Tesseract(context.Background(), &args.TesseractArgs{
		ID: assetReceipt.AssetID.AsIdentifier(),
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	require.True(te.T(), ts.HasParticipant(assetReceipt.AssetID.AsIdentifier()))
	require.Equal(te.T(), uint64(0), ts.Height(assetReceipt.AssetID.AsIdentifier()))
}

func (te *TestEnvironment) TestAssetCreate() {
	acc := te.chooseRandomAccount()

	testcases := []struct {
		name               string
		assetCreatePayload *common.AssetCreatePayload
		isFailure          bool
		postTest           func(
			te *TestEnvironment,
			acc identifiers.Identifier,
			ixHash common.Hash,
			txnID int,
			isFailure bool,
			assetCreatePayload *common.AssetCreatePayload,
		)
		expectedError error
	}{
		// TODO: add more test cases
		{
			name: "create valid asset of type MAS0",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1000),
				common.MAS0,
				acc.ID,
				func(payload *common.AssetCreatePayload) {
					payload.Dimension = 0
					payload.StaticMetadata = map[string][]byte{
						"key1": []byte("value1"),
					}
				},
			),
			postTest: validateAssetCreation,
		},
		{
			name: "MAS1 asset creation should fail without logic payload",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1000),
				common.MAS1,
				acc.ID,
				func(payload *common.AssetCreatePayload) {
					payload.Dimension = 0
					payload.StaticMetadata = map[string][]byte{
						"key1": []byte("value1"),
					}
				},
			),
			isFailure: true,
			postTest:  validateAssetCreation,
		},
		{
			name: "invalid asset details",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1000),
				3, // invalid standard
				acc.ID,
				nil,
			),
			expectedError: common.ErrInvalidAssetStandard,
		},
	}

	for _, test := range testcases {
		te.Run(test.name, func() {
			ixHash, err := te.createAsset(acc, test.assetCreatePayload)
			if test.expectedError != nil {
				require.ErrorContains(te.T(), err, test.expectedError.Error())

				return
			}

			require.NoError(te.T(), err)
			test.postTest(te, acc.ID, ixHash, 0, test.isFailure, test.assetCreatePayload)
		})
	}
}

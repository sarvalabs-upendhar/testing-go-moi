package e2e

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/sarvalabs/go-moi-identifiers"
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
		"sender", acc.Addr, "symbol", assetCreatePayload.Symbol, "supply", assetCreatePayload.Supply)

	payload, err := assetCreatePayload.Bytes()
	te.Suite.NoError(err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Nonce:     moiclient.GetLatestNonce(te.T(), te.moiClient, acc.Addr),
		Sender:    acc.Addr,
		FuelPrice: DefaulFuelPrice,
		FuelLimit: DefaultFuelLimit,
		Payload:   payload,
	}

	sendIX := moiclient.CreateSendIXFromSendIXArgs(te.T(), sendIXArgs, acc.Mnemonic)

	return te.moiClient.SendInteractions(context.Background(), sendIX)
}

// 1. check if receipt generated for ix successfully
// 2. check if asset descriptor matches with asset create payload
// 3. check if asset id account got created
// 4. check if senders balance updated
func validateAssetCreation(
	te *TestEnvironment,
	sender identifiers.Address,
	ixHash common.Hash,
	assetCreatePayload *common.AssetCreatePayload,
) {
	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var assetReceipt common.AssetCreationReceipt

	err := json.Unmarshal(receipt.ExtraData, &assetReceipt)
	require.NoError(te.T(), err)

	assetDescriptor, err := te.moiClient.AssetInfoByAssetID(context.Background(), &args.GetAssetInfoArgs{
		AssetID: assetReceipt.AssetID,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	require.Equal(te.T(), assetCreatePayload.Symbol, assetDescriptor.Symbol)
	require.Equal(te.T(), assetCreatePayload.Supply.Uint64(), assetDescriptor.Supply.ToInt().Uint64())
	require.Equal(te.T(), uint16(assetCreatePayload.Standard), assetDescriptor.Standard.ToInt())
	require.Equal(te.T(), assetCreatePayload.Dimension, assetDescriptor.Dimension.ToInt())
	require.Equal(te.T(), assetCreatePayload.IsStateFul, assetDescriptor.IsStateFul)
	require.Equal(te.T(), assetCreatePayload.IsLogical, assetDescriptor.IsLogical)
	// TODO compare logic payload

	ts, err := te.moiClient.Tesseract(context.Background(), &args.TesseractArgs{
		Address: assetReceipt.AssetID.Address(),
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	require.Equal(te.T(), ts.Address(), assetReceipt.AssetID.Address())
	require.Equal(te.T(), uint64(0), ts.Header.Height.ToUint64())

	bal, err := te.moiClient.Balance(context.Background(), &args.BalArgs{
		Address: sender,
		AssetID: assetReceipt.AssetID,
		Options: args.TesseractNumberOrHash{
			TesseractNumber: &args.LatestTesseractHeight,
		},
	})
	te.Suite.NoError(err)

	require.Equal(te.T(), assetCreatePayload.Supply.Uint64(), bal.ToInt().Uint64())
}

func (te *TestEnvironment) TestAssetCreate() {
	acc := te.chooseRandomAccount()

	testcases := []struct {
		name               string
		assetCreatePayload *common.AssetCreatePayload
		postTest           func(
			te *TestEnvironment,
			acc identifiers.Address,
			ixHash common.Hash,
			assetCreatePayload *common.AssetCreatePayload,
		)
		expectedError error
	}{
		{
			name: "create valid asset of type MAS0",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1000),
				common.MAS0,
				func(payload *common.AssetCreatePayload) {
					payload.Dimension = 1
					payload.IsStateFul = true
					payload.IsLogical = true
				},
			),
			postTest: validateAssetCreation,
		},
		{
			name: "create valid asset of type MAS1",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1),
				common.MAS1,
				func(payload *common.AssetCreatePayload) {
					payload.Dimension = 1
				},
			),
			postTest: validateAssetCreation,
		},
		{
			name: "invalid asset standard",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1000),
				2,
				nil,
			),
			expectedError: common.ErrInvalidAssetStandard,
		},
		{
			name: "invalid asset supply for MAS1 asset standard",
			assetCreatePayload: createAssetCreatePayload(
				tests.GetRandomUpperCaseString(te.T(), 8),
				big.NewInt(1000),
				common.MAS1,
				nil,
			),
			expectedError: common.ErrInvalidAssetSupply,
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
			test.postTest(te, acc.Addr, ixHash, test.assetCreatePayload)
		})
	}
}

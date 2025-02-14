package e2e

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

var DefaultBeneficiary identifiers.Identifier = identifiers.RandomParticipantIDv0().AsIdentifier()

func createAssetCreatePayload(
	symbol string,
	supply *big.Int,
	standard common.AssetStandard,
	payloadCallBack func(payload *common.AssetCreatePayload),
) *common.AssetCreatePayload {
	payload := &common.AssetCreatePayload{
		Symbol:   symbol,
		Supply:   supply,
		Standard: standard,
	}

	if payloadCallBack != nil {
		payloadCallBack(payload)
	}

	return payload
}

func transferAsset(
	te *TestEnvironment,
	sender tests.AccountWithMnemonic,
	payload *common.AssetActionPayload,
) {
	ixHash, err := te.transferAsset(sender, payload)
	require.NoError(te.T(), err)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConfirmIxTimeout)
	defer cancel()

	receipt := moiclient.RetryFetchReceipt(te.T(), ctx, te.moiClient, ixHash)
	require.Equal(te.T(), common.ReceiptOk, receipt.Status)
}

func approveAsset(te *TestEnvironment, sender tests.AccountWithMnemonic, payload *common.AssetActionPayload) {
	ixHash, err := te.approveAsset(sender, payload)
	require.NoError(te.T(), err)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConfirmIxTimeout)
	defer cancel()

	receipt := moiclient.RetryFetchReceipt(te.T(), ctx, te.moiClient, ixHash)
	require.Equal(te.T(), common.ReceiptOk, receipt.Status)
}

func lockupAsset(te *TestEnvironment, sender tests.AccountWithMnemonic, payload *common.AssetActionPayload) {
	ixHash, err := te.lockupAsset(sender, payload)
	require.NoError(te.T(), err)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConfirmIxTimeout)
	defer cancel()

	receipt := moiclient.RetryFetchReceipt(te.T(), ctx, te.moiClient, ixHash)
	require.Equal(te.T(), common.ReceiptOk, receipt.Status)
}

func createAsset(
	te *TestEnvironment,
	sender tests.AccountWithMnemonic,
	payload *common.AssetCreatePayload,
) identifiers.AssetID {
	ixHash, err := te.createAsset(sender, payload)
	require.NoError(te.T(), err)

	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var assetReceipt common.AssetCreationResult

	err = json.Unmarshal(receipt.IxOps[0].Data, &assetReceipt)
	require.NoError(te.T(), err)

	return assetReceipt.AssetID
}

func createParticipant(
	te *TestEnvironment,
	sender tests.AccountWithMnemonic,
	payload *common.ParticipantCreatePayload,
) {
	ixHash, err := te.createParticipant(sender, payload)
	require.NoError(te.T(), err)

	checkForReceiptSuccess(te.T(), te.moiClient, ixHash)
}

func deployLogic(
	te *TestEnvironment,
	sender tests.AccountWithMnemonic,
	payload *common.LogicPayload,
) identifiers.LogicID {
	ixHash, err := te.deployLogic(sender, payload)
	require.NoError(te.T(), err)

	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var logicDeployReceipt common.LogicDeployResult

	err = json.Unmarshal(receipt.IxOps[0].Data, &logicDeployReceipt)
	require.NoError(te.T(), err)

	return logicDeployReceipt.LogicID
}

func getBalance(te *TestEnvironment, id identifiers.Identifier, assetID identifiers.AssetID, height int64) uint64 {
	senderBal, err := te.moiClient.Balance(context.Background(), &rpcargs.BalArgs{
		ID:      id,
		AssetID: assetID,
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &height,
		},
	})
	te.Suite.NoError(err)

	return senderBal.ToInt().Uint64()
}

// func checkForReceiptFailure(t *testing.T, client *moiclient.Client, ixHash common.Hash) {
//	t.Helper()
//
//	// make sure interaction executed successfully
//	ctx, cancel := context.WithTimeout(context.Background(), DefaultConfirmIxTimeout)
//	defer cancel()
//
//	receipt := moiclient.RetryFetchReceipt(t, ctx, client, ixHash)
//	require.Equal(t, common.ReceiptStateReverted, receipt.Status)
// }

func checkForReceiptSuccess(t *testing.T, client *moiclient.Client, ixHash common.Hash) *rpcargs.RPCReceipt {
	t.Helper()
	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConfirmIxTimeout)
	defer cancel()

	receipt := moiclient.RetryFetchReceipt(t, ctx, client, ixHash)
	require.Equal(t, common.ReceiptOk, receipt.Status)

	return receipt
}

func DocGen(t *testing.T, values map[string]any) polo.Document {
	t.Helper()

	doc, err := polo.PolorizeDocument(values, polo.DocStructs(), polo.DocStringMaps())
	require.NoError(t, err, "cannot generate document")

	return doc
}

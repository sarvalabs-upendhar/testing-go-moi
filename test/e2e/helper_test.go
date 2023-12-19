package e2e

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/require"
)

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
	receiver common.Address,
	transferValues map[common.AssetID]*big.Int,
) {
	ixHash, err := te.transferAsset(sender, receiver, transferValues)
	require.NoError(te.T(), err)

	// make sure interaction executed successfully
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConfirmIxTimeout)
	defer cancel()

	receipt := moiclient.RetryFetchReceipt(te.T(), ctx, te.moiClient, ixHash)
	require.Equal(te.T(), receipt.Status, common.ReceiptOk)
}

func createAsset(
	te *TestEnvironment,
	sender tests.AccountWithMnemonic,
	payload *common.AssetCreatePayload,
) common.AssetID {
	ixHash, err := te.createAsset(sender, payload)
	require.NoError(te.T(), err)

	receipt := checkForReceiptSuccess(te.T(), te.moiClient, ixHash)

	var assetReceipt common.AssetCreationReceipt

	err = json.Unmarshal(receipt.ExtraData, &assetReceipt)
	require.NoError(te.T(), err)

	return assetReceipt.AssetID
}

func getBalance(te *TestEnvironment, addr common.Address, assetID common.AssetID, height int64) uint64 {
	senderBal, err := te.moiClient.Balance(context.Background(), &rpcargs.BalArgs{
		Address: addr,
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
//	require.Equal(t, common.ReceiptFailed, receipt.Status)
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

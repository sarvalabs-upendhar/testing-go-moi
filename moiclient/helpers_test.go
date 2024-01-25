package moiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/jsonrpc/websocket"

	"github.com/sarvalabs/go-moi/common/tests"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// IxnTimeout is the max time to wait for an ixn to be completed
const IxnTimeout = 1 * time.Minute

var (
	genesisHeight         = int64(0)
	LatestTesseractNumber = int64(-1)
)

// makeHTTPRequest takes method, args and makes an HTTP POST request to node specified by url constant
// returning a response with data, status, and error.
func makeHTTPRequest(t *testing.T, url string, method string, args interface{}) *rpcargs.Response {
	t.Helper()

	params, err := json.Marshal(args)
	require.NoError(t, err)

	values := &jsonrpcMessage{
		Version: vsn,
		ID:      strconv.AppendUint(nil, uint64(1), 10),
		Method:  method,
		Params:  params,
	}

	jsonData, err := json.Marshal(values)
	require.NoError(t, err)

	httpResponse, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData)) //nolint
	require.NoError(t, err)

	// status should be >= 200 && < 300
	require.GreaterOrEqual(t, httpResponse.StatusCode, 200)
	require.Less(t, httpResponse.StatusCode, 300)

	var jsonResp jsonrpcMessage

	err = json.NewDecoder(httpResponse.Body).Decode(&jsonResp)
	require.NoError(t, err)
	require.Nil(t, jsonResp.Error)

	var resp rpcargs.Response

	err = json.Unmarshal(jsonResp.Result, &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	return &resp
}

// httpTesseract returns RPCTesseract based on the given arguments
func httpTesseract(t *testing.T, url string, args interface{}) *rpcargs.RPCTesseract {
	t.Helper()

	resp := makeHTTPRequest(t, url, "moi.Tesseract", args)

	var tess rpcargs.RPCTesseract

	err := json.Unmarshal(resp.Data, &tess)
	require.NoError(t, err)

	return &tess
}

func createAssetWithNonce(t *testing.T, client *Client, addr identifiers.Address, mnemonic string, nonce uint64) {
	t.Helper()

	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Nonce:     nonce,
		Sender:    addr,
		FuelPrice: big.NewInt(1),
		FuelLimit: 200,
		Payload:   payload,
	}

	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	_, err = client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)
}

// moiclientRetryFetchReceipt keeps trying to fetch receipt for given ixHash until it is timed out
// and also checks if moi client response matches with http response
// Use this to check if interaction is successful on the chain.
func moiclientRetryFetchReceipt(
	t *testing.T,
	ctx context.Context,
	client *Client,
	ixHash common.Hash,
) *rpcargs.RPCReceipt {
	t.Helper()

	receiptArgs := &rpcargs.ReceiptArgs{
		Hash: ixHash,
	}

	for {
		select {
		case <-ctx.Done():
			require.FailNow(t, "ix receipt not found,"+
				" as forming the ICS took more time, so try running tests again", ixHash)
		default:
			receipt, err := client.InteractionReceipt(ctx, receiptArgs)
			if err == nil {
				require.Equal(t, common.ReceiptOk, receipt.Status)

				return receipt
			}

			time.Sleep(time.Second)
		}
	}
}

// createAsset creates asset named "MOI"
func createAsset(t *testing.T, client *Client, addr identifiers.Address, mnemonic string) (
	common.Hash, identifiers.Address,
) {
	t.Helper()

	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	sendIXArgs := &common.SendIXArgs{
		Type:      common.IxAssetCreate,
		Nonce:     GetLatestNonce(t, client, addr),
		Sender:    addr,
		FuelPrice: big.NewInt(1),
		FuelLimit: 200,
		Payload:   payload,
	}

	sendIX := CreateSendIXFromSendIXArgs(t, sendIXArgs, mnemonic)

	ixHash, err := client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	receipt := moiclientRetryFetchReceipt(t, ctx, client, ixHash)

	var assetReceipt common.AssetCreationReceipt
	err = json.Unmarshal(receipt.ExtraData, &assetReceipt)
	require.NoError(t, err)

	return ixHash, assetReceipt.AssetAccount
}

func checkForCallReceipt(
	t *testing.T,
	expectedReceipt *rpcargs.RPCReceipt,
	actualReceipt *rpcargs.RPCReceipt,
) {
	t.Helper()

	require.Equal(t, expectedReceipt.IxType, actualReceipt.IxType)
	require.Equal(t, expectedReceipt.FuelUsed, actualReceipt.FuelUsed)
	require.Equal(t, expectedReceipt.ExtraData, actualReceipt.ExtraData)
	require.Equal(t, expectedReceipt.From, actualReceipt.From)
	require.Equal(t, expectedReceipt.To, actualReceipt.To)
}

func createTesseractFilter(t *testing.T, ctx context.Context, moiClient *Client) *rpcargs.FilterResponse {
	t.Helper()

	tsFilter, err := moiClient.NewTesseractFilter(ctx, &rpcargs.TesseractFilterArgs{})
	require.NoError(t, err)

	return tsFilter
}

func createTesseractsByAccountFilter(
	t *testing.T,
	ctx context.Context,
	moiClient *Client,
	addr identifiers.Address,
) *rpcargs.FilterResponse {
	t.Helper()

	tsByAccFilter, err := moiClient.NewTesseractsByAccountFilter(ctx, &rpcargs.TesseractByAccountFilterArgs{
		Addr: addr,
	})
	require.NoError(t, err)

	return tsByAccFilter
}

func createPendingIxnsFilter(t *testing.T, ctx context.Context, moiClient *Client) *rpcargs.FilterResponse {
	t.Helper()

	ixnsFilter, err := moiClient.PendingIxnsFilter(ctx, &rpcargs.PendingIxnsFilterArgs{})
	require.NoError(t, err)

	return ixnsFilter
}

func createLogFilter(
	t *testing.T,
	ctx context.Context,
	moiClient *Client,
	addr identifiers.Address,
) *rpcargs.FilterResponse {
	t.Helper()

	logFilter, err := moiClient.NewLogFilter(ctx, &websocket.LogQuery{
		Address: addr,
	})
	require.NoError(t, err)

	return logFilter
}

func getRPCTesseractUntilTimeout(
	t *testing.T,
	ctx context.Context,
	client *Client,
	filterQueryArgs *rpcargs.FilterArgs,
	subscriptionType rpcargs.SubscriptionType,
	count int,
) []*rpcargs.RPCTesseract {
	t.Helper()

	rpcTS := make([]*rpcargs.RPCTesseract, 0)
	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		filterChanges, err := client.GetFilterChanges(
			ctx,
			filterQueryArgs,
			subscriptionType,
		)
		require.NoError(t, err)

		rpcTesseracts, ok := filterChanges.([]*rpcargs.RPCTesseract)
		require.True(t, ok)

		count -= len(rpcTesseracts)
		rpcTS = append(rpcTS, rpcTesseracts...)

		// TODO: change to count == 0, after issue #756 is resolved
		// Less than or equal to 0 to capture extra tesseract
		if count <= 0 {
			return rpcTS, false
		}

		return nil, true
	})
	require.NoError(t, err)

	return rpcTS
}

func getIxHashesUntilTimeout(
	t *testing.T,
	ctx context.Context,
	client *Client,
	filterQueryArgs *rpcargs.FilterArgs,
	subscriptionType rpcargs.SubscriptionType,
	count int,
) []*common.Hash {
	t.Helper()

	ixHashes := make([]*common.Hash, 0)
	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		filterChanges, err := client.GetFilterChanges(
			ctx,
			filterQueryArgs,
			subscriptionType,
		)
		require.NoError(t, err)

		ixnHash, ok := filterChanges.([]*common.Hash)
		require.True(t, ok)

		count -= len(ixnHash)
		ixHashes = append(ixHashes, ixnHash...)

		if count == 0 {
			return ixHashes, false
		}

		return nil, true
	})
	require.NoError(t, err)

	return ixHashes
}

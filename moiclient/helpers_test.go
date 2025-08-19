package moiclient

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/sarvalabs/go-moi/common/tests"

	"github.com/sarvalabs/go-polo"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// IxnTimeout is the max time to wait for an ixn to be completed
const IxnTimeout = 1 * time.Minute

var (
	manifest = func() string {
		// Register the PISA element registry with the EngineIO package
		engineio.RegisterEngine(pisa.NewEngine())

		// Read manifest file
		manifest, err := engineio.NewManifestFromFile("../compute/exlogics/tokenledger/tokenledger.yaml")
		if err != nil {
			panic(err)
		}

		encodedManifest, err := manifest.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return hex.EncodeToString(encodedManifest)
	}()

	genesisHeight         = int64(0)
	LatestTesseractNumber = int64(-1)
)

// makeHTTPRequest takes method, args and makes an HTTP POST request to node specified by url constant
// returning a response with data, status, and error.
func makeHTTPRequest(t *testing.T, url string, method string, result interface{}, args ...interface{}) {
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

	httpResponse, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)

	// status should be >= 200 && < 300
	require.GreaterOrEqual(t, httpResponse.StatusCode, 200)
	require.Less(t, httpResponse.StatusCode, 300)

	var jsonResp jsonrpcMessage

	err = json.NewDecoder(httpResponse.Body).Decode(&jsonResp)
	require.NoError(t, err)
	require.Nil(t, jsonResp.Error)

	err = json.Unmarshal(jsonResp.Result, &result)
	require.NoError(t, err)
}

// httpTesseract returns RPCTesseract based on the given arguments
func httpTesseract(t *testing.T, url string, args ...interface{}) *rpcargs.RPCTesseract {
	t.Helper()

	var tess rpcargs.RPCTesseract

	makeHTTPRequest(t, url, "moi.Tesseract", &tess, args...)

	return &tess
}

func createAssetWithNonce(t *testing.T, client *Client, id identifiers.Identifier,
	nonce uint64, key tests.AccountWithMnemonic,
) {
	t.Helper()

	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         id,
			SequenceID: nonce,
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 200,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := CreateSendIXFromIxData(t, ixData, []AccountKeyWithMnemonic{
		{
			ID:       id,
			Mnemonic: key.Mnemonic,
		},
	})

	_, err = client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)
}

// createAsset creates asset named "MOI"
func createAsset(t *testing.T, client *Client, id identifiers.Identifier, key tests.AccountWithMnemonic) (
	common.Hash, identifiers.Identifier,
) {
	t.Helper()

	supply, _ := new(big.Int).SetString("130D41", 16)

	assetCreationPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
		Supply: supply,
	}

	payload, err := polo.Polorize(assetCreationPayload)
	require.NoError(t, err)

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         id,
			SequenceID: GetLatestSequenceID(t, client, id, 0),
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 200,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: payload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       id,
				LockType: common.MutateLock,
			},
		},
	}

	sendIX := CreateSendIXFromIxData(t, ixData, []AccountKeyWithMnemonic{
		{
			ID:       key.ID,
			Mnemonic: key.Mnemonic,
		},
	})

	ixHash, err := client.SendInteractions(context.Background(), sendIX)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), IxnTimeout)
	defer cancel()

	receipt := RetryFetchReceipt(t, ctx, client, ixHash)
	require.Equal(t, common.ReceiptOk, receipt.Status)

	var assetReceipt common.AssetCreationResult
	err = json.Unmarshal(receipt.IxOps[0].Data, &assetReceipt)
	require.NoError(t, err)

	return ixHash, assetReceipt.AssetID.AsIdentifier()
}

func checkForCallReceipt(
	t *testing.T,
	expectedReceipt *rpcargs.RPCReceipt,
	actualReceipt *rpcargs.RPCReceipt,
) {
	t.Helper()

	require.Equal(t, expectedReceipt.FuelUsed, actualReceipt.FuelUsed)
	fmt.Println("111", expectedReceipt.IxOps[0].Data, actualReceipt.IxOps[0].Data)
	fmt.Println("###", string(expectedReceipt.IxOps[0].Data), string(actualReceipt.IxOps[0].Data))
	require.Equal(t, expectedReceipt.IxOps[0].Data, actualReceipt.IxOps[0].Data)
	require.Equal(t, expectedReceipt.IxOps[0].TxType, actualReceipt.IxOps[0].TxType)
	require.Equal(t, expectedReceipt.From, actualReceipt.From)
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
	id identifiers.Identifier,
) *rpcargs.FilterResponse {
	t.Helper()

	tsByAccFilter, err := moiClient.NewTesseractsByAccountFilter(ctx, &rpcargs.TesseractByAccountFilterArgs{
		ID: id,
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
	id identifiers.Identifier,
) *rpcargs.FilterResponse {
	t.Helper()

	logFilter, err := moiClient.NewLogFilter(ctx, &jsonrpc.LogQuery{
		ID: id,
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

package moiclient

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/api"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

var LatestTesseractNumber = int64(-1)

// IxnTimeout is the max time to wait for an ixn to be completed
// 180 seconds chosen as it is taking 90 seconds in worst case to execute interaction
// so we are taking (2*90) seconds to be on safe side
const IxnTimeout = 180 * time.Second

var (
	sender               = types.HexToAddress("377a4674fca572f072a8176d61b86d9015914b9df0a57bb1d80fafecce233084")
	receiver             = "bdfa7699df0d291d368a7d6bdda0a34d703458d2ba2c76c6a5dcac0915ce16ad"
	genesisHeight        = int64(0)
	deployLogicHeight    = int64(1)
	executeLogicHeight   = int64(2)
	createAssetHeight    = int64(3)
	transferTokensHeight = int64(4)
	ixnPendingCount      = 10
)

// Reads the ERC20 JSON Manifest and returns it as POLO encoded hex string
func readERC20Manifest(t *testing.T) string {
	t.Helper()

	// Read erc20.json manifest from jug/manifests
	data, err := ioutil.ReadFile("./../jug/manifests/erc20.json")
	require.NoError(t, err)

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
	// Decode the JSON manifest into a Manifest object
	manifest, err := engineio.NewManifest(data, engineio.JSON)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	encoded, err := manifest.Encode(engineio.POLO)
	require.NoError(t, err)

	// Hex encode the POLO manifest
	return hex.EncodeToString(encoded)
}

// makeHTTPRequest takes method, args and makes an HTTP POST request to node http://0.0.0.0:1600,
// returning a response with data, status, and error.
func makeHTTPRequest(t *testing.T, method string, args interface{}) *ptypes.Response {
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

	httpResponse, err := http.Post("http://0.0.0.0:1600", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)

	// status should be >= 200 && < 300
	require.GreaterOrEqual(t, httpResponse.StatusCode, 200)
	require.Less(t, httpResponse.StatusCode, 300)

	var jsonResp jsonrpcMessage

	err = json.NewDecoder(httpResponse.Body).Decode(&jsonResp)
	require.NoError(t, err)
	require.Nil(t, jsonResp.Error)

	var resp ptypes.Response

	err = json.Unmarshal(jsonResp.Result, &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	return &resp
}

// getIXArgsForLogicDeployment returns interaction args for logic deployment
func getIXArgsForLogicDeployment(t *testing.T) *ptypes.SendIXArgs {
	t.Helper()

	logicDeployArgs := &ptypes.LogicDeployArgs{
		Manifest: readERC20Manifest(t),
		Callsite: "Seeder!",
		Calldata: "0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29ec4" +
			"42dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49",
	}

	payload, err := json.Marshal(logicDeployArgs)
	require.NoError(t, err)

	ixArgs := &ptypes.SendIXArgs{
		Type:      7,
		FuelPrice: "030D40",
		FuelLimit: "030D40",
		Sender:    sender.Hex(),
		Payload:   payload,
	}

	return ixArgs
}

// getTesseract returns tesseract for the given sender and height
func getTesseract(t *testing.T, client *Client, sender string, height *int64) *ptypes.RPCTesseract {
	t.Helper()

	args := &ptypes.TesseractArgs{
		From:             sender,
		WithInteractions: true,
		Options: ptypes.TesseractNumberOrHash{
			TesseractNumber: height,
		},
	}

	ts, err := client.Tesseract(args)
	require.NoError(t, err)

	return ts
}

// getAssetID returns assetID for the given sender and height
func getAssetID(t *testing.T, client *Client, sender string, height *int64) string {
	t.Helper()

	ts := getTesseract(t, client, sender, height)

	receiptArgs := &ptypes.ReceiptArgs{
		Hash: ts.Ixns[0].Hash.Hex(),
	}

	receipt, err := client.InteractionReceipt(receiptArgs)
	require.NoError(t, err)

	var assetReceipt types.AssetCreationReceipt

	err = json.Unmarshal(receipt.ExtraData, &assetReceipt)
	require.NoError(t, err)

	return assetReceipt.AssetID
}

// getLogicID returns logicID for the given sender and height
func getLogicID(t *testing.T, client *Client, sender string, height *int64) string {
	t.Helper()

	ts := getTesseract(t, client, sender, height)

	receiptArgs := &ptypes.ReceiptArgs{
		Hash: ts.Ixns[0].Hash.Hex(),
	}

	receipt, err := client.InteractionReceipt(receiptArgs)
	require.NoError(t, err)

	var logicReceipt types.LogicDeployReceipt

	err = json.Unmarshal(receipt.ExtraData, &logicReceipt)
	require.NoError(t, err)

	return logicReceipt.LogicID
}

// retryFetchReceipt keeps trying to fetch receipt for given ixHash until it is timed out
// and also checks if moi client response matches with http response
// Use this to check if interaction is successful on the chain.
func retryFetchReceipt(t *testing.T, ctx context.Context, client *Client, ixHash string) *types.Receipt {
	t.Helper()

	receiptArgs := &ptypes.ReceiptArgs{
		Hash: ixHash,
	}

	for {
		select {
		case <-ctx.Done():
			require.FailNow(t, "ix receipt not found,"+
				" as forming the ICS took more time, so try running tests again")
		default:
			receipt, err := client.InteractionReceipt(receiptArgs)
			if err == nil {
				httpReceipt := httpInteractionReceipt(t, receiptArgs)
				require.Equal(t, httpReceipt, receipt)

				return receipt
			}

			time.Sleep(time.Second)
		}
	}
}

// httpTesseract returns RPCTesseract based on the given arguments
func httpTesseract(t *testing.T, args interface{}) *ptypes.RPCTesseract {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.Tesseract", args)

	var tess ptypes.RPCTesseract

	err := json.Unmarshal(resp.Data, &tess)
	require.NoError(t, err)

	return &tess
}

// httpGetAssetInfoByAssetID returns asset description for the given assetID
func httpGetAssetInfoByAssetID(t *testing.T, assetID string) *types.AssetDescriptor {
	t.Helper()

	args := &ptypes.AssetDescriptorArgs{
		AssetID: assetID,
	}

	resp := makeHTTPRequest(t, "moi.AssetInfoByAssetID", args)

	var assetInfo types.AssetDescriptor

	err := json.Unmarshal(resp.Data, &assetInfo)
	require.NoError(t, err)

	return &assetInfo
}

// httpGetBalance returns the balance of assetID for given BalArgs
func httpGetBalance(t *testing.T, args *ptypes.BalArgs) uint64 {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.Balance", args)

	var bal uint64

	err := json.Unmarshal(resp.Data, &bal)
	require.NoError(t, err)

	return bal
}

// httpTDU retrieves the TDU of the queried address
func httpTDU(t *testing.T, args *ptypes.TesseractArgs) types.AssetMap {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.TDU", args)

	var assetMap types.AssetMap

	err := json.Unmarshal(resp.Data, &assetMap)
	require.NoError(t, err)

	return assetMap
}

// httpGetContextInfo returns the context Info of the queried address.
func httpGetContextInfo(t *testing.T, args *ptypes.ContextInfoArgs) *ptypes.ContextResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.ContextInfo", args)

	var contextResp ptypes.ContextResponse

	err := json.Unmarshal(resp.Data, &contextResp)
	require.NoError(t, err)

	return &contextResp
}

// httpInteractionReceipt returns the receipt of the interaction for given hash
func httpInteractionReceipt(t *testing.T, args *ptypes.ReceiptArgs) *types.Receipt {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.InteractionReceipt", args)

	var receipt types.Receipt

	err := json.Unmarshal(resp.Data, &receipt)
	require.NoError(t, err)

	return &receipt
}

// httpInteractionCount returns the number of interactions sent for the given address
func httpInteractionCount(t *testing.T, args *ptypes.InteractionCountArgs) uint64 {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.InteractionCount", args)

	var count uint64

	err := json.Unmarshal(resp.Data, &count)
	require.NoError(t, err)

	return count
}

// httpPendingInteractionCount returns the number of interactions sent for the given address.
func httpPendingInteractionCount(t *testing.T, args *ptypes.InteractionCountArgs) *uint64 {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.PendingInteractionCount", args)

	var count uint64

	err := json.Unmarshal(resp.Data, &count)
	require.NoError(t, err)

	return &count
}

// httpStorage returns the data associated with the given httpStorage slot
func httpStorage(t *testing.T, args *ptypes.GetStorageArgs) []byte {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.Storage", args)

	var res []byte

	err := json.Unmarshal(resp.Data, &res)
	require.NoError(t, err)

	return res
}

// httpAccountState returns the account state of the given address
func httpAccountState(t *testing.T, args *ptypes.GetAccountArgs) *types.Account {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.AccountState", args)

	var account types.Account

	err := json.Unmarshal(resp.Data, &account)
	require.NoError(t, err)

	return &account
}

// httpLogicManifest returns the manifest associated with the given logic id
func httpLogicManifest(t *testing.T, args *ptypes.LogicManifestArgs) []byte {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.LogicManifest", args)

	var res []byte

	err := json.Unmarshal(resp.Data, &res)
	require.NoError(t, err)

	return res
}

// httpContent returns the interactions present in the given IxPool.
func httpContent(t *testing.T, args *ptypes.ContentArgs) *api.ContentResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.Content", args)

	var contentRes api.ContentResponse

	err := json.Unmarshal(resp.Data, &contentRes)
	require.NoError(t, err)

	return &contentRes
}

// httpContentFrom returns the interactions present in IxPool for the queried address.
func httpContentFrom(t *testing.T, args *ptypes.IxPoolArgs) *api.ContentFromResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.ContentFrom", args)

	var content api.ContentFromResponse

	err := json.Unmarshal(resp.Data, &content)
	require.NoError(t, err)

	return &content
}

// httpStatus returns the number of pending and queued interactions in the IxPool.
func httpStatus(t *testing.T, args *ptypes.StatusArgs) *api.StatusResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.Status", args)

	var status api.StatusResponse

	err := json.Unmarshal(resp.Data, &status)
	require.NoError(t, err)

	return &status
}

// httpInspect returns the interactions present in the IxPool in a clear and easy-to-read format,
func httpInspect(t *testing.T, args *ptypes.InspectArgs) *api.InspectResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.Inspect", args)

	var response api.InspectResponse

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return &response
}

// httpWaitTime returns the wait time for an account in IxPool, based on the queried address.
func httpWaitTime(t *testing.T, args *ptypes.IxPoolArgs) int64 {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.WaitTime", args)

	var time int64

	err := json.Unmarshal(resp.Data, &time)
	require.NoError(t, err)

	return time
}

// httpPeers returns an array of Krama IDs connected to a client
func httpPeers(t *testing.T, args *ptypes.NetArgs) *[]kramaid.KramaID {
	t.Helper()

	resp := makeHTTPRequest(t, "net.Peers", args)

	var response []kramaid.KramaID

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return &response
}

// httpDBGet returns raw value of the key stored in the database
func httpDBGet(t *testing.T, args *ptypes.DebugArgs) string {
	t.Helper()

	resp := makeHTTPRequest(t, "debug.DBGet", args)

	var response string

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return response
}

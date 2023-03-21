package moiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	manifest             = "0e8f0206165e9e25de28ce940190970190970131504953413f0ede14af0" +
		"20306d302e602b305c6059308a608b30cc60c6" +
		"e616d65202020202020205b20737472696e67205d0173796d626f6c20202020205b20737472696e67205d02737570706c79202020202" +
		"05b2075696e743634205d0362616c616e6365732020205b206d61705b616464726573735d75696e743634205d04616c6c6f77616e636" +
		"573205b206d61705b616464726573735d6d61705b616464726573735d75696e743634205d8f01060ee009e009ee09ef010306d301e60" +
		"1d303e603d305e6056e616d65205b737472696e675d0173796d626f6c205b737472696e675d02737570706c79205b75696e7436345d0" +
		"3736565646572205b616464726573735d3f06f003100000140000100001140001100002140002130103100203410102001401036f030" +
		"6a301b60175696e7436342831302901626f6f6c287472756529ef04030ed304ee04f3098e0ac310de10e316fe1683209e20a32dbe2d8" +
		"33e9e3ec34ede4ef3598e5a7f06404ec002ce024e616d652f03066e616d65205b737472696e675d2f0660130000110000017f06606e8" +
		"0038e0353796d626f6c2f030673796d626f6c205b737472696e675d2f0660130001110000029f010680018e01c003ce03446563696d6" +
		"16c732f0306646563696d616c73205b75696e7436345d3f069001050100000600110000039f0106b001be01d003de03546f74616c537" +
		"570706c792f0306737570706c79205b75696e7436345d2f0660130002110000049f01069e01ae03d005de0542616c616e63654f662f0" +
		"30661646472205b616464726573735d2f030662616c616e6365205b75696e7436345d3f06d00113000310010040020001110200059f0" +
		"1069e019e06e008ee08416c6c6f77616e63656f0306f30186026f776e6572205b616464726573735d017370656e646572205b6164647" +
		"26573735d2f0306616c6c6f77616e6365205b75696e7436345d3f06c0021300041001004002000110030140040203110400069f01068" +
		"e01d008d008de08417070726f766521af010306f30186029304a6046f776e6572205b616464726573735d017370656e646572205b616" +
		"464726573735d02616d6f756e74205b75696e7436345d3f0690061300041001004002000133030256030501040a03030405010400070" +
		"43c0204011003011004024102030441000102140004079f01069e01800880088e085472616e7366657221af010306e301f601b303c60" +
		"366726f6d205b616464726573735d01746f205b616464726573735d02616d6f756e74205b75696e7436345d3f06c0061300031001004" +
		"0020001100302500403020501050803040500015a04020341000104100501400400055906040341000506140003088f01065ea005a00" +
		"5ae054d696e74216f0306f3018602616d6f756e74205b75696e7436345d0161646472205b616464726573735d3f06a00413000210010" +
		"059000001140002130003100201400300025903030141000203140003098f01065ea005a005ae054275726e216f0306f3018602616d6" +
		"f756e74205b75696e7436345d0161646472205b616464726573735d3f06f005130003100101400200011003005004030205010508030" +
		"40500015a040203410001041400031300025a0000031400022f03066d61705b616464726573735d75696e743634"
)

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
		Manifest: manifest,
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

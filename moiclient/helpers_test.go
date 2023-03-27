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
	manifest             = "0e4f065ede01302e312e302f064e504953410fcf07000ec314de149328ae28f32b8e2cd32fee2f83399e39" +
		"f3428e43b34ece4e83599e59b367ce67e37afe7ad39101ee9101e3a801fea801b3bf01cebf013f06505e73746174653f06ae01706" +
		"57273697374656e74af02000ed301ee01d303ee03d305ee05c309de092f06466e616d65737472696e67012f066673796d626f6c73" +
		"7472696e67022f0666737570706c7975696e743634033f06860162616c616e6365736d61705b616464726573735d75696e7436340" +
		"43f06a601616c6c6f77616e6365736d61705b616464726573735d6d61705b616464726573735d75696e743634014f067e9e01726f" +
		"7574696e651f00af010676fe01d00bd00bde0b536565646572216465706c6f796572ef01000ed301ee01d303ee03d305ee052f064" +
		"66e616d65737472696e67012f066673796d626f6c737472696e67022f0666737570706c7975696e743634032f0666736565646572" +
		"616464726573735f06f603f00310000014000010000114000110000214000213010310020341010200140103025f0680018e01636" +
		"f6e7374616e742f066675696e743634307830333061033f067076747970656465666d61705b616464726573735d75696e74363404" +
		"4f067e9e01726f7574696e651f00af010646d001de01d003de034e616d65696e766f6b61626c652f000e2f06466e616d657374726" +
		"96e674f0006e0013078313330303030313130303030054f067e9e01726f7574696e651f00af010666f001fe0190049e0453796d62" +
		"6f6c696e766f6b61626c652f000e2f066673796d626f6c737472696e674f0006e0013078313330303031313130303030064f067ea" +
		"e01726f7574696e651f0302bf0106860190029e02e004ee04446563696d616c73696e766f6b61626c652f000e3f06860164656369" +
		"6d616c7375696e7436344f0006c0023078303530313030303230363030313130303030074f067e9e01726f7574696e651f00bf010" +
		"6b601c002ce02e004ee04546f74616c537570706c79696e766f6b61626c652f000e2f0666737570706c7975696e7436344f0006e0" +
		"013078313330303032313130303030084f067e9e01726f7574696e651f00bf01069601ae02be04e006ee0642616c616e63654f666" +
		"96e766f6b61626c652f000e2f064661646472616464726573732f000e2f067662616c616e636575696e7436344f0006c003307831" +
		"33303030333130303130303430303230303031313130323030094f067e9e01726f7574696e651f00bf01069601ae02ae07800a8e0" +
		"a416c6c6f77616e6365696e766f6b61626c656f000ef3018e022f06566f776e657261646472657373012f06767370656e64657261" +
		"6464726573732f000e3f069601616c6c6f77616e636575696e7436344f0006a005307831333030303431303031303034303032303" +
		"0303131303033303134303034303230333131303430300a4f067ebe01726f7574696e652f000303bf010686019e02ee09a00bae0b" +
		"417070726f766521696e766f6b61626c65af01000ef3018e029304ae042f06566f776e657261646472657373012f06767370656e6" +
		"4657261646472657373022f0666616d6f756e7475696e7436342f000e2f06266f6b626f6f6c5f0696079007130004100100400200" +
		"0133030256030501040a0303040501040007043c020401100301100402410203044100010214000404000156001100000b4f067e9" +
		"e01726f7574696e651f00bf01069601ae029e09d00ade0a5472616e7366657221696e766f6b61626c65af01000ee301fe01b303ce" +
		"032f064666726f6d61646472657373012f0626746f61646472657373022f0666616d6f756e7475696e7436342f000e2f06266f6b6" +
		"26f6f6c5f06a608a00813000310010040020001100302500403020501050a03040504000111000000015a04020341000104100501" +
		"40040005590604034100050614000304000156001100000c4f067e9e01726f7574696e651f00af010656ee01be06f007fe074d696" +
		"e7421696e766f6b61626c656f000ef3018e022f0666616d6f756e7475696e743634012f064661646472616464726573732f000e2f" +
		"06266f6b626f6f6c4f0006e00a3078313330303032313030313030353930303030303131343030303231333030303331303032303" +
		"1343030333030303235393033303330313431303030323033313430303033303430303031353630303131303030300d4f067e9e01" +
		"726f7574696e651f00af010656ee01be06f007fe074275726e21696e766f6b61626c656f000ef3018e022f0666616d6f756e74756" +
		"96e743634012f064661646472616464726573732f000e2f06266f6b626f6f6c4f0006c00f30783133303030333130303130313430" +
		"303230303031313030333030353030343033303230353031303530413033303430353034303030313131303030303030303135413" +
		"034303230333431303030313034313430303033313330303032354130303030303331343030303230343030303135363030313130" +
		"303030"
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

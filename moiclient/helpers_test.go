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
	manifest              = "0x0e4f065ede01302e312e302f064e504953410fef030e8e13fe25ae29fe2cae359e3e8e48ae51de5d9e6ede84019e9b019eac014f0300065e73746174653f06ae0170657273697374656e749f010eee01fe03de05ae093f0306466e616d65737472696e673f0316760173796d626f6c737472696e673f03167602737570706c797536344f031696010362616c616e6365736d61705b616464726573735d7536344f0316b60104616c6c6f77616e6365736d61705b616464726573735d6d61705b616464726573735d7536345f031e36ae01011f03726f7574696e65af010676fe01e00aee0ab00f536565646572216465706c6f7965727f0eee01fe03de053f0306466e616d65737472696e673f0316760173796d626f6c737472696e673f03167602737570706c797536343f03167603736565646572616464726573735f06f603f003040000810000040001810001040002810002800103040203540102008101035f0310169e0102636f6e7374616e742f06367536343078303330615f031016860103747970656465666d61705b616464726573735d7536345f031e36ae01041f03726f7574696e65af010646d001de01de03f0044e616d65696e766f6b61626c651f0e3f0306466e616d65737472696e673f0666608000000500005f031e36ae01051f03726f7574696e65af010666f001fe019e04b00553796d626f6c696e766f6b61626c651f0e3f03066673796d626f6c737472696e673f0666608000010500005f031e36ae01061f03726f7574696e65bf0106860190029e02be04a006446563696d616c73696e766f6b61626c651f0e4f03068601646563696d616c737536345f06960190011100021000000500005f031e36ae01071f03726f7574696e65bf0106b601c002ce02be04d005546f74616c537570706c79696e766f6b61626c651f0e3f030666737570706c797536343f0666608000020500005f031e36ae01081f03726f7574696e65bf01069601ae02be04be06e00842616c616e63654f66696e766f6b61626c651f0e3f03064661646472616464726573731f0e3f03067662616c616e63657536345f06d601d001800003040100530200010502005f031e36ae01091f03726f7574696e65bf01069601ae02ae07de09f00c416c6c6f77616e6365696e766f6b61626c653f0e8e023f0306566f776e6572616464726573734f03168601017370656e646572616464726573731f0e4f03069601616c6c6f77616e63657536345f06c602c00280000404010053020001040301530402030504005f031e56ce010a2f030303726f7574696e65bf010686019e029e09de0ad012417070726f766521696e766f6b61626c655f0e8e02ce043f0306566f776e6572616464726573734f03168601017370656e646572616464726573733f03167602616d6f756e747536341f0e3f0306266f6b626f6f6c5f06a607a0078000040401005302000120030262030311040a0304031104002804042402040104030104040254020304540001028100042900016200000500005f031e36ae010b1f03726f7574696e65bf01069601ae02be08fe09f0125472616e7366657221696e766f6b61626c655f0efe01de033f03064666726f6d616464726573733f03163601746f616464726573733f03167602616d6f756e747536341f0e3f0306266f6b626f6f6c5f06a608a008800003040100530200010403024404030211050a030504290001050000000166040203540001040405015304000565060403540005068100032900016200000500005f031e36ae010c1f03726f7574696e65af010656ee01fe05be07c00d4d696e7421696e766f6b61626c653f0ede013f030666616d6f756e747536343f0316560161646472616464726573731f0e3f0306266f6b626f6f6c5f06b605b005800002040100650000018100028000030402015303000265030301540002038100032900016200000500005f031e36ae010d1f03726f7574696e65af010656ee01fe05be07e00f4275726e21696e766f6b61626c653f0ede013f030666616d6f756e747536343f0316560161646472616464726573731f0e3f0306266f6b626f6f6c5f06d607d007800003040101530200010403004404030211050a0305042900010500000001660402035400010481000380000266000003810002290001620000050000" //nolint
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

	receipt := RetryFetchReceipt(t, ctx, client, ixHash)
	require.Equal(t, common.ReceiptOk, receipt.Status)

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

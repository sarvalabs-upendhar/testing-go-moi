package moiclient

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/kramaid"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/jsonrpc/api"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

var LatestTesseractNumber = int64(-1)

// IxnTimeout is the max time to wait for an ixn to be completed
const IxnTimeout = 5 * time.Minute

var (
	genesisHeight        = int64(0)
	deployLogicHeight    = int64(1)
	executeLogicHeight   = int64(1)
	createAssetHeight    = int64(1)
	transferTokensHeight = int64(2)
	ixnPendingCount      = 10
)

// makeHTTPRequest takes method, args and makes an HTTP POST request to node specified by url constant
// returning a response with data, status, and error.
func makeHTTPRequest(t *testing.T, method string, args interface{}) *rpcargs.Response {
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

	httpResponse, err := http.Post(localURL, "application/json", bytes.NewBuffer(jsonData))
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

func createSendIXFromSendIXArgs(t *testing.T, sendIxArgs *common.SendIXArgs, mnemonic string) *rpcargs.SendIX {
	t.Helper()

	bz, err := polo.Polorize(sendIxArgs)
	require.NoError(t, err)

	sign, err := crypto.GetSignature(bz, mnemonic)
	require.NoError(t, err)

	return &rpcargs.SendIX{
		IXArgs:    hex.EncodeToString(bz),
		Signature: sign,
	}
}

// getIXArgsForLogicDeployment returns interaction args for logic deployment
func getIXArgsForLogicDeployment(t *testing.T, addr common.Address) *common.SendIXArgs {
	t.Helper()

	calldata := "0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206ffcd8ee6a29ec4" +
		"42dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49"

	manifest := "0x" + common.BytesToHex(tests.ReadManifest(t, "./../compute/manifests/erc20.json"))

	logicPayload := &common.LogicPayload{
		Manifest: hexutil.Bytes(common.Hex2Bytes(manifest)),
		Callsite: "Seeder!",
		Calldata: hexutil.Bytes(common.Hex2Bytes(calldata)),
	}

	payload, err := polo.Polorize(logicPayload)
	require.NoError(t, err)

	fuelPrice := new(big.Int).SetUint64(1)
	fuelLimit := new(big.Int).SetUint64(1000)

	ixArgs := &common.SendIXArgs{
		Type:      common.IxLogicDeploy,
		FuelPrice: fuelPrice,
		FuelLimit: fuelLimit,
		Sender:    addr,
		Payload:   payload,
	}

	return ixArgs
}

// getTesseract returns tesseract for the given senderAddr and height
func getTesseract(t *testing.T, client *Client, addr common.Address, height *int64) *rpcargs.RPCTesseract {
	t.Helper()

	args := &rpcargs.TesseractArgs{
		Address:          addr,
		WithInteractions: true,
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: height,
		},
	}

	ts, err := client.Tesseract(args)
	require.NoError(t, err)

	return ts
}

// getAssetID returns assetID for the given senderAddr and height
func getAssetID(t *testing.T, client *Client, addr common.Address, height *int64) common.AssetID {
	t.Helper()

	ts := getTesseract(t, client, addr, height)

	receiptArgs := &rpcargs.ReceiptArgs{
		Hash: ts.Ixns[0].Hash,
	}

	receipt, err := client.InteractionReceipt(receiptArgs)
	require.NoError(t, err)

	var assetReceipt common.AssetCreationReceipt

	err = json.Unmarshal(receipt.ExtraData, &assetReceipt)
	require.NoError(t, err)

	return assetReceipt.AssetID
}

// getLogicID returns logicID for the given senderAddr and height
func getLogicID(t *testing.T, client *Client, addr common.Address, height *int64) common.LogicID {
	t.Helper()

	ts := getTesseract(t, client, addr, height)

	receiptArgs := &rpcargs.ReceiptArgs{
		Hash: ts.Ixns[0].Hash,
	}

	receipt, err := client.InteractionReceipt(receiptArgs)
	require.NoError(t, err)

	var logicReceipt common.LogicDeployReceipt

	err = json.Unmarshal(receipt.ExtraData, &logicReceipt)
	require.NoError(t, err)

	return logicReceipt.LogicID
}

// retryFetchReceipt keeps trying to fetch receipt for given ixHash until it is timed out
// and also checks if moi client response matches with http response
// Use this to check if interaction is successful on the chain.
func retryFetchReceipt(t *testing.T, ctx context.Context, client *Client, ixHash common.Hash) *rpcargs.RPCReceipt {
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
			receipt, err := client.InteractionReceipt(receiptArgs)
			if err == nil {
				httpReceipt := httpInteractionReceipt(t, receiptArgs)
				checkForRPCReceipt(t, httpReceipt, receipt)

				return receipt
			}

			time.Sleep(time.Second)
		}
	}
}

// getLogicManifestByEncodingType returns the manifest according to the given encoding type POLO, JSON or YAML
func getLogicManifestByEncodingType(
	t *testing.T,
	res hexutil.Bytes,
	args *rpcargs.LogicManifestArgs,
) (hexutil.Bytes, error) {
	t.Helper()

	switch args.Encoding {
	case "POLO", "":
		return res, nil
	case "JSON":
		logicManifest := res.Bytes()

		depolorizedManifest, err := engineio.NewManifest(logicManifest, engineio.POLO)
		if err != nil {
			return nil, err
		}

		manifest, err := depolorizedManifest.Encode(engineio.JSON)
		if err != nil {
			return nil, err
		}

		return manifest, nil
	case "YAML":
		logicManifest := res.Bytes()

		depolorizedManifest, err := engineio.NewManifest(logicManifest, engineio.POLO)
		if err != nil {
			return nil, err
		}

		manifest, err := depolorizedManifest.Encode(engineio.YAML)
		if err != nil {
			return nil, err
		}

		return manifest, nil
	default:
		return nil, errors.New("invalid encoding type")
	}
}

// SetupAddrs sanitises the given addrs array and validates it
func SetupAddrs(addrs []common.Address) ([]common.Address, error) {
	if len(addrs) < 12 {
		return nil, errors.New("not sufficient genesis accounts to run moiclient tests")
	}

	temp := make([]common.Address, 0)

	for _, addr := range addrs {
		if addr == common.SargaAddress {
			continue
		}

		if common.ContainsAddress(common.GenesisLogicAddrs, addr) {
			continue
		}

		temp = append(temp, addr)
	}

	return temp, nil
}

// httpTesseract returns RPCTesseract based on the given arguments
func httpTesseract(t *testing.T, args interface{}) *rpcargs.RPCTesseract {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.Tesseract", args)

	var tess rpcargs.RPCTesseract

	err := json.Unmarshal(resp.Data, &tess)
	require.NoError(t, err)

	return &tess
}

// httpGetAssetInfoByAssetID returns asset description for the given assetID
func httpGetAssetInfoByAssetID(t *testing.T, args *rpcargs.GetAssetInfoArgs) *rpcargs.RPCAssetDescriptor {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.AssetInfoByAssetID", args)

	var assetInfo rpcargs.RPCAssetDescriptor

	err := json.Unmarshal(resp.Data, &assetInfo)
	require.NoError(t, err)

	return &assetInfo
}

// httpGetBalance returns the balance of assetID for given BalArgs
func httpGetBalance(t *testing.T, args *rpcargs.BalArgs) *hexutil.Big {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.Balance", args)

	var bal *hexutil.Big

	err := json.Unmarshal(resp.Data, &bal)
	require.NoError(t, err)

	return bal
}

// httpTDU retrieves the TDU of the queried address
func httpTDU(t *testing.T, args *rpcargs.QueryArgs) []rpcargs.TDU {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.TDU", args)

	var tdu []rpcargs.TDU

	err := json.Unmarshal(resp.Data, &tdu)
	require.NoError(t, err)

	return tdu
}

// httpGetContextInfo returns the context Info of the queried address.
func httpGetContextInfo(t *testing.T, args *rpcargs.ContextInfoArgs) *rpcargs.ContextResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.ContextInfo", args)

	var contextResp rpcargs.ContextResponse

	err := json.Unmarshal(resp.Data, &contextResp)
	require.NoError(t, err)

	return &contextResp
}

// httpInteractionReceipt returns the receipt of the interaction for given hash
func httpInteractionReceipt(t *testing.T, args *rpcargs.ReceiptArgs) *rpcargs.RPCReceipt {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.InteractionReceipt", args)

	var receipt rpcargs.RPCReceipt

	err := json.Unmarshal(resp.Data, &receipt)
	require.NoError(t, err)

	return &receipt
}

// httpInteractionByHash returns the interaction for given ix hash
func httpInteractionByHash(t *testing.T, args *rpcargs.InteractionByHashArgs) rpcargs.RPCInteraction {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.InteractionByHash", args)

	var ix rpcargs.RPCInteraction

	err := json.Unmarshal(resp.Data, &ix)
	require.NoError(t, err)

	return ix
}

// httpInteractionByTesseract returns the interaction for the given tesseract hash
func httpInteractionByTesseract(t *testing.T, args *rpcargs.InteractionByTesseract) rpcargs.RPCInteraction {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.InteractionByTesseract", args)

	var ix rpcargs.RPCInteraction

	err := json.Unmarshal(resp.Data, &ix)
	require.NoError(t, err)

	return ix
}

// httpInteractionCount returns the number of interactions sent for the given address
func httpInteractionCount(t *testing.T, args *rpcargs.InteractionCountArgs) *hexutil.Uint64 {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.InteractionCount", args)

	var count *hexutil.Uint64

	err := json.Unmarshal(resp.Data, &count)
	require.NoError(t, err)

	return count
}

// httpPendingInteractionCount returns the number of interactions sent for the given address.
func httpPendingInteractionCount(t *testing.T, args *rpcargs.InteractionCountArgs) *hexutil.Uint64 {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.PendingInteractionCount", args)

	var count *hexutil.Uint64

	err := json.Unmarshal(resp.Data, &count)
	require.NoError(t, err)

	return count
}

// httpStorage returns the data associated with the given httpStorage slot
func httpStorage(t *testing.T, args *rpcargs.GetLogicStorageArgs) hexutil.Bytes {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.Storage", args)

	var res hexutil.Bytes

	err := json.Unmarshal(resp.Data, &res)
	require.NoError(t, err)

	return res
}

// httpAccountState returns the account state of the given address
func httpAccountState(t *testing.T, args *rpcargs.GetAccountArgs) *rpcargs.RPCAccount {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.AccountState", args)

	var account rpcargs.RPCAccount

	err := json.Unmarshal(resp.Data, &account)
	require.NoError(t, err)

	return &account
}

// httpLogicIDs returns the logic IDs of the given address
func httpLogicIDs(t *testing.T, args *rpcargs.GetLogicIDArgs) []common.LogicID {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.LogicIDs", args)

	var logicIDs []common.LogicID

	err := json.Unmarshal(resp.Data, &logicIDs)
	require.NoError(t, err)

	return logicIDs
}

// httpLogicCall returns the LogicCallResult of the given address
func httpLogicCall(t *testing.T, args *rpcargs.LogicCallArgs) *rpcargs.LogicCallResult {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.LogicCall", args)

	var logicCall *rpcargs.LogicCallResult

	err := json.Unmarshal(resp.Data, &logicCall)
	require.NoError(t, err)

	return logicCall
}

// httpLogicManifest returns the manifest associated with the given logic id
func httpLogicManifest(t *testing.T, args *rpcargs.LogicManifestArgs) hexutil.Bytes {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.LogicManifest", args)

	var res hexutil.Bytes

	err := json.Unmarshal(resp.Data, &res)
	require.NoError(t, err)

	return res
}

// httpContent returns the interactions present in the given IxPool.
func httpContent(t *testing.T, args *rpcargs.ContentArgs) *api.ContentResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.Content", args)

	var contentRes api.ContentResponse

	err := json.Unmarshal(resp.Data, &contentRes)
	require.NoError(t, err)

	return &contentRes
}

// httpContentFrom returns the interactions present in IxPool for the queried address.
func httpContentFrom(t *testing.T, args *rpcargs.IxPoolArgs) *api.ContentFromResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.ContentFrom", args)

	var content api.ContentFromResponse

	err := json.Unmarshal(resp.Data, &content)
	require.NoError(t, err)

	return &content
}

// httpStatus returns the number of pending and queued interactions in the IxPool.
func httpStatus(t *testing.T, args *rpcargs.StatusArgs) *api.StatusResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.Status", args)

	var status api.StatusResponse

	err := json.Unmarshal(resp.Data, &status)
	require.NoError(t, err)

	return &status
}

// httpInspect returns the interactions present in the IxPool in a clear and easy-to-read format,
func httpInspect(t *testing.T, args *rpcargs.InspectArgs) *api.InspectResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.Inspect", args)

	var response api.InspectResponse

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return &response
}

// httpWaitTime returns the wait time for an account in IxPool, based on the queried address.
func httpWaitTime(t *testing.T, args *rpcargs.IxPoolArgs) *api.WaitTimeResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "ixpool.WaitTime", args)

	var waitTime api.WaitTimeResponse

	err := json.Unmarshal(resp.Data, &waitTime)
	require.NoError(t, err)

	return &waitTime
}

// httpPeers returns an array of Krama IDs connected to a client
func httpPeers(t *testing.T, args *rpcargs.NetArgs) *[]kramaid.KramaID {
	t.Helper()

	resp := makeHTTPRequest(t, "net.Peers", args)

	var response []kramaid.KramaID

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return &response
}

// httpPeers returns an array of Krama IDs connected to a client
func httpVersion(t *testing.T, args *rpcargs.NetArgs) string {
	t.Helper()

	resp := makeHTTPRequest(t, "net.Version", args)

	var response string

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return response
}

// httpInfo returns the kramaID of the node
func httpInfo(t *testing.T, args *rpcargs.NetArgs) *rpcargs.NodeInfoResponse {
	t.Helper()

	resp := makeHTTPRequest(t, "net.Info", args)

	var response rpcargs.NodeInfoResponse

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return &response
}

// httpDBGet returns raw value of the key stored in the database
func httpDBGet(t *testing.T, args *rpcargs.DebugArgs) string {
	t.Helper()

	resp := makeHTTPRequest(t, "debug.DBGet", args)

	var response string

	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	return response
}

// httpAccounts returns the addresses of all the accounts
func httpAccounts(t *testing.T, args *rpcargs.AccountArgs) []common.Address {
	t.Helper()

	resp := makeHTTPRequest(t, "debug.Accounts", args)

	var addrs []common.Address

	err := json.Unmarshal(resp.Data, &addrs)
	require.NoError(t, err)

	return addrs
}

// httpAccountMetaInfo returns the account meta info associated with the given address
func httpAccountMetaInfo(t *testing.T, args *rpcargs.GetAccountArgs) *rpcargs.RPCAccountMetaInfo {
	t.Helper()

	resp := makeHTTPRequest(t, "moi.AccountMetaInfo", args)

	var info rpcargs.RPCAccountMetaInfo

	err := json.Unmarshal(resp.Data, &info)
	require.NoError(t, err)

	return &info
}

func GetMnemonicFromAccounts(addr common.Address, accs []tests.AccountWithMnemonic) (tests.AccountWithMnemonic, bool) {
	for _, acc := range accs {
		if acc.Addr == addr {
			return acc, true
		}
	}

	return tests.AccountWithMnemonic{}, false
}

func checkForRPCReceipt(
	t *testing.T,
	expectedRPCReceipt *rpcargs.RPCReceipt,
	actualRPCReceipt *rpcargs.RPCReceipt,
) {
	t.Helper()

	require.Equal(t, expectedRPCReceipt.IxType, actualRPCReceipt.IxType)
	require.Equal(t, expectedRPCReceipt.IxHash, actualRPCReceipt.IxHash)
	require.Equal(t, expectedRPCReceipt.FuelUsed, actualRPCReceipt.FuelUsed)
	require.Equal(t, expectedRPCReceipt.ExtraData, actualRPCReceipt.ExtraData)
	require.Equal(t, expectedRPCReceipt.From, actualRPCReceipt.From)
	require.Equal(t, expectedRPCReceipt.To, actualRPCReceipt.To)
	require.Equal(t, expectedRPCReceipt.IXIndex, actualRPCReceipt.IXIndex)
	require.Equal(t, expectedRPCReceipt.Parts, actualRPCReceipt.Parts)

	require.True(t, reflect.DeepEqual(expectedRPCReceipt.Hashes, actualRPCReceipt.Hashes))
}

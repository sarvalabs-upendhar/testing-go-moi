package moiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"sync/atomic"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/jsonrpc/api"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/websocket"
)

type Client struct {
	url string
}

var ErrNoResult = errors.New("no result in JSON-RPC response")

// NewClient creates a new rpc client.
func NewClient(url string) (*Client, error) {
	return &Client{url}, nil
}

func (c *Client) URL() string {
	return c.url
}

type requestOp struct {
	ids  []json.RawMessage
	err  error
	resp chan *jsonrpcMessage // receives up to len(ids) responses
}

func (op *requestOp) wait(ctx context.Context) (*jsonrpcMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-op.resp:
		return resp, op.err
	}
}

func (c *Client) sendHTTP(ctx context.Context, op *requestOp, msg interface{}) error {
	respBody, err := c.doRequest(ctx, msg)
	if err != nil {
		return err
	}
	defer respBody.Close()

	var respmsg jsonrpcMessage
	if err := json.NewDecoder(respBody).Decode(&respmsg); err != nil {
		return err
	}

	op.resp <- &respmsg

	return nil
}

// Call performs a JSON-RPC call with the given arguments and unmarshals into
// result if no error occurred.
func (c *Client) Call(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	return c.CallContext(ctx, result, method, args...)
}

// CallContext performs a JSON-RPC call with the given arguments.
func (c *Client) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if result != nil && reflect.TypeOf(result).Kind() != reflect.Ptr {
		return fmt.Errorf("call result parameter must be pointer or nil interface: %v", result)
	}

	msg, err := c.newMessage(method, args...)
	if err != nil {
		return err
	}

	op := &requestOp{ids: []json.RawMessage{msg.ID}, resp: make(chan *jsonrpcMessage, 1)}
	err = c.sendHTTP(ctx, op, msg)

	if err != nil {
		return err
	}

	// dispatch has accepted the request and will close the channel when it quits.
	switch resp, err := op.wait(ctx); {
	case err != nil:
		return err
	case resp.Error != nil:
		return resp.Error
	case len(resp.Result) == 0:
		return ErrNoResult
	default:
		if result == nil {
			return nil
		}

		return json.Unmarshal(resp.Result, result)
	}
}

func (c *Client) doRequest(ctx context.Context, msg interface{}) (io.ReadCloser, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	_, err = url.Parse(c.url)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	// create http
	conn := new(http.Client)

	// do request
	resp, err := conn.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var buf bytes.Buffer

		var body []byte
		if _, err := buf.ReadFrom(resp.Body); err == nil {
			body = buf.Bytes()
		}

		return nil, HTTPError{
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       body,
		}
	}

	return resp.Body, nil
}

func (c *Client) newMessage(method string, paramsIn ...interface{}) (*jsonrpcMessage, error) {
	msg := &jsonrpcMessage{Version: vsn, ID: c.nextID(), Method: method}

	if paramsIn != nil { // prevent sending "params":null
		var err error
		if msg.Params, err = json.Marshal(paramsIn); err != nil {
			return nil, err
		}
	}

	return msg, nil
}

func (c *Client) nextID() json.RawMessage {
	randID := rand.Uint32()
	id := atomic.AddUint32(&randID, 1)

	return strconv.AppendUint(nil, uint64(id), 10)
}

// Registry returns the asset registry info for the given address and tesseract options
func (c *Client) Registry(ctx context.Context, args *rpcargs.QueryArgs) ([]rpcargs.RPCRegistry, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.Registry", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var entries []rpcargs.RPCRegistry

	err = json.Unmarshal(resp.Data, &entries)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// Tesseract returns RPCTesseract based on the given arguments
func (c *Client) Tesseract(ctx context.Context, args *rpcargs.TesseractArgs) (*rpcargs.RPCTesseract, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.Tesseract", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var tess rpcargs.RPCTesseract

	err = json.Unmarshal(resp.Data, &tess)
	if err != nil {
		return nil, err
	}

	return &tess, nil
}

// AssetInfoByAssetID returns asset description for the given assetID
func (c *Client) AssetInfoByAssetID(
	ctx context.Context,
	args *rpcargs.GetAssetInfoArgs,
) (*rpcargs.RPCAssetDescriptor, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.AssetInfoByAssetID", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var assetInfo rpcargs.RPCAssetDescriptor
	err = json.Unmarshal(resp.Data, &assetInfo)

	if err != nil {
		return nil, err
	}

	return &assetInfo, nil
}

// Balance returns the balance of assetID for given api.BalArgs
func (c *Client) Balance(ctx context.Context, args *rpcargs.BalArgs) (*hexutil.Big, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.Balance", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var bal *hexutil.Big

	err = json.Unmarshal(resp.Data, &bal)

	if err != nil {
		return nil, errors.New("invalid response type")
	}

	return bal, nil
}

// TDU retrieves the TDU of the queried address
func (c *Client) TDU(ctx context.Context, args *rpcargs.QueryArgs) ([]rpcargs.TDU, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.TDU", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var tdu []rpcargs.TDU

	err = json.Unmarshal(resp.Data, &tdu)

	if err != nil {
		return nil, err
	}

	return tdu, nil
}

// ContextInfo returns the context Info of the queried address.
func (c *Client) ContextInfo(ctx context.Context, args *rpcargs.ContextInfoArgs) (*rpcargs.ContextResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.ContextInfo", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var contextResp rpcargs.ContextResponse

	err = json.Unmarshal(resp.Data, &contextResp)
	if err != nil {
		return nil, err
	}

	return &contextResp, nil
}

// InteractionByTesseract returns the interaction for the given tesseract hash
func (c *Client) InteractionByTesseract(
	ctx context.Context,
	args *rpcargs.InteractionByTesseract,
) (*rpcargs.RPCInteraction, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.InteractionByTesseract", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var ix rpcargs.RPCInteraction

	err = json.Unmarshal(resp.Data, &ix)
	if err != nil {
		return nil, err
	}

	return &ix, nil
}

// InteractionByHash returns the interaction for given ix hash
func (c *Client) InteractionByHash(
	ctx context.Context,
	args *rpcargs.InteractionByHashArgs,
) (*rpcargs.RPCInteraction, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.InteractionByHash", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var ix rpcargs.RPCInteraction

	err = json.Unmarshal(resp.Data, &ix)
	if err != nil {
		return nil, err
	}

	return &ix, nil
}

// InteractionReceipt returns the receipt of the interaction for given hash
func (c *Client) InteractionReceipt(ctx context.Context, args *rpcargs.ReceiptArgs) (*rpcargs.RPCReceipt, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.InteractionReceipt", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var receipt rpcargs.RPCReceipt

	err = json.Unmarshal(resp.Data, &receipt)
	if err != nil {
		return nil, err
	}

	return &receipt, nil
}

// InteractionCount returns the number of interactions sent for the given address
func (c *Client) InteractionCount(ctx context.Context, args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.InteractionCount", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var count *hexutil.Uint64

	err = json.Unmarshal(resp.Data, &count)
	if err != nil {
		return nil, err
	}

	return count, nil
}

// PendingInteractionCount returns the number of interactions sent for the given address.
func (c *Client) PendingInteractionCount(
	ctx context.Context,
	args *rpcargs.InteractionCountArgs,
) (*hexutil.Uint64, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.PendingInteractionCount", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var count *hexutil.Uint64

	err = json.Unmarshal(resp.Data, &count)
	if err != nil {
		return nil, err
	}

	return count, nil
}

// LogicStorage returns the data associated with the given storage slot
func (c *Client) LogicStorage(ctx context.Context, args *rpcargs.GetLogicStorageArgs) (hexutil.Bytes, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.LogicStorage", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var res hexutil.Bytes

	err = json.Unmarshal(resp.Data, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// AccountState returns the account state of the given address
func (c *Client) AccountState(ctx context.Context, args *rpcargs.GetAccountArgs) (*rpcargs.RPCAccount, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.AccountState", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var account rpcargs.RPCAccount

	err = json.Unmarshal(resp.Data, &account)
	if err != nil {
		return nil, err
	}

	return &account, nil
}

// LogicIDs returns the logic IDs of the given address
func (c *Client) LogicIDs(ctx context.Context, args *rpcargs.GetLogicIDArgs) ([]identifiers.LogicID, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.LogicIDs", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var logicIDs []identifiers.LogicID

	err = json.Unmarshal(resp.Data, &logicIDs)
	if err != nil {
		return nil, err
	}

	return logicIDs, nil
}

// LogicManifest returns the manifest associated with the given logic id
func (c *Client) LogicManifest(ctx context.Context, args *rpcargs.LogicManifestArgs) (hexutil.Bytes, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.LogicManifest", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var res hexutil.Bytes

	err = json.Unmarshal(resp.Data, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// SendInteractions sends given Interactions
func (c *Client) SendInteractions(ctx context.Context, args *rpcargs.SendIX) (common.Hash, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.SendInteractions", args)
	if err != nil {
		return common.NilHash, err
	}

	if resp.Error != nil {
		return common.NilHash, resp.Error
	}

	var res common.Hash

	err = json.Unmarshal(resp.Data, &res)
	if err != nil {
		return common.NilHash, err
	}

	return res, nil
}

// AccountMetaInfo returns the account meta info associated with the given address
func (c *Client) AccountMetaInfo(
	ctx context.Context,
	args *rpcargs.GetAccountArgs,
) (*rpcargs.RPCAccountMetaInfo, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.AccountMetaInfo", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var accMetaInfo rpcargs.RPCAccountMetaInfo

	err = json.Unmarshal(resp.Data, &accMetaInfo)
	if err != nil {
		return nil, err
	}

	return &accMetaInfo, nil
}

// FuelEstimate returns an estimate of the fuel that is required for executing an interaction
func (c *Client) FuelEstimate(ctx context.Context, args *rpcargs.CallArgs) (*hexutil.Big, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.FuelEstimate", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var fuelUsed *hexutil.Big

	err = json.Unmarshal(resp.Data, &fuelUsed)
	if err != nil {
		return nil, err
	}

	return fuelUsed, nil
}

// Syncing returns the sync status of an account if address is given else returns the node sync status
func (c *Client) Syncing(ctx context.Context, args *rpcargs.SyncStatusRequest) (*rpcargs.SyncStatusResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.Syncing", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var syncStatus *rpcargs.SyncStatusResponse

	err = json.Unmarshal(resp.Data, &syncStatus)
	if err != nil {
		return nil, err
	}

	return syncStatus, nil
}

// InteractionCall returns stateless version of an interaction submit
func (c *Client) InteractionCall(ctx context.Context, args *rpcargs.CallArgs) (*rpcargs.RPCReceipt, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.Call", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var receipt rpcargs.RPCReceipt

	err = json.Unmarshal(resp.Data, &receipt)
	if err != nil {
		return nil, err
	}

	return &receipt, nil
}

// NewTesseractFilter subscribes to all new tesseract events
func (c *Client) NewTesseractFilter(ctx context.Context,
	args *rpcargs.TesseractFilterArgs,
) (*rpcargs.FilterResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.NewTesseractFilter", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var filterResponse rpcargs.FilterResponse

	err = json.Unmarshal(resp.Data, &filterResponse)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// NewTesseractsByAccountFilter subscribes to all new tesseract events for a given account
func (c *Client) NewTesseractsByAccountFilter(ctx context.Context,
	args *rpcargs.TesseractByAccountFilterArgs,
) (*rpcargs.FilterResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.NewTesseractsByAccountFilter", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var filterResponse rpcargs.FilterResponse

	err = json.Unmarshal(resp.Data, &filterResponse)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// NewLogFilter creates a log filter based on LogQuery.
func (c *Client) NewLogFilter(ctx context.Context,
	args *websocket.LogQuery,
) (*rpcargs.FilterResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.NewLogFilter", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var filterResponse rpcargs.FilterResponse

	err = json.Unmarshal(resp.Data, &filterResponse)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// PendingIxnsFilter subscribes to all new pending interactions.
func (c *Client) PendingIxnsFilter(ctx context.Context,
	args *rpcargs.PendingIxnsFilterArgs,
) (*rpcargs.FilterResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.PendingIxnsFilter", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var filterResponse rpcargs.FilterResponse

	err = json.Unmarshal(resp.Data, &filterResponse)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// RemoveFilter uninstalls filter for given filter ID.
func (c *Client) RemoveFilter(
	ctx context.Context,
	args *rpcargs.FilterArgs,
) (*rpcargs.FilterUninstallResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.RemoveFilter", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var filterUninstallResponse rpcargs.FilterUninstallResponse

	err = json.Unmarshal(resp.Data, &filterUninstallResponse)
	if err != nil {
		return nil, err
	}

	return &filterUninstallResponse, nil
}

// GetLogs returns an array of logs matching the LogQuery
func (c *Client) GetLogs(ctx context.Context, args *rpcargs.FilterQueryArgs) ([]*rpcargs.RPCLog, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.GetLogs", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var logs []*rpcargs.RPCLog

	err = json.Unmarshal(resp.Data, &logs)
	if err != nil {
		return nil, err
	}

	return logs, nil
}

// GetFilterChanges is a polling method for a filter using a filter ID,
// which returns an array of events which occurred since last poll.
func (c *Client) GetFilterChanges(
	ctx context.Context,
	args *rpcargs.FilterArgs,
	s rpcargs.SubscriptionType,
) (interface{}, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "moi.GetFilterChanges", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	switch s {
	case rpcargs.NewTesseract:
		var ts []*rpcargs.RPCTesseract

		err = json.Unmarshal(resp.Data, &ts)
		if err != nil {
			return nil, err
		}

		return ts, nil
	case rpcargs.NewTesseractsByAccount:
		var ts []*rpcargs.RPCTesseract

		err = json.Unmarshal(resp.Data, &ts)
		if err != nil {
			return nil, err
		}

		return ts, nil
	case rpcargs.NewLogsByFilter:
		var logs []*rpcargs.RPCLog

		err = json.Unmarshal(resp.Data, &logs)
		if err != nil {
			return nil, err
		}

		return logs, nil
	case rpcargs.PendingIxns:
		var ixHashes []*common.Hash

		err = json.Unmarshal(resp.Data, &ixHashes)
		if err != nil {
			return nil, err
		}

		return ixHashes, nil
	default:
		return nil, errors.New("unknown subscription type")
	}
}

// Content returns the interactions present in the given IxPool.
func (c *Client) Content(ctx context.Context, args *rpcargs.ContentArgs) (*api.ContentResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "ixpool.Content", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var content api.ContentResponse

	err = json.Unmarshal(resp.Data, &content)
	if err != nil {
		return nil, err
	}

	return &content, nil
}

// ContentFrom returns the interactions present in IxPool for the queried address.
func (c *Client) ContentFrom(ctx context.Context, args *rpcargs.IxPoolArgs) (*api.ContentFromResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "ixpool.ContentFrom", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var content api.ContentFromResponse

	err = json.Unmarshal(resp.Data, &content)
	if err != nil {
		return nil, err
	}

	return &content, nil
}

// Status returns the number of pending and queued interactions in the IxPool.
func (c *Client) Status(ctx context.Context, args *rpcargs.StatusArgs) (*api.StatusResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "ixpool.Status", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var status api.StatusResponse

	err = json.Unmarshal(resp.Data, &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// Inspect returns the interactions present in the IxPool in a clear and easy-to-read format,
func (c *Client) Inspect(ctx context.Context, args *rpcargs.InspectArgs) (*api.InspectResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "ixpool.Inspect", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var response api.InspectResponse

	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// WaitTime returns the wait time for an account in IxPool, based on the queried address.
func (c *Client) WaitTime(ctx context.Context, args *rpcargs.IxPoolArgs) (*api.WaitTimeResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "ixpool.WaitTime", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var time api.WaitTimeResponse

	err = json.Unmarshal(resp.Data, &time)
	if err != nil {
		return nil, err
	}

	return &time, nil
}

// Peers returns an array of Krama IDs connected to a client
func (c *Client) Peers(ctx context.Context, args *rpcargs.NetArgs) ([]kramaid.KramaID, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "net.Peers", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var response []kramaid.KramaID

	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Version returns the protocol version
func (c *Client) Version(ctx context.Context, args *rpcargs.NetArgs) (string, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "net.Version", args)
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", resp.Error
	}

	var response string

	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return "", err
	}

	return response, nil
}

// Info returns the kramaID of the node
func (c *Client) Info(ctx context.Context, args *rpcargs.NetArgs) (*rpcargs.NodeInfoResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "net.Info", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var response rpcargs.NodeInfoResponse

	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// DBGet returns raw value of the key stored in the database
func (c *Client) DBGet(ctx context.Context, args *rpcargs.DebugArgs) (string, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "debug.DBGet", args)
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", resp.Error
	}

	var response string

	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return "", err
	}

	return response, nil
}

// NodeMetaInfo returns the metadata of nodes stored in the database
func (c *Client) NodeMetaInfo(
	ctx context.Context,
	args *rpcargs.NodeMetaInfoArgs,
) (map[string]rpcargs.NodeMetaInfoResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "debug.NodeMetaInfo", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var response map[string]rpcargs.NodeMetaInfoResponse

	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Accounts returns the address of all the accounts
func (c *Client) Accounts(ctx context.Context) ([]identifiers.Address, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "debug.Accounts", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var addrs []identifiers.Address

	err = json.Unmarshal(resp.Data, &addrs)
	if err != nil {
		return nil, err
	}

	return addrs, nil
}

// Connections returns the total connections of the node
func (c *Client) Connections(ctx context.Context) (*rpcargs.ConnectionsResponse, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "debug.Connections", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var connResp rpcargs.ConnectionsResponse

	err = json.Unmarshal(resp.Data, &connResp)
	if err != nil {
		return nil, err
	}

	return &connResp, nil
}

// GetSyncJob returns the sync job meta info for a given address
func (c *Client) GetSyncJob(ctx context.Context, args *rpcargs.SyncJobRequest) (*rpcargs.SyncJobInfo, error) {
	var resp rpcargs.Response

	err := c.Call(ctx, &resp, "debug.GetSyncJob", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var syncJob rpcargs.SyncJobInfo

	err = json.Unmarshal(resp.Data, &syncJob)
	if err != nil {
		return nil, err
	}

	return &syncJob, nil
}

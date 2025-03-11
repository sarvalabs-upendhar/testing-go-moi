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

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/jsonrpc"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
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

	op := &requestOp{
		ids:  []json.RawMessage{msg.ID},
		resp: make(chan *jsonrpcMessage, 1),
	}

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

// Deeds returns the asset deeds info for the given address and tesseract options
func (c *Client) Deeds(ctx context.Context, args *rpcargs.QueryArgs) ([]rpcargs.RPCDeeds, error) {
	var entries []rpcargs.RPCDeeds

	err := c.Call(ctx, &entries, "moi.Deeds", args)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// Tesseract returns RPCTesseract based on the given arguments
func (c *Client) Tesseract(ctx context.Context, args *rpcargs.TesseractArgs) (*rpcargs.RPCTesseract, error) {
	var tess rpcargs.RPCTesseract

	err := c.Call(ctx, &tess, "moi.Tesseract", args)
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
	var assetInfo rpcargs.RPCAssetDescriptor

	err := c.Call(ctx, &assetInfo, "moi.AssetInfoByAssetID", args)
	if err != nil {
		return nil, err
	}

	return &assetInfo, nil
}

// Balance returns the balance of assetID for given api.BalArgs
func (c *Client) Balance(ctx context.Context, args *rpcargs.BalArgs) (*hexutil.Big, error) {
	var bal *hexutil.Big

	err := c.Call(ctx, &bal, "moi.Balance", args)
	if err != nil {
		return nil, err
	}

	return bal, nil
}

// TDU retrieves the TDU of the queried address
func (c *Client) TDU(ctx context.Context, args *rpcargs.QueryArgs) ([]rpcargs.TDU, error) {
	var tdu []rpcargs.TDU

	err := c.Call(ctx, &tdu, "moi.TDU", args)
	if err != nil {
		return nil, err
	}

	return tdu, nil
}

// ContextInfo returns the context Info of the queried address.
func (c *Client) ContextInfo(ctx context.Context, args *rpcargs.ContextInfoArgs) (*rpcargs.ContextResponse, error) {
	var contextResp rpcargs.ContextResponse

	err := c.Call(ctx, &contextResp, "moi.ContextInfo", args)
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
	var ix rpcargs.RPCInteraction

	err := c.Call(ctx, &ix, "moi.InteractionByTesseract", args)
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
	var ix rpcargs.RPCInteraction

	err := c.Call(ctx, &ix, "moi.InteractionByHash", args)
	if err != nil {
		return nil, err
	}

	return &ix, nil
}

// InteractionReceipt returns the receipt of the interaction for given hash
func (c *Client) InteractionReceipt(ctx context.Context, args *rpcargs.ReceiptArgs) (*rpcargs.RPCReceipt, error) {
	var receipt rpcargs.RPCReceipt

	err := c.Call(ctx, &receipt, "moi.InteractionReceipt", args)
	if err != nil {
		return nil, err
	}

	return &receipt, nil
}

// SubAccountCount returns the number of sub accounts of an identifier
func (c *Client) SubAccountCount(ctx context.Context, args *rpcargs.SubAccountCountArgs) (*hexutil.Uint64, error) {
	var count *hexutil.Uint64

	err := c.Call(ctx, &count, "moi.SubAccountCount", args)
	if err != nil {
		return nil, err
	}

	return count, nil
}

// InteractionCount returns the number of interactions sent for the given address
func (c *Client) InteractionCount(ctx context.Context, args *rpcargs.InteractionCountArgs) (*hexutil.Uint64, error) {
	var count *hexutil.Uint64

	err := c.Call(ctx, &count, "moi.InteractionCount", args)
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
	var count *hexutil.Uint64

	err := c.Call(ctx, &count, "moi.PendingInteractionCount", args)
	if err != nil {
		return nil, err
	}

	return count, nil
}

// LogicEnlisted returns whether the account is enlisted with the logic
func (c *Client) LogicEnlisted(ctx context.Context, args *rpcargs.LogicEnlistedArgs) (bool, error) {
	var res bool

	err := c.Call(ctx, &res, "moi.LogicEnlisted", args)
	if err != nil {
		return false, err
	}

	return res, nil
}

// LogicStorage returns the data associated with the given storage slot
func (c *Client) LogicStorage(ctx context.Context, args *rpcargs.GetLogicStorageArgs) (hexutil.Bytes, error) {
	var res hexutil.Bytes

	err := c.Call(ctx, &res, "moi.LogicStorage", args)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// AccountState returns the account state of the given address
func (c *Client) AccountState(ctx context.Context, args *rpcargs.GetAccountArgs) (*rpcargs.RPCAccount, error) {
	var account rpcargs.RPCAccount

	err := c.Call(ctx, &account, "moi.AccountState", args)
	if err != nil {
		return nil, err
	}

	return &account, nil
}

// AccountKeys returns the account state of the given address
func (c *Client) AccountKeys(ctx context.Context, args *rpcargs.GetAccountKeysArgs) ([]rpcargs.RPCAccountKey, error) {
	var accountKeys []rpcargs.RPCAccountKey

	err := c.Call(ctx, &accountKeys, "moi.AccountKeys", args)
	if err != nil {
		return nil, err
	}

	return accountKeys, nil
}

func (c *Client) Mandates(
	ctx context.Context,
	args *rpcargs.GetAssetMandateOrLockupArgs,
) ([]rpcargs.RPCMandateOrLockup, error) {
	var mandates []rpcargs.RPCMandateOrLockup

	err := c.Call(ctx, &mandates, "moi.Mandates", args)
	if err != nil {
		return nil, err
	}

	return mandates, nil
}

func (c *Client) Lockups(
	ctx context.Context,
	args *rpcargs.GetAssetMandateOrLockupArgs,
) ([]rpcargs.RPCMandateOrLockup, error) {
	var lockups []rpcargs.RPCMandateOrLockup

	err := c.Call(ctx, &lockups, "moi.Lockups", args)
	if err != nil {
		return nil, err
	}

	return lockups, nil
}

// LogicIDs returns the logic IDs of the given address
func (c *Client) LogicIDs(ctx context.Context, args *rpcargs.GetLogicIDArgs) ([]identifiers.LogicID, error) {
	var logicIDs []identifiers.LogicID

	err := c.Call(ctx, &logicIDs, "moi.LogicIDs", args)
	if err != nil {
		return nil, err
	}

	return logicIDs, nil
}

// LogicManifest returns the manifest associated with the given logic id
func (c *Client) LogicManifest(ctx context.Context, args *rpcargs.LogicManifestArgs) (hexutil.Bytes, error) {
	var res hexutil.Bytes

	err := c.Call(ctx, &res, "moi.LogicManifest", args)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// SendInteractions sends given Interactions
func (c *Client) SendInteractions(ctx context.Context, args *rpcargs.SendIX) (common.Hash, error) {
	var res common.Hash

	err := c.Call(ctx, &res, "moi.SendInteractions", args)
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
	var accMetaInfo rpcargs.RPCAccountMetaInfo

	err := c.Call(ctx, &accMetaInfo, "moi.AccountMetaInfo", args)
	if err != nil {
		return nil, err
	}

	return &accMetaInfo, nil
}

// FuelEstimate returns an estimate of the fuel that is required for executing an interaction
func (c *Client) FuelEstimate(ctx context.Context, args *rpcargs.CallArgs) (*hexutil.Big, error) {
	var fuelUsed *hexutil.Big

	err := c.Call(ctx, &fuelUsed, "moi.FuelEstimate", args)
	if err != nil {
		return nil, err
	}

	return fuelUsed, nil
}

// Syncing returns the sync status of an account if address is given else returns the node sync status
func (c *Client) Syncing(ctx context.Context, args *rpcargs.SyncStatusRequest) (*rpcargs.SyncStatusResponse, error) {
	var syncStatus *rpcargs.SyncStatusResponse

	err := c.Call(ctx, &syncStatus, "moi.Syncing", args)
	if err != nil {
		return nil, err
	}

	return syncStatus, nil
}

// InteractionCall returns stateless version of an interaction submit
func (c *Client) InteractionCall(ctx context.Context, args *rpcargs.CallArgs) (*rpcargs.RPCReceipt, error) {
	var receipt rpcargs.RPCReceipt

	err := c.Call(ctx, &receipt, "moi.Call", args)
	if err != nil {
		return nil, err
	}

	return &receipt, nil
}

// NewTesseractFilter subscribes to all new tesseract events
func (c *Client) NewTesseractFilter(ctx context.Context,
	args *rpcargs.TesseractFilterArgs,
) (*rpcargs.FilterResponse, error) {
	var filterResponse rpcargs.FilterResponse

	err := c.Call(ctx, &filterResponse, "moi.NewTesseractFilter", args)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// NewTesseractsByAccountFilter subscribes to all new tesseract events for a given account
func (c *Client) NewTesseractsByAccountFilter(ctx context.Context,
	args *rpcargs.TesseractByAccountFilterArgs,
) (*rpcargs.FilterResponse, error) {
	var filterResponse rpcargs.FilterResponse

	err := c.Call(ctx, &filterResponse, "moi.NewTesseractsByAccountFilter", args)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// NewLogFilter creates a log filter based on LogQuery.
func (c *Client) NewLogFilter(ctx context.Context,
	args *jsonrpc.LogQuery,
) (*rpcargs.FilterResponse, error) {
	var filterResponse rpcargs.FilterResponse

	err := c.Call(ctx, &filterResponse, "moi.NewLogFilter", args)
	if err != nil {
		return nil, err
	}

	return &filterResponse, nil
}

// PendingIxnsFilter subscribes to all new pending interactions.
func (c *Client) PendingIxnsFilter(ctx context.Context,
	args *rpcargs.PendingIxnsFilterArgs,
) (*rpcargs.FilterResponse, error) {
	var filterResponse rpcargs.FilterResponse

	err := c.Call(ctx, &filterResponse, "moi.PendingIxnsFilter", args)
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
	var filterUninstallResponse rpcargs.FilterUninstallResponse

	err := c.Call(ctx, &filterUninstallResponse, "moi.RemoveFilter", args)
	if err != nil {
		return nil, err
	}

	return &filterUninstallResponse, nil
}

// GetLogs returns an array of logs matching the LogQuery
func (c *Client) GetLogs(ctx context.Context, args *rpcargs.FilterQueryArgs) ([]*rpcargs.RPCLog, error) {
	var logs []*rpcargs.RPCLog

	err := c.Call(ctx, &logs, "moi.GetLogs", args)
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
	switch s {
	case rpcargs.NewTesseract:
		var ts []*rpcargs.RPCTesseract

		err := c.Call(ctx, &ts, "moi.GetFilterChanges", args)
		if err != nil {
			return nil, err
		}

		return ts, nil
	case rpcargs.NewTesseractsByAccount:
		var ts []*rpcargs.RPCTesseract

		err := c.Call(ctx, &ts, "moi.GetFilterChanges", args)
		if err != nil {
			return nil, err
		}

		return ts, nil
	case rpcargs.NewLogsByFilter:
		var logs []*rpcargs.RPCLog

		err := c.Call(ctx, &logs, "moi.GetFilterChanges", args)
		if err != nil {
			return nil, err
		}

		return logs, nil
	case rpcargs.PendingIxns:
		var ixHashes []*common.Hash

		err := c.Call(ctx, &ixHashes, "moi.GetFilterChanges", args)
		if err != nil {
			return nil, err
		}

		return ixHashes, nil
	default:
		return nil, errors.New("unknown subscription type")
	}
}

// Content returns the interactions present in the given IxPool.
func (c *Client) Content(ctx context.Context, args *rpcargs.ContentArgs) (*rpcargs.ContentResponse, error) {
	var content rpcargs.ContentResponse

	err := c.Call(ctx, &content, "ixpool.Content", args)
	if err != nil {
		return nil, err
	}

	return &content, nil
}

// ContentFrom returns the interactions present in IxPool for the queried address.
func (c *Client) ContentFrom(ctx context.Context, args *rpcargs.IxPoolArgs) (*rpcargs.ContentFromResponse, error) {
	var content rpcargs.ContentFromResponse

	err := c.Call(ctx, &content, "ixpool.ContentFrom", args)
	if err != nil {
		return nil, err
	}

	return &content, nil
}

// Status returns the number of pending and queued interactions in the IxPool.
func (c *Client) Status(ctx context.Context, args *rpcargs.StatusArgs) (*rpcargs.StatusResponse, error) {
	var status rpcargs.StatusResponse

	err := c.Call(ctx, &status, "ixpool.Status", args)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// Inspect returns the interactions present in the IxPool in a clear and easy-to-read format,
func (c *Client) Inspect(ctx context.Context, args *rpcargs.InspectArgs) (*rpcargs.InspectResponse, error) {
	var response rpcargs.InspectResponse

	err := c.Call(ctx, &response, "ixpool.Inspect", args)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// WaitTime returns the wait time for an account in IxPool, based on the queried address.
func (c *Client) WaitTime(ctx context.Context, args *rpcargs.IxPoolArgs) (*rpcargs.WaitTimeResponse, error) {
	var time rpcargs.WaitTimeResponse

	err := c.Call(ctx, &time, "ixpool.WaitTime", args)
	if err != nil {
		return nil, err
	}

	return &time, nil
}

// Peers returns an array of Krama IDs connected to a client
func (c *Client) Peers(ctx context.Context, args *rpcargs.NetArgs) ([]identifiers.KramaID, error) {
	var response []identifiers.KramaID

	err := c.Call(ctx, &response, "net.Peers", args)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Version returns the protocol version
func (c *Client) Version(ctx context.Context, args *rpcargs.NetArgs) (string, error) {
	var response string

	err := c.Call(ctx, &response, "net.Version", args)
	if err != nil {
		return "", err
	}

	return response, nil
}

// Info returns the kramaID of the node
func (c *Client) Info(ctx context.Context, args *rpcargs.NetArgs) (*rpcargs.NodeInfoResponse, error) {
	var response rpcargs.NodeInfoResponse

	err := c.Call(ctx, &response, "net.Info", args)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// DBGet returns raw value of the key stored in the database
func (c *Client) DBGet(ctx context.Context, args *rpcargs.DebugArgs) (string, error) {
	var response string

	err := c.Call(ctx, &response, "debug.DBGet", args)
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
	var response map[string]rpcargs.NodeMetaInfoResponse

	err := c.Call(ctx, &response, "debug.NodeMetaInfo", args)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Accounts returns the address of all the accounts
func (c *Client) Accounts(ctx context.Context) ([]identifiers.Identifier, error) {
	var ids []identifiers.Identifier

	err := c.Call(ctx, &ids, "debug.Accounts", nil)
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// Connections returns the total connections of the node
func (c *Client) Connections(ctx context.Context) (*rpcargs.ConnectionsResponse, error) {
	var connResp rpcargs.ConnectionsResponse

	err := c.Call(ctx, &connResp, "debug.Connections", nil)
	if err != nil {
		return nil, err
	}

	return &connResp, nil
}

// SyncJob returns the sync job meta info for a given address
func (c *Client) SyncJob(ctx context.Context, args *rpcargs.SyncJobRequest) (*rpcargs.SyncJobInfo, error) {
	var syncJob rpcargs.SyncJobInfo

	err := c.Call(ctx, &syncJob, "debug.SyncJob", args)
	if err != nil {
		return nil, err
	}

	return &syncJob, nil
}

// PeersScore returns the score of all connected peers
func (c *Client) PeersScore(ctx context.Context, args *rpcargs.PeerScoreRequest) ([]rpcargs.RPCPeerScore, error) {
	var peersScore []rpcargs.RPCPeerScore

	err := c.Call(ctx, &peersScore, "debug.PeersScore", args)
	if err != nil {
		return nil, err
	}

	return peersScore, nil
}

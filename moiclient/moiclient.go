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
	"reflect"
	"strconv"
	"sync/atomic"

	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/api"
	poornarpc "github.com/sarvalabs/moichain/poorna/rpc"
	"github.com/sarvalabs/moichain/types"
)

type Client struct {
	url    string
	client *poornarpc.Service
}

var ErrNoResult = errors.New("no result in JSON-RPC response")

// NewClient creates a new rpc client.
func NewClient(url string) (*Client, error) {
	serv := poornarpc.NewRPCService()

	return &Client{url, serv}, nil
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
func (c *Client) Call(result interface{}, method string, args ...interface{}) error {
	ctx := context.Background()

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return nil, err
	}

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

// Tesseract returns tesseract based on the given arguments
func (c *Client) Tesseract(args *api.TesseractArgs) (*types.Tesseract, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.Tesseract", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var tess types.Tesseract

	err = json.Unmarshal(resp.Data, &tess)
	if err != nil {
		return nil, err
	}

	return &tess, nil
}

// GetAssetInfoByAssetID returns asset description for the given assetID
func (c *Client) GetAssetInfoByAssetID(assetID string) (*types.AssetDescriptor, error) {
	args := &api.AssetDescriptorArgs{
		AssetID: assetID,
	}

	var resp api.Response

	err := c.Call(&resp, "moi.AssetInfoByAssetID", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var assetInfo types.AssetDescriptor
	err = json.Unmarshal(resp.Data, &assetInfo)

	if err != nil {
		return nil, err
	}

	return &assetInfo, nil
}

// GetBalance returns the balance of assetID for given api.BalArgs
func (c *Client) GetBalance(args *api.BalArgs) (uint64, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.Balance", args)
	if err != nil {
		return 0, err
	}

	if resp.Error != nil {
		return 0, resp.Error
	}

	var bal uint64
	err = json.Unmarshal(resp.Data, &bal)

	if err != nil {
		return 0, errors.New("invalid response type")
	}

	return bal, nil
}

// TDU retrieves the TDU of the queried address
func (c *Client) TDU(args *api.TesseractArgs) (types.AssetMap, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.TDU", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var assetMap types.AssetMap
	err = json.Unmarshal(resp.Data, &assetMap)

	if err != nil {
		return nil, err
	}

	return assetMap, nil
}

// GetContextInfo returns the context Info of the queried address.
func (c *Client) GetContextInfo(args *api.ContextInfoArgs) (*api.ContextResponse, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.ContextInfo", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var contextResp api.ContextResponse

	err = json.Unmarshal(resp.Data, &contextResp)
	if err != nil {
		return nil, err
	}

	return &contextResp, nil
}

// InteractionReceipt returns the receipt of the interaction for given hash
func (c *Client) InteractionReceipt(args *api.ReceiptArgs) (*types.Receipt, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.InteractionReceipt", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var receipt types.Receipt

	err = json.Unmarshal(resp.Data, &receipt)
	if err != nil {
		return nil, err
	}

	return &receipt, nil
}

// InteractionCount returns the number of interactions sent for the given address
func (c *Client) InteractionCount(args *api.InteractionCountArgs) (uint64, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.InteractionCount", args)
	if err != nil {
		return 0, err
	}

	if resp.Error != nil {
		return 0, resp.Error
	}

	var count uint64

	err = json.Unmarshal(resp.Data, &count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// PendingInteractionCount returns the number of interactions sent for the given address.
func (c *Client) PendingInteractionCount(args *api.InteractionCountArgs) (*uint64, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.PendingInteractionCount", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var count uint64

	err = json.Unmarshal(resp.Data, &count)
	if err != nil {
		return nil, err
	}

	return &count, nil
}

// Storage returns the data associated with the given storage slot
func (c *Client) Storage(args *api.GetStorageArgs) (string, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.Storage", args)
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", resp.Error
	}

	var res string

	err = json.Unmarshal(resp.Data, &res)
	if err != nil {
		return "", err
	}

	return res, nil
}

// AccountState returns the account state of the given address
func (c *Client) AccountState(args *api.GetAccountArgs) (*types.Account, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.AccountState", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var account types.Account

	err = json.Unmarshal(resp.Data, &account)
	if err != nil {
		return nil, err
	}

	return &account, nil
}

// LogicManifest returns the manifest associated with the given logic id
func (c *Client) LogicManifest(args *api.LogicManifestArgs) ([]byte, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.LogicManifest", args)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var res []byte

	err = json.Unmarshal(resp.Data, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// SendInteractions sends given Interactions
func (c *Client) SendInteractions(args *api.SendIXArgs) (string, error) {
	var resp api.Response

	err := c.Call(&resp, "moi.SendInteractions", args)
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", resp.Error
	}

	var res string

	err = json.Unmarshal(resp.Data, &res)
	if err != nil {
		return "", err
	}

	return res, nil
}

// Content returns the interactions present in the given IxPool.
func (c *Client) Content(args *api.IxPoolArgs) (*api.ContentResponse, error) {
	var resp api.Response

	err := c.Call(&resp, "ixPool.Content", args)
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
func (c *Client) ContentFrom(args *api.IxPoolArgs) (*api.ContentFromResponse, error) {
	var resp api.Response

	err := c.Call(&resp, "ixPool.ContentFrom", args)
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
func (c *Client) Status(args *api.IxPoolArgs) (*api.StatusResponse, error) {
	var resp api.Response

	err := c.Call(&resp, "ixPool.Status", args)
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
func (c *Client) Inspect(args *api.IxPoolArgs) (*api.InspectResponse, error) {
	var resp api.Response

	err := c.Call(&resp, "ixPool.Inspect", args)
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
func (c *Client) WaitTime(args *api.IxPoolArgs) (int64, error) {
	var resp api.Response

	err := c.Call(&resp, "ixPool.WaitTime", args)
	if err != nil {
		return 0, err
	}

	if resp.Error != nil {
		return 0, resp.Error
	}

	var time int64

	err = json.Unmarshal(resp.Data, &time)
	if err != nil {
		return 0, err
	}

	return time, nil
}

// Peers returns an array of Krama IDs connected to a client
func (c *Client) Peers(args *api.NetArgs) (*[]kramaid.KramaID, error) {
	var resp api.Response

	err := c.Call(&resp, "net.Peers", args)
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

	return &response, nil
}

// DBGet returns raw value of the key stored in the database
func (c *Client) DBGet(args *api.NetArgs) (*[]kramaid.KramaID, error) {
	var resp api.Response

	err := c.Call(&resp, "debug.DBGet", args)
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

	return &response, nil
}

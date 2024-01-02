package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi-identifiers"
)

type ConnManager interface {
	HasConn() bool
	SetFilterID(filterID string)
	GetFilterID() string
	WriteMessage(messageType int, data []byte) error
}

// Dispatcher handles all websocket requests
type Dispatcher struct {
	logger hclog.Logger
	fm     *FilterManager
}

func NewDispatcher(logger hclog.Logger, filterMan *FilterManager) Dispatcher {
	dispatcher := &Dispatcher{
		logger: logger.Named("Websocket-Dispatcher"),
		fm:     filterMan,
	}

	go dispatcher.fm.Run()

	return *dispatcher
}

// formatResponse formats the websocket response
func formatResponse(id interface{}, resp string) (string, error) {
	switch t := id.(type) {
	case string:
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":"%s","result":"%s"}`, t, resp), nil
	case float64:
		if t == math.Trunc(t) {
			return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"%s"}`, int(t), resp), nil
		} else {
			return "", errors.New("invalid json request")
		}
	case nil:
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":null,"result":"%s"}`, resp), nil
	default:
		return "", errors.New("invalid json request")
	}
}

// decodeTesseractArgs decodes the json rpc request parameter made for new tesseracts event subscription
func decodeTesseractArgs(data interface{}) (*TesseractArgs, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	args := new(TesseractArgs)
	if err := json.Unmarshal(raw, args); err != nil {
		return nil, err
	}

	return args, nil
}

// handleRequests handles all the incoming websocket requests
func (d *Dispatcher) handleRequests(reqBody []byte, conn ConnManager) ([]byte, error) {
	var req WSRequest
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return NewRPCResponse(req.ID, 400, "invalid request payload", "2.0")
	}

	// if the request method is moi.subscribe, create a new subscription with ws connection
	if req.Method == "moi.subscribe" {
		subscriptionID, err := d.handleSubscribe(req, conn)
		if err != nil {
			return NewRPCResponse(req.ID, 400, err.Error(), "2.0")
		}

		resp, err := formatResponse(req.ID, subscriptionID)
		if err != nil {
			return NewRPCResponse(req.ID, 400, err.Error(), "2.0")
		}

		return []byte(resp), nil
	}

	// if the request method is moi.unsubscribe, remove the subscription corresponding to the ws connection
	if req.Method == "moi.unsubscribe" {
		ok, err := d.handleUnsubscribe(req)
		if err != nil {
			return NewRPCResponse(req.ID, 400, err.Error(), "2.0")
		}

		res := "false"
		if ok {
			res = "true"
		}

		resp, err := formatResponse(req.ID, res)
		if err != nil {
			return NewRPCResponse(req.ID, 400, err.Error(), "2.0")
		}

		return []byte(resp), nil
	}

	return NewRPCResponse(req.ID, 400, errors.New("invalid request method").Error(), "2.0")
}

// handleSubscribe method subscribes to a specific event
func (d *Dispatcher) handleSubscribe(req WSRequest, conn ConnManager) (string, error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", errors.New("invalid json request")
	}

	event, ok := params[0].(string)
	if !ok {
		return "", fmt.Errorf("event %s not found", event)
	}

	switch event {
	case "newTesseracts":
		subscriptionID := d.fm.NewTesseractFilter(conn)

		return subscriptionID, nil
	case "newTesseractsByAccount":
		if len(params) != 2 {
			return "", errors.New("invalid params")
		}

		args, err := decodeTesseractArgs(params[1])
		if err != nil {
			return "", err
		}

		addr, _ := identifiers.NewAddressFromHex(args.Address)
		subscriptionID := d.fm.NewTesseractsByAccountFilter(conn, addr)

		return subscriptionID, nil
	case "newLogs":
		if len(params) != 2 {
			return "", errors.New("invalid params")
		}

		args, err := decodeFilterQuery(params[1])
		if err != nil {
			return "", err
		}

		subscriptionID := d.fm.NewLogFilter(conn, args)

		return subscriptionID, nil
	case "newPendingInteractions":
		subscriptionID := d.fm.PendingIxnsFilter(conn)

		return subscriptionID, nil
	default:
		return "", fmt.Errorf("event %s not found", event)
	}
}

// handleUnsubscribe method unsubscribes from a specific event
func (d *Dispatcher) handleUnsubscribe(req WSRequest) (bool, error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return false, errors.New("invalid json request")
	}

	if len(params) != 1 {
		return false, errors.New("invalid params")
	}

	subscriptionID, ok := params[0].(string)
	if !ok {
		return false, fmt.Errorf("subscription id %s not found", subscriptionID)
	}

	return d.fm.Uninstall(subscriptionID), nil
}

// RemoveSubscription removes a filter corresponding to the websocket connection from the filter manager
func (d *Dispatcher) RemoveSubscription(connManager ConnManager) bool {
	return d.fm.Uninstall(connManager.GetFilterID())
}

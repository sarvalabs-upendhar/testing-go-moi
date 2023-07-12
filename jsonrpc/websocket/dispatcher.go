package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/utils"
)

type ConnManager interface {
	HasConn() bool
	SetSubscriptionID(subscriptionID string)
	GetSubscriptionID() string
	WriteMessage(messageType int, data []byte) error
}

// Dispatcher handles all websocket requests
type Dispatcher struct {
	logger hclog.Logger
	sm     *SubscriptionManager
}

func NewDispatcher(logger hclog.Logger, eventMux *utils.TypeMux) Dispatcher {
	dispatcher := &Dispatcher{
		logger: logger.Named("Websocket-Dispatcher"),
		sm:     NewSubscriptionManager(logger, eventMux),
	}

	go dispatcher.sm.Run()

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
		subscriptionID := d.sm.NewTesseractSubscription(conn)

		return subscriptionID, nil
	case "newAccountTesseracts":
		if len(params) != 2 {
			return "", errors.New("invalid params")
		}

		args, err := decodeTesseractArgs(params[1])
		if err != nil {
			return "", err
		}

		subscriptionID := d.sm.NewAccountTesseractSubscription(conn, common.HexToAddress(args.Address))

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

	return d.sm.Uninstall(subscriptionID), nil
}

// RemoveSubscription removes a subscription corresponding to the websocket connection from the subscription manager
func (d *Dispatcher) RemoveSubscription(connManager ConnManager) bool {
	return d.sm.Uninstall(connManager.GetSubscriptionID())
}

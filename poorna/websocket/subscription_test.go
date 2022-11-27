package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sarvalabs/moichain/types"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/utils"
	"github.com/stretchr/testify/require"
)

type Result struct {
	Header struct {
		Address string
		Height  int
	}
}

type MessageParams struct {
	Subscription string `json:"subscription"`
	Result       Result `json:"result"`
}

type Message struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  interface{}   `json:"method"`
	Params  MessageParams `json:"params"`
}

func TestTesseractSubscription(t *testing.T) {
	eventMux := new(utils.TypeMux)
	subscriptionManager := NewSubscriptionManager(hclog.NewNullLogger(), eventMux)

	// start the subscription manager worker process
	go subscriptionManager.Run()

	// Create a mock connection
	connManager := NewMockConnectionManager()
	tesseract := tests.GetTesseract(t, 0)
	// Create a new tesseract subscription
	subscriptionID := subscriptionManager.NewTesseractSubscription(connManager, tesseract.Address())

	// post an event
	if err := eventMux.Post(utils.TesseractAddedEvent{Tesseract: tesseract}); err != nil {
		t.Fatalf("error sending tesseract added event")
	}

	// Wait for 100ms, so that the worker process can handle the event
	time.Sleep(100 * time.Millisecond)

	subscriptionBase := subscriptionManager.subscriptions[subscriptionID].getSubscriptionBase()
	// Check whether websocket connection exists
	require.Equal(t, true, subscriptionBase.hasWSConn())
	// Read the response message from the websocket connection
	messageType, message, err := connManager.readMessage()
	require.NoError(t, err)
	// Check whether the response message type is same as expected message type
	require.Equal(t, websocket.TextMessage, messageType)

	var response Message
	// Unmarshal the message
	err = json.Unmarshal(message, &response)
	require.NoError(t, err)
	require.NotNil(
		t,
		response.Params,
		response.Params.Result,
		response.Params.Subscription,
	)

	// Check if the received subscription id matches the current subscription id
	require.Equal(t, subscriptionID, response.Params.Subscription)

	// Check whether the sent tesseract and received tesseract address and height matches
	require.Equal(t, tesseract.Address(), types.HexToAddress(response.Params.Result.Header.Address))
	require.Equal(t, tesseract.Height(), uint64(response.Params.Result.Header.Height))
}

func TestSubscriptionTimeout(t *testing.T) {
	eventMux := new(utils.TypeMux)
	subscriptionManager := NewSubscriptionManager(hclog.NewNullLogger(), eventMux)

	defer subscriptionManager.Close()

	subscriptionManager.timeout = 1 * time.Second

	go subscriptionManager.Run()

	// post an event
	tesseract := tests.GetTesseract(t, 0)
	subscriptionID := subscriptionManager.NewTesseractSubscription(nil, tesseract.Address())

	// Check if the subscription manager has the subscription
	require.True(t, hasSubscribed(subscriptionID, subscriptionManager.subscriptions))
	time.Sleep(2 * time.Second)
	// Check if the subscription manager has removed the subscription or not
	require.False(t, hasSubscribed(subscriptionID, subscriptionManager.subscriptions))
}

// helper function

// check subscription exists
func hasSubscribed(subscriptionID string, subscriptions map[string]subscription) bool {
	if _, ok := subscriptions[subscriptionID]; ok {
		return true
	}

	return false
}

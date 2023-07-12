package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/common/utils"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
)

type Result struct {
	Header struct {
		Address string
		Height  hexutil.Uint64
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

func TestAccountTesseractSubscription(t *testing.T) {
	eventMux := new(utils.TypeMux)
	subscriptionManager := NewSubscriptionManager(hclog.NewNullLogger(), eventMux)

	// start the subscription manager worker process
	go subscriptionManager.Run()

	// Create a mock connection
	connManager := NewMockConnectionManager()
	params := &tests.CreateTesseractParams{
		Height:         5,
		HeaderCallback: tests.HeaderCallbackWithGridHash(t),
	}
	tesseract := tests.CreateTesseract(t, params)

	// Create a new tesseract subscription
	subscriptionID := subscriptionManager.NewAccountTesseractSubscription(connManager, tesseract.Address())

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
	require.Equal(t, tesseract.Address(), common.HexToAddress(response.Params.Result.Header.Address))
	require.Equal(t, tesseract.Height(), uint64(response.Params.Result.Header.Height))
}

func TestTesseractSubscription(t *testing.T) {
	eventMux := new(utils.TypeMux)
	subscriptionManager := NewSubscriptionManager(hclog.NewNullLogger(), eventMux)

	// start the subscription manager worker process
	go subscriptionManager.Run()

	// Create a mock connection
	connManager := NewMockConnectionManager()
	params := &tests.CreateTesseractParams{
		Height:         5,
		HeaderCallback: tests.HeaderCallbackWithGridHash(t),
	}
	tesseract := tests.CreateTesseract(t, params)

	// Create a new tesseract subscription
	subscriptionID := subscriptionManager.NewTesseractSubscription(connManager)

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
	require.Equal(t, tesseract.Address(), common.HexToAddress(response.Params.Result.Header.Address))
	require.Equal(t, tesseract.Height(), uint64(response.Params.Result.Header.Height))
}

func TestSubscriptionTimeout(t *testing.T) {
	eventMux := new(utils.TypeMux)
	subscriptionManager := NewSubscriptionManager(hclog.NewNullLogger(), eventMux)

	defer subscriptionManager.Close()

	subscriptionManager.timeout = 1 * time.Second

	go subscriptionManager.Run()

	// post an event
	params := &tests.CreateTesseractParams{
		Height:         5,
		HeaderCallback: tests.HeaderCallbackWithGridHash(t),
	}
	tesseract := tests.CreateTesseract(t, params)

	subscriptionID := subscriptionManager.NewAccountTesseractSubscription(nil, tesseract.Address())

	// Check if the subscription manager has the subscription
	require.True(t, subscriptionManager.hasSubscribed(subscriptionID))
	time.Sleep(2 * time.Second)
	// Check if the subscription manager has removed the subscription or not
	require.False(t, subscriptionManager.hasSubscribed(subscriptionID))
}

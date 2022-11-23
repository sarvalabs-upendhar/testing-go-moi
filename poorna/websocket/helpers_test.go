package websocket

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/utils"
)

func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	eventMux := new(utils.TypeMux)
	wsHandler := NewHandler(hclog.NewNullLogger(), eventMux)
	mux.HandleFunc("/ws", wsHandler.HandleWsRequests)

	return httptest.NewServer(mux)
}

func initWSConnection(t *testing.T, address string) (*websocket.Conn, *http.Response) {
	t.Helper()

	dialer := websocket.Dialer{}
	// Establish a websocket connection
	conn, resp, err := dialer.Dial("ws://"+address+"/ws", nil)
	require.NoError(t, err)

	return conn, resp
}

type MockMessage struct {
	messageType int
	data        []byte
}

type MockWSConn struct {
	message *MockMessage
}

type MockConnManager struct {
	wsConn         *MockWSConn
	subscriptionID string
}

func NewMockConnectionManager() *MockConnManager {
	return &MockConnManager{
		wsConn: &MockWSConn{},
	}
}

func (mc *MockConnManager) HasConn() bool {
	return mc.wsConn != nil
}

func (mc *MockConnManager) WriteMessage(messageType int, data []byte) error {
	if mc.wsConn != nil {
		mc.wsConn.message = &MockMessage{
			messageType,
			data,
		}

		return nil
	}

	return errors.New("no message")
}

func (mc *MockConnManager) readMessage() (messageType int, p []byte, err error) {
	if mc.wsConn.message != nil {
		message := mc.wsConn.message
		mc.wsConn.message = nil

		return message.messageType, message.data, nil
	}

	return -1, nil, errors.New("no message found")
}

func (mc *MockConnManager) SetSubscriptionID(subscriptionID string) {
	mc.subscriptionID = subscriptionID
}

func (mc *MockConnManager) GetSubscriptionID() string {
	return mc.subscriptionID
}

package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
)

type RequestArgs struct {
	MessageType int
	Message     []byte
}

func Test_HandleWsRequests_Upgrader(t *testing.T) {
	server := newMockServer(t)

	// check whether websocket upgrader works
	dialer := websocket.Dialer{}
	_, resp, err := dialer.Dial("ws://"+server.Listener.Addr().String()+"/ws", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	err = resp.Body.Close()
	require.NoError(t, err)
}

func Test_HandleWsRequests_Message(t *testing.T) {
	// Establish a websocket connection with a httptest server
	server := newMockServer(t)
	conn, resp := initWSConnection(t, server.Listener.Addr().String())

	testcases := []struct {
		name        string
		args        RequestArgs
		expectedErr error
	}{
		{
			name: "Subscription request message without address param",
			args: RequestArgs{
				MessageType: websocket.TextMessage,
				Message: []byte(`{
					"method": "moi.subscribe",
					"params": ["newTesseracts"]
				}`),
			},
		},
		{
			name: "Subscription request message with valid params",
			args: RequestArgs{
				MessageType: websocket.TextMessage,
				Message: []byte(fmt.Sprintf(`{
					"id": 1,
					"method": "moi.subscribe",
					"params": [
						"newTesseractsByAccount", 
   						{
							"address": "%s"
						}
					]
				}`, tests.RandomAddress(t))),
			},
		},
		{
			name: "Log Subscription request message with valid params",
			args: RequestArgs{
				MessageType: websocket.TextMessage,
				Message: []byte(fmt.Sprintf(`{
					"id": 1,
					"method": "moi.subscribe",
					"params": [
						"newLogs", 
   						{
							"address": "%s"
						}
					]
				}`, tests.RandomAddress(t))),
			},
		},
		{
			name: "Pending Ixns request message with valid params",
			args: RequestArgs{
				MessageType: websocket.TextMessage,
				Message: []byte(`{
					"id": 1,
					"method": "moi.subscribe",
					"params": ["newPendingInteractions"]
				}`),
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			// Write the request message to the websocket connection
			err := conn.WriteMessage(testcase.args.MessageType, testcase.args.Message)
			require.NoError(t, err)

			// Read the response message from the websocket connection
			messageType, message, err := conn.ReadMessage()
			require.NoError(t, err)

			// Check whether the response message type is same as expected message type
			require.Equal(t, websocket.TextMessage, messageType)

			var response RPCResponse

			err = json.Unmarshal(message, &response)
			require.NoError(t, err)

			if testcase.expectedErr != nil {
				require.NotNil(t, response.Error.Message)
				require.Equal(t, testcase.expectedErr.Error(), response.Error.Message)
			} else {
				require.Nil(t, response.Error)
				require.NotNil(t, response.Result)
			}
		})
	}

	err := resp.Body.Close()
	require.NoError(t, err)
}

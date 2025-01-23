package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaWS "github.com/gorilla/websocket"
	"github.com/sarvalabs/go-moi/common/tests"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common/utils"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

type RequestArgs struct {
	MessageType int
	Message     []byte
}

func MockServer(t *testing.T, addr *net.TCPAddr) *Server {
	t.Helper()

	eventMux := new(utils.TypeMux)
	filterMan := NewFilterManager(hclog.NewNullLogger(), eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(hclog.NewNullLogger(), cfg, filterMan)

	return &Server{
		logger:             hclog.NewNullLogger(),
		router:             http.NewServeMux(),
		url:                "/",
		addr:               addr,
		corsAllowedOrigins: []string{"*"},
		dispatcher:         d,
	}
}

func TestServer_RegisterService(t *testing.T) {
	mockServer := MockServer(t, serverAddr)

	mockRegisterFunc := rpcargs.MockRegisterValidMethod()

	mockrpc := struct {
		Namespace string
		Services  *rpcargs.MockMethodData
	}{
		Namespace: "Test",
		Services:  mockRegisterFunc,
	}

	err := mockServer.RegisterService(mockrpc.Namespace, mockrpc.Services)
	require.NoError(t, err)
}

func TestHandleJSONRPCRequest(t *testing.T) {
	mockServer := MockServer(t, serverAddr)

	request := Request{
		ID:     "1",
		Method: "moi.subscribe",
		Params: json.RawMessage(`["newTesseracts"]`),
	}

	reqBody, _ := json.Marshal(request)

	// Create a mock HTTP request
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Create a response recorder to record the response
	recorder := httptest.NewRecorder()

	mockServer.handleJSONRPCRequest(recorder, req)

	// Check the status code of the response
	require.Equal(t, http.StatusOK, recorder.Code)

	expectedResponseBody := `{"jsonrpc":"2.0","id":"1"`
	require.Contains(t, recorder.Body.String(), expectedResponseBody)
}

func TestServer_handle(t *testing.T) {
	server := MockServer(t, serverAddr)

	// Create a new HTTP request with different methods
	testcases := []struct {
		name             string
		method           string
		expectedResponse string
	}{
		{
			name:             "POST request",
			method:           http.MethodPost,
			expectedResponse: `the method  does not exist/is not available`,
		},
		{
			name:             "OPTION request",
			method:           http.MethodOptions,
			expectedResponse: "",
		}, // OPTIONS request should have no response
		{
			name:             "GET request",
			method:           http.MethodGet,
			expectedResponse: "method GET not allowed",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			req, err := http.NewRequest(testcase.method, "/", nil)
			require.NoError(t, err)

			// For "POST request", add some data to the request body
			if testcase.method == http.MethodPost {
				req.Body = io.NopCloser(strings.NewReader(`{"key": "value"}`))
			}

			// Create a new HTTP recorder (implements http.ResponseWriter) to record the response
			recorder := httptest.NewRecorder()

			// Call the handle method with the mock request and recorder
			server.handle(recorder, req)

			// Check the response status code
			require.Equal(t, http.StatusOK, recorder.Code)

			// Check the response body
			require.Contains(t, strings.TrimSpace(recorder.Body.String()), testcase.expectedResponse)
		})
	}
}

func TestServer_handleWs_wsUpgrader(t *testing.T) {
	_, resp := setupTestHTTPServer(t)

	// check whether websocket upgrader works
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	err := resp.Body.Close()
	require.NoError(t, err)
}

func TestServer_handleWs_Message(t *testing.T) {
	conn, resp := setupTestHTTPServer(t)

	testcases := []struct {
		name string
		args RequestArgs
	}{
		{
			name: "Subscription request message without id param",
			args: RequestArgs{
				MessageType: gorillaWS.TextMessage,
				Message: []byte(`{
					"method": "moi.subscribe",
					"params": ["newTesseracts"]
				}`),
			},
		},
		{
			name: "Subscription request message with valid params",
			args: RequestArgs{
				MessageType: gorillaWS.TextMessage,
				Message: []byte(fmt.Sprintf(`{
					"id": 1,
					"method": "moi.subscribe",
					"params": [
						"newTesseractsByAccount",
 						{
							"id": "%s"
						}
					]
				}`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "Log Subscription request message with valid params",
			args: RequestArgs{
				MessageType: gorillaWS.TextMessage,
				Message: []byte(fmt.Sprintf(`{
					"id": 1,
					"method": "moi.subscribe",
					"params": [
						"newLogs",
 						{
							"id": "%s"
						}
					]
				}`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "Pending Ixns request message with valid params",
			args: RequestArgs{
				MessageType: gorillaWS.TextMessage,
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
			require.Equal(t, gorillaWS.TextMessage, messageType)

			var response SuccessResponse

			err = json.Unmarshal(message, &response)
			require.NoError(t, err)
			require.Nil(t, response.Error)
			require.NotNil(t, response.Result)
		})
	}

	err := resp.Body.Close()
	require.NoError(t, err)
}

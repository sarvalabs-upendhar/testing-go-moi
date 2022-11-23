package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/tests"
	"gitlab.com/sarvalabs/moichain/utils"
)

func Test_handleRequest_Subscribe(t *testing.T) {
	eventMux := new(utils.TypeMux)
	// Create a new dispatcher
	dispatcher := NewDispatcher(hclog.NewNullLogger(), eventMux)
	// Create a mock connection manager
	mockConnManager := NewMockConnectionManager()

	testcases := []struct {
		name        string
		message     []byte
		expectedErr error
	}{
		{
			name: "Subscription request without address param",
			message: []byte(`{
				"method": "moi.subscribe",
				"params": ["newTesseracts"]
			}`),
			expectedErr: errors.New("invalid params"),
		},
		{
			name: "Subscription request without event param",
			message: []byte(fmt.Sprintf(`{
				"id": 1,
				"method": "moi.subscribe",
				"params": [
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: errors.New("invalid params"),
		},
		{
			name: "Subscription request with a non existing event name",
			message: []byte(fmt.Sprintf(`{
				"id": 1,
				"method": "moi.subscribe",
				"params": [
					"newTesseract",
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: errors.New("event newTesseract not found"),
		},
		{
			name: "Subscription request with valid params",
			message: []byte(fmt.Sprintf(`{
				"id": 1,
				"method": "moi.subscribe",
				"params": [
					"newTesseracts", 
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			// Forward the message to dispatcher
			respBytes, err := dispatcher.handleRequests(testcase.message, mockConnManager)
			require.NoError(t, err)

			// Unmarshall dispatcher response
			var response RPCResponse

			err = json.Unmarshal(respBytes, &response)
			require.NoError(t, err)

			if testcase.expectedErr != nil {
				require.Equal(t, testcase.expectedErr.Error(), response.Error.Message)

				return
			}

			require.Nil(t, response.Error)

			// Unmarshall the dispatcher result from the response
			var result string

			err = json.Unmarshal(response.Result, &result)
			require.NoError(t, err)

			// Check if the connection manager's subscription id and dispatcher result is same
			require.Equal(t, mockConnManager.GetSubscriptionID(), result)
		})
	}
}

func Test_handleRequest_Unsubscribe(t *testing.T) {
	eventMux := new(utils.TypeMux)
	// Create a new dispatcher
	dispatcher := NewDispatcher(hclog.NewNullLogger(), eventMux)
	// Create a mock connection manager
	mockConnManager := NewMockConnectionManager()

	subscribeToNewTesseractEvent(t, dispatcher, mockConnManager)

	testcases := []struct {
		name        string
		message     []byte
		expected    string
		expectedErr error
	}{
		{
			name: "Unsubscribe request without subscription ID",
			message: []byte(`{
				"id": 1,
				"method": "moi.unsubscribe",
				"params": []
			}`),
			expectedErr: errors.New("invalid params"),
		},
		{
			name: "Unsubscribe request with a subscription ID that doesn't exists",
			message: []byte(fmt.Sprintf(`{
				"id": 2,
				"method": "moi.unsubscribe",
				"params": ["%s"]
			}`, uuid.New().String())),
			expected: "false",
		},
		{
			name: "Unsubscribe request with valid subscription ID",
			message: []byte(fmt.Sprintf(`{
				"id": 2,
				"method": "moi.unsubscribe",
				"params": ["%s"]
			}`, mockConnManager.GetSubscriptionID())),
			expected: "true",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			// Forward the message to dispatcher
			respBytes, err := dispatcher.handleRequests(testcase.message, mockConnManager)
			require.NoError(t, err)

			// Unmarshall dispatcher response
			var response RPCResponse

			err = json.Unmarshal(respBytes, &response)
			require.NoError(t, err)

			if testcase.expectedErr != nil {
				require.Equal(t, testcase.expectedErr.Error(), response.Error.Message)

				return
			}

			require.Nil(t, response.Error)

			// Unmarshall the dispatcher result from the response
			var result string

			err = json.Unmarshal(response.Result, &result)
			require.NoError(t, err)

			require.Equal(t, testcase.expected, result)
		})
	}
}

func Test_handleRequests_RequestFormats(t *testing.T) {
	eventMux := new(utils.TypeMux)
	// Create a new dispatcher
	dispatcher := NewDispatcher(hclog.NewNullLogger(), eventMux)
	// Create a mock connection manager
	mockConnManager := NewMockConnectionManager()

	testcases := []struct {
		name        string
		message     []byte
		expectedErr error
	}{
		{
			name: "Should not return error, if the request id is of type string",
			message: []byte(fmt.Sprintf(`{
				"id": "1",
				"method": "moi.subscribe",
				"params": [
					"newTesseracts", 
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: nil,
		},
		{
			name: "Should not return error, if the request id is of type float and significand value is 0",
			message: []byte(fmt.Sprintf(`{
				"id": 2.0,
				"method": "moi.subscribe",
				"params": [
					"newTesseracts", 
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: nil,
		},
		{
			name: "Should not return error, if the request id is not sent",
			message: []byte(fmt.Sprintf(`{
				"method": "moi.subscribe",
				"params": [
					"newTesseracts", 
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: nil,
		},
		{
			name: "Should return error, if the request id is of type float and significand value is greater than 0",
			message: []byte(fmt.Sprintf(`{
				"id": 2.1,
				"method": "moi.subscribe",
				"params": [
					"newTesseracts", 
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: errors.New("invalid json request"),
		},
		{
			name: "Should not return error, if the request id is null",
			message: []byte(fmt.Sprintf(`{
				"id": null,
				"method": "moi.subscribe",
				"params": [
					"newTesseracts", 
					{
						"address": "%s"
					}
				]
			}`, tests.RandomAddress(t))),
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			// Forward the message to dispatcher
			respBytes, err := dispatcher.handleRequests(testcase.message, mockConnManager)
			require.NoError(t, err)

			// Unmarshall dispatcher response
			var response RPCResponse

			err = json.Unmarshal(respBytes, &response)
			require.NoError(t, err)

			if testcase.expectedErr != nil {
				require.Equal(t, testcase.expectedErr.Error(), response.Error.Message)

				return
			}

			require.Nil(t, response.Error)
		})
	}
}

func Test_RemoveSubscription(t *testing.T) {
	eventMux := new(utils.TypeMux)
	// Create a new dispatcher
	dispatcher := NewDispatcher(hclog.NewNullLogger(), eventMux)
	// Create a mock connection manager
	mockConnManager := NewMockConnectionManager()

	subscribeToNewTesseractEvent(t, dispatcher, mockConnManager)

	testcases := []struct {
		name           string
		subscriptionID string
		expected       bool
	}{
		{
			name:           "Should return false, when non existing Subscription ID is passed as parameter",
			subscriptionID: uuid.New().String(),
			expected:       true,
		},
		{
			name:           "Should return true, when valid Subscription ID is passed as parameter",
			subscriptionID: mockConnManager.GetSubscriptionID(),
			expected:       false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			result := dispatcher.RemoveSubscription(mockConnManager)
			require.Equal(t, testcase.expected, result)
		})
	}
}

// helper functions
func subscribeToNewTesseractEvent(t *testing.T, dispatcher Dispatcher, mockConnManager *MockConnManager) {
	t.Helper()

	message := []byte(fmt.Sprintf(`{
		"id": 1,
		"method": "moi.subscribe",
		"params": [
			"newTesseracts", 
			{
				"address": "%s"
			}
		]
	}`, tests.RandomAddress(t)))

	// Forward the message to dispatcher
	respBytes, err := dispatcher.handleRequests(message, mockConnManager)
	require.NoError(t, err)

	// Unmarshall dispatcher response
	var response RPCResponse

	err = json.Unmarshal(respBytes, &response)
	require.NoError(t, err)

	require.Nil(t, response.Error)

	// Unmarshall the dispatcher result from the response
	var result string

	err = json.Unmarshal(response.Result, &result)
	require.NoError(t, err)

	// Check whether the connection manager's subscription id and dispatcher result matches
	require.Equal(t, mockConnManager.GetSubscriptionID(), result)
}

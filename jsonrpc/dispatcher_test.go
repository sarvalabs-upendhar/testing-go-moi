package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/sarvalabs/go-moi/common"

	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/sarvalabs/go-moi/common/utils"

	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
)

func Test_handleSingleWs_Subscribe(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	mockConnManager := NewMockConnectionManager()

	testcases := []struct {
		name        string
		request     Request
		expectedErr error
	}{
		{
			name: "Subscription request without id param",
			request: Request{
				Method: "moi.subscribe",
				Params: json.RawMessage(`["newTesseractsByAccount"]`),
			},
			expectedErr: common.ErrInvalidParams,
		},
		{
			name: "Subscription request without event param",
			request: Request{
				ID:     1.0,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`[{"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
			expectedErr: errors.New("subscribe method  not found"),
		},
		{
			name: "Subscription request with a non-existing event name",
			request: Request{
				ID:     2.0,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["mockEvent", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
			expectedErr: errors.New("subscribe method mockEvent not found"),
		},
		{
			name: "Subscription request with valid ts by account filter params",
			request: Request{
				ID:     3.0,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "Log subscription request without id param",
			request: Request{
				Method: "moi.subscribe",
				Params: json.RawMessage(`["newLogs"]`),
			},
			expectedErr: common.ErrInvalidParams,
		},
		{
			name: "Subscription request with valid log filter params",
			request: Request{
				ID:     4.0,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newLogs", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			response := d.handleSingleWs(testcase.request, mockConnManager)

			if testcase.expectedErr != nil {
				errResponse, ok := response.(*ErrorResponse)
				require.True(t, ok)
				require.Equal(t, testcase.expectedErr.Error(), errResponse.Error.Message)

				return
			}

			successResponse, ok := response.(*SuccessResponse)
			require.True(t, ok)
			require.Nil(t, successResponse.Error)

			var resultString string
			err := json.Unmarshal(successResponse.Result, &resultString)
			require.NoError(t, err)

			// Check if the connection manager's subscription id and dispatcher result is same
			require.Equal(t, mockConnManager.GetFilterID(), resultString)
		})
	}
}

func Test_handleSingleWs_default(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	err := d.registerService("test", rpcargs.MockRegisterValidMethod())
	require.NoError(t, err)

	testcases := []struct {
		name        string
		request     Request
		expected    *rpcargs.MockMethodData
		expectedErr error
	}{
		{
			name: "handle single valid ws default request",
			request: Request{
				Method: "test.MockMethodWithResp",
				Params: json.RawMessage(`[{}]`),
			},
			expected: &rpcargs.MockMethodData{
				ID:   1,
				Name: "mockMethodData",
			},
		},
		{
			name: "method returns error response",
			request: Request{
				Method: "test.MockMethodWithError",
				Params: json.RawMessage(`[{}]`),
			},
			expectedErr: errors.New("mock error"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			response := d.handleSingleWs(testcase.request, nil)

			if testcase.expectedErr != nil {
				errResponse, ok := response.(*ErrorResponse)
				require.True(t, ok)
				require.Equal(t, testcase.expectedErr.Error(), errResponse.Error.Message)

				return
			}

			successResponse, ok := response.(*SuccessResponse)
			require.True(t, ok)
			require.Nil(t, successResponse.Error)

			var result rpcargs.MockMethodData
			err := json.Unmarshal(successResponse.Result, &result)
			require.NoError(t, err)

			require.Equal(t, testcase.expected, &result)
		})
	}
}

func Test_handleSingleWs_Unsubscribe(t *testing.T) {
	eventMux := new(utils.TypeMux)
	filterMan := NewFilterManager(hclog.NewNullLogger(), eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(hclog.NewNullLogger(), cfg, filterMan)

	mockConnManager := NewMockConnectionManager()

	subscribeToNewTesseractEvent(t, d, mockConnManager)

	testcases := []struct {
		name        string
		request     Request
		expected    bool
		expectedErr error
	}{
		{
			name: "Unsubscribe request without subscription ID",
			request: Request{
				ID:     "1",
				Method: "moi.unsubscribe",
				Params: json.RawMessage(`[]`),
			},
			expectedErr: common.ErrInvalidParams,
		},
		{
			name: "Unsubscribe request with a subscription ID that doesn't exist",
			request: Request{
				ID:     "2",
				Method: "moi.unsubscribe",
				Params: json.RawMessage(fmt.Sprintf(`["%s"]`, uuid.New().String())),
			},
			expected: false,
		},
		{
			name: "Unsubscribe request with valid subscription ID",
			request: Request{
				ID:     "3",
				Method: "moi.unsubscribe",
				Params: json.RawMessage(fmt.Sprintf(`["%s"]`, mockConnManager.GetFilterID())),
			},
			expected: true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			response := d.handleSingleWs(testcase.request, mockConnManager)

			if testcase.expectedErr != nil {
				errResponse, ok := response.(*ErrorResponse)
				require.True(t, ok)
				require.Equal(t, testcase.expectedErr.Error(), errResponse.Error.Message)

				return
			}

			successResponse, ok := response.(*SuccessResponse)
			require.True(t, ok)
			require.Nil(t, successResponse.Error)

			var result bool
			err := json.Unmarshal(successResponse.Result, &result)
			require.NoError(t, err)

			require.Equal(t, testcase.expected, result)
		})
	}
}

func Test_handleSingleWs_formatID(t *testing.T) {
	eventMux := new(utils.TypeMux)
	filterMan := NewFilterManager(hclog.NewNullLogger(), eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(hclog.NewNullLogger(), cfg, filterMan)

	mockConnManager := NewMockConnectionManager()

	testcases := []struct {
		name        string
		request     Request
		expectedErr error
	}{
		{
			name: "should not return error, if the request id is of type string",
			request: Request{
				ID:     "id123",
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "should not return error, if the request id is of type float and significand value is 0",
			request: Request{
				ID:     2.0,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "should not return error, if the request id is not sent",
			request: Request{
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "should return error, if the request id is of type float and significand value is greater than 0",
			request: Request{
				ID:     2.1,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
			expectedErr: errors.New("invalid json request"),
		},
		{
			name: "should not return error, if the request id is null",
			request: Request{
				ID:     nil,
				Method: "moi.subscribe",
				Params: json.RawMessage(fmt.Sprintf(`["newTesseractsByAccount", {"id": "%s"}]`, tests.RandomIdentifier(t))),
			},
		},
		{
			name: "wrong ID format",
			request: Request{
				ID:     json.RawMessage(`[{}]`),
				Method: "test.MockMethodWithError",
				Params: json.RawMessage(`[{}]`),
			},
			expectedErr: errors.New("invalid json request"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			// Forward the message to dispatcher
			response := d.handleSingleWs(testcase.request, mockConnManager)

			if testcase.expectedErr != nil {
				errResponse, ok := response.(*ErrorResponse)
				require.True(t, ok)
				require.Equal(t, testcase.expectedErr.Error(), errResponse.Error.Message)

				return
			}

			errResponse, ok := response.(*SuccessResponse)
			require.True(t, ok)
			require.Nil(t, errResponse.Error)
		})
	}
}

func Test_RemoveSubscription(t *testing.T) {
	eventMux := new(utils.TypeMux)
	filterMan := NewFilterManager(hclog.NewNullLogger(), eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(hclog.NewNullLogger(), cfg, filterMan)

	mockConnManager := NewMockConnectionManager()

	subscribeToNewTesseractEvent(t, d, mockConnManager)

	testcases := []struct {
		name           string
		subscriptionID string
		expected       bool
	}{
		{
			name:           "should return false, when non existing Subscription ID is passed as parameter",
			subscriptionID: uuid.New().String(),
			expected:       true,
		},
		{
			name:           "should return true, when valid Subscription ID is passed as parameter",
			subscriptionID: mockConnManager.GetFilterID(),
			expected:       false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			result := d.removeSubscription(mockConnManager)
			require.Equal(t, testcase.expected, result)
		})
	}
}

func Test_handleReq(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	err := d.registerService("test", rpcargs.MockRegisterValidMethod())
	require.NoError(t, err)

	testcases := []struct {
		name        string
		request     Request
		expected    *rpcargs.MockMethodData
		expectedErr error
	}{
		{
			name: "Method returns valid response",
			request: Request{
				ID:     3.0,
				Method: "test.MockMethodWithResp",
				Params: json.RawMessage(`[]`),
			},
			expected: &rpcargs.MockMethodData{
				ID:   1,
				Name: "mockMethodData",
			},
			expectedErr: nil,
		},
		{
			name: "Method returns error response",
			request: Request{
				ID:     3.0,
				Method: "test.MockMethodWithError",
				Params: json.RawMessage(`[]`),
			},
			expectedErr: errors.New("mock error"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			response, reqErr := d.handleReq(testcase.request)

			if testcase.expectedErr != nil {
				require.Contains(t, testcase.expectedErr.Error(), reqErr.Error())

				return
			}

			var result rpcargs.MockMethodData
			err := json.Unmarshal(response, &result)
			require.NoError(t, err)

			require.Equal(t, testcase.expected, &result)
		})
	}
}

func Test_getMethodHandler(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	err := d.registerService("test", rpcargs.MockRegisterValidMethod())
	require.NoError(t, err)

	testcases := []struct {
		name        string
		request     Request
		expectedErr error
	}{
		{
			name: "Valid method name",
			request: Request{
				ID:     3.0,
				Method: "test.MockMethodWithResp",
				Params: json.RawMessage(`[]`),
			},
			expectedErr: nil,
		},
		{
			name: "Invalid method name, service name is missing",
			request: Request{
				ID:     3.0,
				Method: "MockMethod",
				Params: json.RawMessage(`[]`),
			},
			expectedErr: errors.New("the method MockMethod does not exist/is not available"),
		},
		{
			name: "Service does not exit in service map",
			request: Request{
				ID:     3.0,
				Method: "moi.MockMethod",
				Params: json.RawMessage(`[]`),
			},
			expectedErr: errors.New("the method moi.MockMethod does not exist/is not available"),
		},
		{
			name: "Method does not exit in method map",
			request: Request{
				ID:     3.0,
				Method: "test.MockMethod",
				Params: json.RawMessage(`[]`),
			},
			expectedErr: errors.New("the method test.MockMethod does not exist/is not available"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			service, methodData, methodErr := d.getMethodHandler(testcase.request)

			if testcase.expectedErr != nil {
				require.Contains(t, testcase.expectedErr.Error(), methodErr.Error())

				return
			}

			require.Equal(t, d.serviceMap["test"], service)
			require.Equal(t, d.serviceMap["test"].methodMap["MockMethodWithResp"], methodData)
		})
	}
}

func Test_handleWs(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	testcases := []struct {
		name        string
		requestBody []byte
		expectedErr error
		isBatch     bool // Indicate if the response is expected to be a batch
	}{
		{
			name: "Handle valid single ws request",
			requestBody: func() []byte {
				req := Request{
					ID:     "1",
					Method: "moi.subscribe",
					Params: json.RawMessage(`["newTesseracts"]`),
				}
				reqBody, _ := json.Marshal(req)

				return reqBody
			}(),
		},
		{
			name: "Handle valid batch ws request",
			requestBody: func() []byte {
				reqs := []Request{
					{
						ID:     "1",
						Method: "moi.subscribe",
						Params: json.RawMessage(`["newTesseracts"]`),
					},
					{
						ID:     "2",
						Method: "moi.subscribe",
						Params: json.RawMessage(`["newTesseracts"]`),
					},
				}
				reqBody, _ := json.Marshal(reqs)

				return reqBody
			}(),
			isBatch: true,
		},
		{
			name:        "Invalid request body",
			requestBody: []byte(`{invalid json`),
			expectedErr: errors.New("invalid json request"),
		},
		{
			name: "Exceed batch length limit for ws requests",
			requestBody: func() []byte {
				reqs := make([]Request, 21) // Batch limit is 20
				for i := range reqs {
					reqs[i] = Request{
						ID:     fmt.Sprintf("%d", i),
						Method: "moi.subscribe",
						Params: json.RawMessage(`["newTesseracts"]`),
					}
				}
				reqBody, _ := json.Marshal(reqs)

				return reqBody
			}(),
			expectedErr: errors.New("batch request length too long"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			mockConnManager := NewMockConnectionManager()

			resp, err := d.handleWs(testcase.requestBody, mockConnManager)
			require.NoError(t, err)

			if testcase.expectedErr != nil {
				var errResponse ErrorResponse
				err = json.Unmarshal(resp, &errResponse)
				require.NoError(t, err)
				require.Equal(t, testcase.expectedErr.Error(), errResponse.Error.Message)

				return
			}

			var resultString string

			if testcase.isBatch {
				var batchResponse []SuccessResponse
				err = json.Unmarshal(resp, &batchResponse)
				require.NoError(t, err)

				require.Len(t, batchResponse, 2)

				// just match second filter id
				err = json.Unmarshal(batchResponse[1].Result, &resultString)
				require.NoError(t, err)
			} else {
				var successResponse SuccessResponse
				err = json.Unmarshal(resp, &successResponse)
				require.NoError(t, err)

				err = json.Unmarshal(successResponse.Result, &resultString)
				require.NoError(t, err)
			}

			require.Equal(t, mockConnManager.GetFilterID(), resultString)
		})
	}
}

func Test_handle(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	err := d.registerService("test", rpcargs.MockRegisterValidMethod())
	require.NoError(t, err)

	testcases := []struct {
		name         string
		requestBody  []byte
		expectedResp rpcargs.MockMethodData
		expectedErr  error
		isBatch      bool // Indicate if the response is expected to be a batch
	}{
		{
			name: "Handle valid single json-rpc request",
			requestBody: func() []byte {
				req := Request{
					ID:     "1",
					Method: "test.MockMethodWithResp",
					Params: json.RawMessage(`[]`),
				}
				reqBody, _ := json.Marshal(req)

				return reqBody
			}(),
			expectedResp: rpcargs.MockMethodData{
				ID:   1,
				Name: "mockMethodData",
			},
		},
		{
			name: "Handle valid batch json-rpc request",
			requestBody: func() []byte {
				reqs := []Request{
					{
						ID:     "1",
						Method: "test.MockMethodWithResp",
						Params: json.RawMessage(`[]`),
					},
					{
						ID:     "2",
						Method: "test.MockMethodWithResp",
						Params: json.RawMessage(`[]`),
					},
				}
				reqBody, _ := json.Marshal(reqs)

				return reqBody
			}(),
			expectedResp: rpcargs.MockMethodData{
				ID:   1,
				Name: "mockMethodData",
			},
			isBatch: true,
		},
		{
			name:        "Invalid request body",
			requestBody: []byte(`{invalid json`),
			expectedErr: errors.New("invalid json request"),
		},
		{
			name: "Exceed batch length limit for json-rpc requests",
			requestBody: func() []byte {
				reqs := make([]Request, 21) // Batch limit is 20
				for i := range reqs {
					reqs[i] = Request{
						ID:     fmt.Sprintf("%d", i),
						Method: "test.MockMethodWithResp",
						Params: json.RawMessage(`[]`),
					}
				}
				reqBody, _ := json.Marshal(reqs)

				return reqBody
			}(),
			expectedErr: errors.New("batch request length too long"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			resp, err := d.handle(testcase.requestBody)
			require.NoError(t, err)

			if testcase.expectedErr != nil {
				var errResponse ErrorResponse
				err = json.Unmarshal(resp, &errResponse)
				require.NoError(t, err)
				require.Equal(t, testcase.expectedErr.Error(), errResponse.Error.Message)

				return
			}

			var result rpcargs.MockMethodData

			if testcase.isBatch {
				var batchResponse []SuccessResponse
				err = json.Unmarshal(resp, &batchResponse)
				require.NoError(t, err)

				require.Len(t, batchResponse, 2)

				// both response are same
				err = json.Unmarshal(batchResponse[1].Result, &result)
				require.NoError(t, err)
			} else {
				var successResponse SuccessResponse
				err = json.Unmarshal(resp, &successResponse)
				require.NoError(t, err)

				err = json.Unmarshal(successResponse.Result, &result)
				require.NoError(t, err)
			}

			require.Equal(t, testcase.expectedResp, result)
		})
	}
}

func Test_registerService(t *testing.T) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	d := newDispatcher(logger, cfg, filterMan)

	testcases := []struct {
		name        string
		serviceName string
		service     interface{}
		expectedErr error
	}{
		{
			name:        "fail to register empty service name",
			serviceName: "", //
			expectedErr: common.ErrEmptyServiceName,
		},
		{
			name:        "fail to register service of  non-struct type",
			serviceName: "test",
			service:     struct{}{},
			expectedErr: errors.New("jsonrpc: service 'test' must be a pointer to struct"),
		},
		{
			name:        "register valid service",
			serviceName: "test",
			service:     rpcargs.MockRegisterValidMethod(),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := d.registerService(testcase.serviceName, testcase.service)

			if testcase.expectedErr != nil {
				require.Equal(t, testcase.expectedErr, err)

				return
			}
		})
	}
}

func Test_validateMethod(t *testing.T) {
	validMethod := rpcargs.MockRegisterValidMethod()
	invalidMethod := rpcargs.MockRegisterInvalidMethod()

	validMethodValue := reflect.ValueOf(validMethod.MockMethodWithResp)

	testcases := []struct {
		name        string
		methodName  string
		methodValue reflect.Value
		inNum       int
		reqt        []reflect.Type
		expectedErr error
	}{
		{
			name:        "failed to validate empty method name",
			methodName:  "",
			expectedErr: common.ErrEmptyMethodName,
		},
		{
			name:        "failed to validate invalid method value",
			methodName:  "methodName",
			methodValue: reflect.ValueOf(""),
			expectedErr: errors.New("'methodName' must be a method instead of string"),
		},
		{
			name:        "invalid number of method output arguments",
			methodName:  "MockMethodWithOnlyResp",
			methodValue: reflect.ValueOf(invalidMethod.MockMethodWithOnlyResp),
			expectedErr: errors.New("unexpected number of output arguments in the method 'MockMethodWithOnlyResp': 1. Expected 2"), //nolint:lll
		},
		{
			name:        "second param of method output is invalid type",
			methodName:  "MockMethodWithNoError",
			methodValue: reflect.ValueOf(invalidMethod.MockMethodWithNoError),
			expectedErr: errors.New("unexpected type for the second return value of the method 'MockMethodWithNoError': '*args.MockInvalidMethodData'. Expected 'error'"), //nolint:lll
		},
		{
			name:        "valid method",
			methodName:  "MockMethodWithResp",
			methodValue: validMethodValue,
			inNum:       1, // number of input params
			reqt: []reflect.Type{
				validMethodValue.Type().In(0),
			}, // request type of first input param
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			inNum, reqt, err := validateMethod(testcase.methodName, testcase.methodValue, true)

			if testcase.expectedErr != nil {
				require.Equal(t, testcase.expectedErr, err)

				return
			}

			require.Equal(t, testcase.inNum, inNum)
			require.Equal(t, testcase.reqt, reqt)
		})
	}
}

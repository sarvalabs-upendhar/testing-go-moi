package websocket

import (
	"encoding/json"
)

// WSRequest is a jsonrpc request
type WSRequest struct {
	ID     interface{}     `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// TesseractArgs is the jsonrpc request parameter for new tesseracts event filter
type TesseractArgs struct {
	Address string `json:"address"`
}

// ResponseError is a jsonrpc error
type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RPCResponse is a jsonrpc success response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// NewRPCResponse is used to create a custom error response
func NewRPCResponse(id interface{}, errCode int, err string, jsonRPCVersion string) ([]byte, error) {
	errObject := &ResponseError{errCode, err, nil}

	response := &RPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   errObject,
	}

	return json.Marshal(response)
}

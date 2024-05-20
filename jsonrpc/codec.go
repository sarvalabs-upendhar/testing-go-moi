package jsonrpc

import (
	"encoding/json"
)

// TesseractArgs is the jsonrpc request parameter for new tesseracts event filter
type TesseractArgs struct {
	Address string `json:"address"`
}

// Request is a jsonrpc request
type Request struct {
	ID     interface{}     `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type BatchRequest []Request

// Response is a jsonrpc response interface
type Response interface {
	GetID() interface{}
	Data() json.RawMessage
	Bytes() ([]byte, error)
}

// ErrorResponse is a jsonrpc error response
type ErrorResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      interface{}    `json:"id,omitempty"`
	Error   *ResponseError `json:"error"`
}

// GetID returns error response id
func (e *ErrorResponse) GetID() interface{} {
	return e.ID
}

// Data returns ObjectError
func (e *ErrorResponse) Data() json.RawMessage {
	data, err := json.Marshal(e.Error)
	if err != nil {
		return json.RawMessage(err.Error())
	}

	return data
}

// Bytes return the serialized response
func (e *ErrorResponse) Bytes() ([]byte, error) {
	return json.Marshal(e)
}

// SuccessResponse is a jsonrpc  success response
type SuccessResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// GetID returns success response id
func (s *SuccessResponse) GetID() interface{} {
	return s.ID
}

// Data returns the result
func (s *SuccessResponse) Data() json.RawMessage {
	if s.Result != nil {
		return s.Result
	}

	return json.RawMessage("No Data")
}

// Bytes return the serialized response
func (s *SuccessResponse) Bytes() ([]byte, error) {
	return json.Marshal(s)
}

// ResponseError is a jsonrpc error
type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewRPCErrorResponse is used to create a custom error response
func NewRPCErrorResponse(id interface{}, errCode int, err string, data []byte, jsonrpcver string) Response {
	errObject := &ResponseError{errCode, err, data}

	response := &ErrorResponse{
		JSONRPC: jsonrpcver,
		ID:      id,
		Error:   errObject,
	}

	return response
}

// NewRPCResponse returns Success/Error response object
func NewRPCResponse(id interface{}, jsonrpcver string, reply []byte, err Error) Response {
	var response Response
	switch err.(type) {
	case nil:
		response = &SuccessResponse{JSONRPC: jsonrpcver, ID: id, Result: reply}
	default:
		response = NewRPCErrorResponse(id, err.ErrorCode(), err.Error(), reply, jsonrpcver)
	}

	return response
}

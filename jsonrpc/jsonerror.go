package jsonrpc

import "fmt"

type Error interface {
	Error() string
	ErrorCode() int
}

type JSONError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (err JSONError) Error() string {
	if err.Message == "" {
		return fmt.Sprintf("json-rpc error %d", err.Code)
	}

	return err.Message
}

func (err JSONError) ErrorCode() int {
	return err.Code
}

type invalidRequestError struct {
	err string
}

func (e invalidRequestError) Error() string {
	return e.err
}

func (e invalidRequestError) ErrorCode() int {
	return -32600
}

func NewInvalidRequestError(msg string) *invalidRequestError {
	return &invalidRequestError{msg}
}

type internalError struct {
	err string
}

func (e internalError) Error() string {
	return e.err
}

func (e internalError) ErrorCode() int {
	return -32603
}

func NewInternalError(msg string) *internalError {
	return &internalError{msg}
}

type invalidParamsError struct {
	err string
}

func (e invalidParamsError) Error() string {
	return e.err
}

func (e invalidParamsError) ErrorCode() int {
	return -32602
}

func NewInvalidParamsError(msg string) *invalidParamsError {
	return &invalidParamsError{msg}
}

type methodNotFoundError struct {
	err string
}

func (e methodNotFoundError) Error() string {
	return e.err
}

func (e methodNotFoundError) ErrorCode() int {
	return -32601
}

func NewMethodNotFoundError(method string) *methodNotFoundError {
	return &methodNotFoundError{fmt.Sprintf("the method %s does not exist/is not available", method)}
}

type subscriptionNotFoundError struct {
	err string
}

func (e *subscriptionNotFoundError) Error() string {
	return e.err
}

func (e *subscriptionNotFoundError) ErrorCode() int {
	return -32601
}

func NewSubscriptionNotFoundError(method string) *subscriptionNotFoundError {
	return &subscriptionNotFoundError{fmt.Sprintf("subscribe method %s not found", method)}
}

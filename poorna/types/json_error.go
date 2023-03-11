package types

import "fmt"

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

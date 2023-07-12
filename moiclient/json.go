package moiclient

import (
	"encoding/json"

	"github.com/sarvalabs/go-moi/jsonrpc/args"
)

const (
	vsn = "2.0"
)

// A value of this type can a JSON-RPC request, notification, successful response or
// error response.
type jsonrpcMessage struct {
	Version string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *args.JSONError `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func (msg *jsonrpcMessage) String() string {
	b, _ := json.Marshal(msg)

	return string(b)
}

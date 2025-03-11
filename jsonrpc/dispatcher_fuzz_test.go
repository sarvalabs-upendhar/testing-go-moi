package jsonrpc

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common/utils"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/stretchr/testify/assert"
)

func FuzzDispatcherWsUnsubscribe(f *testing.F) {
	eventMux := new(utils.TypeMux)
	logger := hclog.NewNullLogger()
	filterMan := NewFilterManager(logger, eventMux, &rpcargs.MockJSONRPCConfig, nil)

	cfg := rpcargs.MockConfig()

	// Create a new mock dispatcher
	dispatcher := newDispatcher(logger, cfg, filterMan)

	// Create a mock connection manager
	mockConnManager := NewMockConnectionManager()

	seeds := []string{
		`{"method": "moi.Unsubscribe", "params": ["12345"]}`,
		`{"method": "moi.Subscribe", "params": ["newTesseracts"]}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, request string) {
		_, err := dispatcher.handleWs([]byte(request), mockConnManager)
		assert.NoError(t, err)
	})
}

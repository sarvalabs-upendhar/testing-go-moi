package api

import (
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-moi/jsonrpc/websocket"
)

// API describes the set of methods offered over the RPC interface
type API struct {
	Namespace string                 // namespace under which the rpc methods of Service are exposed
	Services  map[string]interface{} // holds the list of api instances
}

type PublicAPI struct {
	IxAPI     *PublicIXAPI
	IxPoolAPI *PublicIXPoolAPI
	CoreAPI   *PublicCoreAPI
	NetAPI    *PublicNetAPI
	DebugAPI  *PublicDebugAPI
}

func NewPublicAPI(backend *backend.Backend, filterMan *websocket.FilterManager) *PublicAPI {
	return &PublicAPI{
		IxAPI:     NewPublicIXAPI(backend.Ixpool, backend.SM),
		IxPoolAPI: NewPublicIXPoolAPI(backend.Ixpool),
		CoreAPI:   NewPublicCoreAPI(backend.Ixpool, backend.Chain, backend.SM, backend.Exec, backend.Syncer, filterMan),
		NetAPI:    NewPublicNetAPI(backend.Net),
		DebugAPI:  NewPublicDebugAPI(backend.DB, backend.Net),
	}
}

func GetPublicAPIs(backend *backend.Backend, filterMan *websocket.FilterManager) []API {
	publicAPI := NewPublicAPI(backend, filterMan)

	return []API{
		{
			Namespace: "moi",
			Services: map[string]interface{}{
				"ix":   publicAPI.IxAPI,
				"core": publicAPI.CoreAPI,
			},
		},
		{
			Namespace: "ixpool",
			Services: map[string]interface{}{
				"ixpool": publicAPI.IxPoolAPI,
			},
		},
		{
			Namespace: "net",
			Services: map[string]interface{}{
				"net": publicAPI.NetAPI,
			},
		},
		{
			Namespace: "debug",
			Services: map[string]interface{}{
				"debug": publicAPI.DebugAPI,
			},
		},
	}
}

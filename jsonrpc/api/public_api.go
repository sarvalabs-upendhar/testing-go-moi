package api

import (
	"github.com/sarvalabs/go-moi/jsonrpc"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
)

// API describes the set of methods offered over the RPC interface
type API struct {
	Namespace string      // namespace under which the rpc methods of Service are exposed
	Services  interface{} // holds the list of api instances
}

type PublicAPI struct {
	IxPoolAPI *PublicIXPoolAPI
	CoreAPI   *PublicCoreAPI
	NetAPI    *PublicNetAPI
	DebugAPI  *PublicDebugAPI
}

func NewPublicAPI(backend *backend.Backend, filterMan *jsonrpc.FilterManager) *PublicAPI {
	return &PublicAPI{
		IxPoolAPI: NewPublicIXPoolAPI(backend.Ixpool),
		CoreAPI:   NewPublicCoreAPI(backend.Ixpool, backend.Chain, backend.SM, backend.Exec, backend.Syncer, filterMan),
		NetAPI:    NewPublicNetAPI(backend.Net),
		DebugAPI:  NewPublicDebugAPI(backend.Ixpool, backend.DB, backend.Net, backend.Syncer),
	}
}

func GetPublicAPIs(backend *backend.Backend, filterMan *jsonrpc.FilterManager) []API {
	publicAPI := NewPublicAPI(backend, filterMan)

	return []API{
		{
			Namespace: "moi",
			Services:  publicAPI.CoreAPI,
		},
		{
			Namespace: "ixpool",
			Services:  publicAPI.IxPoolAPI,
		},
		{
			Namespace: "net",
			Services:  publicAPI.NetAPI,
		},
		{
			Namespace: "debug",
			Services:  publicAPI.DebugAPI,
		},
	}
}

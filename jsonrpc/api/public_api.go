package api

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

func NewPublicAPI(backend *Backend) *PublicAPI {
	return &PublicAPI{
		IxAPI:     NewPublicIXAPI(backend.ixpool, backend.sm),
		IxPoolAPI: NewPublicIXPoolAPI(backend.ixpool),
		CoreAPI:   NewPublicCoreAPI(backend.ixpool, backend.chain, backend.sm, backend.exec),
		NetAPI:    NewPublicNetAPI(backend.net),
		DebugAPI:  NewPublicDebugAPI(backend.db, backend.net),
	}
}

func GetPublicAPIs(backend *Backend) []API {
	publicAPI := NewPublicAPI(backend)

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

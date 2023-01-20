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
}

func NewPublicAPI(backend *Backend) *PublicAPI {
	return &PublicAPI{
		IxAPI:     NewPublicIXAPI(backend.ixpool, backend.sm),
		IxPoolAPI: NewPublicIXPoolAPI(backend.ixpool),
		CoreAPI:   NewPublicCoreAPI(backend.ixpool, backend.chain, backend.sm),
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
	}
}

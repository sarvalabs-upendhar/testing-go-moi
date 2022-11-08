package api

type PublicAPI struct {
	IxAPI   *PublicIXAPI
	CoreAPI *PublicCoreAPI
}

func NewPublicAPI(backend *Backend) *PublicAPI {
	return &PublicAPI{
		IxAPI:   NewPublicIXAPI(backend.ixpool, backend.sm, backend.cfg),
		CoreAPI: NewPublicCoreAPI(backend.chain, backend.sm),
	}
}

func GetPublicAPIs(backend *Backend) map[string]interface{} {
	publicAPI := NewPublicAPI(backend)

	apis := make(map[string]interface{})

	apis["ix"] = publicAPI.IxAPI
	apis["core"] = publicAPI.CoreAPI

	return apis
}

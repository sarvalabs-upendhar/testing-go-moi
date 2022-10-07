package rpc

import (
	"errors"
	"net/http"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/poorna/api"
)

// rpcService is a struct that represents a mapping of RPC service APIs
type rpcService struct {
	apis map[string]interface{}
}

// NewRPCService is a constructor function that generates and returns an rpcServer object
func NewRPCService() *rpcService {
	// Create the rpcServer struct and return it
	return &rpcService{apis: make(map[string]interface{})}
}

// RegisterAPI is a method of rpcService that registers a new API to it
func (r *rpcService) RegisterAPIs(apis map[string]interface{}) error {
	for name, api := range apis {
		// Return an error if the API is already registered
		if _, exists := r.apis[name]; exists {
			return errors.New("api already registered")
		}

		// Add the API method to the mapping
		r.apis[name] = api
	}

	return nil
}

// GetLatestTesseract is a method of rpcService that retrieves the latest Tesseract.
// Expects a GetTesseract argument and returns TesseractArg wrapped in a Response.
func (r *rpcService) GetLatestTesseract(req *http.Request, args *api.TesseractArgs, resp *api.Response) error {
	// Retrieve the public core API and call the method to get the latest Tesseract
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	// Retrieve the latest Tesseract for the address from the backend chain manager
	tesseract, err := coreAPI.GetLatestTesseract(args)
	if err != nil {
		return err
	}
	// Wrap the TesseractArg in a Response
	resp.Data = api.NewTesseractArg(tesseract)

	return nil
}

func (r *rpcService) GetTesseractByHash(req *http.Request, args *api.TesseractByHashArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetTesseractByHash(args)

	if err != nil {
		return err
	}

	resp.Data = api.NewTesseractArg(tesseract)

	return nil
}

func (r *rpcService) GetTesseractByHeight(
	req *http.Request,
	args *api.TesseractByHeightArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetTesseractByHeight(args)

	if err != nil {
		return err
	}

	resp.Data = api.NewTesseractArg(tesseract)

	return nil
}

func (r *rpcService) GetAssetInfoByAssetID(req *http.Request, args *api.AssetInfoArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	assetInfo, err := coreAPI.GetAssetInfoByAssetID(args.AssetID)
	if err != nil {
		return err
	}

	resp.Data = assetInfo

	return nil
}

// GetBalance is a method of rpcService that retrieves the balance.
// Expects a GetBalArgs argument and returns an uint64 wrapped in a Response.
func (r *rpcService) GetBalance(req *http.Request, args *api.BalArgs, resp *api.Response) error {
	// Retrieve the public core API and call the method to get the balance for the asset
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	bal, err := coreAPI.GetBalance(args)
	if err != nil {
		return err
	}

	// Wrap the balance in a Response after casting to a u64
	resp.Data = bal.Uint64()

	return nil
}

// SendInteractions is a method of rpcService that sends Interactions
func (r *rpcService) SendInteractions(req *http.Request, args *api.SendIXArgs, resp *api.Response) error {
	// Retrieve the public ix API
	ixAPI, ok := r.apis["ix"].(*api.PublicIXAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	ixn, err := ixAPI.SendInteraction(args)

	if err != nil {
		return err
	}

	resp.Data = ixn[0].GetIxHash().Hex()

	return nil
}

// GetTDU is an RPC method that returns the TDU of the queried address
func (r *rpcService) GetTDU(req *http.Request, args *api.TesseractArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	data, err := coreAPI.GetTDU(args)
	if err != nil {
		return err
	}

	resp.Data = data

	return nil
}

// GetContextInfo is an RPC method that returns the context Info of the queried address
func (r *rpcService) GetContextInfo(req *http.Request, args *api.TesseractArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	behaviour, observer, err := coreAPI.GetContextInfo(args)
	if err != nil {
		return err
	}

	var response api.ContextResponse

	response.BehaviourNodes = behaviour
	response.RandomNodes = observer
	response.StorageNodes = make([]string, 0)
	resp.Data = response

	return nil
}

// GetInteractionReceipt returns the receipt of the interaction
func (r *rpcService) GetInteractionReceipt(req *http.Request, args *api.ReceiptArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	receipt, err := coreAPI.GetInteractionReceipt(args)
	if err != nil {
		return err
	}

	resp.Data = receipt

	return nil
}

func (r *rpcService) GetInteractionCountByAddress(
	req *http.Request,
	args *api.InteractionCountArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	interactionCount, err := coreAPI.GetInteractionCountByAddress(args)
	if err != nil {
		return err
	}

	resp.Data = interactionCount

	return nil
}

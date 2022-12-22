package rpc

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/poorna/api"
	"github.com/sarvalabs/moichain/types"
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

// RegisterAPIs is a method of rpcService that registers a new API to it
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
		return types.ErrInvalidAPI
	}

	// Retrieve the latest Tesseract for the address from the backend lattice manager
	tesseract, err := coreAPI.GetLatestTesseract(args)
	if err != nil {
		return err
	}
	// Wrap the TesseractArg in a Response
	resp.Data = api.NewTesseractArg(tesseract, args.WithInteractions)

	return nil
}

func (r *rpcService) GetTesseractByHash(req *http.Request, args *api.TesseractByHashArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetTesseractByHash(args)
	if err != nil {
		return err
	}

	resp.Data = api.NewTesseractArg(tesseract, args.WithInteractions)

	return nil
}

func (r *rpcService) GetTesseractByHeight(
	req *http.Request,
	args *api.TesseractByHeightArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetTesseractByHeight(args)
	if err != nil {
		return err
	}

	resp.Data = api.NewTesseractArg(tesseract, args.WithInteractions)

	return nil
}

func (r *rpcService) GetAssetInfoByAssetID(req *http.Request, args *api.AssetDescriptorArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
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
		return types.ErrInvalidAPI
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
		return types.ErrInvalidAPI
	}

	ixn, err := ixAPI.SendInteraction(args)
	if err != nil {
		return err
	}

	ixHash := ixn.Hash()
	resp.Data = ixHash.Hex()

	return nil
}

// GetTDU is an RPC method that returns the TDU of the queried address
func (r *rpcService) GetTDU(req *http.Request, args *api.TesseractArgs, resp *api.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	data, err := coreAPI.GetTDU(args)
	if err != nil {
		return err
	}

	resp.Data = data

	return nil
}

// GetContextInfoByHash is an RPC method that returns the context Info of the queried address
func (r *rpcService) GetContextInfoByHash(
	req *http.Request,
	args *api.ContextInfoByHashArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	behaviour, observer, err := coreAPI.GetContextInfoByHash(args)
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
		return types.ErrInvalidAPI
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
		return types.ErrInvalidAPI
	}

	interactionCount, err := coreAPI.GetInteractionCountByAddress(args)
	if err != nil {
		return err
	}

	resp.Data = interactionCount

	return nil
}

func (r *rpcService) GetStorage(
	req *http.Request,
	args *api.GetStorageArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	storageData, err := coreAPI.GetStorageAt(args)
	if err != nil {
		return err
	}

	doc := make(map[types.Address]uint64)
	if err = polo.Depolorize(&doc, storageData); err != nil {
		panic(err)
	}

	fmt.Println(doc)

	resp.Data = storageData

	return nil
}

func (r *rpcService) GetAccountState(req *http.Request,
	args *api.GetAccountArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	account, err := coreAPI.GetAccountState(args)
	if err != nil {
		return err
	}

	resp.Data = account

	return nil
}

func (r *rpcService) GetLogicManifest(
	req *http.Request,
	args *api.GetLogicManifestArgs,
	resp *api.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	manifest, err := coreAPI.GetLogicManifest(args)
	if err != nil {
		return err
	}

	resp.Data = manifest

	return nil
}

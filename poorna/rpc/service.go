package rpc

import (
	"errors"
	"net/http"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
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
func (r *rpcService) RegisterAPI(name string, api interface{}) error {
	// Return an error if the API is already registered
	if _, exists := r.apis[name]; exists {
		return errors.New("api already registered")
	}

	// Add the API method to the mapping
	r.apis[name] = api

	return nil
}

// GetLatestTesseract is a method of rpcService that retrieves the latest Tesseract.
// Expects a GetTesseract argument and returns TesseractArg wrapped in a Response.
func (r *rpcService) GetLatestTesseract(req *http.Request, args *GetTesseract, resp *Response) error {
	// Retrieve the public core API and call the method to get the latest Tesseract
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetLatestTesseract(ktypes.BytesToAddress(kutils.HexToByte(args.From)))
	if err != nil {
		return err
	}

	// Wrap the TesseractArg in a Response
	resp.Data = TesseractResponse(tesseract)

	return nil
}

func (r *rpcService) GetTesseractByHash(req *http.Request, args *GetTesseractByHashArgs, resp *Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetTesseractByHash(args.Hash)
	if err != nil {
		return err
	}

	resp.Data = TesseractResponse(tesseract)

	return nil
}

func (r *rpcService) GetTesseractByHeight(req *http.Request, args *GetTesseractByHeightArgs, resp *Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	tesseract, err := coreAPI.GetTesseractByHeight(args.From, args.Height)
	if err != nil {
		return err
	}

	resp.Data = TesseractResponse(tesseract)

	return nil
}

func (r *rpcService) GetAssetInfoByAssetID(req *http.Request, args *GetAssetInfoArgs, resp *Response) error {
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
func (r *rpcService) GetBalance(req *http.Request, args *GetBalArgs, resp *Response) error {
	// Retrieve the public core API and call the method to get the balance for the asset
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	bal, err := coreAPI.GetBalance(ktypes.BytesToAddress(kutils.HexToByte(args.From)), args.AssetID)
	if err != nil {
		return err
	}

	// Wrap the balance in a Response after casting to a u64
	resp.Data = bal.Uint64()

	return nil
}

// SendInteractions is a method of rpcService that sends Interactions
func (r *rpcService) SendInteractions(req *http.Request, args *SendIXArgs, resp *Response) error {
	// Retrieve the public ixpool API
	ixPoolAPI, ok := r.apis["ixpool"].(*api.PublicIXPoolAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}
	// Construct the interactions data
	ixns := make(ktypes.Interactions, 1)
	ixns[0] = new(ktypes.Interaction)

	nonce, err := ixPoolAPI.GetLatestNonce(ktypes.HexToAddress(args.From))
	if err != nil {
		resp.Status = err.Error()

		return err
	}

	ixns[0].Data = ktypes.IxData{
		Input: ktypes.InteractionInput{
			Type:     args.IxType,
			Nonce:    nonce,
			From:     ktypes.HexToAddress(args.From),
			To:       ktypes.HexToAddress(args.To),
			AnuPrice: args.AnuPrice,
		},
	}

	switch args.IxType {
	case 0:
		ixns[0].Data.Input.TransferValue = map[ktypes.AssetID]uint64{ktypes.AssetID(args.AssetID): uint64(args.Value)}

	case 1:
		ixns[0].Data.Input.Payload = ktypes.InteractionInputPayload{
			AssetData: ktypes.AssetDataInput{
				Dimension:   args.AssetCreation.Dimension,
				TotalSupply: args.AssetCreation.TotalSupply,
				Symbol:      args.AssetCreation.Symbol,
				IsFungible:  args.AssetCreation.IsFungible,
				IsMintable:  args.AssetCreation.IsMintable,
				Code:        args.AssetCreation.Code,
			},
		}
	default:
		return errors.New("invalid interaction type")
	}

	resp.Data = ixns[0].GetIxHash().Hex()

	// Call the API method to add interactions to pool
	return ixPoolAPI.AddIXs(ixns)[0]
}

// GetTDU is an RPC method that returns the TDU of the queried address
func (r *rpcService) GetTDU(req *http.Request, args *GetTesseract, resp *Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	object, err := coreAPI.TDU(ktypes.BytesToAddress(kutils.HexToByte(args.From)))
	if err != nil {
		return err
	}

	data, _ := object.TDU()

	resp.Data = data

	return nil
}

// GetContextInfo is an RPC method that returns the context Info of the queried address
func (r *rpcService) GetContextInfo(req *http.Request, args *GetTesseract, resp *Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	behaviour, observer, err := coreAPI.GetContextInfo(ktypes.BytesToAddress(kutils.HexToByte(args.From)))
	if err != nil {
		return err
	}

	var response ContextResponse

	response.BehaviourNodes = behaviour
	response.RandomNodes = observer
	response.StorageNodes = make([]string, 0)
	resp.Data = response

	return nil
}

// GetInteractionReceipt returns the receipt of the interaction
func (r *rpcService) GetInteractionReceipt(req *http.Request, args *GetReceiptArgs, resp *Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	receipt, err := coreAPI.GetInteractionReceipt(ktypes.HexToAddress(args.Address), ktypes.HexToHash(args.Hash))
	if err != nil {
		return err
	}

	resp.Data = receipt

	return nil
}

func (r *rpcService) GetTransactionCountByAddress(req *http.Request,
	args *GetTransactionCountByAddressArgs, resp *Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return ktypes.ErrInvalidAPI
	}

	transactionCount, err := coreAPI.GetTransactionCountByAddress(args.From, args.Status)
	if err != nil {
		return err
	}

	resp.Data = transactionCount

	return nil
}

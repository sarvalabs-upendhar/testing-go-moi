package rpc

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sarvalabs/moichain/poorna/api"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Service is a struct that represents a mapping of RPC service APIs
type Service struct {
	apis map[string]interface{}
}

// NewRPCService is a constructor function that generates and returns an rpcServer object
func NewRPCService() *Service {
	// Create the rpcServer struct and return it
	return &Service{apis: make(map[string]interface{})}
}

// RegisterAPIs is a method of Service that registers a new API to it
func (r *Service) RegisterAPIs(apis map[string]interface{}) error {
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

/* RPC methods that are associated with the core namespace. */

// Tesseract is a method of Service that retrieves the latest Tesseract.
// Expects a GetTesseract argument and returns types.Tesseract wrapped in a Response.
func (r *Service) Tesseract(req *http.Request, args *ptypes.TesseractArgs, resp *ptypes.Response) error {
	// Retrieve the public core API and call the method to get the latest Tesseract
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	// Retrieve the latest Tesseract for the address from the backend lattice manager
	tesseract, err := coreAPI.GetRPCTesseract(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	// Convert the Tesseract into bytes
	resp.Data, err = json.Marshal(tesseract)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// AssetInfoByAssetID returns the asset information associated with the assetID
func (r *Service) AssetInfoByAssetID(req *http.Request, args *ptypes.GetAssetInfoArgs, resp *ptypes.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	assetInfo, err := coreAPI.GetAssetInfoByAssetID(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(assetInfo)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

func (r *Service) Registry(req *http.Request, args *ptypes.QueryArgs, resp *ptypes.Response) error {
	// Retrieve the public core API and call the method to get the balance for the asset
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	entries, err := coreAPI.GetRegistry(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(entries)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Balance is a method of Service that retrieves the balance.
// Expects BalArgs as argument and returns an uint64 wrapped in a Response.
func (r *Service) Balance(req *http.Request, args *ptypes.BalArgs, resp *ptypes.Response) error {
	// Retrieve the public core API and call the method to get the balance for the asset
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	bal, err := coreAPI.GetBalance(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	// Wrap the balance in a Response after casting to hexadecimal
	resp.Data, err = json.Marshal(bal)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// TDU is an RPC method that returns the TDU of the queried address
func (r *Service) TDU(req *http.Request, args *ptypes.QueryArgs, resp *ptypes.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	tdu, err := coreAPI.GetTDU(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(tdu)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// ContextInfo is an RPC method that returns the context Info of the queried address
func (r *Service) ContextInfo(
	req *http.Request,
	args *ptypes.ContextInfoArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	behaviourSet, observerSet, err := coreAPI.GetContextInfo(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	response := ptypes.ContextResponse{
		BehaviourNodes: behaviourSet,
		RandomNodes:    observerSet,
		StorageNodes:   make([]string, 0),
	}

	resp.Data, err = json.Marshal(response)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// InteractionByTesseract returns interaction for the given tesseract hash
func (r *Service) InteractionByTesseract(
	req *http.Request,
	args *ptypes.InteractionByTesseract,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	interaction, err := coreAPI.GetInteractionByTesseract(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(interaction)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// InteractionByHash returns the interaction for the given interaction hash
func (r *Service) InteractionByHash(
	req *http.Request,
	args *ptypes.InteractionByHashArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	interaction, err := coreAPI.GetInteractionByHash(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(interaction)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// InteractionReceipt returns the receipt of the interaction
func (r *Service) InteractionReceipt(req *http.Request, args *ptypes.ReceiptArgs, resp *ptypes.Response) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	rpcReceipt, err := coreAPI.GetInteractionReceipt(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(rpcReceipt)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// LogicIDs returns the logic IDs from the logic tree for the given address
func (r *Service) LogicIDs(
	req *http.Request,
	args *ptypes.GetAccountArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	logicIDs, err := coreAPI.GetLogicIDs(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(logicIDs)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// InteractionCount returns the number of interactions sent for the given address
func (r *Service) InteractionCount(
	req *http.Request,
	args *ptypes.InteractionCountArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	interactionCount, err := coreAPI.GetInteractionCount(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(interactionCount)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// PendingInteractionCount returns the number of interactions sent for the given address.
func (r *Service) PendingInteractionCount(
	req *http.Request,
	args *ptypes.InteractionCountArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	interactionCount, err := coreAPI.GetPendingInteractionCount(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(interactionCount)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Storage returns the data associated with the given storage slot
func (r *Service) Storage(
	req *http.Request,
	args *ptypes.GetStorageArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	storageData, err := coreAPI.GetStorageAt(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(storageData)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// AccountState returns the account state of the given address
func (r *Service) AccountState(
	req *http.Request,
	args *ptypes.GetAccountArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	account, err := coreAPI.GetAccountState(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(account)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// LogicManifest returns the manifest associated with the given logic id
func (r *Service) LogicManifest(
	req *http.Request,
	args *ptypes.LogicManifestArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	manifest, err := coreAPI.GetLogicManifest(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(manifest)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// AccountMetaInfo returns the account meta info associated with the given address
func (r *Service) AccountMetaInfo(
	req *http.Request,
	args *ptypes.GetAccountArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	accountInfo, err := coreAPI.AccountMetaInfo(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(accountInfo)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// LogicCall supports call to logics that do not transition state
func (r *Service) LogicCall(
	req *http.Request,
	args *ptypes.LogicCallArgs,
	resp *ptypes.Response,
) error {
	coreAPI, ok := r.apis["core"].(*api.PublicCoreAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	logicCallResult, err := coreAPI.LogicCall(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(logicCallResult)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

/* RPC methods that are associated with the ix namespace. */

// SendInteractions is a method of Service that sends Interactions
func (r *Service) SendInteractions(req *http.Request, args *ptypes.SendIX, resp *ptypes.Response) error {
	// Retrieve the public ix API
	ixAPI, ok := r.apis["ix"].(*api.PublicIXAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	ix, err := ixAPI.SendInteraction(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(ix.Hash())
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

/* RPC methods that are associated with the ixpool namespace. */

// Content is an RPC method that returns the interactions present in the IxPool.
func (r *Service) Content(
	req *http.Request,
	args *ptypes.ContentArgs,
	resp *ptypes.Response,
) error {
	ixPoolAPI, ok := r.apis["ixpool"].(*api.PublicIXPoolAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	content, err := ixPoolAPI.Content()
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(content)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// ContentFrom is an RPC method that returns the interactions present in IxPool for the queried address.
func (r *Service) ContentFrom(
	req *http.Request,
	args *ptypes.IxPoolArgs,
	resp *ptypes.Response,
) error {
	ixPoolAPI, ok := r.apis["ixpool"].(*api.PublicIXPoolAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	content, err := ixPoolAPI.ContentFrom(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(content)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Status is an RPC method that returns the number of pending and queued interactions in the IxPool.
func (r *Service) Status(
	req *http.Request,
	args *ptypes.StatusArgs,
	resp *ptypes.Response,
) error {
	ixPoolAPI, ok := r.apis["ixpool"].(*api.PublicIXPoolAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	status, err := ixPoolAPI.Status()
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(status)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Inspect is an RPC method that returns the interactions present in the IxPool in a clear and easy-to-read format,
// as well as a list of all the accounts in IxPool and their respective wait times.
func (r *Service) Inspect(
	req *http.Request,
	args *ptypes.InspectArgs,
	resp *ptypes.Response,
) error {
	ixPoolAPI, ok := r.apis["ixpool"].(*api.PublicIXPoolAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	data, err := ixPoolAPI.Inspect()
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(data)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// WaitTime is an RPC method that returns the wait time for an account in IxPool, based on the queried address.
func (r *Service) WaitTime(
	req *http.Request,
	args *ptypes.IxPoolArgs,
	resp *ptypes.Response,
) error {
	ixPoolAPI, ok := r.apis["ixpool"].(*api.PublicIXPoolAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	waitTime, err := ixPoolAPI.WaitTime(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(waitTime)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Peers is an RPC Method that returns an array of Krama ID's connected to a client
func (r *Service) Peers(
	req *http.Request,
	args *ptypes.NetArgs,
	resp *ptypes.Response,
) error {
	NetAPI, ok := r.apis["net"].(*api.PublicNetAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	peers, err := NetAPI.Peers()
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(peers)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Version is an RPC Method that returns an the version of the protocol
func (r *Service) Version(
	req *http.Request,
	args *ptypes.NetArgs,
	resp *ptypes.Response,
) error {
	NetAPI, ok := r.apis["net"].(*api.PublicNetAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	version, err := NetAPI.Version()
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(version)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// DBGet is an RPC Method that returns the raw value of the key stored in the database
func (r *Service) DBGet(
	req *http.Request,
	args *ptypes.DebugArgs,
	resp *ptypes.Response,
) error {
	DebugAPI, ok := r.apis["debug"].(*api.PublicDebugAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	key, err := DebugAPI.DBGet(args)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(key)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

// Accounts is an RPC Method that returns the address of all the accounts
func (r *Service) Accounts(
	req *http.Request,
	args *ptypes.AccountArgs,
	resp *ptypes.Response,
) error {
	DebugAPI, ok := r.apis["debug"].(*api.PublicDebugAPI)
	if !ok {
		return types.ErrInvalidAPI
	}

	key, err := DebugAPI.GetAccounts()
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	resp.Data, err = json.Marshal(key)
	if err != nil {
		resp.Error = &ptypes.JSONError{Message: err.Error()}

		return nil
	}

	return nil
}

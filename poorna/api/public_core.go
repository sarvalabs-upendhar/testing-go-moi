package api

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

// PublicCoreAPI is a struct that represents a wrapper for the core public core APIs
type PublicCoreAPI struct {
	// Represents the API backend
	ixpool IxPool
	chain  ChainManager
	sm     StateManager
}

// NewPublicCoreAPI is a constructor function that generates and returns a new
// PublicCoreAPI object for a given API backend object.
func NewPublicCoreAPI(ixpool IxPool, chain ChainManager, sm StateManager) *PublicCoreAPI {
	// Create the core public API wrapper and return it
	return &PublicCoreAPI{ixpool, chain, sm}
}

func getTesseractArgs(address string, options ptypes.TesseractNumberOrHash) *ptypes.TesseractArgs {
	return &ptypes.TesseractArgs{
		From:    address,
		Options: options,
	}
}

// getTesseractByHash returns the tesseract based on the hash given
func (p *PublicCoreAPI) getTesseractByHash(hash string, withInteractions bool) (*types.Tesseract, error) {
	hash, err := utils.ValidateHash(hash)
	if err != nil {
		return nil, err
	}

	return p.chain.GetTesseract(types.HexToHash(hash), withInteractions)
}

// getTesseractByHeight returns the tesseract based on the height given
func (p *PublicCoreAPI) getTesseractByHeight(
	from string,
	height int64,
	withInteractions bool,
) (*types.Tesseract, error) {
	address, err := utils.ValidateAddress(from)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	if height == ptypes.LatestTesseractHeight {
		return p.chain.GetLatestTesseract(address, withInteractions)
	}

	tesseract, err := p.chain.GetTesseractByHeight(address, uint64(height), withInteractions)
	if err != nil {
		return nil, err
	}

	return tesseract, nil
}

// getTesseract returns tesseract using arguments.
func (p *PublicCoreAPI) getTesseract(args *ptypes.TesseractArgs) (*types.Tesseract, error) {
	if args.Options.TesseractHash != nil && args.Options.TesseractNumber != nil {
		return nil, errors.New("can not use both tesseract number and tesseract hash")
	}

	if hash, ok := args.Options.Hash(); ok {
		return p.getTesseractByHash(hash, args.WithInteractions)
	}

	height, err := args.Options.Number()
	if err == nil {
		return p.getTesseractByHeight(args.From, height, args.WithInteractions)
	}

	if errors.Is(err, types.ErrEmptyHeight) {
		return nil, types.ErrEmptyOptions
	}

	return nil, errors.Wrap(err, "invalid options")
}

// GetRPCTesseract returns the rpc tesseract using given arguments
func (p *PublicCoreAPI) GetRPCTesseract(args *ptypes.TesseractArgs) (*ptypes.RPCTesseract, error) {
	ts, err := p.getTesseract(args)
	if err != nil {
		return nil, err
	}

	return createRPCTesseract(ts)
}

// GetContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfo(args *ptypes.ContextInfoArgs) ([]string, []string, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.From, args.Options))
	if err != nil {
		return nil, nil, err
	}

	_, behaviourSet, RandomSet, err := p.sm.GetContextByHash(ts.Address(), ts.ContextHash())
	if err != nil {
		return nil, nil, err
	}

	return utils.KramaIDToString(behaviourSet), utils.KramaIDToString(RandomSet), nil
}

// GetBalance is a method of PublicCoreAPI for retrieving the balance of an address.
// Accepts the address and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) GetBalance(args *ptypes.BalArgs) (*big.Int, error) {
	assetID, err := utils.ValidateAssetID(args.AssetID)
	if err != nil {
		return nil, types.ErrInvalidAssetID
	}

	ts, err := p.getTesseract(getTesseractArgs(args.From, args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetBalance(ts.Address(), assetID, ts.StateHash())
}

// GetTDU will return the total digital utility associated with address
func (p *PublicCoreAPI) GetTDU(args *ptypes.TesseractArgs) (types.AssetMap, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.From, args.Options))
	if err != nil {
		return nil, err
	}

	object, err := p.sm.GetBalances(ts.Address(), ts.StateHash())
	if err != nil {
		return nil, err
	}

	data, _ := object.TDU()

	return data, nil
}

// GetInteractionReceipt returns the receipt for the given interaction hash
func (p *PublicCoreAPI) GetInteractionReceipt(args *ptypes.ReceiptArgs) (*types.Receipt, error) {
	hash, err := utils.ValidateHash(args.Hash)
	if err != nil {
		return nil, err
	}

	return p.chain.GetReceiptByIxHash(types.HexToHash(hash))
}

// GetInteractionCount returns the number of interactions sent for the given address
func (p *PublicCoreAPI) GetInteractionCount(args *ptypes.InteractionCountArgs) (uint64, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.From, args.Options))
	if err != nil {
		return 0, err
	}

	return p.sm.GetNonce(ts.Address(), ts.StateHash())
}

// GetPendingInteractionCount returns the number of interactions sent for the given address.
// Including the pending interactions in IxPool.
func (p *PublicCoreAPI) GetPendingInteractionCount(args *ptypes.InteractionCountArgs) (uint64, error) {
	addr, err := utils.ValidateAddress(args.From)
	if err != nil {
		return 0, err
	}

	interactionCount, err := p.ixpool.GetNonce(addr)
	if err != nil {
		return 0, err
	}

	return interactionCount, nil
}

// GetAccountState returns the account state of the given address
func (p *PublicCoreAPI) GetAccountState(args *ptypes.GetAccountArgs) (*types.Account, error) {
	ts, err := p.getTesseract(getTesseractArgs(args.Address, args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetAccountState(ts.Address(), ts.StateHash())
}

// GetLogicManifest returns the manifest associated with the given logic id
func (p *PublicCoreAPI) GetLogicManifest(args *ptypes.LogicManifestArgs) ([]byte, error) {
	logicID, err := utils.ValidateLogicID(args.LogicID)
	if err != nil {
		return nil, err
	}

	ts, err := p.getTesseract(getTesseractArgs(logicID.Address().Hex(), args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetLogicManifest(logicID, ts.StateHash())
}

// GetStorageAt returns the data associated with the given storage slot
func (p *PublicCoreAPI) GetStorageAt(args *ptypes.GetStorageArgs) ([]byte, error) {
	logicID, err := utils.ValidateLogicID(args.LogicID)
	if err != nil {
		return nil, err
	}

	ts, err := p.getTesseract(getTesseractArgs(logicID.Address().String(), args.Options))
	if err != nil {
		return nil, err
	}

	return p.sm.GetStorageEntry(logicID, types.FromHex(args.StorageKey), ts.StateHash())
}

// GetAssetInfoByAssetID returns the asset info associated with the given asset id
func (p *PublicCoreAPI) GetAssetInfoByAssetID(id string) (*types.AssetDescriptor, error) {
	_, err := utils.ValidateAssetID(id)
	if err != nil {
		return nil, err
	}

	aID, err := hex.DecodeString(id)
	if err != nil {
		return nil, err
	}

	assetInfo := parseAssetMetaInfo(aID)

	assetData, err := p.chain.GetAssetDataByAssetHash(aID[2:])
	if err != nil {
		return nil, err
	}

	assetInfo.Symbol = assetData.Symbol
	assetInfo.Supply = assetData.Supply
	assetInfo.Owner = assetData.Owner
	assetInfo.LogicID = assetData.LogicID

	return assetInfo, nil
}

// helper functions
func parseAssetMetaInfo(aID []byte) *types.AssetDescriptor {
	var dimension, info uint8

	assetInfo := new(types.AssetDescriptor)

	dimension = uint8(big.NewInt(0).SetBytes(aID[:1]).Uint64())
	info = uint8(big.NewInt(0).SetBytes(aID[1:2]).Uint64())

	// extract most significant bit
	if 0x80&info == 0x80 {
		assetInfo.IsFungible = true
	} else {
		assetInfo.IsFungible = false
	}

	// extract least significant bit
	if 0x01&info == 1 {
		assetInfo.IsMintable = true
	} else {
		assetInfo.IsMintable = false
	}

	assetInfo.Dimension = dimension

	return assetInfo
}

// createRPCInteraction creates an RPC Interaction by copying all fields of the interaction into the RPC Interaction,
// depolarizing the payload based on the interaction type, JSON marshalling it, and storing it in the input payload.
func createRPCInteraction(ix *types.Interaction) (*ptypes.RPCInteraction, error) {
	rpcIX := &ptypes.RPCInteraction{
		Input:     ix.Input(),
		Compute:   ix.Compute(),
		Trust:     ix.Trust(),
		Hash:      ix.Hash(),
		Signature: ix.Signature(),
	}

	var err error

	switch ix.Type() {
	case types.IxValueTransfer:

	case types.IxAssetCreate:
		assetPayload := new(types.AssetPayload)
		if err = assetPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcIX.Input.Payload, err = json.Marshal(assetPayload)
		if err != nil {
			return nil, err
		}

	case types.IxLogicDeploy:
		fallthrough

	case types.IxLogicExecute:
		logicPayload := new(types.LogicPayload)

		if err = logicPayload.FromBytes(ix.Payload()); err != nil {
			return nil, err
		}

		rpcIX.Input.Payload, err = json.Marshal(logicPayload)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("invalid interaction type")
	}

	return rpcIX, nil
}

// createRPCTesseract creates rpc tesseract from tesseract
func createRPCTesseract(ts *types.Tesseract) (*ptypes.RPCTesseract, error) {
	var err error

	rpcIxns := make([]*ptypes.RPCInteraction, len(ts.Ixns))

	for i, ixn := range ts.Ixns {
		rpcIxns[i], err = createRPCInteraction(ixn)
		if err != nil {
			return nil, err
		}
	}

	return &ptypes.RPCTesseract{
		Header:   ts.Header,
		Body:     ts.Body,
		Ixns:     rpcIxns,
		Receipts: ts.Receipts,
		Seal:     ts.Seal,
	}, nil
}

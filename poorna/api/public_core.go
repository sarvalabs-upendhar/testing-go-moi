package api

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"

	gtypes "github.com/sarvalabs/moichain/guna/types"

	"github.com/sarvalabs/moichain/utils"

	"github.com/sarvalabs/moichain/types"
)

// PublicCoreAPI is a struct that represents a wrapper for the core public APIs
type PublicCoreAPI struct {
	// Represents the API backend
	chain ChainManager
	sm    StateManager
}

// NewPublicCoreAPI is a constructor function that generates and returns a new
// PublicCoreAPI object for a given API backend object.
func NewPublicCoreAPI(chain ChainManager, sm StateManager) *PublicCoreAPI {
	// Create the core public API wrapper and return it
	return &PublicCoreAPI{chain, sm}
}

// GetBalance is a method of PublicCoreAPI for retrieving the balance of an address.
// Accepts the address and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) GetBalance(args *BalArgs) (*big.Int, error) {
	address, err := utils.ValidateAddress(args.From)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	assetID, err := utils.ValidateAssetID(args.AssetID)
	if err != nil {
		return nil, types.ErrInvalidAssetID
	}

	// Retrieve the state object for the address from the backend state manager and return if an error occurs
	// Get the balance of the asset from the state object and return if an error occurs
	bal, err := p.sm.GetBalance(address, assetID)
	if err != nil {
		return nil, err
	}

	// Return the balance and a nil error
	return bal, nil
}

// GetLatestTesseract is a method of PublicCoreAPI for retrieving the latest Tesseract of an address
func (p *PublicCoreAPI) GetLatestTesseract(args *TesseractArgs) (*types.Tesseract, error) {
	address, err := utils.ValidateAddress(args.From)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	return p.chain.GetLatestTesseract(address, args.WithInteractions)
}

func (p *PublicCoreAPI) GetTesseractByHash(args *TesseractByHashArgs) (*types.Tesseract, error) {
	hash, err := utils.ValidateHash(args.Hash)
	if err != nil {
		return nil, err
	}

	return p.chain.GetTesseract(types.BytesToHash(types.Hex2Bytes(hash)), args.WithInteractions)
}

func (p *PublicCoreAPI) GetTesseractByHeight(args *TesseractByHeightArgs) (*types.Tesseract, error) {
	from, err := utils.ValidateAddress(args.From)
	if err != nil {
		return nil, err
	}

	tesseract, err := p.chain.GetTesseractByHeight(from, args.Height, args.WithInteractions)
	if err != nil {
		return nil, err
	}

	return tesseract, nil
}

func (p *PublicCoreAPI) GetAssetInfoByAssetID(id string) (*types.AssetInfo, error) {
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

	assetInfo.TotalSupply = binary.BigEndian.Uint64(assetData.Extra)

	assetInfo.Owner = assetData.Owner.Hex()

	assetInfo.LogicID = assetData.LogicID

	return assetInfo, nil
}

// GetContextInfoByHash will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfoByHash(args *ContextInfoByHashArgs) ([]string, []string, error) {
	address, err := utils.ValidateAddress(args.From)
	if err != nil {
		return nil, nil, types.ErrInvalidAddress
	}

	hash, err := utils.ValidateHash(args.Hash)
	if err != nil && args.Hash != "" {
		return nil, nil, types.ErrInvalidHash
	}

	_, behaviourSet, RandomSet, err := p.sm.GetContextByHash(address, types.HexToHash(hash))
	if err != nil {
		return nil, nil, err
	}

	return utils.KramaIDToString(behaviourSet), utils.KramaIDToString(RandomSet), nil
}

// GetTDU will return the total digital utility associated with address
func (p *PublicCoreAPI) GetTDU(args *TesseractArgs) (gtypes.AssetMap, error) {
	address, err := utils.ValidateAddress(args.From)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	object, err := p.sm.GetBalances(address)
	if err != nil {
		return nil, err
	}

	data, _ := object.TDU()

	return data, nil
}

func (p *PublicCoreAPI) GetInteractionReceipt(args *ReceiptArgs) (*types.Receipt, error) {
	address, err := utils.ValidateAddress(args.Address)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	hash, err := utils.ValidateHash(args.Hash)
	if err != nil {
		return nil, types.ErrInvalidHash
	}

	return p.chain.GetReceipt(address, types.HexToHash(hash))
}

func (p *PublicCoreAPI) GetInteractionCountByAddress(args *InteractionCountArgs) (uint64, error) {
	addr, err := utils.ValidateAddress(args.From)
	if err != nil {
		return 0, err
	}

	interactionCount, err := p.sm.GetLatestNonce(addr)
	if err != nil {
		return 0, err
	}

	return interactionCount, nil
}

// helper functions
func parseAssetMetaInfo(aID []byte) *types.AssetInfo {
	var dimension, info uint8

	assetInfo := new(types.AssetInfo)

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

package api

import (
	"encoding/hex"
	"math/big"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
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
	address, err := kutils.ValidateAddress(args.From)

	if err != nil {
		return nil, ktypes.ErrInvalidAddress
	}

	assetID, err := kutils.ValidateAssetID(args.AssetID)

	if err != nil {
		return nil, ktypes.ErrInvalidAssetID
	}

	// Retrieve the state object for the address from the backend state manager and return if an error occurs
	// Get the balance of the asset from the state object and return if an error occurs
	bal, err := p.sm.GetBalance(ktypes.HexToAddress(address), ktypes.AssetID(assetID))
	if err != nil {
		return nil, err
	}

	// Return the balance and a nil error
	return bal, nil
}

// GetLatestTesseract is a method of PublicCoreAPI for retrieving the latest Tesseract of an address
func (p *PublicCoreAPI) GetLatestTesseract(args *TesseractArgs) (*ktypes.Tesseract, error) {
	address, err := kutils.ValidateAddress(args.From)
	if err != nil {
		return nil, ktypes.ErrInvalidAddress
	}

	return p.chain.GetLatestTesseract(ktypes.HexToAddress(address))
}
func (p *PublicCoreAPI) GetTesseractByHash(args *TesseractByHashArgs) (*ktypes.Tesseract, error) {
	hash, err := kutils.ValidateHash(args.Hash)
	if err != nil {
		return nil, err
	}

	return p.chain.GetTesseract(ktypes.BytesToHash(ktypes.Hex2Bytes(hash)))
}

func (p *PublicCoreAPI) GetTesseractByHeight(args *TesseractByHeightArgs) (*ktypes.Tesseract, error) {
	from, err := kutils.ValidateAddress(args.From)
	if err != nil {
		return nil, err
	}

	tesseract, err := p.chain.GetTesseractByHeight(from, args.Height)
	if err != nil {
		return nil, err
	}

	return tesseract, nil
}

func (p *PublicCoreAPI) GetAssetInfoByAssetID(assetID string) (*ktypes.AssetInfo, error) {
	assetID, err := kutils.ValidateAssetID(assetID)
	if err != nil {
		return nil, err
	}

	aID, err := hex.DecodeString(assetID)
	if err != nil {
		return nil, err
	}

	assetHash := aID[2:]
	assetInfo := createAsset(aID)
	assetData, err := p.chain.GetAssetDataByAssetHash(assetHash)

	if err != nil {
		return nil, err
	}

	assetInfo.Symbol = assetData.Symbol

	assetInfo.TotalSupply = big.NewInt(0).SetBytes(assetData.Extra).Uint64()

	if err != nil {
		return nil, err
	}

	assetInfo.Owner = assetData.Owner.Hex()

	return assetInfo, nil
}

// GetContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfo(args *TesseractArgs) ([]string, []string, error) {
	address, err := kutils.ValidateAddress(args.From)

	if err != nil {
		return nil, nil, ktypes.ErrInvalidAddress
	}

	_, behaviourSet, RandomSet, err := p.sm.GetLatestContext(ktypes.HexToAddress(address))

	if err != nil {
		return nil, nil, err
	}

	return ktypes.KIPPeerIDToString(behaviourSet), ktypes.KIPPeerIDToString(RandomSet), nil
}

// GetTDU will return the total digital utility associated with address
func (p *PublicCoreAPI) GetTDU(args *TesseractArgs) (ktypes.AssetMap, error) {
	address, err := kutils.ValidateAddress(args.From)

	if err != nil {
		return nil, ktypes.ErrInvalidAddress
	}

	object, err := p.sm.GetBalances(ktypes.HexToAddress(address))
	if err != nil {
		return nil, err
	}

	data, _ := object.TDU()

	return data, nil
}

func (p *PublicCoreAPI) GetInteractionReceipt(args *ReceiptArgs) (*ktypes.Receipt, error) {
	address, err := kutils.ValidateAddress(args.Address)

	if err != nil {
		return nil, ktypes.ErrInvalidAddress
	}

	hash, err := kutils.ValidateHash(args.Hash)

	if err != nil {
		return nil, ktypes.ErrInvalidHash
	}

	return p.chain.GetReceipt(ktypes.HexToAddress(address), ktypes.HexToHash(hash))
}

func (p *PublicCoreAPI) GetInteractionCountByAddress(args *InteractionCountArgs) (uint64, error) {
	addr, err := kutils.ValidateAddress(args.From)
	if err != nil {
		return 0, err
	}

	interactionCount, err := p.sm.GetLatestNonce(ktypes.HexToAddress(addr))
	if err != nil {
		return 0, err
	}

	return interactionCount, nil
}

// helper functions
func createAsset(aID []byte) *ktypes.AssetInfo {
	var dimension, info uint8

	assetInfo := new(ktypes.AssetInfo)

	dimension = uint8(big.NewInt(0).SetBytes(aID[:1]).Uint64())

	info = uint8(big.NewInt(0).SetBytes(aID[1:2]).Uint64())
	//extract most significant bit
	if 0x80&info == 0x80 {
		assetInfo.IsFungible = true
	} else {
		assetInfo.IsFungible = false
	}
	//extract least significant bit
	if 0x01&info == 1 {
		assetInfo.IsMintable = true
	} else {
		assetInfo.IsMintable = false
	}

	assetInfo.Dimension = dimension

	return assetInfo
}

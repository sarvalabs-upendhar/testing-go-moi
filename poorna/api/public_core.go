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
	backend *Backend
}

// NewPublicCoreAPI is a constructor function that generates and returns a new
// PublicCoreAPI object for a given API backend object.
func NewPublicCoreAPI(b *Backend) *PublicCoreAPI {
	// Create the core public API wrapper and return it
	return &PublicCoreAPI{backend: b}
}

// GetBalance is a method of PublicCoreAPI for retrieving the balance of an address.
// Accepts the address and asset for which to retrieve the balance.
// Returns the balance as a big Integer and any error that occurs.
func (p *PublicCoreAPI) GetBalance(addr ktypes.Address, assetID string) (*big.Int, error) {
	// Retrieve the state object for the address from the backend state manager and return if an error occurs
	so, err := p.backend.sm.GetLatestStateObject(addr)
	if err != nil {
		return nil, err
	}

	// Get the balance of the asset from the state object and return if an error occurs
	bal, err := so.BalanceOf(ktypes.AssetID(assetID))
	if err != nil {
		return nil, err
	}

	// Return the balance and a nil error
	return bal, nil
}

// GetLatestTesseract is a method of PublicCoreAPI for retrieving the latest Tesseract of an address
func (p *PublicCoreAPI) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	// Retrieve the latest Tesseract for the address from the backend chain manager
	return p.backend.chain.GetLatestTesseract(addr)
}
func (p *PublicCoreAPI) GetTesseractByHash(hash string) (*ktypes.Tesseract, error) {
	hash, err := kutils.ValidateHash(hash)
	if err != nil {
		return nil, err
	}

	return p.backend.chain.GetTesseract(ktypes.BytesToHash(ktypes.Hex2Bytes(hash)))
}

func (p *PublicCoreAPI) GetTesseractByHeight(from string, height uint64) (*ktypes.Tesseract, error) {
	from, err := kutils.ValidateAddress(from)
	if err != nil {
		return nil, err
	}

	tesseract, err := p.backend.chain.GetTesseractByHeight(from, height)
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

	var dimension, info uint8

	assetInfo := new(ktypes.AssetInfo)

	dimension = uint8(big.NewInt(0).SetBytes(aID[:1]).Uint64())

	info = uint8(big.NewInt(0).SetBytes(aID[1:2]).Uint64())

	assetHash := aID[2:]
	assetData, err := p.backend.chain.GetAssetDataByAssetHash(assetHash)

	if err != nil {
		return nil, err
	}

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
	assetInfo.Symbol = assetData.Symbol

	assetInfo.TotalSupply = big.NewInt(0).SetBytes(assetData.Extra).Uint64()

	if err != nil {
		return nil, err
	}

	assetInfo.Owner = assetData.Owner.Hex()

	return assetInfo, nil
}

// GetContextInfo will fetch the context associated with the given address
func (p *PublicCoreAPI) GetContextInfo(add ktypes.Address) ([]string, []string, error) {
	_, behaviourSet, RandomSet, err := p.backend.sm.GetLatestContext(add)
	if err != nil {
		return nil, nil, err
	}

	return ktypes.KIPPeerIDToString(behaviourSet), ktypes.KIPPeerIDToString(RandomSet), nil
}

// TDU will return the total digital utility associated with address
func (p *PublicCoreAPI) TDU(addr ktypes.Address) (*ktypes.BalanceObject, error) {
	return p.backend.sm.GetBalances(addr)
}

func (p *PublicCoreAPI) GetInteractionReceipt(address ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error) {
	return p.backend.chain.GetReceipt(address, ixHash)
}

func (p *PublicCoreAPI) GetTransactionCountByAddress(addr string, status bool) (uint64, error) {
	addr, err := kutils.ValidateAddress(addr)
	if err != nil {
		return 0, err
	}

	transactionCount, err := p.backend.sm.GetLatestNonce(ktypes.HexToAddress(addr))
	if err != nil {
		return 0, err
	}

	return transactionCount, nil
}

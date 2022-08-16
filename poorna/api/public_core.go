package api

import (
	"math/big"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
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
func (p *PublicCoreAPI) GetTesseractByHash(hash []byte) (*ktypes.Tesseract, error) {
	return p.backend.chain.GetTesseract(ktypes.BytesToHash(hash))
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

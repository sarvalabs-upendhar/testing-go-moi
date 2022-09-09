package api

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

// PublicIXPoolAPI is a struct that represents a wrapper for the interaction pool public APIs
type PublicIXPoolAPI struct {
	// Represents the API backend
	backend *Backend
}

// NewPublicIXPoolAPI is a constructor function that generates and returns a new
// PublicIXPoolAPI object for a given API backend object.
func NewPublicIXPoolAPI(b *Backend) *PublicIXPoolAPI {
	// Create the ixpool public API wrapper and return it
	return &PublicIXPoolAPI{backend: b}
}

// AddIXs is a method of PublicIXPoolAPI for adding Interactions to the pool.
// Accepts a slice of Interactions and returns any error that
// occurs while adding them to pool.
func (p *PublicIXPoolAPI) AddIXs(ixs []*ktypes.Interaction) []error {
	// Add  interactions to  interaction pool and return any error that might occur
	return p.backend.ixpool.AddInteractions(ixs)
}

func (p *PublicIXPoolAPI) GetLatestNonce(addr ktypes.Address) (uint64, error) {
	return p.backend.ixpool.GetNonce(addr)
}

package api

import (
	"gitlab.com/sarvalabs/moichain/core/chain"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/guna"
)

// Backend is a struct that represents the API backend
type Backend struct {
	// Represents the API interaction pool
	ixpool *ixpool.IxPool

	// Represents the API chain manager
	chain *chain.ChainManager

	// Represents the API state manager
	sm *guna.StateManager
}

// NewBackend is a constructor function that generates and returns a new API Backend object
func NewBackend(ixpool *ixpool.IxPool, chain *chain.ChainManager, sm *guna.StateManager) *Backend {
	// Create a new API Backend object and return it
	return &Backend{ixpool, chain, sm}
}

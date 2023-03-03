package api

import (
	id "github.com/sarvalabs/moichain/mudra/kramaid"
)

// PublicNetAPI is a struct that represents a wrapper for the public Net APIs.
type PublicNetAPI struct {
	network Network
}

func NewPublicNetAPI(network Network) *PublicNetAPI {
	// Create the public net API wrapper and return it
	return &PublicNetAPI{network}
}

// Peers returns an array of Krama ID's that are connected to a node
func (p *PublicNetAPI) Peers() ([]id.KramaID, error) {
	content, err := p.network.GetPeers()
	if err != nil {
		return nil, err
	}

	return content, nil
}

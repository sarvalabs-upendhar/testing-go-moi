package api

import (
	id "github.com/sarvalabs/go-moi/common/kramaid"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
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
	return p.network.GetPeers(), nil
}

// Version returns the protocol version
func (p *PublicNetAPI) Version() (string, error) {
	return p.network.GetVersion(), nil
}

// Info returns krama id of the node
func (p *PublicNetAPI) Info() (rpcargs.NodeInfoResponse, error) {
	return rpcargs.NodeInfoResponse{
		KramaID: p.network.GetKramaID(),
	}, nil
}

package api

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// PublicDebugAPI is the collection of APIs exposed over the public debugging endpoint
type PublicDebugAPI struct {
	db      DB
	network Network
}

func NewPublicDebugAPI(db DB, network Network) *PublicDebugAPI {
	// Create the public Debug API wrapper and return it
	return &PublicDebugAPI{
		db:      db,
		network: network,
	}
}

// DBGet returns the raw value of the key that is stored in the database
func (p *PublicDebugAPI) DBGet(args *rpcargs.DebugArgs) (string, error) {
	encodedData := common.FromHex(args.Key)

	// Read the value of the encodedData from the database
	content, err := p.db.ReadEntry(encodedData)
	if err != nil {
		return "", err
	}

	decodedData := common.BytesToHex(content)

	return decodedData, nil
}

// GetAccounts returns a list of registered account addresses
func (p *PublicDebugAPI) GetAccounts() ([]common.Address, error) {
	return p.db.GetRegisteredAccounts()
}

// GetConnections returns a list of active connections and connection stats
func (p *PublicDebugAPI) GetConnections() rpcargs.ConnectionsResponse {
	connections := make([]rpcargs.Connection, 0, len(p.network.GetConns()))

	for _, conn := range p.network.GetConns() {
		connections = append(connections, rpcargs.Connection{
			PeerID:  conn.RemotePeer().String(),
			Streams: getStreams(conn),
		})
	}

	return rpcargs.ConnectionsResponse{
		Conns:              connections,
		InboundConnCount:   p.network.GetInboundConnCount(),
		OutboundConnCount:  p.network.GetOutboundConnCount(),
		ActivePubSubTopics: p.network.GetSubscribedTopics(),
	}
}

// helper functions

// getStreams retrieves stream information from a network connection
func getStreams(conn network.Conn) []rpcargs.Stream {
	streams := make([]rpcargs.Stream, 0, len(conn.GetStreams()))

	for _, stream := range conn.GetStreams() {
		streams = append(streams, rpcargs.Stream{
			Protocol:  stream.Protocol(),
			Direction: int(stream.Stat().Direction),
		})
	}

	return streams
}

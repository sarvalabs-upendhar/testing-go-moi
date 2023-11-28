package p2p

import (
	kdht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	maddr "github.com/multiformats/go-multiaddr"
)

type ConnectionGater struct {
	isGating bool
}

// NewConnectionGater returns a new instance of ConnectionGater.
func NewConnectionGater(isGating bool) connmgr.ConnectionGater {
	return ConnectionGater{
		isGating: isGating,
	}
}

// InterceptPeerDial intercepts peer dialing and allows all connections.
func (cg ConnectionGater) InterceptPeerDial(peerID peer.ID) (allow bool) {
	return true
}

// InterceptAddrDial intercepts address dialing and applies gating if enabled.
func (cg ConnectionGater) InterceptAddrDial(peerID peer.ID, multiaddr maddr.Multiaddr) (allow bool) {
	if cg.isGating {
		return kdht.PublicQueryFilter(nil, peer.AddrInfo{
			ID:    peerID,
			Addrs: []maddr.Multiaddr{multiaddr},
		})
	}

	return true
}

// InterceptAccept intercepts incoming connections and allows all of them.
func (cg ConnectionGater) InterceptAccept(multiaddrs network.ConnMultiaddrs) (allow bool) {
	return true
}

// InterceptSecured intercepts secured connections and allows all of them.
func (cg ConnectionGater) InterceptSecured(
	direction network.Direction,
	peerID peer.ID,
	multiaddrs network.ConnMultiaddrs,
) (allow bool) {
	return true
}

// InterceptUpgraded intercepts upgraded connections and allows all of them.
func (cg ConnectionGater) InterceptUpgraded(conn network.Conn) (allow bool, reason control.DisconnectReason) {
	return true, 0
}

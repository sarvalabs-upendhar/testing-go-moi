package p2p

import "github.com/libp2p/go-libp2p/core/peer"

// NewPeerEvent occurs when a new peer is discovered in KIP network
type NewPeerEvent struct {
	Peer *Peer
}

// DiscoverPeerEvent is fired to discover a peer from the p2p network
type DiscoverPeerEvent struct {
	ID peer.ID
}

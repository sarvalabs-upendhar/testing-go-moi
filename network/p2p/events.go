package p2p

import "github.com/libp2p/go-libp2p/core/peer"

// DiscoverPeerEvent is fired to discover a peer from the p2p network
type DiscoverPeerEvent struct {
	ID peer.ID
}

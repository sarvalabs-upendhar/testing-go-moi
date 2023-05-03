package utils

import (
	"github.com/libp2p/go-libp2p/core/peer"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

// NewIxsEvent occurs when new transactions enter the transaction pool
type NewIxsEvent struct {
	Ixs []*types.Interaction
}

// NewPeerEvent occurs when a new peer is discovered in KIP network
type NewPeerEvent struct {
	PeerID peer.ID
}

type PeerDiscoveredEvent struct {
	ID peer.ID
}

// NewMinedTesseractEvent occurs when new block is generated
type NewMinedTesseractEvent struct {
	Tesseract *types.Tesseract
	Delta     map[types.Hash][]byte
}

// TesseractReceivedEvent occurs when a new block is received from the peer
type TesseractReceivedEvent struct {
	Tesseract   *types.Tesseract
	ClusterInfo *types.ICSClusterInfo
	Sender      id.KramaID
}

// TesseractAddedEvent occurs when a new block is added to the lattice
type TesseractAddedEvent struct {
	Tesseract *types.Tesseract
}

// TesseractSyncEvent is fired when a new tesseract received and needs to be synced up.
type TesseractSyncEvent struct {
	Tesseract   *types.Tesseract
	ClusterInfo *types.ICSClusterInfo
	Delta       map[types.Hash][]byte
	Context     []id.KramaID
}

// SyncRequestEvent is fired by krama engine to sync the tesseract lattice
type SyncRequestEvent struct {
	Address  types.Address
	Height   uint64
	BestPeer id.KramaID
}

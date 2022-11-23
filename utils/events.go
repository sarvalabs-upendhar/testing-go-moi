package utils

import (
	ptypes "gitlab.com/sarvalabs/moichain/poorna/types"

	"github.com/libp2p/go-libp2p/core/peer"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/types"
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
	ClusterInfo *ptypes.ICSClusterInfo
	Sender      id.KramaID
}

// TesseractAddedEvent occurs when a new block is added to the chain
type TesseractAddedEvent struct {
	Tesseract *types.Tesseract
}

// TesseractSyncEvent is fired when a new tesseract received and needs to be synced up.
type TesseractSyncEvent struct {
	Tesseract *types.Tesseract
	Context   []id.KramaID
}

type SyncStatusUpdate struct {
	BucketID int32
	Count    int64
}

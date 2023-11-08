package utils

import (
	"github.com/libp2p/go-libp2p/core/peer"
	id "github.com/sarvalabs/go-moi/common/kramaid"

	"github.com/sarvalabs/go-moi/common"
)

// NewPeerEvent occurs when a new peer is discovered in KIP network
type NewPeerEvent struct {
	PeerID peer.ID
}

type PeerDiscoveredEvent struct {
	ID peer.ID
}

// NewMinedTesseractEvent occurs when new block is generated
type NewMinedTesseractEvent struct {
	Tesseract *common.Tesseract
	Delta     map[common.Hash][]byte
}

// TesseractReceivedEvent occurs when a new block is received from the peer
type TesseractReceivedEvent struct {
	Tesseract   *common.Tesseract
	ClusterInfo *common.ICSClusterInfo
	Sender      id.KramaID
}

// TesseractAddedEvent occurs when a new block is added to the lattice
type TesseractAddedEvent struct {
	Tesseract *common.Tesseract
}

// TesseractSyncEvent is fired when a new tesseract received and needs to be synced up.
type TesseractSyncEvent struct {
	Tesseract   *common.Tesseract
	ClusterInfo *common.ICSClusterInfo
	Delta       map[common.Hash][]byte
	Context     []id.KramaID
}

// SyncRequestEvent is fired by krama engine to sync the tesseract lattice
type SyncRequestEvent struct {
	Address  common.Address
	Height   uint64
	BestPeer id.KramaID
}

// PendingAccountEvent is fired to update the pending accounts variable in SyncStatusTracker
type PendingAccountEvent struct {
	Address common.Address
	Count   int64
}

// AddedInteractionEvent emits added interactions in the account queue
type AddedInteractionEvent struct {
	Ixs common.Interactions
}

// EnqueuedInteractionEvent emits enqueued interactions in the account queue
type EnqueuedInteractionEvent struct {
	Ixs common.Interactions
}

// PromotedInteractionEvent emits promoted interactions in the account queue
type PromotedInteractionEvent struct {
	Ixs common.Interactions
}

// PrunedEnqueuedInteractionEvent emits pruned enqueue in the account queue
type PrunedEnqueuedInteractionEvent struct {
	Ixs common.Interactions
}

// PrunedPromotedInteractionEvent emits pruned promoted interactions in the account queue
type PrunedPromotedInteractionEvent struct {
	Ixs common.Interactions
}

// DroppedInteractionEvent emits dropped interactions in the account queue
type DroppedInteractionEvent struct {
	Ixs common.Interactions
}

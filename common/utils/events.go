package utils

import (
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"

	"github.com/sarvalabs/go-moi/common"
)

// NewMinedTesseractEvent occurs when new block is generated
type NewMinedTesseractEvent struct {
	Tesseract *common.Tesseract
	Delta     map[common.Hash][]byte
}

// TesseractAddedEvent occurs when a new block is added to the lattice
type TesseractAddedEvent struct {
	Tesseract *common.Tesseract
}

// TSTrackerEvent is triggered when a new tesseract is added to the lattice via consensus
// or when a tesseract is received from another node
// Msg distinguishes whether the the tesseract is added or received
type TSTrackerEvent struct {
	TSHash     common.Hash
	Msg        *networkmsg.TesseractMsg
	ExpiryTime time.Time
}

// SyncRequestEvent is fired by krama engine to sync the tesseract lattice
type SyncRequestEvent struct {
	ID       identifiers.Identifier
	Height   uint64
	BestPeer kramaid.KramaID
}

// PendingAccountEvent is fired to update the pending accounts variable in SyncStatusTracker
type PendingAccountEvent struct {
	ID    identifiers.Identifier
	Count int64
}

// AddedInteractionEvent emits added interactions in the account queue
type AddedInteractionEvent struct {
	Ixs []*common.Interaction
}

// PromotedInteractionEvent emits promoted interactions in the account queue
type PromotedInteractionEvent struct {
	Ixs []*common.Interaction
}

// PrunedEnqueuedInteractionEvent emits pruned enqueue in the account queue
type PrunedEnqueuedInteractionEvent struct {
	Ixs []*common.Interaction
}

// PrunedPromotedInteractionEvent emits pruned promoted interactions in the account queue
type PrunedPromotedInteractionEvent struct {
	Ixs []*common.Interaction
}

// DroppedInteractionEvent emits dropped interactions in the account queue
type DroppedInteractionEvent struct {
	Ixs []*common.Interaction
}

// SystemAccountsSyncedEvent signals system account's syncing done
type SystemAccountsSyncedEvent struct{}

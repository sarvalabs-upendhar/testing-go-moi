package forage

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type BucketSyncRequest struct {
	BucketID  uint64
	Timestamp uint64
}

type BucketSyncResponse struct {
	BucketID         uint64
	BucketCount      uint64
	AccountMetaInfos [][]byte
}

type SnapRequest struct {
	AccountID identifiers.Identifier
	Height    uint64
}

type LatticeRequest struct {
	AccountID   identifiers.Identifier
	StartHeight uint64
	EndHeight   uint64
}

package forage

import (
	"github.com/sarvalabs/go-moi-identifiers"
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
	Address identifiers.Address
	Height  uint64
}

type LatticeRequest struct {
	Address     identifiers.Address
	StartHeight uint64
	EndHeight   uint64
}

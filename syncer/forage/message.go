package forage

import "github.com/sarvalabs/moichain/common"

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
	Address common.Address
	Height  uint64
}

type SnapMetaInfo struct {
	Hash          common.Hash
	CreatedAt     int64
	TotalSnapSize uint64
}

type SnapResponse struct {
	MetaInfo *SnapMetaInfo
	Data     []byte
}

type LatticeRequest struct {
	Address     common.Address
	StartHeight uint64
	EndHeight   uint64
}

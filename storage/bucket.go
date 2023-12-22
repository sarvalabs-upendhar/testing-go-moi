package storage

import (
	"encoding/binary"
	"math/big"

	"github.com/sarvalabs/go-moi/common"
)

func BucketKeyAndID(addr common.Address) ([]byte, uint64) {
	accID := new(big.Int).SetBytes(addr.Bytes())

	bucketNo := accID.Mod(accID, new(big.Int).SetUint64(MaxBucketCount))

	return append(bucketPrefix(bucketNo.Uint64()), addr.Bytes()...), bucketNo.Uint64()
}

func bucketPrefix(id uint64) []byte {
	countBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(countBytes, id)

	return dbKey(common.NilAddress, Bucket, countBytes)
}

func bucketCountKey(count uint64) []byte {
	countBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(countBytes, count)

	return dbKey(common.NilAddress, BucketCount, countBytes)
}

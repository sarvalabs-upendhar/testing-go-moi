package storage

import (
	"encoding/binary"
	"math/big"

	"github.com/sarvalabs/go-moi/common"
)

func BucketKeyAndID(addr common.Address) ([]byte, uint64) {
	accID := new(big.Int).SetBytes(addr.Bytes())

	bucketNo := accID.Mod(accID, new(big.Int).SetUint64(MaxBucketCount))

	countBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(countBytes, bucketNo.Uint64())

	return append(append([]byte{Bucket.Byte()}, countBytes...), addr.Bytes()...), bucketNo.Uint64()
}

func bucketPrefix(id uint64) []byte {
	countBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(countBytes, id)

	return append([]byte{Bucket.Byte()}, countBytes...)
}

func bucketCountKey(count uint64) []byte {
	countBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(countBytes, count)

	return dbKey(common.NilAddress, BucketCount, countBytes)
}

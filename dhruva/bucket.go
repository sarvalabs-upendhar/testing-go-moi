package dhruva

import (
	"math/big"
)

// BucketID holds "bucket"+ []byte(bucket number)
type BucketID []byte

// BucketCount tells the no of buckets , accounts can be classified into
const (
	BucketCount int64 = 1024
)

// IDToBytes takes bucket number and returns BucketID
func IDToBytes(id int64) BucketID {
	bucket := make([]byte, 4)
	bigID := big.NewInt(id)

	bigID.FillBytes(bucket)

	return append([]byte("bucket"), bucket...)
}

// getID removes bucket prefix from BucketID and returns bucket number
func (b BucketID) getID() int64 {
	bigInt := new(big.Int).SetBytes(b[6:])

	return bigInt.Int64()
}

// getIDBytes removes bucket prefix and returns []byte(bucket number)
func (b BucketID) getIDBytes() []byte {
	return b[6:]
}

// BucketIDFromAddress takes an address and return bucket key and BucketID
func BucketIDFromAddress(addr []byte) ([]byte, BucketID) {
	accID := new(big.Int).SetBytes(addr)
	bucketNo := accID.Mod(accID, big.NewInt(BucketCount))
	id := IDToBytes(bucketNo.Int64())

	return append(id, addr...), id
}

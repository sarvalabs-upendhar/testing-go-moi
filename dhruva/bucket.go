package dhruva

import "math/big"

type BucketID []byte

func IDToBytes(id int64) BucketID {
	bucket := make([]byte, 4)
	bigID := big.NewInt(id)

	bigID.FillBytes(bucket)

	return append([]byte("bucket"), bucket...)
}

func (b BucketID) getID() int64 {
	bigInt := new(big.Int).SetBytes(b[6:])

	return bigInt.Int64()
}
func (b BucketID) getIDBytes() []byte {
	return b[6:]
}

func BucketIDFromAddress(addr []byte) ([]byte, BucketID) {
	accID := new(big.Int).SetBytes(addr)
	bucketNo := accID.Mod(accID, big.NewInt(BucketCount))
	id := IDToBytes(bucketNo.Int64())

	return append(id, addr...), id
}

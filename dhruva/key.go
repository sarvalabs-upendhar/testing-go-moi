package dhruva

import (
	"encoding/binary"
	"math/big"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/moichain/types"
)

type Prefix byte

const (
	// Prefix MSB is set for non account based keys

	Interaction         Prefix = 0x80
	NTQ                 Prefix = 0x81
	Tesseract           Prefix = 0x82
	TSGridLookup        Prefix = 0x83
	Receipt             Prefix = 0x84
	AccountSyncJob      Prefix = 0x85
	AccountSyncStatus   Prefix = 0x86
	PrincipalSyncStatus Prefix = 0x87
	Bucket              Prefix = 0x88
	BucketCount         Prefix = 0x89

	// Prefix MSB is unset for account based keys

	Account         Prefix = 0x00
	Context         Prefix = 0x01
	Logic           Prefix = 0x02
	File            Prefix = 0x03
	Storage         Prefix = 0x04
	Balance         Prefix = 0x05
	Approvals       Prefix = 0x06
	PreImage        Prefix = 0x07
	TesseractHeight Prefix = 0x08
)

func (p Prefix) Byte() byte {
	return byte(p)
}

func (p Prefix) IsAccountBasedKey() bool {
	return !(p&0x80 == 0x80)
}

func DBKey(address types.Address, prefix Prefix, key []byte) []byte {
	return dbKey(address, prefix, key)
}

func dbKey(address types.Address, prefix Prefix, key []byte) []byte {
	if !prefix.IsAccountBasedKey() {
		return append([]byte{prefix.Byte()}, key...)
	}

	return append(address.Bytes(), append([]byte{prefix.Byte()}, key...)...)
}

func NtqDBKey(peerID peer.ID) []byte {
	return append([]byte{NTQ.Byte()}, []byte(peerID)...)
}

func NtqCacheKey(peerID peer.ID) string {
	return types.BytesToHex([]byte{NTQ.Byte()}) + string(peerID)
}

func AccSyncStatusKey(addrs types.Address) []byte {
	return dbKey(types.NilAddress, AccountSyncStatus, addrs.Bytes())
}

func ContextObjectKey(address types.Address, contextHash types.Hash) []byte {
	return dbKey(address, Context, contextHash.Bytes())
}

func BalanceObjectKey(address types.Address, balanceHash types.Hash) []byte {
	return dbKey(address, Balance, balanceHash.Bytes())
}

func AccountKey(address types.Address, stateHash types.Hash) []byte {
	return dbKey(address, Account, stateHash.Bytes())
}

func PreImageKey(address types.Address, hash types.Hash) []byte {
	return dbKey(address, PreImage, hash.Bytes())
}

func tesseractHeightKey(addr types.Address, height uint64) []byte {
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)

	return dbKey(addr, TesseractHeight, heightBytes)
}

func principalSyncStatusKey() []byte {
	return dbKey(types.NilAddress, PrincipalSyncStatus, nil)
}

func BucketKeyAndID(addr types.Address) ([]byte, uint64) {
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

	return dbKey(types.NilAddress, BucketCount, countBytes)
}

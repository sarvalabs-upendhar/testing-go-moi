package dhruva

import (
	"encoding/binary"

	"gitlab.com/sarvalabs/moichain/types"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type Prefix byte

const (
	// Prefix MSB is set for non account based keys

	Interaction     Prefix = 0x80
	NTQ             Prefix = 0x81
	Tesseract       Prefix = 0x82
	TesseractHeight Prefix = 0x83

	// Prefix MSB is unset for account based keys

	Account   Prefix = 0x00
	Context   Prefix = 0x01
	Logic     Prefix = 0x02
	File      Prefix = 0x03
	Storage   Prefix = 0x04
	Balance   Prefix = 0x05
	Approvals Prefix = 0x06
	PreImage  Prefix = 0x07
)

func (p Prefix) Byte() byte {
	return byte(p)
}

func dbKey(address types.Address, prefix Prefix, key []byte) []byte {
	if address.IsNil() {
		return append([]byte{prefix.Byte()}, key...)
	}

	return append(address.Bytes(), append([]byte{prefix.Byte()}, key...)...)
}

func NtqDBKey(kramaID id.KramaID) []byte {
	return append([]byte{NTQ.Byte()}, []byte(kramaID)...)
}

func NtqCacheKey(key id.KramaID) string {
	return types.BytesToHex([]byte{NTQ.Byte()}) + string(key)
}

func ContextObjectKey(address types.Address, hash types.Hash) []byte {
	return dbKey(address, Context, hash.Bytes())
}

func BalanceObjectKey(address types.Address, hash types.Hash) []byte {
	return dbKey(address, Balance, hash.Bytes())
}

func AccountKey(address types.Address, hash types.Hash) []byte {
	return dbKey(address, Account, hash.Bytes())
}

func PreImageKey(address types.Address, hash types.Hash) []byte {
	return dbKey(address, PreImage, hash.Bytes())
}

func tesseractHeightKey(addr types.Address, height uint64) []byte {
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)

	return dbKey(addr, TesseractHeight, heightBytes)
}

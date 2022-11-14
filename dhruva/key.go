package dhruva

import (
	"encoding/binary"

	"gitlab.com/sarvalabs/moichain/types"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type Prefix uint8

const (
	Interaction Prefix = iota
	NTQ
	Tesseract
	TesseractHeight
	Account
	Context
	Logic
	File
	Storage
	Balance
	Approvals
	PreImage
)

func (groupID Prefix) Byte() byte {
	return []byte{0x01, 0x02, 0x3, 0x04, 0x01, 0x02, 0x3, 0x04, 0x05, 0x06, 0x07, 0x08}[groupID]
}

func dbKey(address types.Address, groupID Prefix, key []byte) []byte {
	if address != types.NilAddress {
		return append(address.Bytes(), append([]byte{groupID.Byte()}, key...)...)
	}

	return append([]byte{groupID.Byte()}, key...)
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

package ktypes

import (
	"encoding/binary"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

type GroupID uint8

const (
	InteractionGID GroupID = iota
	NtqGID
	TesseractGID
	AccountGID
	ContextGID
	LogicsGID
	FilesGID
	StorageGID
	BalanceGID
	ApprovalsGID
)

func (groupID GroupID) Byte() byte {
	return []byte{0x01, 0x02, 0x3, 0x01, 0x02, 0x3, 0x04, 0x05, 0x06, 0x07}[groupID]
}

func DBKey(address Address, groupID GroupID, hash Hash) []byte {
	if address != NilAddress {
		return append(address.Bytes(), append([]byte{groupID.Byte()}, hash.Bytes()...)...)
	}

	return append([]byte{groupID.Byte()}, hash.Bytes()...)
}

func NtqDBKey(kramaID id.KramaID) []byte {
	return append([]byte{NtqGID.Byte()}, []byte(kramaID)...)
}

func NtqCacheKey(key id.KramaID) string {
	return BytesToHex([]byte{NtqGID.Byte()}) + string(key)
}

func GetAddressHeightKey(addr Address, height uint64) []byte {
	prefix := "h"
	prefixByte := []byte(prefix)
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)

	addressBytes := addr.Bytes()
	result := append(prefixByte, addressBytes...)
	result = append(result, heightBytes...)

	return result
}

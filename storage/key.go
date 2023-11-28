package storage

import (
	"encoding/binary"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/go-moi/common"
)

func DBKey(address common.Address, prefix Prefix, key []byte) []byte {
	return dbKey(address, prefix, key)
}

func dbKey(address common.Address, prefix Prefix, key []byte) []byte {
	if !prefix.IsAccountBasedKey() {
		return append([]byte{prefix.Byte()}, key...)
	}

	return append(address.Bytes(), append([]byte{prefix.Byte()}, key...)...)
}

func SenatusDBKey(peerID peer.ID) []byte {
	return append([]byte{Senatus.Byte()}, []byte(peerID)...)
}

func SenatusCacheKey(peerID peer.ID) string {
	return common.BytesToHex([]byte{Senatus.Byte()}) + string(peerID)
}

func AccSyncStatusKey(addrs common.Address) []byte {
	return dbKey(common.NilAddress, AccountSyncStatus, addrs.Bytes())
}

func ContextObjectKey(address common.Address, contextHash common.Hash) []byte {
	return dbKey(address, Context, contextHash.Bytes())
}

func RegistryObjectKey(address common.Address, registryHash common.Hash) []byte {
	return dbKey(address, Registry, registryHash.Bytes())
}

func BalanceObjectKey(address common.Address, balanceHash common.Hash) []byte {
	return dbKey(address, Balance, balanceHash.Bytes())
}

func LogicManifestKey(address common.Address, manifestHash common.Hash) []byte {
	return dbKey(address, LogicManifest, manifestHash.Bytes())
}

func AccountKey(address common.Address, stateHash common.Hash) []byte {
	return dbKey(address, Account, stateHash.Bytes())
}

func PreImageKey(address common.Address, hash common.Hash) []byte {
	return dbKey(address, PreImage, hash.Bytes())
}

func SenatusPeerCountKey() []byte {
	return []byte("senatus_peer_count")
}

func tesseractHeightKey(addr common.Address, height uint64) []byte {
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)

	return dbKey(addr, TesseractHeight, heightBytes)
}

func principalSyncStatusKey() []byte {
	return dbKey(common.NilAddress, PrincipalSyncStatus, nil)
}

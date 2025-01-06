package storage

import (
	"encoding/binary"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
)

func DBKey(address identifiers.Address, tag PrefixTag, key []byte) []byte {
	return dbKey(address, tag, key)
}

func dbKey(address identifiers.Address, tag PrefixTag, key []byte) []byte {
	if !tag.IsAccountBasedKey() {
		return append([]byte(NonAccountPrefix), append([]byte{tag.Byte()}, key...)...)
	}

	return append(address.Bytes(), append([]byte{tag.Byte()}, key...)...)
}

func SenatusDBKey(peerID peer.ID) []byte {
	return dbKey(identifiers.NilAddress, Senatus, []byte(peerID))
}

func SenatusCacheKey(peerID peer.ID) string {
	return common.BytesToHex([]byte{Senatus.Byte()}) + string(peerID)
}

func AccSyncStatusKey(addrs identifiers.Address) []byte {
	return dbKey(identifiers.NilAddress, AccountSyncStatus, addrs.Bytes())
}

func ContextObjectKey(address identifiers.Address, contextHash common.Hash) []byte {
	return dbKey(address, Context, contextHash.Bytes())
}

func DeedsKey(address identifiers.Address, registryHash common.Hash) []byte {
	return dbKey(address, Deeds, registryHash.Bytes())
}

func LogicManifestKey(address identifiers.Address, manifestHash common.Hash) []byte {
	return dbKey(address, LogicManifest, manifestHash.Bytes())
}

func AccountKey(address identifiers.Address, stateHash common.Hash) []byte {
	return dbKey(address, Account, stateHash.Bytes())
}

func KeyObjectKey(address identifiers.Address, accountKeysHash common.Hash) []byte {
	return dbKey(address, AccountKeys, accountKeysHash.Bytes())
}

func PreImageKey(address identifiers.Address, hash common.Hash) []byte {
	return dbKey(address, PreImage, hash.Bytes())
}

func AccountSafetyInfoKey(address identifiers.Address) []byte {
	return dbKey(identifiers.NilAddress, ConsensusSafetyInfo, address.Bytes())
}

func InteractionsKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.NilAddress, Interaction, tsHash.Bytes())
}

func ReceiptsKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.NilAddress, Receipt, tsHash.Bytes())
}

func TesseractKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.NilAddress, Tesseract, tsHash.Bytes())
}

func TesseractCommitInfoKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.NilAddress, TesseractCommitInfo, tsHash.Bytes())
}

func ConsensusProposalKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.NilAddress, ConsensusProposals, tsHash.Bytes())
}

func tesseractHeightKey(addr identifiers.Address, height uint64) []byte {
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)

	return dbKey(addr, TesseractHeight, heightBytes)
}

func principalSyncStatusKey() []byte {
	return dbKey(identifiers.NilAddress, PrincipalSyncStatus, nil)
}

package storage

import (
	"encoding/binary"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/common"
)

type IdentifierKey [32]byte

func NewIdentifierKey(id identifiers.Identifier) IdentifierKey {
	return IdentifierKey(id.Bytes())
}

func (ik IdentifierKey) Bytes() []byte {
	return ik[:]
}

func DBKey(id identifiers.Identifier, tag PrefixTag, key []byte) []byte {
	return dbKey(id, tag, key)
}

func dbKey(id identifiers.Identifier, tag PrefixTag, key []byte) []byte {
	if !tag.IsAccountBasedKey() {
		return append([]byte(NonAccountPrefix), append([]byte{tag.Byte()}, key...)...)
	}

	return append(NewIdentifierKey(id).Bytes(), append([]byte{tag.Byte()}, key...)...)
}

func SenatusDBKey(peerID peer.ID) []byte {
	return dbKey(identifiers.Nil, Senatus, []byte(peerID))
}

func SenatusCacheKey(peerID peer.ID) string {
	return common.BytesToHex([]byte{Senatus.Byte()}) + string(peerID)
}

func AccSyncStatusKey(ids identifiers.Identifier) []byte {
	return dbKey(identifiers.Nil, AccountSyncStatus, ids.Bytes())
}

func ContextObjectKey(id identifiers.Identifier, contextHash common.Hash) []byte {
	return dbKey(id, Context, contextHash.Bytes())
}

func DeedsKey(id identifiers.Identifier, registryHash common.Hash) []byte {
	return dbKey(id, Deeds, registryHash.Bytes())
}

func LogicManifestKey(id identifiers.Identifier, manifestHash common.Hash) []byte {
	return dbKey(id, LogicManifest, manifestHash.Bytes())
}

func AccountKey(id identifiers.Identifier, stateHash common.Hash) []byte {
	return dbKey(id, Account, stateHash.Bytes())
}

func KeyObjectKey(id identifiers.Identifier, accountKeysHash common.Hash) []byte {
	return dbKey(id, AccountKeys, accountKeysHash.Bytes())
}

func PreImageKey(id identifiers.Identifier, hash common.Hash) []byte {
	return dbKey(id, PreImage, hash.Bytes())
}

func AccountSafetyInfoKey(id identifiers.Identifier) []byte {
	return dbKey(identifiers.Nil, ConsensusSafetyInfo, id.Bytes())
}

func InteractionsKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.Nil, Interaction, tsHash.Bytes())
}

func ReceiptsKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.Nil, Receipt, tsHash.Bytes())
}

func TesseractKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.Nil, Tesseract, tsHash.Bytes())
}

func TesseractCommitInfoKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.Nil, TesseractCommitInfo, tsHash.Bytes())
}

func ConsensusProposalKey(tsHash common.Hash) []byte {
	return dbKey(identifiers.Nil, ConsensusProposals, tsHash.Bytes())
}

func tesseractHeightKey(id identifiers.Identifier, height uint64) []byte {
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)

	return dbKey(id, TesseractHeight, heightBytes)
}

func principalSyncStatusKey() []byte {
	return dbKey(identifiers.Nil, PrincipalSyncStatus, nil)
}

package tree

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	db "github.com/sarvalabs/go-moi/storage"
)

type mockDB struct {
	data map[string][]byte
}

func NewMockDB() *mockDB {
	return &mockDB{
		data: make(map[string][]byte),
	}
}

func (m *mockDB) SetMerkleTreeEntry(address common.Address, prefix db.PrefixTag, key, value []byte) error {
	dbKey := append(append(address.Bytes(), prefix.Byte()), key...)

	m.data[string(dbKey)] = value

	return nil
}

func (m *mockDB) GetMerkleTreeEntry(address common.Address, prefix db.PrefixTag, key []byte) ([]byte, error) {
	dbKey := append(append(address.Bytes(), prefix.Byte()), key...)

	value, ok := m.data[string(dbKey)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return value, nil
}

func (m *mockDB) SetMerkleTreeEntries(address common.Address, prefix db.PrefixTag, entries map[string][]byte) error {
	for k, v := range entries {
		dbKey := append(append(address.Bytes(), prefix.Byte()), []byte(k)...)

		m.data[string(dbKey)] = v
	}

	return nil
}

func (m *mockDB) WritePreImages(address common.Address, entries map[common.Hash][]byte) error {
	for k, v := range entries {
		dbKey := db.PreImageKey(address, k)

		m.data[string(dbKey)] = v
	}

	return nil
}

func (m *mockDB) GetPreImage(address common.Address, hash common.Hash) ([]byte, error) {
	dbKey := db.PreImageKey(address, hash)

	value, ok := m.data[string(dbKey)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return value, nil
}

func checkForReferences(t *testing.T, kht, copiedKHT *KramaHashTree) {
	t.Helper()

	require.NotEqual(t,
		reflect.ValueOf(kht.preImages).Pointer(),
		reflect.ValueOf(copiedKHT.preImages).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(kht.root.HashTable).Pointer(),
		reflect.ValueOf(copiedKHT.root.HashTable).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(kht.root).Pointer(),
		reflect.ValueOf(copiedKHT.root).Pointer(),
	)
	require.NotEqual(t,
		reflect.ValueOf(kht.tree).Pointer(),
		reflect.ValueOf(copiedKHT.tree).Pointer(),
	)
}

func checkForPreImage(t *testing.T, key []byte, hashTree *KramaHashTree, shouldExist bool) {
	t.Helper()

	v, ok := hashTree.preImages[HashKey(key)]
	if !shouldExist {
		require.False(t, ok)

		return
	}

	require.True(t, ok)
	require.Equal(t, key, v, "pre image mismatch")
}

func checkForDeltaNodes(t *testing.T, key, value []byte, hashTree *KramaHashTree, shouldExist bool) {
	t.Helper()

	v, ok := hashTree.root.HashTable[hex.EncodeToString(key)]
	if !shouldExist {
		require.False(t, ok)

		return
	}

	require.True(t, ok)
	require.Equal(t, value, v, "leaf value mismatch")
}

func checkForEntry(t *testing.T, key, value []byte, hashTree *KramaHashTree, shouldExist bool) {
	t.Helper()

	dbValue, err := hashTree.tree.GetDescend(HashKey(key).Bytes())
	if !shouldExist {
		require.Empty(t, dbValue)

		return
	}

	require.NoError(t, err)
	require.Equal(t, value, dbValue)
}

func createTestHashTreeWithEntries(
	t *testing.T,
	address common.Address,
	persistentDB persistentDB,
	entries map[string][]byte,
) *KramaHashTree {
	t.Helper()

	hashTree, err := NewKramaHashTree(address, common.NilHash, persistentDB, blake256.New(), db.Storage)
	require.NoError(t, err)

	for k, v := range entries {
		err := hashTree.Set([]byte(k), v)
		require.NoError(t, err, "failed to insert")
	}

	return hashTree
}

func fetchRootNodeAndDelta(t *testing.T, hashTree *KramaHashTree) *common.RootNode {
	t.Helper()

	rootHash, err := hashTree.RootHash()
	require.NoError(t, err)

	rawData, err := hashTree.db.Get(rootHash.Bytes())
	require.NoError(t, err)

	root := new(common.RootNode)

	require.NoError(t, polo.Depolorize(root, rawData))

	return root
}

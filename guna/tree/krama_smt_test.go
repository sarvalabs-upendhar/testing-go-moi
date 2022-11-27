package tree

import (
	"testing"

	"github.com/sarvalabs/moichain/dhruva"

	"github.com/sarvalabs/moichain/types"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common/tests"
)

func TestKramaHashTree_Set_NewEntry(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	tt := []struct {
		name          string
		inputKey      []byte
		inputValue    []byte
		expectedError error
	}{
		{
			name:          "Zero length key",
			inputKey:      []byte{},
			inputValue:    []byte("Test Value"),
			expectedError: types.ErrInvalidKey,
		},
		{
			name:          "Zero length value",
			inputKey:      []byte("Test-Key"),
			inputValue:    []byte{},
			expectedError: types.ErrInvalidValue,
		},
		{
			name:          "Add new key value entry",
			inputKey:      []byte("Test-Key"),
			inputValue:    []byte("Test Value"),
			expectedError: nil,
		},
	}

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		nil,
	)

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			err := hashTree.Set(test.inputKey, test.inputValue)
			if test.expectedError != nil {
				// check for error
				require.ErrorIs(t, err, test.expectedError)
			} else {
				// check for the value in db
				checkForEntryInDB(t, test.inputKey, test.inputValue, hashTree, true)

				// check for preimage
				checkForPreImage(t, test.inputKey, hashTree, true)

				// check for newly added key-value entry in delta nodes
				checkForDeltaNodes(t, test.inputKey, test.inputValue, hashTree, true)
			}
		})
	}
}

func TestKramaHashTree_Set_UpdateEntry(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	key := []byte("Test-Key")
	initialValue := []byte("Test-Value")
	updatedValue := []byte("Updated-Value")

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		map[string][]byte{string(key): initialValue},
	)

	err := hashTree.Set(key, initialValue)
	require.NoError(t, err)

	// check for the initial value in db
	checkForEntryInDB(t, key, initialValue, hashTree, true)

	// update the entry
	err = hashTree.Set(key, updatedValue)
	require.NoError(t, err)

	// check for the updated value in db
	checkForEntryInDB(t, key, updatedValue, hashTree, true)

	// check for preimage
	checkForPreImage(t, key, hashTree, true)

	// check for newly added key-value entry in delta nodes
	checkForDeltaNodes(t, key, updatedValue, hashTree, true)
}

func TestKramaHashTree_Get_UnCommitted_Entry(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	key := []byte("Test-Key")
	value := []byte("Test-Value")

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		map[string][]byte{string(key): value},
	)

	err := hashTree.Set(key, value)
	require.NoError(t, err)

	uncommittedValue, err := hashTree.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, uncommittedValue)
}

func TestKramaHashTree_Get_Committed_Entry(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	key := []byte("Test-Key")
	value := []byte("Test-Value")

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		map[string][]byte{string(key): value},
	)

	err := hashTree.Set(key, value)
	require.NoError(t, err)

	// Commit the changes and flush to db
	err = hashTree.Commit()
	require.NoError(t, err)

	err = hashTree.Flush()
	require.NoError(t, err)

	// Since the tree is flushed, Get will fetch from persistent disk
	diskValue, err := hashTree.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, diskValue)
}

func TestNewKramaHashTree_Flush(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	key := []byte("Test-Key")
	value := []byte("Test-Value")

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		map[string][]byte{string(key): value},
	)

	// commit the tree
	require.NoError(t, hashTree.Commit())

	nodes := make(map[types.Hash][]byte)
	// create an iterator to iterate over all nodes
	it := hashTree.NewIterator()
	for it.Next() {
		nodes[hashTree.hashKey(it.NodeBlob())] = it.NodeBlob()
	}

	// flush the tree changes
	require.NoError(t, hashTree.Flush())

	// IsDirty should return false as all modified entries are committed to db
	require.False(t, hashTree.IsDirty())

	// check for pre-image in persistent db
	preImage, err := db.GetPreImage(address, hashTree.hashKey(key))
	require.NoError(t, err, "pre-image not found in persistent db")

	require.Equal(t, key, preImage)

	// check for the tree nodes in db
	for k, nodeBlob := range nodes {
		v, err := db.GetMerkleTreeEntry(address, dhruva.Storage, k.Bytes())
		require.NoError(t, err)

		require.Equal(t, nodeBlob, v)
	}
}

func TestKramaHashTree_Commit(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		nil,
	)

	initialRoot, err := hashTree.Root()
	require.NoError(t, err)

	key := []byte("Test-Key")
	value := []byte("Test-Value")

	err = hashTree.Set(key, value)
	require.NoError(t, err)

	// commit the tree so that root gets updated
	err = hashTree.Commit()
	require.NoError(t, err)

	// verify that root has changed
	updatedRoot, err := hashTree.Root()
	require.NoError(t, err)
	require.NotEqual(t, updatedRoot, initialRoot)

	_, deltaNodes := fetchRootNodeAndDelta(t, hashTree)

	// check for the inserted values in delta set
	dbValue, ok := deltaNodes[string(key)]
	require.True(t, ok, "leaf not found in delta nodes")
	require.Equal(t, value, dbValue, "value mismatch")
}

func TestKramaHashTree_Delete(t *testing.T) {
	address := tests.RandomAddress(t)
	db := NewMockDB()

	key := []byte("Test-Key")
	value := []byte("Test-Value")

	hashTree := createTestHashTreeWithEntries(
		t,
		address,
		db,
		map[string][]byte{string(key): value},
	)

	err := hashTree.Set(key, value)
	require.NoError(t, err)

	err = hashTree.Delete(key)
	require.NoError(t, err)

	// check for the value in db
	checkForEntryInDB(t, key, value, hashTree, false)

	// check for preimage
	checkForPreImage(t, key, hashTree, false)

	// check for newly added key-value entry in delta nodes
	checkForDeltaNodes(t, key, value, hashTree, false)
}

func checkForPreImage(t *testing.T, key []byte, hashTree *KramaHashTree, shouldExist bool) {
	t.Helper()

	v, ok := hashTree.preImages[hashTree.hashKey(key)]
	if !shouldExist {
		require.False(t, ok)

		return
	}

	require.True(t, ok)
	require.Equal(t, key, v, "pre image mismatch")
}

func checkForDeltaNodes(t *testing.T, key, value []byte, hashTree *KramaHashTree, shouldExist bool) {
	t.Helper()

	v, ok := hashTree.deltaNodes[string(key)]
	if !shouldExist {
		require.False(t, ok)

		return
	}

	require.True(t, ok)
	require.Equal(t, value, v, "leaf value mismatch")
}

func checkForEntryInDB(t *testing.T, key, value []byte, hashTree *KramaHashTree, shouldExist bool) {
	t.Helper()

	dbValue, err := hashTree.db.Get(hashTree.hashKey(key).Bytes())
	if !shouldExist {
		require.ErrorIs(t, err, types.ErrKeyNotFound)

		return
	}

	require.NoError(t, err)
	require.Equal(t, value, dbValue)
}

func createTestHashTreeWithEntries(
	t *testing.T,
	address types.Address,
	db persistentDB,
	entries map[string][]byte,
) *KramaHashTree {
	t.Helper()

	hashTree, err := NewKramaHashTree(address, types.NilHash, db, blake256.New())
	require.NoError(t, err)

	for k, v := range entries {
		err := hashTree.Set([]byte(k), v)
		require.NoError(t, err, "failed to insert")
	}

	return hashTree
}

func fetchRootNodeAndDelta(t *testing.T, hashTree *KramaHashTree) (*rootNode, map[string][]byte) {
	t.Helper()

	rootHash, err := hashTree.Root()
	require.NoError(t, err)

	rawData, err := hashTree.db.Get(rootHash.Bytes())
	require.NoError(t, err)

	root := new(rootNode)

	require.NoError(t, polo.Depolorize(root, rawData))

	deltaInfo, err := hashTree.db.Get(root.HashTable.Bytes())
	require.NoError(t, err)

	deltaNodes, err := FetchDeltaNodes(deltaInfo)
	require.NoError(t, err)

	return root, deltaNodes
}

package tree

import (
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	db "github.com/sarvalabs/moichain/dhruva"
	"github.com/stretchr/testify/require"
)

func TestTreeDB_Get_CommittedEntry(t *testing.T) {
	address := tests.RandomAddress(t)
	mockDB := NewMockDB()

	testKey := []byte("test-key")
	testValue := []byte("test-value")

	treeDB := NewTreeDB(address, db.Storage, mockDB)

	// entry is only written to persistence db
	err := mockDB.SetMerkleTreeEntry(address, db.Storage, testKey, testValue)
	require.NoError(t, err, "failed  to add entry to mock")

	fetchedValue, err := treeDB.Get(testKey)
	require.NoError(t, err)
	require.Equal(t, testValue, fetchedValue)
}

func TestTreeDB_Get_UncommittedEntry(t *testing.T) {
	address := tests.RandomAddress(t)
	mockDB := NewMockDB()

	testKey := []byte("test-key")
	testValue := []byte("test-value")

	treeDB := NewTreeDB(address, db.Storage, mockDB)

	// entry is written to dirty storage
	treeDB.dirty[string(testKey)] = testValue

	fetchedValue, err := treeDB.Get(testKey)
	require.NoError(t, err)
	require.Equal(t, testValue, fetchedValue)
}

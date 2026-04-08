package tree

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
	db "github.com/sarvalabs/go-moi/storage"
)

func TestTreeDB_Get_CommittedEntry(t *testing.T) {
	id := tests.RandomIdentifier(t)
	mockDB := NewMockDB()

	testKey := []byte("test-key")
	testValue := []byte("test-value")

	treeDB := NewTreeDB(id, db.Storage, mockDB, tests.NewTestTreeCache(), NilMetrics())

	// entry is only written to persistence db
	err := mockDB.SetMerkleTreeEntry(id, db.Storage, testKey, testValue)
	require.NoError(t, err, "failed  to add entry to mock")

	fetchedValue, err := treeDB.Get(testKey)
	require.NoError(t, err)
	require.Equal(t, testValue, fetchedValue)
}

func TestTreeDB_Get_UncommittedEntry(t *testing.T) {
	id := tests.RandomIdentifier(t)
	mockDB := NewMockDB()

	testKey := []byte("test-key")
	testValue := []byte("test-value")

	treeDB := NewTreeDB(id, db.Storage, mockDB, tests.NewTestTreeCache(), NilMetrics())

	// entry is written to dirty storage
	treeDB.dirty[string(testKey)] = testValue

	fetchedValue, err := treeDB.Get(testKey)
	require.NoError(t, err)
	require.Equal(t, testValue, fetchedValue)
}

func TestTreeDB_Copy(t *testing.T) {
	treeDB := NewTreeDB(tests.RandomIdentifier(t), db.Storage, NewMockDB(), tests.NewTestTreeCache(), NilMetrics())
	treeDB.dirty["hello"] = []byte{1, 2}

	copiedDB := treeDB.Copy()
	copiedTreeDB, ok := copiedDB.(*TreeDB)
	require.True(t, ok)

	require.Equal(t, treeDB, copiedTreeDB)
	require.NotEqual(t,
		reflect.ValueOf(treeDB.dirty).Pointer(),
		reflect.ValueOf(copiedTreeDB.dirty).Pointer(),
	)
}

package guna

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/require"
)

func TestReputationEngine_GetInfo_FetchFromDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mstore := NewMockStore()
	mState := NewMockState()
	kramaIDs := tests.GetTestKramaIDs(t, 1)
	engine := NewTestReputationEngine(t, ctx, mState, mstore)

	// create an entry
	info := ReputationInfo{
		PublickKey: []byte{0x02, 0x03},
	}
	// add entry to DB
	err := mstore.CreateEntry(dhruva.NtqDBKey(kramaIDs[0]), polo.Polorize(info))
	require.NoError(t, err, "error adding reputation info to db")

	storedInfo, err := engine.getInfo(kramaIDs[0])
	require.NoError(t, err)
	require.NotNilf(t, storedInfo, "stored info is nil")
	require.Equal(t, storedInfo.PublickKey, info.PublickKey)
}

func NewTestReputationEngine(t *testing.T, ctx context.Context, state state, db store) *ReputationEngine {
	t.Helper()

	r, err := NewReputationEngine(ctx, hclog.NewNullLogger(), state, db)

	require.NoError(t, err, "error initiating reputation engine")

	return r
}

type mockStore struct {
	data map[string][]byte
}

func NewMockStore() *mockStore {
	return &mockStore{
		data: make(map[string][]byte),
	}
}

func (store *mockStore) ReadEntry(key []byte) ([]byte, error) {
	hexKey := hex.EncodeToString(key)

	data, ok := store.data[hexKey]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (store *mockStore) CreateEntry(key, value []byte) error {
	hexKey := hex.EncodeToString(key)
	if _, ok := store.data[hexKey]; ok {
		return types.ErrKeyExists
	}

	store.data[hexKey] = value

	return nil
}

func (store *mockStore) Contains(key []byte) (bool, error) {
	hexKey := hex.EncodeToString(key)
	if _, ok := store.data[hexKey]; ok {
		return true, nil
	}

	return false, nil
}

func (store *mockStore) UpdateEntry(key, value []byte) error {
	hexKey := hex.EncodeToString(key)

	if _, ok := store.data[hexKey]; !ok {
		return types.ErrKeyNotFound
	}

	store.data[hexKey] = value

	return nil
}

func (store *mockStore) NewBatchWriter() db.BatchWriter {
	return nil
}

func (store *mockStore) GetEntries(prefix []byte) chan types.DBEntry {
	return nil
}

type mockState struct {
	publicKeys map[id.KramaID][]byte
}

func NewMockState() *mockState {
	return &mockState{
		publicKeys: make(map[id.KramaID][]byte),
	}
}

func (state *mockState) GetPublicKeyFromContract(ctx context.Context, ids ...id.KramaID) (keys [][]byte, err error) {
	return nil, nil
}

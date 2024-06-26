package senatus

import (
	"bytes"
	"context"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

type MockDB struct {
	data          map[string][]byte
	peerCount     uint64
	peerCountHook func() error
}

func (db *MockDB) ReadEntry(key []byte) ([]byte, error) {
	data, ok := db.data[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (db *MockDB) NewBatchWriter() db.BatchWriter {
	return &mockBatchWriter{db: db}
}

func (db *MockDB) GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan *common.DBEntry, error) {
	entries := make(chan *common.DBEntry)

	go func() {
		for k, v := range db.data {
			if bytes.HasPrefix([]byte(k), prefix) {
				entries <- &common.DBEntry{
					Key:   []byte(k),
					Value: v,
				}
			}
		}

		close(entries)
	}()

	return entries, nil
}

func (db *MockDB) TotalPeersCount() (uint64, error) {
	if db.peerCountHook != nil {
		return 0, db.peerCountHook()
	}

	return db.peerCount, nil
}

func (db *MockDB) UpdatePeerCount(count uint64) error {
	db.peerCount += count

	return nil
}

func (db *MockDB) setEntry(key string, value []byte) {
	db.data[key] = value
}

func (db *MockDB) setNodeInfo(t *testing.T, peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
	t.Helper()

	metaInfo, err := nodeMetaInfo.Bytes()
	require.NoError(t, err)

	db.setEntry(string(storage.SenatusDBKey(peerID)), metaInfo)
}

func NewMockDB() *MockDB {
	return &MockDB{
		data: make(map[string][]byte),
	}
}

type mockBatchWriter struct {
	db *MockDB
}

func (bw *mockBatchWriter) WriteBuffer(buf []byte) error {
	// TODO implement me
	panic("implement me")
}

func (bw *mockBatchWriter) Set(key []byte, value []byte) error {
	bw.db.data[string(key)] = value

	return nil
}

func (bw *mockBatchWriter) Flush() error {
	return nil
}

type MockStateManager struct {
	accMetaInfo  map[identifiers.Address]*common.AccountMetaInfo
	logicStorage map[string]map[string]string // first key denotes logic id, second key denotes storage key
}

func NewMockState() *MockStateManager {
	return &MockStateManager{
		accMetaInfo:  make(map[identifiers.Address]*common.AccountMetaInfo),
		logicStorage: make(map[string]map[string]string),
	}
}

func (m *MockStateManager) setAccountMetaInfo(t *testing.T, accMetaInfo *common.AccountMetaInfo) {
	t.Helper()

	m.accMetaInfo[accMetaInfo.Address] = accMetaInfo
}

func (m *MockStateManager) GetAccountMetaInfo(addr identifiers.Address) (*common.AccountMetaInfo, error) {
	accMetaInfo, ok := m.accMetaInfo[addr]
	if !ok {
		return nil, common.ErrAccountNotFound
	}

	return accMetaInfo, nil
}

func (m *MockStateManager) setStorageEntry(
	t *testing.T,
	logicID identifiers.LogicID,
	slot []byte,
) {
	t.Helper()

	store := make(map[string]string)

	store[string(slot)] = "value"

	m.logicStorage[string(logicID)] = store
}

func (m *MockStateManager) GetPersistentStorageEntry(
	logicID identifiers.LogicID,
	slot []byte,
	state common.Hash,
) ([]byte, error) {
	store, ok := m.logicStorage[string(logicID)]
	if !ok {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	value, ok := store[string(slot)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return []byte(value), nil
}

type MockChain struct {
	tesseractsByHash map[common.Hash]*common.Tesseract
}

func (m MockChain) setTesseract(t *testing.T, hash common.Hash, ts *common.Tesseract) {
	t.Helper()

	m.tesseractsByHash[hash] = ts
}

func (m MockChain) GetTesseract(hash common.Hash, withInteractions bool) (*common.Tesseract, error) {
	ts, ok := m.tesseractsByHash[hash]
	if !ok {
		return nil, common.ErrFetchingTesseract
	}

	tsCopy := *ts // copy, so that stored tesseract won't be modified

	if !withInteractions {
		tsCopy = *tsCopy.GetTesseractWithoutIxns()
	}

	return &tsCopy, nil
}

func NewMockChain() *MockChain {
	return &MockChain{
		tesseractsByHash: make(map[common.Hash]*common.Tesseract),
	}
}

func createTestReputationEngine(t *testing.T) (*ReputationEngine, *MockDB) {
	t.Helper()

	mockDB := NewMockDB()
	nodeMetaInfo := &NodeMetaInfo{
		KramaID:    tests.RandomKramaID(t, 0),
		Registered: true, // self node should have registered flag true
	}

	r, err := NewReputationEngine(
		hclog.NewNullLogger(),
		mockDB,
		nodeMetaInfo,
		&utils.TypeMux{},
		nil,
	)

	require.NoError(t, err)

	return r, mockDB
}

package senatus

import (
	"bytes"
	"context"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-legacy-kramaid"
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

type MockState struct {
	publicKeys map[kramaid.KramaID][]byte
}

func NewMockState() *MockState {
	return &MockState{
		publicKeys: make(map[kramaid.KramaID][]byte),
	}
}

func (state *MockState) GetPublicKeyFromContract(ids ...kramaid.KramaID) (keys [][]byte, err error) {
	for _, kramaID := range ids {
		key, ok := state.publicKeys[kramaID]
		if ok {
			keys = append(keys, key)
		}
	}

	return
}

type mockServer struct{}

func NewMockServer() *mockServer {
	return &mockServer{}
}

func (m *mockServer) Subscribe(
	ctx context.Context,
	topicName string,
	validator utils.WrappedVal,
	defaultValidator bool,
	handler func(msg *pubsub.Message) error,
) error {
	return nil
}

func createTestReputationEngine(t *testing.T) (*ReputationEngine, *MockDB, *MockState) {
	t.Helper()

	mockDB := NewMockDB()
	mockState := NewMockState()
	nodeMetaInfo := &NodeMetaInfo{
		KramaID: tests.GetTestKramaID(t, 0),
	}

	r, err := NewReputationEngine(
		hclog.NewNullLogger(),
		mockDB,
		nodeMetaInfo,
	)

	require.NoError(t, err)

	return r, mockDB, mockState
}

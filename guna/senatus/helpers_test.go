package senatus

import (
	"bytes"
	"context"
	"testing"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/dhruva/db"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type MockDB struct {
	data map[string][]byte
}

func (db *MockDB) ReadEntry(key []byte) ([]byte, error) {
	data, ok := db.data[string(key)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (db *MockDB) NewBatchWriter() db.BatchWriter {
	return &mockBatchWriter{db: db}
}

func (db *MockDB) GetEntriesWithPrefix(ctx context.Context, prefix []byte) (chan types.DBEntry, error) {
	entries := make(chan types.DBEntry)

	go func() {
		for k, v := range db.data {
			if bytes.HasPrefix([]byte(k), prefix) {
				entries <- types.DBEntry{
					Key:   []byte(k),
					Value: v,
				}
			}
		}

		close(entries)
	}()

	return entries, nil
}

func (db *MockDB) setEntry(key string, value []byte) {
	db.data[key] = value
}

func (db *MockDB) setNodeInfo(t *testing.T, peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo) {
	t.Helper()

	metaInfo, err := nodeMetaInfo.Bytes()
	require.NoError(t, err)

	db.setEntry(string(dhruva.NtqDBKey(peerID)), metaInfo)
}

func NewMockDB() *MockDB {
	return &MockDB{
		data: make(map[string][]byte),
	}
}

type mockBatchWriter struct {
	db *MockDB
}

func (bw *mockBatchWriter) Set(key []byte, value []byte) error {
	bw.db.data[string(key)] = value

	return nil
}

func (bw *mockBatchWriter) Flush() error {
	return nil
}

type MockState struct {
	publicKeys map[id.KramaID][]byte
}

func NewMockState() *MockState {
	return &MockState{
		publicKeys: make(map[id.KramaID][]byte),
	}
}

func (state *MockState) GetPublicKeyFromContract(ids ...id.KramaID) (keys [][]byte, err error) {
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

func (m *mockServer) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	return nil
}

func CreateTestReputationEngine(t *testing.T) (*ReputationEngine, *MockDB, *MockState) {
	t.Helper()

	mockDB := NewMockDB()
	mockState := NewMockState()
	r, err := NewReputationEngine(
		context.Background(),
		hclog.NewNullLogger(),
		NewMockServer(),
		mockDB,
		tests.GetTestKramaID(t, 0),
		&gtypes.NodeMetaInfo{},
	)

	require.NoError(t, err)

	return r, mockDB, mockState
}

func getHelloMessage(t *testing.T, addr string) []byte {
	t.Helper()

	nodeMetaInfoMsg := &gtypes.NodeMetaInfoMsg{
		KramaID: tests.GetTestKramaID(t, 1),
		Address: []string{addr},
	}

	data, err := nodeMetaInfoMsg.HelloMessageBytes()
	require.NoError(t, err)

	return data
}

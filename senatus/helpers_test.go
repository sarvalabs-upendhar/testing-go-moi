package senatus

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/storage/db"
)

type MockDB struct {
	data map[string][]byte
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

func (db *MockDB) setEntry(key string, value []byte) {
	db.data[key] = value
}

func (db *MockDB) setNodeInfo(t *testing.T, peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
	t.Helper()

	metaInfo, err := nodeMetaInfo.Bytes()
	require.NoError(t, err)

	db.setEntry(string(storage.NtqDBKey(peerID)), metaInfo)
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
		hclog.NewNullLogger(),
		NewMockServer(),
		mockDB,
		tests.GetTestKramaID(t, 0),
		&NodeMetaInfo{},
	)

	require.NoError(t, err)

	return r, mockDB, mockState
}

func getHelloMessage(t *testing.T, addr string) []byte {
	t.Helper()

	nodeMetaInfoMsg := &NodeMetaInfoMsg{
		KramaID: tests.GetTestKramaID(t, 1),
		Address: []string{addr},
	}

	data, err := nodeMetaInfoMsg.HelloMessageBytes()
	require.NoError(t, err)

	return data
}

func createSignedHelloMsg(t *testing.T) networkmsg.HelloMsg {
	t.Helper()

	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	// create keystore.json in current directory
	password := "test123"

	_, _, err = poi.RandGenKeystore(dir, password)
	require.NoError(t, err)

	config := &crypto.VaultConfig{
		DataDir:      dir,
		NodePassword: password,
	}

	vault, err := crypto.NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	msg := networkmsg.HelloMsg{
		KramaID:   vault.KramaID(),
		Address:   []string{tests.RandomAddress(t).String()},
		Signature: nil,
	}

	rawMsg, err := msg.Bytes()
	require.NoError(t, err)

	signature, err := vault.Sign(rawMsg, mudracommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	require.NoError(t, err)

	msg.Signature = signature

	return msg
}

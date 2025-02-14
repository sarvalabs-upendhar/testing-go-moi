package decision

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/syncer/agora/db"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

type (
	ledgerCallBack       func(l *MockLedger)
	dbCallBack           func(db *MockDB)
	engineParamsCallBack func(e *Engine)
)

func NewTest(t *testing.T,
	dbFn dbCallBack,
	ledgerFn ledgerCallBack,
	paramsFn engineParamsCallBack,
) (*Engine, *MockDB, *MockLedger, *MockNetwork) {
	t.Helper()

	mockLedger := NewMockLedger()
	mockDB := NewMockDB()
	mockNetwork := NewMockNetwork()

	defaultEngine := NewEngine(
		hclog.NewNullLogger(),
		0,
		0,
		mockDB,
		mockLedger,
		mockNetwork,
		NilMetrics(),
		0,
	)

	if dbFn != nil {
		dbFn(mockDB)
	}

	if ledgerFn != nil {
		ledgerFn(mockLedger)
	}

	if paramsFn != nil {
		paramsFn(defaultEngine)
	}

	return defaultEngine, mockDB, mockLedger, mockNetwork
}

type MockDB struct {
	mtx  sync.Mutex
	data map[string][]byte
}

func NewMockDB() *MockDB {
	return &MockDB{
		data: make(map[string][]byte),
	}
}

func (db *MockDB) DoesStateExists(id identifiers.Identifier, stateHash cid.CID) bool {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	_, ok := db.data[string(stateHash.Bytes())]

	return ok
}

func (db *MockDB) Get(key []byte) ([]byte, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	data, ok := db.data[string(key)]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return data, nil
}

func (db *MockDB) Set(key, value []byte) {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	db.data[string(key)] = value
}

func (db *MockDB) GetBatchWriter() db.BatchWriter {
	return &mockBatchWriter{db: db}
}

func (db *MockDB) GetData(
	ctx context.Context,
	id identifiers.Identifier,
	keys []cid.CID,
) (map[cid.CID][]byte, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	resp := make(map[cid.CID][]byte, len(keys))

	for _, key := range keys {
		data := db.data[string(key[:])]
		resp[key] = data
	}

	return resp, nil
}

type mockBatchWriter struct {
	db *MockDB
}

func (bw *mockBatchWriter) Set(key []byte, value []byte) error {
	bw.db.mtx.Lock()
	defer bw.db.mtx.Unlock()

	bw.db.data[string(key)] = value

	return nil
}

func (bw *mockBatchWriter) Flush() error {
	return nil
}

type MockLedger struct {
	peers map[cid.CID][]kramaid.KramaID
}

func NewMockLedger() *MockLedger {
	return &MockLedger{
		peers: make(map[cid.CID][]kramaid.KramaID),
	}
}

func (mc *MockLedger) GetAssociatedPeers(id identifiers.Identifier, stateHash cid.CID) ([]kramaid.KramaID, error) {
	peers, ok := mc.peers[stateHash]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return peers, nil
}

func (mc *MockLedger) UpdateAssociatedPeers(
	id identifiers.Identifier,
	stateHash cid.CID,
	peerID kramaid.KramaID,
) error {
	peers, ok := mc.peers[stateHash]
	if ok {
		return common.ErrKeyNotFound
	}

	peers = append(peers, peerID)

	mc.peers[stateHash] = peers

	return nil
}

type MockNetwork struct {
	mtx sync.Mutex
	msg map[kramaid.KramaID]message.Message
}

func NewMockNetwork() *MockNetwork {
	return &MockNetwork{
		msg: make(map[kramaid.KramaID]message.Message),
	}
}

func (mn *MockNetwork) SendAgoraMessage(id kramaid.KramaID, msgType networkmsg.MsgType, msg message.Message) error {
	mn.mtx.Lock()
	defer mn.mtx.Unlock()

	mn.msg[id] = msg

	return nil
}

func WaitForResponse(ctx context.Context, respChan chan *message.Response) (*message.Response, error) {
	select {
	case <-ctx.Done():
		return nil, common.ErrTimeOut
	case resp := <-respChan:
		return resp, nil
	}
}

func WaitForResponseMsg(
	ctx context.Context,
	from kramaid.KramaID,
	network *MockNetwork,
) (*message.AgoraResponseMsg, error) {
	resp, err := tests.RetryUntilTimeout(ctx, 500*time.Millisecond, func() (interface{}, bool) {
		network.mtx.Lock()
		defer network.mtx.Unlock()

		msg, ok := network.msg[from]
		if !ok {
			return nil, true
		}

		return msg, false
	})
	if err != nil {
		return nil, err
	}

	msg, ok := resp.(*message.AgoraResponseMsg)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	return msg, nil
}

func randomCID(t *testing.T, contentType byte) cid.CID {
	t.Helper()

	var cid cid.CID

	cid[0] = contentType

	copy(cid[1:], tests.RandomHash(t).Bytes())

	return cid
}

package decision

import (
	"context"
	"sync"
	"testing"

	"github.com/sarvalabs/moichain/poorna/agora/db"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common/tests"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
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
		context.Background(),
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

func (db *MockDB) DoesStateExists(address types.Address, stateHash atypes.CID) bool {
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
		return nil, types.ErrKeyNotFound
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
	address types.Address,
	keys []atypes.CID,
) (map[atypes.CID][]byte, error) {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	resp := make(map[atypes.CID][]byte, len(keys))

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
	peers map[atypes.CID][]id.KramaID
}

func NewMockLedger() *MockLedger {
	return &MockLedger{
		peers: make(map[atypes.CID][]id.KramaID),
	}
}

func (mc *MockLedger) GetAssociatedPeers(addr types.Address, stateHash atypes.CID) ([]id.KramaID, error) {
	peers, ok := mc.peers[stateHash]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return peers, nil
}

func (mc *MockLedger) UpdateAssociatedPeers(addr types.Address, stateHash atypes.CID, peerID id.KramaID) error {
	peers, ok := mc.peers[stateHash]
	if ok {
		return types.ErrKeyNotFound
	}

	peers = append(peers, peerID)

	mc.peers[stateHash] = peers

	return nil
}

type MockNetwork struct {
	mtx sync.Mutex
	msg map[id.KramaID]atypes.Message
}

func NewMockNetwork() *MockNetwork {
	return &MockNetwork{
		msg: make(map[id.KramaID]atypes.Message),
	}
}

func (mn *MockNetwork) SendAgoraMessage(id id.KramaID, msgType ptypes.MsgType, msg atypes.Message) error {
	mn.mtx.Lock()
	defer mn.mtx.Unlock()

	mn.msg[id] = msg

	return nil
}

func WaitForResponse(ctx context.Context, respChan chan *atypes.Response) (*atypes.Response, error) {
	select {
	case <-ctx.Done():
		return nil, types.ErrTimeOut
	case resp := <-respChan:
		return resp, nil
	}
}

func WaitForResponseMsg(
	ctx context.Context,
	from id.KramaID,
	network *MockNetwork,
) (*atypes.AgoraResponseMsg, error) {
	resp, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
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

	msg, ok := resp.(*atypes.AgoraResponseMsg)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return msg, nil
}

func randomCID(t *testing.T, contentType byte) atypes.CID {
	t.Helper()

	var cid atypes.CID

	cid[0] = contentType

	copy(cid[1:], tests.RandomHash(t).Bytes())

	return cid
}

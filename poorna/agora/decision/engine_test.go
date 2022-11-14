package decision

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/tests"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/db"
	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/moichain/types"
)

func TestHandleRequest_StateNotAvailable(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	engine := NewTestEngine(
		context.Background(),
		newMockDB,
		newMockLedger,
		newMockNetwork,
		0,
		0,
		0,
	)

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	req := &Request{
		PeerID:    "TestPeer",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
	}

	engine.HandleRequest(req)

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	resp, err := WaitForResponse(ctx, engine.responses)
	require.NoError(t, err)

	require.Equal(t, resp.PeerID, req.PeerID)
	require.False(t, resp.Status)
}

func TestHandleRequest_RequestFromSamePeer(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	engine := NewTestEngine(
		context.Background(),
		newMockDB,
		newMockLedger,
		newMockNetwork,
		2,
		0,
		0,
	)

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	// Add the state hash to db
	newMockDB.Set(stateHash.Bytes(), []byte{0x00})

	req := &Request{
		PeerID:    "TestPeer",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
	}

	// Add the initial request
	if err := engine.requests.Push(req); err != nil {
		require.NoError(t, err)
	}

	// Add the request from same peer
	engine.HandleRequest(req)

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	resp, err := WaitForResponse(ctx, engine.responses)
	require.NoError(t, err)

	require.Equal(t, resp.PeerID, req.PeerID)
	require.False(t, resp.Status)
}

func TestHandleRequest_RequestQueueIsFull(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	// Set queue size to 0
	MaxQueueSize := 0

	engine := NewTestEngine(
		context.Background(),
		newMockDB,
		newMockLedger,
		newMockNetwork,
		MaxQueueSize,
		0,
		0,
	)

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	// Add the state hash to db
	newMockDB.Set(stateHash.Bytes(), []byte{0x00})

	req := &Request{
		PeerID:    "TestPeer",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
	}

	engine.HandleRequest(req)

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	resp, err := WaitForResponse(ctx, engine.responses)
	require.NoError(t, err)

	require.Equal(t, resp.PeerID, req.PeerID)
	require.False(t, resp.Status)
}

func TestHandleRequest_AssociatedPeers(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	// Set queue size to 0
	MaxQueueSize := 0

	engine := NewTestEngine(
		context.Background(),
		newMockDB,
		newMockLedger,
		newMockNetwork,
		MaxQueueSize,
		0,
		0,
	)

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	kramaIds := tests.GetTestKramaIDs(t, 1)

	// Add the state hash to db
	newMockDB.Set(stateHash.Bytes(), []byte{0x00})

	// update associated peers with test kramaIDs
	err := newMockLedger.UpdateAssociatedPeers(sessionID, stateHash, kramaIds[0])
	require.NoError(t, err)

	req := &Request{
		PeerID:    "TestPeer",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
	}

	engine.HandleRequest(req)

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	resp, err := WaitForResponse(ctx, engine.responses)
	require.NoError(t, err)

	// Check the requester kramaID
	require.Equal(t, resp.PeerID, req.PeerID)
	// Check the status
	require.False(t, resp.Status)
	// Check for the associated peers
	assert.Contains(t, resp.PeerSet, kramaIds[0])
}

func TestHandleRequest_ValidRequestAddedToQueue(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	// Set queue size to 1
	MaxQueueSize := 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewTestEngine(
		ctx,
		newMockDB,
		newMockLedger,
		newMockNetwork,
		MaxQueueSize,
		1,
		1,
	)

	engine.Start()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	kramaIds := tests.GetTestKramaIDs(t, 1)

	// add the state hash to db
	newMockDB.Set(stateHash.Bytes(), []byte{0x00})

	// update associated peers with test kramaIDs
	err := newMockLedger.UpdateAssociatedPeers(sessionID, stateHash, kramaIds[0])
	require.NoError(t, err)

	req := &Request{
		PeerID:    "TestPeer_1",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
	}

	engine.HandleRequest(req)
	// check if added request is available in the queue
	if !engine.requests.Contains(req.PeerID) {
		assert.Fail(t, "request not found in queue")
	}
}

func TestEngine_TimeOutRequest(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	// Set queue size to 1
	MaxQueueSize := 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewTestEngine(
		ctx,
		newMockDB,
		newMockLedger,
		newMockNetwork,
		MaxQueueSize,
		0,
		1,
	)

	engine.Start()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	kramaIds := tests.GetTestKramaIDs(t, 1)

	// add the state hash to db
	newMockDB.Set(stateHash.Bytes(), []byte{0x00})

	// update associated peers with test kramaIDs
	err := newMockLedger.UpdateAssociatedPeers(sessionID, stateHash, kramaIds[0])
	require.NoError(t, err)

	req := &Request{
		PeerID:    "TestPeer",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
		ReqTime:   time.Now().Add(time.Duration(-1010) * time.Millisecond), // set an invalid request time
	}

	engine.HandleRequest(req)

	// wait 2 secs for response
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// No response is received since the request time has crossed the limit i.e. 1 sec
	_, err = WaitForResponse(waitCtx, engine.responses)
	require.ErrorIs(t, err, types.ErrTimeOut)
}

func TestEngine_RequestWithEmptyWantList(t *testing.T) {
	newMockDB := NewMockDB()
	newMockLedger := NewMockLedger()
	newMockNetwork := NewMockNetwork()

	// Set queue size to 1
	MaxQueueSize := 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewTestEngine(
		ctx,
		newMockDB,
		newMockLedger,
		newMockNetwork,
		MaxQueueSize,
		1,
		1,
	)

	engine.Start()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)
	stateData := []byte("MOI-State")

	// add the state hash to db
	newMockDB.Set(stateHash.Bytes(), stateData)

	req := &Request{
		PeerID:    "TestPeer_3",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
		ReqTime:   time.Now(),
	}

	engine.HandleRequest(req)

	// wait 2 secs for response
	waitCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	msg, err := WaitForResponseMsg(waitCtx, req.PeerID, newMockNetwork)
	require.NoError(t, err)

	assert.Contains(t, msg.HaveList, stateData)
}

func NewTestEngine(
	ctx context.Context,
	db store,
	ledger ledger,
	network network,
	queueSize int,
	responseWorkerCount,
	requestWorkerCount int,
) *Engine {
	return &Engine{
		ctx:                 ctx,
		logger:              hclog.NewNullLogger(),
		requests:            NewRequestQueue(queueSize),
		requestWorkerCount:  requestWorkerCount,
		responseWorkerCount: responseWorkerCount,
		responses:           make(chan *atypes.Response),
		workSignal:          make(chan struct{}),
		db:                  db,
		ledger:              ledger,
		network:             network,
		metrics:             NilMetrics(),
	}
}

type mockDB struct {
	data map[string][]byte
}

func NewMockDB() *mockDB {
	return &mockDB{
		data: make(map[string][]byte),
	}
}

func (db *mockDB) DoesStateExists(stateHash types.Hash) bool {
	_, ok := db.data[string(stateHash.Bytes())]

	return ok
}

func (db *mockDB) Get(key []byte) ([]byte, error) {
	data, ok := db.data[string(key)]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return data, nil
}

func (db *mockDB) Set(key, value []byte) {
	db.data[string(key)] = value
}

func (db *mockDB) GetBatchWriter() db.BatchWriter {
	return &mockBatchWriter{db: db}
}

func (db *mockDB) GetData(ctx context.Context, keys []types.Hash) ([][]byte, error) {
	resp := make([][]byte, 0, len(keys))

	for _, key := range keys {
		data := db.data[string(key.Bytes())]
		resp = append(resp, data)
	}

	return resp, nil
}

type mockBatchWriter struct {
	db *mockDB
}

func (bw *mockBatchWriter) Set(key []byte, value []byte) error {
	bw.db.data[string(key)] = value

	return nil
}

func (bw *mockBatchWriter) Flush() error {
	return nil
}

type mockLedger struct {
	peers map[types.Hash][]id.KramaID
}

func NewMockLedger() *mockLedger {
	return &mockLedger{
		peers: make(map[types.Hash][]id.KramaID),
	}
}

func (mc *mockLedger) GetAssociatedPeers(addr types.Address, stateHash types.Hash) ([]id.KramaID, error) {
	peers, ok := mc.peers[stateHash]
	if !ok {
		return nil, types.ErrKeyNotFound
	}

	return peers, nil
}

func (mc *mockLedger) UpdateAssociatedPeers(addr types.Address, stateHash types.Hash, peerID id.KramaID) error {
	peers, ok := mc.peers[stateHash]
	if ok {
		return types.ErrKeyNotFound
	}

	peers = append(peers, peerID)

	mc.peers[stateHash] = peers

	return nil
}

type mockNetwork struct {
	msg map[id.KramaID]atypes.Message
}

func NewMockNetwork() *mockNetwork {
	return &mockNetwork{
		msg: make(map[id.KramaID]atypes.Message),
	}
}

func (mn *mockNetwork) SendAgoraMessage(id id.KramaID, msgType types.MsgType, msg atypes.Message) error {
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
	network *mockNetwork,
) (*atypes.AgoraResponseMsg, error) {
	resp, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
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

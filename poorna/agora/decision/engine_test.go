package decision

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
)

func TestHandleRequest_StateNotAvailable(t *testing.T) {
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())

	engine, _, _, _ := NewTest(
		t,
		nil,
		nil,
		// engine params call back
		func(e *Engine) {
			e.requestWorkerCount = 0
			e.responseWorkerCount = 0
		},
	)

	req := NewRequest(
		"TestPeer",
		sessionID,
		stateHash,
		nil,
		time.Time{},
	)

	engine.HandleRequest(req)

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	resp, err := WaitForResponse(ctx, engine.responses)
	require.NoError(t, err)

	require.Equal(t, resp.PeerID, req.PeerID)
	require.False(t, resp.Status)
}

func TestHandleRequest_RequestFromSamePeer(t *testing.T) {
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())

	engine, _, _, _ := NewTest(
		t,

		// db call back
		func(db *MockDB) {
			// add the state hash to db
			db.Set(stateHash.Bytes(), []byte{0x00})
		},

		// ledge call back
		nil,

		// engine params call back
		func(e *Engine) {
			e.requestWorkerCount = 0
			e.responseWorkerCount = 0
			e.requests = NewRequestQueue(2)
		},
	)

	req := NewRequest(
		"TestPeer",
		sessionID,
		stateHash,
		nil,
		time.Time{},
	)

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
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())

	engine, _, _, _ := NewTest(
		t,

		// db call back
		func(db *MockDB) {
			// Add the state hash to db
			db.Set(stateHash.Bytes(), []byte{0x00})
		},

		// ledge call back
		nil,

		// engine params call back
		func(e *Engine) {
			e.requestWorkerCount = 0
			e.responseWorkerCount = 0
		},
	)

	req := NewRequest(
		"TestPeer",
		sessionID,
		stateHash,
		nil,
		time.Time{},
	)

	engine.HandleRequest(req)

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFn()

	resp, err := WaitForResponse(ctx, engine.responses)
	require.NoError(t, err)

	require.Equal(t, resp.PeerID, req.PeerID)
	require.False(t, resp.Status)
}

func TestHandleRequest_AssociatedPeers(t *testing.T) {
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	kramaIds := tests.GetTestKramaIDs(t, 1)

	engine, _, _, _ := NewTest(
		t,

		// db call back
		func(db *MockDB) {
			// Add the state hash to db
			db.Set(stateHash.Bytes(), []byte{0x00})
		},

		// ledge call back
		func(l *MockLedger) {
			// update associated peers with test kramaIDs
			err := l.UpdateAssociatedPeers(sessionID, stateHash, kramaIds[0])
			require.NoError(t, err)
		},

		// engine params call back
		func(e *Engine) {
			e.requestWorkerCount = 0
			e.responseWorkerCount = 0
		},
	)

	req := NewRequest(
		"TestPeer",
		sessionID,
		stateHash,
		nil,
		time.Time{},
	)

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
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	kramaIds := tests.GetTestKramaIDs(t, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, _, _, _ := NewTest(
		t,

		// db call back
		func(db *MockDB) {
			// add the state hash to db
			db.Set(stateHash.Bytes(), []byte{0x00})
		},

		// ledge call back
		func(l *MockLedger) {
			// update associated peers with test kramaIDs
			err := l.UpdateAssociatedPeers(sessionID, stateHash, kramaIds[0])
			require.NoError(t, err)
		},

		// engine params call back
		func(e *Engine) {
			e.ctx = ctx
			e.requestWorkerCount = 0
			e.responseWorkerCount = 0
			e.requests = NewRequestQueue(1)
		},
	)

	engine.Start()

	req := NewRequest(
		"TestPeer_1",
		sessionID,
		stateHash,
		nil,
		time.Time{},
	)

	engine.HandleRequest(req)
	// check if added request is available in the queue
	if !engine.requests.Contains(req.PeerID) {
		assert.Fail(t, "request not found in queue")
	}
}

func TestEngine_TimeOutRequest(t *testing.T) {
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	kramaIds := tests.GetTestKramaIDs(t, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, _, _, _ := NewTest(
		t,

		// db call back
		func(db *MockDB) {
			// add the state hash to db
			db.Set(stateHash.Bytes(), []byte{0x00})
		},

		// ledge call back
		func(l *MockLedger) {
			// update associated peers with test kramaIDs
			err := l.UpdateAssociatedPeers(sessionID, stateHash, kramaIds[0])
			require.NoError(t, err)
		},

		// engine params call back
		func(e *Engine) {
			e.ctx = ctx
			e.requestWorkerCount = 1
			e.responseWorkerCount = 0
			e.requests = NewRequestQueue(1)
		},
	)

	req := &Request{
		PeerID:    "TestPeer",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
		ReqTime:   time.Now().Add(time.Duration(-1010) * time.Millisecond), // set an invalid request time
	}

	engine.Start()

	engine.HandleRequest(req)

	// wait 2 secs for response
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// No response is received since the request time has crossed the limit i.e. 1 sec
	_, err := WaitForResponse(waitCtx, engine.responses)
	require.ErrorIs(t, err, types.ErrTimeOut)
}

func TestEngine_RequestWithEmptyWantList(t *testing.T) {
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, dhruva.Account.Byte())
	stateData := []byte("MOI-State")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, _, _, mockNetwork := NewTest(
		t,

		// db call back
		func(db *MockDB) {
			// add the state hash to db
			db.Set(stateHash.Bytes(), stateData)
		},
		nil,
		// engine params call back
		func(e *Engine) {
			e.ctx = ctx
			e.requestWorkerCount = 1
			e.responseWorkerCount = 1
			e.requests = NewRequestQueue(1)
		},
	)

	req := &Request{
		PeerID:    "TestPeer_3",
		SessionID: sessionID,
		StateHash: stateHash,
		WantList:  nil,
		ReqTime:   time.Now(),
	}

	engine.Start()

	engine.HandleRequest(req)

	// wait 2 secs for response
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	msg, err := WaitForResponseMsg(waitCtx, req.PeerID, mockNetwork)
	require.NoError(t, err)

	assert.Contains(t, msg.HaveList, atypes.NewBlockFromRawData(dhruva.Account.Byte(), stateData).BytesForMessage())
}

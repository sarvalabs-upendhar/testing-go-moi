package session

import (
	"context"
	"crypto/rand"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/common/tests"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"log"
	"testing"
	"time"
)

func TestHandleMessage_UpdatePeerStatus(t *testing.T) {
	network := NewMockNetwork()
	mockSessionManager := NewMockSessionManager()
	notifier := types.NewNotifier()
	mockInterestManager := NewInterestManager()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewTestSession(
		ctx, sessionID,
		hclog.NewNullLogger(),
		stateHash,
		network,
		notifier,
		mockInterestManager,
		mockSessionManager,
		peerID)

	// set the status to true
	success := session.pm.UpdatePeerStatus(peerID, true)
	require.True(t, success)

	resp := &types.AgoraResponseMsg{
		SessionID: sessionID,
		Status:    false,
		HaveList:  nil,
		PeerSet:   nil,
	}
	// create a buffered channel to store the input
	peerSignal := make(chan bool, 1)

	timedCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Wait for peer response until timeout
	WaitForPeerResponse(t, timedCtx, session.pm.PeerRespChan(peerID), peerSignal)

	session.HandleMessage(peerID, resp)

	// check peerSignal
	require.False(t, <-peerSignal)

	// peer status should be false
	require.False(t, session.pm.PeerStatus(peerID))
}

func TestHandleMessage_UpdatePeerSet(t *testing.T) {
	network := NewMockNetwork()
	sessionManager := NewMockSessionManager()
	notifier := types.NewNotifier()
	interestManager := NewInterestManager()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewTestSession(
		ctx,
		sessionID,
		hclog.NewNullLogger(),
		stateHash,
		network,
		notifier,
		interestManager,
		sessionManager,
		peerID,
	)

	// set the status to true
	success := session.pm.UpdatePeerStatus(peerID, true)
	require.True(t, success)

	// create kramaIds for peerSet
	peerSet := tests.GetTestKramaIDs(t, 1)

	resp := &types.AgoraResponseMsg{
		SessionID: sessionID,
		Status:    false,
		HaveList:  nil,
		PeerSet:   peerSet,
	}
	// create a buffered channel to store the input
	peerSignal := make(chan bool, 1)

	timedCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Wait for peer response until timeout
	WaitForPeerResponse(t, timedCtx, session.pm.PeerRespChan(peerID), peerSignal)

	session.HandleMessage(peerID, resp)

	// check peerSignal
	require.False(t, <-peerSignal)

	// check if the suggested peers are added

	require.Contains(t, session.pm.peers, peerSet[0])
}

func TestGetBlocks_RecordSessionInterest(t *testing.T) {
	network := NewMockNetwork()
	sessionManager := NewMockSessionManager()
	notifier := types.NewNotifier()
	interestManager := NewInterestManager()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewTestSession(
		ctx,
		sessionID,
		hclog.NewNullLogger(),
		stateHash,
		network,
		notifier,
		interestManager,
		sessionManager,
		peerID,
	)

	idSet, blocks := GetDummyBlocks(t, 3)

	outChan := make(chan *types.Block, 3) // Create a buffered channel to avoid blocking

	timedCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		err := session.getBlocks(timedCtx, peerID, outChan, idSet)
		require.NoError(t, err)
	}()

	keys := idSet.Keys()
	// check if session interests are recorded
	recorded := AreSessionInterestRecorded(ctx, interestManager, sessionID, keys)
	require.True(t, recorded)

	// publish the blocks
	for _, block := range blocks {
		notifier.Publish(block)
	}

	removed := AreSessionInterestRemoved(ctx, interestManager, sessionID, keys)
	require.True(t, removed)
}

func TestGetBlock(t *testing.T) {
	network := NewMockNetwork()
	sessionManager := NewMockSessionManager()
	notifier := types.NewNotifier()
	interestManager := NewInterestManager()

	sessionID := tests.RandomAddress(t)
	stateHash := tests.RandomHash(t)

	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewTestSession(
		ctx,
		sessionID,
		hclog.NewNullLogger(),
		stateHash,
		network,
		notifier,
		interestManager,
		sessionManager,
		peerID,
	)

	idSet, blocks := GetDummyBlocks(t, 3)

	timedCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	outChan := session.GetBlocks(timedCtx, idSet.Keys())

	// wait  until getblocks process the request
	time.Sleep(1 * time.Second)

	for _, block := range blocks {
		notifier.Publish(block)
	}

	receivedBlockCount := WaitForBlocks(ctx, outChan, idSet)

	require.Equal(t, 3, receivedBlockCount)
}

func NewTestSession(
	ctx context.Context,
	addr ktypes.Address,
	logger hclog.Logger,
	stateHash ktypes.Hash,
	network sessionNetwork,
	notifier types.PubSub,
	im sessionInterestManager,
	sm sessionManager,
	contextPeers ...id.KramaID,
) *Session {
	return NewSession(ctx, addr, logger, stateHash, network, notifier, im, sm, contextPeers)
}

type mockSessionManager struct {
	sessions map[ktypes.Address]interface{}
}

func NewMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[ktypes.Address]interface{}),
	}
}

func (msm *mockSessionManager) CloseSession(id ktypes.Address) {
	delete(msm.sessions, id)
}

type mockNetwork struct {
	msg map[id.KramaID]types.Message
}

func NewMockNetwork() *mockNetwork {
	return &mockNetwork{
		msg: make(map[id.KramaID]types.Message),
	}
}

func (mn *mockNetwork) SendAgoraMessage(id id.KramaID, msgType ktypes.MsgType, msg types.Message) error {
	mn.msg[id] = msg

	return nil
}

func (mn *mockNetwork) ClosePeerSession(id id.KramaID, sessionID ktypes.Address) error {
	return nil
}

func WaitForPeerResponse(t *testing.T, ctx context.Context, peerRespChan <-chan bool, out chan bool) {
	t.Helper()

	go func() {
		select {
		case <-ctx.Done():
			assert.Fail(t, "Timeout occurred")

			return
		case resp := <-peerRespChan:
			out <- resp

			return
		}
	}()
}

func GetDummyBlocks(t *testing.T, count int) (*kutils.Set, map[ktypes.Hash]types.Block) {
	t.Helper()

	set := kutils.NewSet()
	blocks := make(map[ktypes.Hash]types.Block, count)

	for i := 0; i < count; i++ {
		rawBytes := make([]byte, 64)
		_, err := rand.Read(rawBytes)
		require.NoError(t, err)

		block := types.NewBlock(rawBytes)
		set.Add(block.GetID())
		blocks[block.GetID()] = block
	}

	return set, blocks
}

func AreSessionInterestRecorded(
	ctx context.Context,
	im *InterestManager,
	sessionID ktypes.Address,
	keys []ktypes.Hash,
) bool {
	status, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		for _, hash := range keys {
			data, ok := im.wants[hash]

			if !ok || !data[sessionID] {
				return nil, true
			}
		}

		return true, false
	})

	if err != nil {
		return false
	}

	keysRecorded, ok := status.(bool)
	if !ok {
		return false
	}

	return keysRecorded
}

func AreSessionInterestRemoved(
	ctx context.Context,
	im *InterestManager,
	sessionID ktypes.Address,
	keys []ktypes.Hash,
) bool {
	status, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		for _, hash := range keys {
			data, ok := im.wants[hash]
			if ok && data[sessionID] {
				return nil, true
			}
		}

		return true, false
	})

	if err != nil {
		return false
	}

	keysRemoved, ok := status.(bool)
	if !ok {
		return false
	}

	return keysRemoved
}

func WaitForBlocks(ctx context.Context, blocks chan *types.Block, ids *kutils.Set) (receivedCount int) {
	for {
		select {
		case <-ctx.Done():
		case block, ok := <-blocks:
			if !ok {
				log.Println("channel closed")

				return
			}

			if !ids.Has(block.GetID()) {
				return
			}

			receivedCount++
		default:
			if receivedCount == ids.Len() {
				return
			}
		}
	}
}

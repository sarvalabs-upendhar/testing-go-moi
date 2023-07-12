package session

import (
	"context"
	"testing"
	"time"

	atypes "github.com/sarvalabs/moichain/syncer/agora/block"
	"github.com/sarvalabs/moichain/syncer/agora/message"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/storage"
)

func TestHandleMessage_UpdatePeerStatus(t *testing.T) {
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, storage.Account.Byte())
	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, _, _ := NewTestSession(
		ctx, sessionID,
		stateHash,
		peerID,
	)

	// set the status to true
	success := session.pm.UpdatePeerStatus(peerID, true)
	require.True(t, success)

	resp := &message.AgoraResponseMsg{
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
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, storage.Account.Byte())
	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, _, _ := NewTestSession(
		ctx,
		sessionID,
		stateHash,
		peerID,
	)

	// set the status to true
	success := session.pm.UpdatePeerStatus(peerID, true)
	require.True(t, success)

	// create kramaIds for peerSet
	peerSet := tests.GetTestKramaIDs(t, 1)

	resp := &message.AgoraResponseMsg{
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
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, storage.Account.Byte())
	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, interestManager, notifier := NewTestSession(
		ctx,
		sessionID,
		stateHash,
		peerID,
	)

	idSet, blocks := GetDummyBlocks(t, 3)

	outChan := make(chan *atypes.Block, 3) // Create a buffered channel to avoid blocking

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
	sessionID := tests.RandomAddress(t)
	stateHash := randomCID(t, storage.Account.Byte())
	peerID := tests.GetTestKramaIDs(t, 1)[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, _, notifier := NewTestSession(
		ctx,
		sessionID,
		stateHash,
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

package session

import (
	"context"
	"crypto/rand"
	"log"
	"testing"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/moichain/common/tests"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewTestPeerManager(sessionID types.Address, network sessionNetwork) *PeerManager {
	return NewSessionPeerManager(sessionID, hclog.NewNullLogger(), network)
}

func NewTestSession(
	ctx context.Context,
	addr types.Address,
	stateHash atypes.CID,
	contextPeers ...id.KramaID,
) (*Session, *InterestManager, atypes.PubSub) {
	interestManager := NewInterestManager()
	notifier := atypes.NewNotifier()

	return NewSession(
		ctx,
		addr,
		hclog.NewNullLogger(),
		stateHash,
		NewMockNetwork(),
		notifier,
		interestManager,
		NewMockSessionManager(),
		contextPeers,
	), interestManager, notifier
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

func GetDummyBlocks(t *testing.T, count int) (*atypes.CIDSet, map[atypes.CID]atypes.Block) {
	t.Helper()

	set := atypes.NewHashSet()
	blocks := make(map[atypes.CID]atypes.Block, count)

	for i := 0; i < count; i++ {
		rawBytes := make([]byte, 64)
		_, err := rand.Read(rawBytes)
		require.NoError(t, err)

		block := atypes.NewBlockFromRawData(0x00, rawBytes)
		set.Add(block.GetCid())
		blocks[block.GetCid()] = block
	}

	return set, blocks
}

func AreSessionInterestRecorded(
	ctx context.Context,
	im *InterestManager,
	sessionID types.Address,
	keys []atypes.CID,
) bool {
	status, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		im.mutex.Lock()
		defer im.mutex.Unlock()

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
	sessionID types.Address,
	keys []atypes.CID,
) bool {
	status, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		im.mutex.Lock()
		defer im.mutex.Unlock()
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

func WaitForBlocks(ctx context.Context, blocks chan *atypes.Block, ids *atypes.CIDSet) (receivedCount int) {
	for {
		select {
		case <-ctx.Done():
		case block, ok := <-blocks:
			if !ok {
				log.Println("channel closed")

				return
			}

			if !ids.Has(block.GetCid()) {
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

func appendBlocks(set1, set2 map[atypes.CID]atypes.Block) []atypes.Block {
	blocks := make([]atypes.Block, 0, len(set1)+len(set2))

	for _, v := range set1 {
		blocks = append(blocks, v)
	}

	for _, v := range set2 {
		blocks = append(blocks, v)
	}

	return blocks
}

type mockSessionManager struct {
	sessions map[types.Address]interface{}
}

func NewMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[types.Address]interface{}),
	}
}

func (msm *mockSessionManager) CloseSession(id types.Address) {
	delete(msm.sessions, id)
}

type mockNetwork struct {
	msg map[id.KramaID]atypes.Message
}

func NewMockNetwork() *mockNetwork {
	return &mockNetwork{
		msg: make(map[id.KramaID]atypes.Message),
	}
}

func (mn *mockNetwork) SendAgoraMessage(id id.KramaID, msgType ptypes.MsgType, msg atypes.Message) error {
	mn.msg[id] = msg

	return nil
}

func (mn *mockNetwork) ClosePeerSession(id id.KramaID, sessionID types.Address) error {
	return nil
}

func randomCID(t *testing.T, contentType byte) atypes.CID {
	t.Helper()

	var cid atypes.CID

	cid[0] = contentType

	copy(cid[1:], tests.RandomHash(t).Bytes())

	return cid
}

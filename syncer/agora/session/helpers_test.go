package session

import (
	"context"
	"crypto/rand"
	"log"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
	"github.com/sarvalabs/go-moi/syncer/agora/notifications"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

func NewTestPeerManager(sessionID identifiers.Identifier, network sessionNetwork) *PeerManager {
	return NewSessionPeerManager(sessionID, hclog.NewNullLogger(), network)
}

func NewTestSession(
	ctx context.Context,
	id identifiers.Identifier,
	stateHash cid.CID,
	contextPeers ...identifiers.KramaID,
) (*Session, *InterestManager, notifications.PubSubNotifier) {
	interestManager := NewInterestManager()
	notifier := notifications.NewNotifier()

	return NewSession(
		ctx,
		id,
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
			out <- true

			return
		case resp := <-peerRespChan:
			out <- resp

			return
		}
	}()
}

func GetDummyBlocks(t *testing.T, count int) (*cid.CIDSet, map[cid.CID]block.Block) {
	t.Helper()

	set := cid.NewHashSet()
	blocks := make(map[cid.CID]block.Block, count)

	for i := 0; i < count; i++ {
		rawBytes := make([]byte, 64)
		_, err := rand.Read(rawBytes)
		require.NoError(t, err)

		block := block.NewAccountBlockFromRawData(0x00, rawBytes)
		set.Add(block.GetCid())
		blocks[block.GetCid()] = block
	}

	return set, blocks
}

func removeSession(im *InterestManager, id identifiers.Identifier) []cid.CID {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// The keys that no session is interested in
	deletedKeys := make([]cid.CID, 0)

	// For each known key
	for c := range im.wants {
		deleteSession(c, im.wants, id, &deletedKeys)
	}

	return deletedKeys
}

func AreSessionInterestRecorded(
	ctx context.Context,
	im *InterestManager,
	sessionID identifiers.Identifier,
	keys []cid.CID,
) bool {
	status, err := tests.RetryUntilTimeout(ctx, 500*time.Millisecond, func() (interface{}, bool) {
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
	sessionID identifiers.Identifier,
	keys []cid.CID,
) bool {
	status, err := tests.RetryUntilTimeout(ctx, 500*time.Millisecond, func() (interface{}, bool) {
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

func WaitForBlocks(ctx context.Context, blocks chan *block.Block, ids *cid.CIDSet) (receivedCount int) {
	for {
		select {
		case <-ctx.Done():
			return
		case blk, ok := <-blocks:
			if !ok {
				log.Println("channel closed")

				return
			}

			if !ids.Has(blk.GetCid()) {
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

func appendBlocks(set1, set2 map[cid.CID]block.Block) []block.Block {
	blocks := make([]block.Block, 0, len(set1)+len(set2))

	for _, v := range set1 {
		blocks = append(blocks, v)
	}

	for _, v := range set2 {
		blocks = append(blocks, v)
	}

	return blocks
}

type mockSessionManager struct {
	sessions map[identifiers.Identifier]interface{}
}

func NewMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[identifiers.Identifier]interface{}),
	}
}

func (msm *mockSessionManager) CloseSession(id identifiers.Identifier) {
	delete(msm.sessions, id)
}

type mockNetwork struct {
	msg map[identifiers.KramaID]message.Message
}

func NewMockNetwork() *mockNetwork {
	return &mockNetwork{
		msg: make(map[identifiers.KramaID]message.Message),
	}
}

func (mn *mockNetwork) SendAgoraMessage(id identifiers.KramaID, msgType networkmsg.MsgType, msg message.Message) error {
	mn.msg[id] = msg

	return nil
}

func (mn *mockNetwork) ClosePeerSession(id identifiers.KramaID, sessionID identifiers.Identifier) error {
	return nil
}

func randomCID(t *testing.T, contentType byte) cid.CID {
	t.Helper()

	var cid cid.CID

	cid[0] = contentType

	copy(cid[1:], tests.RandomHash(t).Bytes())

	return cid
}

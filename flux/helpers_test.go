package flux

import (
	"context"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/stretchr/testify/require"
)

func getTestEntries(t *testing.T, kramaIDs []kramaid.KramaID) map[peer.ID]*senatus.NodeMetaInfo {
	t.Helper()

	count := len(kramaIDs)
	entries := make(map[peer.ID]*senatus.NodeMetaInfo, count)

	for i := 0; i < count; i++ {
		peerID, _ := kramaIDs[i].DecodedPeerID()
		entries[peerID] = &senatus.NodeMetaInfo{
			KramaID: kramaIDs[i],
		}
	}

	return entries
}

type MockReputationEngine struct {
	data      map[peer.ID][]byte
	peerCount uint64
}

func NewMockReputationEngine() *MockReputationEngine {
	return &MockReputationEngine{
		data: make(map[peer.ID][]byte),
	}
}

func (re *MockReputationEngine) setEntries(t *testing.T, entries map[peer.ID]*senatus.NodeMetaInfo) {
	t.Helper()

	for peerID, nodeMetaInfo := range entries {
		metaInfo, err := nodeMetaInfo.Bytes()
		require.NoError(t, err)

		re.data[peerID] = metaInfo
	}
}

func (re *MockReputationEngine) StreamPeerInfos(ctx context.Context) (chan *senatus.PeerInfo, error) {
	entries := make(chan *senatus.PeerInfo)

	go func() {
		for peerID, metaInfo := range re.data {
			entries <- &senatus.PeerInfo{
				ID:   peerID,
				Data: metaInfo,
			}
		}

		close(entries)
	}()

	return entries, nil
}

func (re *MockReputationEngine) setPeerCount(count uint64) {
	re.peerCount = count
}

func (re *MockReputationEngine) TotalPeerCount() uint64 {
	return re.peerCount
}

type MockServer struct {
	bootstrapPeerIDs map[peer.ID]bool
}

func NewMockServer() *MockServer {
	return &MockServer{
		bootstrapPeerIDs: make(map[peer.ID]bool),
	}
}

func (m *MockServer) GetPeersCount() int {
	// TODO implement me
	panic("implement me")
}

func (m *MockServer) setBootstrapPeerIDs(peerIDs []peer.ID) {
	for _, peerID := range peerIDs {
		m.bootstrapPeerIDs[peerID] = true
	}
}

func (m *MockServer) GetBootstrapPeerIDs() map[peer.ID]bool {
	return m.bootstrapPeerIDs
}

type createRandomizerParams struct {
	server             *MockServer
	repEngine          *MockReputationEngine
	serverCallback     func(server *MockServer)
	repEngineCallback  func(reputationEngine *MockReputationEngine)
	randomizerCallback func(randomizer *Randomizer)
}

func createTestRandomizer(t *testing.T, params *createRandomizerParams) *Randomizer {
	t.Helper()

	var (
		server    = NewMockServer()
		repEngine = NewMockReputationEngine()
	)

	if params == nil {
		params = &createRandomizerParams{}
	}

	if params.server != nil {
		server = params.server
	}

	if params.serverCallback != nil {
		params.serverCallback(server)
	}

	if params.repEngine != nil {
		repEngine = params.repEngine
	}

	if params.repEngineCallback != nil {
		params.repEngineCallback(repEngine)
	}

	randomizer := NewRandomizer(
		hclog.NewNullLogger(),
		server,
		repEngine,
		NilMetrics(),
	)

	if params.randomizerCallback != nil {
		params.randomizerCallback(randomizer)
	}

	return randomizer
}

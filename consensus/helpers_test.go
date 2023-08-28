package consensus

import (
	"context"
	"sync"
	"testing"

	"github.com/sarvalabs/go-moi/crypto"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/network/rpc"
)

type MockServer struct {
	subscribers map[string]context.Context
	peers       []id.KramaID
	peersLock   sync.RWMutex
}

func NewMockServer() *MockServer {
	return &MockServer{
		subscribers: make(map[string]context.Context),
		peers:       make([]id.KramaID, 0),
		peersLock:   sync.RWMutex{},
	}
}

func (m *MockServer) Unsubscribe(topic string) error {
	delete(m.subscribers, topic)

	return nil
}

func (m *MockServer) Broadcast(topic string, data []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockServer) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	m.subscribers[topic] = ctx

	return nil
}

func (m *MockServer) StartNewRPCServer(protocol protocol.ID) *rpc.Client {
	// TODO implement me
	panic("implement me")
}

func (m *MockServer) ConnectPeer(kramaID id.KramaID) error {
	for _, peer := range m.peers {
		if peer == kramaID {
			return common.ErrConnectionExists
		}
	}

	m.peers = append(m.peers, kramaID)

	return nil
}

func (m *MockServer) DisconnectPeer(kramaID id.KramaID) error {
	m.peersLock.Lock()
	defer m.peersLock.Unlock()

	indexOf := func(peerID id.KramaID) int {
		for i, peer := range m.peers {
			if peer == kramaID {
				return i
			}
		}

		return -1
	}

	if index := indexOf(kramaID); index != -1 {
		m.peers = append(m.peers[:index], m.peers[index+1:]...)
	}

	return nil
}

func (m *MockServer) RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error {
	// TODO implement me
	panic("implement me")
}

func (m *MockServer) GetKramaID() id.KramaID {
	// TODO implement me
	panic("implement me")
}

// func (m *MockServer) getPeers() []id.KramaID {
//	m.peersLock.RLock()
//	defer m.peersLock.RUnlock()
//
//	return m.peers
// }

type MockEngine struct {
	logger   hclog.Logger
	requests chan Request
}

func processRequests(requestsChan <-chan Request) {
	go func() {
		request := <-requestsChan
		request.responseChan <- Response{}
	}()
}

func (k *MockEngine) Requests() chan Request {
	processRequests(k.requests)

	return k.requests
}

func (k *MockEngine) Logger() hclog.Logger {
	return k.logger
}

func getRawInteraction(t *testing.T, ixData common.IxData, sign []byte) []byte {
	t.Helper()

	ix, err := common.NewInteraction(ixData, sign)
	require.NoError(t, err)

	rawInteractions, err := polo.Polorize([]*common.Interaction{ix})
	require.NoError(t, err)

	return rawInteractions
}

func getSignature(t *testing.T, kramaID id.KramaID, rawInteractions []byte, vault *crypto.KramaVault) ([]byte, []byte) {
	t.Helper()

	canonicalICSReq := networkmsg.CanonicalICSRequest{
		Operator: string(kramaID),
		IxData:   rawInteractions,
	}

	rawCanonicalICSReq, err := polo.Polorize(canonicalICSReq)
	require.NoError(t, err)

	icsReqSign, err := vault.Sign(rawCanonicalICSReq, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	require.NoError(t, err)

	return rawCanonicalICSReq, icsReqSign
}

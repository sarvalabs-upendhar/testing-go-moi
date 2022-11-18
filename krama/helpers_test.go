package krama

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/protocol"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/moirpc"
	"gitlab.com/sarvalabs/moichain/types"
)

type MockServer struct {
	subscribers map[string]context.Context
	peers       []id.KramaID
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

func (m *MockServer) InitNewRPCServer(protocol protocol.ID) *moirpc.Client {
	// TODO implement me
	panic("implement me")
}

func (m *MockServer) ConnectPeer(kramaID id.KramaID) error {
	for _, peer := range m.peers {
		if peer == kramaID {
			return types.ErrConnectionExists
		}
	}

	m.peers = append(m.peers, kramaID)

	return nil
}

func (m *MockServer) DisconnectPeer(kramaID id.KramaID) error {
	indexOf := func(kramaID id.KramaID) int {
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

func NewMockServer() *MockServer {
	return &MockServer{
		subscribers: make(map[string]context.Context),
		peers:       make([]id.KramaID, 0),
	}
}

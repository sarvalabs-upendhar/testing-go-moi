package rpc

import (
	"context"
	"testing"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/crypto"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

type MockConnectionManager struct {
	host host.Host
}

func NewMockConnectionManager(hostAddr string) *MockConnectionManager {
	networkKey, _ := identifiers.RandomNetworkKey()

	nPriv := new(crypto.SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)

	prvKey, _ := p2pcrypto.UnmarshalSecp256k1PrivateKey(nPriv.Bytes())

	host, _ := libp2p.New(
		libp2p.ListenAddrStrings(hostAddr),
		libp2p.Identity(prvKey),
	)

	return &MockConnectionManager{
		host: host,
	}
}

func (mcm *MockConnectionManager) NewStream(
	ctx context.Context,
	id peer.ID,
	protocol protocol.ID,
	tag string,
) (network.Stream, error) {
	stream, err := mcm.host.NewStream(ctx, id, protocol)
	if err != nil {
		return nil, err
	}

	// Protect the peer connection
	mcm.host.ConnManager().Protect(stream.Conn().RemotePeer(), tag)

	return stream, nil
}

func (mcm *MockConnectionManager) SetupStreamHandler(protocolID protocol.ID, tag string, handle func(network.Stream)) {
	mcm.host.SetStreamHandler(protocolID, func(stream network.Stream) {
		mcm.host.ConnManager().Protect(stream.Conn().RemotePeer(), tag)

		handle(stream)
	})
}

func (mcm *MockConnectionManager) ResetStream(stream network.Stream, tag string) error {
	// Release peer connection protection
	mcm.host.ConnManager().Unprotect(stream.Conn().RemotePeer(), tag)

	if err := stream.Reset(); err != nil {
		return err
	}

	return nil
}

func (mcm *MockConnectionManager) CloseStream(stream network.Stream, tag string) error {
	// Release peer connection protection
	mcm.host.ConnManager().Unprotect(stream.Conn().RemotePeer(), tag)

	if err := stream.Close(); err != nil {
		return err
	}

	return nil
}

func (mcm *MockConnectionManager) GetAddrsFromPeerStore(peerID peer.ID) []multiaddr.Multiaddr {
	return mcm.host.Peerstore().Addrs(peerID)
}

func (mcm *MockConnectionManager) AddToPeerStore(peerInfo *peer.AddrInfo) {
	mcm.host.Peerstore().AddAddr(peerInfo.ID, peerInfo.Addrs[0], peerstore.PermanentAddrTTL)
}

func (mcm *MockConnectionManager) GetHostPeerID() peer.ID {
	return mcm.host.ID()
}

func createConnectionMangers(t *testing.T, count int) []*MockConnectionManager {
	t.Helper()

	connManagers := make([]*MockConnectionManager, 0)

	for i := 0; i < count; i++ {
		cm := NewMockConnectionManager("/ip4/127.0.0.1/tcp/0")
		connManagers = append(connManagers, cm)
	}

	for i := 0; i < count; i++ {
		for j := 0; j < count; j++ {
			if i != j {
				connManagers[i].host.Peerstore().AddAddrs(
					connManagers[j].host.ID(),
					connManagers[j].host.Addrs(),
					peerstore.PermanentAddrTTL,
				)
			}
		}
	}

	return connManagers
}

func stopConnectionManagers(t *testing.T, nodes []*MockConnectionManager) {
	t.Helper()

	for _, cm := range nodes {
		err := cm.host.Close()
		require.NoError(t, err)
	}
}

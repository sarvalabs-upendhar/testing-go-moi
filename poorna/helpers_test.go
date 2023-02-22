package poorna

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	mrand "math/rand"

	errors "github.com/pkg/errors"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"

	"github.com/multiformats/go-multiaddr"

	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	kcrypto "github.com/sarvalabs/moichain/mudra"
	mudracommon "github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

var (
	SignedData  = []byte{1, 2, 3}
	emptyParams = &CreateServerParams{}
)

type MockVault struct {
	networkPrivateKey kcrypto.PrivateKey // Private key used in p2p communication
}

type MockReputationEngine struct {
	ntq map[kramaid.KramaID]int32
}

func (m *MockReputationEngine) GetAddress(key kramaid.KramaID) (multiAddrs []multiaddr.Multiaddr, err error) {
	// TODO implement me
	panic("implement me")
}

type CreateServerParams struct {
	ConfigCallback func(c *common.NetworkConfig) // Additional logic that needs to be executed on the configuration
	// Additional logic that needs to be executed on the server before starting
	ServerCallback func(server *Server)
	Logger         hclog.Logger
	EventMux       *utils.TypeMux
}

type TestMessage struct {
	message string
}

func (msg TestMessage) Bytes() ([]byte, error) {
	data, err := polo.Polorize(msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize interaction message")
	}

	return data, nil
}

func (msg *TestMessage) FromBytes(data []byte) error {
	if err := polo.Depolorize(msg, data); err != nil {
		return errors.Wrap(err, "failed to depolorize interaction message")
	}

	return nil
}

func getInvalidHandshakeMsg() *TestMessage {
	return &TestMessage{message: "xyz"}
}

func getParamsToCreateMultipleServers(
	t *testing.T,
	count int,
	bootNodes []host.Host,
	inboundConnLimit uint,
	outboundConnLimit uint,
) []*CreateServerParams {
	t.Helper()

	var arrayOfParams []*CreateServerParams

	for i := 0; i < count; i++ {
		arrayOfParams = append(arrayOfParams, &CreateServerParams{
			EventMux: &utils.TypeMux{},
			ConfigCallback: func(c *common.NetworkConfig) {
				c.BootstrapPeers = getMultiAddresses(t, bootNodes)
				c.InboundConnLimit = inboundConnLimit
				c.OutboundConnLimit = outboundConnLimit
			},
			ServerCallback: func(s *Server) {
				m := NewMockReputationEngine()
				m.SetNTQ(s.GetKramaID(), getRandomNumber(t, 27))
				s.Senatus = m
			},
			Logger: testLogger,
		})
	}

	return arrayOfParams
}

func getRandomMultiAddrArray(t *testing.T) []multiaddr.Multiaddr {
	t.Helper()

	randomMultiAddressArray := make([]multiaddr.Multiaddr, 1)
	randomAddress, err := multiaddr.NewMultiaddr(
		"/ip4/127.0.0.1/tcp/60876/p2p/QmaQW6kQeQS6huPBzDH3LxbhPfwLYcQrUspYpsP4hKZoGj",
	)

	require.NoError(t, err)

	randomMultiAddressArray[0] = randomAddress

	return randomMultiAddressArray
}

func NewMockReputationEngine() *MockReputationEngine {
	return &MockReputationEngine{
		ntq: make(map[kramaid.KramaID]int32),
	}
}

func (m *MockReputationEngine) SenatusHandler(msg *pubsub.Message) error {
	return nil
}

func (m *MockReputationEngine) GetNTQ(id kramaid.KramaID) (int32, error) {
	ntq, ok := m.ntq[id]
	if !ok {
		return -1, types.ErrKeyNotFound
	}

	return ntq, nil
}

func (m *MockReputationEngine) SetNTQ(id kramaid.KramaID, val int) {
	m.ntq[id] = int32(val)
}

func (vault *MockVault) GetNetworkPrivateKey() kcrypto.PrivateKey {
	return vault.networkPrivateKey
}

func (vault *MockVault) Sign(data []byte, sigType mudracommon.SigType) ([]byte, error) {
	return SignedData, nil
}

func (vault *MockVault) SetNetworkPrivateKey(t *testing.T, privKeyBytes []byte) {
	t.Helper()

	networkPrivKey := new(kcrypto.SECP256K1PrivKey)
	networkPrivKey.UnMarshal(privKeyBytes)
	vault.networkPrivateKey = networkPrivKey
}

func setServerPeers(t *testing.T, s *Server, peersList []kramaid.KramaID) {
	t.Helper()

	for _, peer := range peersList {
		peerID, err := peer.DecodedPeerID()
		require.NoError(t, err)
		s.Peers.addPeer(&Peer{kramaID: peer, networkID: peerID})
	}
}

// helper functions
func getPeerInfo(t *testing.T, server *Server) *libp2pPeer.AddrInfo {
	t.Helper()

	address := server.GetAddrs()
	networkID, err := server.id.PeerID()
	require.NoError(t, err)

	peerID, err := libp2pPeer.Decode(networkID)
	require.NoError(t, err)

	return &libp2pPeer.AddrInfo{
		ID:    peerID,
		Addrs: address,
	}
}

// getPeer only network ID is initialized
func getPeer(t *testing.T, s *Server, destServer *Server) *Peer {
	t.Helper()

	var stream network.Stream
	stream, err := s.NewStream(s.ctx, destServer.host.ID(), s.cfg.ProtocolID)
	require.NoError(t, err)
	// Create new kip peer
	return newPeer(stream, s.logger)
}

func getMultiAddresses(t *testing.T, hosts []host.Host) []multiaddr.Multiaddr {
	t.Helper()

	addresses := make([]multiaddr.Multiaddr, len(hosts))

	for i, h := range hosts {
		address := h.Addrs()[0].String()
		hostID := h.ID().String()
		p2pAddress := address + "/p2p/" + hostID
		multiAddress, err := multiaddr.NewMultiaddr(p2pAddress)
		require.NoError(t, err)

		addresses[i] = multiAddress
	}

	return addresses
}

func getPeerIDs(t *testing.T, hosts []host.Host) []libp2pPeer.ID {
	t.Helper()

	peerIDs := make([]libp2pPeer.ID, len(hosts))

	for i, h := range hosts {
		peerIDs[i] = h.ID()
	}

	return peerIDs
}

// Finds an empty port and returns a new multi-address(on localhost) with it
func getListenAddresses(t *testing.T, count int) []multiaddr.Multiaddr {
	t.Helper()

	ListenAddresses := make([]multiaddr.Multiaddr, count)

	for i := 0; i < count; i++ {
		port, err := tests.GetAvailablePort(t)
		require.NoError(t, err)

		ListenAddresses[i], err = multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
		require.NoError(t, err)
	}

	return ListenAddresses
}

func getBootstrapNodes(t *testing.T, count int) []host.Host {
	t.Helper()

	nodes := make([]host.Host, count)

	for i := 0; i < count; i++ {
		r := mrand.New(mrand.NewSource(time.Now().Unix()))
		privateKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
		require.NoError(t, err)

		freePort, err := tests.GetAvailablePort(t)
		require.NoError(t, err)

		sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freePort))

		bootNode := startBootStrapNode(t, privateKey, sourceMultiAddr)
		nodes[i] = bootNode
	}

	return nodes
}

// getKramaIDAndNetworkKey returns kramaID and network key pair
func getKramaIDAndNetworkKey(t *testing.T, nthValidator uint32) (kramaid.KramaID, []byte) {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:]) // fill sign key with random bytes
	require.NoError(t, err)

	// get private key and public key
	privKeyBytes, moiPubBytes, err := tests.GetPrivKeysForTest(signKey[:])
	require.NoError(t, err)

	networkKey := privKeyBytes[32:]

	kramaID, err := kramaid.NewKramaID( // Create kramaID from private key , public key
		networkKey,
		nthValidator,
		hex.EncodeToString(moiPubBytes),
		1,
		true,
	)
	require.NoError(t, err)

	return kramaID, networkKey
}

func defaultNetworkConfig() *common.NetworkConfig {
	return &common.NetworkConfig{
		ListenAddresses: make([]multiaddr.Multiaddr, 0),
		BootstrapPeers:  make([]multiaddr.Multiaddr, 0),
		ProtocolID:      protocol.ID("MOI"),
		MaxPeers:        0,
	}
}

func createServerWithoutHost(t *testing.T) *Server {
	t.Helper()

	defaultConfig := defaultNetworkConfig()

	kramaID, networkKey := getKramaIDAndNetworkKey(t, 0)
	vault := &MockVault{}

	nPriv := new(kcrypto.SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)
	vault.networkPrivateKey = nPriv

	return NewServer(
		context.Background(),
		hclog.NewNullLogger(),
		kramaID,
		nil,
		defaultConfig,
		vault,
	)
}

func createServer(
	t *testing.T,
	nthValidator int,
	params *CreateServerParams,
) *Server {
	t.Helper()

	ctx := context.Background()
	cfg := common.DefaultConfig("test")
	cfg.Network.ListenAddresses = getListenAddresses(t, 1)

	if params == nil {
		params = emptyParams
	}

	if params.ConfigCallback != nil {
		params.ConfigCallback(cfg.Network)
	}

	if params.Logger == nil {
		params.Logger = hclog.NewNullLogger()
	}

	kramaID, networkKey := getKramaIDAndNetworkKey(t, uint32(nthValidator))
	vault := &MockVault{}

	nPriv := new(kcrypto.SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)
	vault.networkPrivateKey = nPriv

	// Create a new server instance
	server := NewServer(ctx, params.Logger, kramaID, params.EventMux, cfg.Network, vault)

	if params.ServerCallback != nil {
		params.ServerCallback(server)
	}

	// err := server.StartServer()
	err := server.setupHost()
	require.NoError(t, err)

	err = server.setupPubSub()
	require.NoError(t, err)

	if len(server.cfg.BootstrapPeers) > 0 {
		err = server.connectToBootStrapNodes()
		require.NoError(t, err)
	}

	server.SetStreamHandler()

	// go server.Discover()

	return server
}

func createMultipleServers(
	t *testing.T,
	count int,
	paramsMap map[int]*CreateServerParams,
) []*Server {
	t.Helper()

	servers := make([]*Server, count)

	if paramsMap == nil {
		paramsMap = map[int]*CreateServerParams{}
	}

	for i := 0; i < count; i++ {
		servers[i] = createServer(t, i, paramsMap[i])
	}

	return servers
}

func closeTestServer(t *testing.T, s *Server) {
	t.Helper()

	require.NoError(t, s.host.Close())
}

func closeTestServers(t *testing.T, servers []*Server) {
	t.Helper()

	for _, server := range servers {
		closeTestServer(t, server)
	}
}

func startDiscovery(t *testing.T, servers ...*Server) {
	t.Helper()

	for _, s := range servers {
		// s.SetStreamHandler()
		go s.Discover()
	}
}

func getRandomNumber(t *testing.T, max int) int {
	t.Helper()

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	require.NoError(t, err)

	return int(nBig.Int64())
}

func getNTQ(t *testing.T, s *Server) int32 {
	t.Helper()

	ntq, err := s.Senatus.GetNTQ(s.GetKramaID())
	require.NoError(t, err)

	return ntq
}

func getHandShakeMsg(t *testing.T, msg *ptypes.Message) ptypes.HandshakeMSG {
	t.Helper()

	var handShake ptypes.HandshakeMSG
	err := handShake.FromBytes(msg.Payload)
	require.NoError(t, err)

	return handShake
}

func readMessageFromBuffer(t *testing.T, p *Peer) *ptypes.Message {
	t.Helper()

	buffer := make([]byte, 4096)

	byteCount, err := p.rw.Reader.Read(buffer)
	require.NoError(t, err)

	message := new(ptypes.Message)
	err = message.FromBytes(buffer[0:byteCount])
	require.NoError(t, err)

	return message
}

func openStream(t *testing.T, source *Server, destination *Server) *Peer {
	t.Helper()

	connectTo(t, source, destination)

	// Setup a new stream to the peer over the MOI protocol
	stream, err := source.NewStream(source.ctx, destination.host.ID(), source.cfg.ProtocolID)
	require.NoError(t, err)

	return newPeer(stream, source.logger)
}

func sendMessage(t *testing.T, p *Peer, msg []byte) {
	t.Helper()

	// Write the message bytes into the peer's io buffer
	_, err := p.rw.Writer.Write(msg)
	require.NoError(t, err)

	// Flush the peer's io buffer. This will push the message to the network
	err = p.rw.Flush()
	require.NoError(t, err)
}

func addPeerInfo(t *testing.T, s *Server, info *libp2pPeer.AddrInfo) {
	t.Helper()
	s.host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
}

func connectTo(t *testing.T, source *Server, destinations ...*Server) {
	t.Helper()

	for _, destination := range destinations {
		ctx := context.Background()
		err := source.host.Connect(ctx, *getPeerInfo(t, destination))
		require.NoError(t, err)
	}
}

func startBootStrapNode(t *testing.T, privateKey crypto.PrivKey, sourceMultiAddr multiaddr.Multiaddr) host.Host {
	t.Helper()

	ctx := context.Background()
	selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		dhtOpts := []dht.Option{
			dht.Mode(dht.ModeServer),
			dht.ProtocolPrefix("MOI"),
		}
		Dht, err := dht.New(ctx, h, dhtOpts...)
		require.NoError(t, err)

		return Dht, nil
	})

	bootStrapNode, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.NATPortMap(),
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableNATService(),
		libp2p.Identity(privateKey),
		selfRouting,
	)
	require.NoError(t, err)

	return bootStrapNode
}

func addPeer(t *testing.T, s *Server, kipPeer *Peer) {
	t.Helper()

	s.Peers.peers[kipPeer.networkID] = kipPeer
}

func unsubscribeServers(t *testing.T, server *Server, topic string) {
	t.Helper()

	err := server.Unsubscribe(topic)
	require.NoError(t, err)
}

func registerStreamHandler(servers ...*Server) {
	for _, server := range servers {
		server.SetupStreamHandler("MOI", server.streamHandlerFunc)
	}
}

// subscribeHelloMsg subscribes to given topic and validates the hello message received on topic
func subscribeHelloMsg(t *testing.T, s *Server, topic string, sender *Server, response chan int) {
	t.Helper()

	err := s.Subscribe(s.ctx, topic, func(msg *pubsub.Message) error {
		var hello ptypes.HelloMsg

		data := msg.GetData()
		err := hello.FromBytes(data)
		require.NoError(t, err)

		// check if peer info and signed data matches the published data
		validateHelloMessage(t, hello, sender)
		response <- 1

		return nil
	})
	require.NoError(t, err)
}

// subscribeMessage subscribes to given topic and validates the message received on topic
func subscribeMessage(t *testing.T, s *Server, topic string, response chan int) {
	t.Helper()

	err := s.Subscribe(s.ctx, topic, func(msg *pubsub.Message) error {
		var name string

		data := msg.GetData()
		err := polo.Depolorize(&name, data)
		require.NoError(t, err)

		require.Equal(t, message, name) // checks if published data matches received data
		response <- 1

		return nil
	})
	require.NoError(t, err)
}

// registerEmptySubscriptionHandler subscribes to given topic
//
// throws error when it receives data on topic if shouldError is true,
// does nothing when it receives data on topic if shouldError is false
func registerEmptySubscriptionHandler(t *testing.T, s *Server, topic string, shouldError bool) {
	t.Helper()

	err := s.Subscribe(s.ctx, topic, func(msg *pubsub.Message) error {
		if shouldError {
			require.Error(t, nil) // shouldn't receive message
		}

		return nil
	})
	require.NoError(t, err)
}

// registerMessageHandler sets handler function for given protocol id
// validates message received on stream
func registerMessageHandler(
	t *testing.T, s *Server,
	pID protocol.ID,
	id kramaid.KramaID,
	handshakeMessage ptypes.HandshakeMSG,
	response chan int,
) {
	t.Helper()

	s.host.SetStreamHandler(pID, func(stream network.Stream) {
		peer := newPeer(stream, s.logger)
		msgReceived := readMessageFromBuffer(t, peer)

		validateMessage(t, id, msgReceived)
		validatePayload(t, handshakeMessage, msgReceived)
		response <- 1
	})
}

func waitForResponse(ctx context.Context, respChan chan int) error {
	select {
	case <-ctx.Done():
		return types.ErrTimeOut
	case <-respChan:
		return nil // nil value returned if respChan filled
	}
}

// Validation functions
func checkForPeerRegistration(t *testing.T, serverList *Server, hasServer *Server, exists bool) {
	t.Helper()

	if exists {
		require.True(t, serverList.Peers.ContainsPeer(hasServer.host.ID()))
	} else {
		require.False(t, serverList.Peers.ContainsPeer(hasServer.host.ID()))
	}
}

func checkForHost(t *testing.T, server *Server) {
	t.Helper()

	require.NotNil(t, server.host) // make sure host set up
}

func checkForSubscription(t *testing.T, server *Server) {
	t.Helper()

	require.NotNil(t, server.pubSubTopics) // make sure host subscription topics initialized
	require.NotNil(t, server.psRouter)     // make sure host  subscribed
}

func checkForTopic(t *testing.T, server *Server, topic string, exist bool) {
	t.Helper()

	if exist {
		require.NotNil(t, server.pubSubTopics.getTopicSet(topic))
	} else {
		require.Nil(t, server.pubSubTopics.getTopicSet(topic))
	}
}

func checkForTopicSet(t *testing.T, server *Server, topic string) {
	t.Helper()

	require.NotNil(t, server.pubSubTopics.getTopicSet(topic).topicHandle) // make sure host subscription topics initialized
	require.NotNil(t, server.pubSubTopics.getTopicSet(topic).subHandle)   // make sure host  subscribed
}

func checkForKadDHT(t *testing.T, server *Server) {
	t.Helper()

	require.NotNil(t, server.kadDHT) // make sure dht setup
}

func checkConnection(t *testing.T, server *Server, peerID libp2pPeer.ID, connected bool) {
	t.Helper()

	if connected {
		require.Equal(t, network.Connected, server.host.Network().Connectedness(peerID))
	} else {
		require.Equal(t, network.NotConnected, server.host.Network().Connectedness(peerID))
	}
}

func validateMessage(t *testing.T, id kramaid.KramaID, msg *ptypes.Message) {
	t.Helper()

	require.Equal(t, ptypes.HANDSHAKEMSG, msg.MsgType)
	require.Equal(t, id, msg.Sender)
}

func validateHandShakeMsg(t *testing.T, s *Server, msg *ptypes.Message, expectedError error) {
	t.Helper()

	validateMessage(t, s.id, msg)

	handShake := getHandShakeMsg(t, msg)

	if expectedError != nil {
		require.Contains(t, handShake.Error, expectedError.Error())
	} else {
		require.Equal(t, getNTQ(t, s), handShake.NTQ)
		require.Equal(t, s.host.Addrs(), utils.MultiAddrFromString(handShake.Address...))
	}
}

func validateNewPeerEvent(t *testing.T, newPeerSub *utils.Subscription, s *Server) {
	t.Helper()

	obj := <-newPeerSub.Chan()
	p, ok := obj.Data.(utils.NewPeerEvent)
	require.True(t, ok)
	require.Equal(t, s.host.ID(), p.PeerID)
}

func validatePeerDiscoveredEvent(t *testing.T, peerDiscoveredSub *utils.Subscription, s *Server) {
	t.Helper()

	obj := <-peerDiscoveredSub.Chan()
	p, ok := obj.Data.(utils.PeerDiscoveredEvent)
	require.True(t, ok)
	require.Equal(t, s.host.ID(), p.ID)
}

func validateHelloMessage(t *testing.T, hello ptypes.HelloMsg, sender *Server) {
	t.Helper()

	require.Equal(t, getNTQ(t, sender), hello.Info.Ntq)
	require.Equal(t, sender.id, hello.Info.ID)
	require.Equal(t, sender.GetAddrs(), utils.MultiAddrFromString(hello.Info.Address...))
	require.Equal(t, SignedData, hello.Signature)
}

func validatePayload(t *testing.T, handshakeMessage ptypes.HandshakeMSG, msg *ptypes.Message) {
	t.Helper()

	var payload ptypes.HandshakeMSG
	err := polo.Depolorize(&payload, msg.Payload)
	require.NoError(t, err)
	require.Equal(t, handshakeMessage, payload)
}

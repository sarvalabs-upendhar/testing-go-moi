package p2p

import (
	"context"
	"fmt"
	mrand "math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/kramaid"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/senatus"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
)

var (
	SignedData  = []byte{1, 2, 3}
	emptyParams = &CreateServerParams{}
)

type MockVault struct {
	networkPrivateKey crypto.PrivateKey // Private key used in p2p communication
}

type MockReputationEngine struct {
	ntq      map[kramaid.KramaID]float32
	peerInfo map[peer.ID]*senatus.NodeMetaInfo
	mutex    sync.RWMutex
}

func NewMockReputationEngine() *MockReputationEngine {
	return &MockReputationEngine{
		ntq:      make(map[kramaid.KramaID]float32),
		peerInfo: make(map[peer.ID]*senatus.NodeMetaInfo),
	}
}

func (m *MockReputationEngine) UpdatePeer(data *senatus.NodeMetaInfo) error {
	peerID, err := data.KramaID.DecodedPeerID()
	if err != nil {
		return common.ErrInvalidKramaID
	}

	return m.AddNewPeerWithPeerID(peerID, data)
}

func (m *MockReputationEngine) AddNewPeerWithPeerID(peerID peer.ID, data *senatus.NodeMetaInfo) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.peerInfo[peerID] = data

	return nil
}

func (m *MockReputationEngine) GetNTQ(id kramaid.KramaID) (float32, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ntq, ok := m.ntq[id]
	if !ok {
		return -1, common.ErrKeyNotFound
	}

	return ntq, nil
}

func (m *MockReputationEngine) SetNTQ(id kramaid.KramaID, val int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ntq[id] = float32(val)
}

func (m *MockReputationEngine) GetAddress(key kramaid.KramaID) ([]multiaddr.Multiaddr, error) {
	peerID, err := key.DecodedPeerID()
	if err != nil {
		return nil, common.ErrInvalidKramaID
	}

	return m.GetAddressByPeerID(peerID)
}

func (m *MockReputationEngine) GetAddressByPeerID(peerID peer.ID) ([]multiaddr.Multiaddr, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return utils.MultiAddrFromString(peerInfo.Addrs...), nil
	}

	return nil, common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) GetKramaIDByPeerID(peerID peer.ID) (kramaid.KramaID, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return peerInfo.KramaID, nil
	}

	return "", common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) GetRTTByPeerID(peerID peer.ID) (int64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if peerInfo, ok := m.peerInfo[peerID]; ok {
		return peerInfo.RTT, nil
	}

	return 0, common.ErrKramaIDNotFound
}

func (m *MockReputationEngine) SetPeerInfo(key peer.ID, peerInfo *senatus.NodeMetaInfo) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.peerInfo[key] = peerInfo
}

type CreateServerParams struct {
	ConfigCallback func(c *config.NetworkConfig) // Additional logic that needs to be executed on the configuration
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
	inboundConnLimit int64,
	outboundConnLimit int64,
	noDiscovery bool,
) []*CreateServerParams {
	t.Helper()

	var arrayOfParams []*CreateServerParams

	for i := 0; i < count; i++ {
		arrayOfParams = append(arrayOfParams, &CreateServerParams{
			EventMux: &utils.TypeMux{},
			ConfigCallback: func(c *config.NetworkConfig) {
				c.BootstrapPeers = getMultiAddresses(t, bootNodes)
				c.InboundConnLimit = inboundConnLimit
				c.OutboundConnLimit = outboundConnLimit
				c.NoDiscovery = noDiscovery
			},
			ServerCallback: func(s *Server) {
				m := NewMockReputationEngine()
				m.SetNTQ(s.GetKramaID(), tests.GetRandomNumber(t, 27))
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

func (vault *MockVault) GetNetworkPrivateKey() crypto.PrivateKey {
	return vault.networkPrivateKey
}

func (vault *MockVault) Sign(
	data []byte,
	sigType mudracommon.SigType,
	signOptions ...crypto.SignOption,
) ([]byte, error) {
	return SignedData, nil
}

func (vault *MockVault) SetNetworkPrivateKey(t *testing.T, privKeyBytes []byte) {
	t.Helper()

	networkPrivKey := new(crypto.SECP256K1PrivKey)
	networkPrivKey.UnMarshal(privKeyBytes)
	vault.networkPrivateKey = networkPrivKey
}

func setServerPeers(t *testing.T, s *Server, peersList ...kramaid.KramaID) {
	t.Helper()

	for _, kPeer := range peersList {
		peerID, err := kPeer.DecodedPeerID()
		require.NoError(t, err)

		s.Peers.addPeer(&Peer{kramaID: kPeer, networkID: peerID})
	}
}

// helper functions
func getPeerInfo(t *testing.T, server *Server) *peer.AddrInfo {
	t.Helper()

	address := server.GetAddrs()
	networkID, err := server.id.PeerID()
	require.NoError(t, err)

	peerID, err := peer.Decode(networkID)
	require.NoError(t, err)

	return &peer.AddrInfo{
		ID:    peerID,
		Addrs: address,
	}
}

// getPeer only network ID is initialized
func getPeer(t *testing.T, s *Server, destServer *Server) *Peer {
	t.Helper()

	stream, err := s.ConnManager.NewStream(s.ctx, destServer.host.ID(), config.MOIProtocolStream, MOIStreamTag)
	require.NoError(t, err)
	// Create new kip peer
	kPeer := newPeer(stream, destServer.id, 50, destServer.logger)

	return kPeer
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

func getPeerIDs(t *testing.T, hosts []host.Host) []peer.ID {
	t.Helper()

	peerIDs := make([]peer.ID, len(hosts))

	for i, h := range hosts {
		peerIDs[i] = h.ID()
	}

	return peerIDs
}

func getBootstrapNodes(t *testing.T, count int) []host.Host {
	t.Helper()

	nodes := make([]host.Host, count)

	for i := 0; i < count; i++ {
		r := mrand.New(mrand.NewSource(time.Now().Unix()))
		privateKey, _, err := libp2pcrypto.GenerateKeyPairWithReader(libp2pcrypto.RSA, 2048, r)
		require.NoError(t, err)

		freePort, err := tests.GetAvailablePort(t)
		require.NoError(t, err)

		sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freePort))

		bootNode := startBootStrapNode(t, privateKey, sourceMultiAddr)
		nodes[i] = bootNode
	}

	return nodes
}

func defaultNetworkConfig() *config.NetworkConfig {
	return &config.NetworkConfig{
		ListenAddresses: make([]multiaddr.Multiaddr, 0),
		BootstrapPeers:  make([]multiaddr.Multiaddr, 0),
		MaxPeers:        0,
	}
}

func createServerWithoutHost(t *testing.T) *Server {
	t.Helper()

	defaultConfig := defaultNetworkConfig()

	kramaID, networkKey := tests.GetKramaIDAndNetworkKey(t, 0)
	vault := &MockVault{}

	nPriv := new(crypto.SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)
	vault.networkPrivateKey = nPriv

	return NewServer(
		hclog.NewNullLogger(),
		kramaID,
		nil,
		defaultConfig,
		vault,
		NilMetrics(),
	)
}

func createServer(
	t *testing.T,
	nthValidator int,
	params *CreateServerParams,
) *Server {
	t.Helper()

	cfg := config.DefaultDevnetConfig("test")
	cfg.Network.ListenAddresses = tests.GetListenAddresses(t, 1)

	if params == nil {
		params = emptyParams
	}

	if params.ConfigCallback != nil {
		params.ConfigCallback(cfg.Network)
	}

	if params.Logger == nil {
		params.Logger = hclog.NewNullLogger()
	}

	kramaID, networkKey := tests.GetKramaIDAndNetworkKey(t, uint32(nthValidator))
	vault := &MockVault{}

	nPriv := new(crypto.SECP256K1PrivKey)
	nPriv.UnMarshal(networkKey)
	vault.networkPrivateKey = nPriv

	// Create a new server instance
	server := NewServer(params.Logger, kramaID, params.EventMux, cfg.Network, vault, NilMetrics())

	if params.ServerCallback != nil {
		params.ServerCallback(server)
	}

	// err := server.StartServer()
	err := server.setupHost()
	require.NoError(t, err)

	err = server.setupPubSub()
	require.NoError(t, err)

	server.ds = NewDiscoveryService(server)
	server.ConnManager = NewConnectionManager(server)

	if len(server.cfg.BootstrapPeers) > 0 {
		err = server.ConnManager.connectToBootStrapNodes()
		require.NoError(t, err)
	}

	server.ConnManager.setStreamHandler()

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
		// s.setStreamHandler()
		go s.ds.advertise()
		go s.ds.discover()
	}
}

func initDiscoveryAndAdvertise(t *testing.T, servers ...*Server) {
	t.Helper()

	for _, s := range servers {
		s.cfg.DiscoveryInterval = 50 * time.Millisecond

		// Advertise the rendezvous string to the discovery service
		t.Log("Announcing ourselves")

		s.ds.Start()
	}
}

func findPeer(t *testing.T, server *Server) peer.AddrInfo {
	t.Helper()

	peerchan, err := server.ds.discovery.FindPeers(server.ctx, string(config.MOIProtocolStream))
	require.NoError(t, err)

	peerInfo := <-peerchan

	return peerInfo
}

func getHandShakeMsg(t *testing.T, msg *networkmsg.Message) networkmsg.HandshakeMSG {
	t.Helper()

	var handShake networkmsg.HandshakeMSG
	err := handShake.FromBytes(msg.Payload)
	require.NoError(t, err)

	return handShake
}

func readMessageFromBuffer(t *testing.T, p *Peer) *networkmsg.Message {
	t.Helper()

	reader := msgio.NewReader(p.rw.Reader)
	buffer, err := reader.ReadMsg()
	require.NoError(t, err)

	message := new(networkmsg.Message)
	err = message.FromBytes(buffer)
	require.NoError(t, err)

	return message
}

func openStream(t *testing.T, source *Server, destination *Server) *Peer {
	t.Helper()

	connectTo(t, source, destination)

	// Setup a new stream to the peer over the MOI protocol
	stream, err := source.ConnManager.NewStream(
		source.ctx,
		destination.host.ID(),
		config.MOIProtocolStream,
		MOIStreamTag,
	)
	require.NoError(t, err)

	kPeer := newPeer(stream, destination.id, 50, source.logger)

	return kPeer
}

func sendMessage(t *testing.T, p *Peer, msg []byte) {
	t.Helper()

	// Write the message bytes into the peer's io buffer
	writer := msgio.NewWriter(p.rw.Writer)
	err := writer.WriteMsg(msg)
	require.NoError(t, err)

	err = p.rw.Writer.Flush()
	require.NoError(t, err)
}

func connectTo(t *testing.T, source *Server, destinations ...*Server) {
	t.Helper()

	for _, destination := range destinations {
		ctx := context.Background()
		err := source.host.Connect(ctx, *getPeerInfo(t, destination))
		require.NoError(t, err)
	}
}

func startBootStrapNode(t *testing.T, privateKey libp2pcrypto.PrivKey, sourceMultiAddr multiaddr.Multiaddr) host.Host {
	t.Helper()

	ctx := context.Background()
	selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		dhtOpts := []dht.Option{
			dht.Mode(dht.ModeServer),
			dht.ProtocolPrefix(config.MOIProtocolStream),
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
		server.ConnManager.SetupStreamHandler(config.MOIProtocolStream, MOIStreamTag, server.ConnManager.streamHandler)
	}
}

// subscribeHelloMsg subscribes to given topic and validates the hello message received on topic
func subscribeHelloMsg(t *testing.T, s *Server, topic string, sender *Server, response chan int) {
	t.Helper()

	err := s.Subscribe(s.ctx, topic, nil, true, func(msg *pubsub.Message) error {
		var hello networkmsg.HelloMsg

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

	err := s.Subscribe(s.ctx, topic, nil, true, func(msg *pubsub.Message) error {
		var name string

		data := msg.GetData()
		err := polo.Depolorize(&name, data)
		require.NoError(t, err)

		require.Equal(t, hellomessage, name) // checks if published data matches received data
		response <- 1

		return nil
	})
	require.NoError(t, err)
}

// registerEmptySubscriptionHandler subscribes to given topic
//
// throws error when it receives data on topic if shouldError is true,
// does nothing when it receives data on topic if shouldError is false
func registerEmptySubscriptionHandler(t *testing.T, s *Server, topic string, defaultValidator bool, shouldError bool) {
	t.Helper()

	err := s.Subscribe(s.ctx, topic, nil, defaultValidator, func(msg *pubsub.Message) error {
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
	destServer *Server,
	handshakeMessage networkmsg.HandshakeMSG,
	response chan int,
) {
	t.Helper()

	s.host.SetStreamHandler(pID, func(stream network.Stream) {
		kPeer := newPeer(stream, destServer.id, 50, s.logger)

		msgReceived := readMessageFromBuffer(t, kPeer)

		validateMessage(t, destServer.id, msgReceived)
		validatePayload(t, handshakeMessage, msgReceived)
		response <- 1
	})
}

func waitForResponse(ctx context.Context, respChan chan int) error {
	select {
	case <-ctx.Done():
		return common.ErrTimeOut
	case <-respChan:
		return nil // nil value returned if respChan filled
	}
}

// Validation functions
func checkForPeerRegistration(t *testing.T, serverList *Server, hasServer *Server, exists bool) {
	t.Helper()

	if exists {
		require.True(t, serverList.Peers.ContainsPeer(hasServer.host.ID()))

		return
	}

	require.False(t, serverList.Peers.ContainsPeer(hasServer.host.ID()))
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

func checkConnection(t *testing.T, server *Server, peerID peer.ID, connected bool) {
	t.Helper()

	if connected {
		require.Equal(t, network.Connected, server.host.Network().Connectedness(peerID))
	} else {
		require.Equal(t, network.NotConnected, server.host.Network().Connectedness(peerID))
	}
}

func checkConnectionProtection(t *testing.T, server *Server, peerID peer.ID, protected bool) {
	t.Helper()

	require.Equal(t, protected, server.host.ConnManager().IsProtected(peerID, MOIStreamTag))
}

func postDiscoverPeerEvent(t *testing.T, source *Server, peerID peer.ID) {
	t.Helper()

	err := source.mux.Post(utils.DiscoverPeerEvent{
		ID: peerID,
	})
	require.NoError(t, err)
}

func assertNodeMetaInfoInSenatus(t *testing.T, source *Server, peerID peer.ID, shouldExist bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		if kramaID, err := source.Senatus.GetKramaIDByPeerID(peerID); err == nil && kramaID != "" {
			return true, false
		}

		return nil, true
	})

	if shouldExist {
		require.NoError(t, err)

		return
	}

	require.Error(t, err)
}

func validateMessage(t *testing.T, id kramaid.KramaID, msg *networkmsg.Message) {
	t.Helper()

	require.Equal(t, networkmsg.HANDSHAKEMSG, msg.MsgType)
	require.Equal(t, id, msg.Sender)
}

func validateHandShakeMsg(t *testing.T, s *Server, msg *networkmsg.Message, expectedError error) {
	t.Helper()

	validateMessage(t, s.id, msg)

	handShake := getHandShakeMsg(t, msg)

	if expectedError != nil {
		require.Contains(t, handShake.Error, expectedError.Error())

		return
	}

	require.Equal(t, []byte("ping"), handShake.Data)
}

func validateNewPeerEvent(t *testing.T, newPeerSub *utils.Subscription, s *Server) {
	t.Helper()

	obj := <-newPeerSub.Chan()
	p, ok := obj.Data.(utils.NewPeerEvent)
	require.True(t, ok)
	require.Equal(t, s.host.ID(), p.PeerID)
}

func validateHelloMessage(t *testing.T, hello networkmsg.HelloMsg, sender *Server) {
	t.Helper()

	require.Equal(t, sender.id, hello.KramaID)
	require.Equal(t, sender.GetAddrs(), utils.MultiAddrFromString(hello.Address...))
	require.Equal(t, SignedData, hello.Signature)
}

func validatePayload(t *testing.T, handshakeMessage networkmsg.HandshakeMSG, msg *networkmsg.Message) {
	t.Helper()

	var payload networkmsg.HandshakeMSG
	err := polo.Depolorize(&payload, msg.Payload)
	require.NoError(t, err)
	require.Equal(t, handshakeMessage, payload)
}

func waitForDiscovery(t *testing.T, server *Server, expectedPeerCount int) {
	t.Helper()

	waitTime := 150 * time.Millisecond

	if expectedPeerCount > 0 {
		waitTime = 800 * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), waitTime)
	defer cancel()

	_, err := tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
		if len(server.ConnManager.getPeers()) > 0 {
			return true, false
		}

		return false, true
	})

	if expectedPeerCount == 0 {
		require.Error(t, err)

		return
	}

	require.NoError(t, err)
}

func createSignedHelloMsg(t *testing.T) networkmsg.HelloMsg {
	t.Helper()

	dir, err := os.MkdirTemp(os.TempDir(), " ")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	})

	// create keystore.json in current directory
	password := "test123"

	_, _, err = poi.RandGenKeystore(dir, password)
	require.NoError(t, err)

	config := &crypto.VaultConfig{
		DataDir:      dir,
		NodePassword: password,
	}

	vault, err := crypto.NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	msg := networkmsg.HelloMsg{
		KramaID:   vault.KramaID(),
		Address:   []string{tests.RandomAddress(t).String()},
		Signature: nil,
	}

	rawMsg, err := msg.Bytes()
	require.NoError(t, err)

	signature, err := vault.Sign(rawMsg, mudracommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	require.NoError(t, err)

	msg.Signature = signature

	return msg
}

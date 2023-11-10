package p2p

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	kdht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pMetrics "github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-msgio"
	maddr "github.com/multiformats/go-multiaddr"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
	mcommon "github.com/sarvalabs/go-moi/crypto/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/network/rpc"
	"github.com/sarvalabs/go-moi/senatus"
)

const (
	MinimumConnReq          = 100
	MaximumConnReq          = 1000
	MinimumBootNodeConn int = 1
)

var (
	ErrNoBootnodes      = errors.New("no bootnodes")
	ErrMinBootnodes     = fmt.Errorf("minimum %v bootnode is required", MinimumBootNodeConn)
	ErrMinBootnodeConns = fmt.Errorf("unable to connect to at least %v bootstrap node(s)", MinimumBootNodeConn)
)

type Vault interface {
	GetNetworkPrivateKey() crypto.PrivateKey
	Sign(data []byte, sigType mcommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
}

type Senatus interface {
	GetNTQ(peerID id.KramaID) (float32, error)
	GetAddress(key id.KramaID) ([]maddr.Multiaddr, error)
	GetAddressByPeerID(peerID peer.ID) ([]maddr.Multiaddr, error)
	UpdatePeer(key id.KramaID, data *senatus.NodeMetaInfo) error
	AddNewPeerWithPeerID(key peer.ID, data *senatus.NodeMetaInfo) error
}

// Server is a struct that represents a node on the network
type Server struct {
	ctx       context.Context // context of server lifecycle
	ctxCancel context.CancelFunc
	logger    hclog.Logger
	cfg       *config.NetworkConfig // config of the node

	host      host.Host     // libp2p host of the node
	kadDHT    *kdht.IpfsDHT // libp2p Kad DHT of the node
	discovery *discovery.RoutingDiscovery
	psRouter  *pubsub.PubSub // libp2p PubSub router of the node

	id id.KramaID // KramaID of the node

	Peers *peerSet // peerSet of node

	connInfo *ConnectionInfo // connection info of the node

	rpcServers map[protocol.ID]*rpc.Server // map of MOI-RPC Server's Protocol ID to respective MOI-RPC Server

	pubSubTopics *pubSubTopics

	vault   Vault   // Vault interface to access network keys and to sign messages
	Senatus Senatus // Senatus interface to access reputation info

	mux *utils.TypeMux // typemux of the node

	init    sync.Once
	metrics *Metrics
}

// NewServer is a constructor function that generates, configures and returns a Server.
// Accepts lifecycle context for the node along with a typemux and a config.
func NewServer(
	logger hclog.Logger,
	id id.KramaID,
	mux *utils.TypeMux,
	config *config.NetworkConfig,
	vault Vault,
	metrics *Metrics,
) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		id:         id,
		ctx:        ctx,
		ctxCancel:  cancel,
		logger:     logger.Named("P2P-Server"),
		cfg:        config,
		mux:        mux,
		Peers:      newPeerSet(),
		rpcServers: make(map[protocol.ID]*rpc.Server),
		vault:      vault,
		metrics:    metrics,
	}

	server.connInfo = NewConnectionInfo(server, config.InboundConnLimit, config.OutboundConnLimit)

	return server
}

func (s *Server) Close() error {
	if err := s.host.Close(); err != nil {
		return err
	}

	s.ctxCancel()

	return nil
}

func (s *Server) AddPeerInfo(info *peer.AddrInfo) {
	s.host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.ConnectedAddrTTL)
}

func (s *Server) SetupServer() error {
	if err := s.setupHost(); err != nil {
		return fmt.Errorf("setup host: %w", err)
	}

	if err := s.setupPubSub(); err != nil {
		return fmt.Errorf("setup PubSub: %w", err)
	}

	s.logger.Info("Starting server", "krama-ID", s.id, "addr", s.host.Addrs())

	if !s.cfg.NoDiscovery {
		if err := s.connectToBootStrapNodes(); err != nil {
			return fmt.Errorf("bootstrap nodes connection: %w", err)
		}
	}

	return nil
}

// StartServer sets up node's libp2p host, Kad DHT, PubSub route
// after which it bootstraps itself by attempting to connect to bootstrap peers.
func (s *Server) StartServer() error {
	s.setStreamHandler()

	if s.cfg.NoDiscovery {
		go s.connectToTrustedNodes()

		if !s.cfg.RefreshSenatus {
			return nil
		}
	}

	go s.discover(s.cfg.DiscoveryInterval)

	return nil
}

// connectToBootStrapNodes Attempts connecting to all the bootstrap nodes in config
// returns error if
// --boostrap nodes in config is Nil
// --boostrap nodes count in config is Zero
// --unable to connect to at least one bootstrap node
func (s *Server) connectToBootStrapNodes() error {
	if s.cfg.BootstrapPeers == nil {
		return ErrNoBootnodes
	}

	if len(s.cfg.BootstrapPeers) < MinimumBootNodeConn {
		return ErrMinBootnodes
	}

	var bootstrapConnections int8

	for _, bootstrapPeer := range s.cfg.BootstrapPeers {
		if err := s.connectToMaddr(bootstrapPeer); err != nil {
			s.logger.Error("Bootstrap connection failed", "peer", bootstrapPeer, "err", err)

			continue
		}

		s.logger.Info("Connection established with bootstrap node", "peer", bootstrapPeer)

		bootstrapConnections += 1
	}

	if bootstrapConnections == 0 {
		return ErrMinBootnodeConns
	}

	return nil
}

func (s *Server) connectToMaddr(peerAddr maddr.Multiaddr) error {
	peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
	if err != nil {
		return err
	}

	if err = s.host.Connect(s.ctx, *peerInfo); err != nil {
		return err
	}

	return nil
}

// connectToTrustedNodes attempts to connect to and register the given list of trusted nodes.
func (s *Server) connectToTrustedNodes() {
	s.logger.Info("Connecting to trusted nodes")

	for _, trustedPeer := range s.cfg.TrustedPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(trustedPeer.Address)
		if err != nil {
			s.logger.Error("Invalid trusted peer address", "err", err)

			continue
		}

		if err := s.ConnectAndRegisterPeer(*peerInfo); err != nil {
			s.logger.Error("Failed to establish connection with trusted peer", "err", err)
		}
	}
}

// StartNewRPCServer starts a new MOI-RPC server & client with the given ProtocolID
// adds the server to map of poorna-server's rpcServers map and returns Client
func (s *Server) StartNewRPCServer(protocol protocol.ID) *rpc.Client {
	s.logger.Trace("Starting new MOI-RPC server", "protocol-ID", protocol)

	s.rpcServers[protocol] = rpc.NewServer(s.logger.Named(string(protocol)), s.host, protocol)

	return rpc.NewClient(s.logger.Named(string(protocol)), s.host, protocol, s.Senatus)
}

// setupHost sets up the libp2p host for the node
// Expects the node to already be configured with a lifecycle context and config
//
// Acquires an identity key based on the key file constructed from the Config and the multiaddr
// by querying the host network interface
// Configures the host stream handler for the protocol
func (s *Server) setupHost() (err error) {
	if s.ctx == nil {
		return errors.New("lifecycle context for node not configured")
	}

	hostOptions, err := s.getLibp2pHostOptions()
	if err != nil {
		return err
	}

	// create a new libp2p host with the host options
	s.host, err = libp2p.New(hostOptions)
	if err != nil {
		return fmt.Errorf("new libp2p host %w", err)
	}

	return nil
}

func (s *Server) getLibp2pHostOptions() (libp2p.Option, error) {
	prvKey, err := s.getPrivateKey()
	if err != nil {
		return nil, err
	}

	mgr, err := connmgr.NewConnManager(MinimumConnReq, MaximumConnReq)
	if err != nil {
		panic(err)
	}

	resourceManager, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits))
	if err != nil {
		panic(err)
	}

	addrsFactory := func(addrs []maddr.Multiaddr) []maddr.Multiaddr {
		if len(s.cfg.PublicP2pAddresses) > 0 {
			addrs = append(addrs, s.cfg.PublicP2pAddresses...)
		}

		return addrs
	}

	return libp2p.ChainOptions(
		// Enable UPnP and hole punching
		s.getSelfRouting(),
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.Identity(prvKey),
		libp2p.ListenAddrs(s.cfg.ListenAddresses...),
		libp2p.BandwidthReporter(newBandwidthReporter(s.metrics, libp2pMetrics.NewBandwidthCounter())),
		libp2p.ConnectionManager(mgr),
		libp2p.ResourceManager(resourceManager),
		libp2p.AddrsFactory(addrsFactory),
	), nil
}

func (s *Server) getPrivateKey() (p2pcrypto.PrivKey, error) {
	prvKey, err := p2pcrypto.UnmarshalSecp256k1PrivateKey(s.vault.GetNetworkPrivateKey().Bytes())
	if err != nil {
		return nil, fmt.Errorf("unmarshalling network key %w", err)
	}

	return prvKey, err
}

func (s *Server) getSelfRouting() libp2p.Option {
	return libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		if err := s.setupKadDht(h); err != nil {
			return nil, fmt.Errorf("kaddht setup error: %w", err)
		}

		return s.kadDHT, nil
	})
}

func (s *Server) GetConns() []network.Conn {
	return s.host.Network().Conns()
}

func (s *Server) GetInboundConnCount() int64 {
	return s.connInfo.inboundConnCount
}

func (s *Server) GetOutboundConnCount() int64 {
	return s.connInfo.outboundConnCount
}

// GetKramaID returns the KramaID of node
func (s *Server) GetKramaID() id.KramaID {
	return s.id
}

// SetupStreamHandler sets the stream handler for a given ProtocolID on the Host's Mux
func (s *Server) SetupStreamHandler(protocolID protocol.ID, handle func(network.Stream)) {
	s.host.SetStreamHandler(protocolID, handle)
}

// RemoveStreamHandler removes a handler for a given ProtocolId on the mux that was set by setStreamHandler
func (s *Server) RemoveStreamHandler(protocolID protocol.ID) {
	s.host.RemoveStreamHandler(protocolID)
}

// setupKadDht sets up the kademlia DHT & bootstraps it for the node.
// expects the node host to already be configured with a libp2p host.
func (s *Server) setupKadDht(host host.Host) (err error) {
	// create a new Kad DHT for the Server with the dht options
	s.kadDHT, err = kdht.New(s.ctx, host, s.getKdhtOptions()...)
	if err != nil {
		return fmt.Errorf("new kdht %w", err)
	}

	// bootstrap the Kad DHT and check for errors
	if err = s.kadDHT.Bootstrap(s.ctx); err != nil {
		return fmt.Errorf("bootstrap kdht %w", err)
	}

	return nil
}

func (s *Server) getKdhtOptions() []kdht.Option {
	return []kdht.Option{
		kdht.Concurrency(10),
		kdht.Mode(kdht.ModeServer),
		kdht.ProtocolPrefix(config.MOIProtocolStream),
	}
}

// streamHandlerFunc is a method of Server that defines the behaviour of stream
// handling for streams acquired over the protocol.
//
// Creates a NewPeerEvent when a new stream is acquired and posts it
// to the network for all receivers registered to receive the event.
func (s *Server) streamHandlerFunc(stream network.Stream) {
	if s.connInfo.isInboundConnLimitReached() {
		if err := stream.Reset(); err != nil {
			s.logger.Error("Failed to reset stream", "err", err)
		}

		return
	}

	s.logger.Trace("Handling new stream", "protocol", stream.Protocol(), "peer-ID", stream.Conn().RemotePeer())

	kPeer := newPeer(stream, s.logger)

	if err := kPeer.handleHandshakeMessage(); err != nil {
		if err := kPeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Handle handshake", "err", err)
		}

		return
	}

	// Register the kPeer to the handler working set
	if err := s.Peers.Register(kPeer); err != nil {
		if err := kPeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Handshake error response", "err", err)
		}

		return
	}

	// Update the inbound connection count
	s.connInfo.updateInboundConnCount(1)

	if err := kPeer.SendHandshakeMessage(s); err != nil {
		s.logger.Error("Send hand shake message", "err", err)

		return
	}

	// Post the event to the registered receivers for NewPeerEvents
	if err := s.postNewPeerEvent(kPeer.networkID); err != nil {
		s.logger.Error("Stream handler function", "err", err)

		return
	}

	s.logger.Info("Handled stream, connected and registered", "krama-ID", kPeer.kramaID)
}

// discover is a method of Server that starts a discovery routine using a libp2p routing
// discovery mechanism that uses the Kad DHT of the node.
//
// Advertises the protocol rendezvous and discovers other peers that are advertising it.
func (s *Server) discover(interval time.Duration) {
	s.discovery = discovery.NewRoutingDiscovery(s.kadDHT)

	// Advertise the rendezvous string to the discovery service
	s.logger.Info("Announcing ourselves")

	// TODO: explore about how many times to advertise
	_, err := s.discovery.Advertise(s.ctx, string(config.MOIProtocolStream))
	if err != nil {
		s.logger.Error("Failed to advertise the rendezvous string to the discovery service", "err", err)
	}

	// discover other peers that are advertising themselves
	s.logger.Info("Starting discovery routine")

	for {
		select {
		case <-time.After(interval):
		case <-s.ctx.Done():
			return
		}

		if err = s.handleDiscovery(); err != nil {
			s.logger.Error("Handle discovery", "err", err)
		}
	}
}

func (s *Server) handleDiscovery() error {
	// Retrieve a channel of peer addresses from the discovery service
	peerChan, err := s.discovery.FindPeers(s.ctx, string(config.MOIProtocolStream))
	if err != nil {
		return err
	}

	// Iterate over the channel of peer addresses
	for p := range peerChan {
		// Skip iteration if the peer addresses points to self
		if p.ID == s.host.ID() {
			continue
		}

		if !s.cfg.NoDiscovery {
			if err = s.ConnectAndRegisterPeer(p); err != nil {
				/*
					Skip iteration on,
					* Connection failure
					* Outbound connection limit failure
					* Stream setup failure
					* Error fetching ntq
					* Handshake failure
				*/
				continue
			}
		}

		if s.cfg.RefreshSenatus {
			err = s.Senatus.AddNewPeerWithPeerID(p.ID, &senatus.NodeMetaInfo{
				Addrs: utils.MultiAddrToString(p.Addrs...),
				NTQ:   senatus.DefaultPeerNTQ,
			})
			if err != nil && !errors.Is(err, common.ErrAlreadyKnown) {
				s.logger.Error("Failed to add peer info to senatus", "err", err)

				continue
			}
		}
	}

	return nil
}

// ConnectAndRegisterPeer is a method of Server that connects to peer associated with a given KipID and
// register's the peer to the handler working set.
func (s *Server) ConnectAndRegisterPeer(peerInfo peer.AddrInfo) error {
	var (
		stream network.Stream
		err    error
		kPeer  *Peer
	)

	if !s.cfg.NoDiscovery && s.connInfo.isOutboundConnLimitReached() {
		return common.ErrOutboundConnLimit
	}

	if s.Peers.ContainsPeer(peerInfo.ID) {
		return errAlreadyRegistered
	}

	if err = s.connectPeer(peerInfo); err != nil && !errors.Is(err, common.ErrConnectionExists) {
		return err
	}

	// create a new stream to the kPeer over the MOI protocol
	if stream, err = s.host.NewStream(s.ctx, peerInfo.ID, config.MOIProtocolStream); err != nil {
		s.logger.Error("Failed to open new stream", "err", err)
		// return error if stream setup fails
		return err
	}

	kPeer = newPeer(stream, s.logger)

	if err = kPeer.InitHandshake(s); err != nil {
		if !errors.Is(err, network.ErrReset) {
			s.logger.Error("Handshake failed", "krama-ID", peerInfo.ID, "err", err)
		}

		return err
	}

	// Register the kPeer to the handler working set
	if err = s.Peers.Register(kPeer); err != nil {
		s.logger.Error("Failed to register", "krama-ID", peerInfo.ID, "err", err)

		return err
	}

	// Update the outbound connection count
	s.connInfo.updateOutboundConnCount(1)

	// Post the event to the registered receivers for NewPeerEvents
	if err = s.postNewPeerEvent(kPeer.networkID); err != nil {
		s.logger.Error("Failed to post new peer event", "err", err)

		return err
	}

	// Post the event to the registered receivers for Syncer
	if err = s.postPeerDiscoveredEvent(kPeer.networkID); err != nil {
		s.logger.Error("Failed to post peer discovery event", "err", err)

		return err
	}

	s.logger.Info("Peer Connected", "krama-ID", kPeer.kramaID)

	// Return a nil error
	return nil
}

func (s *Server) postNewPeerEvent(peerID peer.ID) error {
	if err := s.mux.Post(utils.NewPeerEvent{PeerID: peerID}); err != nil {
		return err
	}

	return nil
}

func (s *Server) postPeerDiscoveredEvent(peerID peer.ID) error {
	if err := s.mux.Post(utils.PeerDiscoveredEvent{ID: peerID}); err != nil {
		return err
	}

	return nil
}

func (s *Server) connectPeer(peerInfo peer.AddrInfo) error {
	// check if the host is already connected to the peer
	if s.isConnectedToPeer(peerInfo.ID) {
		return common.ErrConnectionExists
	}

	// attempt to connect the node host to the peer
	if err := s.host.Connect(s.ctx, peerInfo); err != nil {
		return err
	}

	//	s.logger.Trace("Connect peer success", "from", s.id, "to", peerID)

	return nil
}

// ConnectPeer is a method of Server that connects to peer associated with a given KramaID.
func (s *Server) ConnectPeer(kramaID id.KramaID) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	// get the peer info from peer store
	peerInfo, err := s.GetPeerInfo(peerID)
	if err != nil {
		return err
	}

	err = s.connectPeer(*peerInfo)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) isConnectedToPeer(peerID peer.ID) bool {
	return s.host.Network().Connectedness(peerID) == network.Connected
}

// GetPeerInfo retrieves and returns peer information from the peer store, or from senatus if not found in the store.
func (s *Server) GetPeerInfo(peerID peer.ID) (*peer.AddrInfo, error) {
	// get the peer info from peer store
	peerInfo := s.getFromPeerStore(peerID)
	// retrieves peer information from senatus and adds it to the peer store if it is not present in the store.
	if peerInfo == nil || len(peerInfo.Addrs) == 0 {
		addr, err := s.Senatus.GetAddressByPeerID(peerID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get peer info from senatus")
		}

		peerInfo = &peer.AddrInfo{ID: peerID, Addrs: addr}

		s.AddToPeerStore(peerInfo)
	}

	return peerInfo, nil
}

// getFromPeerStore gets the peer information from the node's peer store
func (s *Server) getFromPeerStore(peerID peer.ID) *peer.AddrInfo {
	peerInfo := s.host.Peerstore().PeerInfo(peerID)

	return &peerInfo
}

// AddToPeerStore adds peer information to the node's peer store
func (s *Server) AddToPeerStore(peerInfo *peer.AddrInfo) {
	s.host.Peerstore().AddAddr(peerInfo.ID, peerInfo.Addrs[0], peerstore.PermanentAddrTTL)
}

// RemoveFromPeerStore removes peer information from the node's peer store
func (s *Server) removeFromPeerStore(peerID peer.ID) {
	s.host.Peerstore().RemovePeer(peerID)
}

// DisconnectPeer is a method of Server that disconnects a peer associated with a given KramaID from the network.
func (s *Server) DisconnectPeer(kramaID id.KramaID) error {
	// Retrieve the libp2pID from the KramaID
	libp2pID, err := kramaID.PeerID()
	if err != nil {
		s.logger.Error("Error parsing libp2p ID from kramaID", "err", err)

		return err
	}

	// Decode the encoded PeerID
	peerID, err := peer.Decode(libp2pID)
	if err != nil {
		s.logger.Error("Error decoding peer ID", "err", err)

		return err
	}

	// Check if the host is already connected to the peer
	if s.host.Network().Connectedness(peerID) == network.Connected {
		if err := s.host.Network().ClosePeer(peerID); err != nil {
			return err
		}
		// Log the successful connection closure to the peer
		s.logger.Info("Peer Disconnected", "peer-ID", peerID)
	}

	// Remove the peer information from peer store
	if s.getFromPeerStore(peerID) != nil {
		s.removeFromPeerStore(peerID)
	}

	return nil
}

func (s *Server) RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error {
	// Register the service with the RPC server of the node
	return s.rpcServers[protocol].RegisterName(serviceName, service)
}

// GetRandomNode returns the libp2p ID of random node from the server's kDHT routing table
func (s *Server) GetRandomNode() peer.ID {
	peers := s.kadDHT.RoutingTable().ListPeers()

	// TODO: Improve the seed
	s1 := rand.NewSource(time.Now().UnixNano())
	reg := rand.New(s1)
	index := reg.Intn(len(peers))

	return peers[index]
}

// SendMessage sends message of Poorna MsgType's to a given libp2p id
func (s *Server) SendMessage(peerID peer.ID, msgType networkmsg.MsgType, msg networkmsg.Payload) error {
	var (
		stream  network.Stream
		rw      *bufio.ReadWriter
		rawData []byte
		err     error
	)

	if s.Peers.ContainsPeer(peerID) {
		p := s.Peers.Peer(peerID)

		return p.Send(s.id, msgType, msg)
	}

	if stream, err = s.NewStream(s.ctx, peerID, config.MOIProtocolStream); err != nil {
		// Return error if stream setup fails
		return err
	}
	// Create a new read/write buffer
	rw = bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	if rawData, err = generateWireMessage(s.id, msgType, msg); err != nil {
		return err
	}

	return shipMessage(rw, rawData)
}

func shipMessage(rw *bufio.ReadWriter, data []byte) error {
	// Write the message bytes into the peer's io buffer
	writer := msgio.NewWriter(rw.Writer)
	if err := writer.WriteMsg(data); err != nil {
		return err
	}

	return rw.Writer.Flush()
}

func generateWireMessage(
	senderKramaID id.KramaID,
	msgType networkmsg.MsgType,
	msg networkmsg.Payload,
) ([]byte, error) {
	payloadRawData, err := msg.Bytes()
	if err != nil {
		return nil, fmt.Errorf("polorize message payload %w", err)
	}

	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into a slice of bytes
	m := networkmsg.Message{
		MsgType: msgType,
		Payload: payloadRawData,
		Sender:  senderKramaID,
	}

	rawData, err := m.Bytes()
	if err != nil {
		return nil, err
	}

	return rawData, nil
}

func (s *Server) NewStream(ctx context.Context, id peer.ID, protocol protocol.ID) (network.Stream, error) {
	return s.host.NewStream(ctx, id, protocol)
}

// setStreamHandler starts stream handler for the ProtocolID present in the node's config
func (s *Server) setStreamHandler() {
	s.host.SetStreamHandler(config.MOIProtocolStream, s.streamHandlerFunc)
}

// Stop terminates the running poorna server gracefully
func (s *Server) Stop() {
	s.ctxCancel()
}

// GetAddrs fetches the Multiaddr of the node
func (s *Server) GetAddrs() []maddr.Multiaddr {
	return s.host.Addrs()
}

// SendHelloMessage sends a hello message to complete network using PubSub
func (s *Server) SendHelloMessage() {
	s.init.Do(func() {
		msg := networkmsg.HelloMsg{
			KramaID:   s.GetKramaID(),
			Address:   utils.MultiAddrToString(s.GetAddrs()...),
			Signature: nil,
		}

		rawMsg, err := msg.Bytes()
		if err != nil {
			s.logger.Error("Error polorising message", "err", err)

			return
		}

		signature, err := s.vault.Sign(rawMsg, mcommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
		if err != nil {
			s.logger.Error("Failed to sign hello message", "err", err)

			return
		}

		msg.Signature = signature

		rawData, err := msg.Bytes()
		if err != nil {
			s.logger.Error("Failed to serialize hello message", "err", err)

			return
		}

		if err = s.Broadcast(config.SenatusTopic, rawData); err != nil {
			s.logger.Error("Failed to broadcast hello message", "err", err)

			return
		}
	})
}

func (s *Server) constructHandshakeMSG() (*networkmsg.HandshakeMSG, error) {
	ntq, err := s.Senatus.GetNTQ(s.GetKramaID())
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch ntq")
	}

	return networkmsg.ConstructHandshakeMSG(utils.MultiAddrToString(s.host.Addrs()...), ntq, 0, ""), nil
}

func (s *Server) MinimumPeersCount() uint32 {
	minimumCount := (s.connInfo.getMaxInboundConnCount() + s.connInfo.getMaxOutboundConnCount()) / 3

	return uint32(minimumCount)
}

func (s *Server) GetPeers() []id.KramaID {
	s.Peers.lock.Lock()
	defer s.Peers.lock.Unlock()

	peers := make([]id.KramaID, 0)

	for _, peerInfo := range s.Peers.peers {
		peers = append(peers, peerInfo.kramaID)
	}

	return peers
}

func (s *Server) GetBootstrapPeerIDs() (map[peer.ID]bool, error) {
	addrInfo, err := peer.AddrInfosFromP2pAddrs(s.cfg.BootstrapPeers...)
	if err != nil {
		return nil, errors.Wrap(err, "unable to extract addr-info from multiaddr for bootstrap nodes")
	}

	peerIDs := make(map[peer.ID]bool)

	for _, info := range addrInfo {
		peerIDs[info.ID] = true
	}

	return peerIDs, nil
}

func (s *Server) GetVersion() string {
	return config.ProtocolVersion
}

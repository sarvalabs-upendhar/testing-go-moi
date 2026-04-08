package p2p

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

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
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-msgio"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
	mcommon "github.com/sarvalabs/go-moi/crypto/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/network/rpc"
	"github.com/sarvalabs/go-moi/senatus"
)

type Vault interface {
	GetNetworkPrivateKey() crypto.PrivateKey
	Sign(data []byte, sigType mcommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
}

type Senatus interface {
	GetNTQ(peerID identifiers.KramaID) (float32, error)
	GetAddress(key identifiers.KramaID) ([]maddr.Multiaddr, error)
	GetAddressByPeerID(peerID peer.ID) ([]maddr.Multiaddr, error)
	GetRTTByPeerID(peerID peer.ID) (int64, error)
	GetKramaIDByPeerID(peerID peer.ID) (identifiers.KramaID, error)
	UpdatePeer(data *senatus.NodeMetaInfo) error
	AddNewPeerWithPeerID(key peer.ID, data *senatus.NodeMetaInfo) error
}

// Server is a struct that represents a node on the network
type Server struct {
	ctx       context.Context // context of server lifecycle
	ctxCancel context.CancelFunc
	logger    hclog.Logger
	cfg       *config.NetworkConfig // config of the node

	host     host.Host      // libp2p host of the node
	kadDHT   *kdht.IpfsDHT  // libp2p Kad DHT of the node
	psRouter *pubsub.PubSub // libp2p PubSub router of the node

	id identifiers.KramaID // KramaID of the node

	Peers      *peerSet // peerSet of node
	peerScores *PeerScores

	ds *DiscoveryService

	ConnManager *ConnectionManager // connection info of the node

	rpcServers map[protocol.ID]*rpc.Server // map of MOI-RPC Server's Protocol ID to respective MOI-RPC Server

	pubSubTopics *pubSubTopics

	vault   Vault   // Vault interface to access network keys and to sign messages
	Senatus Senatus // Senatus interface to access reputation info

	mux *utils.TypeMux // typemux of the node

	init    sync.Once
	metrics *Metrics

	basicSeqnoValidator pubsub.ValidatorEx // default validation for messages
	msgs                chan *networkmsg.Message

	inboundStreamsMx sync.Mutex
	inboundStreams   map[peer.ID]network.Stream
}

// NewServer is a constructor function that generates, configures and returns a Server.
// Accepts lifecycle context for the node along with a typemux and a config.
func NewServer(
	logger hclog.Logger,
	id identifiers.KramaID,
	mux *utils.TypeMux,
	config *config.NetworkConfig,
	vault Vault,
	metrics *Metrics,
) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		id:                  id,
		ctx:                 ctx,
		ctxCancel:           cancel,
		logger:              logger.Named("P2P-Server"),
		cfg:                 config,
		mux:                 mux,
		Peers:               newPeerSet(),
		peerScores:          newPeerScores(),
		rpcServers:          make(map[protocol.ID]*rpc.Server),
		vault:               vault,
		metrics:             metrics,
		basicSeqnoValidator: pubsub.NewBasicSeqnoValidator(newpeerMsgNonceStore(), slog.Default()),
		msgs:                make(chan *networkmsg.Message, 100),
		inboundStreams:      make(map[peer.ID]network.Stream),
	}

	return server
}

func (s *Server) Close() error {
	if err := s.host.Close(); err != nil {
		return err
	}

	s.ctxCancel()

	return nil
}

func (s *Server) GetPeersScores() map[peer.ID]*pubsub.PeerScoreSnapshot {
	return s.peerScores.Get()
}

func (s *Server) AddPeerInfo(info peer.AddrInfo) {
	s.host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.ConnectedAddrTTL)
}

func (s *Server) AddPeerInfoPermanently(info *peer.AddrInfo) {
	s.host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
}

func (s *Server) GetAddrsFromPeerStore(peerID peer.ID) []maddr.Multiaddr {
	return s.host.Peerstore().Addrs(peerID)
}

func (s *Server) validateNetworkConfig() error {
	if s.cfg.DiscoveryInterval < time.Second {
		return errors.New("discovery interval cannot be less than 1 second")
	}

	return nil
}

func (s *Server) SetupServer() error {
	if err := s.validateNetworkConfig(); err != nil {
		return err
	}

	if err := s.setupHost(); err != nil {
		return fmt.Errorf("setup host: %w", err)
	}

	if err := s.setupPubSub(); err != nil {
		return fmt.Errorf("setup PubSub: %w", err)
	}

	s.ds = NewDiscoveryService(s)
	s.ConnManager = NewConnectionManager(s)

	return nil
}

// StartServer sets up node's libp2p host, Kad DHT, PubSub route
// after which it bootstraps itself by attempting to connect to bootstrap peers.
func (s *Server) StartServer() error {
	s.logger.Info("Starting server", "krama-id", s.id, "addr", s.host.Addrs())

	if err := s.ConnManager.Start(); err != nil {
		return err
	}

	if s.cfg.NoDiscovery && !s.cfg.RefreshSenatus {
		return nil
	}

	s.ds.Start()

	return nil
}

// StartNewRPCServer starts a new MOI-RPC server & client with the given ProtocolID
// adds the server to map of poorna-server's rpcServers map and returns Client
func (s *Server) StartNewRPCServer(protocol protocol.ID, tag string) *rpc.Client {
	s.logger.Trace("Starting new MOI-RPC server", "protocol-ID", protocol)

	s.rpcServers[protocol] = rpc.NewServer(s.logger.Named(string(protocol)), s.ConnManager, tag, protocol)

	return rpc.NewClient(s.logger.Named(string(protocol)), s.ConnManager, protocol, s.Senatus)
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

	mgr, err := connmgr.NewConnManager(s.cfg.MinimumConnections, s.cfg.MaximumConnections)
	if err != nil {
		return nil, err
	}

	rmOpts := make([]rcmgr.Option, 0)

	if s.cfg.ConnRateLimiter != nil {
		rmOpts = append(rmOpts, rcmgr.WithConnRateLimiters(s.cfg.ConnRateLimiter))
	}

	if len(s.cfg.IPV4ConnLimitPerSubnet) > 0 || len(s.cfg.IPV6ConnLimitPerSubnet) == 0 {
		rmOpts = append(rmOpts, rcmgr.WithLimitPerSubnet(s.cfg.IPV4ConnLimitPerSubnet, s.cfg.IPV6ConnLimitPerSubnet))
	}

	resourceManager, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits), rmOpts...)
	if err != nil {
		return nil, err
	}

	// filters out address based on server config flags
	addrsFactory, err := makeAddrsFactory(s.cfg.DisablePrivateIP, s.cfg.AllowIPv6Addresses, s.cfg.PublicP2PAddresses)
	if err != nil {
		return nil, err
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
		libp2p.ConnectionGater(NewConnectionGater(s.cfg.DisablePrivateIP)),
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

// GetKramaID returns the KramaID of node
func (s *Server) GetKramaID() identifiers.KramaID {
	return s.id
}

func (s *Server) GetPeerSetLen() int {
	return s.Peers.Len()
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

// ConnectPeerByKramaID connects to peer associated with a given KramaID.
func (s *Server) ConnectPeerByKramaID(ctx context.Context, kramaID identifiers.KramaID) error {
	return s.ConnManager.ConnectPeerByKramaID(ctx, kramaID)
}

// RegisterNewRPCService registers the service with the RPC server of the node
func (s *Server) RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error {
	return s.rpcServers[protocol].RegisterName(serviceName, service)
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

	if stream, err = s.ConnManager.NewStream(s.ctx, peerID, config.MOIProtocolStream, MOIStreamTag); err != nil {
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
	senderKramaID identifiers.KramaID,
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

		if err = s.Broadcast(config.HelloTopic, rawData); err != nil {
			s.logger.Error("Failed to broadcast hello message", "err", err)

			return
		}
	})
}

func (s *Server) GetPeersCount() int {
	return s.Peers.Len()
}

func (s *Server) GetVersion() string {
	return config.ProtocolVersion
}

func (s *Server) GetPeers() []identifiers.KramaID {
	return s.ConnManager.getPeers()
}

func (s *Server) GetConns() []network.Conn {
	return s.ConnManager.getConns()
}

func (s *Server) GetInboundConnCount() int64 {
	return s.ConnManager.getInboundConnCount()
}

func (s *Server) GetOutboundConnCount() int64 {
	return s.ConnManager.getOutboundConnCount()
}

func (s *Server) constructHandshakeMSG() (*networkmsg.HandshakeMSG, error) {
	return networkmsg.ConstructHandshakeMSG([]byte("ping")), nil
}

func (s *Server) MsgChan() chan *networkmsg.Message {
	return s.msgs
}

// handlePeerMessage is a method of SubHandler that handles a message from a Peer
func (s *Server) handlePeerMessage(p *Peer) error {
	// Read the peer's io read/writer into a buffer
	// p.mtxLock.Lock()
	// defer p.mtxLock.Unlock()
	reader := msgio.NewReader(p.rw.Reader)

	buffer, err := reader.ReadMsg()
	if err != nil {
		return err
	}

	// Unmarshal the buffer into a proto message
	message := new(networkmsg.Message)
	if err := message.FromBytes(buffer); err != nil {
		return err
	}

	select {
	case <-s.ctx.Done():
	case s.msgs <- message:
	}

	return nil
}

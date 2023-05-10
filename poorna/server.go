package poorna

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-msgio"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	kdht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/guna/senatus"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/mudra"
	mcommon "github.com/sarvalabs/moichain/mudra/common"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/moirpc"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

const (
	SenatusTopic            = "MOI_PUBSUB_SENATUS"
	MinimumPeerCount        = 3
	MinimumBootNodeConn int = 1
)

var (
	ErrNoBootnodes      = errors.New("no bootnodes")
	ErrMinBootnodes     = fmt.Errorf("minimum %v bootnode is required", MinimumBootNodeConn)
	ErrMinBootnodeConns = fmt.Errorf("unable to connect to at least %v bootstrap node(s)", MinimumBootNodeConn)
)

type Vault interface {
	GetNetworkPrivateKey() mudra.PrivateKey
	Sign(data []byte, sigType mcommon.SigType) ([]byte, error)
}
type Senatus interface {
	GetNTQ(peerID id.KramaID) (float32, error)
	GetAddress(key id.KramaID) ([]maddr.Multiaddr, error)
	GetAddressByPeerID(peerID peer.ID) ([]maddr.Multiaddr, error)
	AddNewPeer(key id.KramaID, data *gtypes.NodeMetaInfo) error
	AddNewPeerWithPeerID(key peer.ID, data *gtypes.NodeMetaInfo) error
}

// TopicSet is a wrapper for Topic & Subscription
type TopicSet struct {
	topicHandle *pubsub.Topic        // PubSub topic handler
	subHandle   *pubsub.Subscription // PubSub subscription handler
}

// pubSubTopics is a struct that represents a set of pub-sub topics sucscribed by the node
type pubSubTopics struct {
	psTopics     map[string]*TopicSet // map of PubSub topic names to respective TopicSet
	topicSetLock sync.RWMutex         // lock for the psTopics map
}

// Server is a struct that represents a node on the network
type Server struct {
	ctx       context.Context // context of server lifecycle
	ctxCancel context.CancelFunc
	logger    hclog.Logger
	cfg       *common.NetworkConfig // config of the node

	host      host.Host     // libp2p host of the node
	kadDHT    *kdht.IpfsDHT // libp2p Kad DHT of the node
	discovery *discovery.RoutingDiscovery
	psRouter  *pubsub.PubSub // libp2p PubSub router of the node

	id id.KramaID // KramaID of the node

	Peers *peerSet // peerSet of node

	connInfo *ConnectionInfo // connection info of the node

	rpcServers map[protocol.ID]*moirpc.Server // map of MOI-RPC Server's Protocol ID to respective MOI-RPC Server

	pubSubTopics *pubSubTopics

	vault   Vault   // Vault interface to access network keys and to sign messages
	Senatus Senatus // Senatus interface to access reputation info

	mux *utils.TypeMux // typemux of the node

	init sync.Once
}

// NewServer is a constructor function that generates, configures and returns a Server.
// Accepts lifecycle context for the node along with a typemux and a config.
func NewServer(
	parentCtx context.Context,
	logger hclog.Logger,
	id id.KramaID,
	mux *utils.TypeMux,
	config *common.NetworkConfig,
	vault Vault,
) *Server {
	ctx, ctxCancel := context.WithCancel(parentCtx)
	server := &Server{
		id:         id,
		ctx:        ctx,
		ctxCancel:  ctxCancel,
		logger:     logger.Named("Poorna"),
		cfg:        config,
		mux:        mux,
		Peers:      newPeerSet(),
		connInfo:   NewConnectionInfo(config.InboundConnLimit, config.OutboundConnLimit),
		rpcServers: make(map[protocol.ID]*moirpc.Server),
		vault:      vault,
	}

	return server
}

func (s *Server) SetupServer() error {
	if err := s.setupHost(); err != nil {
		return fmt.Errorf("setup host: %w", err)
	}

	if err := s.setupPubSub(); err != nil {
		return fmt.Errorf("setup PubSub: %w", err)
	}

	s.logger.Info("[StartServer]", "Krama-ID", s.id, "Address", s.host.Addrs())

	if err := s.connectToBootStrapNodes(); err != nil {
		return fmt.Errorf("bootstrap nodes connection: %w", err)
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

	go s.discover()

	return nil
}

// Attempts connecting to all the bootstrap nodes in config
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

	s.logger.Info("BootNodes", s.cfg.BootstrapPeers)

	for _, bootstrapPeer := range s.cfg.BootstrapPeers {
		if err := s.connectToMaddr(bootstrapPeer); err != nil {
			s.logger.Error("Bootstrap connection failed", "peer", bootstrapPeer, "error", err)

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
			s.logger.Error("invalid trusted peer address", "error", err)

			continue
		}

		if err := s.ConnectAndRegisterPeer(*peerInfo); err != nil {
			s.logger.Error("failed to establish connection with trusted peer", "error", err)
		}
	}
}

// StartNewRPCServer starts a new MOI-RPC server & client with the given ProtocolID
// adds the server to map of poorna-server's rpcServers map and returns Client
func (s *Server) StartNewRPCServer(protocol protocol.ID) *moirpc.Client {
	s.logger.Debug("starting new moirpc server", "protocol", protocol)

	s.rpcServers[protocol] = moirpc.NewServer(s.logger.Named(string(protocol)), s.host, protocol)

	return moirpc.NewClient(s.logger.Named(string(protocol)), s.host, protocol, s.Senatus)
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

	return libp2p.ChainOptions(
		// Enable UPnP and hole punching
		s.getSelfRouting(),
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.Identity(prvKey),
		libp2p.ListenAddrs(s.cfg.ListenAddresses...),
	), nil
}

func (s *Server) getPrivateKey() (crypto.PrivKey, error) {
	prvKey, err := crypto.UnmarshalSecp256k1PrivateKey(s.vault.GetNetworkPrivateKey().Bytes())
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
		kdht.ProtocolPrefix(common.MOIProtocolStream),
	}
}

// creates a custom gossipSub parameter set.
func pubsubGossipParam() pubsub.GossipSubParams {
	gParams := pubsub.DefaultGossipSubParams()
	gParams.Dlo = 6
	gParams.D = 8
	gParams.Dhi = 16
	gParams.HeartbeatInterval = 500 * time.Millisecond

	return gParams
}

func (s *Server) setPubSubRouter() error {
	var err error

	s.psRouter, err = pubsub.NewGossipSub(
		s.ctx,
		s.host,
		[]pubsub.Option{
			pubsub.WithGossipSubParams(pubsubGossipParam()),
		}...,
	)
	if err != nil {
		return err
	}

	return nil
}

// Sets up the PubSub router for the node.
// Expects the node to already be configured with a libp2p host.
// Returns any error that occurs during the setup.
//
// Creates a new FloodSub router and map of topic sets for the node.
func (s *Server) setupPubSub() (err error) {
	if s.host == nil {
		return errors.New("libp2p host for node not configured")
	}

	// initialize an empty pubSubTopics
	s.pubSubTopics = &pubSubTopics{
		psTopics:     make(map[string]*TopicSet),
		topicSetLock: sync.RWMutex{},
	}

	// create a PubSub router for the Server with the pubsub options
	err = s.setPubSubRouter()
	if err != nil {
		return fmt.Errorf("new gossip sub %w", err)
	}

	return nil
}

// streamHandlerFunc is a method of Server that defines the behaviour of stream
// handling for streams acquired over the protocol.
//
// Creates a NewPeerEvent when a new stream is acquired and posts it
// to the network for all receivers registered to receive the event.
func (s *Server) streamHandlerFunc(stream network.Stream) {
	if s.connInfo.isInboundConnLimitReached() {
		s.logger.Error("Closing peer connection", stream.Conn().RemotePeer())

		if err := stream.Reset(); err != nil {
			s.logger.Error("Failed to reset stream", "error", err)
		}

		return
	}

	s.logger.Trace("new stream", "protocol", stream.Protocol(), "kPeer", stream.Conn().RemotePeer())

	kPeer := newPeer(stream, s.logger)

	if err := kPeer.handleHandshakeMessage(); err != nil {
		if err := kPeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Handle Handshake", "error", err)
		}

		return
	}

	// Register the kPeer to the handler working set
	if err := s.Peers.Register(kPeer); err != nil {
		if err := kPeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Handshake err response", "error", err)
		}

		return
	}

	// Update the inbound connection count
	s.connInfo.updateInboundConnCount(1)

	if err := kPeer.SendHandshakeMessage(s); err != nil {
		s.logger.Error("SendHandshakeMessage", "error", err)

		return
	}

	// Post the event to the registered receivers for NewPeerEvents
	if err := s.postNewPeerEvent(kPeer.networkID); err != nil {
		s.logger.Error("streamHandlerFunc", "error", err)

		return
	}

	s.logger.Info("Handled stream, connected and registered", "kPeer", kPeer.kramaID)
}

// discover is a method of Server that starts a discovery routine using a libp2p routing
// discovery mechanism that uses the Kad DHT of the node.
//
// Advertises the protocol rendezvous and discovers other peers that are advertising it.
// The peer discovery process is repeated every 5 seconds.
func (s *Server) discover() {
	s.discovery = discovery.NewRoutingDiscovery(s.kadDHT)

	// Advertise the rendezvous string to the discovery service
	s.logger.Info("Announcing ourselves")

	// TODO: explore about how many times to advertise
	_, err := s.discovery.Advertise(s.ctx, string(common.MOIProtocolStream))
	if err != nil {
		s.logger.Error("Failed to advertise the rendezvous string to the discovery service", "error", err)
	}

	// discover other peers that are advertising themselves
	s.logger.Info("Starting discovery routine")

	for {
		select {
		case <-time.After(3 * time.Second):
		case <-s.ctx.Done():
			return
		}

		if err = s.handleDiscovery(); err != nil {
			s.logger.Error("handle discovery", "error", err)
		}
	}
}

func (s *Server) handleDiscovery() error {
	// Retrieve a channel of peer addresses from the discovery service
	peerChan, err := s.discovery.FindPeers(s.ctx, string(common.MOIProtocolStream))
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
			err = s.Senatus.AddNewPeerWithPeerID(p.ID, &gtypes.NodeMetaInfo{
				Addrs: utils.MultiAddrToString(p.Addrs...),
				NTQ:   senatus.DefaultPeerNTQ,
			})
			if err != nil && !errors.Is(err, types.ErrAlreadyKnown) {
				s.logger.Error("failed to add peer information to senatus", err)

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
		return types.ErrOutboundConnLimit
	}

	if s.Peers.ContainsPeer(peerInfo.ID) {
		return errAlreadyRegistered
	}

	if err = s.connectPeer(peerInfo.ID); err != nil && !errors.Is(err, types.ErrConnectionExists) {
		return err
	}

	// create a new stream to the kPeer over the MOI protocol
	if stream, err = s.host.NewStream(s.ctx, peerInfo.ID, common.MOIProtocolStream); err != nil {
		s.logger.Error("Failed to open NewStream", "error", err)
		// return error if stream setup fails
		return err
	}

	kPeer = newPeer(stream, s.logger)

	if err = kPeer.InitHandshake(s); err != nil {
		if !errors.Is(err, types.ErrStreamReset) {
			s.logger.Error("Handshake Failed", "kPeer", peerInfo.ID, "error", err)
		}

		return err
	}

	// Register the kPeer to the handler working set
	if err = s.Peers.Register(kPeer); err != nil {
		s.logger.Error("Failed to Register", "kPeer", peerInfo.ID, "error", err)

		return err
	}

	// Update the outbound connection count
	s.connInfo.updateOutboundConnCount(1)

	// Post the event to the registered receivers for NewPeerEvents
	if err = s.postNewPeerEvent(kPeer.networkID); err != nil {
		s.logger.Error("Failed to post new peer event", err)

		return err
	}

	// Post the event to the registered receivers for Syncer
	if err = s.postPeerDiscoveredEvent(kPeer.networkID); err != nil {
		s.logger.Error("Failed to post peer discovery event", err)

		return err
	}

	s.logger.Info("Connected and registered kPeer successfully ", "id", kPeer.kramaID)

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

func (s *Server) connectPeer(peerID peer.ID) error {
	// check if the host is already connected to the peer
	if s.isConnectedToPeer(peerID) {
		return types.ErrConnectionExists
	}

	// get the peer info from peer store
	peerInfo, err := s.getPeerInfo(peerID)
	if err != nil {
		return err
	}

	// attempt to connect the node host to the peer
	if err := s.host.Connect(s.ctx, *peerInfo); err != nil {
		return err
	}

	//	s.logger.Debug("connect peer success", "from", s.id, "to", peerID)

	return nil
}

// ConnectPeer is a method of Server that connects to peer associated with a given KramaID.
func (s *Server) ConnectPeer(kramaID id.KramaID) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	err = s.connectPeer(peerID)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) isConnectedToPeer(peerID peer.ID) bool {
	return s.host.Network().Connectedness(peerID) == network.Connected
}

// getPeerInfo retrieves and returns peer information from the peer store, or from senatus if not found in the store.
func (s *Server) getPeerInfo(peerID peer.ID) (*peer.AddrInfo, error) {
	// get the peer info from peer store
	peerInfo := s.getFromPeerStore(peerID)
	// retrieves peer information from senatus and adds it to the peer store if it is not present in the store.
	if peerInfo == nil || len(peerInfo.Addrs) == 0 {
		addr, err := s.Senatus.GetAddressByPeerID(peerID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get peer info from senatus")
		}

		peerInfo = &peer.AddrInfo{ID: peerID, Addrs: addr}

		s.addToPeerStore(peerInfo)
	}

	return peerInfo, nil
}

// getFromPeerStore gets the peer information from the node's peer store
func (s *Server) getFromPeerStore(peerID peer.ID) *peer.AddrInfo {
	peerInfo := s.host.Peerstore().PeerInfo(peerID)

	return &peerInfo
}

// addToPeerStore adds peer information to the node's peer store
func (s *Server) addToPeerStore(peerInfo *peer.AddrInfo) {
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
		s.logger.Error("Error parsing krama libp2pID", err)

		return err
	}

	// Decode the encoded PeerID
	peerID, err := peer.Decode(libp2pID)
	if err != nil {
		s.logger.Error("Error decoding peerID", err)

		return err
	}

	// Check if the host is already connected to the peer
	if s.host.Network().Connectedness(peerID) == network.Connected {
		if err := s.host.Network().ClosePeer(peerID); err != nil {
			return err
		}
		// Log the successful connection closure to the peer
		s.logger.Info("Disconnected", "peer", peerID)
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

// Broadcast is a method of Server that broadcasts a given polo message to a
// given PubSub topic. Expects the node to be subscribed to that topic.
func (s *Server) Broadcast(topicName string, data []byte) error {
	s.pubSubTopics.topicSetLock.RLock()
	defer s.pubSubTopics.topicSetLock.RUnlock()

	topicSet := s.pubSubTopics.getTopicSet(topicName)
	if topicSet == nil {
		return errors.New("topic not found")
	}

	// Attempt to publish the message to the pubsub topic
	if err := topicSet.topicHandle.Publish(s.ctx, data); err != nil {
		s.logger.Error("PubSub Topic Publish", "topic", topicName, "error", err)
		// Return the error
		return err
	}

	return nil
}

// JoinPubSubTopic joins the pubsub topic and returns the TopicSet with topic handler only
// Note that this doesn't subscribe to the given topic, Call Subscribe for creating a subscription handler
func (s *Server) JoinPubSubTopic(topicName string) (*TopicSet, error) {
	s.pubSubTopics.topicSetLock.Lock()
	defer s.pubSubTopics.topicSetLock.Unlock()

	topicSet := s.pubSubTopics.getTopicSet(topicName)
	if topicSet == nil {
		topic, err := s.psRouter.Join(topicName)
		if err != nil {
			return nil, err
		}

		s.pubSubTopics.addTopicSet(topicName, &TopicSet{topic, nil})
	}

	return s.pubSubTopics.getTopicSet(topicName), nil
}

// Subscribe subscribes the node to a given PubSub topic.
// Accepts the topic name to subscribe and handler function to handle messages from that subscription.
//
// Creates topic and subscription handles for the topic, wraps it in a TopicSet
// and adds it to the node's pubsub topicset. Creates a handler pipeline with the
// given handler function and starts a subscription loop that invokes the pipeline.
func (s *Server) Subscribe(ctx context.Context, topicName string, handler func(msg *pubsub.Message) error) error {
	s.pubSubTopics.topicSetLock.Lock()
	defer s.pubSubTopics.topicSetLock.Unlock()

	// Join pubsub topic and get a topic handle
	topicHandle, err := s.psRouter.Join(topicName)
	if err != nil {
		// Return the error
		return err
	}

	// Subscribe to the topic and get a subscription handle
	subcHandle, err := topicHandle.Subscribe()
	if err != nil {
		// Return the error
		return err
	}

	s.pubSubTopics.addTopicSet(topicName, &TopicSet{topicHandle, subcHandle})

	go s.routeSubscriptionMessages(ctx, topicName, handler, subcHandle)

	return nil
}

// routeSubscriptionMessages listens to the messages over the subscription
// and calls the respective handler with message
func (s *Server) routeSubscriptionMessages(
	ctx context.Context,
	topicName string,
	handler func(msg *pubsub.Message) error,
	subcHandle *pubsub.Subscription,
) {
	pipeline := func(msg *pubsub.Message) {
		// Call the given subscription handler
		// an error because it is being invoked as a goroutine
		if err := handler(msg); err != nil {
			if !errors.Is(err, types.ErrAlreadyKnown) {
				s.logger.Error("subcHandlerPipeline", "error calling handler method", err)
			}

			return
		}
	}

	for {
		// Retrieve the next message from the subscription
		msg, err := subcHandle.Next(ctx)
		if err != nil {
			s.logger.Error("Topic subscription closed", "topic", topicName)

			return
		}

		// Skip handling self published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		go pipeline(msg)
	}
}

// Unsubscribe is a method of Server that unsubscribes the node from a given PubSub topic
func (s *Server) Unsubscribe(topicName string) error {
	s.pubSubTopics.topicSetLock.Lock()
	defer s.pubSubTopics.topicSetLock.Unlock()

	topicSet := s.pubSubTopics.getTopicSet(topicName)

	// Check if topic exists
	if topicSet == nil {
		return nil
	}

	// Cancel the subscription to the topic
	topicSet.subHandle.Cancel()

	// Attempt to close the topic handler for the topic
	if err := topicSet.topicHandle.Close(); err != nil {
		return err
	}

	s.pubSubTopics.deleteTopicSet(topicName)

	return nil
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

// SendMessage sends message of Poorna MsgType's to a given given libp2p id
func (s *Server) SendMessage(peerID peer.ID, msgType ptypes.MsgType, msg ptypes.MessagePayload) error {
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

	if stream, err = s.NewStream(s.ctx, peerID, common.MOIProtocolStream); err != nil {
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

func generateWireMessage(senderKramaID id.KramaID, msgType ptypes.MsgType, msg ptypes.MessagePayload) ([]byte, error) {
	payloadRawData, err := msg.Bytes()
	if err != nil {
		return nil, fmt.Errorf("polorize message payload %w", err)
	}

	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into a slice of bytes
	m := ptypes.Message{
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
	s.host.SetStreamHandler(common.MOIProtocolStream, s.streamHandlerFunc)
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
		msg := ptypes.HelloMsg{
			KramaID:   s.GetKramaID(),
			Address:   utils.MultiAddrToString(s.GetAddrs()...),
			Signature: nil,
		}

		rawMsg, err := msg.Bytes()
		if err != nil {
			s.logger.Error("Error polorising message", "error", err)
			panic(err)
		}

		signature, err := s.vault.Sign(rawMsg, mcommon.BlsBLST)
		if err != nil {
			s.logger.Error("Error signing message", "error", err)
			panic(err)
		}

		msg.Signature = signature

		rawData, err := msg.Bytes()
		if err != nil {
			s.logger.Error("Error serializing hello message", "error", err)
			panic(err)
		}

		if err = s.Broadcast(SenatusTopic, rawData); err != nil {
			s.logger.Error("Error broadcasting hello message", "error", err)
			panic(err)
		}
	})
}

func (s *Server) constructHandshakeMSG() (ptypes.HandshakeMSG, error) {
	ntq, err := s.Senatus.GetNTQ(s.GetKramaID())
	if err != nil {
		return ptypes.NilHandshakeMSG, fmt.Errorf("get NTQ %w", err)
	}

	handShakeMessage := ptypes.ConstructHandshakeMSG(utils.MultiAddrToString(s.host.Addrs()...), ntq, 0, "")

	return handShakeMessage, nil
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

func (s *Server) GetVersion() string {
	return common.ProtocolVersion
}

func (pst *pubSubTopics) addTopicSet(topicName string, topicSet *TopicSet) {
	pst.psTopics[topicName] = topicSet
}

func (pst *pubSubTopics) getTopicSet(topicName string) *TopicSet {
	return pst.psTopics[topicName]
}

func (pst *pubSubTopics) deleteTopicSet(topicName string) {
	delete(pst.psTopics, topicName)
}

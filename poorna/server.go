package poorna

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/libp2p/go-libp2p"

	"gitlab.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/routing"
	maddr "github.com/multiformats/go-multiaddr"
	"gitlab.com/sarvalabs/moichain/mudra"
	mcommon "gitlab.com/sarvalabs/moichain/mudra/common"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"

	lrpc "github.com/libp2p/go-libp2p-gorpc"
	kdht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/types"
)

const (
	SenatusTopic = "MOI_PUBSUB_SENATUS"

	MinimumPeerCount = 3
)

type Vault interface {
	GetNetworkPrivateKey() mudra.PrivateKey
	Sign(data []byte, sigType mcommon.SigType) ([]byte, error)
}
type Senatus interface {
	SenatusHandler(msg *pubsub.Message) error
	GetNTQ(id id.KramaID) (int32, error)
}

// TopicSet is a struct that represents a wrapper for  topic handlers which
// include the topic and the subscription handlers for that topic
type TopicSet struct {
	// Represents the PubSub topic handler
	topicHandle *pubsub.Topic

	// Represents the PubSub subscription handler
	subHandle *pubsub.Subscription
}

// Server is a struct that represents a node on the KIP network
type Server struct {
	// Represents the KipID of the node
	id id.KramaID

	// Represents the context of server lifecycle
	ctx context.Context

	// Context cancel function
	ctxCancel context.CancelFunc

	// Represents the config of the node
	cfg *common.NetworkConfig

	// Represents the libp2p host of the node
	host host.Host

	// Represents the libp2p Kad DHT of the node
	KadDHT *kdht.IpfsDHT

	// Represents the libp2p PubSub router of the node
	PSrouter *pubsub.PubSub

	// Represents a mapping of topic names to their topic and subscription handlers
	// The topic and subscription handlers are wrapped in a TopicSet object.
	pstopics map[string]*TopicSet

	topicSetLock sync.RWMutex

	// Represents the KIP typemux of the node
	mux *utils.TypeMux

	rpcServers map[protocol.ID]*lrpc.Server

	discovery *discovery.RoutingDiscovery

	Peers *peerSet

	logger hclog.Logger

	// vault interface to access network keys and sign messages
	vault Vault

	// Senatus interface to access reputation info
	Senatus Senatus

	init sync.Once
}

// NewServer is a constructor function that generates, configures and returns a Server.
// Accepts lifecycle context for the node along with a typemux and a KIP config.
//
// Creates the node and sets up its libp2p host, Kad DHT, PubSub router and p2p RPC server,
// after which it bootstraps itself by attempting to connect to KIP bootstrap peers.
// Display the KipID of the node after it has been successfully set up.
func NewServer(
	parentCtx context.Context,
	logger hclog.Logger,
	id id.KramaID,
	mux *utils.TypeMux,
	config *common.NetworkConfig,
	vault Vault,
) (*Server, error) {
	ctx, ctxCancel := context.WithCancel(parentCtx)
	// Create a new Server
	kn := &Server{
		id:         id,
		ctx:        ctx,
		ctxCancel:  ctxCancel,
		logger:     logger.Named("Poorna"),
		cfg:        config,
		mux:        mux,
		Peers:      newPeerSet(),
		rpcServers: make(map[protocol.ID]*lrpc.Server),
		vault:      vault,
	}

	// HashSet up the node's Host
	if err := kn.setupHost(); err != nil {
		// Return the error
		return nil, fmt.Errorf("host setup error: %w", err)
	}

	// HashSet up the node's PubSub router
	if err := kn.setupPubSub(); err != nil {
		// Return the error
		return nil, fmt.Errorf("pubsub setup error: %w", err)
	}

	// Print the Krama ID
	kn.logger.Info("[NewServer]", "Krama-ID", kn.id, "Addrs", kn.host.Addrs())
	// Declare a wait-group
	var wg sync.WaitGroup
	// Iterate over the configured KIP bootstrap peers
	for _, peerAddr := range config.BootstrapPeers {
		// Retrieve the peer address from its multiaddr
		peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			// Return the error
			return nil, err
		}

		// Increment the waitgroup
		wg.Add(1)
		// Start a goroutine to connect to the peer
		go func() {
			// Defer the waitgroup decrement
			defer wg.Done()
			// Connect the node to peer using its peer address
			if err := kn.host.Connect(kn.ctx, *peerInfo); err != nil {
				// Log the error
				kn.logger.Error("Bootstrap connection failed", "error", err)
			} else {
				// Log the successful connection
				kn.logger.Info("Connection established with bootstrap node:", "info", *peerInfo)
			}
		}()
	}

	// Wait for all connections to fail/succeed
	wg.Wait()
	// Return the node and a nil error
	return kn, nil
}

func (s *Server) InitNewRPCServer(protocol protocol.ID) *lrpc.Client {
	s.rpcServers[protocol] = lrpc.NewServer(s.host, protocol)

	return lrpc.NewClient(s.host, protocol)
}

// setupHost is a method of Server that sets up the libp2p host for the node.
// Expects the node to already be configured with a lifecycle context and KIP config.
// Returns any error that occurs during the setup.
//
// Acquires an identity key based on the key file constructed from the KIP Config and the multiaddr
// by querying the host network interface. Configures the host stream handler for the KIP protocol
func (s *Server) setupHost() (err error) {
	// Check if the context of the node has been set
	if s.ctx == nil {
		return fmt.Errorf("lifecycle context for node not configured")
	}

	selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		if err := s.setupKadDht(h); err != nil {
			// Return the error
			return nil, fmt.Errorf("kaddht setup error: %w", err)
		}

		return s.KadDHT, nil
	})

	prvKey, err := crypto.UnmarshalSecp256k1PrivateKey(s.vault.GetNetworkPrivateKey().Bytes())
	if err != nil {
		s.logger.Error("Error unmarshalling network key", err)

		return err
	}
	// Chain the host options
	hostOptions := libp2p.ChainOptions(
		// Enable UPnP and hole punching
		selfRouting,
		libp2p.NATPortMap(),
		libp2p.EnableAutoRelay(),
		libp2p.EnableNATService(),
		libp2p.Identity(prvKey),
		libp2p.ListenAddrs(s.cfg.ListenAddresses...),
	)

	// Create a new libp2p host with the host options
	s.host, err = libp2p.New(hostOptions)
	if err != nil {
		// Return the error
		return
	}

	log.Println("Listener address", s.host.Addrs())

	return nil
}

func (s *Server) GetKramaID() id.KramaID {
	return s.id
}

func (s *Server) SetupStreamHandler(protocolID protocol.ID, handle func(network.Stream)) {
	s.host.SetStreamHandler(protocolID, handle)
}

func (s *Server) RemoveStreamHandler(protocolID protocol.ID) {
	s.host.RemoveStreamHandler(protocolID)
}

// A method of Server that sets up the Kademlia DHT for the node.
// setupKadDht is a method of Server that sets up the Kademlia DHT for the node.
// Expects the node host to already be configured with a libp2p host.
// Returns any error that occurs during the setup.
//
// Creates a new Kad DHT and bootstraps it.
func (s *Server) setupKadDht(host host.Host) (err error) {
	dhtOptions := []kdht.Option{
		kdht.Concurrency(10),
		kdht.Mode(kdht.ModeServer),
		kdht.ProtocolPrefix(s.cfg.ProtocolID),
	}

	// Create a new Kad DHT for the Server with the dht options
	s.KadDHT, err = kdht.New(s.ctx, host, dhtOptions...)
	if err != nil {
		// Return the error
		return
	}

	// Bootstrap the Kad DHT and check for errors
	if err = s.KadDHT.Bootstrap(s.ctx); err != nil {
		// Return the error
		return
	}

	return nil
}

// creates a custom gossipsub parameter set.
func pubsubGossipParam() pubsub.GossipSubParams {
	gParams := pubsub.DefaultGossipSubParams()
	gParams.Dlo = 6
	gParams.D = 8
	gParams.Dhi = 16
	gParams.HeartbeatInterval = 500 * time.Millisecond

	return gParams
}

// setupPubSub is a method of Server that sets up the PubSub router for the node.
// Expects the node to already be configured with a libp2p host.
// Returns any error that occurs during the setup.
//
// Creates a new FloodSub router and map of topic sets for the node.
func (s *Server) setupPubSub() (err error) {
	// Check if the libp2p host for the node has been set
	if s.host == nil {
		return fmt.Errorf("libp2p host for node not configured")
	}

	// Chain the PubSub options
	options := []pubsub.Option{
		pubsub.WithGossipSubParams(pubsubGossipParam()),
	}

	// Initialize an empty map of topic sets
	s.pstopics = make(map[string]*TopicSet)
	// Create a PubSub router for the Server with the pubsub options
	s.PSrouter, err = pubsub.NewGossipSub(s.ctx, s.host, options...)
	if err != nil {
		// Return the error
		return
	}

	return nil
}

// streamHandlerFunc is a method of Server that defines the behaviour of stream
// handling for streams acquired over the KIP protocol.
//
// Creates a NewPeerEvent when a new stream is acquired and posts it
// to the network for all receivers registered to receive the event.
func (s *Server) streamHandlerFunc(stream network.Stream) {
	// Log the acquisition of a new stream
	s.logger.Trace("Got a new Stream", stream.Protocol(), stream.Conn().RemotePeer())
	// Create a new read/write buffer
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	kpeer := NewKIPPeer(stream.Conn().RemotePeer(), *rw)
	buffer := make([]byte, 4096)

	byteCount, err := kpeer.rw.Reader.Read(buffer)
	if err != nil {
		if err := kpeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Hand shake failed", "error", err)
		}

		return
	}

	message := new(types.Message)

	err = polo.Depolorize(message, buffer[0:byteCount])
	if err != nil {
		if err := kpeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Hand shake failed", "error", err)
		}

		return
	}
	// Unmarshal message proto into a NewPeer message
	var msg types.HandshakeMSG
	if err := polo.Depolorize(&msg, message.Payload); err != nil {
		if err := kpeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Hand shake failed", "error", err)
		}

		return
	}

	// HashSet the KIP id of the peer based on the message
	kpeer.setID(message.Sender)

	// Register the peer to the handler working set
	if err := s.Peers.Register(kpeer); err != nil {
		if err := kpeer.sendHandshakeErrorResp(s.id, err); err != nil {
			s.logger.Error("Hand shake failed", "error", err)
		}

		return
	}

	ntq, err := s.Senatus.GetNTQ(s.id)
	if err != nil {
		s.logger.Error("Error fetching ntr", "error", err, "peer", s.id)
	}

	if err := kpeer.SendID(s.id, ntq, s.host.Addrs()); err != nil {
		s.logger.Error("Hand shake failed : Error sending id", "error", err)

		return
	}

	// Post the event to the registered receivers for NewPeerEvents
	peerEvent := utils.NewPeerEvent{PeerID: kpeer.networkID}
	// Post the event to the registered receivers for NewPeerEvents
	if err := s.mux.Post(peerEvent); err != nil {
		// Log any error that occurs
		log.Fatal(err)
	}

	s.logger.Info("[streamHandlerFunc] Peer Connected id", "id", kpeer.id)
}

// Discover is a method of Server that starts a discovery routine using a libp2p routing
// discovery mechanism that uses the Kad DHT of the node.
//
// Advertises the protocol rendezvous and discovers other peers that are advertising it.
// The peer discovery process is repeated every 3 seconds.
func (s *Server) Discover() {
	// Setup a new routing discovery using the Kademlia DHT of the node
	s.discovery = discovery.NewRoutingDiscovery(s.KadDHT)

	// Advertise the rendezvous string to the discovery service
	log.Println("Announcing ourselves...")

	_, err := s.discovery.Advertise(s.ctx, string(s.cfg.ProtocolID))
	if err != nil {
		s.logger.Error("Failed to advertise the rendezvous string to the discovery service", "error", err)
	}

	// Discover other peers that are advertising themselves
	log.Println("Discovering Peers...")

	for {
		select {
		// Sleep for 5 seconds before next discovery iteration
		case <-time.After(5 * time.Second):
		case <-s.ctx.Done():
			return
		}

		if err := s.handleDiscovery(); err != nil {
			log.Println(err)
		}
	}
}

func (s *Server) handleDiscovery() error {
	// Retrieve a channel of peer addresses from the discovery service
	peerChan, err := s.discovery.FindPeers(s.ctx, string(s.cfg.ProtocolID))
	if err != nil {
		return err
	}

	// Iterate over the channel of peer addresses
	for p := range peerChan {
		// log.Println("In peer", p)
		// Skip iteration if the peer addresses points to self
		if p.ID == s.host.ID() {
			continue
		}

		// Check if the host is already connected to the peer.
		if !s.Peers.ContainsPeer(p.ID) {
			// Attempt to connect the node host to the peer
			// log.Println("Connecting to peer", p.id)
			if err := s.host.Connect(s.ctx, p); err != nil {
				// Skip iteration if connection fails
				continue
			}

			// Setup a new stream to the peer over the MOI protocol
			stream, err := s.host.NewStream(s.ctx, p.ID, s.cfg.ProtocolID)
			if err != nil {
				// Skip iteration if stream setup fails
				continue
			}

			// Create a new read/write buffer
			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
			// Create a NewPeerEvent

			kPeer := NewKIPPeer(stream.Conn().RemotePeer(), *rw)

			ntq, err := s.Senatus.GetNTQ(s.GetKramaID())
			if err != nil {
				s.logger.Error("Error fetching ntq", "error", err, "peer", s.GetKramaID())

				continue
			}

			if err := kPeer.InitHandshake(s.GetKramaID(), ntq, s.host.Addrs()); err != nil {
				s.logger.Error("Handshake Failed", "peer", p.ID, "error", err)

				continue
			}

			// Register the peer to the handler working set
			if err := s.Peers.Register(kPeer); err != nil {
				s.logger.Error("Handshake Failed:", "peer", p.ID, "error", err)
			}

			// Post the event to the registered receivers for NewPeerEvents
			peerEvent := utils.NewPeerEvent{PeerID: kPeer.networkID}
			if err := s.mux.Post(peerEvent); err != nil {
				log.Fatal(err)
			}
			// // Post the event to the registered receivers for Syncer
			if err := s.mux.Post(utils.PeerDiscoveredEvent{ID: p.ID}); err != nil {
				log.Fatal(err)
			}
			// Log the successful connection to the peer
			s.logger.Info("[handleDiscoveryFunc] Peer Connected id", "id", kPeer.id)
		}
	}

	return nil
}

// ConnectPeer is a method of Server that connects to peer associated with a given KramaID.
func (s *Server) ConnectPeer(kramaID id.KramaID) error {
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

	// Get peer info from the peer store
	peerInfo := s.host.Peerstore().PeerInfo(peerID)

	// Check if the host is already connected to the peer
	if s.host.Network().Connectedness(peerInfo.ID) != network.Connected {
		// Attempt to connect the node host to the peer
		if err := s.host.Connect(s.ctx, peerInfo); err != nil {
			return err
		}
		// Log the successful connection to the peer
		log.Println("Connected", peerInfo.ID)
	}

	// Return a nil error
	return types.ErrConnectionExists
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
		log.Println("Disconnected", peerID)
	}

	return nil
}

func (s *Server) RegisterNewRPCService(protocol protocol.ID, serviceName string, service interface{}) error {
	// Register the service with the RPC server of the node
	return s.rpcServers[protocol].RegisterName(serviceName, service)
}

// Broadcast is a method of Server that broadcasts a given protocol buffer message to a
// given PubSub topic. Expects the node to be subscribed to that topic.
func (s *Server) Broadcast(topic string, data []byte) error {
	s.topicSetLock.Lock()
	defer s.topicSetLock.Unlock()

	// Retrieve the topic handler from the node's pubsub topicsets
	// s.topicSetLock.RLock()
	topicSet := s.pstopics[topic]
	// s.topicSetLock.RUnlock()
	// log.Println("Printing the topic", topicSet)
	if topicSet == nil {
		tophandle, err := s.PSrouter.Join(topic)
		if err != nil {
			// Return the error
			return err
		}
		//	s.topicSetLock.Lock()
		s.pstopics[topic] = &TopicSet{tophandle, nil}
	} else {
		//	s.topicSetLock.RLock()
		//	s.topicSetLock.RUnlock()
		// Attempt to publish the message to the pubsub topic
		if err := topicSet.topicHandle.Publish(s.ctx, data); err != nil {
			// Return the error
			return err
		}
	}

	// Return a nil error
	return nil
}

// Unsubscribe is a method of Server that unsubscribes the node from a given PubSub topic.
func (s *Server) Unsubscribe(topic string) error {
	// Cancel the subscription to the topic
	s.topicSetLock.Lock()
	defer s.topicSetLock.Unlock()
	// Check if topic exists
	if s.pstopics[topic] == nil {
		return nil
	}

	s.pstopics[topic].subHandle.Cancel()
	// Attempt to close the topic handler for the topic
	if err := s.pstopics[topic].topicHandle.Close(); err != nil {
		return err
	}

	delete(s.pstopics, topic)

	return nil
}

// Subscribe is a method of Server that subscribes the node to a given PubSub topic.
// Accepts the topic name to subscribe and handler function to handle messages from that subscription.
//
// Creates topic and subscription handles for the topic, wraps it in a TopicSet
// and adds it to the node's pubsub topicset. Creates an handler pipeline with the
// given handler function and starts a subscription loop that invokes the pipeline.
func (s *Server) Subscribe(ctx context.Context, topic string, handler func(msg *pubsub.Message) error) error {
	// Join pubsub topic and get a topic handle
	tophandle, err := s.PSrouter.Join(topic)
	if err != nil {
		// Return the error
		return err
	}

	// Subscribe to the topic and get a subscription handle
	subhandle, err := tophandle.Subscribe()
	if err != nil {
		// Return the error
		return err
	}

	s.topicSetLock.Lock()
	// Create a TopicSet and assign it to the node's topicset map
	s.pstopics[topic] = &TopicSet{tophandle, subhandle}
	s.topicSetLock.Unlock()

	// Define a subscription pipeline closure
	pipeline := func(msg *pubsub.Message) {
		// Call the given subscription handler
		// an error because it is being invoked as a goroutine
		if err := handler(msg); err != nil {
			return
		}
	}

	// Start a goroutine for the subscription pipeline
	go func() {
		// Start an infinite loop
		for {
			// Retrieve the next message from the subscription
			msg, err := subhandle.Next(ctx)
			if err != nil {
				s.logger.Error("Topic subscription closed", "topic", topic)

				return
			}

			// Skip handling self published messages
			peerID, err := s.id.PeerID()
			if err != nil {
				s.logger.Error("Error parsing krama peerID", err)
			}

			if msg.ReceivedFrom.String() == peerID {
				continue
			}

			// Invoke the pipeline
			go pipeline(msg)
		}
	}()

	// Return a nil message
	return nil
}

func (s *Server) GetRandomNode() peer.ID {
	routingTable := s.KadDHT.RoutingTable()
	peers := routingTable.ListPeers()

	// TODO: Improve the seed
	s1 := rand.NewSource(time.Now().UnixNano())
	reg := rand.New(s1)
	index := reg.Intn(len(peers))

	return peers[index]
}

func (s *Server) SendMessage(peerID peer.ID, msgType types.MsgType, msg interface{}) error {
	if s.Peers.ContainsPeer(peerID) {
		p := s.Peers.Peer(peerID)

		return p.Send(s.id, msgType, msg)
	}

	bytes := polo.Polorize(msg)
	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it into a slice of bytes
	m := types.Message{
		MsgType: msgType,
		Payload: bytes,
		Sender:  s.id,
	}

	stream, err := s.host.NewStream(s.ctx, peerID, s.cfg.ProtocolID)
	if err != nil {
		// Return error if stream setup fails
		return err
	}

	// Create a new read/write buffer
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	// Create a NewPeerEvent

	rawData := polo.Polorize(m)

	// Write the message bytes into the peer's io buffer
	_, err = rw.Writer.Write(rawData)
	if err != nil {
		return err
	}

	// Flush the peer's io buffer. This will push the message to the network
	return rw.Flush()
}

func (s *Server) NewStream(ctx context.Context, protocol protocol.ID, id peer.ID) (network.Stream, error) {
	return s.host.NewStream(ctx, id, protocol)
}

func (s *Server) Start() error {
	s.host.SetStreamHandler(s.cfg.ProtocolID, s.streamHandlerFunc)

	go s.Discover()

	return nil
}

func (s *Server) Stop() {
	defer s.ctxCancel()
}

func (s *Server) GetAddrs() []maddr.Multiaddr {
	return s.host.Addrs()
}

// SendHelloMessage sends a hello message to complete network using PubSub
func (s *Server) SendHelloMessage() {
	s.init.Do(func() {
		ntq, err := s.Senatus.GetNTQ(s.id)
		if err != nil {
			s.logger.Error("Error fetching NTQ", "error", err)
			panic(err)
		}

		peerInfo := types.PeerInfo{
			ID:      s.id,
			Ntq:     ntq,
			Address: utils.MultiAddrToString(s.GetAddrs()...),
		}

		signature, err := s.vault.Sign(polo.Polorize(peerInfo), mcommon.BlsBLST)
		if err != nil {
			s.logger.Error("Error signing message", "error", err)
			panic(err)
		}

		msg := types.HelloMsg{
			Info:      peerInfo,
			Signature: signature,
		}

		if err := s.Broadcast(SenatusTopic, polo.Polorize(msg)); err != nil {
			s.logger.Error("Error broadcasting hello message", "error", err)
			panic(err)
		}
	})
}

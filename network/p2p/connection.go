package p2p

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/senatus"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/common/utils"
)

const (
	MinimumBootNodeConn int   = 1
	NoOfRTTSlots        uint8 = 4
)

var (
	ErrNoBootnodes      = errors.New("no bootnodes")
	ErrMinBootnodes     = fmt.Errorf("minimum %v bootnode is required", MinimumBootNodeConn)
	ErrMinBootnodeConns = fmt.Errorf("unable to connect to at least %v bootstrap node(s)", MinimumBootNodeConn)
)

// These tags are used to protect the connections from being pruned by the libp2p connection manager
const (
	MOIStreamTag   = "moi-core-stream"
	FluxStreamTag  = "moi-flux-stream"
	AgoraStreamTag = "moi-agora-stream"
)

// ConnectionManager handles and manages all the connection-related activities within the p2p network, including
// management of inbound and outbound connections and various connection-related tasks.
type ConnectionManager struct {
	server                   *Server
	pingService              *PingService
	inboundConnCount         int64
	outboundConnCount        int64
	maxInboundConnCount      int64
	maxOutboundConnCount     int64
	outboundConnCountByRange [4]int64
	coolDownCache            *coolDownCache
}

// NewConnectionManager returns a new instance of ConnectionManager.
func NewConnectionManager(server *Server) *ConnectionManager {
	return &ConnectionManager{
		server:               server,
		pingService:          NewPingService(server.id, server.host, server.logger),
		inboundConnCount:     0,
		outboundConnCount:    0,
		maxInboundConnCount:  server.cfg.InboundConnLimit,
		maxOutboundConnCount: server.cfg.OutboundConnLimit,
		coolDownCache:        newCoolDownCache(),
	}
}

// getInboundConnCount returns the number of active inbound connections.
func (cm *ConnectionManager) getInboundConnCount() int64 {
	return atomic.LoadInt64(&cm.inboundConnCount)
}

// getOutboundConnCount returns the number of active outbound connections.
func (cm *ConnectionManager) getOutboundConnCount() int64 {
	return atomic.LoadInt64(&cm.outboundConnCount)
}

// getOutboundConnCountByRange returns the outbound connection count for the specified RTT range.
func (cm *ConnectionManager) getOutboundConnCountByRange(rttRange uint8) int64 {
	return atomic.LoadInt64(&cm.outboundConnCountByRange[rttRange])
}

// getMaxInboundConnCount returns the maximum number of inbound connections.
func (cm *ConnectionManager) getMaxInboundConnCount() int64 {
	return cm.maxInboundConnCount
}

// getMaxOutboundConnCount returns the maximum number of outbound connections.
func (cm *ConnectionManager) getMaxOutboundConnCount() int64 {
	return cm.maxOutboundConnCount
}

// getMaxOutboundConnCountByRange returns the maximum number of outbound connections
func (cm *ConnectionManager) getMaxOutboundConnCountByRange() int64 {
	return int64(math.Ceil(float64(cm.getMaxOutboundConnCount()) / float64(NoOfRTTSlots)))
}

// isInboundConnLimitReached returns true if the inbound connection count exceeds the limit.
func (cm *ConnectionManager) isInboundConnLimitReached() bool {
	return cm.getInboundConnCount() >= cm.getMaxInboundConnCount()
}

// isOutboundConnLimitReached returns true if the outbound connection count exceeds the limit.
func (cm *ConnectionManager) isOutboundConnLimitReached() bool {
	return cm.getOutboundConnCount() >= cm.getMaxOutboundConnCount()
}

// isOutboundConnLimitReachedForRange checks if the outbound connection limit is reached for the specified RTT range.
func (cm *ConnectionManager) isOutboundConnLimitReachedForRange(rttRange uint8) bool {
	return cm.getOutboundConnCountByRange(rttRange) >= cm.getMaxOutboundConnCountByRange()
}

// updateInboundConnCount increments the inbound connection count by the specified delta value.
func (cm *ConnectionManager) updateInboundConnCount(delta int64) {
	atomic.AddInt64(&cm.inboundConnCount, delta)
	cm.server.metrics.CaptureInboundConn(float64(delta))
	cm.server.metrics.CaptureTotalConn(float64(len(cm.getConns())))
}

// updateOutboundConnCount increments the outbound connection count by the specified delta value.
func (cm *ConnectionManager) updateOutboundConnCount(delta int64) {
	atomic.AddInt64(&cm.outboundConnCount, delta)
	cm.server.metrics.CaptureOutboundConn(float64(delta))
	cm.server.metrics.CaptureTotalConn(float64(len(cm.getConns())))
}

// UpdateOutboundConnCountByRange increments the outbound connection count for a specific range.
func (cm *ConnectionManager) updateOutboundConnCountByRange(rttRange uint8, delta int64) {
	atomic.AddInt64(&cm.outboundConnCountByRange[rttRange], delta)
	cm.updateOutboundConnCount(delta)
}

// updateConnCount updates the inbound and outbound connection counts by the specified delta,
// based on the given direction.
func (cm *ConnectionManager) updateConnCount(direction network.Direction, rtt int64, delta int64) {
	switch direction {
	case network.DirInbound:
		cm.updateInboundConnCount(delta)
	case network.DirOutbound:
		rttRange := cm.getConnRTTRange(rtt)
		cm.updateOutboundConnCountByRange(rttRange, delta)
	}
}

// MinimumPeersCount returns the minimum number of peers based on max inbound and outbound connection counts.
func (cm *ConnectionManager) MinimumPeersCount() uint32 {
	minimumCount := (cm.getMaxInboundConnCount() + cm.getMaxOutboundConnCount()) / 3

	return uint32(minimumCount)
}

// GetHostPeerID returns the host's peer ID
func (cm *ConnectionManager) GetHostPeerID() peer.ID {
	return cm.server.host.ID()
}

// GetBootstrapPeerIDs returns peer IDs of bootstrap peers.
func (cm *ConnectionManager) GetBootstrapPeerIDs() (map[peer.ID]bool, error) {
	addrInfo, err := peer.AddrInfosFromP2pAddrs(cm.server.cfg.BootstrapPeers...)
	if err != nil {
		return nil, errors.Wrap(err, "unable to extract addr-info from multiaddr for bootstrap nodes")
	}

	peerIDs := make(map[peer.ID]bool)

	for _, info := range addrInfo {
		peerIDs[info.ID] = true
	}

	return peerIDs, nil
}

// GetRandomPeer returns the libp2p ID of random peer from the server's kDHT routing table
func (cm *ConnectionManager) GetRandomPeer() peer.ID {
	peers := cm.server.kadDHT.RoutingTable().ListPeers()

	// TODO: Improve the seed
	s1 := rand.NewSource(time.Now().UnixNano())
	reg := rand.New(s1)
	index := reg.Intn(len(peers))

	return peers[index]
}

// getConnRTTRange returns a connection range based on the provided round-trip time (RTT).
func (cm *ConnectionManager) getConnRTTRange(rtt int64) uint8 {
	switch {
	case rtt <= 100:
		return 0
	case rtt <= 200:
		return 1
	case rtt <= 300:
		return 2
	default:
		return 3
	}
}

// protectConnection protects the connection to a peer from being pruned or disconnected.
// Tagging allows different parts of the system to manage protections without interfering with one another.
func (cm *ConnectionManager) protectConnection(peerID peer.ID, tag string) {
	cm.server.host.ConnManager().Protect(peerID, tag)
}

// unprotectConnection removes a protection placed on the connection to a peer under the specified tag.
func (cm *ConnectionManager) unprotectConnection(peerID peer.ID, tag string) {
	cm.server.host.ConnManager().Unprotect(peerID, tag)
}

// getConns returns a slice of active network connections.
func (cm *ConnectionManager) getConns() []network.Conn {
	return cm.server.host.Network().Conns()
}

// getPeers returns peer ID's of connected peers.
func (cm *ConnectionManager) getPeers() []id.KramaID {
	peers := make([]id.KramaID, 0)

	for _, peerInfo := range cm.server.Peers.getPeers() {
		peers = append(peers, peerInfo.kramaID)
	}

	return peers
}

// isConnectedToPeer checks if the host is connected to a specific peer.
func (cm *ConnectionManager) isConnectedToPeer(peerID peer.ID) bool {
	return cm.server.host.Network().Connectedness(peerID) == network.Connected
}

// connectPeer connects to a peer if not already connected.
func (cm *ConnectionManager) connectPeer(peerInfo peer.AddrInfo) error {
	// check if the host is already connected to the peer
	if cm.isConnectedToPeer(peerInfo.ID) {
		return common.ErrConnectionExists
	}

	// attempt to connect the node host to the peer
	if err := cm.server.host.Connect(cm.server.ctx, peerInfo); err != nil {
		return err
	}

	cm.server.logger.Trace("Connect peer success", "from", cm.server.id, "to", peerInfo.ID)

	return nil
}

// connectPeerByKramaID connects to a node using its KramaID.
func (cm *ConnectionManager) connectPeerByKramaID(kramaID id.KramaID) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	// get the peer info from peer store
	peerInfo, err := cm.GetPeerInfo(peerID)
	if err != nil {
		return err
	}

	err = cm.connectPeer(*peerInfo)
	if err != nil {
		return err
	}

	return nil
}

// connectToMaddr connects to a peer using a Multiaddr.
func (cm *ConnectionManager) connectToMaddr(peerAddr maddr.Multiaddr) error {
	peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
	if err != nil {
		return err
	}

	if err = cm.server.host.Connect(cm.server.ctx, *peerInfo); err != nil {
		return err
	}

	cm.protectConnection(peerInfo.ID, MOIStreamTag)

	return nil
}

// connectToBootStrapNodes Attempts connecting to all the bootstrap nodes in config
// returns error if
// --boostrap nodes in config is Nil
// --boostrap nodes count in config is Zero
// --unable to connect to at least one bootstrap node
func (cm *ConnectionManager) connectToBootStrapNodes() error {
	if cm.server.cfg.BootstrapPeers == nil {
		return ErrNoBootnodes
	}

	if len(cm.server.cfg.BootstrapPeers) < MinimumBootNodeConn {
		return ErrMinBootnodes
	}

	var bootstrapConnections int8

	for _, bootstrapPeer := range cm.server.cfg.BootstrapPeers {
		if err := cm.connectToMaddr(bootstrapPeer); err != nil {
			cm.server.logger.Error("Bootstrap connection failed", "peer", bootstrapPeer, "err", err)

			continue
		}

		cm.server.logger.Info("Connection established with bootstrap node", "peer", bootstrapPeer)

		bootstrapConnections += 1
	}

	if bootstrapConnections == 0 {
		return ErrMinBootnodeConns
	}

	return nil
}

// connectToTrustedNodes attempts to connect to and register the given list of trusted nodes.
func (cm *ConnectionManager) connectToTrustedNodes() {
	cm.server.logger.Info("Connecting to trusted nodes")

	for _, trustedPeer := range cm.server.cfg.TrustedPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(trustedPeer.Address)
		if err != nil {
			cm.server.logger.Error("Invalid trusted peer address", "err", err)

			continue
		}

		kramaID, rtt, err := cm.retrieveRTTAndRefreshSenatus(*peerInfo)
		if err != nil {
			cm.server.logger.Error("Failed to retrieve rtt and refresh senatus", "err", err)

			continue
		}

		if err := cm.ConnectAndRegisterPeer(*peerInfo, kramaID, rtt); err != nil {
			cm.server.logger.Error("Failed to establish connection with trusted peer", "err", err)
		}
	}
}

// ConnectAndRegisterPeer connects to a specific peer, establishes a stream and register's the peer to the
// handler working set.
func (cm *ConnectionManager) ConnectAndRegisterPeer(peerInfo peer.AddrInfo, kramaID id.KramaID, rtt int64) error {
	var (
		stream network.Stream
		err    error
		kPeer  *Peer
	)

	if !cm.server.cfg.NoDiscovery && cm.isOutboundConnLimitReachedForRange(cm.getConnRTTRange(rtt)) {
		return common.ErrOutboundConnLimit
	}

	if err = cm.connectPeer(peerInfo); err != nil && !errors.Is(err, common.ErrConnectionExists) {
		return err
	}

	// create a new stream to the kPeer over the MOI protocol
	stream, err = cm.NewStream(cm.server.ctx, peerInfo.ID, config.MOIProtocolStream, MOIStreamTag)
	if err != nil {
		cm.server.logger.Error("Failed to open new stream", "err", err)
		// return error if stream setup fails
		return err
	}

	kPeer = newPeer(stream, kramaID, rtt, cm.server.logger)

	if err = kPeer.InitHandshake(cm.server); err != nil {
		if !errors.Is(err, network.ErrReset) {
			cm.server.logger.Error("Handshake failed", "krama-ID", peerInfo.ID, "err", err)
		}

		return err
	}

	// Register the kPeer to the handler working set
	if err := cm.server.Peers.Register(kPeer); err != nil {
		cm.server.logger.Error("Failed to register", "krama-ID", peerInfo.ID, "err", err)

		return err
	}

	// Update the outbound connection count
	cm.updateConnCount(network.DirOutbound, rtt, 1)

	// Post the event to the registered receivers for NewPeerEvents
	if err = cm.postNewPeerEvent(kPeer.networkID); err != nil {
		cm.server.logger.Error("Failed to post new peer event", "err", err)

		return err
	}

	cm.server.logger.Info("Peer Connected", "krama-ID", kPeer.kramaID)

	// Return a nil error
	return nil
}

// NewStream creates a new network stream with the given peer ID and protocol.
func (cm *ConnectionManager) NewStream(
	ctx context.Context,
	id peer.ID,
	protocol protocol.ID,
	tag string,
) (network.Stream, error) {
	stream, err := cm.server.host.NewStream(ctx, id, protocol)
	if err != nil {
		return nil, err
	}

	// Protect the peer connection
	cm.protectConnection(stream.Conn().RemotePeer(), tag)

	return stream, nil
}

// SetupStreamHandler sets the stream handler for a given ProtocolID on the Host's Mux
func (cm *ConnectionManager) SetupStreamHandler(protocolID protocol.ID, tag string, handle func(network.Stream)) {
	cm.server.host.SetStreamHandler(protocolID, func(stream network.Stream) {
		cm.server.ConnManager.protectConnection(stream.Conn().RemotePeer(), tag)

		handle(stream)
	})
}

// RemoveStreamHandler removes a handler for a given ProtocolId on the mux that was set by setStreamHandler
func (cm *ConnectionManager) RemoveStreamHandler(protocolID protocol.ID) {
	cm.server.host.RemoveStreamHandler(protocolID)
}

// CloseStream closes a network stream, releases protection, and returns an error if closing fails.
func (cm *ConnectionManager) CloseStream(stream network.Stream, tag string) error {
	// Release peer connection protection
	cm.unprotectConnection(stream.Conn().RemotePeer(), tag)

	if err := stream.Close(); err != nil {
		return err
	}

	return nil
}

// ResetStream resets a network stream, releases protection, and returns an error if resetting fails.
func (cm *ConnectionManager) ResetStream(stream network.Stream, tag string) error {
	// Release peer connection protection
	cm.unprotectConnection(stream.Conn().RemotePeer(), tag)

	if err := stream.Reset(); err != nil {
		return err
	}

	return nil
}

// setStreamHandler starts stream handler for the ProtocolID present in the node's config
func (cm *ConnectionManager) setStreamHandler() {
	cm.SetupStreamHandler(config.MOIProtocolStream, MOIStreamTag, cm.streamHandler)
}

// streamHandler handles incoming network streams.
func (cm *ConnectionManager) streamHandler(stream network.Stream) {
	if cm.isInboundConnLimitReached() {
		if err := cm.ResetStream(stream, MOIStreamTag); err != nil {
			cm.server.logger.Error("Failed to reset stream", "err", err)
		}

		return
	}

	cm.server.logger.Trace(
		"Handling new stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	kramaID, rtt, err := cm.retrieveRTTAndRefreshSenatus(peer.AddrInfo{
		ID:    stream.Conn().RemotePeer(),
		Addrs: []maddr.Multiaddr{stream.Conn().RemoteMultiaddr()},
	})
	if err != nil {
		if err = cm.ResetStream(stream, MOIStreamTag); err != nil {
			cm.server.logger.Error("Failed to reset stream", "err", err)
		}

		return
	}

	kPeer := newPeer(stream, kramaID, rtt, cm.server.logger)

	if err := kPeer.handleHandshakeMessage(); err != nil {
		if err := kPeer.sendHandshakeErrorResp(cm.server.id, err); err != nil {
			cm.server.logger.Error("Handle handshake", "err", err)
		}

		return
	}

	// Register the kPeer to the handler working set
	if err := cm.server.Peers.Register(kPeer); err != nil {
		if err := kPeer.sendHandshakeErrorResp(cm.server.id, err); err != nil {
			cm.server.logger.Error("Failed to send register error response", kPeer.networkID)
		}

		return
	}

	// Update the inbound connection count
	cm.updateConnCount(network.DirInbound, 0, 1)

	if err := kPeer.SendHandshakeMessage(cm.server); err != nil {
		cm.server.logger.Error("Send hand shake message", "err", err)

		return
	}

	// Post the event to the registered receivers for NewPeerEvents
	if err := cm.postNewPeerEvent(kPeer.networkID); err != nil {
		cm.server.logger.Error("Stream handler function", "err", err)

		return
	}

	cm.server.logger.Info("Handled stream, connected and registered", "krama-ID", kPeer.kramaID)
}

// pingPeer pings the specified peer using the pingService and returns the KramaID, round-trip time (RTT), and
// an error if the ping operation fails.
func (cm *ConnectionManager) pingPeer(peerInfo peer.AddrInfo) (id.KramaID, int64, error) {
	response := <-cm.pingService.Ping(cm.server.ctx, peerInfo.ID)
	if response.Error != nil {
		cm.coolDownCache.Add(peerInfo.ID)
		cm.server.logger.Error("Failed to ping", "peer id", peerInfo.ID)

		return "", 0, errors.Wrap(response.Error, "Failed to ping peer")
	}

	return response.KramaID, response.RTT.Milliseconds(), nil
}

// refreshSenatus updates senatus with the latest information of a peer or adds the peer if not present already.
func (cm *ConnectionManager) refreshSenatus(peerInfo peer.AddrInfo, kramaID id.KramaID, rtt int64) error {
	err := cm.server.Senatus.AddNewPeerWithPeerID(peerInfo.ID, &senatus.NodeMetaInfo{
		Addrs:   utils.MultiAddrToString(peerInfo.Addrs...),
		NTQ:     senatus.DefaultPeerNTQ,
		KramaID: kramaID,
		RTT:     rtt,
	})
	if err != nil && !errors.Is(err, common.ErrAlreadyKnown) {
		return errors.Wrap(err, "Failed to add peer info to senatus")
	}

	return nil
}

// retrieveRTTAndRefreshSenatus retrieves and returns the krama id, round-trip time (RTT) for a given peer
// based on the provided peer information and updates senatus if required.
func (cm *ConnectionManager) retrieveRTTAndRefreshSenatus(peerInfo peer.AddrInfo) (id.KramaID, int64, error) {
	addrs, err := cm.server.Senatus.GetAddressByPeerID(peerInfo.ID)
	if err != nil && !errors.Is(err, common.ErrKramaIDNotFound) && !errors.Is(err, common.ErrAddressNotFound) {
		return "", 0, errors.Wrap(err, "failed to retrieve peer address")
	}

	if addrs != nil && compareMultiaddrs(addrs, peerInfo.Addrs) {
		kramaID, err := cm.server.Senatus.GetKramaIDByPeerID(peerInfo.ID)
		if err != nil {
			return "", 0, errors.Wrap(err, "failed to retrieve krama id")
		}

		rtt, err := cm.server.Senatus.GetRTTByPeerID(peerInfo.ID)
		if err != nil {
			return "", 0, errors.Wrap(err, "failed to retrieve rtt")
		}

		return kramaID, rtt, nil
	}

	kramaID, rtt, err := cm.pingPeer(peerInfo)
	if err != nil {
		return "", 0, err
	}

	if cm.server.cfg.RefreshSenatus {
		if err = cm.refreshSenatus(peerInfo, kramaID, rtt); err != nil {
			return "", 0, err
		}
	}

	return kramaID, rtt, nil
}

// AddToPeerStore adds peer information to the node's peer store
func (cm *ConnectionManager) AddToPeerStore(peerInfo *peer.AddrInfo) {
	cm.server.host.Peerstore().AddAddr(peerInfo.ID, peerInfo.Addrs[0], peerstore.PermanentAddrTTL)
}

// getFromPeerStore gets the peer information from the node's peer store
func (cm *ConnectionManager) getFromPeerStore(peerID peer.ID) *peer.AddrInfo {
	peerInfo := cm.server.host.Peerstore().PeerInfo(peerID)

	return &peerInfo
}

// GetAddrsFromPeerStore gets the peer address from the node's peer store
func (cm *ConnectionManager) GetAddrsFromPeerStore(peerID peer.ID) []maddr.Multiaddr {
	return cm.server.host.Peerstore().Addrs(peerID)
}

// GetPeerInfo retrieves and returns peer information from the peer store, or from senatus if not found in the store.
func (cm *ConnectionManager) GetPeerInfo(peerID peer.ID) (*peer.AddrInfo, error) {
	// get the peer info from peer store
	peerInfo := cm.getFromPeerStore(peerID)
	// retrieves peer information from senatus and adds it to the peer store if it is not present in the store.
	if peerInfo == nil || len(peerInfo.Addrs) == 0 {
		addr, err := cm.server.Senatus.GetAddressByPeerID(peerID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get peer info from senatus")
		}

		peerInfo = &peer.AddrInfo{ID: peerID, Addrs: addr}

		cm.AddToPeerStore(peerInfo)
	}

	return peerInfo, nil
}

// postNewPeerEvent sends a new peer event to the event multiplexer.
func (cm *ConnectionManager) postNewPeerEvent(peerID peer.ID) error {
	if err := cm.server.mux.Post(utils.NewPeerEvent{PeerID: peerID}); err != nil {
		return err
	}

	return nil
}

// Start initiates the connection manager.
func (cm *ConnectionManager) Start() error {
	cm.setStreamHandler()

	if cm.server.cfg.NoDiscovery {
		go cm.connectToTrustedNodes()

		return nil
	}

	if err := cm.connectToBootStrapNodes(); err != nil {
		return fmt.Errorf("bootstrap nodes connection: %w", err)
	}

	return nil
}

// helper function
func compareMultiaddrs(existingAddrs []maddr.Multiaddr, newAddrs []maddr.Multiaddr) bool {
	for i := 0; i < len(newAddrs); i++ {
		for j := 0; j < len(existingAddrs); j++ {
			if newAddrs[i].String() == existingAddrs[j].String() {
				return true
			}
		}
	}

	return false
}

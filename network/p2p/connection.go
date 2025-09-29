package p2p

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-msgio"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/senatus"
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
	ICSMeshTag     = "moi-ics-mesh-stream"
	ICSDirectTag   = "moi-ics-direct-stream"
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

// ProtectConnection protects the connection to a peer from being pruned or disconnected.
// Tagging allows different parts of the system to manage protections without interfering with one another.
func (cm *ConnectionManager) ProtectConnection(peerID peer.ID, tag string) {
	cm.server.host.ConnManager().Protect(peerID, tag)
}

func (cm *ConnectionManager) IsConnectionProtected(peerID peer.ID, tag string) bool {
	return cm.server.host.ConnManager().IsProtected(peerID, tag)
}

func (cm *ConnectionManager) GetConnInfo(peerID peer.ID) *connmgr.TagInfo {
	return cm.server.host.ConnManager().GetTagInfo(peerID)
}

// UnprotectConnection removes a protection placed on the connection to a peer under the specified tag.
func (cm *ConnectionManager) UnprotectConnection(peerID peer.ID, tag string) {
	cm.server.host.ConnManager().Unprotect(peerID, tag)
}

// getConns returns a slice of active network connections.
func (cm *ConnectionManager) getConns() []network.Conn {
	return cm.server.host.Network().Conns()
}

// getPeers returns peer ID's of connected peers.
func (cm *ConnectionManager) getPeers() []identifiers.KramaID {
	peers := make([]identifiers.KramaID, 0)

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
func (cm *ConnectionManager) connectPeer(ctx context.Context, peerInfo peer.AddrInfo) error {
	// check if the host is already connected to the peer
	if cm.isConnectedToPeer(peerInfo.ID) {
		return common.ErrConnectionExists
	}

	// attempt to connect the node host to the peer
	if err := cm.server.host.Connect(ctx, peerInfo); err != nil {
		return err
	}

	cm.server.logger.Trace("Connect peer success", "from", cm.server.id, "to", peerInfo.ID)

	return nil
}

// ConnectPeerByKramaID connects to a node using its KramaID.
func (cm *ConnectionManager) ConnectPeerByKramaID(ctx context.Context, kramaID identifiers.KramaID) error {
	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	// check if the host is already connected to the peer
	if cm.isConnectedToPeer(peerID) {
		return common.ErrConnectionExists
	}

	// get the peer info from peer store
	peerInfo, err := cm.GetPeerInfo(peerID)
	if err == nil {
		return cm.connectPeer(ctx, *peerInfo)
	}

	// If peer info is not available in senatus discover the peer info using kdht
	if postErr := cm.server.mux.Post(DiscoverPeerEvent{ID: peerID}); postErr != nil {
		return postErr
	}

	return err
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

	cm.ProtectConnection(peerInfo.ID, MOIStreamTag)

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

	retryCount := 3

	for _, bootstrapPeer := range cm.server.cfg.BootstrapPeers {
		for i := 0; i < retryCount; i++ {
			if err := cm.connectToMaddr(bootstrapPeer); err != nil {
				cm.server.logger.Error("Bootstrap connection failed", "peer", bootstrapPeer, "err", err)

				time.Sleep(5 * time.Second)

				continue
			}

			cm.server.logger.Info("Connection established with bootstrap node", "peer", bootstrapPeer)

			bootstrapConnections += 1

			break
		}
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
		peerAddrInfo, err := peer.AddrInfoFromP2pAddr(trustedPeer.Address)
		if err != nil {
			cm.server.logger.Error("Invalid trusted peer address", "err", err)

			continue
		}

		kramaID, rtt, err := cm.retrieveRTTAndRefreshSenatus(*peerAddrInfo)
		if err != nil {
			cm.server.logger.Error("Failed to retrieve rtt and refresh senatus", "err", err)

			continue
		}

		if err = cm.ConnectAndRegisterPeer(
			cm.server.ctx,
			PeerInfo{IsInboundPeer: false, AddrInfo: *peerAddrInfo}, kramaID, rtt); err != nil {
			cm.server.logger.Error("Failed to establish connection with trusted peer", "err", err)
		}
	}
}

// ConnectAndRegisterPeer connects to a specific peer, establishes a stream and registers the peer to the
// handler working set.
func (cm *ConnectionManager) ConnectAndRegisterPeer(
	ctx context.Context,
	peerInfo PeerInfo,
	kramaID identifiers.KramaID,
	rtt int64,
) error {
	var (
		stream network.Stream
		err    error
		kPeer  *Peer
	)

	timedCtx, cancel := context.WithTimeout(ctx, connectionTimeout)
	defer cancel()

	if !peerInfo.IsInboundPeer && (cm.isOutboundConnLimitReachedForRange(cm.getConnRTTRange(rtt))) {
		return common.ErrOutboundConnLimit
	}

	if err = cm.connectPeer(timedCtx, peerInfo.AddrInfo); err != nil && !errors.Is(err, common.ErrConnectionExists) {
		return err
	}

	// create a new stream to the kPeer over the MOI protocol
	stream, err = cm.NewStream(timedCtx, peerInfo.AddrInfo.ID, config.MOIProtocolStream, MOIStreamTag)
	if err != nil {
		cm.server.logger.Error("Failed to open new stream", "err", err)
		// return error if stream setup fails
		return err
	}

	kPeer = newPeer(stream, kramaID, rtt, cm.server.logger)

	if err = kPeer.InitHandshake(cm.server); err != nil {
		if !errors.Is(err, network.ErrReset) {
			cm.server.logger.Error("Handshake failed", "krama-id", peerInfo.AddrInfo.ID, "err", err)
		}

		_ = cm.ResetStream(stream, MOIStreamTag)

		return err
	}

	// Register the kPeer to the handler working set
	if err = cm.server.Peers.Register(kPeer); err != nil {
		cm.server.logger.Error("Failed to register", "krama-id", peerInfo.AddrInfo.ID, "err", err)

		_ = cm.ResetStream(stream, MOIStreamTag)

		return err
	}

	cm.server.logger.Info("Peer Connected", "krama-id", kPeer.kramaID)

	// Update the outbound connection count
	cm.updateConnCount(network.DirOutbound, rtt, 1)

	go cm.handleOutboundPeer(kPeer)

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
	if tag != "" {
		cm.ProtectConnection(stream.Conn().RemotePeer(), tag)
	}

	return stream, nil
}

// SetupStreamHandler sets the stream handler for a given ProtocolID on the Host's Mux
func (cm *ConnectionManager) SetupStreamHandler(protocolID protocol.ID, tag string, handle func(network.Stream)) {
	cm.server.host.SetStreamHandler(protocolID, func(stream network.Stream) {
		if tag != "" {
			cm.server.ConnManager.ProtectConnection(stream.Conn().RemotePeer(), tag)
		}

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
	cm.UnprotectConnection(stream.Conn().RemotePeer(), tag)

	if err := stream.Close(); err != nil {
		return err
	}

	return nil
}

// ResetStream resets a network stream, releases protection, and returns an error if resetting fails.
func (cm *ConnectionManager) ResetStream(stream network.Stream, tag string) error {
	// Release peer connection protection
	cm.UnprotectConnection(stream.Conn().RemotePeer(), tag)

	if err := stream.Reset(); err != nil {
		return err
	}

	return nil
}

// setStreamHandler starts stream handler for the ProtocolID present in the node's config
func (cm *ConnectionManager) setStreamHandler() {
	cm.SetupStreamHandler(config.MOIProtocolStream, "", cm.streamHandler)
}

// streamHandler handles incoming network streams.
func (cm *ConnectionManager) streamHandler(stream network.Stream) {
	peerID := stream.Conn().RemotePeer()

	cm.server.inboundStreamsMx.Lock()

	other, dup := cm.server.inboundStreams[peerID]
	if dup {
		cm.server.logger.Debug("duplicate inbound stream from %s; resetting other stream", peerID)
		other.Reset()
	}

	if !dup && cm.isInboundConnLimitReached() {
		if err := cm.ResetStream(stream, ""); err != nil {
			cm.server.logger.Error("Failed to reset stream", "err", err)
		}

		cm.server.inboundStreamsMx.Unlock()

		return
	}
	// Update the inbound connection count
	cm.updateConnCount(network.DirInbound, 0, 1)

	cm.server.inboundStreams[peerID] = stream
	cm.server.inboundStreamsMx.Unlock()

	defer func() {
		cm.server.inboundStreamsMx.Lock()
		if cm.server.inboundStreams[peerID] == stream {
			delete(cm.server.inboundStreams, peerID)
		}
		cm.server.inboundStreamsMx.Unlock()
	}()

	cm.server.logger.Trace(
		"Handling new stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	kPeer := newPeer(stream, "", 0, cm.server.logger)

	if err := kPeer.handleHandshakeMessage(); err != nil {
		cm.server.logger.Error("Failed to handle handshake msg", "err", err)

		if err = cm.ResetStream(stream, ""); err != nil {
			cm.server.logger.Error("Failed to reset stream", "err", err)
		}

		return
	}

	if err := kPeer.SendHandshakeMessage(cm.server); err != nil {
		cm.server.logger.Error("Send hand shake message", "err", err)

		return
	}

	addrInfo := cm.server.host.Peerstore().PeerInfo(stream.Conn().RemotePeer())

	cm.server.ds.peerChan <- PeerInfo{IsInboundPeer: true, AddrInfo: addrInfo}

	cm.server.logger.Info("Handled stream, connected and registered", "krama-id", kPeer.kramaID)

	cm.handleInboundPeer(kPeer)
}

// pingPeer pings the specified peer using the pingService and returns the KramaID, round-trip time (RTT), and
// an error if the ping operation fails.
func (cm *ConnectionManager) pingPeer(peerInfo peer.AddrInfo) (identifiers.KramaID, int64, error) {
	response := <-cm.pingService.Ping(cm.server.ctx, peerInfo.ID)
	if response.Error != nil {
		cm.coolDownCache.Add(peerInfo.ID)
		cm.server.logger.Error("Failed to ping", "peer id", peerInfo.ID)

		return "", 0, errors.Wrap(response.Error, "Failed to ping peer")
	}

	return response.KramaID, response.RTT.Milliseconds(), nil
}

// refreshSenatus updates senatus with the latest information of a peer or adds the peer if not present already.
func (cm *ConnectionManager) refreshSenatus(peerInfo peer.AddrInfo, kramaID identifiers.KramaID, rtt int64) error {
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
func (cm *ConnectionManager) retrieveRTTAndRefreshSenatus(peerInfo peer.AddrInfo) (identifiers.KramaID, int64, error) {
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

// handleInboundPeer handles inbound peer connection and messages.
func (cm *ConnectionManager) handleInboundPeer(peer *Peer) {
	defer func() {
		if err := cm.ResetStream(peer.stream, ""); err != nil {
			cm.server.logger.Trace("Failed to reset connection", "err", err)
		}

		// Update inbound connection count based on the peer stream's direction
		cm.updateConnCount(network.DirInbound, 0, -1)

		cm.server.logger.Info("Inbound Peer Disconnected", "krama-id", peer.kramaID)
	}()

	// Handle messages from the peer
	for {
		if err := cm.server.handlePeerMessage(peer); err != nil {
			cm.server.logger.Error("Error handling inbound peer message", "err", err)

			return
		}
	}
}

// handleOutboundPeer handles the outbound peer connection.
func (cm *ConnectionManager) handleOutboundPeer(peer *Peer) {
	// Defer the peer unregister from the handler working set
	defer func() {
		// Update outbound connection count based on the peer stream's direction
		cm.updateConnCount(network.DirOutbound, peer.GetRTT(), -1)

		if err := cm.server.Peers.Unregister(peer); err != nil {
			cm.server.logger.Error("Error unregistering peer", "err", err)

			return
		}

		if err := cm.ResetStream(peer.stream, MOIStreamTag); err != nil {
			cm.server.logger.Trace("Failed to reset connection", "err", err)
		}

		cm.server.logger.Info("Outbound Peer Disconnected", "krama-id", peer.kramaID)
	}()

	// Handle messages from the peer
	for {
		reader := msgio.NewReader(peer.rw.Reader)

		_, err := reader.ReadMsg()
		if err != nil {
			cm.server.logger.Error("Error handling outbound peer message", "err", err)

			return
		}
	}
}

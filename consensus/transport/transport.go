package transport

import (
	"context"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/network/p2p"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/libp2p/go-msgio"
	"github.com/moby/locker"

	"github.com/sarvalabs/go-moi/telemetry/tracing"

	"github.com/hashicorp/go-hclog"

	p2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
	id "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
)

const (
	GossipInterval         = 200 * time.Millisecond
	ClusterCleanupInterval = 2000 * time.Millisecond
	ConnectionTimeout      = 500 * time.Millisecond
)

// Network defines the interface for network-related operations.
type Network interface {
	Broadcast(topicName string, data []byte) error
}

// ConnectionManager defines the interface for managing peer connections.
type ConnectionManager interface {
	ConnectPeerByKramaID(ctx context.Context, kramaID id.KramaID) error
	SetupStreamHandler(protocolID protocol.ID, tag string, handle func(p2pnet.Stream))
	NewStream(ctx context.Context, id peer.ID, protocol protocol.ID, tag string) (p2pnet.Stream, error)
	GetConnInfo(peerID peer.ID) *connmgr.TagInfo
	IsConnectionProtected(peerID peer.ID, tag string) bool
	CloseStream(stream p2pnet.Stream, tag string) error
	ResetStream(stream p2pnet.Stream, tag string) error
	UnprotectConnection(peerID peer.ID, tag string)
}

// contextRouters represents a collection of ContextRouter instances.
type contextRouters struct {
	routers map[common.ClusterID]*ContextRouter
	mtx     sync.RWMutex
}

// newContextRouters returns a new contextRouters instance.
func newContextRouters() *contextRouters {
	return &contextRouters{
		routers: make(map[common.ClusterID]*ContextRouter),
		mtx:     sync.RWMutex{},
	}
}

// list returns a collection of all the context routers.
func (crs *contextRouters) list() map[common.ClusterID]*ContextRouter {
	crs.mtx.RLock()
	defer crs.mtx.RUnlock()

	routers := make(map[common.ClusterID]*ContextRouter, len(crs.routers))
	for clusterID, router := range crs.routers {
		routers[clusterID] = router
	}

	return routers
}

// has checks if a ContextRouter exists for the given cluster ID.
func (crs *contextRouters) has(clusterID common.ClusterID) bool {
	crs.mtx.RLock()
	defer crs.mtx.RUnlock()

	_, ok := crs.routers[clusterID]

	return ok
}

// get returns the ContextRouter associated with the given cluster ID.
func (crs *contextRouters) get(clusterID common.ClusterID) *ContextRouter {
	crs.mtx.RLock()
	defer crs.mtx.RUnlock()

	return crs.routers[clusterID]
}

// add associates a ContextRouter with the provided cluster ID.
func (crs *contextRouters) add(clusterID common.ClusterID, router *ContextRouter) {
	crs.mtx.Lock()
	defer crs.mtx.Unlock()

	crs.routers[clusterID] = router
}

// remove disassociates the ContextRouter associated with the given cluster ID.
func (crs *contextRouters) remove(clusterID common.ClusterID) {
	crs.mtx.Lock()
	defer crs.mtx.Unlock()

	delete(crs.routers, clusterID)
}

type msg struct {
	msgType message.MsgType
	kramaID id.KramaID
	msg     []byte
}

type closePeer struct {
	kramaID      id.KramaID
	isDirectPeer bool
}

// KramaTransport represents the ICS message transport layer.
type KramaTransport struct {
	ctx                            context.Context
	ctxCancel                      context.CancelFunc
	selfID                         id.KramaID
	logger                         hclog.Logger
	metrics                        *Metrics
	network                        Network
	connManager                    ConnectionManager
	msgsToEngine                   chan *types.ICSMSG
	msgsChan                       chan msg
	closePeerChan                  chan closePeer
	directPeerLock                 *locker.Locker
	meshPeerLock                   *locker.Locker
	directPeerset                  *icsPeerSet
	meshPeerset                    *icsPeerSet
	transitPeers                   *transitPeers
	contextRouters                 *contextRouters
	requestCache                   *RequestCache
	minGossipPeers, maxGossipPeers int
	directInboundStreams           map[peer.ID]p2pnet.Stream
	directInboundStreamsLock       sync.Mutex
}

// NewKramaTransport creates a new instance of KramaTransport.
func NewKramaTransport(
	selfID id.KramaID,
	logger hclog.Logger,
	metrics *Metrics,
	network Network,
	connManager ConnectionManager,
	minGossipPeers, maxGossipPeers int,
) *KramaTransport {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &KramaTransport{
		ctx:                  ctx,
		ctxCancel:            ctxCancel,
		selfID:               selfID,
		logger:               logger.Named("Krama-Transport"),
		metrics:              metrics,
		network:              network,
		connManager:          connManager,
		msgsToEngine:         make(chan *types.ICSMSG),
		msgsChan:             make(chan msg),
		closePeerChan:        make(chan closePeer),
		directPeerLock:       locker.New(),
		meshPeerLock:         locker.New(),
		directPeerset:        newICSPeerSet(),
		meshPeerset:          newICSPeerSet(),
		transitPeers:         newTransitPeers(),
		contextRouters:       newContextRouters(),
		requestCache:         newRequestCache(),
		minGossipPeers:       minGossipPeers,
		maxGossipPeers:       maxGossipPeers,
		directInboundStreams: make(map[peer.ID]p2pnet.Stream),
	}
}

// setupStreamHandlers configures the KramaTransport to handle the incoming streams.
func (kt *KramaTransport) setupStreamHandlers() {
	kt.connManager.SetupStreamHandler(config.ICSProtocolDirectStream, p2p.ICSDirectTag, kt.handleDirectStream)
	kt.connManager.SetupStreamHandler(config.ICSProtocolMeshStream, p2p.ICSMeshTag, kt.handleMeshStream)
}

// Messages returns a channel for receiving messages from the KramaTransport.
func (kt *KramaTransport) Messages() <-chan *types.ICSMSG {
	return kt.msgsToEngine
}

func (kt *KramaTransport) ConnectToDirectPeer(
	ctx context.Context,
	kramaID id.KramaID,
	clusterID common.ClusterID,
) error {
	if kramaID == kt.selfID {
		return nil
	}

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	_, span := tracing.Span(
		ctx,
		"Krama.KramaTransport",
		"ConnectToDirectPeer",
		trace.WithAttributes(attribute.String("peerID", peerID.String())),
	)
	defer span.End()

	kt.directPeerLock.Lock(peerID.String())
	defer func() {
		if err = kt.directPeerLock.Unlock(peerID.String()); err != nil {
			kt.logger.Error("Failed to release peer lock", err)
		}
	}()

	if kPeer := kt.directPeerset.Peer(kramaID); kPeer != nil {
		kPeer.clusters.add(clusterID)

		return nil
	}

	timedCtx, cancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer cancel()

	err = kt.connManager.ConnectPeerByKramaID(timedCtx, kramaID)
	if err != nil && !errors.Is(err, common.ErrConnectionExists) {
		return err
	}

	stream, err := kt.connManager.NewStream(kt.ctx, peerID, config.ICSProtocolDirectStream, p2p.ICSDirectTag)
	if err != nil {
		return err
	}

	kt.logger.Debug("Opening new stream to direct peer", "peer-id", peerID, "stream-id", stream.ID())

	kPeer := newICSPeer(kt.ctx, kramaID, stream, kt.logger, true)

	kPeer.clusters.add(clusterID)

	if err = kt.directPeerset.Register(kPeer); err != nil {
		return err
	}

	kt.metrics.captureActiveDirectPeers(1)

	go kt.directPeerRoutine(kPeer)
	go kt.handleDeadDirectPeer(kPeer)

	return nil
}

func (kt *KramaTransport) meshPeerRoutine(p *icsPeer) {
	defer func() {
		_ = kt.connManager.CloseStream(p.stream, p2p.ICSMeshTag)
		kt.metrics.captureActiveMeshPeers(-1)
	}()

	for {
		select {
		case <-p.ctx.Done():
			return

		case msg, ok := <-p.msgChan:
			if !ok {
				p.logger.Trace("Mesh peer closed", "peer-id", p.networkID)

				return
			}

			if err := shipMessage(&p.rw, msg); err != nil {
				p.logger.Trace("Failed to ship message", "err", err)
			}
		}
	}
}

func (kt *KramaTransport) directPeerRoutine(p *icsPeer) {
	defer func() {
		err := kt.connManager.CloseStream(p.stream, p2p.ICSDirectTag)
		if err != nil {
			kt.logger.Debug("Failed to close stream", "peer-id", p.stream.Conn().RemotePeer())
		}

		kt.metrics.captureActiveDirectPeers(-1)
	}()

	for {
		select {
		case <-p.ctx.Done():
			return

		case msg, ok := <-p.msgChan:
			if !ok {
				p.logger.Trace("Direct peer closed", "peer-id", p.networkID)

				return
			}

			if err := shipMessage(&p.rw, msg); err != nil {
				p.logger.Trace("Failed to ship message", "err", err)
			}
		}
	}
}

// ConnectToMeshPeer establishes a connection to a peer and registers the peer
func (kt *KramaTransport) ConnectToMeshPeer(
	ctx context.Context,
	kramaID id.KramaID,
	clusterID common.ClusterID,
) error {
	if kramaID == kt.selfID {
		return nil
	}

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	_, span := tracing.Span(
		ctx,
		"Krama.KramaTransport",
		"ConnectToMeshPeer",
		trace.WithAttributes(attribute.String("peerID", peerID.String())),
	)

	kt.meshPeerLock.Lock(peerID.String())
	defer func() {
		if err = kt.meshPeerLock.Unlock(peerID.String()); err != nil {
			kt.logger.Error("Failed to release peer lock", err)
		}

		span.End()
	}()

	cr := kt.contextRouters.get(clusterID)
	if cr == nil {
		return errors.New("Context router not found")
	}

	// If the peer is already connected, add the cluster to mesh peer and update connection status for gossip peer.
	if kPeer := kt.meshPeerset.Peer(kramaID); kPeer != nil {
		kPeer.clusters.add(clusterID)

		kt.manageGossipPeerConn(cr, kramaID)

		if err = kt.sendICSGraft(clusterID, kPeer); err != nil {
			return err
		}

		return nil
	}

	timedCtx, cancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer cancel()

	err = kt.connManager.ConnectPeerByKramaID(timedCtx, kramaID)
	if err != nil && !errors.Is(err, common.ErrConnectionExists) {
		return err
	}

	stream, err := kt.connManager.NewStream(kt.ctx, peerID, config.ICSProtocolMeshStream, p2p.ICSMeshTag)
	if err != nil {
		return err
	}

	kPeer := newICSPeer(kt.ctx, kramaID, stream, kt.logger, false)

	kPeer.clusters.add(clusterID)

	if err = kt.meshPeerset.Register(kPeer); err != nil {
		return err
	}

	go kt.meshPeerRoutine(kPeer)

	kt.metrics.captureActiveMeshPeers(1)

	kt.manageGossipPeerConn(cr, kramaID)

	if err = kt.sendICSGraft(clusterID, kPeer); err != nil {
		return err
	}

	cr.sendPendingMessages(clusterID, kramaID)

	go kt.handleDeadMeshPeer(kPeer)

	return nil
}

func (kt *KramaTransport) disconnectPeer(kPeer *icsPeer) {
	kt.logger.Trace(
		"Disconnecting peer",
		"peer-id", kPeer.networkID,
		"direction", kPeer.stream.Stat().Direction,
		"isDirectPeer", kPeer.isDirectPeer,
	)

	kt.closePeerChan <- closePeer{kramaID: kPeer.kramaID, isDirectPeer: kPeer.isDirectPeer}
}

// handleDeadMeshPeer handles dead peers, removes them from the routers and unregisters them from the meshPeerset.
func (kt *KramaTransport) handleDeadMeshPeer(kPeer *icsPeer) {
	defer func() {
		// check if the peer exists
		if p := kt.meshPeerset.Peer(kPeer.kramaID); p == nil {
			return
		}

		kt.meshPeerLock.Lock(kPeer.networkID.String())
		defer func() {
			if err := kt.meshPeerLock.Unlock(kPeer.networkID.String()); err != nil {
				kt.logger.Error("Failed to release peer lock", "peer id", kPeer.networkID, "err", err)
			}
		}()

		kt.logger.Trace("Unregistering dead mesh peer", "peer-id", kPeer.kramaID)

		kt.removePeerFromRouters(kPeer)
		kt.disconnectPeer(kPeer)
		kt.metrics.captureActiveMeshPeers(-1)
	}()

	reader := msgio.NewReader(kPeer.rw.Reader)

	_, err := reader.ReadMsg()
	if err != nil {
		return
	}
}

// handleDeadDirectPeer handles dead peers, removes them from the routers and unregisters them from the directPeerSet.
func (kt *KramaTransport) handleDeadDirectPeer(kPeer *icsPeer) {
	defer func() {
		// check if the peer exists
		if p := kt.directPeerset.Peer(kPeer.kramaID); p == nil {
			return
		}

		kt.directPeerLock.Lock(kPeer.networkID.String())
		defer func() {
			if err := kt.directPeerLock.Unlock(kPeer.networkID.String()); err != nil {
				kt.logger.Error("Failed to release peer lock", "peer id", kPeer.networkID, "err", err)
			}
		}()

		kt.logger.Trace("Unregistering dead direct peer", "peer-id", kPeer.kramaID)
		kt.disconnectPeer(kPeer)
	}()

	reader := msgio.NewReader(kPeer.rw.Reader)

	_, err := reader.ReadMsg()
	if err != nil {
		return
	}
}

// InitClusterConnection initiates connection to transit and random peers.
func (kt *KramaTransport) InitClusterConnection(ctx context.Context, clusterID common.ClusterID) {
	spanCtx, span := tracing.Span(ctx, "Krama.KramaTransport", "InitClusterConnection")
	defer span.End()

	cr := kt.contextRouters.get(clusterID)

	go cr.connectToGossipPeers(spanCtx)
}

func (kt *KramaTransport) peerLifeCycle() {
	for {
		select {
		case <-kt.ctx.Done():
			return
		case m := <-kt.msgsChan:
			peerSet := kt.getPeersetByMsgType(m.msgType)

			if p := peerSet.Peer(m.kramaID); p != nil {
				p.msgChan <- m.msg

				continue
			}

			kt.logger.Trace("Failed to send message to peer", "peer-id", m.kramaID)

		case m := <-kt.closePeerChan:
			if m.isDirectPeer {
				if p := kt.directPeerset.Peer(m.kramaID); p != nil {
					close(p.msgChan)
					kt.directPeerset.Unregister(m.kramaID)

					continue
				}

				kt.logger.Trace(
					"Failed to close direct peer",
					"peer-id", m.kramaID,
					"error", common.ErrPeerNotAvailable,
				)

				continue
			}

			if p := kt.meshPeerset.Peer(m.kramaID); p != nil {
				close(p.msgChan)

				kt.meshPeerset.Unregister(m.kramaID)

				continue
			}

			kt.logger.Trace("Failed to close mesh peer", "peer-id", m.kramaID)
		}
	}
}

func (kt *KramaTransport) handleDirectStream(stream p2pnet.Stream) {
	kt.logger.Trace(
		"Handling stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	peerID := stream.Conn().RemotePeer()

	kt.directInboundStreamsLock.Lock()

	s, ok := kt.directInboundStreams[peerID]
	if ok {
		kt.logger.Trace("Closing the existing direct stream", "peer-id", peerID)
		s.Reset()
	}

	kPeer := newICSPeer(kt.ctx, "", stream, kt.logger, true)

	kt.directInboundStreams[peerID] = stream
	kt.directInboundStreamsLock.Unlock()

	defer func() {
		kt.connManager.ResetStream(kPeer.stream, p2p.ICSDirectTag)
		kt.directInboundStreamsLock.Lock()
		delete(kt.directInboundStreams, peerID)
		kt.directInboundStreamsLock.Unlock()
	}()

	kt.initMessageHandler(kPeer)
}

func (kt *KramaTransport) handleMeshStream(stream p2pnet.Stream) {
	kt.logger.Trace(
		"Handling stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	kPeer := newICSPeer(kt.ctx, "", stream, kt.logger, false)

	go func(peer *icsPeer) {
		defer func() {
			_ = kt.connManager.CloseStream(peer.stream, p2p.ICSMeshTag)
		}()

		kt.initMessageHandler(peer)
	}(kPeer)
}

// getTransitPeers returns the transit peers associated with the given cluster ID.
func (kt *KramaTransport) getTransitPeers(clusterID common.ClusterID) []id.KramaID {
	if list := kt.transitPeers.get(clusterID); list != nil {
		return list.getPeers()
	}

	return nil
}

// removeTransitPeers removes the transit peers associated with the given cluster ID.
func (kt *KramaTransport) removeTransitPeers(clusterID common.ClusterID) {
	kt.transitPeers.remove(clusterID)
}

// RegisterContextRouter creates and registers a ContextRouter for a given cluster ID.
func (kt *KramaTransport) RegisterContextRouter(
	ctx context.Context,
	operator id.KramaID,
	clusterID common.ClusterID,
	nodeset *common.ICSNodeSet,
	voteset *kbft.HeightVoteSet,
) {
	contextRouter := NewContextRouter(
		ctx,
		kt.selfID,
		operator,
		clusterID,
		kt.logger.With("cluster-id", clusterID),
		nodeset,
		voteset,
		kt,
	)

	kt.contextRouters.add(clusterID, contextRouter)
	kt.metrics.captureActiveRouters(1)
}

// GracefullyCloseContextRouter sets the expiry for the ContextRouter associated with the given cluster ID.
func (kt *KramaTransport) GracefullyCloseContextRouter(clusterID common.ClusterID) {
	if cr := kt.contextRouters.get(clusterID); cr != nil {
		cr.setExpiryTime()
	}
}

// DeregisterContextRouter deregisters a ContextRouter based on the given cluster ID.
func (kt *KramaTransport) DeregisterContextRouter(clusterID common.ClusterID) {
	contextRouter := kt.contextRouters.get(clusterID)
	if contextRouter == nil {
		kt.logger.Error("Context router not found", "cluster-id", clusterID)

		return
	}

	kt.CleanDirectPeer(clusterID, kt.directPeerset.List()...)

	kt.cleanMeshPeers(contextRouter)
	contextRouter.close()
	kt.contextRouters.remove(clusterID)
	kt.metrics.captureActiveRouters(-1)

	kt.logger.Trace("Context router de-registered", "cluster-id", clusterID)
}

func (kt *KramaTransport) CleanDirectPeer(clusterID common.ClusterID, peers ...id.KramaID) {
	for _, kramaID := range peers {
		func(kramaID id.KramaID) {
			kPeer := kt.directPeerset.Peer(kramaID)
			if kPeer == nil {
				return
			}

			kt.directPeerLock.Lock(kPeer.networkID.String())
			defer func() {
				if err := kt.directPeerLock.Unlock(kPeer.networkID.String()); err != nil {
					kt.logger.Error("Failed to release peer lock", "peer id", kPeer.networkID, "err", err)
				}
			}()

			if !kPeer.clusters.has(clusterID) {
				return
			}

			kPeer.clusters.remove(clusterID)

			if kPeer.clusters.len() == 0 {
				kt.disconnectPeer(kPeer)
			}
		}(kramaID)
	}
}

func (kt *KramaTransport) cleanMeshPeers(cr *ContextRouter) {
	for kramaID := range cr.gossipPeers.entries() {
		func(peerID id.KramaID) {
			kPeer := kt.meshPeerset.Peer(kramaID)
			if kPeer == nil {
				return
			}

			kt.meshPeerLock.Lock(kPeer.networkID.String())
			defer func() {
				if err := kt.meshPeerLock.Unlock(kPeer.networkID.String()); err != nil {
					kt.logger.Error("Failed to release peer lock", "peer id", kPeer.networkID, "err", err)
				}
			}()

			if !kPeer.clusters.has(cr.clusterID) {
				return
			}

			kPeer.clusters.remove(cr.clusterID)

			if kPeer.clusters.len() == 0 {
				kt.disconnectPeer(kPeer)
			}
		}(kramaID)
	}
}

func (kt *KramaTransport) getPeersetByMsgType(msgType message.MsgType) *icsPeerSet {
	if msgType == message.ICSGRAFT || msgType == message.ICSHAVE || msgType == message.ICSWANT {
		return kt.meshPeerset
	}

	return kt.directPeerset
}

// SendMessage sends a message to a specific peer identified by peerID.
func (kt *KramaTransport) SendMessage(
	peerID id.KramaID,
	sender id.KramaID,
	clusterID common.ClusterID,
	msgType message.MsgType,
	rawMsg types.ICSPayload,
) error {
	if peerID == sender {
		return nil
	}

	wireMsg, err := GenerateWireMessage(sender, clusterID, msgType, rawMsg)
	if err != nil {
		return err
	}

	select {
	case <-kt.ctx.Done():
		return kt.ctx.Err()
	case kt.msgsChan <- msg{kramaID: peerID, msg: wireMsg, msgType: msgType}:
	}

	return nil
}

// BroadcastTesseract broadcasts a Tesseract to peers that are subscribed to the tesseract topic.
func (kt *KramaTransport) BroadcastTesseract(msg *message.TesseractMsg) error {
	rawData, err := msg.Bytes()
	if err != nil {
		return err
	}

	return kt.network.Broadcast(config.TesseractTopic, rawData)
}

// BroadcastMessage broadcasts an ics message to all peers in the specific cluster.
func (kt *KramaTransport) BroadcastMessage(
	ctx context.Context,
	icsmsg *types.ICSMSG,
) {
	_, span := tracing.Span(
		ctx,
		"Krama.KramaTransport",
		"BroadcastMessage",
		trace.WithAttributes(attribute.String("peer-id", string(icsmsg.Sender))))
	defer span.End()

	cr := kt.contextRouters.get(icsmsg.ClusterID)
	if cr == nil {
		kt.logger.Error("Failed to broadcast, context router not found", "cluster-id", icsmsg.ClusterID)

		return
	}

	rawMsg, err := icsmsg.Bytes()
	if err != nil {
		kt.logger.Error("Failed to generate wire message", "error", err)

		return
	}

	for _, peerID := range cr.getBroadcastPeers(icsmsg.MsgType) {
		// Prevent forwarding the message to its original sender or the peer who forwarded it.
		if icsmsg.Sender == peerID || icsmsg.ReceivedFrom == peerID {
			continue
		}

		if icsmsg.MsgType == message.ICSHAVE {
			gossipPeer := cr.gossipPeers.get(peerID)
			if gossipPeer == nil {
				continue
			}

			if !gossipPeer.isConnected() {
				if icsHave, ok := icsmsg.DecodedMsg.(*types.ICSHave); ok {
					gossipPeer.pendingVotes.add(icsHave.Votes...)
				}

				continue
			}
		}

		kt.msgsChan <- msg{kramaID: peerID, msg: rawMsg, msgType: icsmsg.MsgType}
	}
}

// forwardMsgToEngine forwards a message to the krama engine message channel.
func (kt *KramaTransport) forwardMsgToEngine(msg *types.ICSMSG) {
	kt.msgsToEngine <- msg
}

// forwardMsgToRouter forwards a message to the context router message channel based on the given cluster id.
func (kt *KramaTransport) forwardMsgToRouter(msg *types.ICSMSG) {
	if cr := kt.contextRouters.get(msg.ClusterID); cr != nil {
		cr.handleMessage(msg)
	}
}

// GetRoundVoteSetBits returns the voteset bits for a specific round and message type.
func (kt *KramaTransport) GetRoundVoteSetBits(
	clusterID common.ClusterID,
) (map[int32]*types.VoteBitSet, error) {
	if cr := kt.contextRouters.get(clusterID); cr != nil {
		return cr.getRoundVoteSetBits()
	}

	return nil, errors.New("context router not found")
}

// removePeerFromRouters removes a given icsPeer from the active peers entries of all contextRouters.
func (kt *KramaTransport) removePeerFromRouters(peer *icsPeer) {
	for _, cr := range kt.contextRouters.list() {
		cr.gossipPeers.remove(peer.kramaID)
	}
}

// sendICSGraft sends an ICSGRAFT message to a specific peer.
func (kt *KramaTransport) sendICSGraft(clusterID common.ClusterID, peer *icsPeer) error {
	err := kt.SendMessage(peer.kramaID, kt.selfID, clusterID, message.ICSGRAFT, types.NewICSGraft([]byte("graft")))
	if err != nil {
		return err
	}

	return nil
}

// manageGossipPeerConn manages the connection status of a gossip peer identified by peerID.
func (kt *KramaTransport) manageGossipPeerConn(cr *ContextRouter, peerID id.KramaID) {
	gossipPeer := cr.gossipPeers.get(peerID)
	if gossipPeer == nil {
		cr.gossipPeers.add(peerID, true)

		return
	}

	gossipPeer.setConnectionStatus(true)
}

// handleICSGraft processes an incoming ICSGRAFT message.
func (kt *KramaTransport) handleICSGraft(kPeer *icsPeer, msg *types.ICSMSG) {
	if msg.Sender != "" && kPeer.kramaID == "" {
		kPeer.kramaID = msg.Sender
	}

	cr := kt.contextRouters.get(msg.ClusterID)
	if cr == nil {
		kt.transitPeers.add(msg.ClusterID, msg.Sender)

		return
	}

	if kPeer := kt.meshPeerset.Peer(msg.Sender); kPeer != nil {
		kt.manageGossipPeerConn(cr, kPeer.kramaID)

		return
	}

	err := kt.ConnectToMeshPeer(kt.ctx, msg.Sender, msg.ClusterID)
	if err != nil {
		kt.logger.Error("Failed to connect peer", "krama-id", msg.Sender, "err", err)
	}
}

func (kt *KramaTransport) initMessageHandler(peer *icsPeer) {
	for {
		select {
		case <-kt.ctx.Done():
			return
		default:
			if err := kt.handlePeerMessage(peer); err != nil {
				kt.logger.Trace(
					"Error handling peer message",
					"peer-id", peer.networkID,
					"err", err,
					"direct-peer", peer.isDirectPeer,
				)

				return
			}
		}
	}
}

// handlePeerMessage decodes and processes the incoming ics message.
func (kt *KramaTransport) handlePeerMessage(peer *icsPeer) error {
	msg, err := peer.decodePeerMessage()
	if err != nil {
		return err
	}

	switch msg.MsgType {
	case message.ICSGRAFT:
		go kt.handleICSGraft(peer, msg)

	case message.ICSREQUEST:
		fallthrough
	case message.ICSRESPONSE:
		fallthrough
	case message.ICSSUCCESS:
		fallthrough
	case message.ICSFAILURE:
		kt.forwardMsgToEngine(msg)
	case message.ICSHAVE:
		fallthrough
	case message.ICSWANT:
		kt.forwardMsgToRouter(msg)
	}

	return nil
}

// removeExpiredRouters removes the expired context routers from the contextRouters.
func (kt *KramaTransport) removeExpiredRouters() {
	currentTime := time.Now().Unix()

	for _, cr := range kt.contextRouters.list() {
		if cr.getExpiryTime() > 0 && cr.getExpiryTime() <= currentTime {
			kt.DeregisterContextRouter(cr.clusterID)
		}
	}
}

// removeExpiredTransitPeers removes the expired peers from the transitPeers.
func (kt *KramaTransport) removeExpiredTransitPeers() {
	currentTime := time.Now().Unix()

	for clusterID, list := range kt.transitPeers.list() {
		if currentTime-list.getUpdatedAt() >= 60*1000 && !kt.contextRouters.has(clusterID) {
			kt.transitPeers.remove(clusterID)
		}
	}
}

// cleanup periodically removes expired context routers from the transport.
func (kt *KramaTransport) cleanup(cleanupInterval time.Duration) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-kt.ctx.Done():
			return
		case <-ticker.C:
			kt.removeExpiredRouters()
			kt.removeExpiredTransitPeers()
		}
	}
}

// StartGossip initiates the periodic message broadcast routine.
func (kt *KramaTransport) StartGossip(clusterID common.ClusterID) {
	cr := kt.contextRouters.get(clusterID)
	if cr == nil {
		return
	}

	cr.broadcast(GossipInterval)
}

// Start initializes the KramaTransport.
func (kt *KramaTransport) Start() {
	kt.setupStreamHandlers()
	go kt.cleanup(ClusterCleanupInterval)
	go kt.peerLifeCycle()
}

// Close stops the KramaTransport.
func (kt *KramaTransport) Close() {
	kt.ctxCancel()
}

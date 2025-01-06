package transport

import (
	"bufio"
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
	ctx            context.Context
	ctxCancel      context.CancelFunc
	selfID         id.KramaID
	logger         hclog.Logger
	metrics        *Metrics
	network        Network
	connManager    ConnectionManager
	msgsToEngine   chan *types.ICSMSG
	msgsChan       chan msg
	closePeerChan  chan closePeer
	directPeerLock *locker.Locker
	directPeerset  *icsPeerSet
	contextRouters *contextRouters
	// requestCache                   *RequestCache
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
		ctx:            ctx,
		ctxCancel:      ctxCancel,
		selfID:         selfID,
		logger:         logger.Named("Krama-Transport"),
		metrics:        metrics,
		network:        network,
		connManager:    connManager,
		msgsToEngine:   make(chan *types.ICSMSG, 100),
		msgsChan:       make(chan msg),
		closePeerChan:  make(chan closePeer),
		directPeerLock: locker.New(),
		directPeerset:  newICSPeerSet(),
		contextRouters: newContextRouters(),
		// requestCache:         newRequestCache(),
		minGossipPeers:       minGossipPeers,
		maxGossipPeers:       maxGossipPeers,
		directInboundStreams: make(map[peer.ID]p2pnet.Stream),
	}
}

// setupStreamHandlers configures the KramaTransport to handle the incoming streams.
func (kt *KramaTransport) setupStreamHandlers() {
	kt.connManager.SetupStreamHandler(config.ICSProtocolTransientStream, "", kt.handleTransientStream)
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

func (kt *KramaTransport) disconnectPeer(kPeer *icsPeer) {
	kt.logger.Trace(
		"Disconnecting peer",
		"krama-id", kPeer.kramaID,
		"peer-id", kPeer.networkID,
		"direction", kPeer.stream.Stat().Direction,
		"isDirectPeer", kPeer.isDirectPeer,
	)

	kt.closePeerChan <- closePeer{kramaID: kPeer.kramaID, isDirectPeer: kPeer.isDirectPeer}
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

func (kt *KramaTransport) SendMessageTransientPeer(kramaID id.KramaID, m []byte) error {
	timedCtx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()

	err := kt.connManager.ConnectPeerByKramaID(timedCtx, kramaID)
	if err != nil && !errors.Is(err, common.ErrConnectionExists) {
		return err
	}

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	stream, err := kt.connManager.NewStream(kt.ctx, peerID, config.ICSProtocolTransientStream, "")
	if err != nil {
		return err
	}

	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	return shipMessage(rw, m)
}

func (kt *KramaTransport) peerLifeCycle() {
	for {
		select {
		case <-kt.ctx.Done():
			return
		case m := <-kt.msgsChan:
			if p := kt.directPeerset.Peer(m.kramaID); p != nil {
				p.msgChan <- m.msg

				continue
			}

			// we should open a temporary stream and send the msg if the peer is not registered with directPeerSet
			// this with happen for prepared messages
			if err := kt.SendMessageTransientPeer(m.kramaID, m.msg); err != nil {
				kt.logger.Trace(
					"Failed to send message to transient peer",
					"error", err,
					"peer-id", m.kramaID,
					"msg-type", m.msgType)
			}

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

			kt.logger.Trace("Failed to close mesh peer", "peer-id", m.kramaID)
		}
	}
}

func (kt *KramaTransport) handleTransientStream(stream p2pnet.Stream) {
	peerID := stream.Conn().RemotePeer()

	kt.logger.Trace(
		"Handling transient stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	kPeer := newICSPeer(kt.ctx, "", stream, kt.logger, false)

	defer func() {
		kt.connManager.ResetStream(kPeer.stream, "")
	}()

	if err := kt.handlePeerMessage(kPeer); err != nil {
		kt.logger.Trace("Error handling peer message", "peer-id", peerID, "err", err)
	}
}

func (kt *KramaTransport) handleDirectStream(stream p2pnet.Stream) {
	peerID := stream.Conn().RemotePeer()

	kt.logger.Trace(
		"Handling stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	kt.directInboundStreamsLock.Lock()

	s, ok := kt.directInboundStreams[peerID]
	if ok {
		kt.logger.Trace("Closing the existing direct stream", "peer-id", peerID)

		if err := kt.connManager.ResetStream(s, p2p.ICSDirectTag); err != nil {
			kt.logger.Error("Failed to reset stream", "peer-id", peerID, "err", err)
		}
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

// RegisterContextRouter creates and registers a ContextRouter for a given cluster ID.
func (kt *KramaTransport) RegisterContextRouter(
	ctx context.Context,
	operator id.KramaID,
	clusterID common.ClusterID,
	nodeset *types.ICSCommittee,
	voteset *types.HeightVoteSet,
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

// DeregisterContextRouter unregister a ContextRouter based on the given cluster ID.
func (kt *KramaTransport) DeregisterContextRouter(clusterID common.ClusterID) {
	contextRouter := kt.contextRouters.get(clusterID)
	if contextRouter == nil {
		kt.logger.Error("Context router not found", "cluster-id", clusterID)

		return
	}

	kt.CleanDirectPeer(clusterID, kt.directPeerset.List()...)

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

// SendMessage sends a message to a specific peer identified by peerID.
func (kt *KramaTransport) SendMessage(
	ctx context.Context,
	peerID id.KramaID,
	icsMsg *types.ICSMSG,
) error {
	if peerID == kt.selfID {
		return nil
	}

	rawData, err := icsMsg.Bytes()
	if err != nil {
		return err
	}

	select {
	case <-kt.ctx.Done():
		return kt.ctx.Err()
	case kt.msgsChan <- msg{kramaID: peerID, msg: rawData, msgType: icsMsg.MsgType}:
	}

	return nil
}

// BroadcastTesseract broadcasts a ts to peers that are subscribed to the tesseract topic.
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

	rawMsg, err := icsmsg.Bytes()
	if err != nil {
		kt.logger.Error("Failed to generate wire message", "error", err)

		return
	}

	cr := kt.contextRouters.get(icsmsg.ClusterID)

	for _, peerID := range cr.committee.GetNodes(true) {
		if peerID == kt.selfID {
			continue
		}

		kramaID := peerID
		kt.msgsChan <- msg{kramaID: kramaID, msg: rawMsg, msgType: icsmsg.MsgType}
	}
}

// forwardMsgToEngine forwards a message to the krama engine message channel.
func (kt *KramaTransport) forwardMsgToEngine(msg *types.ICSMSG) {
	kt.msgsToEngine <- msg
}

/*

// GetRoundVoteSetBits returns the voteset bits for a specific round and message type.
func (kt *KramaTransport) GetRoundVoteSetBits(
	clusterID common.ClusterID,
) (map[uint64]*types.VoteBitSet, error) {
	if cr := kt.contextRouters.get(clusterID); cr != nil {
		return cr.getRoundVoteSetBits()
	}

	return nil, errors.New("context router not found")
}


// sendICSGraft sends an ICSGRAFT message to a specific peer.
func (kt *KramaTransport) sendICSGraft(clusterID common.ClusterID, peer *icsPeer) error {
	rawICSGraft, err := types.NewICSGraft([]byte("graft")).Bytes()
	if err != nil {
		return err
	}

	err = kt.SendMessage(
		context.Background(),
		peer.kramaID,
		types.NewICSMsg(
			kt.selfID,
			clusterID,
			message.ICSGRAFT,
			rawICSGraft,
		))
	if err != nil {
		return err
	}

	return nil
}

// handleICSGraft processes an incoming ICSGRAFT message.
func (kt *KramaTransport) handleICSGraft(kPeer *icsPeer, msg *types.ICSMSG) {
	if msg.SenderAddr != "" && kPeer.kramaID == "" {
		kPeer.kramaID = msg.SenderAddr
	}

	cr := kt.contextRouters.get(msg.ClusterID)
	if cr == nil {
		kt.transitPeers.add(msg.ClusterID, msg.SenderAddr)

		return
	}
}

*/

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

	kt.logger.Debug("Handling peer message", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	switch msg.MsgType {
	case message.PREPARE:
		fallthrough
	case message.PREPARED:
		fallthrough
	case message.PROPOSAL:
		fallthrough
	case message.VOTEMSG:
		kt.forwardMsgToEngine(msg)
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
		}
	}
}

// StartGossip initiates the periodic message broadcast routine.
func (kt *KramaTransport) StartGossip(clusterID common.ClusterID) {
	cr := kt.contextRouters.get(clusterID)
	if cr == nil {
		return
	}
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

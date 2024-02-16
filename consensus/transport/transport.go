package transport

import (
	"context"
	"sync"
	"time"

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
	ProtectConnection(peerID peer.ID, tag string)
	UnprotectConnection(peerID peer.ID, tag string)
	ResetStream(stream p2pnet.Stream, tag string) error
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

// KramaTransport represents the ICS message transport layer.
type KramaTransport struct {
	ctx                            context.Context
	ctxCancel                      context.CancelFunc
	selfID                         id.KramaID
	logger                         hclog.Logger
	metrics                        *Metrics
	network                        Network
	connManager                    ConnectionManager
	msgChan                        chan *types.ICSMSG
	peerLocks                      *locker.Locker
	peerset                        *icsPeerSet
	transitPeers                   *transitPeers
	contextRouters                 *contextRouters
	requestCache                   *RequestCache
	minGossipPeers, maxGossipPeers int
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
		msgChan:        make(chan *types.ICSMSG),
		peerLocks:      locker.New(),
		peerset:        newICSPeerSet(),
		transitPeers:   newTransitPeers(),
		contextRouters: newContextRouters(),
		requestCache:   newRequestCache(),
		minGossipPeers: minGossipPeers,
		maxGossipPeers: maxGossipPeers,
	}
}

// setupStreamHandler configures the stream handler to handle the incoming streams.
func (kt *KramaTransport) setupStreamHandler() {
	kt.connManager.SetupStreamHandler(config.ICSProtocolStream, "", kt.handleStream)
}

// Messages returns a channel for receiving messages from the KramaTransport.
func (kt *KramaTransport) Messages() <-chan *types.ICSMSG {
	return kt.msgChan
}

// Connect establishes a connection to a peer and registers the peer
func (kt *KramaTransport) Connect(ctx context.Context, kramaID id.KramaID, clusterID common.ClusterID) error {
	if kramaID == kt.selfID {
		return nil
	}

	peerID, err := kramaID.DecodedPeerID()
	if err != nil {
		return err
	}

	spanCtx, span := tracing.Span(
		ctx,
		"Krama.KramaTransport",
		"Connect",
		trace.WithAttributes(attribute.String("peerID", peerID.String())),
	)

	kt.peerLocks.Lock(peerID.String())
	defer func() {
		if err = kt.peerLocks.Unlock(peerID.String()); err != nil {
			kt.logger.Error("Failed to release peer lock", err)
		}

		span.End()
	}()

	cr := kt.contextRouters.get(clusterID)
	if cr == nil {
		return errors.New("Context router not found")
	}

	// If the peer is already connected, add the cluster and update context router.
	if kPeer := kt.peerset.Peer(kramaID); kPeer != nil {
		if kt.connManager.IsConnectionProtected(peerID, clusterID.String()) {
			kt.logger.Trace("Connection already tagged", "cluster-id", clusterID)

			return nil
		}

		cr.activePeers.add(kramaID)

		kt.connManager.ProtectConnection(peerID, clusterID.String())

		if err = kt.sendICSGraft(spanCtx, clusterID, kPeer); err != nil {
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

	stream, err := kt.connManager.NewStream(kt.ctx, peerID, config.ICSProtocolStream, clusterID.String())
	if err != nil {
		return err
	}

	kPeer := newICSOutboundPeer(kt.ctx, kramaID, stream, kt.logger)

	if err = kt.peerset.Register(kPeer); err != nil {
		return err
	}

	kt.metrics.captureActivePeers(1)
	cr.activePeers.add(kramaID)

	if err = kt.sendICSGraft(spanCtx, clusterID, kPeer); err != nil {
		return err
	}

	go kt.handleDeadPeers(kPeer)

	return nil
}

// handleDeadPeers handles dead peers, removes them from the routers and unregisters them from the peerset.
func (kt *KramaTransport) handleDeadPeers(kPeer *icsOutboundPeer) {
	defer func() {
		kt.logger.Trace("Unregistering dead peers", "peer-id", kPeer.kramaID)

		kt.removePeerFromRouters(kPeer)

		if err := kt.peerset.Unregister(kPeer); err != nil {
			kt.logger.Error("Failed to unregister peer", "peer-id", kPeer.kramaID, "err", err)

			return
		}

		kt.metrics.captureActivePeers(-1)
	}()

	reader := msgio.NewReader(kPeer.rw.Reader)

	_, err := reader.ReadMsg()
	if err != nil {
		return
	}
}

// InitClusterConnection initiates connection to nodes based on the role.
func (kt *KramaTransport) InitClusterConnection(ctx context.Context, clusterID common.ClusterID, isOperator bool) {
	spanCtx, span := tracing.Span(ctx, "Krama.KramaTransport", "InitClusterConnection")
	defer span.End()

	cr := kt.contextRouters.get(clusterID)

	// check if the current node is a validator
	if !isOperator {
		cr.connectToTransitPeers(spanCtx)
		cr.connectToConsecutivePeers(spanCtx)

		return
	}

	cr.connectToICSPeers(spanCtx)
}

// handleStream handles the incoming ics streams and registers the clusters
func (kt *KramaTransport) handleStream(stream p2pnet.Stream) {
	kt.logger.Trace(
		"Handling new stream",
		"protocol",
		stream.Protocol(),
		"peer-ID",
		stream.Conn().RemotePeer(),
	)

	kPeer := newICSInboundPeer(stream)

	go func() {
		for {
			select {
			case <-kt.ctx.Done():
				return
			default:
				if err := kt.handlePeerMessage(kPeer); err != nil {
					kt.logger.Error("Error handling peer message", "peer-id", kPeer.networkID, "err", err)

					return
				}
			}
		}
	}()
}

// getTransitPeers returns the transit peers associated with the given cluster ID.
func (kt *KramaTransport) getTransitPeers(clusterID common.ClusterID) map[id.KramaID]struct{} {
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
		kt.logger.With("cluster-ID", clusterID),
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

	for _, kramaID := range contextRouter.activePeers.list() {
		peerID, _ := kramaID.DecodedPeerID()
		kt.connManager.UnprotectConnection(peerID, clusterID.String())
	}

	contextRouter.close()
	kt.contextRouters.remove(clusterID)
	kt.metrics.captureActiveRouters(-1)

	kt.logger.Trace("Context router de-registered", "cluster-id", clusterID)
}

// SendMessage sends a message to a specific peer identified by peerID.
func (kt *KramaTransport) SendMessage(
	ctx context.Context,
	peerID id.KramaID,
	sender id.KramaID,
	clusterID common.ClusterID,
	msgType message.MsgType,
	rawMsg types.ICSPayload,
) error {
	kPeer := kt.peerset.Peer(peerID)
	if kPeer == nil {
		return errNotRegistered
	}

	if peerID == sender {
		return nil
	}

	if err := kPeer.send(sender, clusterID, msgType, rawMsg); err != nil {
		return err
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
	msg *types.ICSMSG,
) {
	_, span := tracing.Span(
		ctx,
		"Krama.KramaTransport",
		"BroadcastMessage",
		trace.WithAttributes(attribute.String("peer-id", string(msg.Sender))))
	defer span.End()

	cr := kt.contextRouters.get(msg.ClusterID)
	if cr == nil {
		kt.logger.Error("Failed to broadcast, context router not found", "cluster-id", msg.ClusterID)

		return
	}

	rawMsg, err := msg.Bytes()
	if err != nil {
		kt.logger.Error("Failed to generate wire message", "error", err)

		return
	}

	for _, peerID := range cr.getBroadcastPeers(msg.MsgType) {
		// if operator avoid, broadcasting the message to the peer again,
		// since every peer sends the vote message to the operator directly
		// this assumption has implications
		if (peerID == cr.operator) && msg.Sender != cr.selfID {
			continue
		}

		if msg.ReceivedFrom == peerID {
			continue
		}

		kPeer := kt.peerset.Peer(peerID)
		if kPeer == nil {
			kt.logger.Error("Failed to send msg", "error", errNotRegistered)

			continue
		}

		currentTime := time.Now()

		kPeer.msgChan <- rawMsg

		if time.Since(currentTime).Milliseconds() >= 1 {
			kt.logger.Trace(":Time taken to forward message write lock:", time.Since(currentTime), kPeer.kramaID, msg.ClusterID)
		}
	}
}

// forwardMsgToEngine forwards a message to the krama engine message channel.
func (kt *KramaTransport) forwardMsgToEngine(msg *types.ICSMSG) {
	kt.msgChan <- msg
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

// removePeerFromRouters removes a given icsOutboundPeer from the active peers list of all contextRouters.
func (kt *KramaTransport) removePeerFromRouters(peer *icsOutboundPeer) {
	for _, cr := range kt.contextRouters.list() {
		cr.activePeers.remove(peer.kramaID)
	}
}

// sendICSGraft sends an ICSGRAFT message to a specific peer.
func (kt *KramaTransport) sendICSGraft(ctx context.Context, clusterID common.ClusterID, peer *icsOutboundPeer) error {
	err := kt.SendMessage(ctx, peer.kramaID, kt.selfID, clusterID, message.ICSGRAFT, types.NewICSGraft([]byte("graft")))
	if err != nil {
		return err
	}

	return nil
}

// handleICSGraft processes an incoming ICSGRAFT message.
func (kt *KramaTransport) handleICSGraft(peer *icsInboundPeer, msg *types.ICSMSG) {
	if msg.Sender != "" {
		peer.KramaID = msg.Sender
	}

	if !kt.contextRouters.has(msg.ClusterID) {
		kt.transitPeers.add(msg.ClusterID, msg.Sender)

		return
	}

	if kPeer := kt.peerset.Peer(msg.Sender); kPeer != nil {
		if cr := kt.contextRouters.get(msg.ClusterID); cr != nil {
			cr.activePeers.add(kPeer.kramaID)
		}

		return
	}

	err := kt.Connect(kt.ctx, msg.Sender, msg.ClusterID)
	if err != nil {
		kt.logger.Error("Failed to connect peer", "krama-id", msg.Sender, "err", err)
	}
}

// handlePeerMessage decodes and processes the incoming ics message.
func (kt *KramaTransport) handlePeerMessage(peer *icsInboundPeer) error {
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
func (kt *KramaTransport) cleanup(cleanupInteraval time.Duration) {
	ticker := time.NewTicker(cleanupInteraval)
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

func (kt *KramaTransport) StartGossip(clusterID common.ClusterID) {
	cr := kt.contextRouters.get(clusterID)
	if cr == nil {
		return
	}

	go cr.broadcast(GossipInterval)
}

// Start initializes the KramaTransport.
func (kt *KramaTransport) Start() {
	kt.setupStreamHandler()
	go kt.cleanup(ClusterCleanupInterval)
}

// Close stops the KramaTransport.
func (kt *KramaTransport) Close() {
	kt.ctxCancel()
}

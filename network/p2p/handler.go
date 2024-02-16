package p2p

import (
	"context"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-msgio"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/lattice"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/senatus"
)

const (
	MaxQueueSize  = 200
	MsgsPerWorker = 10
)

type ixPool interface {
	AddInteractions(ixs common.Interactions) []error
}

type ReputationManager interface {
	UpdatePeer(data *senatus.NodeMetaInfo) error
}

type SubHandler struct {
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	id                  kramaid.KramaID
	peers               *peerSet
	ixpool              ixPool
	chain               *lattice.ChainManager
	mux                 *utils.TypeMux
	ixSub               *utils.Subscription
	newPeerSub          *utils.Subscription
	server              *Server
	reputationManager   ReputationManager
	pendingMessageQueue *RequestQueue
	logger              hclog.Logger
	signalChan          chan struct{}
}

// NewSubHandler is a constructor function generates and returns a new subHandle object.
// Accepts a KramaID, an event TypeMux, an IxPool, an ICS, a BFT engine and a Chain manager.
func NewSubHandler(
	id kramaid.KramaID,
	logger hclog.Logger,
	server *Server,
	reputationManager ReputationManager,
	peerSet *peerSet,
	mux *utils.TypeMux,
	pool ixPool,
	chain *lattice.ChainManager,
) *SubHandler {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &SubHandler{
		id:                  id,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		peers:               peerSet,
		mux:                 mux,
		chain:               chain,
		ixpool:              pool,
		server:              server,
		reputationManager:   reputationManager,
		logger:              logger.Named("Sub-Handler"),
		signalChan:          make(chan struct{}),
		pendingMessageQueue: NewRequestQueue(MaxQueueSize), // Max message queue limit is 200
		// Subscribe the TypeMux to AddedInteractionEvent and NewPeerEvent events
		ixSub:      mux.Subscribe(utils.AddedInteractionEvent{}),
		newPeerSub: mux.Subscribe(utils.NewPeerEvent{}),
	}
}

// Start is a method of SubHandler that start the handler.
// Initializes it TypeMux subscriptions and handler loops.
func (eh *SubHandler) Start() error {
	if err := eh.server.Subscribe(
		eh.ctx,
		config.HelloTopic,
		nil,
		true,
		eh.helloMsgHandler,
	); err != nil {
		return errors.Wrap(err, "failed to subscribe senatus topic")
	}

	go eh.messageWorker()
	// Start the handler loops for new peers, broadcasting interactions
	go eh.newPeerLoop()
	go eh.ixBroadcastLoop()

	return nil
}

// newPeerLoop is a method of SubHandler that handles NewPeerEvents.
// Creates a new Peer for every NewPeerEvent signal, registers it with the
// handler working set and starts a goroutine to listen for messages from the peer.
func (eh *SubHandler) newPeerLoop() {
	// Read events from a newpeer channel
	for obj := range eh.newPeerSub.Chan() {
		// Assert event as a NewPeerEvent
		if p, ok := obj.Data.(utils.NewPeerEvent); ok {
			// If minimum peer count is met, send a hello message
			if eh.peers.Len() > 3 {
				eh.server.SendHelloMessage()
			}

			peer := eh.peers.Peer(p.PeerID)
			// Asynchronously handle the new peer
			go func(peer *Peer) {
				// Defer the peer unregister from the handler working set
				defer func() {
					if err := eh.peers.Unregister(peer); err != nil {
						eh.logger.Error("Error unregistering peer", "err", err)

						return
					}

					// Update inbound/outbound connection count based on the peer stream's direction
					eh.server.ConnManager.updateConnCount(peer.stream.Stat().Direction, peer.GetRTT(), -1)

					if err := eh.server.ConnManager.ResetStream(peer.stream, MOIStreamTag); err != nil {
						eh.server.logger.Trace("Failed to reset connection", "err", err)
					}

					eh.logger.Info("Peer Disconnected", "krama-ID", peer.kramaID)
				}()

				// Handle messages from the peer
				for {
					if err := eh.handlePeerMessage(peer); err != nil {
						eh.logger.Error("Error handling peer message", "err", err)

						eh.sendDisconnectRequest(peer, err)

						return
					}
				}
			}(peer)
		}
	}
}

// handlePeerMessage is a method of SubHandler that handles a message from a Peer
func (eh *SubHandler) handlePeerMessage(p *Peer) error {
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

	// log.Println("Got new Message", message.GetType())
	switch message.MsgType {
	// NEWIXTS
	case networkmsg.NEWIXSMSG:
		// Unmarshal message proto into an InteractionsData message
		ixns := new(common.Interactions)
		if err := ixns.FromBytes(message.Payload); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return err
			}
		}

		// Mark the interactions in the message as 'known' by the peer
		for _, v := range *ixns {
			eh.logger.Info("Received interactions from", "krama-ID", p.kramaID, "ix-hash", v.Hash())

			p.markInteraction(v.Hash())
		}

		// Add the interactions to the handler's interaction pool
		errs := eh.ixpool.AddInteractions(*ixns)
		for index, err := range errs {
			if err != nil {
				if errors.Is(err, common.ErrAlreadyKnown) {
					continue
				}

				ixnss := *ixns

				eh.logger.Error("Unable to add interaction", "ix-hash", ixnss[index].Hash(), "err", err)

				return nil
			}
		}

	case networkmsg.DISCONNECTREQ:
		eh.handleDisconnectRequest(p, message)

		return nil
	}

	return nil
}

func (eh *SubHandler) handleDisconnectRequest(peer *Peer, msg *networkmsg.Message) {
	var disconnectMsg networkmsg.DisconnectReq

	if err := disconnectMsg.FromBytes(msg.Payload); err != nil {
		eh.logger.Error("Decode disconnect request.", "from", msg.Sender, "err", err)

		peer.stream.Conn().Close()

		return
	}

	eh.logger.Info("Handled disconnect request from", "krama-ID", msg.Sender, "reason", disconnectMsg.Reason)
}

func (eh *SubHandler) sendDisconnectRequest(peer *Peer, err error) {
	disconnectMsg := &networkmsg.DisconnectReq{
		Reason: err.Error(),
	}

	peer.Send(eh.server.GetKramaID(), networkmsg.DISCONNECTREQ, disconnectMsg)
}

// ixBroadcastLoop is a method of SubHandler that handles NewIxsEvents.
// Creates an ICS cluster and Krama state object from it before starting
// the BFT engine to reach a consensus on the new State
func (eh *SubHandler) ixBroadcastLoop() {
	// Read events from an ix channel
	for obj := range eh.ixSub.Chan() {
		// Assert event as a AddedInteractionEvent
		if event, ok := obj.Data.(utils.AddedInteractionEvent); ok {
			if err := eh.broadcastIXs(event.Ixs); err != nil {
				eh.logger.Error("Failed to broadcast interactions", "err", err)
			}
		}
	}
}

// broadcastIXs is a method of SubHandler that broadcasts a given slice of Interactions.
// Only emits it from peers that are not already aware of the interaction.
func (eh *SubHandler) broadcastIXs(ixs []*common.Interaction) error {
	// Accumulate a mapping of peers to the Interaction they do not know about
	peerIxSet := make(map[*Peer][]*common.Interaction)

	for _, ix := range ixs {
		ixhash := ix.Hash()

		// Identify the peers in the handler's working set that do not know of the interaction
		peers := eh.peers.PeersWithoutIX(ixhash)
		for _, peer := range peers {
			// Add the peer and the interaction it does not know about to the peerIxSet
			peerIxSet[peer] = append(peerIxSet[peer], ix)
		}
	}

	// Emit the Interaction
	for peer, ixs := range peerIxSet {
		go func(peer *Peer, ixs []*common.Interaction) {
			if err := peer.SendIXs(eh.id, ixs); err != nil {
				eh.logger.Error("Error sending interaction", "err", err)
			}
		}(peer, ixs)
	}

	return nil
}

func (eh *SubHandler) helloMsgHandler(msg *pubsub.Message) error {
	helloMsg := new(networkmsg.HelloMsg)

	if err := helloMsg.FromBytes(msg.Data); err != nil {
		return err
	}

	eh.logger.Trace("Received hello message", "krama-ID", helloMsg.KramaID)

	if err := eh.pendingMessageQueue.Push(helloMsg); err != nil {
		eh.signalNewMessages()
	}

	return nil
}

func (eh *SubHandler) messageWorker() {
	for {
		select {
		case <-time.After(1 * time.Second):
		case <-eh.signalChan:
		case <-eh.ctx.Done():
			return
		}

		eh.handleMessages(eh.pendingMessageQueue.Pop(MsgsPerWorker))
	}
}

func (eh *SubHandler) signalNewMessages() {
	select {
	case eh.signalChan <- struct{}{}:
	default:
	}
}

func (eh *SubHandler) verifyHelloMsg(msg *networkmsg.HelloMsg) error {
	rawData, err := msg.Canonical()
	if err != nil {
		return errors.Wrapf(err, "Failed to fetch hello message bytes")
	}

	if err := crypto.VerifySignatureUsingKramaID(msg.KramaID, rawData, msg.Signature); err != nil {
		return errors.Wrap(err, "failed to verify hello msg signature")
	}

	return nil
}

func (eh *SubHandler) handleMessages(msgs []*networkmsg.HelloMsg) {
	if len(msgs) == 0 {
		return
	}

	for _, msg := range msgs {
		if msg.KramaID == "" {
			continue
		}

		if err := eh.verifyHelloMsg(msg); err != nil {
			eh.logger.Error("Failed to verify hello message", "err", err)

			continue
		}

		filter, err := makeAddrsFactory(
			eh.server.cfg.DisablePrivateIP,
			eh.server.cfg.AllowIPv6Addresses,
			nil,
		)
		if err != nil {
			eh.logger.Error("Failed to create multi addr filter", "err", err)

			continue
		}

		filteredAddr := filter(utils.MultiAddrFromString(msg.Address...))
		if len(filteredAddr) == 0 {
			eh.logger.Info("Peer with no good address", "krama-id", msg.KramaID)

			continue
		}

		// TODO: Should check RTT

		if err = eh.reputationManager.UpdatePeer(&senatus.NodeMetaInfo{
			Addrs:         utils.MultiAddrToString(filteredAddr...),
			KramaID:       msg.KramaID,
			NTQ:           senatus.DefaultPeerNTQ,
			PeerSignature: msg.Signature,
		}); err != nil {
			eh.logger.Error("Failed to add node meta information", "err", err, "krama-ID", msg.KramaID)

			continue
		}
	}
}

func (eh *SubHandler) Close() {
	eh.ixSub.Unsubscribe()
	eh.newPeerSub.Unsubscribe()
}

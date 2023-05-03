package poorna

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/libp2p/go-msgio"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/ixpool"
	"github.com/sarvalabs/moichain/lattice"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

// Struct that represents the network event handler
type SubHandler struct {
	ctx context.Context // Context for handler life cycle

	ctxCancel context.CancelFunc // Context cancel function

	id id.KramaID // KramaID of the node running the handler

	peers *peerSet // working set of peers of the handler

	ixpool *ixpool.IxPool // Interaction pool of the handler

	// Represents the chain manager of the handler
	chain *lattice.ChainManager

	mux *utils.TypeMux // event type mux of the handler

	ixSub *utils.Subscription // interactions event subscription

	newPeerSub *utils.Subscription // newpeer event subscription

	server *Server

	logger hclog.Logger
}

// NewSubHandler is a constructor function generates and returns a new subHandle object.
// Accepts a KramaID, an event TypeMux, an IxPool, an ICS, a BFT engine and a Chain manager.
func NewSubHandler(
	ctx context.Context,
	id id.KramaID,
	logger hclog.Logger,
	server *Server,
	peerSet *peerSet,
	mux *utils.TypeMux,
	pool *ixpool.IxPool,
	chain *lattice.ChainManager,
) *SubHandler {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &SubHandler{
		id:        id,
		ctx:       ctx,
		ctxCancel: ctxCancel,
		peers:     peerSet,
		mux:       mux,
		chain:     chain,
		ixpool:    pool,
		server:    server,
		logger:    logger.Named("Sub-Handler"),
	}
}

// Start is a method of SubHandler that start the handler.
// Initializes it TypeMux subscriptions and handler loops.
func (eh *SubHandler) Start() {
	log.Println("Handler started")
	// Subscribe the TypeMux to NewIxsEvent and NewPeerEvent events
	eh.ixSub = eh.mux.Subscribe(utils.NewIxsEvent{})
	eh.newPeerSub = eh.mux.Subscribe(utils.NewPeerEvent{})

	// Start the handler loops for new peers, broadcasting
	// interactions and handling ICS events

	go eh.newPeerLoop()
	go eh.ixBroadcastLoop()
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
						eh.logger.Error("Error unregistering peer", "error", err)

						return
					}

					// Update inbound/outbound connection count based on the peer stream's direction
					eh.server.connInfo.updateConnCount(peer.stream.Stat().Direction, -1)

					eh.logger.Info("Peer Disconnected", "id", peer.kramaID)
				}()

				// Handle messages from the peer
				for {
					if err := eh.handlePeerMessage(peer); err != nil {
						eh.logger.Error("Error handling peer message", err)

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
	message := new(ptypes.Message)
	if err := message.FromBytes(buffer); err != nil {
		return err
	}

	// log.Println("Got new Message", message.GetType())
	switch message.MsgType {
	// NEWIXTS
	case ptypes.NEWIXSMSG:
		// Print the KramaID of the interactions sender
		eh.logger.Info("Received Interactions from", "id", p.kramaID)

		// Unmarshal message proto into an InteractionsData message
		ixns := new(types.Interactions)
		if err := ixns.FromBytes(message.Payload); err != nil {
			if !errors.Is(err, polo.ErrNullPack) {
				return err
			}
		}

		// Mark the interactions in the message as 'known' by the peer
		for _, v := range *ixns {
			p.markInteraction(v.Hash())
		}

		// Add the interactions to the handler's interaction pool
		errs := eh.ixpool.AddInteractions(*ixns)
		for index, err := range errs {
			if err != nil {
				ixnss := *ixns

				eh.logger.Trace("Unable to add Interaction ", "hash", ixnss[index].Hash(), "error", err)

				return nil
			}
		}

		if err := eh.broadcastIXs(*ixns); err != nil {
			eh.logger.Error("Failed to broadcast interactions", "error", err)
		}

	case ptypes.RANDOMWALKREQ:
		eh.logger.Info("Received a random-walk request", "from", message.Sender)

	case ptypes.DISCONNECTREQ:
		eh.handleDisconnectRequest(p, message)

		return nil
	}

	return nil
}

func (eh *SubHandler) handleDisconnectRequest(peer *Peer, msg *ptypes.Message) {
	var disconnectMsg ptypes.DisconnectReq

	if err := disconnectMsg.FromBytes(msg.Payload); err != nil {
		eh.logger.Error("Decode disconnect req", "from", msg.Sender, "error", err)

		peer.stream.Conn().Close()

		return
	}

	eh.logger.Info("Received disconnect request", "from", msg.Sender, "reason", disconnectMsg.Reason)
}

func (eh *SubHandler) sendDisconnectRequest(peer *Peer, err error) {
	disconnectMsg := &ptypes.DisconnectReq{
		Reason: err.Error(),
	}
	peer.Send(eh.server.GetKramaID(), ptypes.DISCONNECTREQ, disconnectMsg)
}

// ixBroadcastLoop is a method of SubHandler that handles NewIxsEvents.
// Creates an ICS cluster and Krama state object from it before starting
// the BFT engine to reach a consensus on the new State
func (eh *SubHandler) ixBroadcastLoop() {
	// Read events from an ix channel
	for obj := range eh.ixSub.Chan() {
		// Assert event as a NewIxsEvent
		if event, ok := obj.Data.(utils.NewIxsEvent); ok {
			if err := eh.broadcastIXs(event.Ixs); err != nil {
				eh.logger.Error("Failed to broadcast interactions", "error", err)
			}
		}
	}
}

// broadcastIXs is a method of SubHandler that broadcasts a given slice of Interactions.
// Only emits it from peers that are not already aware of the interaction.
func (eh *SubHandler) broadcastIXs(ixs []*types.Interaction) error {
	// Accumulate a mapping of peers to the the Interaction they do not know about
	peerIxSet := make(map[*Peer][]*types.Interaction)

	for _, ix := range ixs {
		ixhash := ix.Hash()

		// Identify the peers in the handler's working set that do not know of the interaction
		peers := eh.peers.PeersWithoutIX(ixhash)
		for _, peer := range peers {
			// Add the peer and the interaction it does not know about to the peerIxSet
			peerIxSet[peer] = append(peerIxSet[peer], ix)
		}
		// Log the Interaction broadcast
		fmt.Printf("Broadcasting Interaction %s ", ixhash)
	}

	// FIXME: Include the following line of code
	// peers = peers[:int(math.Sqrt(float64(len(peers))))]

	// Emit the Interaction
	for peer, ixs := range peerIxSet {
		go func(peer *Peer, ixs []*types.Interaction) {
			if err := peer.SendIXs(eh.id, ixs); err != nil {
				eh.logger.Error("Error sending interaction", "error", err)
			}
		}(peer, ixs)
	}

	return nil
}

func (eh *SubHandler) Close() {
	eh.ixSub.Unsubscribe()
	eh.newPeerSub.Unsubscribe()
	eh.ctxCancel()
}

package poorna

import (
	"context"
	"fmt"
	"log"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/moichain/chain"
	"github.com/sarvalabs/moichain/ixpool"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

// SubHandler is a struct that represents the network event handler
type SubHandler struct {
	// Context for handler life cycle
	ctx context.Context

	// Context cancel function
	ctxCancel context.CancelFunc

	// Represents the KipID of the node running the handler
	id id.KramaID

	// Represents the working set of peers of the handler
	peers *peerSet

	// Represents the Interaction pool of the handler
	ixpool *ixpool.IxPool

	// Represents the chain manager of the handler
	chain *chain.ChainManager

	// Represents the event type mux of the handler
	mux *utils.TypeMux

	// Represents the interactions event subscription
	ixSub *utils.Subscription

	// Represents the newpeer event subscription
	newPeerSub *utils.Subscription

	server *Server

	logger hclog.Logger
}

// NewSubHandler is a constructor function generates and returns a new subHandle object.
// Accepts a KipID, an event TypeMux, an IxPool, an ICS, a BFT engine and a Chain manager.
func NewSubHandler(
	ctx context.Context,
	id id.KramaID,
	logger hclog.Logger,
	server *Server,
	peerSet *peerSet,
	mux *utils.TypeMux,
	pool *ixpool.IxPool,
	chain *chain.ChainManager,
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
// Creates a new KipPeer for every NewPeerEvent signal, registers it with the
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
			go func(peer *KipPeer) {
				// Defer the peer unregister from the handler working set
				defer func() {
					if err := eh.peers.Unregister(peer); err != nil {
						eh.logger.Error("Error unregistering peer", "error", err)
					}

					eh.logger.Info("Peer Disconnected", "id", peer.id)
				}()

				// Handle messages from the peer
				for {
					if err := eh.handlePeerMessage(peer); err != nil {
						return
					}
				}
			}(peer)
		}
	}
}

// handlePeerMessage is a method of SubHandler that handles a message from a KipPeer
func (eh *SubHandler) handlePeerMessage(p *KipPeer) error {
	// Read the peer's io read/writer into a buffer
	// p.mtxLock.Lock()
	// defer p.mtxLock.Unlock()
	buffer := make([]byte, 4096)

	bytecount, err := p.rw.Reader.Read(buffer)
	if err != nil {
		return err
	}

	// Unmarshal the buffer into a proto message
	message := new(ptypes.Message)
	if err = message.FromBytes(buffer[0:bytecount]); err != nil {
		return err
	}

	// log.Println("Got new Message", message.GetType())
	switch message.MsgType {
	// NEWIXTS
	case ptypes.NEWIXSMSG:
		// Print the KipID of the interactions sender
		eh.logger.Info("Received Interactions from", "id", p.id)

		// Unmarshal message proto into an InteractionsData message
		var ixns ptypes.InteractionMsg
		if err = ixns.FromBytes(message.Payload); err != nil {
			return err
		}

		// Mark the interactions in the message as 'known' by the peer
		for _, v := range ixns.Ixs {
			ixHash, err := v.GetIxHash()
			if err != nil {
				return err
			}

			p.markInteraction(ixHash)
		}

		// Add the interactions to the handler's interaction pool
		errs := eh.ixpool.AddInteractions(ixns.Ixs)
		for index, err := range errs {
			if err != nil {
				ixHash, ixHashErr := ixns.Ixs[index].GetIxHash()
				if ixHashErr != nil {
					eh.logger.Error("Unable to create interaction hash", "hash", ixHash, "error", ixHashErr)

					return nil
				}

				eh.logger.Trace("Unable to add Interaction ", "hash", ixHash, "error", err)

				return nil
			}
		}

		if err := eh.broadcastIXs(ixns.Ixs); err != nil {
			eh.logger.Error("Failed to broadcast interactions", "error", err)
		}

	case ptypes.RANDOMWALKREQ:
		log.Println("Got an random request", message.Sender)
	}

	return nil
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
	peerIxSet := make(map[*KipPeer][]*types.Interaction)

	for _, ix := range ixs {
		// Identify the peers in the handler's working set that do not know of the interaction
		ixHash, err := ix.GetIxHash()
		if err != nil {
			return err
		}

		peers := eh.peers.PeersWithoutIX(ixHash)
		for _, peer := range peers {
			// Add the peer and the interaction it does not know about to the peerIxSet
			peerIxSet[peer] = append(peerIxSet[peer], ix)
		}
		// Log the Interaction broadcast
		fmt.Printf("Broadcasting Interaction %s ", ixHash.Hex())
	}

	// FIXME: Include the following line of code
	// peers = peers[:int(math.Sqrt(float64(len(peers))))]

	// Emit the Interaction
	for peer, ixs := range peerIxSet {
		go func(peer *KipPeer, ixs []*types.Interaction) {
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

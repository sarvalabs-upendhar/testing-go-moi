package kbft

import (
	"context"
	"gitlab.com/sarvalabs/polo/go-polo"
	"log"

	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

// MessageRouter acts as a bridge between ics and bft routines,
type MessageRouter struct {
	ctx context.Context

	ctxCancel context.CancelFunc

	// Represents the channel on which ClusterInfo messages are received
	inboundMsgChan chan *ktypes.ICSMSG

	// Represents the channel on which ClusterInfo messages are sent
	outboundMsgChan chan *ktypes.ICSMSG

	// Channel on which consensus messages are received
	bftOutboundChan chan ktypes.ConsensusMessage

	// Represents the Krama BFT service.
	bft *KBFT

	clusterID ktypes.ClusterID

	setType ktypes.IcsSetType

	msgs []*ktypes.ICSMSG
}

// NewMessageRouter is a constructor function that generates and returns a new MessageRouter.
func NewMessageRouter(
	ctx context.Context,
	inboundChan, outboundChan chan *ktypes.ICSMSG,
	bchan chan ktypes.ConsensusMessage,
	kbft *KBFT,
	clusterID ktypes.ClusterID,
	setType ktypes.IcsSetType,
) *MessageRouter {
	m := &MessageRouter{
		inboundMsgChan:  inboundChan,
		outboundMsgChan: outboundChan,
		bftOutboundChan: bchan,
		bft:             kbft,
		clusterID:       clusterID,
		setType:         setType,
	}
	m.ctx, m.ctxCancel = context.WithTimeout(ctx, MaxBFTimeout)

	return m
}

// Start is a method of ConsensusHandler that starts the event handler loops for reading messages.
func (m *MessageRouter) Start() {
	// Start the read loop routine
	defer log.Println("Closing message router", m.clusterID)

	if m.setType == ktypes.ObserverSet {
		go m.CollectMessages()
	} else {
		go m.HandleMessages()
	}

	<-m.ctx.Done()

}

// handleICSMessage is a method of ConsensusHandler that handles an ClusterInfo message
// Marshals the received messages based on their type, wraps it into a
// Consensus Message and sends it into the KBFT service.
func (m *MessageRouter) handleICSMessage(message *ktypes.ICSMSG) {
	// Retrieve the peerID, message data and SID
	peerID, msgdata, msgtype := message.Sender, message.Msg, message.ReqType

	switch msgtype {
	// VOTE message
	case ktypes.VOTEMSG:
		vote := new(ktypes.Vote)

		// Unmarshal message from proto
		if err := polo.Depolorize(vote, msgdata); err != nil {
			log.Panicln(err)
		}
		// Create a consensus message for the Vote
		cmsg := &ktypes.ConsensusMessage{
			PeerID:  peerID,
			Message: &ktypes.VoteMessage{Vote: vote},
		}

		// Send the consensus message
		m.bft.HandlePeerMsg(*cmsg)
	default:
		m.bft.logger.Error("Invalid ics message received", msgtype)
	}
}

// handleBroadcastMessage is a method of ConsensusHandler that handles consensus message broadcast.
func (m *MessageRouter) handleBroadcastMessage(message ktypes.ConsensusMessage) {
	// Retrieve the peerID and message data from the message
	peerID, data := message.PeerID, message.Message

	switch msg := data.(type) {
	// Vote Message
	case *ktypes.VoteMessage:
		// Marshal proto message into an ClusterInfo message and push into the send queue
		rawData := msg.Vote.Bytes()
		m.outboundMsgChan <- &ktypes.ICSMSG{
			ReqType:   ktypes.VOTEMSG,
			Msg:       rawData,
			Sender:    peerID,
			ClusterID: string(m.clusterID),
		}

	// Unsupported Message
	default:
		m.bft.logger.Error("Invalid broadcast message received")
	}
}

func (m *MessageRouter) CollectMessages() {
	for v := range m.inboundMsgChan {
		m.msgs = append(m.msgs, v) //FIXME: handle race conditions at the message queue
	}
}

func (m *MessageRouter) HandleMessages() {
	defer close(m.outboundMsgChan)
	for {
		select {
		// Broadcast read routine
		case msg, ok := <-m.bftOutboundChan:
			if !ok {
				return
			}

			go m.handleBroadcastMessage(msg)

		// PoXc read routine
		case msg, ok := <-m.inboundMsgChan:
			if !ok {
				return
			}

			go m.handleICSMessage(msg)
		}
	}
}

func (m *MessageRouter) Msgs() []*ktypes.ICSMSG {
	return m.msgs
}

package poorna

import (
	"bufio"
	"sync"

	"github.com/libp2p/go-msgio"

	mapset "github.com/deckarep/golang-set"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// Peer is a struct that represents a peer on the network
type Peer struct {
	kramaID id.KramaID // Represents the KramaID of the peer

	networkID peer.ID // Represents the libp2p-peer-id of the peer

	stream network.Stream // Represents peer's stream

	rw bufio.ReadWriter // Represents the peer's read/write buffer

	knownIXs mapset.Set // Represents the set of interactions known to the peer

	logger hclog.Logger

	mtxLock sync.RWMutex
}

// newPeer is a constructor function that generates and returns a Peer
// for a given libp2p peerID and a read/write io buffer.
func newPeer(stream network.Stream, logger hclog.Logger) *Peer {
	return &Peer{
		stream:    stream,
		networkID: stream.Conn().RemotePeer(),
		rw:        *bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream)),
		knownIXs:  mapset.NewSet(),
		logger:    logger.Named("Peer"),
	}
}

// SendHandshakeMessage is a method of KipPeer that emits the peer's id to the network
func (p *Peer) SendHandshakeMessage(s *Server) error {
	msg, err := s.constructHandshakeMSG()
	if err != nil {
		return err
	}
	// Convert to a polo message and send it to the network
	return p.Send(s.GetKramaID(), ptypes.HANDSHAKEMSG, &msg)
}

func (p *Peer) handleHandshakeMessage() error {
	message, err := p.decodeHandshakeMessage()
	if err != nil {
		return err
	}
	// HashSet the id of the peer based on the message
	p.setKramaID(message.Sender)

	return nil
}

func (p *Peer) decodeHandshakeMessage() (ptypes.Message, error) {
	message, err := p.decodePeerMessage()
	if err != nil {
		return ptypes.NilMessage, errors.New("invalid message")
	}

	messageIsHandShakeMsg := message.IsHandShakeMessage()
	if !messageIsHandShakeMsg {
		return ptypes.NilMessage, errors.New("wrong message type")
	}

	return message, nil
}

func (p *Peer) resetStream() {
	if err := p.stream.Reset(); err != nil {
		p.logger.Error("stream reset", "error", err, "peerID", p)
	}
}

func (p *Peer) decodePeerMessage() (ptypes.Message, error) {
	reader := msgio.NewReader(p.rw.Reader)

	buffer, err := reader.ReadMsg()
	if err != nil {
		return ptypes.NilMessage, err
	}

	var message ptypes.Message

	err = message.FromBytes(buffer)
	if err != nil {
		return ptypes.NilMessage, err
	}

	return message, nil
}

// SetID is a method of Peer that sets the KramaID of the Peer.
// Accepts NewPeer proto from which the KramaID is generated.
func (p *Peer) setKramaID(id id.KramaID) {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()
	// Generate the ID for the peer and assign it to the field
	p.kramaID = id
}

func (p *Peer) GetKramaID() id.KramaID {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()

	return p.kramaID
}

func (p *Peer) InitHandshake(s *Server) error {
	if err := p.SendHandshakeMessage(s); err != nil {
		return err
	}

	reader := msgio.NewReader(p.rw.Reader)

	buffer, err := reader.ReadMsg()
	if err != nil {
		return err
	}

	message := new(ptypes.Message)
	if err := message.FromBytes(buffer); err != nil {
		return err
	}
	// Unmarshal message proto into a NewPeer message
	handshakeMsg := new(ptypes.HandshakeMSG)
	if err := handshakeMsg.FromBytes(message.Payload); err != nil {
		return err
	}

	if handshakeMsg.Error != "" {
		return errors.New(handshakeMsg.Error)
	}
	// HashSet the KramaID of the peer based on the message
	p.setKramaID(message.Sender)

	return nil
}

func (p *Peer) sendHandshakeErrorResp(id id.KramaID, err error) error {
	msg := &ptypes.HandshakeMSG{
		Error: err.Error(),
	}

	return p.Send(id, ptypes.HANDSHAKEMSG, msg)
}

// SendIXs is a method of Peer that emits interactions to the network.
// Accepts a slice of Interactions to emit.
func (p *Peer) SendIXs(id id.KramaID, ixs types.Interactions) error {
	// Mark the given Interactions as 'known'
	for _, ix := range ixs {
		p.markInteraction(ix.Hash())
	}

	return p.Send(id, ptypes.NEWIXSMSG, &ixs)
}

// Send is a method of Peer that writes a proto message to the respective peer's stream
// Accepts the sender id, the message type and message itself.
func (p *Peer) Send(id id.KramaID, code ptypes.MsgType, msg ptypes.MessagePayload) error {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()

	rawData, err := generateWireMessage(id, code, msg)
	if err != nil {
		return err
	}

	return shipMessage(&p.rw, rawData)
}

// markInteraction is a method of Peer that 'marks' an Interaction as known.
// Accepts the Hash of an Interaction and adds it to set of known interaction hashes.
//
// Maintains 150 known interactions in the set at any point.
func (p *Peer) markInteraction(hash types.Hash) {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()
	// Drop a previously known interaction hash if the cardinality (count)
	// of the known interaction set has exceeded the limit of 150.
	for p.knownIXs.Cardinality() >= 150 {
		p.knownIXs.Pop()
	}

	// Add the given interaction hash to set of known interactions
	p.knownIXs.Add(hash)
}

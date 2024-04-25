package p2p

import (
	"bufio"
	"context"
	"sync"

	mapset "github.com/deckarep/golang-set"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-msgio"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"

	"github.com/sarvalabs/go-moi/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

// Peer is a struct that represents a peer on the network
type Peer struct {
	kramaID   kramaid.KramaID  // Represents the KramaID of the peer
	networkID peer.ID          // Represents the libp2p-peer-id of the peer
	stream    network.Stream   // Represents peer's stream
	rtt       int64            // Represents the Round trip time of the peer
	rw        bufio.ReadWriter // Represents the peer's read/write buffer
	knownIXs  mapset.Set       // Represents the set of interactions known to the peer
	logger    hclog.Logger
	mtxLock   sync.RWMutex
}

// newPeer is a constructor function that generates and returns a Peer
// for a given kramaID, libp2p peerID and a read/write io buffer.
func newPeer(stream network.Stream, kramaID kramaid.KramaID, rtt int64, logger hclog.Logger) *Peer {
	return &Peer{
		kramaID:   kramaID,
		stream:    stream,
		networkID: stream.Conn().RemotePeer(),
		rtt:       rtt,
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
	return p.Send(s.GetKramaID(), networkmsg.HANDSHAKEMSG, msg)
}

func (p *Peer) handleHandshakeMessage() error {
	msg, err := p.decodeHandshakeMessage()
	if err != nil {
		return err
	}

	if p.kramaID == "" {
		p.kramaID = msg.Sender
	}

	return nil
}

func (p *Peer) decodeHandshakeMessage() (networkmsg.Message, error) {
	msg, err := p.decodePeerMessage()
	if err != nil {
		return networkmsg.NilMessage, errors.New("invalid message")
	}

	messageIsHandShakeMsg := msg.IsHandShakeMessage()
	if !messageIsHandShakeMsg {
		return networkmsg.NilMessage, errors.New("wrong message type")
	}

	return msg, nil
}

func (p *Peer) decodePeerMessage() (networkmsg.Message, error) {
	reader := msgio.NewReader(p.rw.Reader)

	buffer, err := reader.ReadMsg()
	if err != nil {
		return networkmsg.NilMessage, err
	}

	var msg networkmsg.Message

	err = msg.FromBytes(buffer)
	if err != nil {
		return networkmsg.NilMessage, err
	}

	return msg, nil
}

func (p *Peer) GetKramaID() kramaid.KramaID {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()

	return p.kramaID
}

func (p *Peer) GetRTT() int64 {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()

	return p.rtt
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

	message := new(networkmsg.Message)
	if err := message.FromBytes(buffer); err != nil {
		return err
	}
	// Unmarshal message proto into a NewPeer message
	handshakeMsg := new(networkmsg.HandshakeMSG)
	if err := handshakeMsg.FromBytes(message.Payload); err != nil {
		return err
	}

	return nil
}

// SendIXs ships the ixn to the peer and  marks as know
func (p *Peer) SendIXs(id kramaid.KramaID, ixs common.Interactions) error {
	// Mark the given Interactions as 'known'
	for _, ix := range ixs {
		p.markInteraction(ix.Hash())
	}

	return p.Send(id, networkmsg.NEWIXSMSG, &ixs)
}

// Send bundles the given payload into a network message and ship it to the peer
func (p *Peer) Send(id kramaid.KramaID, code networkmsg.MsgType, msg networkmsg.Payload) error {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()

	rawData, err := generateWireMessage(id, code, msg)
	if err != nil {
		return err
	}

	return shipMessage(&p.rw, rawData)
}

// markInteraction is a method of Peer that 'marks' an Interaction as known.
// Maintains 150 known interactions in the set at any point.
func (p *Peer) markInteraction(hash common.Hash) {
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

// peerMsgNonceStore implements PeerMetaDataStore interface, which stores pubsub message sequence number for each peer
// check https://pkg.go.dev/github.com/libp2p/go-libp2p-pubsub#BasicSeqnoValidator for more info
// TODO: We should consider using a persistent storage option
type peerMsgNonceStore struct {
	meta map[peer.ID][]byte
}

func newpeerMsgNonceStore() *peerMsgNonceStore {
	return &peerMsgNonceStore{
		meta: make(map[peer.ID][]byte),
	}
}

func (m *peerMsgNonceStore) Get(ctx context.Context, p peer.ID) ([]byte, error) {
	v := m.meta[p]

	return v, nil
}

func (m *peerMsgNonceStore) Put(ctx context.Context, p peer.ID, v []byte) error {
	m.meta[p] = v

	return nil
}

package poorna

import (
	"bufio"
	"errors"
	"log"
	"sync"

	"gitlab.com/sarvalabs/moichain/utils"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"

	mapset "github.com/deckarep/golang-set"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/sarvalabs/moichain/types"
)

// KipPeer is a struct that represents a peer on the KIP network
type KipPeer struct {
	// Represents the KipID of the peer
	id id.KramaID

	// Represents the peerid of the peer [libp2p]
	networkID peer.ID

	// Represents the multiaddr of the peer [libp2p]
	// ip multiaddr.Multiaddr
	mtxLock sync.RWMutex
	// Represents the peer's read/write buffer
	rw bufio.ReadWriter

	// Represents the set of interactions known to the peer
	knownIXs mapset.Set
}

// NewKIPPeer is a constructor function that generates and returns a KipPeer
// for a given libp2p peerid, multiaddr and a read/write io buffer.
func NewKIPPeer(networkID peer.ID, rw bufio.ReadWriter) *KipPeer {
	return &KipPeer{
		networkID: networkID,
		rw:        rw,
		knownIXs:  mapset.NewSet(),
	}
}

// SendID is a method of KipPeer that emits the peer's id to the network
func (p *KipPeer) SendID(id id.KramaID, ntq int32, addrs []multiaddr.Multiaddr) error {
	// Create a NewPeer proto message
	msg := types.HandshakeMSG{
		NTQ:     ntq,
		Address: utils.MultiAddrToString(addrs...),
	}
	// Convert to a proto message and send it to the network

	return p.Send(id, types.HANDSHAKEMSG, msg)
}

// SetID is a method of KipPeer that sets the KipID of the KipPeer.
// Accepts NewPeer proto from which the KipID is generated.
func (p *KipPeer) setID(id id.KramaID) {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()
	// Generate the KipID for the peer and assign it to the field
	p.id = id
}

func (p *KipPeer) InitHandshake(id id.KramaID, ntq int32, addrs []multiaddr.Multiaddr) error {
	if err := p.SendID(id, ntq, addrs); err != nil {
		return err
	}

	buffer := make([]byte, 4096)

	byteCount, err := p.rw.Reader.Read(buffer)
	if err != nil {
		log.Println(err)

		return err
	}

	message := new(types.Message)
	if err = polo.Depolorize(message, buffer[0:byteCount]); err != nil {
		return err
	}
	// Unmarshal message proto into a NewPeer message
	var msg types.HandshakeMSG
	if err := polo.Depolorize(&msg, message.Payload); err != nil {
		return err
	}

	if msg.Error != "" {
		return errors.New(msg.Error)
	}
	// HashSet the KIP id of the peer based on the message
	p.setID(message.Sender)

	return nil
}

func (p *KipPeer) sendHandshakeErrorResp(id id.KramaID, err error) error {
	msg := types.HandshakeMSG{
		Error: err.Error(),
	}

	return p.Send(id, types.HANDSHAKEMSG, msg)
}

// SendIXs is a method of KipPeer that emits interactions to the network.
// Accepts a slice of Interactions to emit.
func (p *KipPeer) SendIXs(id id.KramaID, ixs types.Interactions) error {
	// Mark the given Interactions as 'known'
	for _, j := range ixs {
		p.markInteraction(j.GetIxHash())
	}

	msg := types.InteractionMsg{Ixs: ixs}

	return p.Send(id, types.NEWIXSMSG, msg)
}

// Send is a method of KipPeer that emits an arbitrary proto message to the network
// Accepts the sender id, the message type and message itself.
func (p *KipPeer) Send(id id.KramaID, code types.MsgType, msg interface{}) error {
	p.mtxLock.Lock()
	defer p.mtxLock.Unlock()

	// Marshal the proto message into slice of bytes and log and return if an error occurs
	bytes := polo.Polorize(msg)

	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it  into a slice of bytes
	m := types.Message{
		MsgType: code,
		Payload: bytes,
		Sender:  id,
	}

	bytes = polo.Polorize(&m)

	// Write the message bytes into the peer's iobuffer
	if _, err := p.rw.Writer.Write(bytes); err != nil {
		return err
	}

	// Flush the peer's iobuffer. This will push the message to the network

	return p.rw.Flush()
}

// markInteraction is a method of KipPeer that 'marks' an Interaction as known.
// Accepts the Hash of an Interaction and adds it to set of known interaction hashes.
//
// Maintains 150 known interactions in the set at any point.
func (p *KipPeer) markInteraction(hash types.Hash) {
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

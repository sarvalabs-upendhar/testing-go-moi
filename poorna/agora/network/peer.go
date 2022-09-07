package network

import (
	"bufio"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"log"
	"sync"
	"time"
)

type AgoraPeer struct {
	mtx            sync.Mutex
	id             peer.ID
	stream         network.Stream
	connected      bool
	lastActiveTime time.Time
	activeSessions map[ktypes.Address]struct{}
}

func (a *AgoraPeer) getActiveSessions() []ktypes.Address {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	sessions := make([]ktypes.Address, 0, len(a.activeSessions))

	for sessionID := range a.activeSessions {
		sessions = append(sessions, sessionID)
	}

	return sessions
}

func (a *AgoraPeer) addActiveSession(sessionID ktypes.Address) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.activeSessions[sessionID] = struct{}{}
}

func (a *AgoraPeer) removeActiveSession(sessionID ktypes.Address) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	delete(a.activeSessions, sessionID)
}

func (a *AgoraPeer) updateLastActiveTime() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.lastActiveTime = time.Now()
}

func (a *AgoraPeer) sendMessage(senderID id.KramaID, msgType ktypes.MsgType, msg interface{}) error {
	rw := bufio.NewReadWriter(bufio.NewReader(a.stream), bufio.NewWriter(a.stream))
	// Marshal the proto message into slice of bytes and log and return if an error occurs
	bytes := polo.Polorize(msg)

	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it  into a slice of bytes
	m := ktypes.Message{
		MsgType: msgType,
		Payload: bytes,
		Sender:  senderID,
	}

	bytes = polo.Polorize(&m)

	// Write the message bytes into the peer's io buffer
	if _, err := rw.Writer.Write(bytes); err != nil {
		return err
	}

	// Flush the peer's io buffer. This will push the message to the network

	return rw.Flush()
}

func (a *AgoraPeer) Close() {
	if err := a.stream.Reset(); err != nil {
		log.Println("Error closing stream")
	}
}

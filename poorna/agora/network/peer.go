package network

import (
	"bufio"
	"log"
	"sync"
	"time"

	"github.com/pkg/errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

type AgoraPeer struct {
	mtx            sync.Mutex
	id             peer.ID
	stream         network.Stream
	connected      bool
	lastActiveTime time.Time
	activeSessions map[types.Address]struct{}
}

func (a *AgoraPeer) getActiveSessions() []types.Address {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	sessions := make([]types.Address, 0, len(a.activeSessions))

	for sessionID := range a.activeSessions {
		sessions = append(sessions, sessionID)
	}

	return sessions
}

func (a *AgoraPeer) addActiveSession(sessionID types.Address) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.activeSessions[sessionID] = struct{}{}
}

func (a *AgoraPeer) removeActiveSession(sessionID types.Address) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	delete(a.activeSessions, sessionID)
}

func (a *AgoraPeer) updateLastActiveTime() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.lastActiveTime = time.Now()
}

func (a *AgoraPeer) sendMessage(senderID id.KramaID, msgType ptypes.MsgType, msg interface{}) error {
	rw := bufio.NewReadWriter(bufio.NewReader(a.stream), bufio.NewWriter(a.stream))
	// Marshal the proto message into slice of bytes and log and return if an error occurs
	rawData, err := polo.Polorize(msg)
	if err != nil {
		return errors.Wrap(err, "failed to polorize message payload")
	}

	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it  into a slice of bytes
	m := ptypes.Message{
		MsgType: msgType,
		Payload: rawData,
		Sender:  senderID,
	}

	rawData, err = m.Bytes()
	if err != nil {
		return err
	}

	// Write the message bytes into the peer's io buffer
	if _, err := rw.Writer.Write(rawData); err != nil {
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

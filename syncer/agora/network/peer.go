package network

import (
	"bufio"
	"log"
	"sync"
	"time"

	p2pnet "github.com/libp2p/go-libp2p/core/network"
	id "github.com/sarvalabs/moichain/common/kramaid"
	"github.com/sarvalabs/moichain/network/message"

	"github.com/libp2p/go-msgio"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/common"
)

type AgoraPeer struct {
	mtx            sync.Mutex
	id             peer.ID
	stream         p2pnet.Stream
	connected      bool
	lastActiveTime time.Time
	activeSessions map[common.Address]struct{}
}

func (a *AgoraPeer) getActiveSessions() []common.Address {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	sessions := make([]common.Address, 0, len(a.activeSessions))

	for sessionID := range a.activeSessions {
		sessions = append(sessions, sessionID)
	}

	return sessions
}

func (a *AgoraPeer) addActiveSession(sessionID common.Address) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.activeSessions[sessionID] = struct{}{}
}

func (a *AgoraPeer) removeActiveSession(sessionID common.Address) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	delete(a.activeSessions, sessionID)
}

func (a *AgoraPeer) updateLastActiveTime() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.lastActiveTime = time.Now()
}

func (a *AgoraPeer) sendMessage(senderID id.KramaID, msgType message.MsgType, msg interface{}) error {
	// Marshal the proto message into slice of bytes and log and return if an error occurs
	rawData, err := polo.Polorize(msg)
	if err != nil {
		return errors.Wrap(err, "failed to polorize message payload")
	}

	// Create a network message proto with the bytes payload of the message to send
	// and convert into a proto message and marshal it  into a slice of bytes
	m := message.Message{
		MsgType: msgType,
		Payload: rawData,
		Sender:  senderID,
	}

	rawData, err = m.Bytes()
	if err != nil {
		return err
	}

	wr := bufio.NewWriter(a.stream)
	// Write the message bytes into the peer's io buffer
	writer := msgio.NewWriter(wr)
	if err := writer.WriteMsg(rawData); err != nil {
		return err
	}

	return wr.Flush()
}

func (a *AgoraPeer) Close() {
	if err := a.stream.Reset(); err != nil {
		log.Println("Error closing stream")
	}
}

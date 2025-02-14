package network

import (
	"bufio"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	p2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-msgio"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/network/p2p"
)

type AgoraPeer struct {
	mtx            sync.Mutex
	id             peer.ID
	stream         p2pnet.Stream
	connected      bool
	lastActiveTime time.Time
	activeSessions map[identifiers.Identifier]struct{}
}

func (a *AgoraPeer) getActiveSessions() []identifiers.Identifier {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	sessions := make([]identifiers.Identifier, 0, len(a.activeSessions))

	for sessionID := range a.activeSessions {
		sessions = append(sessions, sessionID)
	}

	return sessions
}

func (a *AgoraPeer) addActiveSession(sessionID identifiers.Identifier) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.activeSessions[sessionID] = struct{}{}
}

func (a *AgoraPeer) removeActiveSession(sessionID identifiers.Identifier) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	delete(a.activeSessions, sessionID)
}

func (a *AgoraPeer) updateLastActiveTime() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.lastActiveTime = time.Now()
}

func (a *AgoraPeer) sendMessage(senderID kramaid.KramaID, msgType message.MsgType, msg interface{}) error {
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

func (a *AgoraPeer) Close(connManager *p2p.ConnectionManager) error {
	if err := connManager.ResetStream(a.stream, p2p.AgoraStreamTag); err != nil {
		return err
	}

	return nil
}

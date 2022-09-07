package network

import (
	"bufio"
	"context"
	"errors"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/polo/go-polo"
	"sync"
	"time"
)

const (
	AgoraStreamProtocol = protocol.ID("moi/stream/agora")
)

type SessionManager interface {
	HandlePeerMessage(id id.KramaID, msg interface{})
	PeerDisconnected(sessions []ktypes.Address, id peer.ID)
}

type AgoraNetwork struct {
	ctx    context.Context
	logger hclog.Logger
	peers  sync.Map
	server *poorna.Server
	sm     SessionManager
}

func NewAgoraNetwork(ctx context.Context, logger hclog.Logger, server *poorna.Server) *AgoraNetwork {
	an := &AgoraNetwork{
		ctx:    ctx,
		logger: logger.Named("Network"),
		server: server,
	}

	server.SetupStreamHandler(AgoraStreamProtocol, an.streamHandler)

	return an
}

func (an *AgoraNetwork) streamHandler(stream network.Stream) {
	agoraPeer := &AgoraPeer{
		id:             stream.Conn().RemotePeer(),
		stream:         stream,
		connected:      true,
		activeSessions: make(map[ktypes.Address]struct{}),
	}

	an.peers.Store(agoraPeer.id, agoraPeer)

	go an.handlePeerMessages(agoraPeer)
}

func (an *AgoraNetwork) handlePeerMessages(peer *AgoraPeer) {
	defer func() {
		if err := peer.stream.Reset(); err != nil {
			an.logger.Error("Error closing stream", "peer", peer.id)
		}

		an.peers.Delete(peer.id)
		an.sm.PeerDisconnected(peer.getActiveSessions(), peer.id)
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(peer.stream), bufio.NewWriter(peer.stream))

	for {
		buffer := make([]byte, 4096)

		byteCount, err := rw.Reader.Read(buffer)
		if err != nil {
			an.logger.Error("Error reading data from stream", "error", err)

			return
		}

		// Unmarshal the buffer into a proto message
		message := new(ktypes.Message)
		if err = polo.Depolorize(message, buffer[0:byteCount]); err != nil {
			an.logger.Error("Error reading data from stream", "error", err)

			return
		}

		peer.updateLastActiveTime() // check if remote peer can spam with invalid messages

		switch message.MsgType {
		case ktypes.AGORAREQ:
			reqMsg := new(types.AgoraRequestMsg)
			if err := polo.Depolorize(reqMsg, message.Payload); err != nil {
				an.logger.Error("Error depolarising agora message")

				continue
			}

			an.sm.HandlePeerMessage(message.Sender, reqMsg)

		case ktypes.AGORARESP:
			respMsg := new(types.AgoraResponseMsg)
			if err := polo.Depolorize(respMsg, message.Payload); err != nil {
				an.logger.Error("Error depolarising agora message")

				continue
			}

			an.sm.HandlePeerMessage(message.Sender, respMsg)
		}
	}
}

func (an *AgoraNetwork) SendAgoraMessage(id id.KramaID, msgType ktypes.MsgType, msg types.Message) error {
	peerID, err := kutils.GetNetworkID(id)
	if err != nil {
		an.logger.Error("Unable to decode peer id", "error", err)

		return err
	}

	peer, ok := an.peers.Load(peerID)
	if !ok {
		stream, err := an.server.NewStream(context.Background(), AgoraStreamProtocol, peerID)
		if err != nil {
			return err
		}

		peer = &AgoraPeer{
			activeSessions: map[ktypes.Address]struct{}{msg.GetSessionID(): {}},
			id:             peerID,
			stream:         stream,
			connected:      true,
		}

		an.peers.Store(peerID, peer)

		agoraPeer, ok := peer.(*AgoraPeer)
		if !ok {
			return ktypes.ErrInterfaceConversion
		}

		go an.handlePeerMessages(agoraPeer)
	}

	agoraPeer, ok := peer.(*AgoraPeer)
	if !ok {
		return ktypes.ErrInterfaceConversion
	}

	if err := agoraPeer.sendMessage(an.server.GetKramaID(), msgType, msg); err != nil {
		return err
	}

	agoraPeer.addActiveSession(msg.GetSessionID())
	agoraPeer.updateLastActiveTime()

	return nil
}

func (an *AgoraNetwork) pruneInactivePeers() {
	for {
		select {
		case <-an.ctx.Done():
		case <-time.After(1 * time.Second):
		}

		an.peers.Range(func(key, value interface{}) bool {
			agoraPeer, ok := value.(*AgoraPeer)
			if !ok {
				return false
			}
			agoraPeer.mtx.Lock()
			defer agoraPeer.mtx.Unlock()

			if len(agoraPeer.activeSessions) == 0 && time.Since(agoraPeer.lastActiveTime) > 1*time.Second {
				an.logger.Info("Pruning Inactive peer ", "id", agoraPeer.id)

				// cancel the receiving routine
				agoraPeer.Close()

				// cleanup the memory
				an.peers.Delete(key)
			}

			return true
		})
	}
}

func (an *AgoraNetwork) ClosePeerSession(kramaID id.KramaID, sessionID ktypes.Address) error {
	peerID, err := kutils.GetNetworkID(kramaID)
	if err != nil {
		an.logger.Error("Error parsing krama id", kramaID)

		return err
	}

	p, ok := an.peers.Load(peerID)
	if !ok {
		return errors.New("p not found")
	}

	agoraPeer, ok := p.(*AgoraPeer)
	if !ok {
		return ktypes.ErrInterfaceConversion
	}

	agoraPeer.removeActiveSession(sessionID)

	return nil
}

func (an *AgoraNetwork) Start(sm SessionManager) {
	an.sm = sm

	go an.pruneInactivePeers()
}

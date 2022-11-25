package network

import (
	"bufio"
	"context"
	"errors"
	"sync"
	"time"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
)

const (
	AgoraStreamProtocol = protocol.ID("moi/stream/agora")
)

type SessionManager interface {
	HandlePeerMessage(id id.KramaID, msg interface{})
	PeerDisconnected(sessions []types.Address, id peer.ID)
}

type AgoraNetwork struct {
	ctx     context.Context
	logger  hclog.Logger
	peers   sync.Map
	server  *poorna.Server
	sm      SessionManager
	metrics *Metrics
}

func NewAgoraNetwork(ctx context.Context, logger hclog.Logger, server *poorna.Server, metrics *Metrics) *AgoraNetwork {
	an := &AgoraNetwork{
		ctx:     ctx,
		logger:  logger.Named("Network"),
		server:  server,
		metrics: metrics,
	}

	server.SetupStreamHandler(AgoraStreamProtocol, an.streamHandler)

	return an
}

func (an *AgoraNetwork) streamHandler(stream network.Stream) {
	agoraPeer := &AgoraPeer{
		id:             stream.Conn().RemotePeer(),
		stream:         stream,
		connected:      true,
		activeSessions: make(map[types.Address]struct{}),
	}

	an.peers.Store(agoraPeer.id, agoraPeer)

	go an.handlePeerMessages(agoraPeer)
}

func (an *AgoraNetwork) handlePeerMessages(peer *AgoraPeer) {
	defer func() {
		if err := peer.stream.Reset(); err != nil {
			an.logger.Info("Closed stream", "peer", peer.id)
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
		message := new(ptypes.Message)
		if err = polo.Depolorize(message, buffer[0:byteCount]); err != nil {
			an.logger.Error("Error reading data from stream", "error", err)

			return
		}

		peer.updateLastActiveTime() // check if remote peer can spam with invalid messages

		switch message.MsgType {
		case ptypes.AGORAREQ:
			reqMsg := new(atypes.AgoraRequestMsg)
			if err := polo.Depolorize(reqMsg, message.Payload); err != nil {
				an.logger.Error("Error depolarising agora message")

				continue
			}

			an.metrics.captureInboundDataSize(float64(len(message.Payload)))
			an.sm.HandlePeerMessage(message.Sender, reqMsg)

		case ptypes.AGORARESP:
			respMsg := new(atypes.AgoraResponseMsg)
			if err := polo.Depolorize(respMsg, message.Payload); err != nil {
				an.logger.Error("Error depolarising agora message")

				continue
			}

			an.metrics.captureOutboundDataSize(float64(len(message.Payload)))
			an.sm.HandlePeerMessage(message.Sender, respMsg)
		}
	}
}

func (an *AgoraNetwork) SendAgoraMessage(id id.KramaID, msgType ptypes.MsgType, msg atypes.Message) error {
	peerID, err := utils.GetNetworkID(id)
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
			activeSessions: map[types.Address]struct{}{msg.GetSessionID(): {}},
			id:             peerID,
			stream:         stream,
			connected:      true,
		}

		an.peers.Store(peerID, peer)

		agoraPeer, ok := peer.(*AgoraPeer)
		if !ok {
			return types.ErrInterfaceConversion
		}

		go an.handlePeerMessages(agoraPeer)
	}

	agoraPeer, ok := peer.(*AgoraPeer)
	if !ok {
		return types.ErrInterfaceConversion
	}

	if err := agoraPeer.sendMessage(an.server.GetKramaID(), msgType, msg); err != nil {
		return err
	}

	agoraPeer.addActiveSession(msg.GetSessionID())
	agoraPeer.updateLastActiveTime()
	an.metrics.captureActiveConnections(1)

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

func (an *AgoraNetwork) ClosePeerSession(kramaID id.KramaID, sessionID types.Address) error {
	peerID, err := utils.GetNetworkID(kramaID)
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
		return types.ErrInterfaceConversion
	}

	agoraPeer.removeActiveSession(sessionID)
	an.metrics.captureActiveConnections(-1)

	return nil
}

func (an *AgoraNetwork) Start(sm SessionManager) {
	an.sm = sm

	an.metrics.initMetrics()

	go an.pruneInactivePeers()
}

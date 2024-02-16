package network

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	p2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-msgio"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
)

type SessionManager interface {
	HandlePeerMessage(id kramaid.KramaID, msg interface{})
	PeerDisconnected(sessions []identifiers.Address, id peer.ID)
}

type AgoraNetwork struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    hclog.Logger
	peers     sync.Map
	server    *p2p.Server
	sm        SessionManager
	metrics   *Metrics
}

func NewAgoraNetwork(logger hclog.Logger, server *p2p.Server, metrics *Metrics) *AgoraNetwork {
	ctx, cancel := context.WithCancel(context.Background())

	an := &AgoraNetwork{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger.Named("Agora-Network"),
		server:    server,
		metrics:   metrics,
	}

	server.ConnManager.SetupStreamHandler(config.AgoraProtocolStream, p2p.AgoraStreamTag, an.streamHandler)

	return an
}

func (an *AgoraNetwork) streamHandler(stream p2pnet.Stream) {
	agoraPeer := &AgoraPeer{
		id:             stream.Conn().RemotePeer(),
		stream:         stream,
		connected:      true,
		activeSessions: make(map[identifiers.Address]struct{}),
	}

	an.peers.Store(agoraPeer.id, agoraPeer)

	go an.handlePeerMessages(agoraPeer)
}

func (an *AgoraNetwork) handlePeerMessages(peer *AgoraPeer) {
	defer func() {
		if err := an.server.ConnManager.ResetStream(peer.stream, p2p.AgoraStreamTag); err != nil {
			an.logger.Info("Failed to close stream", "err", err, "peer-id", peer.id)
		}

		an.peers.Delete(peer.id)
		an.sm.PeerDisconnected(peer.getActiveSessions(), peer.id)
	}()

	reader := msgio.NewReader(peer.stream)

	for {
		buffer, err := reader.ReadMsg()
		if err != nil {
			return
		}

		// Unmarshal the buffer into a proto message
		msg := new(networkmsg.Message)
		if err = msg.FromBytes(buffer); err != nil {
			an.logger.Error("Failed to decode message", "err", err)

			return
		}

		peer.updateLastActiveTime() // check if remote peer can spam with invalid messages

		an.metrics.captureInboundDataSize(float64(len(msg.Payload)))

		switch msg.MsgType {
		case networkmsg.AGORAREQ:
			reqMsg := new(message.AgoraRequestMsg)
			if err = reqMsg.FromBytes(msg.Payload); err != nil {
				an.logger.Error("Failed to depolorize agora request message", "err", err)

				continue
			}

			an.sm.HandlePeerMessage(msg.Sender, reqMsg)

		case networkmsg.AGORARESP:
			respMsg := new(message.AgoraResponseMsg)
			if err = respMsg.FromBytes(msg.Payload); err != nil {
				an.logger.Error("Failed to depolorize agora response message", "err", err)

				continue
			}

			an.sm.HandlePeerMessage(msg.Sender, respMsg)
		}
	}
}

func (an *AgoraNetwork) SendAgoraMessage(id kramaid.KramaID, msgType networkmsg.MsgType, msg message.Message) error {
	peerID, err := utils.GetNetworkID(id)
	if err != nil {
		an.logger.Error("Unable to decode peer ID", "err", err)

		return err
	}

	abstractPeer, ok := an.peers.Load(peerID)
	if !ok {
		if _, err := an.server.ConnManager.GetPeerInfo(peerID); err != nil {
			return err
		}

		stream, err := an.server.ConnManager.NewStream(
			context.Background(),
			peerID,
			config.AgoraProtocolStream,
			p2p.AgoraStreamTag,
		)
		if err != nil {
			return err
		}

		abstractPeer = &AgoraPeer{
			activeSessions: map[identifiers.Address]struct{}{msg.GetSessionID(): {}},
			id:             peerID,
			stream:         stream,
			connected:      true,
		}

		an.peers.Store(peerID, abstractPeer)

		agoraPeer, ok := abstractPeer.(*AgoraPeer)
		if !ok {
			return common.ErrInterfaceConversion
		}

		go an.handlePeerMessages(agoraPeer)
	}

	agoraPeer, ok := abstractPeer.(*AgoraPeer)
	if !ok {
		return common.ErrInterfaceConversion
	}

	if err := agoraPeer.sendMessage(an.server.GetKramaID(), msgType, msg); err != nil {
		return err
	}

	rawData, err := polo.Polorize(msg)
	if err != nil {
		return err
	}

	an.metrics.captureOutboundDataSize(float64(len(rawData)))

	agoraPeer.addActiveSession(msg.GetSessionID())
	agoraPeer.updateLastActiveTime()
	an.metrics.captureActiveConnections(1)

	return nil
}

func (an *AgoraNetwork) pruneInactivePeers() {
	for {
		select {
		case <-an.ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}

		an.peers.Range(func(key, value interface{}) bool {
			agoraPeer, ok := value.(*AgoraPeer)
			if !ok {
				return false
			}
			agoraPeer.mtx.Lock()
			defer agoraPeer.mtx.Unlock()

			if len(agoraPeer.activeSessions) == 0 && time.Since(agoraPeer.lastActiveTime) > 3*time.Second {
				an.logger.Info("Pruning inactive peer", "peer-ID", agoraPeer.id)

				// cancel the receiving routine
				if err := agoraPeer.Close(an.server.ConnManager); err != nil {
					an.logger.Info("Failed to close the stream", "err", err)
				}

				// cleanup the memory
				an.peers.Delete(key)
			}

			return true
		})
	}
}

func (an *AgoraNetwork) ClosePeerSession(kramaID kramaid.KramaID, sessionID identifiers.Address) error {
	peerID, err := utils.GetNetworkID(kramaID)
	if err != nil {
		an.logger.Error("Error parsing krama ID", "krama-ID", kramaID)

		return err
	}

	p, ok := an.peers.Load(peerID)
	if !ok {
		return errors.New("p not found")
	}

	agoraPeer, ok := p.(*AgoraPeer)
	if !ok {
		return common.ErrInterfaceConversion
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

func (an *AgoraNetwork) Close() {
	an.logger.Info("Closing Agora-Network")
	an.ctxCancel()
}

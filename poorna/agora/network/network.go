package network

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sarvalabs/go-polo"

	"github.com/libp2p/go-msgio"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/moichain/common"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
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
		logger:  logger.Named("Agora-Network"),
		server:  server,
		metrics: metrics,
	}

	server.SetupStreamHandler(common.AgoraProtocolStream, an.streamHandler)

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
			an.logger.Info("Closed stream", "peer-ID", peer.id)
		}

		an.peers.Delete(peer.id)
		an.sm.PeerDisconnected(peer.getActiveSessions(), peer.id)
	}()

	reader := msgio.NewReader(peer.stream)

	for {
		buffer, err := reader.ReadMsg()
		if err != nil {
			an.logger.Error("Error reading data from stream", "err", err)

			return
		}

		// Unmarshal the buffer into a proto message
		message := new(ptypes.Message)
		if err := message.FromBytes(buffer); err != nil {
			an.logger.Error("Error reading data from stream", "err", err)

			return
		}

		peer.updateLastActiveTime() // check if remote peer can spam with invalid messages

		an.metrics.captureInboundDataSize(float64(len(message.Payload)))

		switch message.MsgType {
		case ptypes.AGORAREQ:
			reqMsg := new(atypes.AgoraRequestMsg)
			if err := reqMsg.FromBytes(message.Payload); err != nil {
				an.logger.Error("Error depolarising agora message", "err", err)

				continue
			}

			an.sm.HandlePeerMessage(message.Sender, reqMsg)

		case ptypes.AGORARESP:
			respMsg := new(atypes.AgoraResponseMsg)
			if err := respMsg.FromBytes(message.Payload); err != nil {
				an.logger.Error("Error depolarising agora message", "err", err)

				continue
			}

			an.sm.HandlePeerMessage(message.Sender, respMsg)
		}
	}
}

func (an *AgoraNetwork) SendAgoraMessage(id id.KramaID, msgType ptypes.MsgType, msg atypes.Message) error {
	peerID, err := utils.GetNetworkID(id)
	if err != nil {
		an.logger.Error("Unable to decode peer ID", "err", err)

		return err
	}

	abstractPeer, ok := an.peers.Load(peerID)
	if !ok {
		if _, err := an.server.GetPeerInfo(peerID); err != nil {
			return err
		}

		stream, err := an.server.NewStream(context.Background(), peerID, common.AgoraProtocolStream)
		if err != nil {
			return err
		}

		abstractPeer = &AgoraPeer{
			activeSessions: map[types.Address]struct{}{msg.GetSessionID(): {}},
			id:             peerID,
			stream:         stream,
			connected:      true,
		}

		an.peers.Store(peerID, abstractPeer)

		agoraPeer, ok := abstractPeer.(*AgoraPeer)
		if !ok {
			return types.ErrInterfaceConversion
		}

		go an.handlePeerMessages(agoraPeer)
	}

	agoraPeer, ok := abstractPeer.(*AgoraPeer)
	if !ok {
		return types.ErrInterfaceConversion
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
				an.logger.Info("Pruning inactive peer", "peer-ID", agoraPeer.id)

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
		an.logger.Error("Error parsing krama ID", "krama-ID", kramaID)

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

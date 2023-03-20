package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/agora/decision"
	"github.com/sarvalabs/moichain/poorna/agora/network"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	"github.com/sarvalabs/moichain/types"
)

const MaxRounds = 3

type engine interface {
	HandleRequest(req *decision.Request)
}

type interestManager interface {
	InterestedSessions(blocks []atypes.Block) (map[types.Address][]atypes.Block, []atypes.Block)
	RecordSessionInterest(addr types.Address, ids ...atypes.CID)
	RemoveSessionInterest(addr types.Address, ids ...atypes.CID) []atypes.CID
}

type Manager struct {
	logger         hclog.Logger
	activeSessions sync.Map
	im             interestManager
	notifier       atypes.PubSub
	engine         engine
}

func NewSessionManager(logger hclog.Logger, im interestManager, notifier atypes.PubSub, engine engine) *Manager {
	return &Manager{
		logger:   logger,
		im:       im,
		notifier: notifier,
		engine:   engine,
	}
}

func (s *Manager) GetSession(addrs types.Address) (*Session, error) {
	session, ok := s.activeSessions.Load(addrs)
	if !ok {
		return nil, errors.New("session not found")
	}

	agoraSession, ok := session.(*Session)
	if !ok {
		return nil, types.ErrInterfaceConversion
	}

	return agoraSession, nil
}

func (s *Manager) NewSession(
	ctx context.Context,
	addrs types.Address,
	stateHash atypes.CID,
	network *network.AgoraNetwork,
	contextPeers []id.KramaID,
) (*Session, error) {
	_, ok := s.activeSessions.Load(addrs)
	if !ok {
		session := NewSession(ctx, addrs, s.logger, stateHash, network, s.notifier, s.im, s, contextPeers)
		s.activeSessions.Store(addrs, session)

		return session, nil
	}

	return nil, errors.New("session already exists")
}

func (s *Manager) HandlePeerMessage(id id.KramaID, msg interface{}) {
	switch msg := msg.(type) {
	case *atypes.AgoraRequestMsg:
		s.handleAgoraRequestMsg(id, msg)
	case *atypes.AgoraResponseMsg:
		s.handleAgoraResponseMsg(id, msg)
	default:
		s.logger.Error("invalid message type")
	}
}

func (s *Manager) handleAgoraRequestMsg(id id.KramaID, msg *atypes.AgoraRequestMsg) {
	s.engine.HandleRequest(&decision.Request{
		PeerID:    id,
		ReqTime:   time.Now(),
		SessionID: msg.SessionID,
		StateHash: msg.StateHash,
		WantList:  msg.WantList,
	})
}

func (s *Manager) handleAgoraResponseMsg(id id.KramaID, msg *atypes.AgoraResponseMsg) {
	if !msg.Status {
		session, err := s.GetSession(msg.SessionID)
		if err != nil {
			s.logger.Error("Error session not found", "error", err)
		}

		session.HandleMessage(id, msg)

		return
	}

	sessions, _ := s.im.InterestedSessions(msg.GetBlocks()) // TODO: Add orphans to cache

	for addr, blocks := range sessions {
		session, err := s.GetSession(addr)
		if err != nil {
			s.logger.Error("Error agora session not found", "addr", addr)

			continue
		}

		for _, block := range blocks {
			session.notifier.Publish(block)
		}
	}
}

func (s *Manager) PeerDisconnected(sessions []types.Address, peerID peer.ID) {
	for _, sessionID := range sessions {
		if session, ok := s.activeSessions.Load(sessionID); ok {
			session, ok := session.(*Session)
			if ok {
				session.PeerDisconnected(peerID)
			}
		}
	}
}

func (s *Manager) CloseSession(sessionID types.Address) {
	s.activeSessions.Delete(sessionID)
}

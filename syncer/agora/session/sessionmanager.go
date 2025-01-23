package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/agora/decision"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
	"github.com/sarvalabs/go-moi/syncer/agora/network"
	"github.com/sarvalabs/go-moi/syncer/agora/notifications"
	"github.com/sarvalabs/go-moi/syncer/cid"

	"github.com/sarvalabs/go-moi/common"
)

const MaxRounds = 3

type engine interface {
	HandleRequest(req *decision.Request)
}

type interestManager interface {
	InterestedSessions(blocks []block.Block) (map[identifiers.Identifier][]block.Block, []block.Block)
	RecordSessionInterest(id identifiers.Identifier, ids ...cid.CID)
	RemoveSessionInterest(id identifiers.Identifier, ids ...cid.CID) []cid.CID
}

type Manager struct {
	logger         hclog.Logger
	activeSessions sync.Map
	im             interestManager
	notifier       notifications.PubSubNotifier
	engine         engine
}

func NewSessionManager(
	logger hclog.Logger,
	im interestManager,
	notifier notifications.PubSubNotifier,
	engine engine,
) *Manager {
	return &Manager{
		logger:   logger.Named("Session-Manager"),
		im:       im,
		notifier: notifier,
		engine:   engine,
	}
}

func (s *Manager) GetSession(id identifiers.Identifier) (*Session, error) {
	session, ok := s.activeSessions.Load(id)
	if !ok {
		return nil, errors.New("session not found")
	}

	agoraSession, ok := session.(*Session)
	if !ok {
		return nil, common.ErrInterfaceConversion
	}

	return agoraSession, nil
}

func (s *Manager) NewSession(
	ctx context.Context,
	id identifiers.Identifier,
	stateHash cid.CID,
	network *network.AgoraNetwork,
	contextPeers []kramaid.KramaID,
) (*Session, error) {
	_, ok := s.activeSessions.Load(id)
	if !ok {
		session := NewSession(ctx, id, s.logger, stateHash, network, s.notifier, s.im, s, contextPeers)
		s.activeSessions.Store(id, session)

		return session, nil
	}

	return nil, errors.New("session already exists")
}

func (s *Manager) HandlePeerMessage(id kramaid.KramaID, msg interface{}) {
	switch msg := msg.(type) {
	case *message.AgoraRequestMsg:
		s.handleAgoraRequestMsg(id, msg)
	case *message.AgoraResponseMsg:
		s.handleAgoraResponseMsg(id, msg)
	default:
		s.logger.Error("Invalid message type")
	}
}

func (s *Manager) handleAgoraRequestMsg(id kramaid.KramaID, msg *message.AgoraRequestMsg) {
	s.engine.HandleRequest(&decision.Request{
		PeerID:    id,
		ReqTime:   time.Now(),
		SessionID: msg.SessionID,
		StateHash: msg.StateHash,
		WantList:  msg.WantList,
	})
}

func (s *Manager) handleAgoraResponseMsg(id kramaid.KramaID, msg *message.AgoraResponseMsg) {
	if !msg.Status {
		session, err := s.GetSession(msg.SessionID)
		if err != nil {
			s.logger.Error("Error session not found", "err", err)

			return
		}

		session.HandleMessage(id, msg)

		return
	}

	sessions, _ := s.im.InterestedSessions(msg.GetBlocks()) // TODO: Add orphans to cache

	for id, blocks := range sessions {
		session, err := s.GetSession(id)
		if err != nil {
			s.logger.Error("Error agora session not found", "id", id)

			continue
		}

		for _, blk := range blocks {
			session.notifier.Publish(blk)
		}
	}
}

func (s *Manager) PeerDisconnected(sessions []identifiers.Identifier, peerID peer.ID) {
	for _, sessionID := range sessions {
		if session, ok := s.activeSessions.Load(sessionID); ok {
			session, ok := session.(*Session)
			if ok {
				session.PeerDisconnected(peerID)
			}
		}
	}
}

func (s *Manager) CloseSession(sessionID identifiers.Identifier) {
	s.activeSessions.Delete(sessionID)
}

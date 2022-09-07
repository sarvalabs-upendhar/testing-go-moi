package session

import (
	"context"
	"errors"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/decision"
	"gitlab.com/sarvalabs/moichain/poorna/agora/network"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"sync"
	"time"
)

const MaxRounds = 3

type engine interface {
	HandleRequest(req *decision.Request)
}

type interestManager interface {
	InterestedSessions(blocks []types.Block) (map[ktypes.Address][]types.Block, []types.Block)
	RecordSessionInterest(addr ktypes.Address, ids ...ktypes.Hash)
	RemoveSessionInterest(addr ktypes.Address, ids ...ktypes.Hash) []ktypes.Hash
}

type SessionManager struct {
	logger         hclog.Logger
	activeSessions sync.Map
	im             interestManager
	notifier       types.PubSub
	engine         engine
}

func NewSessionManager(logger hclog.Logger, im interestManager, notifier types.PubSub, engine engine) *SessionManager {
	return &SessionManager{
		logger:   logger,
		im:       im,
		notifier: notifier,
		engine:   engine,
	}
}

func (s *SessionManager) GetSession(addrs ktypes.Address) (*Session, error) {
	session, ok := s.activeSessions.Load(addrs)
	if !ok {
		return nil, errors.New("session not found")
	}

	agoraSession, ok := session.(*Session)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return agoraSession, nil
}

func (s *SessionManager) NewSession(
	ctx context.Context,
	addrs ktypes.Address,
	stateHash ktypes.Hash,
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

func (s *SessionManager) HandlePeerMessage(id id.KramaID, msg interface{}) {
	switch msg := msg.(type) {
	case *types.AgoraRequestMsg:
		s.handleAgoraRequestMsg(id, msg)
	case *types.AgoraResponseMsg:
		s.handleAgoraResponseMsg(id, msg)
	default:
		s.logger.Error("invalid message type")
	}
}

func (s *SessionManager) handleAgoraRequestMsg(id id.KramaID, msg *types.AgoraRequestMsg) {
	s.engine.HandleRequest(&decision.Request{
		PeerID:    id,
		ReqTime:   time.Now(),
		SessionID: msg.SessionID,
		StateHash: msg.StateHash,
		WantList:  msg.WantList,
	})
}

func (s *SessionManager) handleAgoraResponseMsg(id id.KramaID, msg *types.AgoraResponseMsg) {
	if !msg.Status {
		session, err := s.GetSession(msg.SessionID)
		if err != nil {
			s.logger.Error("Error session not found", "error", err)
		}

		session.HandleMessage(id, msg)

		return
	}

	sessions, _ := s.im.InterestedSessions(msg.GetBlocks()) //TODO: Add orphans to cache

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

func (s *SessionManager) PeerDisconnected(Sessions []ktypes.Address, peerID peer.ID) {
	for _, sessionID := range Sessions {
		if session, ok := s.activeSessions.Load(sessionID); ok {
			session, ok := session.(*Session)
			if ok {
				session.PeerDisconnected(peerID)
			}
		}
	}
}

func (s *SessionManager) CloseSession(sessionID ktypes.Address) {
	s.activeSessions.Delete(sessionID)
}

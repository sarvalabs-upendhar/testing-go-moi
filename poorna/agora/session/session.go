package session

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	atypes "github.com/sarvalabs/moichain/poorna/agora/types"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

type sessionInterestManager interface {
	RecordSessionInterest(addr types.Address, ids ...atypes.CID)
	RemoveSessionInterest(addr types.Address, ids ...atypes.CID) []atypes.CID
}

type sessionManager interface {
	CloseSession(id types.Address)
}

type sessionNetwork interface {
	SendAgoraMessage(id id.KramaID, msgType ptypes.MsgType, msg atypes.Message) error
	ClosePeerSession(kramaID id.KramaID, sessionID types.Address) error
}

type Session struct {
	id        types.Address
	ctx       context.Context
	logger    hclog.Logger
	stateHash atypes.CID
	im        sessionInterestManager
	wants     *atypes.WantTracker
	pm        *PeerManager
	notifier  atypes.PubSub
	sm        sessionManager
}

func NewSession(
	ctx context.Context,
	addr types.Address,
	logger hclog.Logger,
	stateHash atypes.CID,
	network sessionNetwork,
	notifier atypes.PubSub,
	im sessionInterestManager,
	sm sessionManager,
	contextPeers []id.KramaID,
) *Session {
	taggedLogger := logger.With("addr", addr.Hex()).Named("Session")
	s := &Session{
		ctx:       ctx,
		id:        addr,
		logger:    taggedLogger,
		stateHash: stateHash,
		im:        im,
		wants:     atypes.NewWantTracker(),
		pm:        NewSessionPeerManager(addr, logger, network),
		notifier:  notifier,
		sm:        sm,
	}

	s.pm.AddPeers(contextPeers...)

	return s
}

func (s *Session) ID() types.Address {
	return s.id
}

func (s *Session) HandleMessage(id id.KramaID, msg *atypes.AgoraResponseMsg) {
	if !msg.Status {
		s.pm.UpdatePeerStatus(id, false)
		s.pm.AddPeers(msg.PeerSet...)
	}

	if err := s.pm.Signal(id, msg.Status); err != nil {
		s.logger.Error("Error signaling the routine", "error", err)
	}
}

func (s *Session) ChooseBestPeer(ctx context.Context, avoid map[id.KramaID]interface{}) (id.KramaID, error) {
	return s.pm.chooseBestPeer(ctx, avoid)
}

func (s *Session) sendWantReq(peerID id.KramaID, cid *atypes.CIDSet) error {
	req := &atypes.AgoraRequestMsg{
		SessionID: s.id,
		StateHash: s.stateHash,
		WantList:  cid.Keys(),
	}

	return s.pm.SendWantReq(peerID, req)
}

func (s *Session) GetBlock(ctx context.Context, cid atypes.CID) (*atypes.Block, error) {
	out := s.GetBlocks(ctx, []atypes.CID{cid})

	data, ok := <-out
	if !ok {
		return nil, types.ErrTimeOut
	}

	return data, nil
}

func (s *Session) getBlocks(
	ctx context.Context,
	peerID id.KramaID,
	out chan *atypes.Block,
	idSet *atypes.CIDSet,
) error {
	s.logger.Debug("Fetching data from ", "peer", peerID, "count", idSet.Len())

	requestCtx, cancelFn := context.WithTimeout(ctx, 3*time.Second)

	defer func() {
		cancelFn()
		s.pm.UpdatePeerStatus(peerID, false)
	}()

	if idSet.Len() <= 0 {
		return nil
	}

	if err := s.sendWantReq(peerID, idSet); err != nil {
		return err
	}

	s.pm.UpdatePeerStatus(peerID, true)

	s.im.RecordSessionInterest(s.id, idSet.Keys()...)

	notifier := s.notifier.Subscribe(requestCtx, idSet.Keys()...)

	statusChan := s.pm.PeerRespChan(peerID)
	if statusChan == nil {
		return errors.New("peer not found")
	}

	for {
		select {
		case block, ok := <-notifier:
			if !ok {
				return nil
			}
			// s.wants.RemoveCid(block.GetCid())
			idSet.Remove(block.GetCid())
			s.im.RemoveSessionInterest(s.id, block.GetCid())

			out <- &block

			if idSet.Len() == 0 {
				s.logger.Info("All keys received returning from getBlocks")

				return nil
			}

		case status := <-statusChan:
			if !status {
				return errors.New("peer not available")
			}

		case <-s.ctx.Done():
			s.logger.Error("Request context expired")

			return s.ctx.Err()
		case <-requestCtx.Done():
			s.logger.Error("Get Blocks context expired")

			return s.ctx.Err()
		}
	}
}

func (s *Session) GetBlocks(ctx context.Context, cids []atypes.CID) chan *atypes.Block {
	out := make(chan *atypes.Block)

	idSet := atypes.NewHashSet()

	for _, cid := range cids {
		idSet.Add(cid)
	}

	attemptedPeers := make(map[id.KramaID]interface{})

	go func() {
		defer close(out)

		attempt := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.ctx.Done():
				return

			default:
			}

			if attempt > MaxRounds {
				s.logger.Error("Error max attempts reached")

				return
			}

			peerID, err := s.ChooseBestPeer(ctx, attemptedPeers)
			if err != nil {
				s.logger.Error("Error finding best peer", "error", err)

				attempt++

				continue
			}

			attemptedPeers[peerID] = true

			if err = s.getBlocks(ctx, peerID, out, idSet); err != nil {
				s.logger.Error("Error fetching blocks", "error", err)

				s.pm.UpdateFailedAttempts(peerID, 1)
				attempt++

				continue
			}

			break
		}
	}()

	return out
}

func (s *Session) PeerDisconnected(id peer.ID) {
	s.pm.PeerDisconnected(id)
}

func (s *Session) Close() {
	s.pm.Close()
	s.sm.CloseSession(s.id)
}

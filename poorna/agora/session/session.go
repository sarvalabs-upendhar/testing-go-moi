package session

import (
	"context"
	"errors"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"time"
)

type sessionInterestManager interface {
	RecordSessionInterest(addr ktypes.Address, ids ...ktypes.Hash)
	RemoveSessionInterest(addr ktypes.Address, ids ...ktypes.Hash) []ktypes.Hash
}

type sessionManager interface {
	CloseSession(id ktypes.Address)
}

type sessionNetwork interface {
	SendAgoraMessage(id id.KramaID, msgType ktypes.MsgType, msg types.Message) error
	ClosePeerSession(kramaID id.KramaID, sessionID ktypes.Address) error
}

type Session struct {
	id        ktypes.Address
	ctx       context.Context
	logger    hclog.Logger
	stateHash ktypes.Hash
	im        sessionInterestManager
	wants     *types.WantTracker
	pm        *SessionPeerManager
	notifier  types.PubSub
	sm        sessionManager
}

func NewSession(
	ctx context.Context,
	addr ktypes.Address,
	logger hclog.Logger,
	stateHash ktypes.Hash,
	network sessionNetwork,
	notifier types.PubSub,
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
		wants:     types.NewWantTracker(),
		pm:        NewSessionPeerManager(addr, logger, network),
		notifier:  notifier,
		sm:        sm,
	}

	s.pm.AddPeers(contextPeers...)

	return s
}

func (s *Session) HandleMessage(id id.KramaID, msg *types.AgoraResponseMsg) {
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

func (s *Session) sendWantReq(peerID id.KramaID, cid *kutils.HashSet) error {
	req := &types.AgoraRequestMsg{
		SessionID: s.id,
		StateHash: s.stateHash,
		WantList:  cid.Keys(),
	}

	return s.pm.SendWantReq(peerID, req)
}
func (s *Session) GetBlock(ctx context.Context, cid ktypes.Hash) (*types.Block, error) {
	out := s.GetBlocks(ctx, []ktypes.Hash{cid})

	data, ok := <-out
	if !ok {
		return nil, ktypes.ErrTimeOut
	}

	return data, nil
}
func (s *Session) getBlocks(
	ctx context.Context,
	peerID id.KramaID,
	out chan *types.Block,
	idSet *kutils.HashSet,
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
			//s.wants.RemoveCid(block.GetID())
			idSet.Remove(block.GetID())
			s.im.RemoveSessionInterest(s.id, block.GetID())

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

func (s *Session) GetBlocks(ctx context.Context, cids []ktypes.Hash) chan *types.Block {
	out := make(chan *types.Block)

	idSet := kutils.NewHashSet()

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

				continue
			}

			attemptedPeers[peerID] = true

			if err := s.getBlocks(ctx, peerID, out, idSet); err != nil {
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

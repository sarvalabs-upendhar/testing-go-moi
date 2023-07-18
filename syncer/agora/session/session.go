package session

import (
	"context"
	"errors"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
	"github.com/sarvalabs/go-moi/syncer/agora/notifications"
	"github.com/sarvalabs/go-moi/syncer/cid"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/go-moi/common"
)

type sessionInterestManager interface {
	RecordSessionInterest(addr common.Address, ids ...cid.CID)
	RemoveSessionInterest(addr common.Address, ids ...cid.CID) []cid.CID
}

type sessionManager interface {
	CloseSession(id common.Address)
}

type sessionNetwork interface {
	SendAgoraMessage(id id.KramaID, msgType networkmsg.MsgType, msg message.Message) error
	ClosePeerSession(kramaID id.KramaID, sessionID common.Address) error
}

type Session struct {
	id        common.Address
	ctx       context.Context
	logger    hclog.Logger
	stateHash cid.CID
	im        sessionInterestManager
	wants     *WantTracker
	pm        *PeerManager
	notifier  notifications.PubSubNotifier
	sm        sessionManager
}

func NewSession(
	ctx context.Context,
	addr common.Address,
	logger hclog.Logger,
	stateHash cid.CID,
	network sessionNetwork,
	notifier notifications.PubSubNotifier,
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
		wants:     NewWantTracker(),
		pm:        NewSessionPeerManager(addr, logger, network),
		notifier:  notifier,
		sm:        sm,
	}

	s.pm.AddPeers(contextPeers...)

	return s
}

func (s *Session) ID() common.Address {
	return s.id
}

func (s *Session) HandleMessage(id id.KramaID, msg *message.AgoraResponseMsg) {
	if !msg.Status {
		s.pm.UpdatePeerStatus(id, false)
		s.pm.AddPeers(msg.PeerSet...)
	}

	if err := s.pm.Signal(id, msg.Status); err != nil {
		s.logger.Error("Error signaling the routine", "err", err)
	}
}

func (s *Session) ChooseBestPeer(ctx context.Context, avoid map[id.KramaID]interface{}) (id.KramaID, error) {
	return s.pm.chooseBestPeer(ctx, avoid)
}

func (s *Session) sendWantReq(peerID id.KramaID, cid *cid.CIDSet) error {
	req := &message.AgoraRequestMsg{
		SessionID: s.id,
		StateHash: s.stateHash,
		WantList:  cid.Keys(),
	}

	return s.pm.SendWantReq(peerID, req)
}

func (s *Session) GetBlock(ctx context.Context, cID cid.CID) (*block.Block, error) {
	out := s.GetBlocks(ctx, []cid.CID{cID})

	data, ok := <-out
	if !ok {
		return nil, common.ErrTimeOut
	}

	return data, nil
}

func (s *Session) getBlocks(
	ctx context.Context,
	peerID id.KramaID,
	out chan *block.Block,
	idSet *cid.CIDSet,
) error {
	s.logger.Debug("Fetching data from", "peer-ID", peerID, "count", idSet.Len())

	//	requestCtx, cancelFn := context.WithTimeout(ctx, 3*time.Second)

	defer func() {
		// cancelFn()
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

	notifier := s.notifier.Subscribe(ctx, idSet.Keys()...)

	statusChan := s.pm.PeerRespChan(peerID)
	if statusChan == nil {
		return errors.New("peer not found")
	}

	for {
		select {
		case blk, ok := <-notifier:
			if !ok {
				return nil
			}
			// s.wants.RemoveCid(blk.GetCid())
			idSet.Remove(blk.GetCid())
			s.im.RemoveSessionInterest(s.id, blk.GetCid())

			out <- &blk

			if idSet.Len() == 0 {
				s.logger.Debug("All keys received returning from getBlocks")

				return nil
			}

		case status := <-statusChan:
			if !status {
				return errors.New("peer not available")
			}

		case <-s.ctx.Done():
			s.logger.Error("Request context expired")

			return s.ctx.Err()
		}
	}
}

func (s *Session) GetBlocks(ctx context.Context, cids []cid.CID) chan *block.Block {
	out := make(chan *block.Block)

	idSet := cid.NewHashSet()

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
				s.logger.Error("Error finding best peer", "err", err)

				attempt++

				continue
			}

			attemptedPeers[peerID] = true

			if err = s.getBlocks(ctx, peerID, out, idSet); err != nil {
				s.logger.Error("Error fetching blocks", "err", err)

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

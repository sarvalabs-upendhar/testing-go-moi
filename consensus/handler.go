package consensus

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-polo"
)

// Handler is the main event loop for the consensus engine.It listens for the following events
// - Context cancellation
// - NewView ticks
// - Inbound messages
// - Slot closures
func (k *Engine) handler() {
	viewTicker := common.NewViewTicker(time.Unix(int64(k.cfg.GenesisTimestamp), 0),
		uint64(k.pool.ViewTimeOut().Seconds()))

	for {
		select {
		case <-k.ctx.Done():
			// If the context is done, close the view ticker and return
			viewTicker.Done()
			k.logger.Info("Closing Krama engine. Reason: context-closed")

			return
		case viewID := <-viewTicker.C():
			k.metrics.captureCurrentView(viewID)
			k.view.Store(viewID) // When a new view tick is received, update the current view and trigger the view handler.
			k.pool.UpdateCurrentView(viewID)
			k.viewTimeOutDeadline.Store(time.Now().Add(k.pool.ViewTimeOut()))

			k.handleNewView(k.ctx, viewID)
		case msg := <-k.transport.Messages():
			k.handleConsensusMessage(msg)
		case clusterID := <-k.icsCloseCh:
			// When a slot close event is received, gracefully close the context router, and clean up the slot.
			k.transport.GracefullyCloseContextRouter(clusterID)
			k.slots.CleanupSlot(clusterID)
			k.logger.Debug("Cleaning consensus slot", "cluster-id", clusterID)
		}
	}
}

// deleteLockedTesseractInfo returns true locked ts already exist in db
func (k *Engine) deleteLockedTesseractInfo(ts *common.Tesseract) bool {
	for addr, state := range ts.Participants() {
		dbState, err := k.db.GetAccountMetaInfo(addr)
		if err == nil && dbState.Height >= state.Height {
			k.logger.Debug("deleting locked tesseract", "cluster-id", ts.ClusterID(), "ts-hash", ts.Hash())

			if err := k.db.DeleteConsensusProposalInfo(ts.Hash()); err != nil {
				k.logger.Error("failed to delete consensus proposal info", "error", err, "ts-hash", ts.Hash())
			}

			for addr := range ts.Participants() {
				if err := k.safety.DeleteSafetyData(addr); err != nil {
					k.logger.Error("Failed to delete safety data", "err", err, "addr", addr)
				}
			}

			return true
		}
	}

	return false
}

// handleNewView checks if there are any failed views to handle first. If failed views exist, it processes them.
// Otherwise, it fetches interactions from the pool and creates a new cluster for the interactions.
func (k *Engine) handleNewView(ctx context.Context, viewID uint64) {
	for _, msg := range k.futureMsg {
		k.handleConsensusMessage(msg)
		k.dequeueFutureMsg()
	}

	proposedTS, err := k.safety.GetFailedViewTS()
	if err != nil {
		k.logger.Error("failed to get failed view ts", "error", err)
	}

	for _, ts := range proposedTS {
		k.logger.Debug("proposing locked tesseracts", "ts-count", len(proposedTS), "ts-hash",
			ts.Hash(), "cluster-id", ts.ClusterID())

		if k.deleteLockedTesseractInfo(ts) {
			continue
		}

		if !k.isOperatorEligible(k.selfID, ts.Interactions()) {
			k.logger.Debug("can not propose locked tesseract", "cluster-id", ts.ClusterID())

			continue
		}

		if err = k.handleFailedView(ts.ConsensusInfo().View, ts); err != nil {
			k.logger.Error("failed to handle old view qc", "error", err)
		}
	}

	for _, batch := range k.pool.ProcessableBatches() {
		clusterID, err := types.GenerateClusterID()
		if err != nil {
			k.logger.Error("failed to create clusterID")

			continue
		}

		ixs := batch.Interactions()

		k.logger.Debug("Handling new ixs for view", "view-id", viewID)

		if !k.isOperatorEligible(k.selfID, ixs) {
			k.logger.Debug("operator not eligible", "view", viewID)

			continue
		}

		k.logger.Debug("Operator is eligible", "view", viewID)

		slot, activeCluster, err := k.createICS(ctx, clusterID, ixs, ixs.Locks())
		if err != nil {
			k.logger.Debug("failed to create a slot", "view", viewID, "active-cluster", activeCluster, "err", err)
			k.slots.CleanupSlot(clusterID)

			return
		}

		go k.icsHandler(ctx, clusterID) // operator
		slot.NewICSChan <- clusterID
	}
}

func (k *Engine) handleFailedView(failedView uint64, ts *common.Tesseract) error {
	clusterID, err := types.GenerateClusterID()
	if err != nil {
		k.logger.Error("failed to create clusterID")

		return err
	}

	k.logger.Debug("Handling failed view", "old cluster id", ts.ClusterID(), "cluster-id", clusterID)

	slot, activeCluster, err := k.createICS(k.ctx, clusterID, ts.Interactions(), ts.ConsensusInfo().AccountLocks)
	if err != nil {
		k.logger.Debug("failed to create a slot", "view", failedView,
			"active-cluster", activeCluster, "cluster-id", ts.ClusterID(), "error", err)
		k.slots.CleanupSlot(clusterID)

		return err
	}

	go k.icsHandler(k.ctx, clusterID) // operator
	slot.NewICSChan <- clusterID

	return nil
}

// handleConsensusMessage processes incoming consensus messages based on their type.
// It supports the following message types:
// - PREPARE
// - PREPARED
// - PROPOSAL
// - VOTEMSG
func (k *Engine) handleConsensusMessage(msg *types.ICSMSG) {
	switch msg.MsgType {
	case message.PREPARE:
		prepare := new(types.Prepare)
		if err := polo.Depolorize(prepare, msg.Payload); err != nil {
			k.logger.Error("failed to depolarize prepare msg", "err", err)

			return
		}

		if err := k.handlePrepare(context.Background(), msg, prepare); err != nil {
			k.logger.Error("failed to handle prepare msg", "err", err, "cluster-id", msg.ClusterID)
		}

	case message.PREPARED:
		prepared := new(types.Prepared)
		if err := polo.Depolorize(prepared, msg.Payload); err != nil {
			k.logger.Error("failed to depolarize prepared msg", "err", err)

			return
		}

		slot := k.slots.GetSlot(msg.ClusterID)
		if slot == nil {
			k.logger.Error("slot missing for cluster", "cluster-id", msg.ClusterID)

			return
		}

		slot.ForwardMsgToICSHandler(types.ConsensusMessage{
			PeerID:  msg.Sender,
			Payload: prepared,
		})

	case message.PROPOSAL:
		proposal := new(types.ProposalMsg)
		if err := proposal.FromBytes(msg.Payload); err != nil {
			k.logger.Error("failed to depolarize proposal msg", "err", err)

			return
		}

		if err := k.createICSForProposal(k.ctx, msg.Sender, proposal.Proposal()); err != nil {
			k.logger.Error("failed to handle proposal msg", "err", err, "cluster-id", msg.ClusterID)

			k.metrics.captureICSParticipationFailureCount(1)
			k.transport.GracefullyCloseContextRouter(msg.ClusterID)
			k.slots.CleanupSlot(msg.ClusterID)
			k.logger.Debug("Cleaning consensus slot", "cluster-id", msg.ClusterID)

			return
		}

	case message.VOTEMSG:
		vote := new(types.Vote)
		if err := vote.FromBytes(msg.Payload); err != nil {
			k.logger.Error("failed to depolarize vote msg", "err", err)

			return
		}

		slot := k.slots.GetSlot(msg.ClusterID)
		if slot == nil {
			k.logger.Error("slot missing for cluster", "cluster-id", msg.ClusterID)

			return
		}

		slot.ForwardMsgToKBFTHandler(types.ConsensusMessage{
			PeerID:  msg.Sender,
			Payload: vote,
		})

	default:
		k.logger.Error("Unsupported message type")
	}
}

func (k *Engine) createPreparedMsg(msg *types.Prepare) (*types.Prepared, error) {
	viewInfos, err := k.loadViewInfo(msg.Ps)
	if err != nil {
		return nil, err
	}

	responseMsg := &types.Prepared{
		View:  msg.View,
		Infos: viewInfos,
	}

	if err = responseMsg.Sign(k.vault.Sign); err != nil {
		return nil, err
	}

	return responseMsg, nil
}

/*
- Validate the view ID
- Load the view information of the participants and send the prepared message
*/
func (k *Engine) handlePrepare(
	ctx context.Context,
	msg *types.ICSMSG,
	prepare *types.Prepare,
) error {
	k.logger.Debug("Handling prepare message", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	if k.view.Load() != prepare.View {
		if prepare.View-k.view.Load() == 1 {
			k.enqueueFutureMsg(msg)
		}

		k.logger.Debug("invalid view", "local view", k.view.Load(), "remote view", prepare.View)
		// leader view and the local view should match
		return errors.New("invalid view")
	}

	preparedMsg, err := k.createPreparedMsg(prepare)
	if err != nil {
		return err
	}

	rawData, err := preparedMsg.Bytes()
	if err != nil {
		return err
	}

	return k.transport.SendMessage(
		ctx,
		msg.Sender,
		types.NewICSMsg(k.selfID, msg.ClusterID, message.PREPARED, rawData),
	)
}

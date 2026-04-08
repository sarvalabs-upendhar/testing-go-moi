package consensus

import (
	"context"
	"sort"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-polo"
)

func findWinner(prepareMsgs []*metaPrepareMsg) identifiers.KramaID {
	for _, prepare := range prepareMsgs {
		prepare.rankHash = prepare.Hash()
	}

	sort.Slice(prepareMsgs, func(i, j int) bool {
		return prepareMsgs[i].rankHash.String() < prepareMsgs[j].rankHash.String()
	})

	return prepareMsgs[0].sender
}

// viewTime calculates the timestamp for a given view number.
func viewTime(genesisTime uint64, viewNumber uint64, viewDuration time.Duration) time.Time {
	genesis := time.Unix(int64(genesisTime), 0)
	offset := time.Duration(viewNumber) * viewDuration

	return genesis.Add(offset)
}

func (k *Engine) schedulePrepareTimeout(prepareTimeoutDeadline time.Time) {
	select {
	case <-k.ctx.Done():
		return
	case <-time.After(time.Until(prepareTimeoutDeadline)):
		k.prepareTimeout <- struct{}{}
	}
}

func (k *Engine) processPrepareMsgs(participantToPrepareMsgs map[identifiers.Identifier][]*metaPrepareMsg) {
	for _, prepareMsgs := range participantToPrepareMsgs {
		if len(prepareMsgs) == 1 { // if number of unique prepare msgs are equal to 1
			continue
		}

		winner := findWinner(prepareMsgs)

		for _, prepare := range prepareMsgs {
			if prepare.sender == winner {
				continue
			}

			prepare.shouldReply = false
		}
	}

	for _, prepareMsgs := range participantToPrepareMsgs {
		for _, prepare := range prepareMsgs {
			if !prepare.shouldReply || prepare.msgSent {
				continue
			}

			ps := prepare.ixns.UniqueIdsWithoutNoLocks()

			viewInfos, err := k.loadViewInfo(ps)
			if err != nil {
				k.logger.Error("failed to load view info", "err", err)

				break
			}

			preparedMsg, err := k.createPreparedMsg(prepare.msg, viewInfos)
			if err != nil {
				break
			}

			rawData, err := preparedMsg.Bytes()
			if err != nil {
				break
			}

			if err := k.publishEventPrepared(preparedMsg); err != nil {
				k.logger.Error("failed to publish prepared message", "err", err)
			}

			if err = k.transport.SendMessage(
				context.Background(),
				prepare.sender,
				types.NewICSMsg(k.selfID, prepare.clusterID, message.PREPARED, rawData, false),
			); err != nil {
				k.logger.Error("failed to send prepared message", "err", err)
			}

			prepare.msgSent = true

			break
		}
	}
}

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

			if k.slots.Len() == 0 {
				k.logger.Info("Closing Krama engine. Reason: context-closed")

				return
			}

			k.ctxClosed.Store(true)
		case viewID := <-viewTicker.C():
			k.metrics.captureCurrentView(viewID)

			viewStartTime := viewTime(k.cfg.GenesisTimestamp, viewID, k.pool.ViewTimeOut())
			currentTime := time.Now()

			// When a new view tick is received, update the current view and trigger the view handler.
			k.currentView = types.NewView(viewID, viewStartTime, currentTime.Add(k.pool.ViewTimeOut()))

			k.pool.UpdateCurrentView(viewID)

			prepareTimeOutDeadline := currentTime.Add(k.cfg.TimeoutPrepare)

			k.participantToPrepareMsg = make(map[identifiers.Identifier][]*metaPrepareMsg)
			k.stopPrepareMsgs = false

			go k.schedulePrepareTimeout(prepareTimeOutDeadline)

			for _, msg := range k.futureMsg {
				k.handleConsensusMessage(msg)
				k.dequeueFutureMsg()
			}

			go k.handleNewView(k.ctx, k.currentView)
		case <-k.prepareTimeout:
			k.stopPrepareMsgs = true

			participantToPrepareMsgs := k.participantToPrepareMsg

			go k.processPrepareMsgs(participantToPrepareMsgs)

		case msg := <-k.transport.Messages():
			k.handleConsensusMessage(msg)
		case clusterID := <-k.icsCloseCh:
			// When a slot close event is received, gracefully close the context router, and clean up the slot.
			k.transport.GracefullyCloseContextRouter(clusterID)
			k.slots.CleanupSlot(clusterID)
			k.logger.Debug("Cleaning consensus slot", "cluster-id", clusterID)

			if err := k.publishEventCleanup(clusterID); err != nil {
				k.logger.Error("failed to publish cleanup event", "err", err)
			}

			if k.ctxClosed.Load() && k.slots.Len() == 0 {
				return
			}
		}
	}
}

// deleteLockedTesseractInfo returns true if locked ts already exist in db
func (k *Engine) deleteLockedTesseractInfo(ts *common.Tesseract) bool {
	for id, state := range ts.Participants() {
		dbState, err := k.db.GetAccountMetaInfo(id)
		if err == nil && dbState.Height >= state.Height {
			k.logger.Debug("deleting locked tesseract", "cluster-id", ts.ClusterID(), "ts-hash", ts.Hash())

			if err := k.db.DeleteConsensusProposalInfo(ts.Hash()); err != nil {
				k.logger.Error("failed to delete consensus proposal info", "error", err, "ts-hash", ts.Hash())
			}

			for id = range ts.Participants() {
				if err := k.safety.DeleteSafetyData(id); err != nil {
					k.logger.Error("Failed to delete safety data", "err", err, "id", id)
				}
			}

			return true
		}
	}

	return false
}

// handleNewView checks if there are any failed views to handle first. If failed views exist, it processes them.
// Otherwise, it fetches interactions from the pool and creates a new cluster for the interactions.
func (k *Engine) handleNewView(ctx context.Context, view *types.View) {
	viewID := view.ID()

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

		if !k.isOperatorEligible(k.selfID, ts.Interactions(), viewID) {
			k.logger.Debug("not eligible to propose locked tesseract", "cluster-id", ts.ClusterID())

			continue
		}

		if err = k.handleFailedView(ts.ConsensusInfo().View, ts, view); err != nil {
			k.logger.Error("failed to handle old view qc", "error", err)
		}
	}

	for _, batch := range k.pool.ProcessableBatches() {
		clusterID, err := common.GenerateClusterID()
		if err != nil {
			k.logger.Error("failed to create clusterID")

			continue
		}

		ixs := batch.Interactions()

		k.logger.Debug("Handling new ixs for view", "view-id", viewID, "batch-size", ixs.Len())

		if !k.isOperatorEligible(k.selfID, ixs, viewID) {
			k.logger.Debug("operator not eligible", "view", viewID)

			continue
		}

		k.logger.Debug("Creator is eligible", "view", viewID)

		for _, ixn := range ixs.IxList() {
			k.logger.Debug("Creating ICS for cluster", ixn.Hash())
		}

		slot, activeCluster, err := k.createICS(ctx, clusterID, ixs, ixs.Locks(), view)
		if err != nil {
			k.logger.Debug("failed to create a slot", "view", viewID, "active-cluster", activeCluster, "err", err)
			k.slots.CleanupSlot(clusterID)

			return
		}

		go k.icsHandler(ctx, clusterID) // operator
		slot.NewICSChan <- clusterID
	}
}

func (k *Engine) handleFailedView(failedView uint64, ts *common.Tesseract, view *types.View) error {
	clusterID, err := common.GenerateClusterID()
	if err != nil {
		k.logger.Error("failed to create clusterID")

		return err
	}

	k.logger.Debug("Handling failed view", "old cluster id", ts.ClusterID(), "cluster-id", clusterID)

	slot, activeCluster, err := k.createICS(k.ctx, clusterID, ts.Interactions(), ts.ConsensusInfo().AccountLocks, view)
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
		if k.stopPrepareMsgs {
			return
		}

		prepare := new(types.Prepare)
		if err := polo.Depolorize(prepare, msg.Payload); err != nil {
			k.logger.Error("failed to depolarize prepare msg", "err", err)

			return
		}

		if err := k.handlePrepare(msg, prepare); err != nil {
			k.logger.Error("failed to handle prepare msg", "err", err, "cluster-id", msg.ClusterID)
		}

	case message.PREPARED:
		prepared := new(types.Prepared)
		if err := polo.Depolorize(prepared, msg.Payload); err != nil {
			k.logger.Error("failed to depolarize prepared msg", "err", err)

			return
		}

		prepared.Verified = msg.Verified

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
		deCompressionTime := time.Now()

		if err := msg.DeCompressPayload(k.compressor); err != nil {
			k.logger.Error("failed to decompress payload", "err", err)

			return
		}

		k.metrics.captureDeCompressionTime(deCompressionTime)

		proposal := new(types.ProposalMsg)
		if err := proposal.FromBytes(msg.Payload); err != nil {
			k.logger.Error("failed to depolarize proposal msg", "err", err)

			return
		}

		if err := k.initICS(k.ctx, msg.Sender, proposal, k.currentView); err != nil {
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
			k.logger.Error("slot missing for cluster", "cluster-id", msg.ClusterID,
				"sender", msg.Sender, "type", msg.MsgType)

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

func (k *Engine) createPreparedMsg(msg *types.Prepare, viewInfos []*common.ViewInfo) (*types.Prepared, error) {
	responseMsg := &types.Prepared{
		View:  msg.View,
		Infos: viewInfos,
	}

	if err := responseMsg.Sign(k.vault.Sign); err != nil {
		return nil, err
	}

	return responseMsg, nil
}

func (k *Engine) isParticipantsContext(id identifiers.Identifier, peerID identifiers.KramaID) bool {
	metaInfo, err := k.state.GetAccountMetaInfo(id)
	if err != nil {
		k.logger.Error("failed to check operator eligibility", "error", err)

		return false
	}

	if id.IsParticipantVariant() {
		metaInfo, err = k.state.GetAccountMetaInfo(metaInfo.InheritedAccount)
		if err != nil {
			k.logger.Error("failed to check operator eligibility", "error", err)

			return false
		}
	}

	consensusNodes, _, err := k.state.GetConsensusNodes(id, metaInfo.ContextHash)
	if err != nil {
		k.logger.Error("failed to check operator eligibility", "error", err)

		return false
	}

	return consensusNodes.Contains(peerID)
}

func (k *Engine) fetchParticipantsThisNodeIsContext(ixns *common.Interactions) []identifiers.Identifier {
	ids := make([]identifiers.Identifier, 0)

	for id, info := range ixns.Participants() {
		if info.IsGenesis || info.LockType == common.NoLock {
			continue
		}

		if k.isParticipantsContext(id, k.selfID) {
			ids = append(ids, id)
		}
	}

	return ids
}

/*
- Validate the view ID
- Load the view information of the participants and send the prepared message
*/
func (k *Engine) handlePrepareMsg(msg *types.ICSMSG, prepare *types.Prepare) error {
	k.logger.Debug("Handling prepare message", "cluster-id", msg.ClusterID, "sender", msg.Sender)

	if !k.currentView.IsEqualID(prepare.View) {
		if k.currentView.IsNextView(prepare.View) {
			k.enqueueFutureMsg(msg)
		}

		k.logger.Debug("invalid view", "local view", k.currentView.ID(), "remote view", prepare.View)
		// leader view and the local view should match
		return common.ErrInvalidView
	}

	ixs, found := k.pool.GetIxns(prepare.Ixns)
	if !found {
		return common.ErrIxnsNotFound
	}

	ixns := common.NewInteractionsWithLeaderCheck(true, ixs...)

	if !k.isOperatorEligible(msg.Sender, ixns, k.currentView.ID()) {
		return common.ErrOperatorNotEligible
	}

	ids := k.fetchParticipantsThisNodeIsContext(&ixns)

	metaPrepareMessage := &metaPrepareMsg{
		msg:         prepare,
		ixns:        &ixns,
		sender:      msg.Sender,
		clusterID:   msg.ClusterID,
		shouldReply: true,
	}

	for _, id := range ids {
		k.participantToPrepareMsg[id] = append(k.participantToPrepareMsg[id], metaPrepareMessage)
	}

	return nil
}

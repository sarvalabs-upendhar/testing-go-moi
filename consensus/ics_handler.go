package consensus

import (
	"bytes"
	"context"
	"sort"
	"time"

	networkmsg "github.com/sarvalabs/go-moi/network/message"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/telemetry/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
)

type LockStatus int

const (
	validLock LockStatus = iota
	invalidLock
	noLock
)

type identifierInfo struct {
	height uint64
	lock   common.ConsensusMsgType
	view   uint64
}

type lockedInfo struct {
	view uint64
	ts   *common.Tesseract
}

// icsHandler is the main handler for ICS Cluster.It handles the timeout events and messages
func (k *Engine) icsHandler(ctx context.Context, clusterID common.ClusterID) {
	spanCtx, span := tracing.Span(
		ctx, "Krama.Handler", "icsHandler",
		trace.WithAttributes(attribute.String("clusterID", clusterID.String())),
	)
	defer span.End()

	slot := k.slots.GetSlot(clusterID)
	cs := slot.ClusterState()

	k.metrics.captureActiveICSClusters(1)

	k.metrics.captureSlotCount(int(slot.SlotType), 1)

	defer func() {
		// signal the core handler to close the slot
		k.metrics.captureActiveICSClusters(-1)
		k.metrics.captureSlotCount(int(slot.SlotType), -1)

		k.closeICS(clusterID)
	}()

	for {
		select {
		case <-time.After(time.Until(slot.ViewTimeoutDeadline())): //nolint
			// we should handle this time out if bft is not started

			if stage := slot.GetStage(); stage == ktypes.PrepareStage || stage == ktypes.PreparedStage {
				if slot.SlotType == ktypes.OperatorSlot {
					k.metrics.captureICSCreationFailureCount(1)
				}

				return
			}

		case msg := <-slot.BftOutboundChan:
			k.handleOutboundMessage(ctx, slot.ClusterID(), msg)
		case <-slot.NewICSChan:
			if err := k.sendPrepare(spanCtx, cs); err != nil {
				k.logger.Error("failed to send prepare msg", "error", err, "cluster-id", clusterID)

				return
			}
		case err := <-slot.BftStopChan:
			if err != nil {
				k.logger.Error("error occurred in bft", "cluster-id", clusterID)

				k.metrics.captureAgreementFailureCount(1)
			}

			return
		case msg := <-slot.Msgs:
			cMsg := msg.Payload

			switch m := cMsg.(type) {
			case *ktypes.Prepared:
				if err := k.handlePrepared(spanCtx, clusterID, m, msg.PeerID); err != nil {
					k.logger.Error("failed to handle prepared msg", "error", err)
				}

			case *ktypes.Proposal:
				if err := k.handleProposal(spanCtx, slot.ClusterState(), m); err != nil {
					k.logger.Error("failed to handle proposal msg", "error", err)

					return
				}

				slot.Stage.CompareAndSwap(ktypes.PrepareStage, ktypes.PreparedStage)

				ixnHash := cs.IxnsHash()

				// Start the BFT handler
				icsEvidence := kbft.NewEvidence(ixnHash, cs.Operator(), cs.Size())
				bft := kbft.NewKBFTService(
					ctx,
					msg.PeerID,
					slot.ViewTimeoutDeadline(),
					k.cfg,
					k.vault, cs.VoteSet(), slot, k.safety, cs.IsTSStored(), k.finalizedTSHandler,
					kbft.WithLogger(k.logger.With("cluster-id", clusterID)),
					kbft.WithWal(kbft.NullWal{}),
					kbft.WithEvidence(icsEvidence),
					kbft.WithMux(k.consensusMux))

				go k.startBFT(bft, slot)

				slot.ForwardMsgToKBFTHandler(msg)

			default:
				slot.ForwardMsgToKBFTHandler(msg)
			}
		}
	}
}

func (k *Engine) startBFT(bft *kbft.KBFT, slot *ktypes.Slot) {
	agreementInitTime := time.Now()

	bft.Start(slot)

	k.metrics.captureAgreementTime(agreementInitTime)
}

func (k *Engine) captureCompressionMetrics(compressionTime time.Time, before float64, after float64) {
	k.metrics.captureCompressionTime(compressionTime)
	k.metrics.captureCompressedPayloadSize(after)
	k.metrics.captureCompressionRatio(((before - after) / before) * 100)
}

// handleOutboundMessage processes the outbound vote messages.
func (k *Engine) handleOutboundMessage(
	ctx context.Context,
	clusterID common.ClusterID,
	msg ktypes.ConsensusMessage,
) {
	icsMsg, err := msg.ICSMsg(clusterID)
	if err != nil {
		k.logger.Error(
			"failed to create ICS msg from consensus msg",
			"krama-id", msg.Recipient,
			"error",
			err)

		return
	}

	k.logger.Debug("Handling outbound message", "cluster-id", clusterID,
		"recipient", msg.Recipient, "msg-type", icsMsg.MsgType)

	if icsMsg.MsgType == networkmsg.PROPOSAL {
		before := float64(len(icsMsg.Payload))
		compressionTime := time.Now()

		k.metrics.captureInputPayloadSize(before)

		if err := icsMsg.CompressPayload(k.compressor); err != nil {
			k.logger.Error("unable to compress payload", err)

			return
		}

		after := float64(len(icsMsg.Payload))

		k.captureCompressionMetrics(compressionTime, before, after)
	}

	// we should broadcast the message if the recipient is empty
	if msg.Recipient == "" {
		k.transport.BroadcastMessage(ctx, icsMsg)

		return
	}

	if err = k.transport.SendMessage(ctx, msg.Recipient, icsMsg); err != nil {
		k.logger.Error("failed to send message to peer", "krama-id", msg.Recipient)
	}
}

// handleProposal processes a proposal message by validating its aggregated signature,
// verifying quorum conditions, validating the highest QC, and executing the tesseract.
/*
	- validate aggregated signature
	- verify quorum conditions
	- verify highest Qc
	- validate tesseract
*/
func (k *Engine) handleProposal(ctx context.Context, cs *ktypes.ClusterState, proposal *ktypes.Proposal) error {
	k.logger.Debug("Handling proposal", "cluster-id", cs.ClusterID, "view", proposal.View())

	_, span := tracing.Span(ctx, "Krama.Handler", "handleProposal")
	defer span.End()

	initTime := time.Now()
	defer k.metrics.captureProposalValidationTime(initTime)

	trueIndices := proposal.PrepareQc.SignerIndices.GetTrueIndices()
	rawPreparedMsgs := make([][]byte, 0, len(trueIndices))

	for _, index := range trueIndices {
		view := proposal.PrepareQc.PeerViews[index]
		if view == nil {
			continue
		}

		pm := &ktypes.Prepared{
			View:  proposal.View(),
			Infos: proposal.PrepareQc.PeerViews[index],
		}

		rawData, err := pm.SignBytes()
		if err != nil {
			return err
		}

		rawPreparedMsgs = append(rawPreparedMsgs, rawData)
	}

	publicKeys, err := cs.Committee().UpdateValidatorResponse(ktypes.VoteCounter, trueIndices)
	if err != nil {
		return errors.Wrap(err, "failed to update node responses")
	}

	if !cs.IsContextQuorum(ktypes.VoteCounter) {
		return common.ErrContextQuorumFailed
	}

	verified, err := crypto.VerifyMultiSig(proposal.PrepareQc.Signature, rawPreparedMsgs, publicKeys)
	if err != nil {
		return errors.Wrap(err, "failed to verify prepare QC")
	}

	if !verified {
		return errors.Wrap(common.ErrSignatureVerificationFailed, "failed to verify prepare QC")
	}

	shouldReturn := false

	for _, peerIndex := range trueIndices {
		peerInfo := proposal.PrepareQc.PeerViews[peerIndex]
		slots, _, peerID, _ := cs.Committee().GetKramaID(int32(peerIndex))

		if cs.Committee().IsSlotsQuorum(slots) {
			continue
		}

		if err = k.updateHighestVI(cs, peerInfo, peerID, true); err != nil {
			k.logger.Error("failed to validate highest QC", "error", err, "peer-id", peerID)

			if errors.Is(err, common.ErrSyncRequestSent) {
				shouldReturn = true
			}
		}

		cs.Committee().UpdateResponse(ktypes.MajorityCounter, peerID)
	}

	if shouldReturn {
		return errors.New("failed to validate proposal or state is lagging")
	}

	lockedTS, status, err := k.getLockStatus(cs.HighestViewInfo(), cs.IxnsHash())
	if err != nil {
		return err
	}

	switch status {
	case invalidLock:
		return common.ErrInvalidLock
	case validLock:
		if lockedTS.Hash() != proposal.Tesseract.Hash() {
			return errors.New("invalid proposal tesseract")
		}
	default:
	}

	if lockedTS != nil && cs.IsTSStored() {
		return nil
	}

	// create a transition object with latest participant objects
	transition, err := k.state.LoadTransitionObjects(proposal.Ixs().Participants(), nil)
	if err != nil {
		return errors.Wrap(err, "failed to load transition objects")
	}

	if err = k.ExecuteAndValidateTS(proposal.Tesseract, transition); err != nil {
		return err
	}

	cs.SetStateTransition(transition)

	return nil
}

func hasLock(views common.Views) bool {
	for _, view := range views {
		if view.Qc == nil {
			continue
		}

		if view.Qc[0].Type == common.PREVOTE {
			return true
		}
	}

	return false
}

func (k *Engine) getTSFromCacheAndDB(tsHash common.Hash) (*common.Tesseract, error) {
	ts, err := k.getLockedTSFromCache(tsHash)
	if err == nil {
		return ts, nil
	}

	ts, err = k.lattice.GetTesseract(tsHash, false, true)
	if err == nil {
		return ts, nil
	}

	return k.safety.GetTesseract(tsHash)
}

// evaluateLockedTesseracts deletes outdated locked tesseracts
// by comparing the height and view number for each participant across the locked and committed tesseracts.
// Finally, it performs one of three actions: proposes a locked tesseract,
// proposes a new tesseract, or skips proposing a tesseract.
func (k *Engine) evaluateLockedTesseracts(views common.Views,
	clusterIxnsHash common.Hash,
) (*common.Tesseract, LockStatus, error) {
	var (
		lockedInfos = make(map[common.Hash]*lockedInfo)
		maxHeights  = make(map[identifiers.Identifier]*identifierInfo)
	)

	for _, view := range views {
		if view.Qc == nil {
			continue
		}

		k.logger.Debug("view details",
			"id", view.Qc[0].ID, "qc-type", view.Qc[0].Type, "view", view.Qc[0].View, "lock", view.Qc[0].LockType)

		qc := view.Qc[0]

		ts, err := k.getTSFromCacheAndDB(qc.TSHash)
		if err != nil {
			return nil, noLock, err
		}

		if qc.Type == common.PREVOTE {
			lockedInfos[view.Qc[0].TSHash] = &lockedInfo{
				view: qc.View,
				ts:   ts,
			}
		}

		for id, state := range ts.Participants() {
			if info, ok := maxHeights[id]; ok && info.height >= state.Height {
				continue
			}

			maxHeights[id] = &identifierInfo{
				height: state.Height,
				lock:   qc.Type,
				view:   qc.View,
			}
		}
	}

	for tsHash, lockInfo := range lockedInfos {
		lockedTS := lockInfo.ts
		remove := false

		for id, lockedState := range lockedTS.Participants() {
			if maxHeights[id].lock == common.PRECOMMIT && lockedState.Height <= maxHeights[id].height {
				remove = true

				break
			}

			if maxHeights[id].lock != common.PREVOTE {
				continue
			}

			if lockedState.Height < maxHeights[id].height ||
				(lockedState.Height == maxHeights[id].height && lockInfo.view < maxHeights[id].view) {
				remove = true

				break
			}
		}

		if remove {
			k.deleteLockedTesseractInfo(lockedTS)
			delete(lockedInfos, tsHash)
		}
	}

	switch len(lockedInfos) {
	case 0:
		return nil, noLock, nil
	default:
		for _, info := range lockedInfos {
			if info.ts.InteractionsHash() == clusterIxnsHash {
				return info.ts, validLock, nil
			}
		}
	}

	return nil, invalidLock, nil
}

// Consensus Locking Mechanism:
//
// 1. No identifier has a prevote lock:
//   - Propose a new Tesseract.
//   - Return status as "No Lock".
//
// 2. If any identifier has a lock, it will be evaluated.
func (k *Engine) getLockStatus(views common.Views, clusterIxnsHash common.Hash) (*common.Tesseract, LockStatus, error) {
	if !hasLock(views) {
		return nil, noLock, nil
	}

	return k.evaluateLockedTesseracts(views, clusterIxnsHash)
}

// shouldCreateJob checks if all the tesseracts for the view are found, if not found new job will be returned.
func (k *Engine) shouldCreateJob(msg *ktypes.Prepared) bool {
	for _, view := range msg.Infos {
		if view.Qc == nil {
			continue
		}

		if _, _, err := k.getTS(view.Qc[0].Type, view.Qc[0].TSHash, "", false); err != nil {
			return true
		}
	}

	return false
}

func (k *Engine) signalNewJob() {
	select {
	case k.workerSignal <- struct{}{}:
	default:
		k.logger.Error("failed to signal new prepared msg worker")
	}
}

func (k *Engine) verifyPreparedMsg(
	msg *ktypes.Prepared,
	sender identifiers.KramaID,
	cs *ktypes.ClusterState,
) error {
	if msg.Verified {
		return nil
	}

	valIndex, publicKey, duplicateVote := cs.HasKramaID(sender)
	if duplicateVote {
		return common.ErrDuplicateVote
	}

	if valIndex == -1 {
		return common.ErrPublicKeyNotFound
	}

	signBytes, err := msg.SignBytes()
	if err != nil {
		return err
	}

	verified, err := crypto.Verify(signBytes, msg.Signature, publicKey)
	if !verified || err != nil {
		return common.ErrSignatureVerificationFailed
	}

	return nil
}

/*
- Validate the viewID
- Validate the BLS signature of the prepareQc
- Update the highest view info by validating the peer info view and qc
- If Quorum conditions are met, execute the interactions and start the BFT system
*/
func (k *Engine) handlePrepared(
	ctx context.Context,
	clusterID common.ClusterID,
	msg *ktypes.Prepared,
	sender identifiers.KramaID,
) error {
	k.logger.Debug("Handling prepared", "cluster-id", clusterID, "sender", sender)

	_, span := tracing.Span(ctx, "Krama.Handler", "handlePrepared")
	defer span.End()

	slot := k.slots.GetSlot(clusterID)
	cs := slot.ClusterState()

	if cs.CurrentView().ID() != msg.View {
		// leader view and the local view should match
		return common.ErrInvalidView
	}

	if err := k.verifyPreparedMsg(msg, sender, cs); err != nil {
		return err
	}

	if ok := k.shouldCreateJob(msg); ok {
		if err := k.preparedMsgQueue.push(clusterID, msg, sender); err != nil {
			k.logger.Error("failed to add prepared msg job", err)
		}

		k.signalNewJob()

		return nil
	}

	// save the view info sent by the peer
	cs.Committee().UpdateNodePreparedMsg(sender, msg)

	if !cs.IsContextQuorum(ktypes.VoteCounter) {
		return nil
	}

	viewInfos, err := cs.Committee().ViewInfos()
	if err != nil {
		return err
	}

	for peerIndex, info := range viewInfos {
		if info == nil { // ignore un-responded nodes
			continue
		}

		slots, _, peerID, _ := cs.Committee().GetKramaID(int32(peerIndex))

		if cs.Committee().IsSlotsQuorum(slots) {
			continue
		}

		if err := k.updateHighestVI(cs, info, peerID, true); err != nil {
			k.logger.Error("failed to validate highest QC as operator", "error", err, "peer-id", peerID)
		}

		cs.Committee().UpdateResponse(ktypes.MajorityCounter, peerID)
	}

	if swapped := slot.UpdateStage(ktypes.PrepareStage, ktypes.PreparedStage); !swapped {
		return nil
	}

	lockedTS, status, err := k.getLockStatus(slot.ClusterState().HighestViewInfo(), slot.ClusterState().IxnsHash())
	if err != nil {
		return err
	}

	ts, err := k.buildProposalTS(lockedTS, slot.ClusterState())
	if err != nil {
		k.logger.Error("Error creating proposal", "error", err)

		return err
	}

	preparedQc, err := k.createPreparedQc(ctx, slot.ClusterState())
	if err != nil {
		return err
	}

	if status == invalidLock {
		slot.BftOutboundChan <- ktypes.ConsensusMessage{
			PeerID:  slot.ClusterState().SelfKramaID(),
			Payload: ktypes.NewProposal(preparedQc, ts),
		}

		return nil
	}

	// update the cluster state with preparedQC and tesseract
	cs.SetPreparedQc(preparedQc)
	cs.SetTesseract(ts)

	k.metrics.captureICSCreationTime(slot.InitTime)

	ixnsHash := cs.IxnsHash()

	// Start the BFT system
	bft := kbft.NewKBFTService(
		ctx,
		k.selfID,
		slot.ViewTimeoutDeadline(),
		k.cfg,
		k.vault, cs.VoteSet(), slot, k.safety, cs.IsTSStored(), k.finalizedTSHandler,
		kbft.WithLogger(k.logger.With("cluster-id", clusterID)),
		kbft.WithWal(kbft.NullWal{}),
		kbft.WithEvidence(kbft.NewEvidence(ixnsHash, cs.Operator(), cs.Size())),
		kbft.WithMux(k.consensusMux))

	go k.startBFT(bft, slot)

	return nil
}

func (k *Engine) fetchAndStoreRandomNodes(ctx context.Context, cs *ktypes.ClusterState) error {
	// choose the stochastic nodes using flux
	contextNodes, _, _ := ktypes.DistinctNodes(k.selfID, cs.Committee().Sets)

	stochasticNodes, err := k.getStochasticNodes(ctx, cs, common.StochasticSetSize, contextNodes)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve random nodes")
	}

	vals, err := cs.SystemObject.GetValidators(stochasticNodes...)
	if err != nil {
		return err
	}
	// update the committee with the stochastic node set
	cs.AppendNodeSet(
		ktypes.NewNodeSet(vals, uint32(common.StochasticSetSize)),
	)

	return nil
}

func (k *Engine) sendPrepare(ctx context.Context, cs *ktypes.ClusterState) error {
	k.logger.Debug(
		"send prepare",
		"ix-Hash", cs.Ixns().Hashes(),
		"cluster-id", cs.ClusterID,
		"ids", cs.Participants.IDs(),
	)

	_, span := tracing.Span(ctx, "Krama.Handler", "sendPrepare")
	defer span.End()

	prepareMsg := &ktypes.Prepare{
		View: cs.CurrentView().ID(),
		Ixns: cs.Ixns().Hashes(),
	}

	if k.trustedPeersPresent {
		// Choose 10 trusted nodes, as a maximum of 8 nodes are required while updating the context
		cs.TrustedPeers = k.getTrustedPeers(10)
	}

	if err := k.fetchAndStoreStochasticNodes(ctx, cs); err != nil {
		return err
	}

	failedCount, err := k.sendPrepareMsg(ctx, cs.ClusterID, prepareMsg, cs.Committee(), cs.LocalViewInfo())
	if err != nil {
		return nil
	}

	k.logger.Error("failed to send prepare msg", "count", failedCount)

	if err := k.publishEventPrepare(prepareMsg); err != nil {
		k.logger.Error("failed to publish prepare msg event", "err", err)
	}

	return nil
}

func (k *Engine) loadViewInfo(ps []identifiers.Identifier) ([]*common.ViewInfo, error) {
	infos := make([]*common.ViewInfo, 0, len(ps))

	for _, id := range ps {
		isRegistered, err := k.state.IsAccountRegistered(id)
		if err != nil {
			return nil, err
		}

		if !isRegistered {
			infos = append(infos, &common.ViewInfo{
				ID:          id,
				LastView:    0,
				CurrentLock: k.accountLockStatus(id),
				Qc:          nil,
			})

			continue
		}

		safetyData, err := k.safety.GetLatestSafetyInfo(id)
		if err != nil {
			return nil, err
		}

		infos = append(infos, &common.ViewInfo{
			ID:          id,
			LastView:    safetyData.LastView(),
			CurrentLock: k.accountLockStatus(id),
			Qc:          safetyData.Qc,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return bytes.Compare(infos[i].ID.Bytes(), infos[j].ID.Bytes()) < 0
	})

	return infos, nil
}

// validateQcAndUpdateSafety traverses all QCs of a given participant, skipping genesis tesseracts.
// It validates each QC using the ICS committee fetched from the tesseract.
// If the tesseract is not available locally, it is fetched from a remote node.
// If the node is lagging in state, syncing is triggered, and validation is skipped as state is unavailable.
// If a tesseract is fetched from a remote node with a prevote QC:
// - Update safety info in the DB if the remote height and view are higher than the local values.
// - Also update safety info if the node has not yet stored the identifier.
func (k *Engine) validateQcAndUpdateSafety(cs *ktypes.ClusterState, remote *common.ViewInfo,
	peerID identifiers.KramaID, isProposal bool,
) error {
	k.logger.Debug(
		"validating peer qc",
		"peer-id", peerID,
		"id", remote.ID,
	)

	accMetaInfo, _ := k.db.GetAccountMetaInfo(remote.ID)

	for _, qc := range remote.Qc {
		if qc.View == common.GenesisView {
			continue
		}

		k.logger.Debug(
			"validating qc",
			"view", qc.View,
			"id", qc.ID,
			"type", qc.Type,
			"signers", qc.SignerIndices.String(),
			"peer-id", peerID,
			"signature", qc.Signature,
		)

		ts, shouldStore, err := k.getTS(qc.Type, qc.TSHash, peerID, isProposal)
		if err != nil {
			return err
		}

		ics, err := k.FetchICSCommittee(ts, ts.CommitInfo(), cs.SystemObject)
		if err != nil {
			return err
		}

		isVerified, err := k.verifyQc(qc.View, ics, qc)
		if err != nil {
			return errors.Wrap(err, "failed to verify QC")
		}

		if !isVerified {
			return common.ErrSignatureVerificationFailed
		}

		// TODO move this safety info update logic to the parent method
		if !shouldStore || qc.Type != common.PREVOTE {
			continue
		}

		remoteQc := qc

		if accMetaInfo != nil {
			// if remote height is less than or equal to local height, we can skip
			if ts.Height(remote.ID) <= accMetaInfo.Height {
				continue
			}

			localSafetyInfo, err := k.safety.GetLatestSafetyInfo(remote.ID)
			if err != nil {
				return err
			}

			// if local view is greater than or equal to remote view, we can skip
			if localSafetyInfo.LastView() >= remoteQc.View {
				continue
			}
		}

		if err = k.safety.UpdateSafetyInfo(ktypes.NewProposal(nil, ts), remoteQc); err != nil {
			return err
		}
	}

	return nil
}

// shouldReplace tells whether to replace highest view info with remote peer view info,
// given that highest view info is already initialized
func (k *Engine) shouldReplace(highestViewInfo *common.ViewInfo, peerViewInfo *common.ViewInfo) bool {
	if highestViewInfo == nil {
		return true
	}

	if len(peerViewInfo.Qc) > 0 {
		if highestViewInfo.LastView < peerViewInfo.Qc[0].View {
			return true
		}

		if highestViewInfo.LastView == peerViewInfo.Qc[0].View && highestViewInfo.Qc != nil &&
			highestViewInfo.Qc[0].Type < peerViewInfo.Qc[0].Type {
			return true
		}
	}

	return false
}

// This logic addresses two main decisions:
//
// 1. **When to store the safety info:**
//    - Store only if the incoming (remote) height and prevote QC are better than the local view’s height and view.
//    - Specifically:
//      - If the remote QC height is **less than** the local view height, do **not** update.
//      - If the remote QC height is **equal to** the local view height **and**
//        the local view is lesser (i.e. local view number is smaller), do **not** update.
//
// 2. **When to update the highest view info:**
//    - If the current highest view info is **nil**, always update it with the remote view info.
//    - If it is **not nil**, update it **only if** the remote peer’s view info is better.
//      - Specifically:
//        - If the remote QC height is **less than** the highest view info QC height, do **not** update.
//        - If the remote QC height is **equal to** the highest view info QC height **and**
//          the remote peer view is lesser (i.e. its view number is smaller), do **not** update.

// updateHighestVI updates the highest view info by validating the peer info view and qc.
func (k *Engine) updateHighestVI(cs *ktypes.ClusterState, peerInfo common.Views,
	peerID identifiers.KramaID, isProposal bool,
) error {
	if len(peerInfo) != len(cs.HighestViewInfo()) {
		return errors.New("view count doesn't match")
	}

	for i, viewInfo := range cs.LocalViewInfo() {
		if peerInfo[i].ID != viewInfo.ID {
			return errors.New("view order doesn't match")
		}

		if shouldReplace := k.shouldReplace(cs.HighestViewInfo()[i], peerInfo[i]); !shouldReplace {
			continue
		}

		// verify the bls signature of the QC
		if err := k.validateQcAndUpdateSafety(cs, peerInfo[i], peerID, isProposal); err != nil {
			return err
		}

		cs.HighestViewInfo()[i] = peerInfo[i]
	}

	return nil
}

// createPreparedQc creates a preparedQC by aggregating all PreparedMsg sent by the replicas.
func (k *Engine) createPreparedQc(
	ctx context.Context,
	cs *ktypes.ClusterState,
) (*ktypes.PreparedInfo, error) {
	initTime := time.Now()
	defer k.metrics.capturePrepareQCSigAggregationTime(initTime)

	infos, signatures, err := cs.Committee().ViewInfosAndSignatures()
	if err != nil {
		return nil, err
	}

	aggSign, err := crypto.AggregateSignatures(signatures)
	if err != nil {
		return nil, err
	}

	return &ktypes.PreparedInfo{
		View:          cs.CurrentView().ID(),
		PeerViews:     infos,
		SignerIndices: cs.Committee().GetVoteset(ktypes.VoteCounter),
		Signature:     aggSign,
	}, nil
}

func (k *Engine) createNewTSFromLockedTS(cs *ktypes.ClusterState, ts *common.Tesseract) (*common.Tesseract, error) {
	return common.NewTesseract(
		ts.Participants(),
		ts.InteractionsHash(),
		ts.ReceiptsHash(),
		ts.Epoch(),
		ts.Timestamp(),
		ts.FuelUsed(),
		ts.FuelLimit(),
		ts.ConsensusInfo(),
		nil,
		cs.SelfKramaID(),
		cs.Ixns(), // use ixns from cluster since locked ts from a remote node may not have them
		ts.Receipts(),
		&common.CommitInfo{
			Operator:                  cs.SelfKramaID(),
			ClusterID:                 cs.ClusterID,
			View:                      cs.CurrentView().ID(),
			RandomSet:                 cs.GetRandomNodes(),
			RandomSetSizeWithoutDelta: cs.Committee().RandomSetSizeWithOutDelta(),
		}), nil
}

// createProposalTS either creates a proposal tesseract from the locked tesseract
// or creates a new tesseract by executing the interactions.
func (k *Engine) createProposalTS(
	lockedTS *common.Tesseract,
	cs *ktypes.ClusterState,
) (*common.Tesseract, error) {
	if lockedTS != nil {
		if _, err := k.executionInteractions(cs); err != nil {
			return nil, err
		}

		return k.createNewTSFromLockedTS(cs, lockedTS)
	}

	return k.createProposalTesseract(cs)
}

func (k *Engine) accountLockStatus(id identifiers.Identifier) common.LockType {
	info, ok := k.accountLocks[id]
	if !ok {
		return common.NoLock
	}

	return info.LockType
}

func (k *Engine) publishEventPrepare(prepare *ktypes.Prepare) error {
	if k.consensusMux != nil {
		return k.consensusMux.Post(eventPrepare{
			prepare: prepare,
		})
	}

	return nil
}

func (k *Engine) publishEventPrepared(prepared *ktypes.Prepared) error {
	if k.consensusMux != nil {
		return k.consensusMux.Post(eventPrepared{
			prepared: prepared,
		})
	}

	return nil
}

func (k *Engine) publishEventCleanup(id common.ClusterID) error {
	if k.consensusMux != nil {
		return k.consensusMux.Post(eventCleanup{
			clusterID: id,
		})
	}

	return nil
}

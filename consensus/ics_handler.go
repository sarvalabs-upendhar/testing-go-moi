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
					k.vault, cs.VoteSet(), slot, k.safety, k.finalizedTesseractHandler,
					kbft.WithLogger(k.logger.With("cluster-id", clusterID)),
					kbft.WithWal(kbft.NullWal{}),
					kbft.WithEvidence(icsEvidence))

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

	publicKeys, err := cs.Committee().UpdateValidatorResponse(trueIndices)
	if err != nil {
		return errors.Wrap(err, "failed to update node responses")
	}

	if !cs.IsContextQuorum() {
		return common.ErrContextQuorumFailed
	}

	if !cs.IsRandomQuorum() {
		return common.ErrRandomQuorumFailed
	}

	verified, err := crypto.VerifyMultiSig(proposal.PrepareQc.Signature, rawPreparedMsgs, publicKeys)
	if err != nil {
		return errors.Wrap(err, "failed to verify prepare QC")
	}

	if !verified {
		return errors.Wrap(common.ErrSignatureVerificationFailed, "failed to verify prepare QC")
	}

	for peerIndex, info := range proposal.PrepareQc.PeerViews {
		_, _, peerID, _ := cs.Committee().GetKramaID(int32(peerIndex))
		if err = k.updateHighestVI(cs, info, peerID); err != nil {
			k.logger.Error("failed to validate highest QC", "error", err, "peer-id", peerID)
		}
	}

	// create a transition object with latest participant objects
	transition, err := k.state.LoadTransitionObjects(proposal.Ixs().Participants(), nil)
	if err != nil {
		return errors.Wrap(err, "failed to load transition objects")
	}

	if err = k.ExecuteAndValidate(proposal.Tesseract, transition); err != nil {
		return err
	}

	cs.SetStateTransition(transition)

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

	if k.view.Load() != msg.View {
		// leader view and the local view should match
		return errors.New("invalid view")
	}

	slot := k.slots.GetSlot(clusterID)
	cs := k.slots.GetSlot(clusterID).ClusterState()

	valIndex, publicKey, conflictingVote := cs.HasKramaID(sender)
	if conflictingVote {
		return common.ErrConflictingVote
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

	if err = k.updateHighestVI(cs, msg.Infos, sender); err != nil {
		return errors.Wrap(err, "failed to update highest view info")
	}

	// save the view info sent by the peer
	cs.Committee().UpdateNodePreparedMsg(sender, msg)

	if !cs.IsContextQuorum() || !cs.IsRandomQuorum() {
		return nil
	}

	if swapped := slot.UpdateStage(ktypes.PrepareStage, ktypes.PreparedStage); !swapped {
		return nil
	}

	localVI := slot.ClusterState().LocalViewInfo()

	lockedTS := make(map[identifiers.Identifier]*common.Tesseract)

	for index, highestVI := range slot.ClusterState().HighestViewInfo() {
		// Qc will be NIL for new accounts
		if highestVI.Qc == nil && localVI[index].Qc == nil {
			continue
		}

		highestViewTS, err := k.getTS(highestVI.Qc[0].TSHash, "")
		if err != nil {
			k.logger.Error("failed to load highest view tesseract", "ts-hash", highestVI.Qc[0].TSHash)

			return err
		}

		currentViewTS, err := k.getTS(localVI[index].Qc[0].TSHash, "")
		if err != nil {
			k.logger.Error("failed to load local view tesseract", "ts-hash", localVI[index].Qc[0].TSHash)

			return err
		}

		heightDiff := highestViewTS.Height(highestVI.ID) - currentViewTS.Height(highestVI.ID)

		if heightDiff == 0 {
			continue
		}

		if heightDiff > 1 {
			// TODO: Trigger sync
			return nil
		}

		if highestVI.Qc[0].Type == common.PRECOMMIT {
			return nil // TODO: Trigger sync
		}

		if highestVI.Qc[0].Type == common.PREVOTE {
			lockedTS[highestVI.Qc[0].ID] = highestViewTS

			continue
		}
	}

	if len(localVI) > 0 && localVI[0].Qc != nil {
		if localVI[0].Qc[0].Type == common.PREVOTE {
			currentViewTS, err := k.getTS(localVI[0].Qc[0].TSHash, "")
			if err != nil {
				k.logger.Error("failed to load local view tesseract", "ts-hash", localVI[0].Qc[0].TSHash)

				return err
			}

			lockedTS[localVI[0].Qc[0].ID] = currentViewTS
		}
	}

	ts, err := k.createProposalTS(lockedTS, slot.ClusterState())
	if err != nil {
		k.logger.Error("Error creating proposal", "error", err)

		return err
	}

	prepareQc, err := k.createPrepareQc(ctx, slot.ClusterState())
	if err != nil {
		return err
	}

	// update the cluster state with prepareQC and tesseract
	cs.SetPrepareQc(prepareQc)
	cs.SetTesseract(ts)

	k.metrics.captureICSCreationTime(slot.InitTime)

	ixnsHash := cs.IxnsHash()

	// Start the BFT system
	bft := kbft.NewKBFTService(
		ctx,
		k.selfID,
		slot.ViewTimeoutDeadline(),
		k.cfg,
		k.vault, cs.VoteSet(), slot, k.safety, k.finalizedTesseractHandler,
		kbft.WithLogger(k.logger.With("cluster-id", clusterID)),
		kbft.WithWal(kbft.NullWal{}),
		kbft.WithEvidence(kbft.NewEvidence(ixnsHash, cs.Operator(), cs.Size())))

	go k.startBFT(bft, slot)

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
		View: cs.CurrentView(),
		Ixns: cs.Ixns().Hashes(),
		Ps:   cs.Participants.IDs(),
	}

	if k.trustedPeersPresent {
		// Choose 10 trusted nodes, as a maximum of 8 nodes are required while updating the context
		cs.TrustedPeers = k.getTrustedPeers(10)
	}

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

	failedCount, err := k.sendPrepareMsg(ctx, cs.ClusterID, prepareMsg, cs.Committee(), cs.LocalViewInfo())
	if err != nil {
		return nil
	}

	k.logger.Error("failed to send prepare msg", "count", failedCount)

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

func (k *Engine) validatePeerHighestQc(remote *common.ViewInfo, peerID identifiers.KramaID) error {
	k.logger.Debug(
		"validating peer qc",
		"peer-id", peerID,
		"id", remote.ID,
	)

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

		ts, err := k.getTS(qc.TSHash, peerID)
		if err != nil {
			return err
		}

		cs := k.slots.GetSlot(ts.ClusterID()).ClusterState()

		ics, err := k.GetICSCommittee(ts, ts.CommitInfo(), cs.SystemObject)
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
	}

	return nil
}

// updateHighestVI updates the highest view info by validating the peer info view and qc.
func (k *Engine) updateHighestVI(cs *ktypes.ClusterState, peerInfo common.Views, peerID identifiers.KramaID) error {
	if len(peerInfo) != len(cs.HighestViewInfo()) {
		return errors.New("view count doesn't match")
	}

	for i, viewInfo := range cs.HighestViewInfo() {
		// check if peer viewID is greater than our viewID
		if peerInfo[i].LastView <= viewInfo.LastView {
			continue
		}

		if peerInfo[i].ID != viewInfo.ID {
			return errors.New("view order doesn't match")
		}

		// verify the bls signature of the QC
		if err := k.validatePeerHighestQc(viewInfo, peerID); err != nil {
			return err
		}

		cs.HighestViewInfo()[i] = peerInfo[i]
	}

	return nil
}

// createPrepareQc creates a prepareQC by aggregating all PreparedMsg sent by the replicas.
func (k *Engine) createPrepareQc(
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
		View:          cs.CurrentView(),
		PeerViews:     infos,
		SignerIndices: cs.Committee().GetVoteset(),
		Signature:     aggSign,
	}, nil
}

func (k *Engine) createNewTSFromLockedTS(cs *ktypes.ClusterState, ts *common.Tesseract) (*common.Tesseract, error) {
	if _, err := k.executionInteractions(cs); err != nil {
		return nil, err
	}

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
		ts.Interactions(),
		ts.Receipts(),
		&common.CommitInfo{
			Operator:                  cs.SelfKramaID(),
			ClusterID:                 cs.ClusterID,
			View:                      cs.CurrentView(),
			RandomSet:                 cs.GetRandomNodes(),
			RandomSetSizeWithoutDelta: cs.Committee().RandomSetSizeWithOutDelta(),
		}), nil
}

// createProposalTS either creates a proposal tesseract from the locked tesseract
// or creates a new tesseract by executing the interactions.
func (k *Engine) createProposalTS(
	lockedTS map[identifiers.Identifier]*common.Tesseract,
	cs *ktypes.ClusterState,
) (*common.Tesseract, error) {
	var lockTS *common.Tesseract

	clusterIxnsHash := cs.IxnsHash()

	for id, ts := range lockedTS {
		if ts.InteractionsHash() != clusterIxnsHash {
			k.logger.Error(
				"Ixns hash doesn't match with locked tesseract",
				"id", id,
				"locked-ts", ts.Hash(),
				"ixns-hash", clusterIxnsHash,
			)

			return nil, errors.New("ts lock mismatch")
		}

		lockTS = ts
	}

	if len(lockedTS) == 0 {
		return k.createProposalTesseract(cs)
	}

	return k.createNewTSFromLockedTS(cs, lockTS)
}

func (k *Engine) accountLockStatus(id identifiers.Identifier) common.LockType {
	info, ok := k.accountLocks[id]
	if !ok {
		return common.NoLock
	}

	return info.LockType
}

package kbft

import (
	"context"
	"sync"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/consensus/safety"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/telemetry/tracing"
)

const (
	TestBFTimeout = 1 * time.Second
	MaxBFTimeout  = 4 * time.Second
	SlotDuration  = 2 * time.Second
)

type vault interface {
	Sign(data []byte, sigType mudracommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
	KramaID() identifiers.KramaID
}

// KBFT is a struct that represents the runner for the Krama Byzantine Fault Tolerant consensus engine
type KBFT struct {
	ViewState
	logger           hclog.Logger
	id               identifiers.KramaID
	config           *config.ConsensusConfig
	mx               sync.Mutex
	inboundMsgChan   chan ktypes.ConsensusMessage
	selfMsgChan      chan ktypes.ConsensusMessage
	outboundMsgChan  chan ktypes.ConsensusMessage
	toTicker         *Ticker
	ics              *ktypes.ClusterState
	nSteps           int
	closeChan        chan error
	ctx              context.Context
	ctxCancel        context.CancelFunc
	evidence         *Evidence
	vault            vault
	wal              WAL
	mux              *utils.TypeMux
	tesseractHandler func(tesseract *common.Tesseract) error
	safety           *safety.ConsensusSafety
	exitChan         chan error
	operator         identifiers.KramaID
	IsTSStored       bool
}

// NewKBFTService initializes a new KBFT instance for the ICS, this service holds the core logic for BFT agreement
func NewKBFTService(
	ctx context.Context,
	operator identifiers.KramaID,
	viewDeadline time.Time,
	config *config.ConsensusConfig,
	vault vault,
	voteset *ktypes.HeightVoteSet,
	slot *ktypes.Slot,
	safety *safety.ConsensusSafety,
	isTSStored bool,
	tesseractHandler func(tesseract *common.Tesseract) error,
	opts ...Option,
) *KBFT {
	k := &KBFT{
		operator:         operator,
		id:               slot.ClusterState().SelfKramaID(),
		config:           config,
		outboundMsgChan:  slot.BftOutboundChan,
		inboundMsgChan:   slot.BftInboundChan,
		selfMsgChan:      make(chan ktypes.ConsensusMessage, 1000),
		vault:            vault,
		ics:              slot.ClusterState(),
		tesseractHandler: tesseractHandler,
		closeChan:        make(chan error),
		exitChan:         slot.BftStopChan,
		safety:           safety,
		IsTSStored:       isTSStored,
	}

	for _, opt := range opts {
		opt(k)
	}

	k.ctx, k.ctxCancel = context.WithTimeout(ctx, time.Until(viewDeadline))
	k.updateToState(voteset)

	return k
}

func (kbft *KBFT) updateToState(voteset *ktypes.HeightVoteSet) {
	kbft.toTicker = NewTicker(kbft.logger)
	kbft.Heights = kbft.ics.NewHeights()
	kbft.updateViewStep(kbft.ics.CurrentView().ID(), ViewStepNewHeight)
	kbft.Proposal = nil
	kbft.ProposalTS = nil

	kbft.Votes = voteset
	kbft.CommitView = 0
	kbft.stepChange()
}

func (kbft *KBFT) scheduleView() {
	kbft.logger.Info("Scheduling View 0. TO:")
	kbft.scheduleTimeout(100*time.Millisecond, kbft.Heights, kbft.View, ViewStepNewHeight)
}

func (kbft *KBFT) handler(maxSteps int) error {
	defer func() {
		close(kbft.outboundMsgChan)
		close(kbft.selfMsgChan)

		kbft.toTicker.Stop()
		kbft.toTicker.Close()
		kbft.ctxCancel()
	}()

	for {
		if maxSteps > 0 {
			if kbft.nSteps >= maxSteps {
				kbft.nSteps = 0

				return errors.New("maximum steps reached")
			}
		}

		viewState := kbft.ViewState

		select {
		case err := <-kbft.closeChan:
			return err

		case <-kbft.ctx.Done():
			kbft.logger.Info("KBFT timeout occurred")

			return kbft.ctx.Err()

		case msg, ok := <-kbft.inboundMsgChan:
			if !ok {
				kbft.logger.Debug("Inbound message channel closed")

				return nil
			}

			kbft.logger.Debug("Handling external message", "sender", msg.PeerID)

			if err := kbft.handleMsg(msg); err != nil {
				kbft.logger.Error("Error handling external message", "sender", msg.PeerID)
			}

		case msg := <-kbft.selfMsgChan:
			kbft.logger.Debug("Handling internal message")

			if err := kbft.wal.WriteSync(msg, kbft.ics.ClusterID); err != nil {
				kbft.logger.Error("Error writing to write-ahead logger", "err", err)
			}

			if err := kbft.handleMsg(msg); err != nil {
				kbft.logger.Error("Error handling internal message", "sender", msg.PeerID)
			}

		case t := <-kbft.toTicker.TimeOutChan():
			kbft.logger.Trace("Handling timeout")
			kbft.handleTimeout(t, viewState)
		}
	}
}

func (kbft *KBFT) handleTimeout(ti timeoutInfo, r ViewState) {
	if r.View > ti.View || (ti.View == r.View && ti.Step < r.Step) {
		kbft.logger.Debug(
			"Returning from time out",
			"view-height", r.Heights,
			"timeout-view", ti.View,
			"view-number", r.View,
			"timeout-step", ti.Step,
			"view-step", r.Step,
		)

		return
	}

	kbft.mx.Lock()
	defer kbft.mx.Unlock()

	if ti.Step == ViewStepNewHeight {
		kbft.enterNewView(ti.View)
	}
}

func (kbft *KBFT) handleMsg(msg ktypes.ConsensusMessage) error {
	spanCtx, span := tracing.Span(kbft.ctx, "Krama.KBFT", "handleMsg")

	kbft.mx.Lock()
	defer func() {
		kbft.mx.Unlock()
		span.End()
	}()

	msgPayload, peerID := msg.Payload, msg.PeerID

	switch m := msgPayload.(type) {
	case *ktypes.Proposal:
		kbft.logger.Trace("Proposal message received", "from", peerID)

		if err := kbft.setProposal(m); err != nil {
			kbft.logger.Error("failed to set proposal", "err", err)
		}

	case *ktypes.Vote:
		kbft.logger.Trace("Vote message received", "vote-type", m.Type, "from", peerID)

		_, err := kbft.addVote(spanCtx, m, peerID)
		if err != nil {
			kbft.logger.Error("failed to add vote", "err", err)

			return nil
		}
	}

	return nil
}

func (kbft *KBFT) enterNewView(view uint64) {
	if view < kbft.View ||
		(kbft.View == view && kbft.Step != ViewStepNewHeight) {
		return
	}

	kbft.logger.Trace("Entering new view", "view", view)
	kbft.updateViewStep(view, ViewStepNewView)
	//	kbft.scheduleTimeout(kbft.viewTimeout(view), heights, view, ViewStepNewView)

	if view != 0 {
		kbft.Proposal = nil
		kbft.ProposalTS = nil

		if err := kbft.publishEventPolka(kbft.viewStateEvent()); err != nil {
			kbft.logger.Error("failed to publish new view step", "err", err)
		}
	}

	if err := kbft.publishEventNewView(kbft.viewStateEvent()); err != nil {
		kbft.logger.Error("failed to publish new view", "err", err)
	}

	// Now ready to enter propose
	kbft.enterPropose(view)
}

func (kbft *KBFT) isProposalReceived() bool {
	if kbft.Proposal == nil || kbft.ProposalTS == nil {
		return false
	}

	return true
}

func (kbft *KBFT) enterPropose(view uint64) {
	if kbft.View > view ||
		(kbft.View == view && kbft.Step >= ViewStepPropose) {
		return
	}

	defer func() {
		kbft.updateViewStep(view, ViewStepPropose)
		kbft.stepChange()

		if kbft.isProposalReceived() {
			kbft.enterPrevote(kbft.View)
		}
	}()

	// kbft.scheduleTimeout(kbft.proposeTimeout(view), heights, view, ViewStepPropose)

	if kbft.vault == nil {
		return
	}

	// if _, _, exists := kbft.ics.HasKramaID(kbft.vault.KramaID()); !exists {
	//	kbft.logger.Error("Validator not found in ICS set")
	//
	//	return
	//}

	if !kbft.isLeader(view, kbft.id) {
		return
	}

	if err := kbft.createProposal(view); err != nil {
		kbft.Close(err)
	}
}

// createProposal will create a proposal message for the given height,view and tesseract
func (kbft *KBFT) createProposal(view uint64) error {
	kbft.logger.Info("Creating proposal", "view", view)

	proposal := ktypes.NewProposal(kbft.ics.PrepareQc(), kbft.ics.Tesseract())

	// Send an internal message
	kbft.sendInternalMessage(
		ktypes.ConsensusMessage{
			PeerID:  kbft.id,
			Payload: proposal,
		},
	)

	return nil
}

func (kbft *KBFT) setProposal(p *ktypes.Proposal) error {
	if kbft.Proposal != nil {
		return errors.New("proposal already set")
	}

	// Don't compare heights, if the tesseract is already stored
	if (!kbft.IsTSStored && !areHeightsEqual(kbft.Heights, p.Heights())) || p.View() != kbft.View {
		return errors.New("invalid height or view")
	}

	kbft.Proposal = p

	// set the proposal tesseract
	kbft.ProposalTS = p.Tesseract

	kbft.Votes.SetTSHash(kbft.ProposalTS.Hash())

	if kbft.isLeader(kbft.View, kbft.id) {
		kbft.sendExternalMessage(ktypes.ConsensusMessage{
			PeerID:  kbft.id,
			Payload: p.Copy(),
		})

		if err := kbft.publishEventProposal(p); err != nil {
			kbft.logger.Error("failed to publish proposal", "err", err)
		}
	}

	if kbft.Step <= ViewStepPropose && kbft.isProposalReceived() {
		kbft.enterPrevote(kbft.View)
	}

	return nil
}

func (kbft *KBFT) enterPrevote(view uint64) {
	kbft.logger.Trace("Entered pre-vote")

	if kbft.View > view || (kbft.View == view && kbft.Step >= ViewStepPrevote) {
		return
	}

	defer func() {
		kbft.updateViewStep(view, ViewStepPrevote)
		kbft.stepChange()
	}()

	if kbft.ProposalTS == nil {
		kbft.logger.Trace("Proposal tesseract is nil")
		kbft.sendVote(common.PREVOTE, common.NilHash, kbft.isLeader(view, kbft.id))

		return
	}

	kbft.sendVote(common.PREVOTE, kbft.ProposalTS.Hash(), kbft.isLeader(view, kbft.id))
}

func (kbft *KBFT) enterPreCommit(view uint64) {
	kbft.logger.Trace("Entered pre-commit", "view", view)

	if kbft.Step >= ViewStepPrecommit {
		return
	}

	defer func() {
		kbft.updateViewStep(view, ViewStepPrecommit)
		kbft.stepChange()
	}()

	// Before preCommit check for >2/3 preVotes
	tsHash, ok := kbft.Votes.Prevotes().SuperMajority()
	if !ok {
		return // Log that we have entered precommit without super majority
	}

	if err := kbft.publishEventPolka(kbft.viewStateEvent()); err != nil {
		kbft.logger.Error("failed to publish polka", "err", err)
	}

	// If Leader, send the prevote QC
	if kbft.isLeader(view, kbft.id) {
		kbft.sendQc(common.PREVOTE, tsHash)
	}

	if !kbft.ProposalTS.CompareHash(tsHash) {
		kbft.logger.Error(
			"Proposal doesn't match with super majority",
			"proposal", kbft.Proposal.Tesseract.Hash(),
			"view", kbft.View,
			"ts-hash", tsHash)

		return
	}

	qc, err := kbft.Votes.GetQC(tsHash, kbft.View, common.PREVOTE)
	if err != nil {
		kbft.logger.Error("failed to fetch prevote Qc", "error", err, "view", kbft.View, "ts-hash", tsHash)

		return
	}

	if !kbft.IsTSStored {
		if err = kbft.safety.UpdateSafetyInfo(kbft.Proposal, qc); err != nil {
			kbft.logger.Error("failed to store safety information", "error", err, "view", kbft.View, "ts-hash", tsHash)

			return
		}
	}

	kbft.sendVote(common.PRECOMMIT, tsHash, kbft.isLeader(view, kbft.id))
}

func (kbft *KBFT) enterCommit(view uint64) {
	if ViewStepCommit <= kbft.Step {
		return
	}

	defer func() {
		kbft.updateViewStep(kbft.View, ViewStepCommit)
		kbft.CommitView = view
		kbft.CommitTime = time.Now()

		kbft.stepChange()

		if kbft.IsTSStored { // avoid storing tesseract again if it is already stored
			kbft.Close(nil)

			return
		}

		kbft.finalizeCommit()
	}()

	tsHash, ok := kbft.Votes.Precommits().SuperMajority()
	if !ok {
		panic("expecting precommits")
	}

	qc, err := kbft.Votes.GetQC(tsHash, kbft.View, common.PRECOMMIT)
	if err != nil {
		kbft.logger.Error("failed to fetch precommit Qc", "error", err, "view", kbft.View, "ts-hash", tsHash)

		return
	}

	if !kbft.IsTSStored {
		if err = kbft.safety.UpdateSafetyInfo(kbft.Proposal, qc); err != nil {
			kbft.logger.Error("failed to store safety information", "error", err, "view", kbft.View, "ts-hash", tsHash)

			return
		}
	}

	if kbft.isLeader(view, kbft.id) {
		kbft.sendQc(common.PRECOMMIT, tsHash)
	}
}

func (kbft *KBFT) finalizeCommit() {
	tsHash, ok := kbft.Votes.Precommits().SuperMajority()
	if !ok || tsHash.IsNil() {
		kbft.logger.Trace("Majority is not available")

		return
	}

	if kbft.Proposal == nil || !kbft.Proposal.Tesseract.CompareHash(tsHash) {
		kbft.logger.Trace("Proposal tesseract doesn't match with the majority")

		return
	}

	if kbft.Step != ViewStepCommit {
		return
	}

	voteBitSet, sign := kbft.Votes.Precommits().GetQC()

	if err := kbft.updateConsensusInfoInTesseracts(voteBitSet, sign); err != nil {
		kbft.Close(err)

		return
	}

	if err := kbft.tesseractHandler(kbft.ProposalTS.Copy()); err != nil {
		kbft.Close(err)

		return
	}

	kbft.logger.Info("Consensus achieved on", "ts-hash", kbft.ProposalTS.Hash())
	// stop the bft engine
	kbft.Close(nil)
}

func (kbft *KBFT) addVote(ctx context.Context, v *ktypes.Vote, peerID identifiers.KramaID) (added bool, err error) {
	_, span := tracing.Span(ctx, "Krama.KBFT", "addVote")
	defer span.End()

	if kbft.View != v.View {
		return false, common.ErrInvalidView
	}

	if kbft.ProposalTS == nil {
		kbft.logger.Trace("Proposal tesseract not found", "ts-hash", v.TSHash)

		return added, err
	}

	if kbft.ProposalTS.Hash() != v.TSHash {
		kbft.logger.Trace(
			"Invalid tesseract hash", "proposal-ts-hash", kbft.ProposalTS.Hash(), "msg-ts-hash", v.TSHash,
		)

		return added, err
	}

	if v.IsQC {
		if !kbft.isLeader(v.View, peerID) {
			return false, errors.New("Invalid Leader")
		}

		added, err = kbft.Votes.AddQC(v)
	} else {
		added, err = kbft.Votes.AddVote(v)
	}

	if errors.Is(err, common.ErrConflictingVote) {
		kbft.evidence.AddVote(v)

		return added, err
	}

	if !added {
		return added, err
	}

	switch {
	case v.Type == common.PREVOTE:
		preVotes := kbft.Votes.Prevotes()

		switch {
		case kbft.View == v.View && kbft.Step >= ViewStepPrevote:
			tsHash, ok := preVotes.SuperMajority()
			if ok && (kbft.isProposalReceived() || !tsHash.IsNil()) {
				kbft.enterPreCommit(v.View)
			}

		default:
			kbft.logger.Debug("Proposal not available")
		}

	case v.Type == common.PRECOMMIT:
		preCommits := kbft.Votes.Precommits()

		tsHash, ok := preCommits.SuperMajority()
		if ok && kbft.isProposalReceived() {
			if !tsHash.IsNil() { // kbft.enterNewView(height, v.View) // kbft.enterPreCommit(height, v.View)
				kbft.enterCommit(v.View)
			}
		} else if kbft.View <= v.View && preCommits.HasMajorityAny() {
			kbft.logger.Error(
				"PreVote received for a future view",
				"current-view", kbft.View,
				"vote-view", v.View)
		}
	}

	return added, err
}

func (kbft *KBFT) sendInternalMessage(msg ktypes.ConsensusMessage) {
	kbft.selfMsgChan <- msg
}

func (kbft *KBFT) sendExternalMessage(msg ktypes.ConsensusMessage) {
	kbft.outboundMsgChan <- msg
}

func (kbft *KBFT) sendQc(msgType common.ConsensusMsgType, tsHash common.Hash) *ktypes.Vote {
	kbft.logger.Debug("Sending quorum certificate", "vote-type", msgType, "ts-hash", tsHash)

	if kbft.vault == nil {
		kbft.logger.Error("Vault service unavailable during sendVote")

		return nil
	}

	vote := &ktypes.Vote{
		Type:   msgType,
		View:   kbft.View,
		TSHash: tsHash,
		IsQC:   true,
	}

	switch msgType {
	case common.PREVOTE:
		vote.SignerIndices, vote.Signature = kbft.Votes.Prevotes().GetQC()
	case common.PRECOMMIT:
		vote.SignerIndices, vote.Signature = kbft.Votes.Precommits().GetQC()

	default:
		kbft.logger.Error("Invalid vote type")
	}

	if err := kbft.publishEventVote(vote); err != nil {
		kbft.logger.Error("Failed to publish vote", "err", err)
	}

	kbft.sendExternalMessage(ktypes.ConsensusMessage{PeerID: kbft.id, Payload: vote})

	return vote
}

// sendVote will send a signed vote message for the given vote-type and tesseract
func (kbft *KBFT) sendVote(msgType common.ConsensusMsgType, tsHash common.Hash, internalMessage bool) *ktypes.Vote {
	kbft.logger.Debug("Sending vote", "vote-type", msgType, "ts-hash", tsHash)

	if kbft.vault == nil {
		kbft.logger.Error("Vault service unavailable during sendVote")

		return nil
	}

	vote, err := kbft.signVote(msgType, tsHash)
	if err != nil {
		kbft.logger.Error("Error signing the vote message during sendVote", "err", err)

		return nil
	}

	if err := kbft.publishEventVote(vote); err != nil {
		kbft.logger.Error("failed to publish vote", "err", err)
	}

	if internalMessage {
		kbft.sendInternalMessage(ktypes.ConsensusMessage{PeerID: kbft.id, Payload: vote})

		return vote
	}

	kbft.sendExternalMessage(
		ktypes.ConsensusMessage{
			PeerID:    kbft.id,
			Recipient: kbft.ics.Operator(),
			Payload:   vote,
		})

	return vote
}

// signVote will create a vote message and sign it using the validator consensus key
func (kbft *KBFT) signVote(msgType common.ConsensusMsgType, tsHash common.Hash) (*ktypes.Vote, error) {
	valIndex, _, _ := kbft.ics.HasKramaID(kbft.id)

	if valIndex == -1 {
		return nil, common.ErrKramaIDNotFound
	}

	v := &ktypes.Vote{
		SignerIndex: valIndex,
		TSHash:      tsHash,
		View:        kbft.View,
		Type:        msgType,
	}

	rawData, err := v.SignBytes()
	if err != nil {
		return nil, err
	}

	sign, err := kbft.vault.Sign(rawData, mudracommon.BlsBLST)
	if err != nil {
		return nil, err
	}

	v.Signature = make([]byte, len(sign))
	copy(v.Signature, sign)

	return v, nil
}

func (kbft *KBFT) updateConsensusInfoInTesseracts(
	preCommitBitSet *common.ArrayOfBits,
	signature []byte,
) (err error) {
	evidenceHash, data, err := kbft.evidence.FlushEvidence()
	if err != nil {
		return err
	}

	qc := &common.Qc{
		Type:          common.PRECOMMIT,
		View:          kbft.View,
		TSHash:        kbft.Proposal.Tesseract.Hash(),
		SignerIndices: preCommitBitSet,
		Signature:     signature,
	}
	// Add evidence data to the dirty list
	kbft.ics.AddDirty(evidenceHash, data)

	tesseract := kbft.ProposalTS

	// for id, _ := range kbft.Heights {
	// TODO: check this out
	//	tesseract.SetEvidenceHash(id, evidenceHash)
	// }

	tesseract.SetCommitQc(qc)

	rawData, err := tesseract.SignBytes()
	if err != nil {
		return err
	}

	seal, err := kbft.vault.Sign(rawData, mudracommon.BlsBLST)
	if err != nil {
		return errors.Wrap(err, "failed to sign the tesseract")
	}

	tesseract.SetSeal(seal)
	tesseract.SetSealBy(kbft.id)

	return nil
}

func (kbft *KBFT) isLeader(view uint64, kramaID identifiers.KramaID) bool {
	return kbft.operator == kramaID
}

// scheduleTimeout will schedule a timeout for the given step,view and height
func (kbft *KBFT) scheduleTimeout(d time.Duration, heights map[identifiers.Identifier]uint64,
	view uint64, step ViewStepType,
) {
	kbft.logger.Debug("Scheduling timeout", "step", step, "duration", d, "heights", heights)
	kbft.toTicker.ScheduleTimeout(timeoutInfo{d, view, step})
}

// updateViewStep
func (kbft *KBFT) updateViewStep(view uint64, step ViewStepType) {
	kbft.View = view
	kbft.Step = step
}

func (kbft *KBFT) stepChange() {
	kbft.nSteps++

	if err := kbft.publishEventNewViewStep(kbft.viewStateEvent()); err != nil {
		kbft.logger.Error("failed to publish new view step", "err", err)
	}
}

func (kbft *KBFT) Start(slot *ktypes.Slot) {
	_, span := tracing.Span(kbft.ctx, "Krama.KBFT", "Start")
	defer span.End()
	// Start the ticker
	if err := kbft.toTicker.Start(); err != nil {
		kbft.logger.Error("Unable to start ticker", "err", err)
	}

	slot.Stage.CompareAndSwap(ktypes.PreparedStage, ktypes.ProposalStage)

	kbft.scheduleView()

	err := kbft.handler(0)

	kbft.exitChan <- err
}

func (kbft *KBFT) Close(err error) {
	kbft.logger.Info("Closing KBFT", "status", kbft.Step, "err", err,
		"isOperator", kbft.isLeader(kbft.View, kbft.id))

	select {
	case kbft.closeChan <- err:
	default:
		go func() {
			kbft.closeChan <- err
		}()
	}
}

func (kbft *KBFT) post(ev interface{}) error {
	if kbft.mux != nil {
		return kbft.mux.Post(ev)
	}

	return nil
}

func (kbft *KBFT) publishEventVote(vote *ktypes.Vote) error {
	return kbft.post(EventVote{vote})
}

func (kbft *KBFT) publishEventPolka(state eventDataViewState) error {
	return kbft.post(eventPolka{state})
}

func (kbft *KBFT) publishEventNewViewStep(state eventDataViewState) error {
	return kbft.post(eventNewViewStep{state})
}

func (kbft *KBFT) publishEventNewView(state eventDataViewState) error {
	return kbft.post(eventNewView{state})
}

// areHeightsEqual is a function that checks if the heights of the two sets are equal.
func areHeightsEqual(
	systemHeights, newHeights map[identifiers.Identifier]uint64,
) bool {
	if len(systemHeights) != len(newHeights) {
		return false
	}

	// Iterate over system heights
	for systemID, systemHeight := range systemHeights {
		newHeight, ok := newHeights[systemID]
		if !ok || systemHeight != newHeight {
			// if system address not found or system heights are not equal, return false
			return false
		}
	}

	// Heights match, return true
	return true
}

func (kbft *KBFT) publishEventProposal(proposal *ktypes.Proposal) error {
	return kbft.post(EventProposal{proposal})
}

// func (kbft *KBFT) PrintMetrics() {
//	prevotes := kbft.Votes.Prevotes(0)
//	precommits := kbft.Votes.Precommits(0)
//	kbft.logger.Trace("Printing metrics")
//
//	if kbft.Proposal != nil {
//		prevoteSet := prevotes.votesByTesseract[kbft.Proposal.Ts.Hash()]
//		precommitSet := precommits.votesByTesseract[kbft.ProposalTS.Hash()]
//		kbft.logger.Debug("Validators", "list", prevotes.valset.committee.String())
//		kbft.logger.Debug("Pre-vote received", "prevote-array", prevoteSet.bitarray)
//		kbft.logger.Debug("Pre-commit received", "precommit-array", precommitSet.bitarray)
//	}
//}

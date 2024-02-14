package kbft

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

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
	MaxBFTimeout  = 10 * time.Second
)

type vault interface {
	Sign(data []byte, sigType mudracommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
	KramaID() kramaid.KramaID
}

// KBFT is a struct that represents the runner for the Krama Byzantine Fault Tolerant consensus engine
type KBFT struct {
	RoundState
	logger                    hclog.Logger
	id                        kramaid.KramaID
	config                    *config.ConsensusConfig
	mx                        sync.Mutex
	inboundMsgChan            chan ktypes.ConsensusMessage
	selfMsgChan               chan ktypes.ConsensusMessage
	outboundMsgChan           chan ktypes.ConsensusMessage
	toTicker                  *Ticker
	ics                       *ktypes.ClusterState
	nSteps                    int
	closeChan                 chan error
	ctx                       context.Context
	ctxCancel                 context.CancelFunc
	evidence                  *Evidence
	vault                     vault
	wal                       WAL
	mux                       *utils.TypeMux
	finalizedTesseractHandler func(tesseract *common.Tesseract) error
}

// NewKBFTService is constructor function that generates a new KBFT engine.
// Accepts a Krama id for the node running the engine, a consensus configuration object, a channel to
// receive consensus messages, a PrivateValidator object, an execution engine and a lattice manager.
func NewKBFTService(
	ctx context.Context,
	timeout time.Duration,
	kid kramaid.KramaID,
	config *config.ConsensusConfig,
	outboundChan, inboundChan chan ktypes.ConsensusMessage,
	vault vault,
	ics *ktypes.ClusterState,
	tesseractHandler func(tesseract *common.Tesseract) error,
	opts ...Option,
) *KBFT {
	k := &KBFT{
		id:                        kid,
		config:                    config,
		outboundMsgChan:           outboundChan,
		inboundMsgChan:            inboundChan,
		selfMsgChan:               make(chan ktypes.ConsensusMessage, 1000),
		vault:                     vault,
		ics:                       ics,
		finalizedTesseractHandler: tesseractHandler,
		closeChan:                 make(chan error),
	}

	for _, opt := range opts {
		opt(k)
	}

	k.ctx, k.ctxCancel = context.WithTimeout(ctx, timeout)
	k.updateToState(ics)

	return k
}

func (kbft *KBFT) updateToState(cs *ktypes.ClusterState) {
	var chainIDs []string

	kbft.toTicker = NewTicker(kbft.logger)
	kbft.Heights = cs.NewHeights()
	kbft.updateRoundStep(0, RoundStepNewHeight)
	kbft.ics = cs
	kbft.Proposal = nil
	kbft.ProposalTS = nil
	kbft.LockedRound = -1
	kbft.LockedTS = nil
	kbft.ValidRound = -1
	kbft.ValidTS = nil
	kbft.Votes = NewHeightVoteSet(chainIDs, kbft.Heights, cs, kbft.logger)
	kbft.CommitRound = -1
	kbft.ics = cs
	kbft.TriggeredTimeoutPrecommit = false
	kbft.stepChange()
}

func (kbft *KBFT) Start() error {
	_, span := tracing.Span(kbft.ctx, "Krama.KBFT", "Start")
	defer span.End()
	// Start the ticker
	if err := kbft.toTicker.Start(); err != nil {
		kbft.logger.Error("Unable to start ticker", "err", err)
	}

	go kbft.ScheduleRound0()

	if err := kbft.handler(0); err != nil {
		kbft.toTicker.Stop()
		kbft.toTicker.Close()

		return err
	}

	return nil
}

func (kbft *KBFT) Close(err error) {
	kbft.logger.Info("Closing KBFT", "err", err)
	kbft.toTicker.Stop()
	kbft.toTicker.Close()

	select {
	case kbft.closeChan <- err:
	default:
		go func() {
			kbft.closeChan <- err
		}()
	}
}

func (kbft *KBFT) HandlePeerMsg(m ktypes.ConsensusMessage) {
	select {
	case kbft.inboundMsgChan <- m:
	default:
		go func() {
			kbft.inboundMsgChan <- m
		}()
	}
}

func (kbft *KBFT) handler(maxSteps int) error {
	defer func() {
		close(kbft.outboundMsgChan)
		close(kbft.selfMsgChan)
		// kbft.PrintMetrics()
		kbft.ctxCancel()
	}()

	for {
		if maxSteps > 0 {
			if kbft.nSteps >= maxSteps {
				kbft.logger.Error("Maximum steps reached")

				kbft.nSteps = 0

				return errors.New("maximum steps reached")
			}
		}

		roundState := kbft.RoundState

		select {
		case err := <-kbft.closeChan:
			return err

		case <-kbft.ctx.Done():
			kbft.logger.Info("KBFT timeout occurred")

			return kbft.ctx.Err()

		case msg := <-kbft.inboundMsgChan:
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
			kbft.handleTimeout(t, roundState)
		}
	}
}

func (kbft *KBFT) handleTimeout(ti timeoutInfo, r RoundState) {
	if !areHeightsEqual(ti.Height, r.Heights) || r.Round > ti.Round || (ti.Round == r.Round && ti.Step < r.Step) {
		kbft.logger.Debug(
			"Returning from time out",
			"timeout-height", ti.Height,
			"round-height", r.Heights,
			"timeout-round", ti.Round,
			"round-number", r.Round,
			"timeout-step", ti.Step,
			"round-step", r.Step,
		)

		return
	}

	kbft.mx.Lock()
	defer kbft.mx.Unlock()

	switch ti.Step {
	case RoundStepNewHeight:
		kbft.enterNewRound(ti.Height, 0)

	case RoundStepPropose:
		if err := kbft.publishEventTimeoutPropose(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish timeout propose", "err", err)
		}

		kbft.enterPrevote(ti.Height, ti.Round)

	case RoundStepPrevoteWait:
		if err := kbft.publishEventTimeoutPrevote(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish timeout prevote", "err", err)
		}

		kbft.enterPreCommit(ti.Height, ti.Round)

	case RoundStepPrecommitWait:
		if err := kbft.publishEventTimeoutPrecommit(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish timeout precommit", "err", err)
		}

		kbft.enterPreCommit(ti.Height, ti.Round)
		kbft.enterNewRound(ti.Height, ti.Round)
	}
}

func (kbft *KBFT) handleMsg(msg ktypes.ConsensusMessage) error {
	_, span := tracing.Span(kbft.ctx, "Krama.KBFT", "handleMsg")

	kbft.mx.Lock()
	defer func() {
		kbft.mx.Unlock()
		span.End()
	}()

	m, peerID := msg.Message, msg.PeerID

	switch m := m.(type) {
	case *ktypes.ProposalMessage:
		if msg.PeerID == kbft.id { // make sure we are not receiving proposal from others through outbound channel
			kbft.logger.Trace("Proposal message received")

			if err := kbft.setProposal(m.Proposal); err != nil {
				kbft.logger.Error("Failed to set proposal", "err", err)
			}
		}

	case *ktypes.VoteMessage:
		kbft.logger.Trace("Vote message received", "vote-type", m.Vote.Type, "from", peerID)

		added, err := kbft.addVote(m.Vote, peerID)
		if err != nil {
			kbft.logger.Error("Failed to add vote", "err", err)

			return nil
		}

		if added && peerID == kbft.id {
			msg.PeerID = kbft.id
			kbft.logger.Trace("Sending vote message for ts-hash", "ts-hash", m.Vote.TSHash)
			kbft.outboundMsgChan <- msg
		}
	}

	return nil
}

func (kbft *KBFT) setProposal(p *ktypes.Proposal) error {
	if kbft.Proposal != nil {
		return nil
	}

	if !areHeightsEqual(kbft.Heights, p.Height) || p.Round != kbft.Round {
		return nil
	}

	if p.POLRound < -1 || (p.POLRound >= 0 && p.POLRound >= p.Round) {
		return errors.New("invalid PoL details")
	}

	// TODO:Verify Proposal Signatures

	kbft.Proposal = p

	// set the proposal tesseract
	kbft.ProposalTS = p.Tesseract

	if err := kbft.publishEventProposal(p); err != nil {
		kbft.logger.Error("Failed to publish proposal", "err", err)
	}

	preVotes := kbft.Votes.getPrevotes(kbft.Round)

	tsHash, majority := preVotes.TwoThirdMajority()
	if majority && !tsHash.IsNil() && (p.Round > kbft.ValidRound) {
		if kbft.ProposalTS.CompareHash(tsHash) {
			kbft.ValidTS = kbft.ProposalTS
			kbft.ValidRound = kbft.Round
		}
	}

	if kbft.Step <= RoundStepPropose && kbft.isProposalReceived() {
		kbft.enterPrevote(kbft.Heights, kbft.Round)

		if majority {
			kbft.enterPreCommit(kbft.Heights, kbft.Round)
		}
	} else if kbft.Step == RoundStepCommit {
		kbft.finalizeCommit(kbft.Heights)
	}

	return nil
}

func (kbft *KBFT) SetProposal(p *ktypes.Proposal, peerID kramaid.KramaID) error {
	if peerID == "" {
		kbft.selfMsgChan <- ktypes.ConsensusMessage{PeerID: "", Message: &ktypes.ProposalMessage{Proposal: p}}
	} else {
		kbft.inboundMsgChan <- ktypes.ConsensusMessage{PeerID: peerID, Message: &ktypes.ProposalMessage{Proposal: p}}
	}

	return nil
}

func (kbft *KBFT) addVote(v *ktypes.Vote, peerID kramaid.KramaID) (added bool, err error) {
	if !areVoteHeightsEqual(v.Heights, kbft.Heights) {
		kbft.logger.Trace("Invalid vote BFT height", "local-heights", kbft.Heights, "msg-heights", v.Heights)

		return
	}

	height := kbft.Heights

	if kbft.ProposalTS != nil && v.TSHash != kbft.ProposalTS.Hash() {
		kbft.evidence.AddVote(v)
	}

	added, err = kbft.Votes.AddVote(v, peerID)
	if err != nil || !added {
		kbft.evidence.AddVote(v)

		return
	}

	if err := kbft.publishEventVote(v); err != nil {
		kbft.logger.Error("Failed to publish vote", "err", err)
	}

	switch v.Type {
	case ktypes.PREVOTE:
		preVotes := kbft.Votes.getPrevotes(v.Round)
		if tsHash, ok := preVotes.TwoThirdMajority(); ok {
			if kbft.LockedTS != nil &&
				kbft.LockedRound < v.Round &&
				v.Round <= kbft.Round &&
				!kbft.LockedTS.CompareHash(tsHash) {
				// Update the locks
				kbft.LockedTS = nil
				kbft.LockedRound = -1

				if err := kbft.publishEventUnlock(kbft.RoundStateEvent()); err != nil {
					kbft.logger.Error("Failed to publish event unlock", "err", err)
				}
			}

			if !tsHash.IsNil() && (kbft.ValidRound < v.Round) && v.Round == kbft.Round {
				if kbft.ProposalTS.CompareHash(tsHash) {
					kbft.ValidTS = kbft.ProposalTS
					kbft.ValidRound = v.Round
				} else {
					kbft.ProposalTS = nil
				}
			}
		}

		switch {
		case kbft.Round < v.Round && preVotes.HasMajorityAny():
			kbft.enterNewRound(height, v.Round)

		case kbft.Round == v.Round && kbft.Step >= RoundStepPrevote:
			tsHash, ok := preVotes.TwoThirdMajority()
			if ok && (kbft.isProposalReceived() || !tsHash.IsNil()) {
				kbft.enterPreCommit(height, v.Round)
			} else if preVotes.HasMajorityAny() {
				kbft.enterPrevoteWait(height, v.Round)
			}

		case kbft.Proposal != nil && kbft.Proposal.POLRound >= 0 && kbft.Proposal.POLRound == v.Round:
			if kbft.isProposalReceived() {
				kbft.enterPrevote(height, kbft.Round)
			}

		default:
			kbft.logger.Debug("Proposal not available")
		}

	case ktypes.PRECOMMIT:
		preCommits := kbft.Votes.getPrecommits(v.Round)

		tsHash, ok := preCommits.TwoThirdMajority()
		if ok {
			kbft.enterNewRound(height, v.Round)
			kbft.enterPreCommit(height, v.Round)

			if !tsHash.IsNil() {
				kbft.enterCommit(height, v.Round)
			} else {
				kbft.enterPrecommitWait(height, v.Round)
			}
		} else if kbft.Round <= v.Round && preCommits.HasMajorityAny() {
			kbft.enterNewRound(height, v.Round)
			kbft.enterPrecommitWait(height, v.Round)
		}
	}

	return added, err
}

func (kbft *KBFT) finalizeCommit(h map[identifiers.Address]uint64) {
	if !areHeightsEqual(kbft.Heights, h) {
		panic("unmatched heights")
	}

	tsHash, ok := kbft.Votes.getPrecommits(kbft.CommitRound).TwoThirdMajority()
	if !ok || tsHash.IsNil() {
		kbft.logger.Trace("Majority is not available")

		return
	}

	if kbft.Proposal == nil || !kbft.Proposal.Tesseract.CompareHash(tsHash) {
		kbft.logger.Trace("Proposal tesseract doesn't match with the majority")

		return
	}

	if !areHeightsEqual(kbft.Heights, h) || kbft.Step != RoundStepCommit {
		return
	}

	kbft.logger.Info("Tesseract finalised", "ts-hash", tsHash)

	preCommits := kbft.Votes.getPrecommits(kbft.Round)
	tesseractPreCommits := preCommits.votesByTesseract[string(tsHash.Bytes())]

	aggregatedSignature, err := tesseractPreCommits.AggregateSignatures()
	if err != nil {
		kbft.Close(errors.Wrap(err, "failed to aggregate signatures"))

		return
	}

	if err = kbft.updateConsensusInfoInTesseracts(tesseractPreCommits, aggregatedSignature); err != nil {
		kbft.Close(err)

		return
	}

	if err = kbft.finalizedTesseractHandler(kbft.ProposalTS.Copy()); err != nil {
		kbft.Close(err)

		return
	}

	// stop the bft engine
	kbft.Close(nil)
}

func (kbft *KBFT) updateConsensusInfoInTesseracts(
	preCommits *tesseractVoteSet,
	signature []byte,
) (err error) {
	evidenceHash, data, err := kbft.evidence.FlushEvidence()
	if err != nil {
		return err
	}

	// Add evidence data to dirty list
	kbft.ics.AddDirty(evidenceHash, data)

	tesseract := kbft.ProposalTS

	tesseract.SetRound(kbft.Round)
	tesseract.SetCommitSignature(signature)
	tesseract.SetBFTVoteSet(preCommits.bitarray)
	tesseract.SetEvidenceHash(evidenceHash)

	// FIXME: Add sealer in tesseract generation

	rawData, err := tesseract.Bytes()
	if err != nil {
		return err
	}

	seal, err := kbft.vault.Sign(rawData, mudracommon.BlsBLST)
	if err != nil {
		return errors.Wrap(err, "failed to sign the tesseract")
	}

	tesseract.SetSeal(seal)

	return nil
}

func (kbft *KBFT) ScheduleRound0() {
	kbft.logger.Info("Scheduling Round 0. TO:")
	kbft.scheduleTimeout(100*time.Millisecond, kbft.Heights, 0, RoundStepNewHeight)
}

func (kbft *KBFT) enterCommit(heights map[identifiers.Address]uint64, round int32) {
	if !areHeightsEqual(kbft.Heights, heights) || RoundStepCommit <= kbft.Step {
		return
	}

	defer func() {
		kbft.updateRoundStep(kbft.Round, RoundStepCommit)
		kbft.CommitRound = round
		kbft.CommitTime = time.Now()

		kbft.stepChange()
		kbft.finalizeCommit(heights)
	}()

	tsHash, ok := kbft.Votes.getPrecommits(round).TwoThirdMajority()
	if !ok {
		panic("expecting precommits")
	}

	// log.Printf(" lockedgrid %s,groupId %s", b.LockedTS.Hash, groupID.Hash)
	// log.Println("In enter commit proposal tesseract", b.ProposalTesseracts, b.LockedTesseracts)
	if kbft.LockedTS.CompareHash(tsHash) {
		kbft.ProposalTS = kbft.LockedTS
	}
}

func (kbft *KBFT) enterPrecommitWait(heights map[identifiers.Address]uint64, r int32) {
	if !areHeightsEqual(kbft.Heights, heights) ||
		r < kbft.Round ||
		(kbft.Round == r && kbft.TriggeredTimeoutPrecommit) {
		return
	}

	if !kbft.Votes.getPrecommits(r).HasMajorityAny() {
		panic("no 2/3 precommits ")
	}

	defer func() {
		kbft.TriggeredTimeoutPrecommit = true
		kbft.stepChange()
	}()

	kbft.scheduleTimeout(kbft.preCommitTimeout(r), heights, r, RoundStepPrecommitWait)
}

func (kbft *KBFT) isProposalReceived() bool {
	if kbft.Proposal == nil || kbft.ProposalTS == nil {
		return false
	}

	if kbft.Proposal.POLRound < 0 {
		return true
	}

	return kbft.Votes.getPrevotes(kbft.Proposal.POLRound).HasMajority()
}

func (kbft *KBFT) enterNewRound(heights map[identifiers.Address]uint64, round int32) {
	if !areHeightsEqual(kbft.Heights, heights) ||
		round < kbft.Round ||
		(kbft.Round == round && kbft.Step != RoundStepNewHeight) {
		return
	}

	kbft.logger.Trace("Entering new KBFT round", "heights", heights, "round", round)
	kbft.updateRoundStep(round, RoundStepNewHeight)

	if round != 0 {
		kbft.Proposal = nil
		kbft.ProposalTS = nil

		if err := kbft.publishEventPolka(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish new round step", "err", err)
		}
	}

	kbft.Votes.SetRound(round + 1)
	kbft.TriggeredTimeoutPrecommit = false

	if err := kbft.publishEventNewRound(kbft.RoundStateEvent()); err != nil {
		kbft.logger.Error("Failed to publish new round", "err", err)
	}

	// Now ready to enter propose
	kbft.enterPropose(heights, round)
}

func (kbft *KBFT) enterPropose(heights map[identifiers.Address]uint64, round int32) {
	if !areHeightsEqual(kbft.Heights, heights) ||
		kbft.Round > round ||
		(kbft.Round == round && kbft.Step >= RoundStepPropose) {
		return
	}

	defer func() {
		kbft.updateRoundStep(round, RoundStepPropose)
		kbft.stepChange()

		if kbft.isProposalReceived() {
			kbft.enterPrevote(heights, kbft.Round)
		}
	}()

	kbft.scheduleTimeout(kbft.proposeTimeout(round), heights, round, RoundStepPropose)

	if kbft.vault == nil {
		return
	}

	if _, exists := kbft.ics.HasKramaID(kbft.vault.KramaID()); !exists {
		kbft.logger.Error("Validator not found in ICS set")

		return
	}

	if err := kbft.createProposal(heights, round); err != nil {
		kbft.Close(err)
	}
}

func (kbft *KBFT) createProposalTesseract() (*common.Tesseract, error) {
	ts := kbft.ics.GetTesseract()
	if ts == nil {
		return nil, errors.New("invalid tesseract")
	}

	return ts, nil
}

// createProposal will create a proposal message for the given height,round and tesseract
func (kbft *KBFT) createProposal(heights map[identifiers.Address]uint64, round int32) error {
	kbft.logger.Info("Creating proposal", "heights", heights, "round", round)

	var (
		ts  *common.Tesseract
		err error
	)

	if kbft.ValidTS != nil {
		ts = kbft.ValidTS
	} else {
		ts, err = kbft.createProposalTesseract()
		if err != nil {
			return err
		}
	}

	proposal := ktypes.NewProposal(heights, round, kbft.ValidRound, ts)

	// Send an internal message
	kbft.sendInternalMessage(&ktypes.ProposalMessage{Proposal: proposal})

	return nil
}

func (kbft *KBFT) sendInternalMessage(msg ktypes.Cmessage) {
	kbft.selfMsgChan <- ktypes.ConsensusMessage{PeerID: kbft.id, Message: msg}
}

func (kbft *KBFT) prevoteTimeout(round int32) time.Duration {
	preVoteTimeout := kbft.config.TimeoutPrevote.Nanoseconds()
	preVoteTimeoutDelta := kbft.config.TimeoutPrevoteDelta.Nanoseconds()

	return time.Duration(preVoteTimeout+preVoteTimeoutDelta*int64(round)) * time.Nanosecond
}

func (kbft *KBFT) preCommitTimeout(round int32) time.Duration {
	preCommitTimeout := kbft.config.TimeoutPrecommit.Nanoseconds()
	preCommitTimeoutDelta := kbft.config.TimeoutPrecommitDelta.Nanoseconds()

	return time.Duration(preCommitTimeout+preCommitTimeoutDelta*int64(round)) * time.Nanosecond
}

func (kbft *KBFT) proposeTimeout(round int32) time.Duration {
	proposeTimeout := kbft.config.TimeoutPropose.Nanoseconds()
	proposeTimeoutDelta := kbft.config.TimeoutProposeDelta.Nanoseconds()

	return time.Duration(proposeTimeout+proposeTimeoutDelta*int64(round)) * time.Nanosecond
}

func (kbft *KBFT) enterPreCommit(heights map[identifiers.Address]uint64, round int32) {
	kbft.logger.Trace("Entered pre-commit", "round", round)

	if !areHeightsEqual(kbft.Heights, heights) ||
		kbft.Round > round ||
		(round == kbft.Round && kbft.Step >= RoundStepPrecommit) {
		return
	}

	defer func() {
		kbft.updateRoundStep(round, RoundStepPrecommit)
		kbft.stepChange()
	}()

	// Before preCommit check for >2/3 preVotes
	tsHash, ok := kbft.Votes.getPrevotes(round).TwoThirdMajority()
	if !ok {
		if kbft.LockedTS != nil {
			log.Println("PreCommit nil due to lock")
		}

		kbft.sendVote(ktypes.PRECOMMIT, common.NilHash)

		return
	}

	if err := kbft.publishEventPolka(kbft.RoundStateEvent()); err != nil {
		kbft.logger.Error("Failed to publish polka", "err", err)
	}

	polRound, _ := kbft.Votes.POLInfo()
	if round > polRound {
		log.Panicln("Since 2/3 votes are received polRound should be same.")
	}

	if tsHash.IsNil() {
		if kbft.LockedTS != nil {
			kbft.LockedRound = -1
			kbft.LockedTS = nil

			if err := kbft.publishEventUnlock(kbft.RoundStateEvent()); err != nil {
				kbft.logger.Error("Failed to publish event unlock", "err", err)
			}
		}

		kbft.sendVote(ktypes.PRECOMMIT, common.NilHash)

		return
	}

	if kbft.LockedTS.CompareHash(tsHash) {
		kbft.LockedRound = round

		if err := kbft.publishEventRelock(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish event relock", "err", err)
		}

		kbft.sendVote(ktypes.PRECOMMIT, tsHash)

		return
	}

	if kbft.ProposalTS.CompareHash(tsHash) {
		// TODO: Validate the tesseractGrid
		kbft.LockedRound = round
		kbft.LockedTS = kbft.ProposalTS

		if err := kbft.publishEventLock(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish event lock", "err", err)
		}

		kbft.sendVote(ktypes.PRECOMMIT, tsHash)

		return
	}

	kbft.LockedRound = -1
	kbft.LockedTS = nil
	// kbft.ProposalTS = nil

	if err := kbft.publishEventUnlock(kbft.RoundStateEvent()); err != nil {
		kbft.logger.Error("Failed to publish event unlock", "err", err)
	}

	kbft.sendVote(ktypes.PRECOMMIT, common.NilHash)
}

// updateRoundStep
func (kbft *KBFT) updateRoundStep(round int32, step RoundStepType) {
	kbft.Round = round
	kbft.Step = step
}

func (kbft *KBFT) enterPrevote(h map[identifiers.Address]uint64, r int32) {
	kbft.logger.Trace("Entered pre-vote")

	if !areHeightsEqual(kbft.Heights, h) || kbft.Round > r || (kbft.Round == r && kbft.Step >= RoundStepPrevote) {
		return
	}

	defer func() {
		kbft.updateRoundStep(r, RoundStepPrevote)
		kbft.stepChange()
	}()

	if kbft.LockedTS != nil {
		kbft.logger.Trace("Voting on locked tesseract", "ts-hash", kbft.LockedTS.Hash)

		kbft.sendVote(ktypes.PREVOTE, kbft.LockedTS.Hash())

		return
	}

	if kbft.ProposalTS == nil {
		kbft.logger.Trace("Proposal tesseract is nil")
		kbft.sendVote(ktypes.PREVOTE, common.NilHash)

		return
	}

	kbft.sendVote(ktypes.PREVOTE, kbft.ProposalTS.Hash())
}

// sendVote will send a signed vote message for the given vote-type and tesseract
func (kbft *KBFT) sendVote(msgType ktypes.ConsensusMsgType, tsHash common.Hash) *ktypes.Vote {
	kbft.logger.Debug("Sending vote", "vote-type", msgType, "ts-hash", tsHash)

	if kbft.vault == nil {
		kbft.logger.Error("Vault service unavailable during sendVote")

		return nil
	}

	if _, exists := kbft.ics.HasKramaID(kbft.id); !exists {
		return nil
	}

	vote, err := kbft.signVote(msgType, tsHash)
	if err != nil {
		kbft.logger.Error("Error signing the vote message during sendVote", "err", err)

		return nil
	}

	kbft.sendInternalMessage(&ktypes.VoteMessage{Vote: vote})

	return vote
}

// signVote will create a vote message and sign it using the validator consensus key
func (kbft *KBFT) signVote(msgType ktypes.ConsensusMsgType, tsHash common.Hash) (*ktypes.Vote, error) {
	valIndex, _ := kbft.ics.HasKramaID(kbft.id)

	if valIndex == -1 {
		return nil, common.ErrKramaIDNotFound
	}

	v := &ktypes.Vote{
		ValidatorIndex: valIndex,
		Heights:        kbft.Heights,
		TSHash:         tsHash,
		Round:          kbft.Round,
		Type:           msgType,
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

func (kbft *KBFT) enterPrevoteWait(h map[identifiers.Address]uint64, r int32) {
	kbft.logger.Trace("Entered pre-vote wait")

	if !areHeightsEqual(kbft.Heights, h) ||
		kbft.Round > r ||
		(kbft.Step >= RoundStepPrevoteWait && kbft.Round == r) {
		return
	}

	if !kbft.Votes.getPrevotes(r).HasMajorityAny() {
		kbft.logger.Error("Entered pre-vote wait without majority")

		return
	}

	defer func() {
		kbft.updateRoundStep(r, RoundStepPrevoteWait)
		kbft.stepChange()
	}()

	kbft.scheduleTimeout(kbft.prevoteTimeout(r), h, r, RoundStepPrevoteWait)
}

// scheduleTimeout will schedule a timeout for the given step,round and height
func (kbft *KBFT) scheduleTimeout(
	d time.Duration,
	heights map[identifiers.Address]uint64,
	r int32,
	step RoundStepType,
) {
	kbft.logger.Info("Scheduling timeout", "step", step, "duration", d, "heights", heights)
	kbft.toTicker.ScheduleTimeout(timeoutInfo{d, heights, r, step})
}

func (kbft *KBFT) stepChange() {
	// Write to the log
	kbft.nSteps++

	if err := kbft.publishEventNewRoundStep(kbft.RoundStateEvent()); err != nil {
		kbft.logger.Error("Failed to publish new round step", "err", err)
	}
}

func (kbft *KBFT) PrintMetrics() {
	prevotes := kbft.Votes.getPrevotes(0)
	precommits := kbft.Votes.getPrecommits(0)
	kbft.logger.Trace("Printing metrics")

	if kbft.Proposal != nil {
		prevoteSet := prevotes.votesByTesseract[string(kbft.Proposal.Tesseract.Hash().Bytes())]
		precommitSet := precommits.votesByTesseract[string(kbft.ProposalTS.Hash().Bytes())]
		kbft.logger.Debug("Validators", "list", prevotes.valset.NodeSet.String())
		kbft.logger.Debug("Pre-vote received", "prevote-array", prevoteSet.bitarray)
		kbft.logger.Debug("Pre-commit received", "precommit-array", precommitSet.bitarray)
	}
}

func (kbft *KBFT) post(ev interface{}) error {
	if kbft.mux != nil {
		return kbft.mux.Post(ev)
	}

	return nil
}

func (kbft *KBFT) publishEventProposal(proposal *ktypes.Proposal) error {
	return kbft.post(eventProposal{proposal})
}

func (kbft *KBFT) publishEventVote(vote *ktypes.Vote) error {
	return kbft.post(eventVote{vote: vote})
}

func (kbft *KBFT) publishEventRelock(state eventDataRoundState) error {
	return kbft.post(eventRelock{state})
}

func (kbft *KBFT) publishEventLock(state eventDataRoundState) error {
	return kbft.post(eventLock{state})
}

func (kbft *KBFT) publishEventUnlock(state eventDataRoundState) error {
	return kbft.post(eventUnlock{state})
}

func (kbft *KBFT) publishEventPolka(state eventDataRoundState) error {
	return kbft.post(eventPolka{state})
}

func (kbft *KBFT) publishEventNewRoundStep(state eventDataRoundState) error {
	return kbft.post(eventNewRoundStep{state})
}

func (kbft *KBFT) publishEventNewRound(state eventDataRoundState) error {
	return kbft.post(eventNewRound{state})
}

func (kbft *KBFT) publishEventTimeoutPropose(state eventDataRoundState) error {
	return kbft.post(eventTimeoutPropose{state})
}

func (kbft *KBFT) publishEventTimeoutPrevote(state eventDataRoundState) error {
	return kbft.post(eventTimeoutPrevote{state})
}

func (kbft *KBFT) publishEventTimeoutPrecommit(state eventDataRoundState) error {
	return kbft.post(eventTimeoutPrecommit{state})
}

// A function that checks if two sets of heights are equal.
// Accepts two sets of heights and compares them. Returns a bool.
// if heights of respective addresses matches then true is returned
func areHeightsEqual(systemHeights map[identifiers.Address]uint64, newHeights map[identifiers.Address]uint64) bool {
	if len(systemHeights) != len(newHeights) {
		return false
	}

	// Iterate over system heights
	for systemAddress, systemHeight := range systemHeights {
		newHeight, ok := newHeights[systemAddress]
		if !ok || systemHeight != newHeight {
			// if system address not found or system heights are not equal, return false
			return false
		}
	}

	// Heights match, return true
	return true
}

// A function that checks if two sets of heights are equal.
// Accepts two sets of heights and compares them. Returns a bool.
// if heights of respective addresses matches then true is returned.
func areVoteHeightsEqual(
	voteHeights map[identifiers.Address]uint64,
	systemHeights map[identifiers.Address]uint64,
) bool {
	if len(voteHeights) != len(systemHeights) {
		return false
	}

	// Iterate over system heights
	for voteAddress, voteHeight := range voteHeights {
		systemHeight, ok := systemHeights[voteAddress]
		if !ok || voteHeight != systemHeight {
			// if system address not found or system heights are not equal, return false
			return false
		}
	}

	// Heights match, return true
	return true
}

// A function that checks if the second set of heights is greater than the first.
// Accepts two sets of heights (int64 slice) and compares them. Returns a bool.
// if heights of respective addresses from first set are greater than second set then true is returned.
func areHeightsGreater(systemHeights map[identifiers.Address]uint64, newHeights map[identifiers.Address]uint64) bool {
	if len(systemHeights) != len(newHeights) {
		return false
	}

	// Iterate over system heights
	for systemAddress, systemHeight := range systemHeights {
		newHeight, ok := newHeights[systemAddress]
		if !ok || systemHeight <= newHeight {
			// if system address not found or system heights less than or equal to new height, return false
			return false
		}
	}

	// All heights are greater, return true
	return true
}

func areGreater(oldValues, newValues []int32) bool {
	// Iterate over system heights
	for idx, value := range oldValues {
		if value < newValues[idx] {
			// Height lesser, return false
			return false
		}
	}

	// All heights are greater, return true
	return true
}

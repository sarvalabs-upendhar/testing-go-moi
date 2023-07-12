package kbft

import (
	"context"
	"log"
	"sync"
	"time"

	id "github.com/sarvalabs/moichain/common/kramaid"
	mudracommon "github.com/sarvalabs/moichain/crypto/common"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/config"
	"github.com/sarvalabs/moichain/common/utils"
	"github.com/sarvalabs/moichain/crypto"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	ktypes "github.com/sarvalabs/moichain/consensus/types"
	"github.com/sarvalabs/moichain/telemetry/tracing"
)

const (
	MaxBFTimeout = 15 * time.Second
)

type vault interface {
	Sign(data []byte, sigType mudracommon.SigType, signOptions ...crypto.SignOption) ([]byte, error)
	KramaID() id.KramaID
}

// KBFT is a struct that represents the runner for the Krama Byzantine Fault Tolerant consensus engine
type KBFT struct {
	RoundState
	logger                    hclog.Logger
	id                        id.KramaID
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
	finalizedTesseractHandler func(tesseracts []*common.Tesseract) error
}

// NewKBFTService is constructor function that generates a new KBFT engine.
// Accepts a Krama id for the node running the engine, a consensus configuration object, a channel to
// receive consensus messages, a PrivateValidator object, an execution engine and a lattice manager.
func NewKBFTService(
	ctx context.Context,
	kid id.KramaID,
	config *config.ConsensusConfig,
	outboundChan, inboundChan chan ktypes.ConsensusMessage,
	vault vault,
	ics *ktypes.ClusterState,
	tesseractHandler func(tesseracts []*common.Tesseract) error,
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

	k.ctx, k.ctxCancel = context.WithTimeout(ctx, MaxBFTimeout)
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
	kbft.ProposalGrid = nil
	kbft.LockedRound = -1
	kbft.LockedGrid = nil
	kbft.ValidRound = -1
	kbft.ValidGrid = nil
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

	return kbft.handler(0)
}

func (kbft *KBFT) Close(err error) {
	kbft.logger.Info("Closing KBFT", "err", err)
	kbft.toTicker.Close()
	kbft.toTicker.Stop()

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
			kbft.logger.Trace("Sending vote message for grid ID", "grid-ID", m.Vote.GridID.Hash)
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

	// set the proposal grid
	kbft.ProposalGrid = p.Grid

	if err := kbft.publishEventProposal(p); err != nil {
		kbft.logger.Error("Failed to publish proposal", "err", err)
	}

	preVotes := kbft.Votes.getPrevotes(kbft.Round)

	gridID, majority := preVotes.TwoThirdMajority()
	if majority && gridID != nil && (p.Round > kbft.ValidRound) {
		if kbft.ProposalGrid.CompareHash(gridID.Hash) {
			kbft.ValidGrid = kbft.ProposalGrid
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

func (kbft *KBFT) SetProposal(p *ktypes.Proposal, peerID id.KramaID) error {
	if peerID == "" {
		kbft.selfMsgChan <- ktypes.ConsensusMessage{PeerID: "", Message: &ktypes.ProposalMessage{Proposal: p}}
	} else {
		kbft.inboundMsgChan <- ktypes.ConsensusMessage{PeerID: peerID, Message: &ktypes.ProposalMessage{Proposal: p}}
	}

	return nil
}

func (kbft *KBFT) addVote(v *ktypes.Vote, peerID id.KramaID) (added bool, err error) {
	if !areVoteHeightsEqual(v.GridID.Parts.Grid, kbft.Heights) {
		kbft.logger.Trace("Invalid vote BFT height", "local-heights", kbft.Heights, "msg-heights", v.GridID.Parts.Grid)

		return
	}

	height := kbft.Heights

	if kbft.ProposalGrid != nil && v.GridID.Hash != kbft.ProposalGrid.Hash {
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
		if tesseractGridID, ok := preVotes.TwoThirdMajority(); ok {
			if kbft.LockedGrid != nil &&
				kbft.LockedRound < v.Round &&
				v.Round <= kbft.Round &&
				!kbft.LockedGrid.CompareHash(tesseractGridID.Hash) {
				// Update the locks
				kbft.LockedGrid = nil
				kbft.LockedRound = -1

				if err := kbft.publishEventUnlock(kbft.RoundStateEvent()); err != nil {
					kbft.logger.Error("Failed to publish event unlock", "err", err)
				}
			}

			if !tesseractGridID.IsNil() && (kbft.ValidRound < v.Round) && v.Round == kbft.Round {
				if kbft.ProposalGrid.CompareHash(tesseractGridID.Hash) {
					kbft.ValidGrid = kbft.ProposalGrid
					kbft.ValidRound = v.Round
				} else {
					kbft.ProposalGrid = nil
				}
			}
		}

		switch {
		case kbft.Round < v.Round && preVotes.HasMajorityAny():
			kbft.enterNewRound(height, v.Round)

		case kbft.Round == v.Round && kbft.Step >= RoundStepPrevote:
			tesseractGroupID, ok := preVotes.TwoThirdMajority()
			if ok && (kbft.isProposalReceived() || !tesseractGroupID.Hash.IsNil()) {
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

		gridID, ok := preCommits.TwoThirdMajority()
		if ok {
			kbft.enterNewRound(height, v.Round)
			kbft.enterPreCommit(height, v.Round)

			if !gridID.IsNil() {
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

func (kbft *KBFT) finalizeCommit(h map[common.Address]uint64) {
	if !areHeightsEqual(kbft.Heights, h) {
		panic("unmatched heights")
	}

	gridID, ok := kbft.Votes.getPrecommits(kbft.CommitRound).TwoThirdMajority()
	if !ok || gridID.IsNil() {
		kbft.logger.Trace("Majority is not available")

		return
	}

	if kbft.Proposal == nil || !kbft.Proposal.Grid.CompareHash(gridID.Hash) {
		kbft.logger.Trace("Proposal grid doesn't match with the majority")

		return
	}

	if !areHeightsEqual(kbft.Heights, h) || kbft.Step != RoundStepCommit {
		return
	}

	kbft.logger.Info("Tesseract finalised", "grid-ID", gridID.Hash)

	preCommits := kbft.Votes.getPrecommits(kbft.Round)
	tesseractPreCommits := preCommits.votesByTesseract[string(gridID.Hash.Bytes())]

	aggregatedSignature, err := tesseractPreCommits.AggregateSignatures()
	if err != nil {
		kbft.Close(errors.Wrap(err, "failed to aggregate signatures"))

		return
	}

	if err = kbft.updateConsensusInfoInTesseracts(gridID, tesseractPreCommits, aggregatedSignature); err != nil {
		kbft.Close(err)

		return
	}

	if err = kbft.finalizedTesseractHandler(kbft.ProposalGrid.TesseractsCopy()); err != nil {
		kbft.Close(err)

		return
	}

	// stop the bft engine
	kbft.Close(nil)
}

func (kbft *KBFT) updateConsensusInfoInTesseracts(
	gridID *common.TesseractGridID,
	preCommits *tesseractVoteSet,
	signature []byte,
) (err error) {
	evidenceHash, data, err := kbft.evidence.FlushEvidence()
	if err != nil {
		return err
	}

	// Add evidence data to dirty list
	kbft.ics.AddDirty(evidenceHash, data)

	for _, tesseract := range kbft.ProposalGrid.Tesseracts {
		extraData := common.CommitData{
			Round:           kbft.Round,
			CommitSignature: signature,
			VoteSet:         preCommits.bitarray,
			EvidenceHash:    evidenceHash,
			GridID:          gridID,
		}

		tesseract.SetExtraData(extraData)
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
	}

	return nil
}

func (kbft *KBFT) ScheduleRound0() {
	kbft.logger.Info("Scheduling Round 0. TO:")
	kbft.scheduleTimeout(100*time.Millisecond, kbft.Heights, 0, RoundStepNewHeight)
}

func (kbft *KBFT) enterCommit(heights map[common.Address]uint64, round int32) {
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

	gridID, ok := kbft.Votes.getPrecommits(round).TwoThirdMajority()
	if !ok {
		panic("expecting precommits")
	}

	// log.Printf(" lockedgrid %s,groupId %s", b.LockedGrid.Hash, groupID.Hash)
	// log.Println("In enter commit proposal tesseract", b.ProposalTesseracts, b.LockedTesseracts)
	if kbft.LockedGrid.CompareHash(gridID.Hash) {
		kbft.ProposalGrid = kbft.LockedGrid
	}
}

func (kbft *KBFT) enterPrecommitWait(heights map[common.Address]uint64, r int32) {
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
	if kbft.Proposal == nil || kbft.ProposalGrid == nil {
		return false
	}

	if kbft.Proposal.POLRound < 0 {
		return true
	}

	return kbft.Votes.getPrevotes(kbft.Proposal.POLRound).HasMajority()
}

func (kbft *KBFT) enterNewRound(heights map[common.Address]uint64, round int32) {
	if !areHeightsEqual(kbft.Heights, heights) ||
		round < kbft.Round ||
		(kbft.Round == round && kbft.Step != RoundStepNewHeight) {
		return
	}

	kbft.logger.Trace("Entering new KBFT round", "heights", heights, "round", round)
	kbft.updateRoundStep(round, RoundStepNewHeight)

	if round != 0 {
		kbft.Proposal = nil
		kbft.ProposalGrid = nil

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

func (kbft *KBFT) enterPropose(heights map[common.Address]uint64, round int32) {
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

func (kbft *KBFT) createProposalGrid() (*ktypes.TesseractGrid, error) {
	grid := kbft.ics.GetTesseractGrid()
	if grid == nil {
		return nil, errors.New("invalid tesseract grid")
	}

	rawHashes := make([]byte, 0)

	for _, v := range grid {
		rawHashes = append(rawHashes, v.Hash().Bytes()...)
	}

	tesseractGrid := &ktypes.TesseractGrid{
		Hash:       common.GetHash(rawHashes),
		Total:      int32(len(grid)),
		Tesseracts: grid,
	}

	return tesseractGrid, nil
}

// createProposal will create a proposal message for the given height,round and tesseract grid
func (kbft *KBFT) createProposal(heights map[common.Address]uint64, round int32) error {
	kbft.logger.Info("Creating proposal", "heights", heights, "round", round)

	var (
		grid *ktypes.TesseractGrid
		err  error
	)

	if kbft.ValidGrid != nil {
		grid = kbft.ValidGrid
	} else {
		grid, err = kbft.createProposalGrid()
		if err != nil {
			return err
		}
	}

	// Create a proposal for tesseract gridID
	proposalGridID, err := grid.GetTesseractGridID()
	if err != nil {
		return err
	}

	proposal := ktypes.NewProposal(heights, round, kbft.ValidRound, grid, proposalGridID)

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

func (kbft *KBFT) enterPreCommit(heights map[common.Address]uint64, round int32) {
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
	gridID, ok := kbft.Votes.getPrevotes(round).TwoThirdMajority()
	if !ok {
		if kbft.LockedGrid != nil {
			log.Println("PreCommit nil due to lock")
		}

		kbft.sendVote(ktypes.PRECOMMIT, nil)

		return
	}

	if err := kbft.publishEventPolka(kbft.RoundStateEvent()); err != nil {
		kbft.logger.Error("Failed to publish polka", "err", err)
	}

	polRound, _ := kbft.Votes.POLInfo()
	if round > polRound {
		log.Panicln("Since 2/3 votes are received polRound should be same.")
	}

	if gridID.IsNil() {
		if kbft.LockedGrid != nil {
			kbft.LockedRound = -1
			kbft.LockedGrid = nil

			if err := kbft.publishEventUnlock(kbft.RoundStateEvent()); err != nil {
				kbft.logger.Error("Failed to publish event unlock", "err", err)
			}
		}

		kbft.sendVote(ktypes.PRECOMMIT, nil)

		return
	}

	if kbft.LockedGrid.CompareHash(gridID.Hash) {
		kbft.LockedRound = round

		if err := kbft.publishEventRelock(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish event relock", "err", err)
		}

		kbft.sendVote(ktypes.PRECOMMIT, gridID)

		return
	}

	if kbft.ProposalGrid.CompareHash(gridID.Hash) {
		// TODO: Validate the tesseractGrid
		kbft.LockedRound = round
		kbft.LockedGrid = kbft.ProposalGrid

		if err := kbft.publishEventLock(kbft.RoundStateEvent()); err != nil {
			kbft.logger.Error("Failed to publish event lock", "err", err)
		}

		kbft.sendVote(ktypes.PRECOMMIT, gridID)

		return
	}

	kbft.LockedRound = -1
	kbft.LockedGrid = nil
	// kbft.ProposalGrid = nil

	if err := kbft.publishEventUnlock(kbft.RoundStateEvent()); err != nil {
		kbft.logger.Error("Failed to publish event unlock", "err", err)
	}

	kbft.sendVote(ktypes.PRECOMMIT, nil)
}

// updateRoundStep
func (kbft *KBFT) updateRoundStep(round int32, step RoundStepType) {
	kbft.Round = round
	kbft.Step = step
}

func (kbft *KBFT) enterPrevote(h map[common.Address]uint64, r int32) {
	kbft.logger.Trace("Entered pre-vote")

	if !areHeightsEqual(kbft.Heights, h) || kbft.Round > r || (kbft.Round == r && kbft.Step >= RoundStepPrevote) {
		return
	}

	defer func() {
		kbft.updateRoundStep(r, RoundStepPrevote)
		kbft.stepChange()
	}()

	if kbft.LockedGrid != nil {
		kbft.logger.Trace("Voting on locked grid", "grid-ID", kbft.LockedGrid.Hash)

		gridID, err := kbft.LockedGrid.GetTesseractGridID()
		if err != nil {
			kbft.logger.Error("Failed to get tesseract grid ID", "err", err)

			return
		}

		kbft.sendVote(ktypes.PREVOTE, gridID)

		return
	}

	if kbft.ProposalGrid == nil {
		kbft.logger.Trace("Proposal grid is nil")
		kbft.sendVote(ktypes.PREVOTE, nil)

		return
	}

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	if err != nil {
		kbft.logger.Error("Failed to get tesseract grid ID", "err", err)

		return
	}

	kbft.sendVote(ktypes.PREVOTE, gridID)
}

// sendVote will send a signed vote message for the given vote-type and tesseractGrid
func (kbft *KBFT) sendVote(msgType ktypes.ConsensusMsgType, tesseractGridID *common.TesseractGridID) *ktypes.Vote {
	kbft.logger.Debug("Sending vote", "vote-type", msgType, "grid-ID", tesseractGridID)

	if kbft.vault == nil {
		kbft.logger.Error("Vault service unavailable during sendVote")

		return nil
	}

	if _, exists := kbft.ics.HasKramaID(kbft.id); !exists {
		return nil
	}

	vote, err := kbft.signVote(msgType, tesseractGridID)
	if err != nil {
		kbft.logger.Error("Error signing the vote message during sendVote", "err", err)

		return nil
	}

	kbft.sendInternalMessage(&ktypes.VoteMessage{Vote: vote})

	return vote
}

// signVote will create a vote message and sign it using the validator consensus key
func (kbft *KBFT) signVote(msgType ktypes.ConsensusMsgType, id *common.TesseractGridID) (*ktypes.Vote, error) {
	valIndex, _ := kbft.ics.HasKramaID(kbft.id)

	if valIndex == -1 {
		return nil, common.ErrKramaIDNotFound
	}

	v := &ktypes.Vote{
		ValidatorIndex: valIndex,
		GridID: &common.TesseractGridID{
			Parts: &common.TesseractParts{
				Grid: getTesseractPartsGridFromHeights(kbft.Heights),
			},
		},
		Round: kbft.Round,
		Type:  msgType,
	}

	if id != nil {
		v.GridID = id
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

func (kbft *KBFT) enterPrevoteWait(h map[common.Address]uint64, r int32) {
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
func (kbft *KBFT) scheduleTimeout(d time.Duration, heights map[common.Address]uint64, r int32, step RoundStepType) {
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
		prevoteSet := prevotes.votesByTesseract[string(kbft.Proposal.GridID.Hash.Bytes())]
		precommitSet := precommits.votesByTesseract[string(kbft.ProposalGrid.Hash.Bytes())]
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
func areHeightsEqual(systemHeights map[common.Address]uint64, newHeights map[common.Address]uint64) bool {
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
	voteHeights map[common.Address]common.TesseractHeightAndHash,
	systemHeights map[common.Address]uint64,
) bool {
	if len(voteHeights) != len(systemHeights) {
		return false
	}

	// Iterate over system heights
	for voteAddress, voteHeightAndHash := range voteHeights {
		systemHeight, ok := systemHeights[voteAddress]
		if !ok || voteHeightAndHash.Height != systemHeight {
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
func areHeightsGreater(systemHeights map[common.Address]uint64, newHeights map[common.Address]uint64) bool {
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

func getTesseractPartsGridFromHeights(
	heights map[common.Address]uint64,
) map[common.Address]common.TesseractHeightAndHash {
	grid := make(map[common.Address]common.TesseractHeightAndHash)

	for address, height := range heights {
		grid[address] = common.TesseractHeightAndHash{
			Height: height,
		}
	}

	return grid
}

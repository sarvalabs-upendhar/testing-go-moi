package kbft

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/guna"
	ktypes "gitlab.com/sarvalabs/moichain/krama/types"
	"gitlab.com/sarvalabs/moichain/mudra"
	common2 "gitlab.com/sarvalabs/moichain/mudra/common"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/telemetry/tracing"
	"gitlab.com/sarvalabs/polo/go-polo"

	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/types"
)

const (
	MaxBFTimeout = 10 * time.Second
)

type vault interface {
	Sign(data []byte, sigType common2.SigType) ([]byte, error)
	KramaID() id.KramaID
}

// KBFT is a struct that represents the runner for the Krama Byzantine Fault Tolerant consensus engine
type KBFT struct {
	RoundState
	logger                    hclog.Logger
	id                        id.KramaID
	config                    *common.ConsensusConfig
	mx                        sync.Mutex
	inboundMsgChan            chan types.ConsensusMessage
	selfMsgChan               chan types.ConsensusMessage
	outboundMsgChan           chan types.ConsensusMessage
	toTicker                  *Ticker
	ics                       *ktypes.ClusterInfo
	nSteps                    int
	closeChan                 chan error
	ctx                       context.Context
	ctxCancel                 context.CancelFunc
	evidence                  *Evidence
	vault                     vault
	wal                       WAL
	finalizedTesseractHandler func(tesseracts []*types.Tesseract) error
}

// NewKBFTService is constructor function that generates a new KBFT engine.
// Accepts a Krama id for the node running the engine, a consensus configuration object, a channel to
// receive consensus messages, a PrivateValidator object, an execution engine and a chain manager.
func NewKBFTService(
	ctx context.Context,
	kid id.KramaID,
	logger hclog.Logger,
	config *common.ConsensusConfig,
	outboundChan, inboundChan chan types.ConsensusMessage,
	vault *mudra.KramaVault,
	evidence *Evidence,
	ics *ktypes.ClusterInfo,
	wal WAL,
	tesseractHandler func(tesseracts []*types.Tesseract) error,
) *KBFT {
	k := &KBFT{
		id:                        kid,
		config:                    config,
		wal:                       wal,
		logger:                    logger.Named("KBFT"),
		outboundMsgChan:           outboundChan,
		inboundMsgChan:            inboundChan,
		selfMsgChan:               make(chan types.ConsensusMessage, 1000),
		toTicker:                  NewTicker(),
		vault:                     vault,
		evidence:                  evidence,
		ics:                       ics,
		finalizedTesseractHandler: tesseractHandler,
		closeChan:                 make(chan error),
	}

	k.ctx, k.ctxCancel = context.WithTimeout(ctx, MaxBFTimeout)
	k.updateToState(ics)

	return k
}

func (kbft *KBFT) updateToState(ics *ktypes.ClusterInfo) {
	log.Println("updating the state", ics.ICS.Nodes)

	var chainIDs []string

	heights := make([]uint64, len(ics.AccountInfos))

	if senderAddr := ics.Ixs[0].FromAddress(); !senderAddr.IsNil() {
		heights[0] = ics.AccountInfos[ics.Ixs[0].FromAddress()].Height.Uint64() + 1
	}

	if receiverAddr := ics.Ixs[0].ToAddress(); !receiverAddr.IsNil() {
		height := ics.AccountInfos[ics.Ixs[0].ToAddress()].Height.Int64()
		if height == -1 {
			heights[2] = ics.AccountInfos[guna.GenesisAddress].Height.Uint64() + 1
		}

		heights[1] = uint64(height + 1)
	}

	log.Println("Heights", heights)

	kbft.toTicker = NewTicker()
	kbft.Height = heights
	kbft.updateRoundStep(0, RoundStepNewHeight)
	kbft.ics = ics
	kbft.Proposal = nil
	kbft.ProposalGrid = nil
	kbft.LockedRound = -1
	kbft.LockedGrid = nil
	kbft.ValidRound = -1
	kbft.ValidGrid = nil
	kbft.Votes = NewHeightVoteSet(chainIDs, heights, ics)
	kbft.CommitRound = -1
	kbft.ics = ics
	kbft.TriggeredTimeoutPrecommit = false
	kbft.stepChange()
}

func (kbft *KBFT) Start() error {
	_, span := tracing.Span(kbft.ctx, "Krama.KBFT", "Start")
	defer span.End()
	// Start the ticker
	if err := kbft.toTicker.Start(); err != nil {
		kbft.logger.Error("Unable to start ticker", "error", err)
	}

	go kbft.ScheduleRound0()

	return kbft.handler(0)
}

func (kbft *KBFT) Close(err error) {
	kbft.logger.Info("Closing KBFT", "error", err)
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

func (kbft *KBFT) HandlePeerMsg(m types.ConsensusMessage) {
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
				kbft.logger.Error("Max steps reached")

				kbft.nSteps = 0

				return errors.New("max steps reached")
			}
		}

		roundState := kbft.RoundState

		select {
		case err := <-kbft.closeChan:
			return err

		case <-kbft.ctx.Done():
			kbft.logger.Info("KBFT Timeout occurred")

			return kbft.ctx.Err()

		case msg := <-kbft.inboundMsgChan:
			kbft.logger.Trace("Handling external Msg", "sender", msg.PeerID)

			if err := kbft.handleMsg(msg); err != nil {
				kbft.logger.Error("Error handling external message", "sender", msg.PeerID)
			}

		case msg := <-kbft.selfMsgChan:
			kbft.logger.Debug("Handling internal Msg", msg.Message)

			if err := kbft.wal.WriteSync(msg, kbft.ics.ID); err != nil {
				kbft.logger.Error("Error Writing to WAL")
			}

			if err := kbft.handleMsg(msg); err != nil {
				kbft.logger.Error("Error handling internal message", "sender", msg.PeerID)
			}

		case t := <-kbft.toTicker.TimeOutChan():
			kbft.logger.Trace("Handling Timeout")
			kbft.handleTimeout(t, roundState)
		}
	}
}

func (kbft *KBFT) handleTimeout(ti timeoutInfo, r RoundState) {
	if !areHeightsEqual(ti.Height, r.Height) || r.Round > ti.Round || (ti.Round == r.Round && ti.Step < r.Step) {
		fmt.Println("returning from time out", ti.Height, r.Height, r.Round, ti.Round, ti.Step, r.Step)

		return
	}

	kbft.mx.Lock()
	defer kbft.mx.Unlock()

	switch ti.Step {
	case RoundStepNewHeight:
		kbft.enterNewRound(ti.Height, 0)

	case RoundStepPropose:
		kbft.enterPrevote(ti.Height, ti.Round)

	case RoundStepPrevoteWait:
		kbft.enterPreCommit(ti.Height, ti.Round)

	case RoundStepPrecommitWait:
		kbft.enterPreCommit(ti.Height, ti.Round)
		kbft.enterNewRound(ti.Height, ti.Round)
	}
}

func (kbft *KBFT) handleMsg(msg types.ConsensusMessage) error {
	kbft.mx.Lock()
	defer kbft.mx.Unlock()

	m, peerID := msg.Message, msg.PeerID

	switch m := m.(type) {
	case *types.ProposalMessage:
		kbft.logger.Trace("Proposal Message Received")

		if err := kbft.setProposal(m.Proposal); err != nil {
			kbft.logger.Trace("Failed to set proposal", err)
		}

	case *types.VoteMessage:
		kbft.logger.Trace("Vote Message Received from ", peerID, m.Vote.Type)

		added, err := kbft.addVote(m.Vote, peerID)
		if err != nil {
			kbft.logger.Error("Failed to add vote", "error", err)
		}

		if added && peerID == kbft.id {
			msg.PeerID = kbft.id
			kbft.logger.Trace("Sending vote message for", "grid-id", m.Vote.GridID.Hash)
			kbft.outboundMsgChan <- msg
		}
	}

	return nil
}

func (kbft *KBFT) setProposal(p *types.Proposal) error {
	if kbft.Proposal != nil {
		return nil
	}

	if !areHeightsEqual(kbft.Height, p.Height) || p.Round != kbft.Round {
		return nil
	}

	if p.POLRound < -1 || (p.POLRound >= 0 && p.POLRound >= p.Round) {
		return errors.New("invalid PoL details")
	}

	// TODO:Verify Proposal Signatures

	kbft.Proposal = p

	// set the proposal grid
	kbft.ProposalGrid = p.Grid

	preVotes := kbft.Votes.getPrevotes(kbft.Round)

	gridID, majority := preVotes.TwoThirdMajority()
	if majority && gridID != nil && (p.Round > kbft.ValidRound) {
		if kbft.ProposalGrid.CompareHash(gridID.Hash) {
			kbft.ValidGrid = kbft.ProposalGrid
			kbft.ValidRound = kbft.Round
		}
	}

	if kbft.Step <= RoundStepPropose && kbft.isProposalReceived() {
		kbft.enterPrevote(kbft.Height, kbft.Round)

		if majority {
			kbft.enterPreCommit(kbft.Height, kbft.Round)
		}
	} else if kbft.Step == RoundStepCommit {
		kbft.finalizeCommit(kbft.Height)
	}

	return nil
}

func (kbft *KBFT) SetProposal(p *types.Proposal, peerID id.KramaID) error {
	if peerID == "" {
		kbft.selfMsgChan <- types.ConsensusMessage{PeerID: "", Message: &types.ProposalMessage{Proposal: p}}
	} else {
		kbft.inboundMsgChan <- types.ConsensusMessage{PeerID: peerID, Message: &types.ProposalMessage{Proposal: p}}
	}

	return nil
}

func (kbft *KBFT) addVote(v *types.Vote, peerID id.KramaID) (added bool, err error) {
	if !areHeightsEqual(v.GridID.Parts.Heights, kbft.Height) {
		kbft.logger.Trace("Invalid vote BFT Height", kbft.Height, v.GridID.Parts.Heights)

		return
	}

	height := kbft.Height

	if kbft.ProposalGrid != nil && v.GridID.Hash != kbft.ProposalGrid.Hash {
		kbft.evidence.AddVote(v)
	}

	added, err = kbft.Votes.AddVote(v, peerID)
	if err != nil || !added {
		kbft.evidence.AddVote(v)

		return
	}

	// TODO: fireevent

	switch v.Type {
	case types.PREVOTE:
		preVotes := kbft.Votes.getPrevotes(v.Round)
		if tesseractGridID, ok := preVotes.TwoThirdMajority(); ok {
			if kbft.LockedGrid != nil &&
				kbft.LockedRound < v.Round &&
				v.Round <= kbft.Round &&
				!kbft.LockedGrid.CompareHash(tesseractGridID.Hash) {
				// Update the locks
				kbft.LockedGrid = nil
				kbft.LockedRound = -1
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

	case types.PRECOMMIT:
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

func (kbft *KBFT) finalizeCommit(h []uint64) {
	if !areHeightsEqual(kbft.Height, h) {
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

	if !areHeightsEqual(kbft.Height, h) || kbft.Step != RoundStepCommit {
		return
	}

	kbft.logger.Info("Tesseract Finalised", "Grid-id", gridID.Hash)

	preCommits := kbft.Votes.getPrecommits(kbft.Round)
	tesseractPreCommits := preCommits.votesByTesseract[string(gridID.Hash.Bytes())]

	aggregatedSignature, err := tesseractPreCommits.AggregateSignatures()
	if err != nil {
		kbft.logger.Error("Error aggregating signatures", err)
		panic(err)
	}

	if err = kbft.updateConsensusInfoInTesseracts(gridID, tesseractPreCommits, aggregatedSignature); err != nil {
		kbft.Close(err)
	}

	if err := kbft.finalizedTesseractHandler(kbft.ProposalGrid.Tesseracts); err != nil {
		kbft.Close(err)
	}

	kbft.logger.Trace("Adding Receipts to dirty storage", "receipt-hash", kbft.ics.Receipts.Hash().Hex())

	// TODO: validate the block

	// TODO: Execute the interactions
	//	var s AccountState
	//	b.updateToState(s)
	//  b.ScheduleRound0(&b.RoundState)

	// Stop the ClusterInfo and other process
	kbft.Close(nil)
}

func (kbft *KBFT) updateConsensusInfoInTesseracts(
	gridID *types.TesseractGridID,
	preCommits *tesseractVoteSet,
	signature []byte,
) (err error) {
	evidenceHash, data := kbft.evidence.FlushEvidence()
	// Add evidence data to dirty list
	kbft.ics.AddDirty(evidenceHash, data)
	// Add Receipts to dirty list
	// This will be modified once smt is integrated
	kbft.ics.AddDirty(kbft.ics.Receipts.Hash(), polo.Polorize(kbft.ics.Receipts))

	for _, tesseract := range kbft.ProposalGrid.Tesseracts {
		tesseract.Header.Extra.Round = kbft.Round
		tesseract.Header.Extra.VoteSet = preCommits.bitarray
		tesseract.Header.Extra.EvidenceHash = evidenceHash
		tesseract.Header.Extra.GridID = gridID
		tesseract.Header.Extra.CommitSignature = signature

		if tesseract.Seal, err = kbft.vault.Sign(tesseract.Bytes(), common2.BlsBLST); err != nil {
			return errors.Wrap(err, "failed to sign the tesseract")
		}
	}

	return nil
}

func (kbft *KBFT) ScheduleRound0() {
	kbft.logger.Info("Scheduling Round 0. TO:")
	kbft.scheduleTimeout(100*time.Millisecond, kbft.Height, 0, RoundStepNewHeight)
}

func (kbft *KBFT) enterCommit(heights []uint64, round int32) {
	kbft.logger.Trace("Entering Commit", "step", kbft.Step)

	if !areHeightsEqual(kbft.Height, heights) || RoundStepCommit <= kbft.Step {
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

func (kbft *KBFT) enterPrecommitWait(heights []uint64, r int32) {
	if !areHeightsEqual(kbft.Height, heights) ||
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

func (kbft *KBFT) enterNewRound(heights []uint64, round int32) {
	if !areHeightsEqual(kbft.Height, heights) ||
		round < kbft.Round ||
		(kbft.Round == round && kbft.Step != RoundStepNewHeight) {
		return
	}

	kbft.logger.Trace("Entering New KBFT Round", "Heights", heights, "Round", round)
	kbft.updateRoundStep(round, RoundStepNewHeight)

	if round != 0 {
		kbft.Proposal = nil
		kbft.ProposalGrid = nil
	}

	kbft.Votes.SetRound(round + 1)
	kbft.TriggeredTimeoutPrecommit = false

	// Now ready to enter propose
	kbft.enterPropose(heights, round)
}

func (kbft *KBFT) enterPropose(heights []uint64, round int32) {
	if !areHeightsEqual(kbft.Height, heights) ||
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
		log.Panic(
			"Validator id missing",
			kbft.vault.KramaID(),
			kbft.ics.ICS.Nodes[0].Ids,
			kbft.ics.ICS.Nodes[1].Ids,
			kbft.ics.ICS.Nodes[1].Ids,
			kbft.ics.ICS.Nodes[3].Ids,
			kbft.ics.ICS.Nodes[4].Ids,
			kbft.ics.ICS.Nodes[5].Ids,
		)

		return
	}

	if err := kbft.createProposal(heights, round); err != nil {
		kbft.Close(err)
	}
}

func (kbft *KBFT) createProposalGrid() (*types.TesseractGrid, error) {
	grid := kbft.ics.GetTesseractGrid()
	if grid == nil {
		return nil, errors.New("invalid tesseract grid")
	}

	rawHashes := make([]byte, 0)

	for _, v := range grid {
		rawHashes = append(rawHashes, v.Hash().Bytes()...)
	}

	tesseractGrid := &types.TesseractGrid{
		Hash:       types.GetHash(rawHashes),
		Total:      int32(len(grid)),
		Tesseracts: grid,
	}

	return tesseractGrid, nil
}

// createProposal will create a proposal message for the given height,round and tesseract grid
func (kbft *KBFT) createProposal(heights []uint64, round int32) error {
	kbft.logger.Info("Creating proposal", "Heights", heights, "Round", round)

	var (
		grid *types.TesseractGrid
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
	proposalGridID := grid.GetTesseractGridID()
	proposal := types.NewProposal(heights, round, kbft.ValidRound, grid, proposalGridID)

	// Send an internal message
	kbft.sendInternalMessage(&types.ProposalMessage{Proposal: proposal})

	return nil
}

func (kbft *KBFT) sendInternalMessage(msg types.Cmessage) {
	kbft.selfMsgChan <- types.ConsensusMessage{PeerID: kbft.id, Message: msg}
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

func (kbft *KBFT) enterPreCommit(heights []uint64, round int32) {
	kbft.logger.Trace("Entered PreCommit", "round", round)

	if !areHeightsEqual(kbft.Height, heights) ||
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

		kbft.sendVote(types.PRECOMMIT, nil)

		return
	}

	polRound, _ := kbft.Votes.POLInfo()
	if round > polRound {
		log.Panicln("since 2/3 votes are received polRound should be same ")
	}

	if gridID.IsNil() {
		if kbft.LockedGrid != nil {
			kbft.LockedRound = -1
			kbft.LockedGrid = nil
		}

		kbft.sendVote(types.PRECOMMIT, nil)

		return
	}

	if kbft.LockedGrid.CompareHash(gridID.Hash) {
		kbft.LockedRound = round
		kbft.sendVote(types.PRECOMMIT, gridID)

		return
	}

	if kbft.ProposalGrid.CompareHash(gridID.Hash) {
		// TODO: Validate the tesseractGrid
		kbft.LockedRound = round
		kbft.LockedGrid = kbft.ProposalGrid
		kbft.sendVote(types.PRECOMMIT, gridID)

		return
	}

	kbft.LockedRound = -1
	kbft.LockedGrid = nil
	// kbft.ProposalGrid = nil
	kbft.sendVote(types.PRECOMMIT, nil)
}

// updateRoundStep
func (kbft *KBFT) updateRoundStep(round int32, step RoundStepType) {
	kbft.Round = round
	kbft.Step = step
}

func (kbft *KBFT) enterPrevote(h []uint64, r int32) {
	kbft.logger.Trace("Entered PreVote")

	if !areHeightsEqual(kbft.Height, h) || kbft.Round > r || (kbft.Round == r && kbft.Step >= RoundStepPrevote) {
		return
	}

	defer func() {
		kbft.updateRoundStep(r, RoundStepPrevote)
		kbft.stepChange()
	}()

	if kbft.LockedGrid != nil {
		kbft.logger.Trace("Voting on locked grid", "grid-id", kbft.LockedGrid.Hash.Hex())
		kbft.sendVote(types.PREVOTE, kbft.LockedGrid.GetTesseractGridID())

		return
	}

	if kbft.ProposalGrid == nil {
		kbft.logger.Trace("Proposal grid is nil")
		kbft.sendVote(types.PREVOTE, nil)

		return
	}

	// TODO: Validate the block and vote
	kbft.sendVote(types.PREVOTE, kbft.ProposalGrid.GetTesseractGridID())
}

// sendVote will send a signed vote message for the given vote-type and tesseractGrid
func (kbft *KBFT) sendVote(msgType types.ConsensusMsgType, tesseractGridID *types.TesseractGridID) *types.Vote {
	kbft.logger.Debug("Sending vote", "vote-type", msgType, "grid-id", tesseractGridID)

	if kbft.vault == nil {
		kbft.logger.Error("Vault service unavailable [sendVote]")

		return nil
	}

	if _, exists := kbft.ics.HasKramaID(kbft.id); !exists {
		return nil
	}

	vote, err := kbft.signVote(msgType, tesseractGridID)
	if err != nil {
		kbft.logger.Error("Error signing the vote message [sendVote]", "error", err)

		return nil
	}

	kbft.sendInternalMessage(&types.VoteMessage{Vote: vote})

	return vote
}

// signVote will create a vote message and sign it using the validator consensus key
func (kbft *KBFT) signVote(msgType types.ConsensusMsgType, id *types.TesseractGridID) (*types.Vote, error) {
	valIndex, _ := kbft.ics.HasKramaID(kbft.id)

	if valIndex == -1 {
		return nil, types.ErrKramaIDNotFound
	}

	v := &types.Vote{
		ValidatorIndex: valIndex,
		GridID: &types.TesseractGridID{
			Hash: types.NilHash,
			Parts: &types.TesseractParts{
				Total:   0,
				Hashes:  make([]types.Hash, 0),
				Heights: make([]uint64, 0),
			},
		},
		Round: kbft.Round,
		Type:  msgType,
	}

	if id != nil {
		v.GridID.Hash = id.Hash
		v.GridID.Parts.Total = id.Parts.Total
		v.GridID.Parts.Hashes = id.Parts.Hashes
		v.GridID.Parts.Heights = id.Parts.Heights
	}

	sign, err := kbft.vault.Sign(v.SignBytes(), common2.BlsBLST)
	if err != nil {
		return nil, err
	}

	v.Signature = make([]byte, len(sign))
	copy(v.Signature, sign)

	return v, nil
}

func (kbft *KBFT) enterPrevoteWait(h []uint64, r int32) {
	kbft.logger.Trace("Entered PreVoteWait")

	if !areHeightsEqual(kbft.Height, h) ||
		kbft.Round > r ||
		(kbft.Step >= RoundStepPrevoteWait && kbft.Round == r) {
		return
	}

	if !kbft.Votes.getPrevotes(r).HasMajorityAny() {
		kbft.logger.Error("Entered PreVoteWait without majority")

		return
	}

	defer func() {
		kbft.updateRoundStep(r, RoundStepPrevoteWait)
		kbft.stepChange()
	}()

	kbft.scheduleTimeout(kbft.prevoteTimeout(r), h, r, RoundStepPrevoteWait)
}

// scheduleTimeout will schedule a timeout for the given step,round and height
func (kbft *KBFT) scheduleTimeout(d time.Duration, heights []uint64, r int32, step RoundStepType) {
	kbft.logger.Info("Scheduling timeout", "step", step, "duration", d, "heights", heights)
	kbft.toTicker.ScheduleTimeout(timeoutInfo{d, heights, r, step})
}

func (kbft *KBFT) stepChange() {
	// Write to the log
	kbft.nSteps++
}

func (kbft *KBFT) PrintMetrics() {
	prevotes := kbft.Votes.getPrevotes(0)
	precommits := kbft.Votes.getPrecommits(0)
	kbft.logger.Debug("Printing metrics")

	if kbft.Proposal != nil {
		prevoteSet := prevotes.votesByTesseract[string(kbft.Proposal.GridID.Hash.Bytes())]
		precommitSet := precommits.votesByTesseract[string(kbft.ProposalGrid.Hash.Bytes())]
		kbft.logger.Debug("Validators", "list", prevotes.valset.ICS.String())
		kbft.logger.Debug("Prevote Received", prevoteSet.bitarray)
		kbft.logger.Debug("Precommit Received", precommitSet.bitarray)
	}
}

// A function that checks if two sets of heights are equal.
// Accepts two sets of heights (int64 slice) and compares them. Returns a bool.
// Every index in each slice must be equal for true result.
func areHeightsEqual(systemHeight []uint64, newHeight []uint64) bool {
	// Iterate over system heights
	for idx, value := range systemHeight {
		if value != newHeight[idx] {
			// Height mismatch, return false
			return false
		}
	}

	// Heights match, return true
	return true
}

// A function that checks if the second set of heights is greater than the first.
// Accepts two sets of heights (int64 slice) and compares them. Returns a bool.
// Every index in the second set must be greater than the first set for a true result.
func areHeightsGreater(systemHeight []uint64, newHeight []uint64) bool {
	// Iterate over system heights
	for idx, value := range systemHeight {
		if value < newHeight[idx] {
			// Height lesser, return false
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

package kbft

import (
	"time"

	"github.com/sarvalabs/moichain/krama/types"
)

// A type alias that represents a type of consensus round step
type RoundStepType uint8

// TODO: Redo documentation for round steps
const (
	RoundStepNewHeight = RoundStepType(0x01)

	RoundStepNewRound = RoundStepType(0x02)

	RoundStepPropose = RoundStepType(0x03)

	RoundStepPrevote = RoundStepType(0x04)

	RoundStepPrevoteWait = RoundStepType(0x05)

	RoundStepPrecommit = RoundStepType(0x06)

	RoundStepPrecommitWait = RoundStepType(0x07)

	RoundStepCommit = RoundStepType(0x08)
)

// IsValid is a method of RoundStepType that returns a bool indicating if the type is valid/known
func (rtype RoundStepType) IsValid() bool {
	return uint8(rtype) >= 0x01 && uint8(rtype) <= 0x08
}

// String is a method of RoundStepType that returns the string representation of the round type
func (rtype RoundStepType) String() string {
	switch rtype {
	case RoundStepNewHeight:
		return "RoundStepNewHeight"
	case RoundStepNewRound:
		return "RoundStepNewRound"
	case RoundStepPropose:
		return "RoundStepPropose"
	case RoundStepPrevote:
		return "RoundStepPrevote"
	case RoundStepPrevoteWait:
		return "RoundStepPrevoteWait"
	case RoundStepPrecommit:
		return "RoundStepPrecommit"
	case RoundStepPrecommitWait:
		return "RoundStepPrecommitWait"
	case RoundStepCommit:
		return "RoundStepCommit"
	default:
		// Avoids panics
		return "RoundStepUnknown"
	}
}

// TODO: Add JSON reflect tags for missing fields
// TODO: Add missing documentation

// RoundState is a struct that represent the state of a consensus round
type RoundState struct {
	// Represents the current round heights being worked on
	Heights []uint64 `json:"heights"`

	// Represents the current round number
	Round int32 `json:"round"`

	// Represents the current round step type
	Step RoundStepType `json:"step"`

	// Represents the start time of the round
	StartTime time.Time `json:"start_time"`

	// Represents a subjective time when 2/3+ precommits for Block in round were found
	CommitTime time.Time `json:"commit_time"`

	// Represents the set of votes across the heights
	Votes *HeightVoteSet `json:"votes"`

	// Represents the round with last known commit
	CommitRound int32 `json:"commit_round"`

	// Represents the round proposal
	Proposal *types.Proposal `json:"proposal"`

	// Represents the proposed tesseract grid for the round
	ProposalGrid *types.TesseractGrid `json:"proposal_tesseract"`

	// TODO: Add docs
	LockedRound int32 `json:"locked_round"`

	LockedGrid *types.TesseractGrid `json:"locked_tesseract"`

	// Represents the last known round with proof of lock for a non-nil valid block
	ValidRound int32 `json:"valid_round"`

	// Represents the tesseract grid (block) for the last known round with proof of lock for a non-nil valid block
	ValidGrid *types.TesseractGrid `json:"valid_tesseract"`

	// Represents the last precommit at height-1
	LastCommit *VoteSet `json:"last_commit"`

	LastValidators *ValidatorSet `json:"last_validators"`

	TriggeredTimeoutPrecommit bool `json:"triggered_timeout_precommit"`
}

// RoundStateEvent returns the H/R/S of the RoundState as an event.
func (rs *RoundState) RoundStateEvent() eventDataRoundState {
	return eventDataRoundState{
		Height: rs.Heights,
		Round:  rs.Round,
		Step:   rs.Step.String(),
	}
}

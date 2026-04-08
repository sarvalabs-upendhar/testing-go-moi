package kbft

import (
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

// A type alias that represents a type of consensus view step
type ViewStepType uint8

// TODO: Redo documentation for view steps
const (
	ViewStepNewHeight = ViewStepType(0x01)

	ViewStepNewView = ViewStepType(0x02)

	ViewStepPropose = ViewStepType(0x03)

	ViewStepPrevote = ViewStepType(0x04)

	ViewStepPrecommit = ViewStepType(0x05)

	ViewStepCommit = ViewStepType(0x06)
)

// IsValid is a method of ViewStepType that returns a bool indicating if the type is valid/known
func (rtype ViewStepType) IsValid() bool {
	return uint8(rtype) >= 0x01 && uint8(rtype) <= 0x06
}

// String is a method of ViewStepType that returns the string representation of the view type
func (rtype ViewStepType) String() string {
	switch rtype {
	case ViewStepNewHeight:
		return "ViewStepNewHeight"
	case ViewStepNewView:
		return "ViewStepNewView"
	case ViewStepPropose:
		return "ViewStepPropose"
	case ViewStepPrevote:
		return "ViewStepPrevote"
	case ViewStepPrecommit:
		return "ViewStepPrecommit"
	case ViewStepCommit:
		return "ViewStepCommit"
	default:
		// Avoids panics
		return "viewStepUnknown"
	}
}

// TODO: Add missing documentation

// ViewState is a struct that represent the state of a consensus view
type ViewState struct {
	// Represents the current view heights being worked on
	Heights map[identifiers.Identifier]uint64 `json:"heights"`

	// Represents the current view number
	View uint64 `json:"view"`

	// Represents the current view step type
	Step ViewStepType `json:"step"`

	// Represents the start time of the view
	StartTime time.Time `json:"start_time"`

	// Represents a subjective time when 2/3+ precommits for Block in view were found
	CommitTime time.Time `json:"commit_time"`

	// Represents the set of votes across the heights
	Votes *ktypes.HeightVoteSet `json:"votes"`

	// Represents the view with last known commit
	CommitView uint64 `json:"commit_view"`

	// Represents the view proposal
	Proposal *ktypes.Proposal `json:"proposal"`

	// Represents the proposed tesseract for the view
	ProposalTS *common.Tesseract `json:"proposal_tesseract"`

	//// Represents the last known view with proof of lock for a non-nil valid block
	// ValidView map[identifiers.Identifier]uint64 `json:"valid_view"`
	//
	//// Represents the tesseract (block) for the last known view with proof of lock for a non-nil valid block
	// ValidTS map[identifiers.Identifier]*common.Tesseract `json:"valid_tesseract"`

	// Represents the last precommit at height-1
	LastCommit *ktypes.VoteSet `json:"last_commit"`

	LastValidators *ValidatorSet `json:"last_validators"`
}

// viewStateEvent returns the H/R/S of the ViewState as an event.
func (rs *ViewState) viewStateEvent() eventDataViewState {
	return eventDataViewState{
		Height: rs.Heights,
		View:   rs.View,
		Step:   rs.Step.String(),
	}
}

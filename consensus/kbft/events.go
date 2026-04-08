package kbft

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

type eventDataViewState struct {
	Height map[identifiers.Identifier]uint64 `json:"height"`
	View   uint64                            `json:"view"`
	Step   string                            `json:"step"`
}

type EventVote struct {
	Vote *ktypes.Vote
}

type eventPolka struct {
	eventDataViewState
}

type eventNewViewStep struct {
	eventDataViewState
}

type eventNewView struct {
	eventDataViewState
}

type EventProposal struct {
	Proposal *ktypes.Proposal
}

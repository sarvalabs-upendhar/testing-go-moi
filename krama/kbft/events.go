package kbft

import (
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/types"
)

type eventDataRoundState struct {
	Height map[types.Address]uint64 `json:"height"`
	Round  int32                    `json:"round"`
	Step   string                   `json:"step"`
}

type eventProposal struct {
	proposal *ktypes.Proposal
}

type eventVote struct {
	vote *ktypes.Vote
}

type eventRelock struct {
	eventDataRoundState
}

type eventLock struct {
	eventDataRoundState
}

type eventUnlock struct {
	eventDataRoundState
}

type eventPolka struct {
	eventDataRoundState
}

type eventNewRoundStep struct {
	eventDataRoundState
}

type eventNewRound struct {
	eventDataRoundState
}

type eventTimeoutPropose struct {
	eventDataRoundState
}

type eventTimeoutPrevote struct {
	eventDataRoundState
}

type eventTimeoutPrecommit struct {
	eventDataRoundState
}

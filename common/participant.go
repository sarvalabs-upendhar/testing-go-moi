package common

import "github.com/sarvalabs/go-moi-identifiers"

type LockType int

const (
	ReadLock LockType = iota // TODO: Should improve lock types
	WriteLock
)

// IxParticipant holds all basic information of the participant account
type IxParticipant struct {
	AccType   AccountType
	IsSigner  bool
	LockType  LockType
	IsGenesis bool
}

// Participant holds all the information required to achieve consensus on the participants state
type Participant struct {
	AccType       AccountType
	Address       identifiers.Address
	IsGenesis     bool
	IsSigner      bool
	Height        uint64
	ContextHash   Hash
	TesseractHash Hash
	// The participants within an ICS file are arranged by their addresses.
	// This field indicates the initial index of the participant's nodeSet.
	NodeSetPosition int
	LockType        LockType
	ContextDelta    *DeltaGroup
	// ConsensusQuorum represents the minimum required consensus votes
	ConsensusQuorum uint32
}

func (p *Participant) NewHeight() uint64 {
	if p.IsGenesis {
		return p.Height
	}

	return p.Height + 1
}

func (p *Participant) TSHash() Hash {
	return p.TesseractHash
}

// IsContextUpdateRequired returns true if this participant is the signer and account type is regular
func (p *Participant) IsContextUpdateRequired() bool {
	if !p.IsSigner && p.AccType == RegularAccount {
		return false
	}

	return true
}

type Participants map[identifiers.Address]*Participant

func (ps Participants) IxnParticipants() map[identifiers.Address]IxParticipant {
	ixnParticipants := make(map[identifiers.Address]IxParticipant)

	for k, v := range ps {
		ixnParticipants[k] = IxParticipant{
			IsGenesis: v.IsGenesis,
			IsSigner:  v.IsSigner,
			AccType:   v.AccType,
		}
	}

	return ixnParticipants
}

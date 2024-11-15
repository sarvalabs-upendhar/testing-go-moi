package common

import (
	"sort"

	"github.com/sarvalabs/go-moi-identifiers"
)

type LockType int

const (
	MutateLock LockType = iota
	ReadLock
	NoLock
)

// ParticipantInfo holds all basic information of the participant account
type ParticipantInfo struct {
	Address   identifiers.Address
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
	// This field indicates the index of the participant's nodeSet.
	NodeSetPosition int
	LockType        LockType
	ContextDelta    *DeltaGroup
	// ConsensusQuorum represents the minimum required consensus votes
	ConsensusQuorum uint32
	CommitHash      Hash
	ExcludeFromICS  bool
}

func (p *Participant) NewHeight() uint64 {
	if p.IsGenesis || p.LockType > MutateLock || p.ExcludeFromICS {
		return p.Height
	}

	return p.Height + 1
}

func (p *Participant) TSHash() Hash {
	return p.TesseractHash
}

// IsContextUpdateRequired returns true if this participant is the signer and account type is regular
func (p *Participant) IsContextUpdateRequired() bool {
	if !p.IsSigner && !p.IsGenesis && p.AccType == RegularAccount {
		return false
	}

	return true
}

func (p *Participant) ExcludedFromICS() bool {
	return p.ExcludeFromICS
}

type Participants map[identifiers.Address]*Participant

func (ps Participants) IxnParticipants() map[identifiers.Address]ParticipantInfo {
	ixnParticipants := make(map[identifiers.Address]ParticipantInfo)

	for k, v := range ps {
		ixnParticipants[k] = ParticipantInfo{
			IsGenesis: v.IsGenesis,
			IsSigner:  v.IsSigner,
			AccType:   v.AccType,
		}
	}

	return ixnParticipants
}

func (ps Participants) HasSystemAccounts() bool {
	for addr := range ps {
		if IsSystemAccount(addr) {
			return true
		}
	}

	return false
}

func (ps Participants) LockInfo(activeParticipantsOnly bool) map[identifiers.Address]LockType {
	lockInfo := make(map[identifiers.Address]LockType)

	for addr, info := range ps {
		if !activeParticipantsOnly {
			lockInfo[addr] = info.LockType

			continue
		}

		if !info.ExcludedFromICS() {
			lockInfo[addr] = info.LockType
		}
	}

	return lockInfo
}

func (ps Participants) ExcludeFromICS(addrs Addresses) {
	for _, addr := range addrs {
		if info := ps[addr]; info != nil {
			info.ExcludeFromICS = true
		}
	}
}

func (ps Participants) Addrs() Addresses {
	addrs := make(Addresses, 0, len(ps))

	for addr := range ps {
		addrs = append(addrs, addr)
	}

	sort.Sort(addrs)

	return addrs
}

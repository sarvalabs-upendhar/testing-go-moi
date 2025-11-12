package common

import (
	"sort"

	"github.com/sarvalabs/go-moi/common/identifiers"
)

type LockType int

const (
	MutateLock LockType = iota
	ReadLock
	NoLock
)

func (lt LockType) String() string {
	switch lt {
	case MutateLock:
		return "MutateLock"
	case ReadLock:
		return "ReadLock"
	case NoLock:
		return "NoLock"
	default:
		return "Unknown"
	}
}

// ParticipantInfo holds all basic information of the participant account
type ParticipantInfo struct {
	ID        identifiers.Identifier
	IsSigner  bool
	LockType  LockType
	IsGenesis bool
}

func (pi *ParticipantInfo) AccountType() AccountType {
	return AccountTypeFromID(pi.ID)
}

// Participant holds all the information required to achieve consensus on the participants state
type Participant struct {
	ID                 identifiers.Identifier
	IsGenesis          bool
	IsSigner           bool
	Height             uint64
	ContextHash        Hash
	ConsensusNodesHash Hash
	TesseractHash      Hash
	// The participants within an ICS file are arranged by their ids.
	// This field indicates the index of the participant's nodeSet.
	NodeSetPosition int
	LockType        LockType
	ContextDelta    *DeltaGroup
	// ConsensusQuorum represents the minimum required consensus votes
	ConsensusQuorum uint32
	CommitHash      Hash
	ExcludeFromICS  bool
}

func (p *Participant) AccountType() AccountType {
	return AccountTypeFromID(p.ID)
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
	if p.ID.IsParticipantVariant() {
		return false
	}

	if !p.IsSigner && !p.IsGenesis && p.AccountType() == RegularAccount {
		return false
	}

	return true
}

func (p *Participant) ExcludedFromICS() bool {
	return !p.IsGenesis && p.ExcludeFromICS // TODO: Check this logic
}

type Participants map[identifiers.Identifier]*Participant

func (ps Participants) IxnParticipants() map[identifiers.Identifier]ParticipantInfo {
	ixnParticipants := make(map[identifiers.Identifier]ParticipantInfo)

	for k, v := range ps {
		ixnParticipants[k] = ParticipantInfo{
			IsGenesis: v.IsGenesis,
			IsSigner:  v.IsSigner,
		}
	}

	return ixnParticipants
}

func (ps Participants) HasSystemAccounts() bool {
	for id := range ps {
		if IsSystemAccount(id) {
			return true
		}
	}

	return false
}

func (ps Participants) LockInfo(activeParticipantsOnly bool) map[identifiers.Identifier]LockType {
	lockInfo := make(map[identifiers.Identifier]LockType)

	for id, info := range ps {
		if !activeParticipantsOnly {
			lockInfo[id] = info.LockType

			continue
		}

		if !info.ExcludedFromICS() {
			lockInfo[id] = info.LockType
		}
	}

	return lockInfo
}

func (ps Participants) ExcludeFromICS(ids IdentifierList) {
	for _, id := range ids {
		if info := ps[id]; info != nil {
			info.ExcludeFromICS = true
		}
	}
}

func (ps Participants) IDs() IdentifierList {
	ids := make(IdentifierList, 0, len(ps))

	for id := range ps {
		ids = append(ids, id)
	}

	sort.Sort(ids)

	return ids
}

func (ps Participants) AccountsWithoutNoLocks() IdentifierList {
	ids := make(IdentifierList, 0)

	for id, state := range ps {
		if state.LockType == NoLock {
			continue
		}

		ids = append(ids, id)
	}

	sort.Sort(ids)

	return ids
}

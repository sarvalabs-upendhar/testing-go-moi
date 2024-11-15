package common

import (
	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

type IxBatch struct {
	ixs []*Interaction
	ps  map[identifiers.Address]struct{}
}

func NewIxnBatch() *IxBatch {
	return &IxBatch{
		ixs: make([]*Interaction, 0),
		ps:  make(map[identifiers.Address]struct{}),
	}
}

func (ib *IxBatch) Ps() map[identifiers.Address]struct{} {
	return ib.ps
}

func (ib *IxBatch) IxList() []*Interaction {
	return ib.ixs
}

func (ib *IxBatch) Interactions() Interactions {
	return NewInteractionsWithLeaderCheck(true, ib.ixs...)
}

func (ib *IxBatch) Flush() {
	ib.ixs = nil
	ib.ps = nil
}

func (ib *IxBatch) IxCount() int {
	return len(ib.ixs)
}

func (ib *IxBatch) PsCount() int {
	return len(ib.ps)
}

func (ib *IxBatch) totalSlots() int {
	return MaxSlotsForIxBatch
}

func (ib *IxBatch) UniqueAccounts(ps map[identifiers.Address]*ParticipantInfo) int {
	count := 0

	for addr, info := range ps {
		if info.IsGenesis {
			continue
		}

		if _, ok := ib.ps[addr]; !ok {
			count++
		}
	}

	return count + len(ib.ps)
}

func (ib *IxBatch) AppendIxns(ixns []*Interaction) {
	ib.ixs = append(ib.ixs, ixns...)
}

func (ib *IxBatch) Add(ixn *Interaction) bool {
	if ib.IxCount() > 100 {
		return false
	}

	uniqueAccs := ib.UniqueAccounts(ixn.ps)
	if uniqueAccs > ib.totalSlots() {
		return false
	}

	ib.ixs = append(ib.ixs, ixn)

	for addr, info := range ixn.ps {
		if info.IsGenesis {
			continue
		}

		ib.ps[addr] = struct{}{}
	}

	return true
}

package common

import (
	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

type IxBatch struct {
	ixs []*Interaction
	ps  map[identifiers.Identifier]struct{}
}

func NewIxnBatch() *IxBatch {
	return &IxBatch{
		ixs: make([]*Interaction, 0),
		ps:  make(map[identifiers.Identifier]struct{}),
	}
}

func (ib *IxBatch) Ps() map[identifiers.Identifier]struct{} {
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

func (ib *IxBatch) AppendIxns(ixns []*Interaction) {
	ib.ixs = append(ib.ixs, ixns...)
}

func (ib *IxBatch) Add(ixn *Interaction) bool {
	if ib.IxCount() > 100 {
		return false
	}

	ib.ixs = append(ib.ixs, ixn)

	for id, info := range ixn.ps {
		if info.IsGenesis {
			continue
		}

		ib.ps[id] = struct{}{}
	}

	return true
}

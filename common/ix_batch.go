package common

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type IxBatch struct {
	ixs                []*Interaction
	consensusNodesHash map[Hash]struct{}
}

func NewIxnBatch() *IxBatch {
	return &IxBatch{
		ixs:                make([]*Interaction, 0),
		consensusNodesHash: make(map[Hash]struct{}),
	}
}

func (ib *IxBatch) ConsensusNodesHash() map[Hash]struct{} {
	return ib.consensusNodesHash
}

func (ib *IxBatch) IxList() []*Interaction {
	return ib.ixs
}

func (ib *IxBatch) Interactions() Interactions {
	return NewInteractionsWithLeaderCheck(true, ib.ixs...)
}

func (ib *IxBatch) Flush() {
	ib.ixs = nil
	ib.consensusNodesHash = nil
}

func (ib *IxBatch) IxCount() int {
	return len(ib.ixs)
}

func (ib *IxBatch) ConsensusNodesHashCount() int {
	return len(ib.consensusNodesHash)
}

func (ib *IxBatch) AppendIxns(ixns []*Interaction) {
	ib.ixs = append(ib.ixs, ixns...)
}

func (ib *IxBatch) Add(ixn *Interaction, consensusNodesHashes map[identifiers.Identifier]Hash) bool {
	if ib.IxCount() > 100 {
		return false
	}

	ib.ixs = append(ib.ixs, ixn)

	for id, info := range ixn.ps {
		if info.IsGenesis {
			continue
		}

		ib.consensusNodesHash[consensusNodesHashes[id]] = struct{}{}
	}

	return true
}

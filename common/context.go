package common

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

const NodeNotFound = -1

const (
	ConsensusNodesSize = 5
	StochasticSetSize  = 5
)

type NodeList []*ValidatorInfo

func (nl NodeList) Contains(id identifiers.KramaID) bool {
	for _, info := range nl {
		if id == info.KramaID {
			return true
		}
	}

	return false
}

func (nl NodeList) Len() int {
	return len(nl)
}

func (nl NodeList) KramaIDs() []identifiers.KramaID {
	kramaIDs := make([]identifiers.KramaID, nl.Len())

	for i, info := range nl {
		kramaIDs[i] = info.KramaID
	}

	return kramaIDs
}

type ContextDelta map[identifiers.Identifier]*DeltaGroup

func (delta ContextDelta) Copy() ContextDelta {
	if len(delta) == 0 {
		return nil
	}

	contextDelta := make(ContextDelta)

	for key, value := range delta {
		contextDelta[key] = value.Copy()
	}

	return contextDelta
}

type DeltaGroup struct {
	ConsensusNodes []identifiers.KramaID `json:"consensus_nodes"`
	ReplacedNodes  []identifiers.KramaID `json:"replaced_nodes"`
}

func (d DeltaGroup) Copy() *DeltaGroup {
	deltaGroup := &DeltaGroup{}

	if len(d.ConsensusNodes) > 0 {
		deltaGroup.ConsensusNodes = make([]identifiers.KramaID, len(d.ConsensusNodes))
		copy(deltaGroup.ConsensusNodes, d.ConsensusNodes)
	}

	if len(d.ReplacedNodes) > 0 {
		deltaGroup.ReplacedNodes = make([]identifiers.KramaID, len(d.ReplacedNodes))
		copy(deltaGroup.ReplacedNodes, d.ReplacedNodes)
	}

	return deltaGroup
}

func (d DeltaGroup) NodeIndex(id identifiers.KramaID) int {
	idx := 0

	for _, node := range d.ConsensusNodes {
		if node == id {
			return idx
		}

		idx++
	}

	return NodeNotFound
}

type ContextLockInfo struct {
	ContextHash   Hash   `json:"context_hash"`
	Height        uint64 `json:"height"`
	TesseractHash Hash   `json:"tesseract_hash"`
}

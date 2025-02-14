package common

import (
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

const NodeNotFound = -1

const (
	ConsensusNodesSize = 5
	StochasticSetSize  = 5
)

type NodeList []kramaid.KramaID

func (nl NodeList) Contains(id kramaid.KramaID) bool {
	for _, kramaID := range nl {
		if id == kramaID {
			return true
		}
	}

	return false
}

func (nl NodeList) Len() int {
	return len(nl)
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
	ConsensusNodes []kramaid.KramaID `json:"consensus_nodes"`
	ReplacedNodes  []kramaid.KramaID `json:"replaced_nodes"`
}

func (d DeltaGroup) Copy() *DeltaGroup {
	deltaGroup := &DeltaGroup{}

	if len(d.ConsensusNodes) > 0 {
		deltaGroup.ConsensusNodes = make([]kramaid.KramaID, len(d.ConsensusNodes))
		copy(deltaGroup.ConsensusNodes, d.ConsensusNodes)
	}

	if len(d.ReplacedNodes) > 0 {
		deltaGroup.ReplacedNodes = make([]kramaid.KramaID, len(d.ReplacedNodes))
		copy(deltaGroup.ReplacedNodes, d.ReplacedNodes)
	}

	return deltaGroup
}

func (d DeltaGroup) NodeIndex(id kramaid.KramaID) int {
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

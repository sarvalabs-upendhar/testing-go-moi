package common

import (
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

type ParticipantRole int

const (
	Sender ParticipantRole = iota
	Receiver
	Genesis
)

const NodeNotFound = -1

const (
	BehaviouralContextSize = 5
	StochasticSetSize      = 5
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

type ContextDelta map[identifiers.Address]*DeltaGroup

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
	BehaviouralNodes []kramaid.KramaID `json:"behavioural_nodes"`
	RandomNodes      []kramaid.KramaID `json:"random_nodes"`
	ReplacedNodes    []kramaid.KramaID `json:"replaced_nodes"`
}

func (d DeltaGroup) Copy() *DeltaGroup {
	deltaGroup := &DeltaGroup{}

	if len(d.BehaviouralNodes) > 0 {
		deltaGroup.BehaviouralNodes = make([]kramaid.KramaID, len(d.BehaviouralNodes))
		copy(deltaGroup.BehaviouralNodes, d.BehaviouralNodes)
	}

	if len(d.RandomNodes) > 0 {
		deltaGroup.RandomNodes = make([]kramaid.KramaID, len(d.RandomNodes))
		copy(deltaGroup.RandomNodes, d.RandomNodes)
	}

	if len(d.ReplacedNodes) > 0 {
		deltaGroup.ReplacedNodes = make([]kramaid.KramaID, len(d.ReplacedNodes))
		copy(deltaGroup.ReplacedNodes, d.ReplacedNodes)
	}

	return deltaGroup
}

func (d DeltaGroup) NodeIndex(id kramaid.KramaID) int {
	idx := 0

	for _, node := range d.BehaviouralNodes {
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

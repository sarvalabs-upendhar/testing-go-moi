package common

import id "github.com/sarvalabs/go-moi/common/kramaid"

type ParticipantRole int

const (
	Sender ParticipantRole = iota
	Receiver
	Genesis
)

type ContextDelta map[Address]*DeltaGroup

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
	Role             ParticipantRole `json:"role"`
	BehaviouralNodes []id.KramaID    `json:"behavioural_nodes"`
	RandomNodes      []id.KramaID    `json:"random_nodes"`
	ReplacedNodes    []id.KramaID    `json:"replaced_nodes"`
}

func (d DeltaGroup) Copy() *DeltaGroup {
	deltaGroup := &DeltaGroup{
		Role: d.Role,
	}

	if len(d.BehaviouralNodes) > 0 {
		deltaGroup.BehaviouralNodes = make([]id.KramaID, len(d.BehaviouralNodes))
		copy(deltaGroup.BehaviouralNodes, d.BehaviouralNodes)
	}

	if len(d.RandomNodes) > 0 {
		deltaGroup.RandomNodes = make([]id.KramaID, len(d.RandomNodes))
		copy(deltaGroup.RandomNodes, d.RandomNodes)
	}

	if len(d.ReplacedNodes) > 0 {
		deltaGroup.ReplacedNodes = make([]id.KramaID, len(d.ReplacedNodes))
		copy(deltaGroup.ReplacedNodes, d.ReplacedNodes)
	}

	return deltaGroup
}

type ContextLockInfo struct {
	ContextHash   Hash   `json:"context_hash"`
	Height        uint64 `json:"height"`
	TesseractHash Hash   `json:"tesseract_hash"`
}

package common

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type Qc struct {
	Type          ConsensusMsgType
	ID            identifiers.Identifier
	LockType      LockType
	View          uint64
	TSHash        Hash
	SignerIndices *ArrayOfBits
	Signature     []byte
}

func (qc *Qc) Copy() *Qc {
	// TODO: Implement deep copy
	return nil
}

type (
	ConsensusMsgType int
	WALMsgType       int
)

const (
	PROPOSAL ConsensusMsgType = iota + 1
	PREVOTE
	PRECOMMIT
)

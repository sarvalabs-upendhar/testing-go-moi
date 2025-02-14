package common

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type Views []*ViewInfo

type ViewInfo struct {
	ID          identifiers.Identifier
	LastView    uint64
	CurrentLock LockType
	Qc          []*Qc
}

func (vs Views) Copy() Views {
	newViews := make(Views, len(vs))
	copy(newViews, vs)

	return newViews
}

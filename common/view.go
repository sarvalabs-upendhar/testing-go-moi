package common

import identifiers "github.com/sarvalabs/go-moi-identifiers"

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

package common

import (
	"fmt"

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

func (vs Views) Print() any {
	fmt.Printf("Views (%d items):\n", len(vs))

	for i, view := range vs {
		if view == nil {
			continue
		}

		fmt.Printf("  [%d]: ID=%s, LastView=%d, CurrentLock=%v, Qc=%d items\n",
			i, view.ID.String(), view.LastView, view.CurrentLock, len(view.Qc))
	}

	return nil
}

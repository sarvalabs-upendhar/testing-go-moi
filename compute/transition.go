package compute

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

type Transition struct {
	objects  state.ObjectMap
	receipts map[common.Hash]*common.Receipt
}

// NewTransition creates a new Transition instance
func NewTransition() *Transition {
	return &Transition{
		objects:  make(state.ObjectMap),
		receipts: make(map[common.Hash]*common.Receipt),
	}
}

func (trans *Transition) Copy() *Transition {
	transition := &Transition{
		objects:  make(state.ObjectMap),
		receipts: make(map[common.Hash]*common.Receipt),
	}

	if len(trans.objects) > 0 {
		for addr, object := range trans.objects {
			transition.objects[addr] = object.Copy()
		}
	}

	if len(trans.receipts) > 0 {
		for hash, receipt := range trans.receipts {
			transition.receipts[hash] = receipt.Copy()
		}
	}

	return transition
}

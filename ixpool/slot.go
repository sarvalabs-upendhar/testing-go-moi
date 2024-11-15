package ixpool

import (
	"log"
	"sync/atomic"

	"github.com/sarvalabs/go-moi/common"
)

const (
	highPressureMark = 80 // 80%
)

// gauge for measuring pool capacity in slots
type slotGauge struct {
	total   uint64 // total no of slots in ixpool
	max     uint64 // max limit
	metrics *Metrics
}

func (g *slotGauge) read() uint64 {
	return atomic.LoadUint64(&g.total)
}

func (g *slotGauge) increase(slots uint64) {
	g.metrics.captureSlotsUsed(float64(atomic.AddUint64(&g.total, slots)))
}

func (g *slotGauge) decrease(slots uint64) {
	g.metrics.captureSlotsUsed(float64(atomic.AddUint64(&g.total, ^(slots - 1))))
}

func (g *slotGauge) highPressure() bool {
	return g.read() > highPressureMark*g.max/100
}

// increaseWithinLimit increases the height of the gauge by the specified slots amount only if the increased height is
// less than max. Returns true if the height is increased.
func (g *slotGauge) increaseWithinLimit(slots uint64) (updated bool) {
	for {
		old := g.read()
		newHeight := old + slots

		if newHeight > g.max {
			return false
		}

		if atomic.CompareAndSwapUint64(&g.total, old, newHeight) {
			return true
		}
	}
}

func slotsRequired(ixns ...*common.Interaction) uint64 {
	var (
		slots = uint64(0)
		size  uint64
		err   error
	)

	for _, ix := range ixns {
		if size, err = ix.Size(); err != nil {
			log.Panic(err)
		}

		slots += (size + IxSlotSize - 1) / IxSlotSize
	}

	return slots
}

package ixpool

import (
	"sort"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
)

type IxBatchRegistry struct {
	ParticipantBatchLookup map[identifiers.Address]int
	batches                []*common.IxBatch
}

func newBatchRegistry() *IxBatchRegistry {
	return &IxBatchRegistry{
		ParticipantBatchLookup: make(map[identifiers.Address]int),
		batches:                make([]*common.IxBatch, 0),
	}
}

func (r *IxBatchRegistry) batchID(addr identifiers.Address) (int, bool) {
	id, ok := r.ParticipantBatchLookup[addr]

	return id, ok
}

func (r *IxBatchRegistry) len() int {
	return len(r.batches)
}

func (r *IxBatchRegistry) appendEmptyBatch() {
	r.batches = append(r.batches, common.NewIxnBatch())
}

func (r *IxBatchRegistry) addIxToBatch(batchID int, ixn *common.Interaction) bool {
	if ok := r.batches[batchID].Add(ixn); !ok {
		return false
	}

	for addr, info := range ixn.Participants() {
		if info.IsGenesis {
			continue
		}

		r.ParticipantBatchLookup[addr] = batchID
	}

	return true
}

func (r *IxBatchRegistry) sort() {
	sort.Slice(r.batches, func(i, j int) bool {
		if r.batches[i].IxCount() == r.batches[j].IxCount() {
			return r.batches[i].PsCount() < r.batches[j].PsCount()
		}

		return r.batches[i].IxCount() > r.batches[j].IxCount()
	})
}

func (r *IxBatchRegistry) addIx(ixn *common.Interaction) bool {
	batchID := r.findBatchID(ixn.Participants())

	switch {
	case batchID == conflictBatchID:
		return false

	case batchID == BatchIDNotFound:
		r.appendEmptyBatch()

		return r.addIxToBatch(r.len()-1, ixn)

	case batchID >= 0:
		return r.addIxToBatch(batchID, ixn)

	default:
		return false
	}
}

func (r *IxBatchRegistry) selectOptimalBatches() []*common.IxBatch {
	r.sort()

	batches := make([]*common.IxBatch, 0)
	mergeCount := 0

	for batchIdx := 0; batchIdx < len(r.batches); batchIdx++ {
		if r.batches[batchIdx].PsCount() == 2 {
			merged := false

			for nextBatchIdx := batchIdx + 1; nextBatchIdx < len(r.batches); nextBatchIdx++ {
				if r.batches[nextBatchIdx].PsCount() == 2 {
					for _, ix := range r.batches[nextBatchIdx].IxList() {
						r.batches[batchIdx].Add(ix)
					}

					r.batches[nextBatchIdx].Flush()

					mergeCount++
					merged = true

					break
				}
			}

			if !merged || mergeCount == maxBatches {
				break
			}
		}
	}

	if mergeCount != 0 {
		r.sort()
	}

	for _, batch := range r.batches {
		if batch.IxCount() > 0 {
			batches = append(batches, batch)

			if len(batches) == maxBatches {
				break
			}
		}
	}

	return batches
}

func (r *IxBatchRegistry) findBatchID(ps map[identifiers.Address]*common.ParticipantInfo) int {
	batchID := BatchIDNotFound

	for addr := range ps {
		if psBatchID, exists := r.batchID(addr); exists {
			batchID = psBatchID

			for addr := range ps {
				if nextPsBatchID, exists := r.batchID(addr); exists &&
					psBatchID != nextPsBatchID {
					return conflictBatchID
				}
			}

			break
		}
	}

	return batchID
}

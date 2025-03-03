package ixpool

import (
	"sort"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
)

type IxBatchRegistry struct {
	participantBatchLookup map[identifiers.Identifier]int
	batches                []*common.IxBatch
	consensusNodesHash     map[identifiers.Identifier]common.Hash
}

func newBatchRegistry() *IxBatchRegistry {
	return &IxBatchRegistry{
		participantBatchLookup: make(map[identifiers.Identifier]int),
		batches:                make([]*common.IxBatch, 0),
		consensusNodesHash:     make(map[identifiers.Identifier]common.Hash),
	}
}

func (r *IxBatchRegistry) addConsensusNodesHash(id identifiers.Identifier, hash common.Hash) {
	r.consensusNodesHash[id] = hash
}

func (r *IxBatchRegistry) batchID(id identifiers.Identifier) (int, bool) {
	batchID, ok := r.participantBatchLookup[id]

	return batchID, ok
}

func (r *IxBatchRegistry) len() int {
	return len(r.batches)
}

func (r *IxBatchRegistry) appendEmptyBatch() {
	r.batches = append(r.batches, common.NewIxnBatch())
}

func (r *IxBatchRegistry) addIxToBatch(batchID int, ixn *common.Interaction) bool {
	if ok := r.batches[batchID].Add(ixn, r.consensusNodesHash); !ok {
		return false
	}

	for id, info := range ixn.Participants() {
		if info.IsGenesis {
			continue
		}

		r.participantBatchLookup[id] = batchID
	}

	return true
}

func (r *IxBatchRegistry) sort() {
	sort.Slice(r.batches, func(i, j int) bool {
		if r.batches[i].IxCount() == r.batches[j].IxCount() {
			return r.batches[i].ConsensusNodesHashCount() < r.batches[j].ConsensusNodesHashCount()
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

func (r *IxBatchRegistry) mergeBatches(consensusNodesHashCount int) bool {
	mergeCount := 0

	for batchIdx := 0; batchIdx < len(r.batches); batchIdx++ {
		if r.batches[batchIdx].ConsensusNodesHashCount() == consensusNodesHashCount {
			merged := false

			for nextBatchIdx := batchIdx + 1; nextBatchIdx < len(r.batches); nextBatchIdx++ {
				if r.batches[nextBatchIdx].ConsensusNodesHashCount() == consensusNodesHashCount {
					for _, ix := range r.batches[nextBatchIdx].IxList() {
						r.batches[batchIdx].Add(ix, r.consensusNodesHash)
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

	return mergeCount != 0
}

// selectOptimalBatches sorts the batches based on number of ixns and consensusNodesHash count per batch.
// It clubs the batches which has consensus nodes hash count pairs (1,1), (2,2)
// If there is a merge, it sorts the batches again as ixn count might have increased in few batches
// Finally, it returns twenty non-empty batches.
func (r *IxBatchRegistry) selectOptimalBatches() []*common.IxBatch {
	r.sort()

	batches := make([]*common.IxBatch, 0)

	merge1 := r.mergeBatches(1) // Merging batches with context hashes of size 1
	merge2 := r.mergeBatches(2) // Merging batches with context hashes of size 2

	if merge1 || merge2 {
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

func (r *IxBatchRegistry) findBatchID(ps map[identifiers.Identifier]*common.ParticipantInfo) int {
	batchID := BatchIDNotFound

	for id := range ps {
		if psBatchID, exists := r.batchID(id); exists {
			batchID = psBatchID

			for id := range ps {
				if nextPsBatchID, exists := r.batchID(id); exists &&
					psBatchID != nextPsBatchID {
					return conflictBatchID
				}
			}

			break
		}
	}

	return batchID
}

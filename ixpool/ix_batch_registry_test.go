package ixpool

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestIxBatchRegistry_addIxToBatch(t *testing.T) {
	ids := tests.GetIdentifiers(t, 2)

	ixns := tests.CreateIxns(t, 3, map[int]*tests.CreateIxParams{
		1: {
			IxDataCallback: func(ix *common.IxData) {
				ix.Participants = append(ix.Participants, []common.IxParticipant{
					{
						ID: ids[0],
					},
					{
						ID: ids[1],
					},
				}...)
			},
		},
		2: {
			IxDataCallback: func(ix *common.IxData) {
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				}
			},
		},
	})

	consensusNodesHash := make(map[identifiers.Identifier]common.Hash)

	for _, ix := range ixns {
		for id := range ix.Participants() {
			consensusNodesHash[id] = tests.RandomHash(t)
		}
	}

	testcases := []struct {
		name                            string
		batchID                         int
		ix                              *common.Interaction
		preTestFn                       func(batch *IxBatchRegistry)
		expectedAdd                     bool
		expectedIxCount                 int
		expectedConsensusNodesHashCount int
	}{
		{
			name:                            "add ixn to batch successfully",
			batchID:                         0,
			ix:                              ixns[0],
			expectedAdd:                     true,
			expectedIxCount:                 1,
			expectedConsensusNodesHashCount: 2,
		},
		{
			name:    "failed to add ixn to batch",
			batchID: 0,
			ix:      ixns[0],
			preTestFn: func(batch *IxBatchRegistry) {
				for i := 0; i < 101; i++ {
					require.True(t, batch.addIxToBatch(0, ixns[0]))
				}
			},
			expectedAdd:                     false,
			expectedIxCount:                 101,
			expectedConsensusNodesHashCount: 2,
		},
		{
			name:                            "add ixn where one of the participant is genesis account",
			ix:                              ixns[2],
			expectedAdd:                     true,
			expectedIxCount:                 1,
			expectedConsensusNodesHashCount: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			batchRegistry := newBatchRegistry()
			batchRegistry.appendEmptyBatch()
			batchRegistry.consensusNodesHash = consensusNodesHash

			if testcase.preTestFn != nil {
				testcase.preTestFn(batchRegistry)
			}

			added := batchRegistry.addIxToBatch(testcase.batchID, testcase.ix)
			require.Equal(t, testcase.expectedAdd, added)

			require.Equal(t, testcase.expectedIxCount, batchRegistry.batches[testcase.batchID].IxCount())
			require.Equal(t, testcase.expectedConsensusNodesHashCount,
				batchRegistry.batches[testcase.batchID].ConsensusNodesHashCount())

			if testcase.expectedAdd {
				ixns := batchRegistry.batches[testcase.batchID].IxList()
				require.Equal(t, testcase.ix, ixns[len(ixns)-1])

				psBatchLookup := batchRegistry.participantBatchLookup
				ps := testcase.ix.Participants()

				for id, p := range ps {
					id, found := psBatchLookup[id]
					if p.IsGenesis {
						require.False(t, found)
					} else {
						require.True(t, found)
						require.Equal(t, testcase.batchID, id)
					}
				}
			}
		})
	}
}

func TestIxBatchRegistry_findBatchID(t *testing.T) {
	ixns := tests.CreateIxns(t, 2, map[int]*tests.CreateIxParams{
		1: {
			IxDataCallback: func(ix *common.IxData) {
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				}
			},
		},
	})

	testcases := []struct {
		name            string
		ps              map[identifiers.Identifier]*common.ParticipantInfo
		preTestFn       func(batch *IxBatchRegistry)
		expectedBatchID int
	}{
		{
			name: "found batch id successfully",
			ps:   ixns[1].Participants(),
			preTestFn: func(batch *IxBatchRegistry) {
				batch.appendEmptyBatch()
				require.True(t, batch.addIxToBatch(0, ixns[0]))
				batch.appendEmptyBatch()
				require.True(t, batch.addIxToBatch(1, ixns[1]))
			},
			expectedBatchID: 1,
		},
		{
			name: "conflicting batch id's among participants",
			ps: map[identifiers.Identifier]*common.ParticipantInfo{
				ixns[0].SenderID(): {ID: ixns[0].SenderID()},

				ixns[1].SenderID(): {ID: ixns[1].SenderID()},
			},
			preTestFn: func(batch *IxBatchRegistry) {
				batch.appendEmptyBatch()
				require.True(t, batch.addIxToBatch(0, ixns[0]))
				batch.appendEmptyBatch()
				require.True(t, batch.addIxToBatch(1, ixns[1]))
			},
			expectedBatchID: conflictBatchID,
		},
		{
			name: "batch id not found",
			ps:   ixns[1].Participants(),
			preTestFn: func(batch *IxBatchRegistry) {
				batch.appendEmptyBatch()
				require.True(t, batch.addIxToBatch(0, ixns[0]))
			},
			expectedBatchID: BatchIDNotFound,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			batchRegistry := newBatchRegistry()

			if testcase.preTestFn != nil {
				testcase.preTestFn(batchRegistry)
			}

			id := batchRegistry.findBatchID(testcase.ps)
			require.Equal(t, testcase.expectedBatchID, id)
		})
	}
}

func TestIxBatchRegistry_addIx(t *testing.T) {
	ids := tests.GetIdentifiers(t, 2)

	ixns := tests.CreateIxns(t, 3, map[int]*tests.CreateIxParams{
		0: {
			IxDataCallback: func(ix *common.IxData) {
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetTransfer,
						Payload: tests.CreateRawAssetActionPayload(t, tests.RandomIdentifier(t)),
					},
				}
				ix.Participants = append(ix.Participants, []common.IxParticipant{
					{
						ID: ids[0],
					},
				}...)
			},
		},
		1: {
			IxDataCallback: func(ix *common.IxData) {
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				}
				ix.Participants = append(ix.Participants, []common.IxParticipant{
					{
						ID: ids[1],
					},
				}...)
			},
		},
		2: {
			IxDataCallback: func(ix *common.IxData) {
				ix.IxOps = []common.IxOpRaw{
					{
						Type:    common.IxAssetCreate,
						Payload: tests.CreateRawAssetCreatePayload(t),
					},
				}
				ix.Participants = append(ix.Participants, []common.IxParticipant{
					{
						ID: ids[0],
					},
					{
						ID: ids[1],
					},
				}...)
			},
		},
	})

	testcases := []struct {
		name            string
		ix              *common.Interaction
		preTestFn       func(batch *IxBatchRegistry)
		expectedBatchID int
		expectedAdd     bool
	}{
		{
			name:            "add ixn to new batch",
			ix:              ixns[0],
			expectedBatchID: 0,
			expectedAdd:     true,
		},
		{
			name: "add ixn to existing batch",
			ix:   ixns[0],
			preTestFn: func(batch *IxBatchRegistry) {
				batch.appendEmptyBatch()
				batch.addIxToBatch(0, ixns[0])
			},
			expectedAdd: true,
		},
		{
			name: "failed to add ixn due to conflicting batch id",
			ix:   ixns[2],
			preTestFn: func(batch *IxBatchRegistry) {
				batch.appendEmptyBatch()
				batch.addIxToBatch(0, ixns[0])
				batch.appendEmptyBatch()
				batch.addIxToBatch(1, ixns[1])
			},
			expectedAdd: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			batchRegistry := newBatchRegistry()

			if testcase.preTestFn != nil {
				testcase.preTestFn(batchRegistry)
			}

			added := batchRegistry.addIx(testcase.ix)
			require.Equal(t, testcase.expectedAdd, added)

			if testcase.expectedAdd {
				ixns := batchRegistry.batches[testcase.expectedBatchID].IxList()
				require.Equal(t, testcase.ix, ixns[len(ixns)-1])
			}
		})
	}
}

func TestIxBatchRegistry_selectOptimalBatches(t *testing.T) {
	testcases := []struct {
		name               string
		batchesList        []CreateBatches
		expectedBatchList  []CreateBatches
		expectedBatchCount int
	}{
		{
			name: "select batches after merging",
			batchesList: []CreateBatches{
				{
					batchCount: 6,
					batch: CreateBatch{
						ixnCount:                1,
						consensusNodesHashCount: 2,
					},
				},
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 3,
					batch: CreateBatch{
						ixnCount:                2,
						consensusNodesHashCount: 4,
					},
				},
			},
			expectedBatchCount: 3,
		},
		{
			name: "select max no of batches",
			batchesList: []CreateBatches{
				{
					batchCount: 46,
					batch: CreateBatch{
						ixnCount:                1,
						consensusNodesHashCount: 2,
					},
				},
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 20,
					batch: CreateBatch{
						ixnCount:                2,
						consensusNodesHashCount: 4,
					},
				},
			},
			expectedBatchCount: 20,
		},
		{
			name: "select batches having max no of ixns",
			batchesList: []CreateBatches{
				{
					batchCount: 10,
					batch: CreateBatch{
						ixnCount:                1,
						consensusNodesHashCount: 3,
					},
				},
				{
					batchCount: 10,
					batch: CreateBatch{
						ixnCount:                2,
						consensusNodesHashCount: 4,
					},
				},
				{
					batchCount: 10,
					batch: CreateBatch{
						ixnCount:                1,
						consensusNodesHashCount: 3,
					},
				},
				{
					batchCount: 10,
					batch: CreateBatch{
						ixnCount:                2,
						consensusNodesHashCount: 4,
					},
				},
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 20,
					batch: CreateBatch{
						ixnCount:                2,
						consensusNodesHashCount: 4,
					},
				},
			},
			expectedBatchCount: 20,
		},
		{
			name: "select batches in sorted fashion after merging",
			batchesList: []CreateBatches{
				{
					batchCount: 4,
					batch: CreateBatch{
						ixnCount:                3,
						consensusNodesHashCount: 2,
					},
				},
				{
					batchCount: 19,
					batch: CreateBatch{
						ixnCount:                4,
						consensusNodesHashCount: 3,
					},
				},
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 2,
					batch: CreateBatch{
						ixnCount:                6,
						consensusNodesHashCount: 4,
					},
				},
				{
					batchCount: 18,
					batch: CreateBatch{
						ixnCount:                4,
						consensusNodesHashCount: 3,
					},
				},
			},
			expectedBatchCount: 20,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			batchRegistry := newBatchRegistry()

			addBatches(t, batchRegistry, testcase.batchesList)

			result := batchRegistry.selectOptimalBatches()
			require.Equal(t, testcase.expectedBatchCount, len(result))

			index := 0

			for _, batches := range testcase.expectedBatchList {
				for i := 0; i < batches.batchCount; i++ {
					require.Equal(t, batches.batch.consensusNodesHashCount, result[index].ConsensusNodesHashCount())
					require.Equal(t, batches.batch.ixnCount, result[index].IxCount())

					index++
				}
			}
		})
	}
}

func TestIxBatchRegistry_sort(t *testing.T) {
	testcases := []struct {
		name               string
		batchesList        []CreateBatches
		expectedBatchList  []CreateBatches
		expectedBatchCount int
	}{
		{
			name: "ensure batches are sorted by ixn count and participant count",
			batchesList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                4,
						consensusNodesHashCount: 2,
					},
				},
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                3,
						consensusNodesHashCount: 4,
					},
				},
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                3,
						consensusNodesHashCount: 2,
					},
				},
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                6,
						consensusNodesHashCount: 4,
					},
				},
			},
			expectedBatchList: []CreateBatches{
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                6,
						consensusNodesHashCount: 4,
					},
				},
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                4,
						consensusNodesHashCount: 2,
					},
				},
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                3,
						consensusNodesHashCount: 2,
					},
				},
				{
					batchCount: 1,
					batch: CreateBatch{
						ixnCount:                3,
						consensusNodesHashCount: 4,
					},
				},
			},
			expectedBatchCount: 20,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			batchRegistry := newBatchRegistry()

			addBatches(t, batchRegistry, testcase.batchesList)

			batchRegistry.sort()

			index := 0

			for _, batches := range testcase.expectedBatchList {
				for i := 0; i < batches.batchCount; i++ {
					require.Equal(t, batches.batch.consensusNodesHashCount, batchRegistry.batches[index].ConsensusNodesHashCount())
					require.Equal(t, batches.batch.ixnCount, batchRegistry.batches[index].IxCount())

					index++
				}
			}
		})
	}
}

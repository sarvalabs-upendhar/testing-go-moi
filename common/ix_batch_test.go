package common_test

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestIxBatch_Add(t *testing.T) {
	consensusNodesHash := make(map[identifiers.Identifier]common.Hash)

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

	// generate random consensus nodes hash for each identifier
	for _, ixn := range ixns {
		for id := range ixn.Participants() {
			consensusNodesHash[id] = tests.RandomHash(t)
		}
	}

	testcases := []struct {
		name                            string
		ix                              *common.Interaction
		preTestFn                       func(batch *common.IxBatch)
		expectedAdd                     bool
		expectedIxCount                 int
		expectedConsensusNodesHashCount int
	}{
		{
			name:                            "add ixn to batch successfully",
			ix:                              ixns[0],
			expectedAdd:                     true,
			expectedIxCount:                 1,
			expectedConsensusNodesHashCount: 2,
		},
		{
			name: "add ixn with same participants mutiple times successfully",
			preTestFn: func(batch *common.IxBatch) {
				require.True(t, batch.Add(ixns[0], consensusNodesHash))
				require.True(t, batch.Add(ixns[0], consensusNodesHash))
			},
			ix:                              ixns[0],
			expectedAdd:                     true,
			expectedIxCount:                 3,
			expectedConsensusNodesHashCount: 2,
		},
		{
			name:                            "add ixn where participant is genesis account",
			ix:                              ixns[1],
			expectedAdd:                     true,
			expectedIxCount:                 1,
			expectedConsensusNodesHashCount: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixBatch := common.NewIxnBatch()

			if testcase.preTestFn != nil {
				testcase.preTestFn(ixBatch)
			}

			added := ixBatch.Add(testcase.ix, consensusNodesHash)
			require.Equal(t, testcase.expectedAdd, added)
			require.Equal(t, testcase.expectedIxCount, ixBatch.IxCount())
			require.Equal(t, testcase.expectedConsensusNodesHashCount, ixBatch.ConsensusNodesHashCount())

			if testcase.expectedAdd {
				ixns := ixBatch.IxList()
				require.Equal(t, testcase.ix, ixns[len(ixns)-1])

				ps := testcase.ix.Participants()
				consensusNodesHashes := ixBatch.ConsensusNodesHash()

				for addr, p := range ps {
					_, ok := consensusNodesHashes[consensusNodesHash[addr]]

					if p.IsGenesis {
						require.False(t, ok)
					} else {
						require.True(t, ok)
					}
				}
			}
		})
	}
}

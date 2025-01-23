package common_test

import (
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestIxBatch_Add(t *testing.T) {
	ids := tests.GetIdentifiers(t, 2)

	ixns := tests.CreateIxns(t, 4, map[int]*tests.CreateIxParams{
		2: {
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
		3: {
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
		ix              *common.Interaction
		preTestFn       func(batch *common.IxBatch)
		expectedAdd     bool
		expectedIxCount int
		expectedPsCount int
	}{
		{
			name:            "add ixn to batch successfully",
			ix:              ixns[0],
			expectedAdd:     true,
			expectedIxCount: 1,
			expectedPsCount: 2,
		},
		{
			name: "add ixn with same participants mutiple times successfully",
			preTestFn: func(batch *common.IxBatch) {
				require.True(t, batch.Add(ixns[0]))
				require.True(t, batch.Add(ixns[0]))
			},
			ix:              ixns[0],
			expectedAdd:     true,
			expectedIxCount: 3,
			expectedPsCount: 2,
		},
		{
			name: "add two ixns with partial overlap of participants successfully",
			preTestFn: func(batch *common.IxBatch) {
				require.True(t, batch.Add(ixns[1]))
			},
			ix:              ixns[0],
			expectedAdd:     true,
			expectedIxCount: 2,
			expectedPsCount: 3,
		},
		{
			name:            "add ixn where participant is genesis account",
			ix:              ixns[3],
			expectedAdd:     true,
			expectedIxCount: 1,
			expectedPsCount: 2,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixBatch := common.NewIxnBatch()

			if testcase.preTestFn != nil {
				testcase.preTestFn(ixBatch)
			}

			added := ixBatch.Add(testcase.ix)
			require.Equal(t, testcase.expectedAdd, added)
			require.Equal(t, testcase.expectedIxCount, ixBatch.IxCount())
			require.Equal(t, testcase.expectedPsCount, ixBatch.PsCount())

			if testcase.expectedAdd {
				ixns := ixBatch.IxList()
				require.Equal(t, testcase.ix, ixns[len(ixns)-1])

				ps := testcase.ix.Participants()
				psInBatch := ixBatch.Ps()

				for addr, p := range ps {
					_, ok := psInBatch[addr]

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

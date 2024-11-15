package common_test

import (
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestIxBatch_uniqueAccounts(t *testing.T) {
	addresses := tests.GetAddresses(t, 3)
	ix := tests.CreateIX(t, nil)

	testcases := []struct {
		name            string
		preTestFn       func(batch *common.IxBatch)
		ps              map[identifiers.Address]*common.ParticipantInfo
		expectedPsCount int
	}{
		{
			name: "find unique accounts on empty batch",
			ps: map[identifiers.Address]*common.ParticipantInfo{
				addresses[0]: {},
				addresses[1]: {},
				addresses[2]: {
					IsGenesis: true,
				},
			},
			expectedPsCount: 2,
		},
		{
			name: "find unique accounts on batch with accounts",
			preTestFn: func(batch *common.IxBatch) {
				require.True(t, batch.Add(ix))
			},
			ps: map[identifiers.Address]*common.ParticipantInfo{
				ix.Sender():  {},
				addresses[1]: {},
				addresses[2]: {
					IsGenesis: true,
				},
			},
			expectedPsCount: 3,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			ixBatch := common.NewIxnBatch()

			if testcase.preTestFn != nil {
				testcase.preTestFn(ixBatch)
			}

			count := ixBatch.UniqueAccounts(testcase.ps)
			require.Equal(t, testcase.expectedPsCount, count)
		})
	}
}

func TestIxBatch_Add(t *testing.T) {
	addresses := tests.GetAddresses(t, 2)

	ixns := tests.CreateIxns(t, 4, map[int]*tests.CreateIxParams{
		2: {
			IxDataCallback: func(ix *common.IxData) {
				ix.Participants = append(ix.Participants, []common.IxParticipant{
					{
						Address: addresses[0],
					},
					{
						Address: addresses[1],
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
			name: "failed to add ixn due to unique accounts overflow",
			preTestFn: func(batch *common.IxBatch) {
				require.True(t, batch.Add(ixns[0]))
			},
			ix:              ixns[2],
			expectedIxCount: 1,
			expectedPsCount: 2,
			expectedAdd:     false,
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

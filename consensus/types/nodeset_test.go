package types

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestGetVoteSet(t *testing.T) {
	nodeset := NewICSCommittee()
	keys := make([][]byte, 2)
	nodeset.AppendNodeSet(tests.RandomHash(t), NewNodeSet(tests.RandomValidatorsInfo(t, keys), 0))
	nodeset.AppendNodeSet(tests.RandomHash(t), NewNodeSet(tests.RandomValidatorsInfo(t, keys), 0))

	nodeset.Sets[0].Responses[VoteCounter].Bits.SetIndex(0, true)
	nodeset.Sets[0].Responses[VoteCounter].Bits.SetIndex(1, false)
	nodeset.Sets[1].Responses[VoteCounter].Bits.SetIndex(0, false)
	nodeset.Sets[1].Responses[VoteCounter].Bits.SetIndex(1, true)

	testcases := []struct {
		name    string
		nodeset *ICSCommittee
	}{
		{
			name:    "fetch combined voteset",
			nodeset: nodeset,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result := test.nodeset.GetVoteset(VoteCounter)

			require.Equal(t, true, result.GetIndex(0))
			require.Equal(t, false, result.GetIndex(1))
			require.Equal(t, false, result.GetIndex(2))
			require.Equal(t, true, result.GetIndex(3))
		})
	}
}

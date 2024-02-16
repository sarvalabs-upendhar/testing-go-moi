package common_test

import (
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestGetVoteSet(t *testing.T) {
	nodeset := common.ICSNodeSet{
		Nodes: []*common.NodeSet{
			{
				Ids:       tests.RandomKramaIDs(t, 2),
				Responses: common.NewArrayOfBits(2),
			},
			{
				Ids:       tests.RandomKramaIDs(t, 2),
				Responses: common.NewArrayOfBits(2),
			},
		},
		Size: 4,
	}

	nodeset.Nodes[0].Responses.SetIndex(0, true)
	nodeset.Nodes[0].Responses.SetIndex(1, false)
	nodeset.Nodes[1].Responses.SetIndex(0, false)
	nodeset.Nodes[1].Responses.SetIndex(1, true)

	testcases := []struct {
		name    string
		nodeset common.ICSNodeSet
	}{
		{
			name:    "fetch combined voteset",
			nodeset: nodeset,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result := test.nodeset.GetVoteset()

			require.Equal(t, true, result.GetIndex(0))
			require.Equal(t, false, result.GetIndex(1))
			require.Equal(t, false, result.GetIndex(2))
			require.Equal(t, true, result.GetIndex(3))
		})
	}
}

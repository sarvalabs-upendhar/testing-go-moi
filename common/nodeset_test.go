package common_test

import (
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestGetVoteSet(t *testing.T) {
	nodeset := common.NewICSNodeSet(2)
	nodeset.UpdateNodeSet(0, common.NewNodeSet(tests.RandomKramaIDs(t, 2), nil, 0))
	nodeset.UpdateNodeSet(1, common.NewNodeSet(tests.RandomKramaIDs(t, 2), nil, 0))

	nodeset.Sets[0].Responses.SetIndex(0, true)
	nodeset.Sets[0].Responses.SetIndex(1, false)
	nodeset.Sets[1].Responses.SetIndex(0, false)
	nodeset.Sets[1].Responses.SetIndex(1, true)

	testcases := []struct {
		name    string
		nodeset *common.ICSNodeSet
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

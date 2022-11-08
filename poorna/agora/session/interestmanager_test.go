package session

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/tests"
	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/moichain/types"
)

func TestRecordSessionInterest_MultipleAddress(t *testing.T) {
	im := NewInterestManager()

	address1 := tests.RandomAddress(t)
	address2 := tests.RandomAddress(t)

	set, _ := GetDummyBlocks(t, 20)
	keys := set.Keys()
	// record hashes with address1
	im.RecordSessionInterest(address1, keys...)

	// record same hashes with address2
	im.RecordSessionInterest(address2, keys...)

	for _, hash := range keys {
		addresses, ok := im.wants[hash]

		// hash should be recorded
		require.True(t, ok, "hash not recorded")

		// both addresses should be recorded
		require.True(t, addresses[address1], "address1 is missing")
		require.True(t, addresses[address2], "address2 is missing")
	}
}

func TestRemoveSession_SingleAddress(t *testing.T) {
	im := NewInterestManager()

	address := tests.RandomAddress(t)

	set, _ := GetDummyBlocks(t, 20)
	keys := set.Keys()
	// record hashes with address1
	im.RecordSessionInterest(address, keys...)

	im.RemoveSession(address)

	for _, hash := range keys {
		_, ok := im.wants[hash]

		// hash should not be available
		require.False(t, ok, "hash still available")
	}
}

func TestRemoveSession_MultipleAddress(t *testing.T) {
	im := NewInterestManager()

	address1 := tests.RandomAddress(t)
	address2 := tests.RandomAddress(t)

	set, _ := GetDummyBlocks(t, 20)
	keys := set.Keys()

	// record hashes with address1
	im.RecordSessionInterest(address1, keys...)
	// record same hashes with address2
	im.RecordSessionInterest(address2, keys...)

	// remove keys associated with address1
	im.RemoveSession(address1)

	for _, hash := range keys {
		addresses, ok := im.wants[hash]
		// hash should be available
		require.True(t, ok, "hash not available")
		// address2 should be available
		require.True(t, addresses[address2])
	}
}

func TestInterestedSessions(t *testing.T) {
	im := NewInterestManager()

	address1 := tests.RandomAddress(t)

	set1, blocks1 := GetDummyBlocks(t, 20)

	set2, blocks2 := GetDummyBlocks(t, 5)

	// record set keys only
	im.RecordSessionInterest(address1, set1.Keys()...)

	interestedSessions, orphanBlocks := im.InterestedSessions(appendBlocks(blocks1, blocks2))

	// All set2 blocks are orphans
	for _, block := range orphanBlocks {
		require.True(t, set2.Has(block.GetID()), "invalid orphan block")
	}

	// Verify that all set1 blocks are returned
	blocks, ok := interestedSessions[address1]
	require.True(t, ok, "set1 blocks missing")

	for _, block := range blocks {
		require.True(t, set1.Has(block.GetID()), "few blocks missing")
	}
}

func appendBlocks(set1, set2 map[types.Hash]atypes.Block) []atypes.Block {
	blocks := make([]atypes.Block, 0, len(set1)+len(set2))

	for _, v := range set1 {
		blocks = append(blocks, v)
	}

	for _, v := range set2 {
		blocks = append(blocks, v)
	}

	return blocks
}

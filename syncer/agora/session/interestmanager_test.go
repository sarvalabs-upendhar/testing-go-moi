package session

import (
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
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

	removeSession(im, address)

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
	removeSession(im, address1)

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
		require.True(t, set2.Has(block.GetCid()), "invalid orphan block")
	}

	// Verify that all set1 blocks are returned
	blocks, ok := interestedSessions[address1]
	require.True(t, ok, "set1 blocks missing")

	for _, block := range blocks {
		require.True(t, set1.Has(block.GetCid()), "few blocks missing")
	}
}

func TestDeleteSession(t *testing.T) {
	cid1 := block.NewBlockFromRawData(0x00, []byte{1}).GetCid()
	addr1 := tests.RandomAddress(t)
	addr2 := tests.RandomAddress(t)

	testcases := []struct {
		name                string
		cid                 cid.CID
		wants               map[cid.CID]map[common.Address]bool
		addr                common.Address
		deletedKeys         []cid.CID
		expectedDeletedKeys []cid.CID
		expectedWantsLength int
	}{
		{
			name: "delete the last session that wants key",
			cid:  cid1,
			wants: map[cid.CID]map[common.Address]bool{
				cid1: {
					addr1: true,
				},
			},
			addr:                addr1,
			deletedKeys:         []cid.CID{},
			expectedDeletedKeys: []cid.CID{cid1},
			expectedWantsLength: 0,
		},
		{
			name: "delete the session that wants key",
			cid:  cid1,
			wants: map[cid.CID]map[common.Address]bool{
				cid1: {
					addr1: true,
					addr2: true,
				},
			},
			addr:                addr1,
			deletedKeys:         []cid.CID{},
			expectedDeletedKeys: []cid.CID{},
			expectedWantsLength: 1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			deleteSession(test.cid, test.wants, test.addr, &test.deletedKeys)
			require.Equal(t, test.expectedDeletedKeys, test.deletedKeys)
			require.Equal(t, test.expectedWantsLength, len(test.wants))
		})
	}
}

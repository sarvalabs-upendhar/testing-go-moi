package session

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/syncer/agora/block"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

func TestRecordSessionInterest_MultipleAddress(t *testing.T) {
	im := NewInterestManager()

	id1 := tests.RandomIdentifier(t)
	id2 := tests.RandomIdentifier(t)

	set, _ := GetDummyBlocks(t, 20)
	keys := set.Keys()
	// record hashes with id1
	im.RecordSessionInterest(id1, keys...)

	// record same hashes with id2
	im.RecordSessionInterest(id2, keys...)

	for _, hash := range keys {
		ids, ok := im.wants[hash]

		// hash should be recorded
		require.True(t, ok, "hash not recorded")

		// both ids should be recorded
		require.True(t, ids[id1], "id1 is missing")
		require.True(t, ids[id2], "id2 is missing")
	}
}

func TestRemoveSession_SingleAddress(t *testing.T) {
	im := NewInterestManager()

	id := tests.RandomIdentifier(t)

	set, _ := GetDummyBlocks(t, 20)
	keys := set.Keys()
	// record hashes with id1
	im.RecordSessionInterest(id, keys...)

	removeSession(im, id)

	for _, hash := range keys {
		_, ok := im.wants[hash]

		// hash should not be available
		require.False(t, ok, "hash still available")
	}
}

func TestRemoveSession_MultipleAddress(t *testing.T) {
	im := NewInterestManager()

	id1 := tests.RandomIdentifier(t)
	id2 := tests.RandomIdentifier(t)

	set, _ := GetDummyBlocks(t, 20)
	keys := set.Keys()

	// record hashes with id1
	im.RecordSessionInterest(id1, keys...)
	// record same hashes with id2
	im.RecordSessionInterest(id2, keys...)

	// remove keys associated with id1
	removeSession(im, id1)

	for _, hash := range keys {
		addresses, ok := im.wants[hash]
		// hash should be available
		require.True(t, ok, "hash not available")
		// id2 should be available
		require.True(t, addresses[id2])
	}
}

func TestInterestedSessions(t *testing.T) {
	im := NewInterestManager()

	id1 := tests.RandomIdentifier(t)

	set1, blocks1 := GetDummyBlocks(t, 20)

	set2, blocks2 := GetDummyBlocks(t, 5)

	// record set keys only
	im.RecordSessionInterest(id1, set1.Keys()...)

	interestedSessions, orphanBlocks := im.InterestedSessions(appendBlocks(blocks1, blocks2))

	// All set2 blocks are orphans
	for _, block := range orphanBlocks {
		require.True(t, set2.Has(block.GetCid()), "invalid orphan block")
	}

	// Verify that all set1 blocks are returned
	blocks, ok := interestedSessions[id1]
	require.True(t, ok, "set1 blocks missing")

	for _, block := range blocks {
		require.True(t, set1.Has(block.GetCid()), "few blocks missing")
	}
}

func TestDeleteSession(t *testing.T) {
	cid1 := block.NewAccountBlockFromRawData(0x00, []byte{1}).GetCid()
	id1 := tests.RandomIdentifier(t)
	id2 := tests.RandomIdentifier(t)

	testcases := []struct {
		name                string
		cid                 cid.CID
		wants               map[cid.CID]map[identifiers.Identifier]bool
		id                  identifiers.Identifier
		deletedKeys         []cid.CID
		expectedDeletedKeys []cid.CID
		expectedWantsLength int
	}{
		{
			name: "delete the last session that wants key",
			cid:  cid1,
			wants: map[cid.CID]map[identifiers.Identifier]bool{
				cid1: {
					id1: true,
				},
			},
			id:                  id1,
			deletedKeys:         []cid.CID{},
			expectedDeletedKeys: []cid.CID{cid1},
			expectedWantsLength: 0,
		},
		{
			name: "delete the session that wants key",
			cid:  cid1,
			wants: map[cid.CID]map[identifiers.Identifier]bool{
				cid1: {
					id1: true,
					id2: true,
				},
			},
			id:                  id1,
			deletedKeys:         []cid.CID{},
			expectedDeletedKeys: []cid.CID{},
			expectedWantsLength: 1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			deleteSession(test.cid, test.wants, test.id, &test.deletedKeys)
			require.Equal(t, test.expectedDeletedKeys, test.deletedKeys)
			require.Equal(t, test.expectedWantsLength, len(test.wants))
		})
	}
}

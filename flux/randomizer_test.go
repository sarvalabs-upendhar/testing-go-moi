package flux

import (
	"context"
	"testing"
	"time"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestRandomizer_addPeers(t *testing.T) {
	peerIDs := tests.GetTestPeerIDs(t, 1)
	kramaIDs := tests.GetTestKramaIDs(t, 15)
	entries := getTestEntries(t, kramaIDs)

	randomizerParams := &createRandomizerParams{
		serverCallback: func(server *MockServer) {
			server.setBootstrapPeerIDs(peerIDs)
		},
		repEngineCallback: func(reputationEngine *MockReputationEngine) {
			reputationEngine.setEntries(t, entries)
			reputationEngine.setPeerCount(uint64(len(kramaIDs)))
		},
	}

	randomizer := createTestRandomizer(t, randomizerParams)

	randomizer.addPeers(0)
	require.Equal(t, 20-len(kramaIDs), randomizer.peers[0].pendingCount)
	require.Equal(t, len(kramaIDs), len(randomizer.peers[0].nonUtilized))
	require.Equal(t, false, randomizer.peers[0].updatePending)
}

func TestRandomizer_getPeers(t *testing.T) {
	peerIDs := tests.GetTestPeerIDs(t, 1)
	kramaIDs := tests.GetTestKramaIDs(t, 20)
	entries := getTestEntries(t, kramaIDs)

	avoidPeers := kramaIDs[:2]
	requiredCount := 16

	randomizerParams := &createRandomizerParams{
		serverCallback: func(server *MockServer) {
			server.setBootstrapPeerIDs(peerIDs)
		},
		repEngineCallback: func(reputationEngine *MockReputationEngine) {
			reputationEngine.setEntries(t, entries)
			reputationEngine.setPeerCount(uint64(len(kramaIDs)))
		},
	}

	randomizer := createTestRandomizer(t, randomizerParams)

	randomizer.addPeers(0)

	peerList := randomizer.getPeers(0, requiredCount, avoidPeers)
	require.Equal(t, requiredCount, len(peerList))
	require.Equal(t, requiredCount, randomizer.peers[0].pendingCount)
	require.Equal(t, true, randomizer.peers[0].updatePending)
}

func TestRandomizer_GetRandomNodes(t *testing.T) {
	peerIDs := tests.GetTestPeerIDs(t, 1)
	kramaIDs := tests.GetTestKramaIDs(t, 20)
	entries := getTestEntries(t, kramaIDs)

	randomizerParams := &createRandomizerParams{
		serverCallback: func(server *MockServer) {
			server.setBootstrapPeerIDs(peerIDs)
		},
		repEngineCallback: func(reputationEngine *MockReputationEngine) {
			reputationEngine.setEntries(t, entries)
			reputationEngine.setPeerCount(uint64(len(kramaIDs)))
		},
	}

	randomizer := createTestRandomizer(t, randomizerParams)

	randomizer.addPeers(0)

	ctx, cancelFn := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFn()

	testcases := []struct {
		name          string
		ctx           context.Context
		count         int
		avoidPeers    []kramaid.KramaID
		expectedError error
	}{
		{
			name:       "Should be able to retrieve random nodes successfully",
			ctx:        ctx,
			count:      16,
			avoidPeers: kramaIDs[:2],
		},
		{
			name:       "Should return an empty list as the required count is negative",
			ctx:        ctx,
			count:      -1,
			avoidPeers: kramaIDs[:2],
		},
		{
			name:          "Should return an error as the context got timed out",
			ctx:           ctx,
			count:         25,
			avoidPeers:    kramaIDs[:2],
			expectedError: common.ErrTimeOut,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			peers, err := randomizer.GetRandomNodes(test.ctx, test.count, test.avoidPeers)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			if test.count >= 0 {
				require.Equal(t, test.count, len(peers))
			}
		})
	}
}

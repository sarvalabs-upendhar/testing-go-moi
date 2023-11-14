package p2p

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sarvalabs/go-moi/common/tests"

	"github.com/stretchr/testify/require"
)

func TestCoolDownCache_Add(t *testing.T) {
	cache := newCoolDownCache()
	peerID := tests.GetTestPeerID(t)

	testcases := []struct {
		name           string
		expectedResult bool
	}{
		{
			name:           "add a peer and check if it's present in the cache",
			expectedResult: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			cache.Add(peerID)
			require.Equal(t, test.expectedResult, cache.Has(peerID))
		})
	}
}

func TestCoolDownCache_Has(t *testing.T) {
	cache := newCoolDownCache()
	peerIDs := tests.GetTestPeerIDs(t, 2)

	testcases := []struct {
		name           string
		peerID         peer.ID
		testFn         func()
		expectedResult bool
	}{
		{
			name:           "peer doesn't exist in cache",
			peerID:         peerIDs[0],
			expectedResult: false,
		},
		{
			name:   "peer exists in cache and hasn't exceeded the cooldown time",
			peerID: peerIDs[0],
			testFn: func() {
				cache.timers[peerIDs[0].String()] = time.Now().Add(-1 * time.Minute)
			},
			expectedResult: true,
		},
		{
			name:   "Peer exists in cache and has reached the cooldown time",
			peerID: peerIDs[1],
			testFn: func() {
				cache.timers[peerIDs[1].String()] = time.Now().Add(-2 * time.Minute)
			},
			expectedResult: false,
		},
		{
			name:   "Peer exists in cache and has exceeded the cooldown time",
			peerID: peerIDs[1],
			testFn: func() {
				cache.timers[peerIDs[1].String()] = time.Now().Add(-3 * time.Minute)
			},
			expectedResult: false,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			require.Equal(t, test.expectedResult, cache.Has(test.peerID))
		})
	}
}

func TestCoolDownCache_Reset(t *testing.T) {
	cache := newCoolDownCache()
	peerID := tests.GetTestPeerID(t)

	testcases := []struct {
		name   string
		testFn func()
	}{
		{
			name: "reset the cache and check if it's empty",
			testFn: func() {
				cache.Add(peerID)
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			test.testFn()
			cache.Reset()

			require.True(t, len(cache.timers) == 0)
		})
	}
}

func TestCoolDownCacheCleanup(t *testing.T) {
	peerIDs := tests.GetTestPeerIDs(t, 4)
	cache := &coolDownCache{
		timers: make(map[string]time.Time),
	}

	testcases := []struct {
		name          string
		testFn        func()
		expectedPeers []peer.ID
	}{
		{
			name: "remove expired entries",
			testFn: func() {
				cache.timers = map[string]time.Time{
					peerIDs[0].String(): time.Now().Add(-4 * time.Minute),
					peerIDs[1].String(): time.Now().Add(-2 * time.Minute),
					peerIDs[2].String(): time.Now().Add(-1 * time.Minute),
					peerIDs[3].String(): time.Now().Add(1 * time.Minute),
				}
			},
			expectedPeers: []peer.ID{peerIDs[2], peerIDs[3]},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			test.testFn()

			cache.cleanup()

			require.Equal(t, len(test.expectedPeers), len(cache.timers))

			for _, expectedPeer := range test.expectedPeers {
				require.True(t, cache.Has(expectedPeer))
			}
		})
	}
}

func TestCoolDownCachePruneOldest(t *testing.T) {
	peerIDs := tests.GetTestPeerIDs(t, 4)
	cache := &coolDownCache{
		timers: make(map[string]time.Time),
	}

	testcases := []struct {
		name          string
		testFn        func()
		expectedPeers []peer.ID
	}{
		{
			name: "remove oldest entries",
			testFn: func() {
				cache.timers = map[string]time.Time{
					peerIDs[0].String(): time.Now().Add(-4 * time.Minute),
					peerIDs[1].String(): time.Now().Add(-30 * time.Minute),
					peerIDs[2].String(): time.Now().Add(3 * time.Minute),
					peerIDs[3].String(): time.Now().Add(5 * time.Minute),
				}
			},
			expectedPeers: []peer.ID{peerIDs[0], peerIDs[2], peerIDs[3]},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			test.testFn()

			cache.pruneOldest()

			require.Equal(t, len(test.expectedPeers), len(cache.timers))

			for peerIDInString := range cache.timers {
				peerID, err := peer.Decode(peerIDInString)
				require.NoError(t, err)
				require.Contains(t, test.expectedPeers, peerID)
			}
		})
	}
}

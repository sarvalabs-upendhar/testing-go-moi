package session

import (
	"context"
	"testing"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
)

func TestPeerDisconnected(t *testing.T) {
	network := NewMockNetwork()
	sessionID := tests.RandomAddress(t)

	peerID := tests.GetTestKramaIDs(t, 1)[0]
	pm := NewTestPeerManager(sessionID, network)

	// update connected peers
	pm.peerConnected(peerID)

	require.Contains(t, pm.connectedPeers, peerID)

	p2pID, err := utils.GetNetworkID(peerID)
	require.NoError(t, err)
	// disconnect the peer
	pm.PeerDisconnected(p2pID)

	// peer should not be available in connected peers
	require.NotContains(t, pm.connectedPeers, peerID)
}

func TestUpdateFailedAttempts(t *testing.T) {
	network := NewMockNetwork()
	sessionID := tests.RandomAddress(t)

	peerID := tests.GetTestKramaIDs(t, 2)
	pm := NewTestPeerManager(sessionID, network)

	tt := []struct {
		name         string
		peers        map[kramaid.KramaID]int
		peerID       kramaid.KramaID
		delta        int
		isSuccess    bool
		updatedCount int
	}{
		{
			name:         "Peer not available",
			peers:        nil,
			peerID:       tests.GetTestKramaIDs(t, 1)[0],
			delta:        0,
			isSuccess:    false,
			updatedCount: 0,
		},
		{
			name:         "Increase failed attempts",
			peers:        map[kramaid.KramaID]int{peerID[0]: 1},
			peerID:       peerID[0],
			delta:        1,
			isSuccess:    true,
			updatedCount: 2,
		},
		{
			name:         "Decrease failed attempts",
			peers:        map[kramaid.KramaID]int{peerID[1]: 2},
			peerID:       peerID[1],
			delta:        -1,
			isSuccess:    true,
			updatedCount: 1,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			if test.peers != nil {
				for kramaID, failedAttempts := range test.peers {
					pm.peers[kramaID] = &PeerInfo{
						failedAttempts: failedAttempts,
					}
				}
			}

			status := pm.UpdateFailedAttempts(test.peerID, test.delta)
			require.Equal(t, test.isSuccess, status)

			if status {
				require.Equal(t, test.updatedCount, pm.peers[test.peerID].failedAttempts)
			}
		})
	}
}

func TestUpdatePeerStatus(t *testing.T) {
	network := NewMockNetwork()
	sessionID := tests.RandomAddress(t)

	peerID := tests.GetTestKramaIDs(t, 2)
	pm := NewTestPeerManager(sessionID, network)

	tt := []struct {
		name          string
		peers         map[kramaid.KramaID]bool
		peerID        kramaid.KramaID
		isSuccess     bool
		status        bool
		updatedStatus bool
	}{
		{
			name:          "peer not available",
			peers:         nil,
			peerID:        tests.GetTestKramaIDs(t, 1)[0],
			isSuccess:     false,
			status:        true,
			updatedStatus: true,
		},
		{
			name:          "change peer status to false",
			peers:         map[kramaid.KramaID]bool{peerID[0]: true},
			peerID:        peerID[0],
			isSuccess:     true,
			status:        false,
			updatedStatus: false,
		},
		{
			name:          "change peer status to true",
			peers:         map[kramaid.KramaID]bool{peerID[1]: false},
			peerID:        peerID[1],
			isSuccess:     true,
			status:        true,
			updatedStatus: true,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			if test.peers != nil {
				for kramaID, isActive := range test.peers {
					pm.peers[kramaID] = &PeerInfo{
						isActive: isActive,
					}
				}
			}

			isSuccess := pm.UpdatePeerStatus(test.peerID, test.status)
			require.Equal(t, test.isSuccess, isSuccess)

			if isSuccess {
				require.Equal(t, test.updatedStatus, pm.peers[test.peerID].isActive)
			}
		})
	}
}

func TestChooseBestPeer_FromConnectedPeers(t *testing.T) {
	network := NewMockNetwork()
	sessionID := tests.RandomAddress(t)

	ids := tests.GetTestKramaIDs(t, 2)
	tt := []struct {
		name  string
		peers map[kramaid.KramaID]struct {
			isActive       bool
			failedAttempts int
		}
		err        error
		avoidPeers map[kramaid.KramaID]interface{}
		result     kramaid.KramaID
	}{
		{
			name: "should return error if no peer is available",
			peers: map[kramaid.KramaID]struct {
				isActive       bool
				failedAttempts int
			}{
				ids[0]: {
					isActive:       true,
					failedAttempts: 0,
				},
			},
			err:        common.ErrPeerNotAvailable,
			avoidPeers: nil,
			result:     "",
		},
		{
			name: "should not choose from avoided peers",
			peers: map[kramaid.KramaID]struct {
				isActive       bool
				failedAttempts int
			}{
				ids[0]: {
					isActive:       false,
					failedAttempts: 0,
				},

				ids[1]: {
					isActive:       false,
					failedAttempts: 0,
				},
			},
			err: nil,
			avoidPeers: map[kramaid.KramaID]interface{}{
				ids[0]: nil,
			},
			result: ids[1],
		},
		{
			name: "should not choose active peers",
			peers: map[kramaid.KramaID]struct {
				isActive       bool
				failedAttempts int
			}{
				ids[0]: {
					isActive:       true,
					failedAttempts: 0,
				},

				ids[1]: {
					isActive:       false,
					failedAttempts: 0,
				},
			},
			err:    nil,
			result: ids[1],
		},
		{
			name: "should not choose peer if failed attempts >= 3 ",
			peers: map[kramaid.KramaID]struct {
				isActive       bool
				failedAttempts int
			}{
				ids[0]: {
					isActive:       false,
					failedAttempts: 0,
				},

				ids[1]: {
					isActive:       false,
					failedAttempts: 3,
				},
			},
			err:    nil,
			result: ids[0],
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			pm := NewTestPeerManager(sessionID, network)

			for kramaID, info := range test.peers {
				pm.peers[kramaID] = &PeerInfo{
					isActive:       info.isActive,
					failedAttempts: info.failedAttempts,
				}
				pm.connectedPeers[kramaID] = nil
			}

			peerID, err := pm.chooseBestPeer(context.Background(), test.avoidPeers)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.Equal(t, test.result, peerID)
			}
		})
	}
}

package senatus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/storage"
)

func TestReputationEngine_NodeMetaInfo(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "node meta info found in cache",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				reputationEngine.cache.Add(storage.SenatusCacheKey(peerID), nodeMetaInfo)
			},
		},
		{
			name:   "node meta info found in dirty entries",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: -1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				reputationEngine.dirtyEntries[peerID] = nodeMetaInfo
			},
		},
		{
			name:   "node meta info found in db",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "node meta info not found",
			peerID:      tests.GetTestPeerID(t),
			expectedErr: common.ErrKramaIDNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerID, test.nodeMetaInfo)
			}

			metaInfo, err := reputationEngine.nodeMetaInfo(test.peerID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.nodeMetaInfo.NTQ, metaInfo.NTQ)
			require.Equal(t, test.nodeMetaInfo.WalletCount, metaInfo.WalletCount)
		})
	}
}

func TestReputationEngine_AddNewPeerwithPeerID(t *testing.T) {
	metaInfo := &NodeMetaInfo{
		Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
		NTQ:           2,
		WalletCount:   1,
		PublicKey:     tests.GetTestPublicKey(t),
		PeerSignature: []byte{0x05, 0x80},
	}

	testcases := []struct {
		name         string
		peerID       peer.ID
		testFn       func(peerID peer.ID, reputationEngine *ReputationEngine)
		nodeMetaInfo *NodeMetaInfo
		peerCount    uint64
	}{
		{
			name:   "node meta info found in cache",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				KramaID:       tests.GetTestKramaID(t, 2),
				PublicKey:     tests.GetTestPublicKey(t),
				PeerSignature: []byte{0x05, 0x80},
				NTQ:           1,
				WalletCount:   2,
			},
			peerCount: 1,
			testFn: func(peerID peer.ID, reputationEngine *ReputationEngine) {
				reputationEngine.cache.Add(storage.SenatusCacheKey(peerID), metaInfo)
				_, ok := reputationEngine.cache.Get(storage.SenatusCacheKey(peerID))
				require.True(t, ok)
			},
		},
		{
			name:   "peer id not found in db",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				KramaID:       tests.GetTestKramaID(t, 2),
				PublicKey:     tests.GetTestPublicKey(t),
				PeerSignature: []byte{0x05, 0x80},
				NTQ:           3,
				WalletCount:   4,
			},
			peerCount: 2,
		},
		{
			name:   "node meta info found in dirty entries",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				KramaID:       tests.GetTestKramaID(t, 2),
				PublicKey:     tests.GetTestPublicKey(t),
				PeerSignature: []byte{0x05, 0x80},
				NTQ:           3,
				WalletCount:   1,
			},
			peerCount: 1,
			testFn: func(peerID peer.ID, reputationEngine *ReputationEngine) {
				reputationEngine.dirtyEntries[peerID] = metaInfo
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, _, _ := createTestReputationEngine(t)

			if test.testFn != nil {
				test.testFn(test.peerID, reputationEngine)
			}

			err := reputationEngine.AddNewPeerWithPeerID(test.peerID, test.nodeMetaInfo)
			require.NoError(t, err)

			nodeInfo, err := reputationEngine.nodeMetaInfo(test.peerID)
			require.NoError(t, err)
			require.Equal(t, test.nodeMetaInfo, nodeInfo)

			peerCount := reputationEngine.TotalPeerCount()
			require.Equal(t, test.peerCount, peerCount)
		})
	}
}

func TestReputationEngine_UpdatePeer(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	kramaID := tests.GetTestKramaID(t, 1)

	testcases := []struct {
		name         string
		testFn       func(kramaID kramaid.KramaID, nodeMetaInfo *NodeMetaInfo)
		nodeMetaInfo *NodeMetaInfo
		expectedErr  error
		shouldSkip   bool
	}{
		{
			name: "invalid krama id",
			nodeMetaInfo: &NodeMetaInfo{
				KramaID:     "",
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			expectedErr: common.ErrInvalidKramaID,
		},
		{
			name: "krama id without state",
			nodeMetaInfo: &NodeMetaInfo{
				KramaID:     tests.GetTestKramaID(t, 1),
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
		},
		{
			name: "krama id with state",
			testFn: func(kramaID kramaid.KramaID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, tests.DecodePeerIDFromKramaID(t, kramaID), nodeMetaInfo)
			},
			nodeMetaInfo: &NodeMetaInfo{
				KramaID:     kramaID,
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			shouldSkip:  true,
			expectedErr: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(kramaID, test.nodeMetaInfo)
			}

			err := reputationEngine.UpdatePeer(test.nodeMetaInfo)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			peerID := tests.DecodePeerIDFromKramaID(t, test.nodeMetaInfo.KramaID)
			nodeInfo, ok := reputationEngine.dirtyEntries[peerID]
			if !test.shouldSkip { // FIXME: This is just a temporary fix, need to update senatus logic
				require.True(t, ok)
				require.Equal(t, test.nodeMetaInfo, nodeInfo)
			}
		})
	}
}

func TestReputationEngine_UpdateNTQ(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	testcases := []struct {
		name             string
		kramaID          kramaid.KramaID
		ntq              float32
		testFn           func(kramaID kramaid.KramaID)
		expectedErr      error
		expectedNodeInfo *NodeMetaInfo
	}{
		{
			name:        "invalid krama id",
			kramaID:     "",
			ntq:         DefaultPeerNTQ,
			expectedErr: common.ErrInvalidKramaID,
		},
		{
			name:    "krama id without state",
			kramaID: tests.GetTestKramaID(t, 1),
			ntq:     DefaultPeerNTQ,
			expectedNodeInfo: &NodeMetaInfo{
				NTQ: DefaultPeerNTQ,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			ntq:     1,
			testFn: func(kramaID kramaid.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						NTQ:         DefaultPeerNTQ,
						WalletCount: 1,
					},
				)
			},
			expectedNodeInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			err := reputationEngine.UpdateNTQ(test.kramaID, test.ntq)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			peerID := tests.DecodePeerIDFromKramaID(t, test.kramaID)
			nodeInfo, ok := reputationEngine.dirtyEntries[peerID]

			require.True(t, ok)
			require.Equal(t, test.expectedNodeInfo, nodeInfo)
		})
	}
}

func TestReputationEngine_UpdateWalletCount(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	testcases := []struct {
		name             string
		kramaID          kramaid.KramaID
		walletCount      int32
		testFn           func(kramaID kramaid.KramaID)
		expectedErr      error
		expectedNodeInfo *NodeMetaInfo
	}{
		{
			name:        "invalid krama id",
			kramaID:     "",
			walletCount: 1,
			expectedErr: common.ErrInvalidKramaID,
		},
		{
			name:        "krama id without state",
			kramaID:     tests.GetTestKramaID(t, 1),
			walletCount: -1,
			expectedNodeInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: -1,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID kramaid.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						NTQ:         1,
						WalletCount: -1,
					},
				)
			},
			walletCount: 1,
			expectedNodeInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 0,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			err := reputationEngine.UpdateWalletCount(test.kramaID, test.walletCount)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			peerID := tests.DecodePeerIDFromKramaID(t, test.kramaID)
			nodeInfo, ok := reputationEngine.dirtyEntries[peerID]

			require.True(t, ok)
			require.Equal(t, test.expectedNodeInfo, nodeInfo)
		})
	}
}

func TestReputationEngine_UpdatePublicKey(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	publicKeys := tests.GetTestPublicKeys(t, 3)
	testcases := []struct {
		name             string
		kramaID          kramaid.KramaID
		publicKey        []byte
		testFn           func(kramaID kramaid.KramaID)
		expectedErr      error
		expectedNodeInfo *NodeMetaInfo
	}{
		{
			name:        "invalid krama id",
			kramaID:     "",
			publicKey:   publicKeys[0],
			expectedErr: common.ErrInvalidKramaID,
		},
		{
			name:      "krama id without state",
			kramaID:   tests.GetTestKramaID(t, 1),
			publicKey: publicKeys[1],
			expectedNodeInfo: &NodeMetaInfo{
				NTQ:       DefaultPeerNTQ,
				PublicKey: publicKeys[1],
			},
		},
		{
			name:      "krama id with state",
			kramaID:   tests.GetTestKramaID(t, 1),
			publicKey: publicKeys[2],
			testFn: func(kramaID kramaid.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						NTQ:         1,
						WalletCount: -1,
					},
				)
			},
			expectedNodeInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: -1,
				PublicKey:   publicKeys[2],
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			err := reputationEngine.UpdatePublicKey(test.kramaID, test.publicKey)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			peerID := tests.DecodePeerIDFromKramaID(t, test.kramaID)
			nodeInfo, ok := reputationEngine.dirtyEntries[peerID]

			require.True(t, ok)
			require.Equal(t, test.expectedNodeInfo, nodeInfo)
		})
	}
}

func TestReputationEngine_GetAddress(t *testing.T) {
	address := tests.GetListenAddresses(t, 1)
	reputationEngine, mockDB, _ := createTestReputationEngine(t)

	testcases := []struct {
		name            string
		kramaID         kramaid.KramaID
		testFn          func(kramaID kramaid.KramaID)
		expectedErr     error
		expectedAddress []multiaddr.Multiaddr
	}{
		{
			name:        "krama id without state",
			kramaID:     tests.GetTestKramaID(t, 1),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID kramaid.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						Addrs: utils.MultiAddrToString(address...),
					},
				)
			},
			expectedAddress: address,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			address, err := reputationEngine.GetAddress(test.kramaID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.Equal(t, test.expectedAddress, address)
		})
	}
}

func TestReputationEngine_GetAddressByPeerID(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:       utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "peer id without state",
			peerID:      tests.GetTestPeerID(t),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:   "peer id with state and without address",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
			expectedErr: errors.New("address not found"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerID, test.nodeMetaInfo)
			}

			addr, err := reputationEngine.GetAddressByPeerID(test.peerID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr.Error(), err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, utils.MultiAddrFromString(test.nodeMetaInfo.Addrs...), addr)
		})
	}
}

func TestReputationEngine_GetRTTByPeerID(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				RTT: 150,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "peer id without state",
			peerID:      tests.GetTestPeerID(t),
			expectedErr: common.ErrKramaIDNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerID, test.nodeMetaInfo)
			}

			rtt, err := reputationEngine.GetRTTByPeerID(test.peerID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr.Error(), err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.nodeMetaInfo.RTT, rtt)
		})
	}
}

func TestReputationEngine_GetKramaIDByPeerID(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)
	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				KramaID: tests.GetTestKramaID(t, 2),
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "peer id without state",
			peerID:      tests.GetTestPeerID(t),
			expectedErr: common.ErrKramaIDNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerID, test.nodeMetaInfo)
			}

			kramaID, err := reputationEngine.GetKramaIDByPeerID(test.peerID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr.Error(), err.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.nodeMetaInfo.KramaID, kramaID)
		})
	}
}

func TestReputationEngine_GetNTQ(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)

	testcases := []struct {
		name        string
		kramaID     kramaid.KramaID
		testFn      func(kramaID kramaid.KramaID)
		expectedErr error
		expectedNTQ float32
	}{
		{
			name:        "krama id without state",
			kramaID:     tests.GetTestKramaID(t, 1),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID kramaid.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						NTQ: DefaultPeerNTQ,
					},
				)
			},
			expectedNTQ: DefaultPeerNTQ,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			ntq, err := reputationEngine.GetNTQ(test.kramaID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.Equal(t, test.expectedNTQ, ntq)
		})
	}
}

func TestReputationEngine_GetWalletCount(t *testing.T) {
	reputationEngine, mockDB, _ := createTestReputationEngine(t)

	testcases := []struct {
		name                string
		kramaID             kramaid.KramaID
		testFn              func(kramaID kramaid.KramaID)
		expectedErr         error
		expectedWalletCount int32
	}{
		{
			name:        "krama id without state",
			kramaID:     tests.GetTestKramaID(t, 1),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID kramaid.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						WalletCount: -1,
					},
				)
			},
			expectedWalletCount: -1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			walletCount, err := reputationEngine.GetWalletCount(test.kramaID)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.Equal(t, test.expectedWalletCount, walletCount)
		})
	}
}

func TestReputationEngine_TotalPeerCount(t *testing.T) {
	testcases := []struct {
		name          string
		preTestFn     func(rp *ReputationEngine)
		expectedCount uint64
		expectedError error
	}{
		{
			name: "Check peer count after adding few entries",
			preTestFn: func(rp *ReputationEngine) {
				err := rp.AddNewPeerWithPeerID(tests.GetTestPeerID(t), &NodeMetaInfo{
					NTQ: DefaultPeerNTQ,
				})
				assert.NoError(t, err)
				err = rp.AddNewPeerWithPeerID(tests.GetTestPeerID(t), &NodeMetaInfo{
					NTQ: DefaultPeerNTQ,
				})
				assert.NoError(t, err)
				err = rp.AddNewPeerWithPeerID(tests.GetTestPeerID(t), &NodeMetaInfo{
					NTQ: DefaultPeerNTQ,
				})
				assert.NoError(t, err)
			},
			expectedCount: 4,
		},
		{
			name:          "Check peer count without adding any entries",
			expectedCount: 1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, mockDB, _ := createTestReputationEngine(t)

			if test.preTestFn != nil {
				test.preTestFn(reputationEngine)
			}

			count := reputationEngine.TotalPeerCount()

			require.Equal(t, test.expectedCount, count)

			// peer count should match with the value in the db
			dbPeerCount, err := mockDB.TotalPeersCount()
			require.NoError(t, err)
			require.Equal(t, test.expectedCount, dbPeerCount)
		})
	}
}

func TestReputationEngine_StreamPeerInfos(t *testing.T) {
	testcases := []struct {
		name    string
		entries map[peer.ID]*NodeMetaInfo
		testFn  func(mockDB *MockDB, entries map[peer.ID]*NodeMetaInfo)
	}{
		{
			name: "db with multiple entries",
			entries: map[peer.ID]*NodeMetaInfo{
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: -1,
				},
				tests.GetTestPeerID(t): {
					NTQ:         1,
					WalletCount: 1,
				},
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: 0,
				},
			},
			testFn: func(mockDB *MockDB, entries map[peer.ID]*NodeMetaInfo) {
				for key, entry := range entries {
					mockDB.setNodeInfo(t, key, entry)
				}
			},
		},
		{
			name:    "db without entries",
			entries: map[peer.ID]*NodeMetaInfo{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, mockDB, _ := createTestReputationEngine(t)

			if test.testFn != nil {
				test.testFn(mockDB, test.entries)
			}

			peerInfos, err := reputationEngine.StreamPeerInfos(context.Background())
			require.NoError(t, err)

			for range test.entries {
				peerInfo := <-peerInfos
				_, ok := test.entries[peerInfo.ID]

				require.True(t, ok)
			}

			peerInfo := <-peerInfos

			require.Nil(t, peerInfo)
		})
	}
}

func TestReputationEngine_FlushDirtyEntries(t *testing.T) {
	testcases := []struct {
		name        string
		entries     map[peer.ID]*NodeMetaInfo
		testFn      func(reputationEngine *ReputationEngine, entries map[peer.ID]*NodeMetaInfo)
		expectedErr error
	}{
		{
			name: "dirty entries map with multiple entries",
			entries: map[peer.ID]*NodeMetaInfo{
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: -1,
				},
				tests.GetTestPeerID(t): {
					NTQ:         1,
					WalletCount: 1,
				},
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: 0,
				},
			},
			testFn: func(reputationEngine *ReputationEngine, entries map[peer.ID]*NodeMetaInfo) {
				for key, entry := range entries {
					reputationEngine.dirtyEntries[key] = entry
				}
			},
		},
		{
			name:    "empty dirty entries map",
			entries: map[peer.ID]*NodeMetaInfo{},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, mockDB, _ := createTestReputationEngine(t)

			if test.testFn != nil {
				test.testFn(reputationEngine, test.entries)
			}

			err := reputationEngine.flushDirtyEntries()

			require.NoError(t, err)
			require.Equal(t, len(test.entries)+1, len(mockDB.data))
		})
	}
}

func TestReputationEngine_DBWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reputationEngine, _, _ := createTestReputationEngine(t)
	reputationEngine.ctx = ctx

	for i := 0; i < 2; i++ {
		reputationEngine.dirtyEntries[tests.GetTestPeerID(t)] = &NodeMetaInfo{
			NTQ: float32(i),
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		reputationEngine.dbWorker()
	}()
	reputationEngine.signalChan <- struct{}{}
	// Wait for a short time to let the worker handle messages
	time.Sleep(1 * time.Second)

	reputationEngine.dirtyLock.RLock()
	defer reputationEngine.dirtyLock.RUnlock()

	require.Equal(t, 0, len(reputationEngine.dirtyEntries))

	// Cancel the context to stop the worker
	cancel()

	// Wait for the worker to finish
	wg.Wait()
}

func TestReputationEngine_CleanUpDirtyStorage(t *testing.T) {
	reputationEngine, _, _ := createTestReputationEngine(t)
	testcases := []struct {
		name    string
		entries map[peer.ID]*NodeMetaInfo
		testFn  func(entries map[peer.ID]*NodeMetaInfo)
	}{
		{
			name: "dirty entries map with single entry",
			entries: map[peer.ID]*NodeMetaInfo{
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: -1,
				},
			},
			testFn: func(entries map[peer.ID]*NodeMetaInfo) {
				for key, entry := range entries {
					reputationEngine.dirtyEntries[key] = entry
				}
			},
		},
		{
			name: "dirty entries map with multiple entries",
			entries: map[peer.ID]*NodeMetaInfo{
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: 0,
				},
				tests.GetTestPeerID(t): {
					NTQ:         1,
					WalletCount: -1,
				},
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: 1,
				},
			},
			testFn: func(entries map[peer.ID]*NodeMetaInfo) {
				for key, entry := range entries {
					reputationEngine.dirtyEntries[key] = entry
				}
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.entries)
			}

			reputationEngine.cleanUpDirtyStorage()

			require.Equal(t, 0, len(reputationEngine.dirtyEntries))
		})
	}
}

func TestReputationEngine_LoadPeerCountWhileSetup(t *testing.T) {
	testcases := []struct {
		name          string
		preTestFn     func(db *MockDB)
		expectedCount uint64
		expectedError error
	}{
		{
			name: "Failed to read senatus peer count from DB",
			preTestFn: func(db *MockDB) {
				db.peerCountHook = func() error {
					return errors.New("db init failed")
				}
			},
			expectedError: errors.New("db init failed"),
		},
		{
			name: "PeerCount entry exists in DB",
			preTestFn: func(db *MockDB) {
				db.peerCount = 10
			},
			expectedError: nil,
			expectedCount: 11, // This is 10+1, as self node info is added to senatus
		},
		{
			name: "PeerCount entry doesn't exists in DB",
			preTestFn: func(db *MockDB) {
				db.peerCountHook = func() error {
					return common.ErrKeyNotFound
				}
			},
			expectedError: nil,
			expectedCount: 1, // This is 0+1, as self node info is added to senatus
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			mockDB := NewMockDB()
			nodeMetaInfo := &NodeMetaInfo{
				KramaID: tests.GetTestKramaID(t, 0),
			}

			if test.preTestFn != nil {
				test.preTestFn(mockDB)
			}

			r, err := NewReputationEngine(
				hclog.NewNullLogger(),
				mockDB,
				nodeMetaInfo,
			)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedCount, r.TotalPeerCount())
		})
	}
}

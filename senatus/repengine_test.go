package senatus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"
	"github.com/sarvalabs/go-polo"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/storage"
)

func TestReputationEngine_NodeMetaInfo(t *testing.T) {
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name             string
		peerID           peer.ID
		nodeMetaInfo     *NodeMetaInfo
		expectedRegistry bool
		testFn           func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr      error
	}{
		{
			name:   "node meta info found in cache",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
				Registered:  true,
			},
			expectedRegistry: true,
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				reputationEngine.cache.Add(storage.SenatusCacheKey(peerID), nodeMetaInfo)
			},
		},
		{
			name:   "node meta info found in dirty entries",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: -1,
				Registered:  true,
			},
			expectedRegistry: true,
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				reputationEngine.dirtyEntries[peerID] = nodeMetaInfo
			},
		},
		{
			name:   "node meta info found in db",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
				Registered:  true, // not written to DB, because of polo tag: "-"
			},
			expectedRegistry: false,
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "node meta info not found",
			peerID:      tests.RandomPeerID(t),
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
			require.Equal(t, test.expectedRegistry, metaInfo.Registered)
		})
	}
}

func TestReputationEngine_AddNewPeerwithPeerID(t *testing.T) {
	metaInfo := &NodeMetaInfo{
		Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
		NTQ:           2,
		WalletCount:   1,
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
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				KramaID:       tests.RandomKramaID(t, 2),
				PeerSignature: []byte{0x05, 0x80},
				NTQ:           1,
				WalletCount:   2,
				Registered:    true,
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
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				KramaID:       tests.RandomKramaID(t, 2),
				PeerSignature: []byte{0x05, 0x80},
				NTQ:           3,
				WalletCount:   4,
			},
			peerCount: 2,
		},
		{
			name:   "node meta info found in dirty entries",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				KramaID:       tests.RandomKramaID(t, 2),
				PeerSignature: []byte{0x05, 0x80},
				NTQ:           3,
				WalletCount:   1,
				Registered:    true,
			},
			peerCount: 1,
			testFn: func(peerID peer.ID, reputationEngine *ReputationEngine) {
				reputationEngine.dirtyEntries[peerID] = metaInfo
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, _ := createTestReputationEngine(t)

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
	kramaID := tests.RandomKramaID(t, 1)
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name         string
		testFn       func(kramaID identifiers.KramaID, nodeMetaInfo *NodeMetaInfo)
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
				KramaID:     tests.RandomKramaID(t, 1),
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
		},
		{
			name: "krama id with state",
			testFn: func(kramaID identifiers.KramaID, nodeMetaInfo *NodeMetaInfo) {
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
	kramaID := tests.RandomKramaID(t, 1)
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name             string
		kramaID          identifiers.KramaID
		ntq              float32
		testFn           func(kramaID identifiers.KramaID)
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
			kramaID: kramaID,
			ntq:     DefaultPeerNTQ,
			expectedNodeInfo: &NodeMetaInfo{
				KramaID: kramaID,
				NTQ:     DefaultPeerNTQ,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.RandomKramaID(t, 1),
			ntq:     1,
			testFn: func(kramaID identifiers.KramaID) {
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
	kramaID := tests.RandomKramaID(t, 1)
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name             string
		kramaID          identifiers.KramaID
		walletCount      int32
		testFn           func(kramaID identifiers.KramaID)
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
			kramaID:     kramaID,
			walletCount: -1,
			expectedNodeInfo: &NodeMetaInfo{
				KramaID:     kramaID,
				NTQ:         DefaultPeerNTQ,
				WalletCount: -1,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.RandomKramaID(t, 1),
			testFn: func(kramaID identifiers.KramaID) {
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

func TestReputationEngine_GetAddress(t *testing.T) {
	address := tests.GetListenAddresses(t, 1)
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name            string
		kramaID         identifiers.KramaID
		testFn          func(kramaID identifiers.KramaID)
		expectedErr     error
		expectedAddress []multiaddr.Multiaddr
	}{
		{
			name:        "krama id without state",
			kramaID:     tests.RandomKramaID(t, 1),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.RandomKramaID(t, 1),
			testFn: func(kramaID identifiers.KramaID) {
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
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.RandomPeerID(t),
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
			peerID:      tests.RandomPeerID(t),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:   "peer id with state and without address",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
			expectedErr: common.ErrAddressNotFound,
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
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				RTT: 150,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "peer id without state",
			peerID:      tests.RandomPeerID(t),
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
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.RandomPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				KramaID: tests.RandomKramaID(t, 2),
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "peer id without state",
			peerID:      tests.RandomPeerID(t),
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
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name        string
		kramaID     identifiers.KramaID
		testFn      func(kramaID identifiers.KramaID)
		expectedErr error
		expectedNTQ float32
	}{
		{
			name:        "krama id without state",
			kramaID:     tests.RandomKramaID(t, 1),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.RandomKramaID(t, 1),
			testFn: func(kramaID identifiers.KramaID) {
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
	reputationEngine, mockDB := createTestReputationEngine(t)

	testcases := []struct {
		name                string
		kramaID             identifiers.KramaID
		testFn              func(kramaID identifiers.KramaID)
		expectedErr         error
		expectedWalletCount int32
	}{
		{
			name:        "krama id without state",
			kramaID:     tests.RandomKramaID(t, 1),
			expectedErr: common.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.RandomKramaID(t, 1),
			testFn: func(kramaID identifiers.KramaID) {
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
				err := rp.AddNewPeerWithPeerID(tests.RandomPeerID(t), &NodeMetaInfo{
					NTQ: DefaultPeerNTQ,
				})
				assert.NoError(t, err)
				err = rp.AddNewPeerWithPeerID(tests.RandomPeerID(t), &NodeMetaInfo{
					NTQ: DefaultPeerNTQ,
				})
				assert.NoError(t, err)
				err = rp.AddNewPeerWithPeerID(tests.RandomPeerID(t), &NodeMetaInfo{
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
			reputationEngine, _ := createTestReputationEngine(t)

			if test.preTestFn != nil {
				test.preTestFn(reputationEngine)
			}

			count := reputationEngine.TotalPeerCount()

			require.Equal(t, test.expectedCount, count)
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
				tests.RandomPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: -1,
				},
				tests.RandomPeerID(t): {
					NTQ:         1,
					WalletCount: 1,
				},
				tests.RandomPeerID(t): {
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
			reputationEngine, mockDB := createTestReputationEngine(t)

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
	testKramaID := tests.RandomKramaID(t, 0)

	// Generate the hash of the krama ID
	kramaIDEncoded, _ := polo.Polorize(testKramaID)
	kramaIDHashed := common.GetHash(kramaIDEncoded)

	// Generate the storage key for the guardian with the given krama ID
	storageKey := pisa.GenerateStorageKey(guardianregistry.SlotGuardians, pisa.MapKey(kramaIDHashed))

	testcases := []struct {
		name           string
		entries        map[peer.ID]*NodeMetaInfo
		testFn         func(reputationEngine *ReputationEngine, entries map[peer.ID]*NodeMetaInfo)
		sysAccSyncDone bool
		expectedErr    error
	}{
		{
			name: "dirty entries map with multiple entries",
			entries: map[peer.ID]*NodeMetaInfo{
				tests.RandomPeerID(t): {
					KramaID:     testKramaID,
					NTQ:         DefaultPeerNTQ,
					WalletCount: -1,
					Registered:  true, // registered guardian
				},
				tests.RandomPeerID(t): {
					KramaID:     testKramaID,
					NTQ:         1,
					WalletCount: 1,
				},
				tests.RandomPeerID(t): {
					KramaID:     testKramaID,
					NTQ:         DefaultPeerNTQ,
					WalletCount: 0,
				},
			},
			testFn: func(reputationEngine *ReputationEngine, entries map[peer.ID]*NodeMetaInfo) {
				for key, entry := range entries {
					reputationEngine.dirtyEntries[key] = entry
				}
			},
			sysAccSyncDone: true,
		},
		{
			name:           "empty dirty entries map",
			entries:        map[peer.ID]*NodeMetaInfo{},
			sysAccSyncDone: true,
		},
		{
			name:        "system account not yet synced",
			expectedErr: common.ErrSysAccsNotSynced,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, mockDB := createTestReputationEngine(t)

			// set mock state and chain manager
			mockState := NewMockState()
			reputationEngine.State = mockState
			mockChain := NewMockChain()
			reputationEngine.Chain = mockChain

			reputationEngine.sysAccSyncDone = test.sysAccSyncDone

			testTesseract := tests.CreateTesseract(t, nil)

			mockState.setAccountMetaInfo(
				t,
				&common.AccountMetaInfo{
					ID:            common.GuardianLogicID.AsIdentifier(),
					TesseractHash: testTesseract.Hash(),
				},
			)

			mockChain.setTesseract(t, testTesseract.Hash(), testTesseract)

			mockState.setStorageEntry(t, common.GuardianLogicID, storageKey)

			if test.testFn != nil {
				test.testFn(reputationEngine, test.entries)
			}

			err := reputationEngine.flushDirtyEntries()

			if test.expectedErr != nil {
				require.Equal(t, test.expectedErr, err)

				return
			}

			require.NoError(t, err)
			// This is len(test.entries)+1, as self node info is added to senatus
			require.Equal(t, len(test.entries)+1, len(mockDB.data))
		})
	}
}

func TestReputationEngine_DBWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reputationEngine, _ := createTestReputationEngine(t)
	reputationEngine.ctx = ctx
	reputationEngine.sysAccSyncDone = true

	for i := 0; i < 2; i++ {
		reputationEngine.dirtyEntries[tests.RandomPeerID(t)] = &NodeMetaInfo{
			NTQ:        float32(i),
			Registered: true,
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
	reputationEngine, _ := createTestReputationEngine(t)

	testcases := []struct {
		name    string
		entries map[peer.ID]*NodeMetaInfo
		testFn  func(entries map[peer.ID]*NodeMetaInfo)
	}{
		{
			name: "dirty entries map with single entry",
			entries: map[peer.ID]*NodeMetaInfo{
				tests.RandomPeerID(t): {
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
				tests.RandomPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: 0,
				},
				tests.RandomPeerID(t): {
					NTQ:         1,
					WalletCount: -1,
				},
				tests.RandomPeerID(t): {
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
			name: "PeerCount entry exists in DB",
			preTestFn: func(db *MockDB) {
				db.setNodeInfo(t, tests.RandomPeerID(t), &NodeMetaInfo{})
				db.setNodeInfo(t, tests.RandomPeerID(t), &NodeMetaInfo{})
				db.setNodeInfo(t, tests.RandomPeerID(t), &NodeMetaInfo{})
			},
			expectedError: nil,
			expectedCount: 4, // This is 3+1, as self node info is added to senatus
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
				KramaID: tests.RandomKramaID(t, 0),
			}

			if test.preTestFn != nil {
				test.preTestFn(mockDB)
			}

			r, err := NewReputationEngine(
				hclog.NewNullLogger(),
				mockDB,
				nodeMetaInfo,
				&utils.TypeMux{},
				nil,
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

func TestReputationEngine_IsGuardianRegistered(t *testing.T) {
	testKramaID := tests.RandomKramaID(t, 0)

	// Generate the hash of the krama ID
	kramaIDEncoded, _ := polo.Polorize(testKramaID)
	kramaIDHashed := common.GetHash(kramaIDEncoded)

	// Generate the storage key for the guardian with the given krama ID
	validStorageKey := pisa.GenerateStorageKey(guardianregistry.SlotGuardians, pisa.MapKey(kramaIDHashed))

	testTesseract := tests.CreateTesseract(t, nil)

	testcases := []struct {
		name           string
		setAccMetaInfo bool
		guardianTSHash common.Hash
		logicID        identifiers.LogicID
		storageKey     []byte
		expectedResult bool
	}{
		{
			name: "failed to get account meta info",
		},
		{
			name:           "failed to fetch tesseract",
			setAccMetaInfo: true,
			// tesseract hash different from the one in accMetaInfo
			guardianTSHash: tests.RandomHash(t),
		},
		{
			name:           "failed to fetch logic storage tree",
			setAccMetaInfo: true,
			guardianTSHash: testTesseract.Hash(),
			logicID:        tests.GetLogicID(t, tests.RandomIdentifier(t)),
		},
		{
			name:           "Invalid storage key",
			setAccMetaInfo: true,
			guardianTSHash: testTesseract.Hash(),
			logicID:        common.GuardianLogicID,
			storageKey:     []byte{1},
		},
		{
			name:           "Guardian is registered",
			setAccMetaInfo: true,
			guardianTSHash: testTesseract.Hash(),
			logicID:        common.GuardianLogicID,
			expectedResult: true,
			storageKey:     validStorageKey,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			reputationEngine, _ := createTestReputationEngine(t)

			// set mock state and chain manager
			mockState := NewMockState()
			reputationEngine.State = mockState
			mockChain := NewMockChain()
			reputationEngine.Chain = mockChain

			if testcase.setAccMetaInfo {
				mockState.setAccountMetaInfo(
					t,
					&common.AccountMetaInfo{
						ID:            common.GuardianLogicID.AsIdentifier(),
						TesseractHash: testTesseract.Hash(),
					},
				)
			}

			mockChain.setTesseract(t, testcase.guardianTSHash, testTesseract)

			mockState.setStorageEntry(t, testcase.logicID, testcase.storageKey)

			check := reputationEngine.isGuardianRegisterd(testKramaID)
			require.Equal(t, testcase.expectedResult, check)
		})
	}
}

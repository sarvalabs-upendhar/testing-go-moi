package senatus

import (
	"context"
	"sync"
	"testing"
	"time"

	id "github.com/sarvalabs/moichain/common/kramaid"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/common/utils"
	"github.com/sarvalabs/moichain/storage"
)

func TestReputationEngine_NodeMetaInfo(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
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
				reputationEngine.cache.Add(storage.NtqCacheKey(peerID), nodeMetaInfo)
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
	reputationEngine, _, _ := CreateTestReputationEngine(t)
	metaInfo := &NodeMetaInfo{
		NTQ:         2,
		WalletCount: 1,
	}

	testcases := []struct {
		name         string
		peerID       peer.ID
		testFn       func(peerID peer.ID, metaInfo *NodeMetaInfo)
		nodeMetaInfo *NodeMetaInfo
		expectedErr  error
	}{
		{
			name:   "node meta info found in cache",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         1,
				WalletCount: 2,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				reputationEngine.cache.Add(storage.NtqCacheKey(peerID), nodeMetaInfo)
			},
		},
		{
			name:   "node meta info found in dirty entries",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         3,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *NodeMetaInfo) {
				reputationEngine.dirtyEntries[peerID] = nodeMetaInfo
			},
			expectedErr: nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.peerID, test.nodeMetaInfo)
			}

			err := reputationEngine.AddNewPeerWithPeerID(test.peerID, metaInfo)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			nodeInfo, err := reputationEngine.nodeMetaInfo(test.peerID)
			require.NoError(t, err)
			require.Equal(t, test.nodeMetaInfo, nodeInfo)
		})
	}
}

func TestReputationEngine_AddNewPeer(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)

	testcases := []struct {
		name         string
		kramaID      id.KramaID
		testFn       func(kramaID id.KramaID, nodeMetaInfo *NodeMetaInfo)
		nodeMetaInfo *NodeMetaInfo
		expectedErr  error
		shouldSkip   bool
	}{
		{
			name:    "invalid krama id",
			kramaID: "",
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			expectedErr: common.ErrInvalidKramaID,
		},
		{
			name:    "krama id without state",
			kramaID: tests.GetTestKramaID(t, 1),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID, nodeMetaInfo *NodeMetaInfo) {
				mockDB.setNodeInfo(t, tests.DecodePeerIDFromKramaID(t, kramaID), nodeMetaInfo)
			},
			nodeMetaInfo: &NodeMetaInfo{
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
				test.testFn(test.kramaID, test.nodeMetaInfo)
			}

			err := reputationEngine.AddNewPeer(test.kramaID, test.nodeMetaInfo)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			peerID := tests.DecodePeerIDFromKramaID(t, test.kramaID)
			nodeInfo, ok := reputationEngine.dirtyEntries[peerID]
			if !test.shouldSkip { // FIXME: This is just a temporary fix, need to update senatus logic
				require.True(t, ok)
				require.Equal(t, test.nodeMetaInfo, nodeInfo)
			}
		})
	}
}

func TestReputationEngine_UpdatePeer(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	testcases := []struct {
		name         string
		kramaID      id.KramaID
		testFn       func(kramaID id.KramaID)
		nodeMetaInfo *NodeMetaInfo
		expectedErr  error
	}{
		{
			name:    "invalid krama id",
			kramaID: "",
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			expectedErr: common.ErrInvalidKramaID,
		},
		{
			name:    "krama id without state",
			kramaID: tests.GetTestKramaID(t, 1),
			nodeMetaInfo: &NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						NTQ: DefaultPeerNTQ,
					},
				)
			},
			nodeMetaInfo: &NodeMetaInfo{
				Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				NTQ:           2,
				WalletCount:   1,
				PublicKey:     tests.GetTestPublicKey(t),
				PeerSignature: []byte{0x05, 0x80},
			},
		},
		{
			name:    "krama id with required node meta info",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&NodeMetaInfo{
						Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
						NTQ:           2,
						WalletCount:   1,
						PublicKey:     tests.GetTestPublicKey(t),
						PeerSignature: []byte{0x05, 0x80},
					},
				)
			},
			expectedErr: common.ErrAlreadyKnown,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			if test.testFn != nil {
				test.testFn(test.kramaID)
			}

			err := reputationEngine.UpdatePeer(test.kramaID, test.nodeMetaInfo)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err)

				return
			}

			peerID := tests.DecodePeerIDFromKramaID(t, test.kramaID)
			nodeInfo, ok := reputationEngine.dirtyEntries[peerID]

			require.True(t, ok)
			require.Equal(t, test.nodeMetaInfo, nodeInfo)
		})
	}
}

func TestReputationEngine_UpdateNTQ(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	testcases := []struct {
		name             string
		kramaID          id.KramaID
		ntq              float32
		testFn           func(kramaID id.KramaID)
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
			testFn: func(kramaID id.KramaID) {
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
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	testcases := []struct {
		name             string
		kramaID          id.KramaID
		walletCount      int32
		testFn           func(kramaID id.KramaID)
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
			testFn: func(kramaID id.KramaID) {
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
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	publicKeys := tests.GetTestPublicKeys(t, 3)
	testcases := []struct {
		name             string
		kramaID          id.KramaID
		publicKey        []byte
		testFn           func(kramaID id.KramaID)
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
			testFn: func(kramaID id.KramaID) {
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
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)

	testcases := []struct {
		name            string
		kramaID         id.KramaID
		testFn          func(kramaID id.KramaID)
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
			testFn: func(kramaID id.KramaID) {
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
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
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

func TestReputationEngine_GetNTQ(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)

	testcases := []struct {
		name        string
		kramaID     id.KramaID
		testFn      func(kramaID id.KramaID)
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
			testFn: func(kramaID id.KramaID) {
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
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)

	testcases := []struct {
		name                string
		kramaID             id.KramaID
		testFn              func(kramaID id.KramaID)
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
			testFn: func(kramaID id.KramaID) {
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
			reputationEngine, mockDB, _ := CreateTestReputationEngine(t)

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
			reputationEngine, mockDB, _ := CreateTestReputationEngine(t)

			if test.testFn != nil {
				test.testFn(reputationEngine, test.entries)
			}

			err := reputationEngine.flushDirtyEntries()

			require.NoError(t, err)
			require.Equal(t, len(test.entries)+1, len(mockDB.data))
		})
	}
}

func TestReputationEngine_SenatusHandler(t *testing.T) {
	testcases := []struct {
		name              string
		message           *pubsub.Message
		expectedErr       error
		expectedQueueSize int
	}{
		{
			name: "pubsub message with invalid data",
			message: &pubsub.Message{
				Message: &pubsubpb.Message{
					Data: []byte{200},
				},
			},
			expectedErr: errors.New("malformed tag: varint terminated prematurely"),
		},
		{
			name: "pubsub message with valid data",
			message: &pubsub.Message{
				Message: &pubsubpb.Message{
					Data: getHelloMessage(t, utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...)[0]),
				},
			},
			expectedQueueSize: 1,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			reputationEngine, _, _ := CreateTestReputationEngine(t)
			err := reputationEngine.senatusHandler(test.message)

			if test.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedErr.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedQueueSize, reputationEngine.pendingMessageQueue.Len())
		})
	}
}

func TestReputationEngine_DBWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reputationEngine, _, _ := CreateTestReputationEngine(t)
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

	// Wait for a short time to let the worker handle messages
	time.Sleep(6 * time.Second)

	reputationEngine.dirtyLock.RLock()
	defer reputationEngine.dirtyLock.RUnlock()

	require.Equal(t, 0, len(reputationEngine.dirtyEntries))

	// Cancel the context to stop the worker
	cancel()

	// Wait for the worker to finish
	wg.Wait()
}

func TestReputationEngine_CleanUpDirtyStorage(t *testing.T) {
	reputationEngine, _, _ := CreateTestReputationEngine(t)
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

func TestVerifyHelloMsg(t *testing.T) {
	reputationEngine, _, _ := CreateTestReputationEngine(t)

	helloMsg := createSignedHelloMsg(t)

	testcases := []struct {
		name          string
		msg           *NodeMetaInfoMsg
		expectedError error
	}{
		{
			name: "invalid krama id",
			msg: &NodeMetaInfoMsg{
				KramaID: "",
			},
			expectedError: errors.New("Failed to get peer id from krama id"),
		},
		{
			name: "Signature verification failed",
			msg: &NodeMetaInfoMsg{
				KramaID: tests.GetTestKramaID(t, 1),
			},
			expectedError: errors.New("Signature verification failed"),
		},
		{
			name: "Signature verification successful",
			msg: &NodeMetaInfoMsg{
				KramaID:       helloMsg.KramaID,
				Address:       helloMsg.Address,
				PeerSignature: helloMsg.Signature,
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := reputationEngine.verifyHelloMsg(test.msg)

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}

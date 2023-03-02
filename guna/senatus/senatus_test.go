package senatus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
	"github.com/stretchr/testify/require"
)

func TestReputationEngine_NodeMetaInfo(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	testcases := []struct {
		name         string
		peerID       peer.ID
		nodeMetaInfo *gtypes.NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "node meta info found in cache",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo) {
				reputationEngine.cache.Add(dhruva.NtqCacheKey(peerID), nodeMetaInfo)
			},
		},
		{
			name:   "node meta info found in dirty entries",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: -1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo) {
				reputationEngine.dirtyEntries[peerID] = nodeMetaInfo
			},
		},
		{
			name:   "node meta info found in db",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "node meta info not found",
			peerID:      tests.GetTestPeerID(t),
			expectedErr: types.ErrKramaIDNotFound,
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

func TestReputationEngine_AddNewPeer(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	testcases := []struct {
		name         string
		kramaID      id.KramaID
		testFn       func(kramaID id.KramaID, nodeMetaInfo *gtypes.NodeMetaInfo)
		nodeMetaInfo *gtypes.NodeMetaInfo
		expectedErr  error
	}{
		{
			name:    "invalid krama id",
			kramaID: "",
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			expectedErr: types.ErrInvalidKramaID,
		},
		{
			name:    "krama id without state",
			kramaID: tests.GetTestKramaID(t, 1),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 1,
			},
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID, nodeMetaInfo *gtypes.NodeMetaInfo) {
				mockDB.setNodeInfo(t, tests.DecodePeerIDFromKramaID(t, kramaID), nodeMetaInfo)
			},
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			expectedErr: types.ErrAlreadyKnown,
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

			require.True(t, ok)
			require.Equal(t, test.nodeMetaInfo, nodeInfo)
		})
	}
}

func TestReputationEngine_UpdatePeer(t *testing.T) {
	reputationEngine, mockDB, _ := CreateTestReputationEngine(t)
	testcases := []struct {
		name         string
		kramaID      id.KramaID
		testFn       func(kramaID id.KramaID)
		nodeMetaInfo *gtypes.NodeMetaInfo
		expectedErr  error
	}{
		{
			name:    "invalid krama id",
			kramaID: "",
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         DefaultPeerNTQ,
				WalletCount: 0,
			},
			expectedErr: types.ErrInvalidKramaID,
		},
		{
			name:    "krama id without state",
			kramaID: tests.GetTestKramaID(t, 1),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
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
					&gtypes.NodeMetaInfo{
						NTQ: DefaultPeerNTQ,
					},
				)
			},
			nodeMetaInfo: &gtypes.NodeMetaInfo{
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
					&gtypes.NodeMetaInfo{
						Addrs:         utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
						NTQ:           2,
						WalletCount:   1,
						PublicKey:     tests.GetTestPublicKey(t),
						PeerSignature: []byte{0x05, 0x80},
					},
				)
			},
			expectedErr: types.ErrAlreadyKnown,
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
		expectedNodeInfo *gtypes.NodeMetaInfo
	}{
		{
			name:        "invalid krama id",
			kramaID:     "",
			ntq:         DefaultPeerNTQ,
			expectedErr: types.ErrInvalidKramaID,
		},
		{
			name:    "krama id without state",
			kramaID: tests.GetTestKramaID(t, 1),
			ntq:     DefaultPeerNTQ,
			expectedNodeInfo: &gtypes.NodeMetaInfo{
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
					&gtypes.NodeMetaInfo{
						NTQ:         DefaultPeerNTQ,
						WalletCount: 1,
					},
				)
			},
			expectedNodeInfo: &gtypes.NodeMetaInfo{
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
		expectedNodeInfo *gtypes.NodeMetaInfo
	}{
		{
			name:        "invalid krama id",
			kramaID:     "",
			walletCount: 1,
			expectedErr: types.ErrInvalidKramaID,
		},
		{
			name:        "krama id without state",
			kramaID:     tests.GetTestKramaID(t, 1),
			walletCount: -1,
			expectedNodeInfo: &gtypes.NodeMetaInfo{
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
					&gtypes.NodeMetaInfo{
						NTQ:         1,
						WalletCount: -1,
					},
				)
			},
			walletCount: 1,
			expectedNodeInfo: &gtypes.NodeMetaInfo{
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
		expectedNodeInfo *gtypes.NodeMetaInfo
	}{
		{
			name:        "invalid krama id",
			kramaID:     "",
			publicKey:   publicKeys[0],
			expectedErr: types.ErrInvalidKramaID,
		},
		{
			name:      "krama id without state",
			kramaID:   tests.GetTestKramaID(t, 1),
			publicKey: publicKeys[1],
			expectedNodeInfo: &gtypes.NodeMetaInfo{
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
					&gtypes.NodeMetaInfo{
						NTQ:         1,
						WalletCount: -1,
					},
				)
			},
			expectedNodeInfo: &gtypes.NodeMetaInfo{
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
			expectedErr: types.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&gtypes.NodeMetaInfo{
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
		nodeMetaInfo *gtypes.NodeMetaInfo
		testFn       func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo)
		expectedErr  error
	}{
		{
			name:   "peer id with state",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				Addrs:       utils.MultiAddrToString(tests.GetListenAddresses(t, 1)...),
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo) {
				mockDB.setNodeInfo(t, peerID, nodeMetaInfo)
			},
		},
		{
			name:        "peer id without state",
			peerID:      tests.GetTestPeerID(t),
			expectedErr: types.ErrKramaIDNotFound,
		},
		{
			name:   "peer id with state and without address",
			peerID: tests.GetTestPeerID(t),
			nodeMetaInfo: &gtypes.NodeMetaInfo{
				NTQ:         1,
				WalletCount: 1,
			},
			testFn: func(peerID peer.ID, nodeMetaInfo *gtypes.NodeMetaInfo) {
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
			expectedErr: types.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&gtypes.NodeMetaInfo{
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
			expectedErr: types.ErrKramaIDNotFound,
		},
		{
			name:    "krama id with state",
			kramaID: tests.GetTestKramaID(t, 1),
			testFn: func(kramaID id.KramaID) {
				mockDB.setNodeInfo(
					t,
					tests.DecodePeerIDFromKramaID(t, kramaID),
					&gtypes.NodeMetaInfo{
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
		entries map[peer.ID]*gtypes.NodeMetaInfo
		testFn  func(mockDB *MockDB, entries map[peer.ID]*gtypes.NodeMetaInfo)
	}{
		{
			name: "db with multiple entries",
			entries: map[peer.ID]*gtypes.NodeMetaInfo{
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
			testFn: func(mockDB *MockDB, entries map[peer.ID]*gtypes.NodeMetaInfo) {
				for key, entry := range entries {
					mockDB.setNodeInfo(t, key, entry)
				}
			},
		},
		{
			name:    "db without entries",
			entries: map[peer.ID]*gtypes.NodeMetaInfo{},
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
		entries     map[peer.ID]*gtypes.NodeMetaInfo
		testFn      func(reputationEngine *ReputationEngine, entries map[peer.ID]*gtypes.NodeMetaInfo)
		expectedErr error
	}{
		{
			name: "dirty entries map with multiple entries",
			entries: map[peer.ID]*gtypes.NodeMetaInfo{
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
			testFn: func(reputationEngine *ReputationEngine, entries map[peer.ID]*gtypes.NodeMetaInfo) {
				for key, entry := range entries {
					reputationEngine.dirtyEntries[key] = entry
				}
			},
		},
		{
			name:    "empty dirty entries map",
			entries: map[peer.ID]*gtypes.NodeMetaInfo{},
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
		reputationEngine.dirtyEntries[tests.GetTestPeerID(t)] = &gtypes.NodeMetaInfo{
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
		entries map[peer.ID]*gtypes.NodeMetaInfo
		testFn  func(entries map[peer.ID]*gtypes.NodeMetaInfo)
	}{
		{
			name: "dirty entries map with single entry",
			entries: map[peer.ID]*gtypes.NodeMetaInfo{
				tests.GetTestPeerID(t): {
					NTQ:         DefaultPeerNTQ,
					WalletCount: -1,
				},
			},
			testFn: func(entries map[peer.ID]*gtypes.NodeMetaInfo) {
				for key, entry := range entries {
					reputationEngine.dirtyEntries[key] = entry
				}
			},
		},
		{
			name: "dirty entries map with multiple entries",
			entries: map[peer.ID]*gtypes.NodeMetaInfo{
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
			testFn: func(entries map[peer.ID]*gtypes.NodeMetaInfo) {
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

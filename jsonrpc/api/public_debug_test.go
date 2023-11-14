package api

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/hexutil"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-moi/senatus"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Debug API Testcases

func TestPublicDebugAPI_DBGet(t *testing.T) {
	db := NewMockDatabase(t)
	debugAPI := NewPublicDebugAPI(db, nil)
	key := tests.RandomHash(t)
	value := tests.RandomHash(t)

	db.setDBEntry(key.Bytes(), value.Bytes())

	testcases := []struct {
		name          string
		args          rpcargs.DebugArgs
		expectedValue string
		expectedError error
	}{
		{
			name: "The key does not exist in the database",
			args: rpcargs.DebugArgs{
				Key: tests.RandomHash(t).String(),
			},
			expectedError: common.ErrKeyNotFound,
		},
		{
			name: "Returns the raw value of a key stored in the database",
			args: rpcargs.DebugArgs{
				Key: key.String(),
			},
			expectedValue: value.String(),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			value, err := debugAPI.DBGet(&test.args)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestPublicDebugAPI_GetNodeMetaInfo(t *testing.T) {
	peerIDs, entries := createNodeMetaInfo(t, 5)

	testcases := []struct {
		name            string
		args            rpcargs.NodeMetaInfoArgs
		entries         map[peer.ID]*senatus.NodeMetaInfo
		expectedEntries []peer.ID
		expectedError   error
	}{
		{
			name:            "Returns all the node meta info stored in the database",
			args:            rpcargs.NodeMetaInfoArgs{},
			entries:         entries,
			expectedEntries: peerIDs,
		},
		{
			name:            "Node meta info doesn't exist in the database for any node",
			args:            rpcargs.NodeMetaInfoArgs{},
			entries:         make(map[peer.ID]*senatus.NodeMetaInfo),
			expectedEntries: []peer.ID{},
		},
		{
			name: "Return node meta info for the specific peer id",
			args: rpcargs.NodeMetaInfoArgs{
				PeerID: peerIDs[1].String(),
			},
			entries:         entries,
			expectedEntries: []peer.ID{peerIDs[1]},
		},
		{
			name: "Return node meta info for the specific krama id",
			args: rpcargs.NodeMetaInfoArgs{
				KramaID: entries[peerIDs[2]].KramaID,
			},
			entries:         entries,
			expectedEntries: []peer.ID{peerIDs[2]},
		},
		{
			name: "Return error if the queried peer id's node meta info doesn't exists",
			args: rpcargs.NodeMetaInfoArgs{
				PeerID: tests.GetTestPeerID(t).String(),
			},
			entries:       entries,
			expectedError: common.ErrKeyNotFound,
		},
		{
			name: "Return error if the queried krama id's node meta info doesn't exists",
			args: rpcargs.NodeMetaInfoArgs{
				KramaID: tests.GetTestKramaID(t, 2),
			},
			entries:       entries,
			expectedError: common.ErrKeyNotFound,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			db := NewMockDatabase(t)
			debugAPI := NewPublicDebugAPI(db, nil)

			if test.entries != nil {
				db.setNodeMetaInfo(t, test.entries)
			}

			nodeMetaInfo, err := debugAPI.GetNodeMetaInfo(&test.args)

			if test.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, err, test.expectedError)

				return
			}

			require.NoError(t, err)
			require.Equal(t, len(test.expectedEntries), len(nodeMetaInfo))

			for _, peerID := range test.expectedEntries {
				nodeInfo := nodeMetaInfo[peerID.String()]

				require.NotNil(t, nodeInfo)
				require.Equal(t, test.entries[peerID].KramaID, nodeInfo["krama_id"])
				require.Equal(t, hexutil.Uint64(test.entries[peerID].RTT), nodeInfo["rtt"])
				require.Equal(t, hexutil.Uint(test.entries[peerID].WalletCount), nodeInfo["wallet_count"])
			}
		})
	}
}

func TestPublicDebugAPI_GetAccounts(t *testing.T) {
	addressList := tests.GetAddresses(t, 5)

	testcases := []struct {
		name         string
		setAddressFn func(db *MockDatabase)
		expectedList []common.Address
	}{
		{
			name:         "Should return an empty list if no accounts are present",
			expectedList: make([]common.Address, 0),
		},
		{
			name: "Returns a list of address of the accounts",
			setAddressFn: func(db *MockDatabase) {
				db.setList(t, addressList)
			},
			expectedList: addressList,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			db := NewMockDatabase(t)
			debugAPI := NewPublicDebugAPI(db, nil)

			if test.setAddressFn != nil {
				test.setAddressFn(db)
			}

			fetchedList, err := debugAPI.GetAccounts()

			require.NoError(t, err)
			require.ElementsMatch(t, test.expectedList, fetchedList)
		})
	}
}

func TestPublicDebugAPI_GetConns(t *testing.T) {
	mn := NewMockNetwork(t)
	debugAPI := NewPublicDebugAPI(nil, mn)

	conns := createConns(t, 3, 3)
	mn.setConns(conns)
	mn.setInboundConnCount(1)
	mn.setOutboundConnCount(2)
	mn.setSubscribedTopics(map[string]int{"topic1": 1, "topic2": 2})

	testcases := []struct {
		name                      string
		expectedConns             []network.Conn
		expectedInboundConnCount  int64
		expectedOutboundConnCount int64
		expectedPubSubTopics      map[string]int
	}{
		{
			name:                      "fetch connections",
			expectedConns:             conns,
			expectedInboundConnCount:  1,
			expectedOutboundConnCount: 2,
			expectedPubSubTopics:      map[string]int{"topic1": 1, "topic2": 2},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connResp := debugAPI.GetConnections()

			for i, expectedConn := range test.expectedConns {
				require.Equal(t, expectedConn.RemotePeer().String(), connResp.Conns[i].PeerID)
				require.Equal(t, len(expectedConn.GetStreams()), len(connResp.Conns[i].Streams))
			}

			require.Equal(t, test.expectedInboundConnCount, connResp.InboundConnCount)
			require.Equal(t, test.expectedOutboundConnCount, connResp.OutboundConnCount)
			require.Equal(t, test.expectedPubSubTopics, connResp.ActivePubSubTopics)
		})
	}
}

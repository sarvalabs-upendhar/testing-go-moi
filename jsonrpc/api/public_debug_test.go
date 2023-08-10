package api

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/network"

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

	db.setDBEntry(key.Bytes())

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
			expectedValue: key.String(),
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
			require.Equal(t, value, test.expectedValue)
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

	testcases := []struct {
		name          string
		expectedConns []network.Conn
	}{
		{
			name:          "fetch connections",
			expectedConns: conns,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			conns := debugAPI.GetConnections()

			for i, expectedConn := range test.expectedConns {
				require.Equal(t, expectedConn.RemotePeer().String(), conns[i].PeerID)
				require.Equal(t, uint64(len(expectedConn.GetStreams())), conns[i].StreamCount)
			}
		})
	}
}

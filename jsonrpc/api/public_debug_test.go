package api

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Debug API Testcases

func TestPublicDebugAPI_DBGet(t *testing.T) {
	db := NewMockDatabase(t)
	debugAPI := NewPublicDebugAPI(db)
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
		t.Run(test.name, func(testing *testing.T) {
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
	db := NewMockDatabase(t)
	debugAPI := NewPublicDebugAPI(db)
	addressList := tests.GetAddresses(t, 5)

	testcases := []struct {
		name         string
		expectedList []common.Address
		setAddressFn func()
	}{
		{
			name:         "Should return an empty list if no accounts are present",
			expectedList: make([]common.Address, 0),
		},
		{
			name: "Returns a list of address of the accounts",
			setAddressFn: func() {
				db.setList(t, addressList)
			},
			expectedList: addressList,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			if test.setAddressFn != nil {
				test.setAddressFn()
			}

			fetchedList, err := debugAPI.GetAccounts()

			require.NoError(t, err)
			require.ElementsMatch(t, test.expectedList, fetchedList)
		})
	}
}

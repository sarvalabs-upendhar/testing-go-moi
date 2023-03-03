package api

import (
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/require"
)

// Debug API Testcases

func TestPublicDebugAPI_DBGet(t *testing.T) {
	db := NewMockDatabase(t)
	debugAPI := NewPublicDebugAPI(db)
	key := tests.RandomHash(t)

	db.setDBEntry(key.Bytes())

	testcases := []struct {
		name          string
		args          DebugArgs
		expectedValue string
		expectedError error
	}{
		{
			name: "The key does not exist in the database",
			args: DebugArgs{
				Key: tests.RandomHash(t).String(),
			},
			expectedError: types.ErrKeyNotFound,
		},
		{
			name: "Returns the raw value of a key stored in the database",
			args: DebugArgs{
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

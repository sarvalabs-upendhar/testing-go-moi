package utils

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestValidateHash(t *testing.T) {
	testcases := []struct {
		name          string
		hash          string
		expectedHash  string
		expectedError error
	}{
		{
			name:         "hash with 0x",
			hash:         "0xa6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedHash: "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:         "hash with out 0x should pass",
			hash:         "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedHash: "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:          "hash with length less than 64 should fail",
			hash:          "0xa6ba9853f131679d0da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedError: common.ErrInvalidHash,
		},
		{
			name:          "hash with length greater than 64 should fail",
			hash:          "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384z",
			expectedError: common.ErrInvalidHash,
		},
		{
			name:         "hash with capitals should pass",
			hash:         "0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedHash: "A6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:          "hash with [g-z] should fail ",
			hash:          "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f938g",
			expectedError: common.ErrInvalidHash,
		},
		{
			name:          "empty hash",
			hash:          "",
			expectedError: common.ErrInvalidHash,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateHash(test.hash)
			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.Equal(t, test.expectedHash, result)
		})
	}
}

// TODO: move this to types package when implementing its tests
func TestNewAccountAddress(t *testing.T) {
	randID := tests.RandomIdentifier(t)
	randKeyID := rand.Uint64()
	randSequenceID := rand.Uint64()

	rawBytes := make([]byte, 48)
	binary.BigEndian.PutUint64(rawBytes[:8], randSequenceID)
	binary.BigEndian.PutUint64(rawBytes[8:16], randKeyID)
	copy(rawBytes[16:], randID.Bytes())

	generatedAddress := common.NewAccountID(common.Sender{
		ID:         randID,
		KeyID:      randKeyID,
		SequenceID: randSequenceID,
	})

	require.Equal(t, generatedAddress, common.NewAccounIDFromBytes(common.GetHash(rawBytes).Bytes()))
}

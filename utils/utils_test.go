package utils

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"

	"github.com/stretchr/testify/require"
)

func TestValidateAddress(t *testing.T) {
	testcases := []struct {
		name            string
		address         string
		expectedAddress string
		expectedError   error
	}{
		{
			name:            "address with 0x should pass",
			address:         "0xa6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedAddress: "0xa6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:            "address with out 0x should pass",
			address:         "a6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedAddress: "0xa6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:          "address with length less than 64 should fail",
			address:       "0xa6ba9853f131679d0da0f033516a3efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:          "address with length greater than 64 should fail",
			address:       "a6ba9853f131679d00da0f033516a3efe9cd53c3d54e1f9a6e60e9077e9f9384z",
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:            "address with capitals should pass",
			address:         "0xA6Ba9853f131679d00da0f033416a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedAddress: "0xa6ba9853f131679d00da0f033416a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:          "address with [g-z] should fail ",
			address:       "a6ba9853f131679d00da0f033516a2afe9cd53c3d54e1f9a6e60e9077e9f938g",
			expectedError: types.ErrInvalidAddress,
		},
		{
			name:          "empty address",
			address:       "",
			expectedError: types.ErrInvalidAddress,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateAddress(test.address)
			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.Equal(t, test.expectedAddress, result.Hex())
		})
	}
}

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
			expectedError: types.ErrInvalidHash,
		},
		{
			name:          "hash with length greater than 64 should fail",
			hash:          "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384z",
			expectedError: types.ErrInvalidHash,
		},
		{
			name:         "hash with capitals should pass",
			hash:         "0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedHash: "A6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
		},
		{
			name:          "hash with [g-z] should fail ",
			hash:          "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f938g",
			expectedError: types.ErrInvalidHash,
		},
		{
			name:          "empty hash",
			hash:          "",
			expectedError: types.ErrInvalidHash,
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

func TestValidateLogicID(t *testing.T) {
	testcases := []struct {
		name            string
		logicID         string
		expectedLogicID types.LogicID
		expectedError   error
	}{
		{
			name:    "logic id with 0x",
			logicID: "0x06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdef",
			expectedLogicID: getLogicID(
				t,
				"0x06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdef"),
		},
		{
			name:    "logic id with out 0x should pass",
			logicID: "06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdef",
			expectedLogicID: getLogicID(
				t,
				"06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdef",
			),
		},
		{
			name:          "logic id with length less than 70 should fail",
			logicID:       "06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			expectedError: types.ErrInvalidLogicID,
		},
		{
			name:          "logic id with length greater than 70 should fail",
			logicID:       "06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdefaa",
			expectedError: types.ErrInvalidLogicID,
		},
		{
			name:    "logic id with capitals should pass",
			logicID: "06BA9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdef",
			expectedLogicID: getLogicID(
				t,
				"06BA9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdef",
			),
		},
		{
			name:          "logic id with [g-z] should fail ",
			logicID:       "06ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcdgz",
			expectedError: errors.New("encoding/hex: invalid byte: U+0067 'g'"),
		},
		{
			name:          "empty logic id",
			logicID:       "",
			expectedError: types.ErrInvalidLogicID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateLogicID(test.logicID)
			if test.expectedError != nil {
				require.EqualError(t, err, test.expectedError.Error())

				return
			}

			require.Equal(t, test.expectedLogicID, result)
		})
	}
}

func TestValidateAssetID(t *testing.T) {
	testcases := []struct {
		name            string
		assetID         string
		expectedAssetID string
		expectedError   error
	}{
		{
			name:            "asset id with 0x should pass",
			assetID:         "0xa6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			expectedAssetID: "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
		},
		{
			name:            "asset id with out 0x should pass",
			assetID:         "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			expectedAssetID: "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
		},
		{
			name:            "asset id with length less than 68 should fail",
			assetID:         "0xa6ba9853f131679d0da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			expectedAssetID: "",
			expectedError:   types.ErrInvalidAssetID,
		},
		{
			name:            "asset id with length greater than 68 should fail",
			assetID:         "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384zabcdef",
			expectedAssetID: "",
			expectedError:   types.ErrInvalidAssetID,
		},
		{
			name:            "asset id with capitals should pass",
			assetID:         "0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			expectedAssetID: "A6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
		},
		{
			name:            "asset id with [g-z] should fail ",
			assetID:         "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f938gabcd",
			expectedAssetID: "",
			expectedError:   types.ErrInvalidAssetID,
		},
		{
			name:            "empty asset id",
			assetID:         "",
			expectedAssetID: "bcd",
			expectedError:   types.ErrInvalidAssetID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateAssetID(test.assetID)
			if test.expectedError != nil {
				require.EqualError(t, test.expectedError, err.Error())

				return
			}

			require.Equal(t, test.expectedAssetID, string(result))
		})
	}
}

// TODO: move this to types package when implementing its tests
func TestNewAccountAddress(t *testing.T) {
	randNonce := rand.Uint64()
	randAddress := tests.RandomAddress(t)

	rawBytes := make([]byte, 40)
	binary.BigEndian.PutUint64(rawBytes, randNonce)
	copy(rawBytes[8:], randAddress.Bytes())

	generatedAddress := types.NewAccountAddress(randNonce, randAddress)

	require.Equal(t, generatedAddress.Bytes(), types.GetHash(rawBytes).Bytes())
}

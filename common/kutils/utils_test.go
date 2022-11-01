package kutils

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name            string
		address         string
		expectedAddress string
		isErrorExpected bool
	}{
		{
			"address with 0x should pass",
			"0xa6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"a6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			false,
		},
		{
			"address with out 0x should pass",
			"a6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"a6ba9853f131679e00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			false,
		},
		{
			"address with length less than 64 should fail",
			"0xa6ba9853f131679d0da0f033516a3efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"",
			true,
		},
		{
			"address with length greater than 64 should fail",
			"a6ba9853f131679d00da0f033516a3efe9cd53c3d54e1f9a6e60e9077e9f9384z",
			"",
			true,
		},
		{
			"address with capitals should pass",
			"0xA6Ba9853f131679d00da0f033416a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"A6Ba9853f131679d00da0f033416a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			false,
		},
		{
			"address with [g-z] should fail ",
			"a6ba9853f131679d00da0f033516a2afe9cd53c3d54e1f9a6e60e9077e9f938g",
			"",
			true,
		},
		{
			"empty address",
			"",
			"",
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateAddress(test.address)
			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedAddress, result)
			}
		})
	}
}

func TestValidateHash(t *testing.T) {
	tests := []struct {
		name            string
		hash            string
		expectedHash    string
		isErrorExpected bool
	}{
		{
			"hash with 0x",
			"0xa6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			false,
		},
		{
			"hash with out 0x should pass",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			false,
		},
		{
			"hash with length less than 64 should fail",
			"0xa6ba9853f131679d0da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"",
			true,
		},
		{
			"hash with length greater than 64 should fail",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384z",
			"",
			true,
		},
		{
			"hash with capitals should pass",
			"0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"A6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			false,
		},
		{
			"hash with [g-z] should fail ",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f938g",
			"",
			true,
		},
		{
			"empty hash",
			"",
			"bcd",
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateHash(test.hash)
			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedHash, result)
			}
		})
	}
}

func TestValidateAssetID(t *testing.T) {
	tests := []struct {
		name            string
		assetID         string
		expectedAssetID string
		isErrorExpected bool
	}{
		{
			"asset id with 0x should pass",
			"0xa6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			false,
		},
		{
			"asset id with out 0x should pass",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			false,
		},
		{
			"asset id with length less than 68 should fail",
			"0xa6ba9853f131679d0da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			"",
			true,
		},
		{
			"asset id with length greater than 68 should fail",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384zabcdef",
			"",
			true,
		},
		{
			"asset id with capitals should pass",
			"0xA6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			"A6Ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384abcd",
			false,
		},
		{
			"asset id with [g-z] should fail ",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f938gabcd",
			"",
			true,
		},
		{
			"empty asset id",
			"",
			"bcd",
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateAssetID(test.assetID)

			if test.isErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedAssetID, result)
			}
		})
	}
}

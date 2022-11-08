package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAddressHeightKey(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		height      uint64
		expectedkey []byte
	}{
		{
			"Valid address and height",
			"a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			1,
			Hex2Bytes("68a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f93840100000000000000"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := GetAddressHeightKey(HexToAddress(test.address), test.height)

			require.Equal(t, test.expectedkey, result)
		})
	}
}

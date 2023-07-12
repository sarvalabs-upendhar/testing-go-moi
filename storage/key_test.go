package storage

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
)

func TestGetAddressHeightKey(t *testing.T) {
	tests := []struct {
		name    string
		address string
		height  uint64
	}{
		{
			name:    "Valid address and height",
			address: "a6ba9853f131679d00da0f033516a2efe9cd53c3d54e1f9a6e60e9077e9f9384",
			height:  1666,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := tesseractHeightKey(common.HexToAddress(test.address), test.height)
			parsedAddress := result[:32]
			parsedPrefix := result[32:33][0]
			parsedHeight := result[33:]

			require.Equal(t, common.Hex2Bytes(test.address), parsedAddress)
			require.Equal(t, TesseractHeight.Byte(), parsedPrefix)
			require.Equal(t, test.height, binary.LittleEndian.Uint64(parsedHeight))
		})
	}
}

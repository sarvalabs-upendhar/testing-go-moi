package storage

import (
	"encoding/binary"
	"testing"

	"github.com/sarvalabs/go-moi/common/identifiers"

	tests "github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestGetIdentifierHeightKey(t *testing.T) {
	tests := []struct {
		name   string
		key    identifiers.Identifier
		height uint64
	}{
		{
			name:   "Valid identifier and height",
			key:    tests.RandomIdentifier(t),
			height: 1666,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := tesseractHeightKey(test.key, test.height)

			parsedIdentifier := result[:32]
			parsedPrefix := result[32:33][0]
			parsedHeight := result[33:]

			require.Equal(t, test.key[:], parsedIdentifier)
			require.Equal(t, TesseractHeight.Byte(), parsedPrefix)
			require.Equal(t, test.height, binary.LittleEndian.Uint64(parsedHeight))
		})
	}
}

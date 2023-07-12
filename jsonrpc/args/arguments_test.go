package args

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
)

func TestSort(t *testing.T) {
	parts := RPCTesseractParts{}

	for i := 0; i < 3; i++ {
		parts = append(
			parts,
			RPCTesseractPart{
				Address: tests.RandomAddress(t),
			},
		)
	}

	testcases := []struct {
		name  string
		parts RPCTesseractParts
	}{
		{
			name:  "sort tesseract parts",
			parts: parts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			test.parts.Sort()

			CheckIfPartsSorted(t, test.parts)
		})
	}
}

func CheckIfPartsSorted(t *testing.T, parts RPCTesseractParts) {
	t.Helper()

	for i := 1; i < len(parts); i++ {
		require.True(t, parts[i-1].Address.Hex() < parts[i].Address.Hex())
	}
}

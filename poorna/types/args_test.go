package types_test

import (
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
)

func TestSort(t *testing.T) {
	parts := ptypes.RPCTesseractParts{}

	for i := 0; i < 3; i++ {
		parts = append(
			parts,
			ptypes.RPCTesseractPart{
				Address: tests.RandomAddress(t),
			},
		)
	}

	testcases := []struct {
		name  string
		parts ptypes.RPCTesseractParts
	}{
		{
			name:  "sort tesseract parts",
			parts: parts,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			test.parts.Sort()

			tests.CheckIfPartsSorted(t, test.parts)
		})
	}
}

package types_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func TestCopyReceipt(t *testing.T) {
	testcases := []struct {
		name    string
		receipt *types.Receipt
	}{
		{
			name:    "copy receipt",
			receipt: createReceiptWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipt := test.receipt
			copiedReceipt := test.receipt.Copy()

			require.Equal(t, expectedReceipt, copiedReceipt)
			require.NotEqual(t,
				reflect.ValueOf(test.receipt.StateHashes).Pointer(),
				reflect.ValueOf(copiedReceipt.StateHashes).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(test.receipt.ContextHashes).Pointer(),
				reflect.ValueOf(copiedReceipt.ContextHashes).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(test.receipt.ExtraData).Pointer(),
				reflect.ValueOf(copiedReceipt.ExtraData).Pointer(),
			)
		})
	}
}

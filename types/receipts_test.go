package types_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func TestCopyReceipt(t *testing.T) {
	receiptWithNilExtraData := createReceiptWithTestData(t)
	receiptWithNilExtraData.ExtraData = nil

	testcases := []struct {
		name           string
		receipt        *types.Receipt
		isExtraDataNil bool
	}{
		{
			name:    "copy receipt",
			receipt: createReceiptWithTestData(t),
		},
		{
			name:           "copy receipt with nil extra data",
			receipt:        receiptWithNilExtraData,
			isExtraDataNil: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipt := test.receipt
			copiedReceipt := test.receipt.Copy()

			require.Equal(t, expectedReceipt, copiedReceipt)

			require.NotEqual(t,
				reflect.ValueOf(expectedReceipt.StateHashes).Pointer(),
				reflect.ValueOf(copiedReceipt.StateHashes).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(expectedReceipt.ContextHashes).Pointer(),
				reflect.ValueOf(copiedReceipt.ContextHashes).Pointer(),
			)

			if !test.isExtraDataNil {
				require.NotEqual(t,
					reflect.ValueOf(expectedReceipt.ExtraData).Pointer(),
					reflect.ValueOf(copiedReceipt.ExtraData).Pointer(),
				)
			}
		})
	}
}

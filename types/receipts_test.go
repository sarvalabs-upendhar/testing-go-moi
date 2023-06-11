package types_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
)

func TestCopyHashes(t *testing.T) {
	hashes := make(types.ReceiptAccHashes, 0)

	for i := 0; i < 3; i++ {
		hashes[tests.RandomAddress(t)] = &types.Hashes{
			StateHash:   tests.RandomHash(t),
			ContextHash: tests.RandomHash(t),
		}
	}

	testcases := []struct {
		name    string
		hashmap types.ReceiptAccHashes
	}{
		{
			name:    "Hashmap copied successfully",
			hashmap: hashes,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedHashMap := test.hashmap

			hashmapCopy := test.hashmap.Copy()

			require.Equal(t, expectedHashMap, hashmapCopy)
		})
	}
}

func TestCopyReceipt(t *testing.T) {
	receiptWithNilExtraData := tests.CreateReceiptWithTestData(t)
	receiptWithNilExtraData.ExtraData = nil
	receiptWithNilExtraData.FuelUsed = nil

	testcases := []struct {
		name           string
		receipt        *types.Receipt
		isExtraDataNil bool
	}{
		{
			name:    "copy receipt",
			receipt: tests.CreateReceiptWithTestData(t),
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
				reflect.ValueOf(expectedReceipt.Hashes).Pointer(),
				reflect.ValueOf(copiedReceipt.Hashes).Pointer(),
			)

			if test.receipt.FuelUsed != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedReceipt.FuelUsed).Pointer(),
					reflect.ValueOf(copiedReceipt.FuelUsed).Pointer(),
				)
			}

			if !test.isExtraDataNil {
				require.NotEqual(t,
					reflect.ValueOf(expectedReceipt.ExtraData).Pointer(),
					reflect.ValueOf(copiedReceipt.ExtraData).Pointer(),
				)
			}
		})
	}
}

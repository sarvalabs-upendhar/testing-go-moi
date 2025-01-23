package common_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestCopyReceipt(t *testing.T) {
	receiptWithNilData := tests.CreateReceiptWithTestData(t)
	receiptWithNilData.IxOps = nil

	testcases := []struct {
		name           string
		receipt        *common.Receipt
		isExtraDataNil bool
	}{
		{
			name:    "copy receipt",
			receipt: tests.CreateReceiptWithTestData(t),
		},
		{
			name:    "copy receipt with empty ops",
			receipt: receiptWithNilData,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipt := test.receipt
			copiedReceipt := test.receipt.Copy()

			require.Equal(t, expectedReceipt, copiedReceipt)

			if test.receipt.IxOps != nil {
				require.Len(t, copiedReceipt.IxOps, len(expectedReceipt.IxOps))

				for i := 0; i < len(expectedReceipt.IxOps); i++ {
					require.NotEqual(t,
						reflect.ValueOf(expectedReceipt.IxOps[i]),
						reflect.ValueOf(copiedReceipt.IxOps[i]),
					)
				}
			}
		})
	}
}

func TestCopyLog(t *testing.T) {
	testcases := []struct {
		name string
		log  common.Log
	}{
		{
			name: "copy log",
			log: common.Log{
				ID:      tests.RandomIdentifier(t),
				LogicID: tests.GetLogicID(t, tests.RandomIdentifier(t)),
				Topics:  tests.GetHashes(t, 1),
				Data:    []byte{1},
			},
		},
		{
			name: "empty addresses,topics,data",
			log: common.Log{
				LogicID: tests.GetLogicID(t, tests.RandomIdentifier(t)),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedLog := test.log
			copiedLog := test.log.Copy()

			// Add assertions to check individual fields of the copied log
			require.Equal(t, expectedLog, copiedLog)
			require.Equal(t, expectedLog.ID, copiedLog.ID)

			if len(expectedLog.Topics) > 0 {
				// Compare Topics pointers
				require.NotEqual(t,
					reflect.ValueOf(expectedLog.Topics).Pointer(),
					reflect.ValueOf(copiedLog.Topics).Pointer(),
				)
			}

			if len(expectedLog.Data) > 0 {
				// Compare Data slices
				if expectedLog.Data != nil {
					require.NotEqual(t,
						reflect.ValueOf(expectedLog.Data).Pointer(),
						reflect.ValueOf(copiedLog.Data).Pointer(),
					)
				}
			}
		})
	}
}

func TestCopyReceipts(t *testing.T) {
	hash := tests.RandomHash(t)

	testcases := []struct {
		name     string
		receipts common.Receipts
	}{
		{
			name:     "copy receipts",
			receipts: tests.CreateReceiptsWithTestData(t, hash),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipts := test.receipts

			receipts := test.receipts.Copy()

			require.Equal(t, expectedReceipts, receipts)

			require.NotEqual(t, reflect.ValueOf(test.receipts).Pointer(), reflect.ValueOf(receipts).Pointer())
			require.NotEqual(t,
				reflect.ValueOf(test.receipts[hash].IxOps).Pointer(),
				reflect.ValueOf(receipts[hash].IxOps).Pointer(),
			)
		})
	}
}

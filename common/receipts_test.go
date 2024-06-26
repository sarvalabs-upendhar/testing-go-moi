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
	receiptWithNilData.ExtraData = nil
	receiptWithNilData.Logs = nil

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
			name:           "copy receipt with nil extra data and logs",
			receipt:        receiptWithNilData,
			isExtraDataNil: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipt := test.receipt
			copiedReceipt := test.receipt.Copy()

			require.Equal(t, expectedReceipt, copiedReceipt)

			if !test.isExtraDataNil {
				require.NotEqual(t,
					reflect.ValueOf(expectedReceipt.ExtraData).Pointer(),
					reflect.ValueOf(copiedReceipt.ExtraData).Pointer(),
				)
			}

			if test.receipt.Logs != nil {
				require.Len(t, expectedReceipt.Logs, len(copiedReceipt.Logs))

				for i := 0; i < len(expectedReceipt.Logs); i++ {
					require.NotEqual(t,
						reflect.ValueOf(expectedReceipt.Logs[i]),
						reflect.ValueOf(copiedReceipt.Logs[i]),
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
				Address: tests.RandomAddress(t),
				LogicID: tests.GetLogicID(t, tests.RandomAddress(t)),
				Topics:  tests.GetHashes(t, 1),
				Data:    []byte{1},
			},
		},
		{
			name: "empty addresses,topics,data",
			log: common.Log{
				LogicID: tests.GetLogicID(t, tests.RandomAddress(t)),
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedLog := test.log
			copiedLog := test.log.Copy()

			// Add assertions to check individual fields of the copied log
			require.Equal(t, expectedLog, copiedLog)
			require.Equal(t, expectedLog.Address, copiedLog.Address)

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
				reflect.ValueOf(test.receipts[hash].ExtraData).Pointer(),
				reflect.ValueOf(receipts[hash].ExtraData).Pointer(),
			)
		})
	}
}

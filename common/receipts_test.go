package common_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestCopyHashes(t *testing.T) {
	hashes := make(common.ReceiptAccHashes, 0)

	for i := 0; i < 3; i++ {
		hashes[tests.RandomAddress(t)] = &common.Hashes{
			StateHash:   tests.RandomHash(t),
			ContextHash: tests.RandomHash(t),
		}
	}

	testcases := []struct {
		name    string
		hashmap common.ReceiptAccHashes
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

			if test.receipt.Hashes != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedReceipt.Hashes).Pointer(),
					reflect.ValueOf(copiedReceipt.Hashes).Pointer(),
				)
			}

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
						reflect.ValueOf(expectedReceipt.Logs[i]).Pointer(),
						reflect.ValueOf(copiedReceipt.Logs[i]).Pointer(),
					)
				}
			}
		})
	}
}

func TestCopyLog(t *testing.T) {
	testcases := []struct {
		name string
		log  *common.Log
	}{
		{
			name: "copy log",
			log: &common.Log{
				Addresses: tests.GetAddresses(t, 1),
				LogicID:   tests.GetLogicID(t, tests.RandomAddress(t)),
				Topics:    tests.GetHashes(t, 1),
				Data:      []byte{1},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedLog := test.log
			copiedLog := test.log.Copy()

			// Add assertions to check individual fields of the copied log
			require.Equal(t, expectedLog, copiedLog)

			// Compare Addresses pointers
			require.NotEqual(t,
				reflect.ValueOf(expectedLog.Addresses).Pointer(),
				reflect.ValueOf(copiedLog.Addresses).Pointer(),
			)

			// Compare Topics pointers
			require.NotEqual(t,
				reflect.ValueOf(expectedLog.Topics).Pointer(),
				reflect.ValueOf(copiedLog.Topics).Pointer(),
			)

			// Compare Data slices
			if expectedLog.Data != nil {
				require.NotEqual(t,
					reflect.ValueOf(expectedLog.Data).Pointer(),
					reflect.ValueOf(copiedLog.Data).Pointer(),
				)
			}
		})
	}
}

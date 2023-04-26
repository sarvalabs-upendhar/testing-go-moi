package types_test

import (
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
)

func createReceiptsWithTestData(t *testing.T, hash types.Hash) types.Receipts {
	t.Helper()

	receipts := make(types.Receipts)
	receipts[hash] = tests.CreateReceiptWithTestData(t)

	return receipts
}

func createIxDataWithTestData(t *testing.T) types.IxData {
	t.Helper()

	IxData := types.IxData{
		Input:   tests.CreateIXInputWithTestData(t, types.IxAssetCreate, []byte{187, 1, 29, 103}, []byte{187, 1, 29, 103}),
		Compute: tests.CreateComputeWithTestData(t, tests.RandomHash(t).Bytes(), tests.GetTestKramaIDs(t, 2)),
		Trust:   tests.CreateTrustWithTestData(t),
	}

	return IxData
}

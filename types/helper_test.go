package types_test

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
)

func createInputWithTestData(t *testing.T) types.IxInput {
	t.Helper()

	IxInput := types.IxInput{
		Type:  types.IxAssetCreate,
		Nonce: 2,

		Sender:   tests.RandomAddress(t),
		Receiver: tests.RandomAddress(t),
		Payer:    tests.RandomAddress(t),

		TransferValues: map[types.AssetID]*big.Int{
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": new(big.Int).SetBytes(nil),
		},
		PerceivedValues: map[types.AssetID]*big.Int{
			"0180127603f47e5aff68053102fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": new(big.Int).SetBytes(nil),
		},
		PerceivedProofs: []byte{183, 3, 27, 101},

		FuelLimit: new(big.Int).SetUint64(0),
		FuelPrice: new(big.Int).SetUint64(0),

		Payload: []byte{187, 1, 29, 103},
	}

	return IxInput
}

func createComputeWithTestData(t *testing.T) types.IxCompute {
	t.Helper()

	IxCompute := types.IxCompute{
		Mode:         3,
		Hash:         []byte{135, 10, 27, 12, 109},
		ComputeNodes: tests.GetTestKramaIDs(t, 2),
	}

	return IxCompute
}

func createTrustWithTestData(t *testing.T) types.IxTrust {
	t.Helper()

	IxTrust := types.IxTrust{
		MTQ:        8,
		TrustNodes: tests.GetTestKramaIDs(t, 2),
	}

	return IxTrust
}

func createIxDataWithTestData(t *testing.T) types.IxData {
	t.Helper()

	IxData := types.IxData{
		Input:   createInputWithTestData(t),
		Compute: createComputeWithTestData(t),
		Trust:   createTrustWithTestData(t),
	}

	return IxData
}

package types_test

import (
	"math/big"
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

func createReceiptWithTestData(t *testing.T) *types.Receipt {
	t.Helper()

	receipt := &types.Receipt{
		IxType:        2,
		StateHashes:   make(map[types.Address]types.Hash),
		ContextHashes: make(map[types.Address]types.Hash),
		ExtraData:     []byte{1, 2},
	}

	receipt.StateHashes[tests.RandomAddress(t)] = tests.RandomHash(t)
	receipt.ContextHashes[tests.RandomAddress(t)] = tests.RandomHash(t)

	return receipt
}

func createReceiptsWithTestData(t *testing.T, hash types.Hash) types.Receipts {
	t.Helper()

	receipts := make(types.Receipts)
	receipts[hash] = createReceiptWithTestData(t)

	return receipts
}

func createCommitDataWithTestData(t *testing.T) types.CommitData {
	t.Helper()

	return types.CommitData{
		Round:           4,
		CommitSignature: []byte{1, 2, 3},
		VoteSet: &types.ArrayOfBits{
			Elements: []uint64{4, 4},
		},
		EvidenceHash: tests.RandomHash(t),
		GridID: &types.TesseractGridID{
			Hash: tests.RandomHash(t),
			Parts: &types.TesseractParts{
				Heights: []uint64{1, 2},
				Hashes:  []types.Hash{tests.RandomHash(t)},
			},
		},
	}
}

func createBodyWithTestData(t *testing.T) types.TesseractBody {
	t.Helper()

	body := types.TesseractBody{
		StateHash:       tests.RandomHash(t),
		ContextHash:     tests.RandomHash(t),
		InteractionHash: tests.RandomHash(t),
		ReceiptHash:     tests.RandomHash(t),
		ContextDelta:    make(map[types.Address]*types.DeltaGroup),
		ConsensusProof: types.PoXCData{
			BinaryHash:   tests.RandomHash(t),
			IdentityHash: tests.RandomHash(t),
			ICSHash:      tests.RandomHash(t),
		},
	}

	body.ContextDelta[tests.RandomAddress(t)] = &types.DeltaGroup{
		BehaviouralNodes: tests.GetTestKramaIDs(t, 2),
		RandomNodes:      make([]id.KramaID, 0),
		ReplacedNodes:    make([]id.KramaID, 0),
	}

	return body
}

func createHeaderWithTestData(t *testing.T) types.TesseractHeader {
	t.Helper()

	header := types.TesseractHeader{
		Address:     tests.RandomAddress(t),
		PrevHash:    tests.RandomHash(t),
		Height:      4444,
		AnuUsed:     5,
		AnuLimit:    6,
		BodyHash:    tests.RandomHash(t),
		GridHash:    tests.RandomHash(t),
		Operator:    "operator",
		ClusterID:   "cluster-ID",
		Timestamp:   1,
		ContextLock: make(map[types.Address]types.ContextLockInfo),
		Extra:       createCommitDataWithTestData(t),
	}

	header.ContextLock[tests.RandomAddress(t)] = types.ContextLockInfo{
		TesseractHash: tests.RandomHash(t),
	}

	return header
}

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

package lattice

import (
	"math/big"

	"github.com/sarvalabs/moichain/common"
)

func createGenesisTesseract(
	addr common.Address,
	stateHash, contextHash common.Hash,
) (*common.Tesseract, error) {
	ixHash := common.GetHash([]byte("Genesis" + stateHash.Hex()))

	body := common.TesseractBody{
		StateHash:       stateHash,
		ContextHash:     contextHash,
		ContextDelta:    nil,
		ReceiptHash:     common.Hash{},
		InteractionHash: ixHash,
		ConsensusProof: common.PoXtData{
			IdentityHash: common.NilHash,
			BinaryHash:   common.NilHash,
		},
	}

	tsBodyHash, err := body.Hash()
	if err != nil {
		return nil, err
	}

	header := common.TesseractHeader{
		Address:   addr,
		PrevHash:  common.NilHash,
		Height:    0,
		FuelUsed:  big.NewInt(0),
		FuelLimit: big.NewInt(0),
		BodyHash:  tsBodyHash,
		GroupHash: common.NilHash,
		ClusterID: common.GenesisIdentifier,
		Operator:  common.GenesisIdentifier,
		Extra: common.CommitData{
			CommitSignature: nil,
			Round:           0,
			VoteSet:         nil,
		},
	}

	return common.NewTesseract(header, body, nil, nil, nil, ""), nil
}

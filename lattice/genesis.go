package lattice

import (
	"math/big"

	"github.com/sarvalabs/moichain/types"
)

const GenesisIdentifier = "genesis"

func createGenesisTesseract(
	addr types.Address,
	stateHash, contextHash types.Hash,
) (*types.Tesseract, error) {
	ixHash := types.GetHash([]byte("Genesis" + stateHash.Hex()))

	body := types.TesseractBody{
		StateHash:       stateHash,
		ContextHash:     contextHash,
		ContextDelta:    nil,
		ReceiptHash:     types.Hash{},
		InteractionHash: ixHash,
		ConsensusProof: types.PoXCData{
			IdentityHash: types.NilHash,
			BinaryHash:   types.NilHash,
		},
	}

	tsBodyHash, err := body.Hash()
	if err != nil {
		return nil, err
	}

	header := types.TesseractHeader{
		Address:   addr,
		PrevHash:  types.NilHash,
		Height:    0,
		FuelUsed:  big.NewInt(0),
		FuelLimit: big.NewInt(0),
		BodyHash:  tsBodyHash,
		GroupHash: types.NilHash,
		ClusterID: GenesisIdentifier,
		Operator:  GenesisIdentifier,
		Extra: types.CommitData{
			CommitSignature: nil,
			Round:           0,
			VoteSet:         nil,
		},
	}

	return types.NewTesseract(header, body, nil, nil, nil, ""), nil
}

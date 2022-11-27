package lattice

import (
	"github.com/sarvalabs/moichain/types"
)

var GenesisIxHash = types.GetHash([]byte("Genesis Interaction"))

type Genesis struct {
	SargaAccount AccountInfo   `json:"sarga_account"`
	Accounts     []AccountInfo `json:"accounts"`
}

type AccountInfo struct {
	Address          string             `json:"address"`
	AccType          types.AccType      `json:"type"`
	MOIId            string             `json:"moi_id"`
	BehaviourContext []string           `json:"behaviour_context"`
	RandomContext    []string           `json:"random_context"`
	AssetDetails     []*types.AssetInfo `json:"assets"`
	Balances         []*BalanceInfo     `json:"balance"`
}

type AssetInfo struct {
	Dimension   int    `json:"dimension"`
	TotalSupply int    `json:"total_supply"`
	Symbol      string `json:"symbol"`
	IsFungible  bool   `json:"isFungible"`
	IsMintable  bool   `json:"isMintable"`
}

type BalanceInfo struct {
	AssetID string `json:"asset_id"`
	Amount  int64  `json:"balance"`
}

func createGenesisTesseract(
	addr types.Address,
	stateHash, contextHash types.Hash,
	contextDelta types.ContextDelta,
) (*types.Tesseract, error) {
	Tesseract := &types.Tesseract{
		Header: types.TesseractHeader{
			Address:  addr,
			PrevHash: types.NilHash,
			Height:   0,
			// Timestamp:     time.Now().UnixNano(),
			AnuUsed:   0,
			AnuLimit:  0,
			BodyHash:  types.NilHash,
			GridHash:  types.NilHash,
			ClusterID: "genesis",
			Operator:  "genesis",
			Extra: types.CommitData{
				CommitSignature: nil,
				Round:           0,
				VoteSet:         nil,
			},
		},
		Body: types.TesseractBody{
			StateHash:       stateHash,
			ContextHash:     contextHash,
			ContextDelta:    contextDelta,
			ReceiptHash:     types.Hash{},
			InteractionHash: GenesisIxHash,
			ConsensusProof: types.PoXCData{
				IdentityHash: types.NilHash,
				BinaryHash:   types.NilHash,
			},
		},
		Ixns: nil,
	}

	tsBodyHash, err := Tesseract.ComputeBodyHash()
	if err != nil {
		return nil, err
	}

	Tesseract.Header.BodyHash = tsBodyHash

	return Tesseract, nil
}

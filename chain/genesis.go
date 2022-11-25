package chain

import (
	"github.com/sarvalabs/moichain/types"
)

var GenesisIxHash = types.GetHash([]byte("Genesis Interaction"))

type Genesis struct {
	Accounts []AccountInfo `json:"accounts"`
}

type AccountInfo struct {
	Address          string         `json:"address"`
	MOIId            string         `json:"moi_id"`
	BehaviourContext []string       `json:"behaviour_context"`
	RandomContext    []string       `json:"random_context"`
	AccType          types.AccType  `json:"type"`
	AssetDetails     []*AssetInfo   `json:"assets"`
	Balances         []*BalanceInfo `json:"balance"`
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

func CreateGenesisTesseract(
	addr types.Address,
	stateHash, contextHash types.Hash,
	contextDelta types.ContextDelta,
) *types.Tesseract {
	Tesseract := &types.Tesseract{
		Header: types.TesseractHeader{
			Address:  addr,
			PrevHash: types.NilHash,
			Height:   0,
			// Timestamp:     time.Now().UnixNano(),
			AnuUsed:       0,
			AnuLimit:      0,
			TesseractHash: types.NilHash,
			GridHash:      types.NilHash,
			ClusterID:     "genesis",
			Operator:      "genesis",
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
	Tesseract.Header.TesseractHash = Tesseract.BodyHash()

	return Tesseract
}

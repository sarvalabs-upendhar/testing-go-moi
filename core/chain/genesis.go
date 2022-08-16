package chain

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

var (
	GenesisIxHash = ktypes.GetHash([]byte("Genesis Interaction"))
)

type Genesis struct {
	Accounts []AccountInfo `json:"accounts"`
}

type AccountInfo struct {
	Address          string         `json:"address"`
	MOIId            string         `json:"moi_id"`
	BehaviourContext []string       `json:"behaviour_context"`
	RandomContext    []string       `json:"random_context"`
	AccType          ktypes.AccType `json:"type"`
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
	addr ktypes.Address,
	stateHash, contextHash ktypes.Hash,
	contextDelta ktypes.ContextDelta,
) *ktypes.Tesseract {
	Tesseract := &ktypes.Tesseract{
		Header: ktypes.TesseractHeader{
			Address:  addr,
			PrevHash: ktypes.NilHash,
			Height:   0,
			//Timestamp:     time.Now().UnixNano(),
			AnuUsed:       0,
			AnuLimit:      0,
			TesseractHash: ktypes.NilHash,
			GroupHash:     ktypes.NilHash,
			ClusterID:     "genesis",
			Operator:      "genesis",
			Extra: ktypes.CommitData{
				CommitSignature: nil,
				Round:           0,
				VoteSet:         nil,
			},
		},
		Body: ktypes.TesseractBody{
			StateHash:       stateHash,
			ContextHash:     contextHash,
			Interactions:    nil,
			ContextDelta:    contextDelta,
			ReceiptHash:     ktypes.Hash{},
			InteractionHash: GenesisIxHash,
			ConsensusProof: ktypes.PoXCData{
				IdentityHash: ktypes.NilHash,
				BinaryHash:   ktypes.NilHash,
			},
		},
	}
	Tesseract.Header.TesseractHash = Tesseract.BodyHash()

	return Tesseract
}

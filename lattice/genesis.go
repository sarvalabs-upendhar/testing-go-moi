package lattice

import (
	"github.com/sarvalabs/moichain/types"
)

type Genesis struct {
	SargaAccount AccountInfo   `json:"sarga_account"`
	Accounts     []AccountInfo `json:"accounts"`
}

type AccountInfo struct {
	Address          string            `json:"address"`
	AccountType      types.AccountType `json:"type"`
	MOIId            string            `json:"moi_id"`
	BehaviourContext []string          `json:"behaviour_context"`
	RandomContext    []string          `json:"random_context"`
	AssetDetails     []*AssetInfo      `json:"assets"`
	Balances         []*BalanceInfo    `json:"balance"`
}

type AssetInfo struct {
	Type           int    `json:"type"`
	Symbol         string `json:"symbol"`
	Owner          string `json:"owner"`
	TotalSupply    uint64 `json:"total_supply"`
	Dimension      uint8  `json:"dimension"`
	Decimals       uint8  `json:"decimals"`
	IsFungible     bool   `json:"is_fungible"`
	IsMintable     bool   `json:"is_mintable"`
	IsTransferable bool   `json:"is_transferable"`
	LogicID        string `json:"logic_id"`
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
	ixHash := types.GetHash([]byte("Genesis" + stateHash.Hex()))
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
			InteractionHash: ixHash,
			ConsensusProof: types.PoXCData{
				IdentityHash: types.NilHash,
				BinaryHash:   types.NilHash,
			},
		},
	}

	tsBodyHash, err := Tesseract.ComputeBodyHash()
	if err != nil {
		return nil, err
	}

	Tesseract.Header.BodyHash = tsBodyHash

	return Tesseract, nil
}

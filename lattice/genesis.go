package lattice

import (
	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/types"
)

const GenesisIdentifier = "genesis"

// GenesisV1 is Genesis file with V1 version
type GenesisV1 struct {
	SargaAccount AccountInfoV1   `json:"sarga_account"`
	Accounts     []AccountInfoV1 `json:"accounts"`
	Logics       []GenesisLogic  `json:"logics"`
	AssetInfos   []AssetInfoV1   `json:"asset_infos"`
}

type AccountInfoV1 struct {
	Address            string            `json:"address"`
	AccountType        types.AccountType `json:"type"`
	MoiID              string            `json:"moi_id"`
	BehaviouralContext []string          `json:"behaviour_context"`
	RandomContext      []string          `json:"random_context"`
}

type AssetInfoV1 struct {
	Symbol      string         `json:"symbol"`
	Dimension   uint8          `json:"dimension"`
	Standard    uint16         `json:"standard"`
	IsLogical   bool           `json:"is_logical"`
	IsMintable  bool           `json:"is_mintable"`
	Owner       string         `json:"owner"`
	Allocations []AllocationV1 `json:"allocations"`
}

type AllocationV1 struct {
	Address string `json:"address"`
	Balance uint64 `json:"balance"`
}

// Genesis will be older version
type Genesis struct {
	SargaAccount AccountInfo    `json:"sarga_account"`
	Accounts     []AccountInfo  `json:"accounts"`
	Logics       []GenesisLogic `json:"logics"`
}

func (g *GenesisV1) AddSargaAccount(info AccountInfoV1) {
	g.SargaAccount = info
}

func (g *GenesisV1) AddAccount(info AccountInfoV1) {
	g.Accounts = append(g.Accounts, info)
}

func (g *GenesisV1) AddLogic(logic GenesisLogic) {
	g.Logics = append(g.Logics, logic)
}

func (g *GenesisV1) AddAssetInfo(info AssetInfoV1) {
	g.AssetInfos = append(g.AssetInfos, info)
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

type GenesisLogic struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`

	BehaviouralContext []string `json:"behaviour_context"`
	RandomContext      []string `json:"random_context"`
}

func createGenesisTesseract(
	addr types.Address,
	stateHash, contextHash types.Hash,
	contextDelta types.ContextDelta,
) (*types.Tesseract, error) {
	ixHash := types.GetHash([]byte("Genesis" + stateHash.Hex()))

	body := types.TesseractBody{
		StateHash:       stateHash,
		ContextHash:     contextHash,
		ContextDelta:    contextDelta,
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
		Address:  addr,
		PrevHash: types.NilHash,
		Height:   0,
		// Timestamp:     time.Now().UnixNano(),
		FuelUsed:  0,
		FuelLimit: 0,
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

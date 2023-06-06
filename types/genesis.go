package types

import (
	"math/big"

	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/mudra/kramaid"
)

// GenesisFile is Genesis file with V1 version
type GenesisFile struct {
	SargaAccount  AccountSetupArgs        `json:"sarga_account"`
	Accounts      []AccountSetupArgs      `json:"accounts"`
	Logics        []GenesisLogic          `json:"logics"`
	AssetAccounts []AssetAccountSetupArgs `json:"asset_accounts"`
}

type AssetAccountSetupArgs struct {
	AssetInfo          *AssetCreationArgs `json:"asset_info"`
	BehaviouralContext []kramaid.KramaID  `json:"behaviour_context"`
	RandomContext      []kramaid.KramaID  `json:"random_context"`
}

type AssetCreationArgs struct {
	Type        AssetKind      `json:"type"`
	Symbol      string         `json:"symbol"`
	Dimension   hexutil.Uint8  `json:"dimension"`
	Standard    hexutil.Uint16 `json:"standard"`
	IsLogical   bool           `json:"is_logical"`
	IsStateful  bool           `json:"is_stateful"`
	Owner       Address        `json:"owner"`
	Allocations []Allocation   `json:"allocations"`
}

func (ac *AssetCreationArgs) AssetDescriptor() *AssetDescriptor {
	totalSupply := big.NewInt(0)

	for _, allocation := range ac.Allocations {
		totalSupply.Add(totalSupply, allocation.Amount.ToInt())
	}

	return &AssetDescriptor{
		Type:       ac.Type,
		Symbol:     ac.Symbol,
		Owner:      ac.Owner,
		Supply:     totalSupply,
		Dimension:  ac.Dimension.ToInt(),
		Standard:   ac.Standard.ToInt(),
		IsLogical:  ac.IsLogical,
		IsStateFul: ac.IsStateful,
	}
}

type Allocation struct {
	Address Address      `json:"address"`
	Amount  *hexutil.Big `json:"amount"`
}

func (g *GenesisFile) AddSargaAccount(info AccountSetupArgs) {
	g.SargaAccount = info
}

func (g *GenesisFile) AddAccount(info AccountSetupArgs) {
	g.Accounts = append(g.Accounts, info)
}

func (g *GenesisFile) AddLogic(logic GenesisLogic) {
	g.Logics = append(g.Logics, logic)
}

func (g *GenesisFile) AddAssetInfo(info AssetAccountSetupArgs) {
	g.AssetAccounts = append(g.AssetAccounts, info)
}

type GenesisLogic struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`

	BehaviouralContext []string `json:"behaviour_context"`
	RandomContext      []string `json:"random_context"`
}

type AccountSetupArgs struct {
	Address            Address           `json:"address"`
	AccType            AccountType       `json:"type"`
	MoiID              string            `json:"moi-id"`
	BehaviouralContext []kramaid.KramaID `json:"behaviour_context"`
	RandomContext      []kramaid.KramaID `json:"random_context"`
}

func (as *AccountSetupArgs) ContextDelta() ContextDelta {
	return map[Address]*DeltaGroup{
		as.Address: {
			BehaviouralNodes: as.BehaviouralContext,
			RandomNodes:      as.RandomContext,
		},
	}
}

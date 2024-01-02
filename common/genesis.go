package common

import (
	"encoding/json"
	"math/big"
	"os"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common/hexutil"
)

const GenesisIdentifier = "genesis"

// GenesisFile is Genesis file with V1 version
type GenesisFile struct {
	SargaAccount  AccountSetupArgs        `json:"sarga_account"`
	Accounts      []AccountSetupArgs      `json:"accounts"`
	Logics        []LogicSetupArgs        `json:"logics"`
	AssetAccounts []AssetAccountSetupArgs `json:"asset_accounts"`
}

type AssetAccountSetupArgs struct {
	AssetInfo          *AssetCreationArgs `json:"asset_info"`
	BehaviouralContext []kramaid.KramaID  `json:"behaviour_context"`
	RandomContext      []kramaid.KramaID  `json:"random_context"`
}

type AssetCreationArgs struct {
	Symbol      string              `json:"symbol"`
	Dimension   hexutil.Uint8       `json:"dimension"`
	Standard    hexutil.Uint16      `json:"standard"`
	IsLogical   bool                `json:"is_logical"`
	IsStateful  bool                `json:"is_stateful"`
	Operator    identifiers.Address `json:"operator"`
	Allocations []Allocation        `json:"allocations"`
}

func (ac *AssetCreationArgs) AssetDescriptor() *AssetDescriptor {
	totalSupply := big.NewInt(0)

	for _, allocation := range ac.Allocations {
		totalSupply.Add(totalSupply, allocation.Amount.ToInt())
	}

	return &AssetDescriptor{
		Symbol:     ac.Symbol,
		Operator:   ac.Operator,
		Supply:     totalSupply,
		Dimension:  ac.Dimension.ToInt(),
		Standard:   AssetStandard(ac.Standard.ToInt()),
		IsLogical:  ac.IsLogical,
		IsStateFul: ac.IsStateful,
	}
}

type Allocation struct {
	Address identifiers.Address `json:"address"`
	Amount  *hexutil.Big        `json:"amount"`
}

func (g *GenesisFile) AddSargaAccount(info AccountSetupArgs) {
	g.SargaAccount = info
}

func (g *GenesisFile) AddAccount(info AccountSetupArgs) {
	g.Accounts = append(g.Accounts, info)
}

func (g *GenesisFile) AddLogic(logic LogicSetupArgs) {
	g.Logics = append(g.Logics, logic)
}

func (g *GenesisFile) AddAssetInfo(info AssetAccountSetupArgs) {
	g.AssetAccounts = append(g.AssetAccounts, info)
}

func ReadGenesisFile(path string) (*GenesisFile, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &GenesisFile{}, nil
	}

	genesis := new(GenesisFile)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("error reading genesis file")
	}

	if err = json.Unmarshal(file, genesis); err != nil {
		return nil, errors.New("error reading genesis file")
	}

	return genesis, nil
}

type LogicSetupArgs struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`

	BehaviouralContext []kramaid.KramaID `json:"behaviour_context"`
	RandomContext      []kramaid.KramaID `json:"random_context"`
}

type AccountSetupArgs struct {
	Address            identifiers.Address `json:"address"`
	AccType            AccountType         `json:"type"`
	MoiID              string              `json:"moi-id"`
	BehaviouralContext []kramaid.KramaID   `json:"behaviour_context"`
	RandomContext      []kramaid.KramaID   `json:"random_context"`
}

func (as *AccountSetupArgs) ContextDelta() ContextDelta {
	return map[identifiers.Address]*DeltaGroup{
		as.Address: {
			BehaviouralNodes: as.BehaviouralContext,
			RandomNodes:      as.RandomContext,
		},
	}
}

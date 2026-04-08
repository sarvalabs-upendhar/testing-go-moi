package common

import (
	"encoding/json"
	"math/big"
	"os"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

const GenesisView = 0

// GenesisFile is Genesis file with V1 version
type GenesisFile struct {
	SargaAccount  AccountSetupArgs        `json:"sarga_account"`
	SystemAccount SystemAccountSetupArgs  `json:"system_account"`
	Accounts      []AccountSetupArgs      `json:"accounts"`
	Logics        []LogicSetupArgs        `json:"logics"`
	AssetAccounts []AssetAccountSetupArgs `json:"asset_accounts"`
	AssetLogics   []AssetLogicArgs        `json:"asset_logics"`
}

type AssetAccountSetupArgs struct {
	AssetInfo      *AssetCreationArgs    `json:"asset_info"`
	ConsensusNodes []identifiers.KramaID `json:"consensus_nodes"`
}

type AssetCreationArgs struct {
	Symbol            string                   `json:"symbol"`
	Dimension         hexutil.Uint8            `json:"dimension"`
	Decimals          hexutil.Uint8            `json:"decimals"`
	Standard          hexutil.Uint16           `json:"standard"`
	Creator           identifiers.Identifier   `json:"creator"`
	Manager           identifiers.Identifier   `json:"manager"`
	MaxSupply         hexutil.Big              `json:"max_supply"`
	CirculatingSupply hexutil.Big              `json:"circulating_supply"`
	StaticMetadata    map[string]hexutil.Bytes `json:"static_metadata"`
	DynamicMetadata   map[string]hexutil.Bytes `json:"dynamic_metadata"`
	LogicPayload      LogicSetupArgs           `json:"logic_payload"`
	Allocations       []Allocation             `json:"allocations"`
	descriptor        *AssetDescriptor
}

func (ac *AssetCreationArgs) AssetID() identifiers.AssetID {
	return ac.AssetDescriptor().AssetID
}

func (ac *AssetCreationArgs) AssetDescriptor() *AssetDescriptor {
	if ac.descriptor != nil {
		return ac.descriptor
	}

	logicID := CreateLogicIDFromString(ac.Symbol, 0, identifiers.AssetLogical, identifiers.Systemic)

	ad := &AssetDescriptor{
		Symbol:            ac.Symbol,
		Decimals:          ac.Decimals.ToInt(),
		Creator:           ac.Creator,
		Manager:           ac.Manager,
		MaxSupply:         ac.MaxSupply.ToInt(),
		CirculatingSupply: big.NewInt(0),
		StaticMetaData:    make(map[string][]byte),
		DynamicMetaData:   make(map[string][]byte),
		LogicID:           logicID,
	}

	for k, v := range ac.StaticMetadata {
		ad.StaticMetaData[k] = v.Bytes()
	}

	for k, v := range ac.DynamicMetadata {
		ad.DynamicMetaData[k] = v.Bytes()
	}

	assetID := CreateAssetIDFromString(ac.Symbol, 0, uint16(ac.Standard), ad.Flags()...)

	ad.AssetID = assetID

	return ad
}

type Allocation struct {
	ID     identifiers.Identifier `json:"id"`
	Amount *hexutil.Big           `json:"amount"`
}

type PayoutDetails struct {
	Beneficiary identifiers.Identifier
	AssetID     identifiers.AssetID
	TokenID     TokenID
	Amount      *big.Int
}

func PayoutsFromAllocations(assetID identifiers.AssetID, allocs []Allocation) []PayoutDetails {
	allocations := make([]PayoutDetails, len(allocs))
	for i, alloc := range allocs {
		allocations[i] = PayoutDetails{
			Beneficiary: alloc.ID,
			Amount:      alloc.Amount.ToInt(),
			AssetID:     assetID,
			TokenID:     DefaultTokenID,
		}
	}

	return allocations
}

func (g *GenesisFile) AddSargaAccount(info AccountSetupArgs) {
	g.SargaAccount = info
}

func (g *GenesisFile) AddSystemAccount(info SystemAccountSetupArgs) {
	g.SystemAccount = info
}

func (g *GenesisFile) AddAccount(info AccountSetupArgs) {
	g.Accounts = append(g.Accounts, info)
}

func (g *GenesisFile) AddLogic(logic LogicSetupArgs) {
	g.Logics = append(g.Logics, logic)
}

func (g *GenesisFile) AddAssetLogic(info AssetLogicArgs) {
	g.AssetLogics = append(g.AssetLogics, info)
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
		return nil, ErrReadingGenesisFile
	}

	if err = json.Unmarshal(file, genesis); err != nil {
		return nil, ErrReadingGenesisFile
	}

	return genesis, nil
}

type LogicSetupArgs struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`

	ConsensusNodes []identifiers.KramaID `json:"consensus_nodes"`
}

type KeyArgs struct {
	PublicKey          hexutil.Bytes  `json:"public_key"`
	Weight             hexutil.Uint64 `json:"weight"`
	SignatureAlgorithm hexutil.Uint64 `json:"signature_algorithm"`
}

type AccountSetupArgs struct {
	ID             identifiers.Identifier `json:"id"`
	Keys           []KeyArgs              `json:"keys"`
	MoiID          string                 `json:"moi-id"`
	ConsensusNodes []identifiers.KramaID  `json:"consensus_nodes"`
}

type SystemAccountSetupArgs struct {
	ID             identifiers.Identifier `json:"id"`
	Keys           []KeyArgs              `json:"keys"`
	ConsensusNodes []identifiers.KramaID  `json:"consensus_nodes"`
	Validators     []*Validator           `json:"validators"`
}

type AssetLogicArgs struct {
	Standard hexutil.Uint16 `json:"standard"`
	Manifest hexutil.Bytes  `json:"manifest"`
}

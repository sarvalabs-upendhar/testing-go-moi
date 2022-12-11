package types

import (
	"math/big"
)

type AssetKind int

const (
	AssetKindValue AssetKind = iota
	AssetKindFile
	AssetKindLogic
	AssetKindContext
)

type AssetMap map[AssetID]*big.Int

type AssetDescriptor struct {
	Type   AssetKind `json:"type"`
	Symbol string    `json:"symbol"`
	Owner  Address   `json:"owner"`
	Supply *big.Int  `json:"supply"`

	Dimension uint8 `json:"dimension"`
	Decimals  uint8 `json:"decimals"`

	IsFungible     bool `json:"is_fungible"`
	IsMintable     bool `json:"is_mintable"`
	IsTransferable bool `json:"is_transferable"`

	LogicID LogicID `json:"logic_id"`
}

func NewAssetDescriptor(owner Address, asset AssetCreatePayload) *AssetDescriptor {
	return &AssetDescriptor{
		Owner:          owner,
		Type:           asset.Type,
		Symbol:         asset.Symbol,
		Supply:         asset.Supply,
		Dimension:      asset.Dimension,
		Decimals:       asset.Decimals,
		IsFungible:     asset.IsFungible,
		IsMintable:     asset.IsMintable,
		IsTransferable: asset.IsTransferable,
		LogicID:        asset.LogicID,
	}
}

type AssetDimension byte

const (
	Economic AssetDimension = iota
	Possession
)

func (dimension AssetDimension) String() string {
	switch dimension {
	case Economic:
		return "Economic"

	case Possession:
		return "Possession"
	}

	return ""
}

func StringToDimensionID(str string) AssetDimension {
	switch str {
	case "Economic":
		return Economic
	case "Possession":
		return Possession
	}

	return 0
}

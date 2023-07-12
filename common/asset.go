package common

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type AssetMap map[AssetID]*big.Int

func (assets AssetMap) Copy() AssetMap {
	copied := make(AssetMap, len(assets))
	for asset, amount := range assets {
		copied[asset] = new(big.Int).SetBytes(amount.Bytes())
	}

	return copied
}

type AssetDescriptor struct {
	Symbol   string   `json:"symbol"`
	Operator Address  `json:"operator"`
	Supply   *big.Int `json:"supply"`

	Dimension  uint8         `json:"dimension"`
	Standard   AssetStandard `json:"standard"`
	IsLogical  bool          `json:"is_logical"`
	IsStateFul bool          `json:"is_stateful"`

	LogicID LogicID `json:"logic_id"`
}

func NewAssetDescriptor(operator Address, asset AssetCreatePayload) *AssetDescriptor {
	return &AssetDescriptor{
		Operator:   operator,
		Symbol:     asset.Symbol,
		Supply:     asset.Supply,
		Dimension:  asset.Dimension,
		Standard:   asset.Standard,
		IsStateFul: asset.IsStateFul,
		IsLogical:  asset.IsLogical,
	}
}

func (ad *AssetDescriptor) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ad)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset descriptor")
	}

	return rawData, nil
}

func (ad *AssetDescriptor) FromBytes(data []byte) error {
	if err := polo.Depolorize(ad, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset descriptor")
	}

	return nil
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

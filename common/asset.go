package common

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
)

type AssetMap map[identifiers.AssetID]*big.Int

func (assets AssetMap) Copy() AssetMap {
	copied := make(AssetMap, len(assets))
	for asset, amount := range assets {
		copied[asset] = new(big.Int).SetBytes(amount.Bytes())
	}

	return copied
}

type AssetDescriptor struct {
	Symbol   string                 `json:"symbol"`
	Operator identifiers.Identifier `json:"operator"`
	Supply   *big.Int               `json:"supply"`

	Dimension  uint8         `json:"dimension"`
	Standard   AssetStandard `json:"standard"`
	IsLogical  bool          `json:"is_logical"`
	IsStateFul bool          `json:"is_stateful"`

	LogicID identifiers.Identifier `json:"logic_id"`
}

func NewAssetDescriptor(operator identifiers.Identifier, asset AssetCreatePayload) *AssetDescriptor {
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

func (ad *AssetDescriptor) Flags() []identifiers.Flag {
	flags := make([]identifiers.Flag, 0)

	if ad.Symbol == KMOITokenSymbol {
		flags = append(flags, identifiers.Systemic)
	}

	if ad.IsLogical {
		flags = append(flags, identifiers.AssetLogical)
	}

	if ad.IsStateFul {
		flags = append(flags, identifiers.AssetStateful)
	}

	return flags
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

type AssetStandard uint16

// MAS is moi asset standard
const (
	MAS0 AssetStandard = iota
	MAS1
)

type AssetMandateOrLockup struct {
	AssetID identifiers.AssetID
	ID      identifiers.Identifier
	Amount  *big.Int
}

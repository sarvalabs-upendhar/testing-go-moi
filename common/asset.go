package common

import (
	"math"
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

const (
	DefaultTokenID TokenID = 0
)

type TokenID uint64

type AmountWithExpiry struct {
	Amount    *big.Int
	ExpiresAt uint64
}
type AssetMap map[identifiers.AssetID]map[TokenID]*big.Int

func (assets AssetMap) Copy() AssetMap {
	copied := make(AssetMap, len(assets))
	for asset, tokens := range assets {
		copied[asset] = make(map[TokenID]*big.Int, len(tokens))

		for tokenID, amount := range tokens {
			copied[asset][tokenID] = new(big.Int).SetBytes(amount.Bytes())
		}
	}

	return copied
}

type AssetDescriptor struct {
	AssetID           identifiers.AssetID    `json:"asset_id"`
	Symbol            string                 `json:"symbol"`
	Decimals          uint8                  `json:"decimals"`
	Dimension         uint8                  `json:"dimension"`
	Creator           identifiers.Identifier `json:"creator"`
	Manager           identifiers.Identifier `json:"manager"`
	MaxSupply         *big.Int               `json:"max_supply"`
	CirculatingSupply *big.Int               `json:"circulating_supply"`
	EnableEvents      bool                   `json:"enable_events"`
	Metadata          map[string][]byte      `json:"metadata"`

	LogicID identifiers.LogicID `json:"logic_id"`
}

func NewAssetDescriptor(
	assetID identifiers.AssetID,
	symbol string,
	decimals uint8,
	dimension uint8,
	manager identifiers.Identifier,
	creator identifiers.Identifier,
	maxSupply *big.Int,
	metadata map[string][]byte,
	enableEvents bool,
	logicID identifiers.LogicID,
) *AssetDescriptor {
	return &AssetDescriptor{
		AssetID:           assetID,
		Symbol:            symbol,
		Decimals:          decimals,
		Dimension:         dimension,
		Creator:           creator,
		Manager:           manager,
		MaxSupply:         maxSupply,
		CirculatingSupply: big.NewInt(0),
		EnableEvents:      enableEvents,
		Metadata:          metadata,
		LogicID:           logicID,
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

	if ad.LogicID != identifiers.Nil {
		flags = append(flags, identifiers.AssetLogical)
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

	MASX = math.MaxUint16
)

var ValidAssetStandards = map[AssetStandard]string{
	MAS0: "MAS0",
	MAS1: "MAS1",
	MASX: "MASX",
}

type AssetMandateOrLockup struct {
	AssetID identifiers.AssetID
	ID      identifiers.Identifier
	Amount  map[TokenID]*AmountWithExpiry
}

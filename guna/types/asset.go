package types

import (
	"encoding/hex"
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

type AssetObject struct {
	Symbol   string
	Supply   *big.Int
	Decimals uint8

	Owner   types.Address
	LogicID types.LogicID
	Extra   []byte
}

func (ad *AssetObject) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ad, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize asset data")
	}

	return nil
}

func GetAssetID(asset *types.AssetDescriptor) (types.AssetID, types.Hash, []byte, error) {
	assetObject := AssetObject{
		Owner:    asset.Owner,
		Symbol:   asset.Symbol,
		Decimals: asset.Decimals,
		Extra:    make([]byte, 8),
	}

	var (
		buf  []byte
		info uint8 = 0x00
	)

	if asset.IsMintable {
		info |= 0x01
	} else {
		assetObject.Supply = asset.Supply
	}

	if asset.IsFungible {
		info |= 0x80
	}

	buf = append(buf, asset.Dimension)
	buf = append(buf, info)

	data, err := polo.Polorize(assetObject)
	if err != nil {
		return "", types.NilHash, nil, err
	}

	assetCID := types.GetHash(data)
	buf = append(buf, assetCID.Bytes()...)
	assetID := types.AssetID(hex.EncodeToString(buf))

	return assetID, assetCID, data, nil
}

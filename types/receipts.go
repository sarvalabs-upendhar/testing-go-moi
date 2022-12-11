package types

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type Receipt struct {
	IxType        int
	IxHash        Hash
	FuelUsed      uint64
	StateHashes   map[Address]Hash
	ContextHashes map[Address]Hash
	ExtraData     json.RawMessage
}

func (r *Receipt) SetExtraData(data interface{}) error {
	rawData, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(errors.New("Receipt generation failed"), err.Error())
	}

	r.ExtraData = rawData

	return nil
}

type Receipts map[Hash]*Receipt

func (rs Receipts) Hash() (Hash, error) {
	hash, err := PoloHash(rs)
	if err != nil {
		return NilHash, errors.Wrap(err, "failed to polorize receipts")
	}

	return hash, nil
}

func (rs Receipts) GetReceipt(ixHash Hash) (*Receipt, error) {
	if receipt, ok := rs[ixHash]; ok {
		return receipt, nil
	}

	return nil, ErrReceiptNotFound
}

func (rs Receipts) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(rs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize receipts")
	}

	return rawData, nil
}

func (rs *Receipts) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rs, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize receipts")
	}

	return nil
}

type AssetCreationReceipt struct {
	AssetID string `json:"asset_id"`
}

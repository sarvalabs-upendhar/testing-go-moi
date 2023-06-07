package types

import (
	"encoding/json"
	"math/big"

	"github.com/sarvalabs/moichain/common/hexutil"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type ReceiptStatus uint64

const (
	ReceiptOk ReceiptStatus = iota
	ReceiptFailed
)

type Receipt struct {
	IxType IxType        `json:"ix_type"`
	IxHash Hash          `json:"ix_hash"`
	Status ReceiptStatus `json:"status"`

	FuelUsed      *big.Int         `json:"fuel_used"`
	StateHashes   map[Address]Hash `json:"state_hashes"`
	ContextHashes map[Address]Hash `json:"context_hashes"`
	ExtraData     json.RawMessage  `json:"extra_data"`
}

func (r *Receipt) Copy() *Receipt {
	receipt := *r

	receipt.StateHashes = make(map[Address]Hash)
	receipt.ContextHashes = make(map[Address]Hash)

	for key, value := range r.StateHashes {
		receipt.StateHashes[key] = value
	}

	for key, value := range r.ContextHashes {
		receipt.ContextHashes[key] = value
	}

	if len(r.ExtraData) > 0 {
		receipt.ExtraData = make(json.RawMessage, len(r.ExtraData))
		copy(receipt.ExtraData, r.ExtraData)
	}

	return &receipt
}

func (r *Receipt) IncreaseFuelUsed(fuel *big.Int) {
	r.FuelUsed = new(big.Int).Add(r.FuelUsed, fuel)
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

func (rs Receipts) Copy() Receipts {
	if len(rs) == 0 {
		return nil
	}

	receipts := make(Receipts)

	for key, value := range rs {
		receipts[key] = value.Copy()
	}

	return receipts
}

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
	AssetID      AssetID `json:"asset_id"`
	AssetAccount Address `json:"address"`
}

type AssetMintOrBurnReceipt struct {
	TotalSupply hexutil.Big `json:"total_supply"`
}

type LogicDeployReceipt struct {
	LogicID LogicID       `json:"logic_id,omitempty"`
	Error   hexutil.Bytes `json:"error"`
}

type LogicInvokeReceipt struct {
	Outputs hexutil.Bytes `json:"outputs"`
	Error   hexutil.Bytes `json:"error"`
}

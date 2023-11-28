package common

import (
	"encoding/json"

	"github.com/sarvalabs/go-moi/common/hexutil"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type ReceiptStatus uint64

const (
	ReceiptOk ReceiptStatus = iota
	ReceiptFailed
)

type Log struct {
	Addresses []Address
	LogicID   LogicID
	Topics    []Hash
	Data      []byte
}

type Receipt struct {
	IxType    IxType           `json:"ix_type"`
	IxHash    Hash             `json:"ix_hash"`
	Status    ReceiptStatus    `json:"status"`
	FuelUsed  uint64           `json:"fuel_used"`
	Hashes    ReceiptAccHashes `json:"hashes"`
	ExtraData json.RawMessage  `json:"extra_data"`
	Logs      []*Log           `polo:"-" json:"logs"`
}

func NewReceipt(ix *Interaction) *Receipt {
	return &Receipt{
		IxType:   ix.Type(),
		IxHash:   ix.Hash(),
		Hashes:   make(ReceiptAccHashes),
		FuelUsed: 0,
	}
}

type Hashes struct {
	StateHash   Hash `json:"state_hash"`
	ContextHash Hash `json:"context_hash"`
}

func (l *Log) Copy() *Log {
	log := *l

	if len(l.Addresses) > 0 {
		log.Addresses = make([]Address, len(l.Addresses))
		copy(log.Addresses, l.Addresses)
	}

	if len(l.Topics) > 0 {
		log.Topics = make([]Hash, len(l.Topics))
		copy(log.Topics, l.Topics)
	}

	if len(l.Data) > 0 {
		log.Data = make([]byte, len(l.Data))
		copy(log.Data, l.Data)
	}

	return &log
}

type ReceiptAccHashes map[Address]*Hashes

func (h ReceiptAccHashes) SetContextHash(addr Address, contextHash Hash) {
	hashes, ok := h[addr]
	if !ok {
		h[addr] = &Hashes{ContextHash: contextHash}

		return
	}

	hashes.ContextHash = contextHash
}

func (h ReceiptAccHashes) SetStateHash(addr Address, stateHash Hash) {
	hashes, ok := h[addr]

	if !ok {
		h[addr] = &Hashes{StateHash: stateHash}

		return
	}

	hashes.StateHash = stateHash
}

func (h ReceiptAccHashes) ContextHash(addr Address) Hash {
	hashes, ok := h[addr]
	if !ok {
		return NilHash
	}

	return hashes.ContextHash
}

func (h ReceiptAccHashes) StateHash(addr Address) Hash {
	hashes, ok := h[addr]
	if !ok {
		return NilHash
	}

	return hashes.StateHash
}

func (h ReceiptAccHashes) Copy() ReceiptAccHashes {
	if len(h) == 0 {
		return nil
	}

	hashmap := make(ReceiptAccHashes)

	for key, value := range h {
		hashmap[key] = &Hashes{
			StateHash:   value.StateHash,
			ContextHash: value.ContextHash,
		}
	}

	return hashmap
}

func (r *Receipt) Copy() *Receipt {
	receipt := *r

	receipt.FuelUsed = r.FuelUsed
	receipt.Hashes = r.Hashes.Copy()

	if len(r.ExtraData) > 0 {
		receipt.ExtraData = make(json.RawMessage, len(r.ExtraData))
		copy(receipt.ExtraData, r.ExtraData)
	}

	if len(r.Logs) > 0 {
		receipt.Logs = make([]*Log, len(r.Logs))

		for i, log := range r.Logs {
			if log != nil {
				receipt.Logs[i] = log.Copy()
			}
		}
	}

	return &receipt
}

func (r *Receipt) SetFuelUsed(fuel uint64) {
	r.FuelUsed = fuel
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

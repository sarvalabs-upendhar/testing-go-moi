package common

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/hexutil"
)

type ReceiptStatus uint64

const (
	ReceiptOk ReceiptStatus = iota
	ReceiptExceptionRaised
	ReceiptStateReverted
	ReceiptFuelExhausted
)

type Log struct {
	Addresses []identifiers.Address
	LogicID   identifiers.LogicID
	Topics    []Hash
	Data      []byte
}

func (l *Log) Copy() *Log {
	log := *l

	if len(l.Addresses) > 0 {
		log.Addresses = make([]identifiers.Address, len(l.Addresses))
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

type StateAndContextHash struct {
	StateHash   Hash `json:"state_hash"`
	ContextHash Hash `json:"context_hash"`
}

type AccStateHashes map[identifiers.Address]*StateAndContextHash

func (h AccStateHashes) SetContextHash(addr identifiers.Address, contextHash Hash) {
	hashes, ok := h[addr]
	if !ok {
		h[addr] = &StateAndContextHash{ContextHash: contextHash}

		return
	}

	hashes.ContextHash = contextHash
}

func (h AccStateHashes) SetStateHash(addr identifiers.Address, stateHash Hash) {
	hashes, ok := h[addr]

	if !ok {
		h[addr] = &StateAndContextHash{StateHash: stateHash}

		return
	}

	hashes.StateHash = stateHash
}

func (h AccStateHashes) ContextHash(addr identifiers.Address) Hash {
	hashes, ok := h[addr]
	if !ok {
		return NilHash
	}

	return hashes.ContextHash
}

func (h AccStateHashes) StateHash(addr identifiers.Address) Hash {
	hashes, ok := h[addr]
	if !ok {
		return NilHash
	}

	return hashes.StateHash
}

func (h AccStateHashes) Copy() AccStateHashes {
	if len(h) == 0 {
		return nil
	}

	hashmap := make(AccStateHashes)

	for key, value := range h {
		hashmap[key] = &StateAndContextHash{
			StateHash:   value.StateHash,
			ContextHash: value.ContextHash,
		}
	}

	return hashmap
}

type Receipt struct {
	IxType    IxType          `json:"ix_type"`
	IxHash    Hash            `json:"ix_hash"`
	Status    ReceiptStatus   `json:"status"`
	FuelUsed  uint64          `json:"fuel_used"`
	ExtraData json.RawMessage `json:"extra_data"`
	Logs      []*Log          `polo:"-" json:"logs"`
}

func NewReceipt(ix *Interaction) *Receipt {
	return &Receipt{
		IxType:   ix.Type(),
		IxHash:   ix.Hash(),
		FuelUsed: 0,
	}
}

func (r *Receipt) Copy() *Receipt {
	receipt := *r

	receipt.FuelUsed = r.FuelUsed

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

func SetReceiptExtraData[Payload ReceiptPayload](r *Receipt, payload Payload) {
	raw, _ := json.Marshal(payload)
	r.ExtraData = raw
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

type ReceiptPayload interface {
	AssetCreationReceipt | AssetMintOrBurnReceipt | LogicDeployReceipt | LogicInvokeReceipt
}

type AssetCreationReceipt struct {
	AssetID      identifiers.AssetID `json:"asset_id"`
	AssetAccount identifiers.Address `json:"address"`
}

type AssetMintOrBurnReceipt struct {
	TotalSupply hexutil.Big `json:"total_supply"`
}

type LogicDeployReceipt struct {
	LogicID identifiers.LogicID `json:"logic_id,omitempty"`
	Error   hexutil.Bytes       `json:"error"`
}

type LogicInvokeReceipt struct {
	Outputs hexutil.Bytes `json:"outputs"`
	Error   hexutil.Bytes `json:"error"`
}

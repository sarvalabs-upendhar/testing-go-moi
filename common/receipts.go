package common

import (
	"encoding/json"
	"sort"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common/hexutil"
)

type ReceiptStatus uint64

const (
	ReceiptOk ReceiptStatus = iota
	ReceiptStateReverted
	ReceiptInsufficientFuel
)

type IxOpStatus uint64

const (
	ResultOk IxOpStatus = iota
	ResultExceptionRaised
	ResultStateReverted
	ResultFuelExhausted
)

type Log struct {
	ID      identifiers.Identifier
	LogicID identifiers.LogicID
	Topics  []Hash
	Data    []byte
}

func (log Log) Copy() Log {
	clone := Log{
		ID:      log.ID,
		LogicID: log.LogicID,
	}

	if len(log.Topics) > 0 {
		clone.Topics = make([]Hash, len(log.Topics))
		copy(clone.Topics, log.Topics)
	}

	if len(log.Data) > 0 {
		clone.Data = make([]byte, len(log.Data))
		copy(clone.Data, log.Data)
	}

	return clone
}

type StateAndContextHash struct {
	StateHash   Hash `json:"state_hash"`
	ContextHash Hash `json:"context_hash"`
}

type AccStateHashes map[identifiers.Identifier]*StateAndContextHash

func (h AccStateHashes) SetContextHash(id identifiers.Identifier, contextHash Hash) {
	hashes, ok := h[id]
	if !ok {
		h[id] = &StateAndContextHash{ContextHash: contextHash}

		return
	}

	hashes.ContextHash = contextHash
}

func (h AccStateHashes) SetStateHash(id identifiers.Identifier, stateHash Hash) {
	hashes, ok := h[id]

	if !ok {
		h[id] = &StateAndContextHash{StateHash: stateHash}

		return
	}

	hashes.StateHash = stateHash
}

func (h AccStateHashes) ContextHash(id identifiers.Identifier) Hash {
	hashes, ok := h[id]
	if !ok {
		return NilHash
	}

	return hashes.ContextHash
}

func (h AccStateHashes) StateHash(id identifiers.Identifier) Hash {
	hashes, ok := h[id]
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

func (h AccStateHashes) ExcludedAccounts() IdentifierList {
	ids := make(IdentifierList, 0, len(h))

	for id, hashes := range h {
		// Account is excluded if state hash is excluded
		if hashes.StateHash == NilHash {
			ids = append(ids, id)
		}
	}

	sort.Sort(ids)

	return ids
}

// IxOpResult represents the outcome of a ixn execution.
type IxOpResult struct {
	IxType IxOpType        `json:"ix_type"`
	Status IxOpStatus      `json:"status"`
	Data   json.RawMessage `json:"data"`
	Logs   []Log           `json:"logs"`
}

// NewIxOpResult initializes and returns a new IxOpResult with the given op type.
func NewIxOpResult(ixType IxOpType) *IxOpResult {
	return &IxOpResult{
		IxType: ixType,
		Logs:   make([]Log, 0),
	}
}

// Copy creates a deep copy of the IxOpResult.
func (r *IxOpResult) Copy() *IxOpResult {
	result := *r

	if len(r.Data) > 0 {
		result.Data = make(json.RawMessage, len(r.Data))
		copy(result.Data, r.Data)
	}

	if len(r.Logs) > 0 {
		result.Logs = make([]Log, len(r.Logs))

		for i, log := range r.Logs {
			result.Logs[i] = log.Copy()
		}
	}

	return &result
}

// SetLogs assigns the given logs to the IxOpResult.
func (r *IxOpResult) SetLogs(logs []Log) {
	copies := make([]Log, len(logs))

	for i, log := range logs {
		copies[i] = log.Copy()
	}

	r.Logs = append(r.Logs, copies...)
}

// SetStatus sets the operation status.
func (r *IxOpResult) SetStatus(status IxOpStatus) {
	r.Status = status
}

func (r *IxOpResult) WithStatus(status IxOpStatus) *IxOpResult {
	r.Status = status

	return r
}

// SetResultPayload serializes the payload and assigns it to the Data field of the IxOpResult.
func SetResultPayload[Payload OperationResultPayload](op *IxOpResult, payload Payload) {
	raw, _ := json.Marshal(payload)
	op.Data = raw
}

// Receipt represents the outcome of an interaction.
type Receipt struct {
	IxHash   Hash          `json:"ix_hash"`
	Status   ReceiptStatus `json:"status"`
	FuelUsed uint64        `json:"fuel_used"`
	IxOps    []*IxOpResult `json:"ix_operations"`
}

// NewReceipt initializes and returns a new Receipt for the given interaction.
func NewReceipt(ix *Interaction) *Receipt {
	return &Receipt{
		IxHash: ix.Hash(),
		IxOps:  make([]*IxOpResult, 0),
	}
}

// Copy creates a deep copy of the Receipt.
func (r *Receipt) Copy() *Receipt {
	receipt := *r

	receipt.FuelUsed = r.FuelUsed

	if len(r.IxOps) > 0 {
		receipt.IxOps = make([]*IxOpResult, len(r.IxOps))

		for i, op := range r.IxOps {
			receipt.IxOps[i] = op.Copy()
		}
	}

	return &receipt
}

// SetFuelUsed sets the amount of fuel used.
func (r *Receipt) SetFuelUsed(fuel uint64) {
	r.FuelUsed = fuel
}

// SetStatus sets the receipt status.
func (r *Receipt) SetStatus(status ReceiptStatus) {
	r.Status = status
}

// AddIxOpResult adds ixOpResult to the Receipt.
func (r *Receipt) AddIxOpResult(op *IxOpResult) {
	r.IxOps = append(r.IxOps, op.Copy())
}

// Logs aggregate and return logs from all ops in the Receipt.
func (r *Receipt) Logs() []Log {
	logs := make([]Log, 0)

	for _, op := range r.IxOps {
		logs = append(logs, op.Logs...)
	}

	return logs
}

type Receipts map[Hash]*Receipt

// Copy creates and returns a deep copy of the Receipts
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

// Hash computes and returns the hash of the Receipts.
func (rs Receipts) Hash() (Hash, error) {
	hash, err := PoloHash(rs)
	if err != nil {
		return NilHash, errors.Wrap(err, "failed to polorize receipts")
	}

	return hash, nil
}

// GetReceipt retrieves a Receipt by its interaction hash.
func (rs Receipts) GetReceipt(ixHash Hash) (*Receipt, error) {
	if receipt, ok := rs[ixHash]; ok {
		return receipt, nil
	}

	return nil, ErrReceiptNotFound
}

// Bytes serializes the Receipts and returns the bytes.
func (rs Receipts) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(rs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize receipts")
	}

	return rawData, nil
}

// FromBytes deserializes the Receipts from bytes.
func (rs *Receipts) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(rs, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize receipts")
	}

	return nil
}

// FuelUsed calculates and returns the total fuel used from receipts.
func (rs Receipts) FuelUsed() (fuelUsed uint64) {
	for _, receipt := range rs {
		fuelUsed += receipt.FuelUsed
	}

	return fuelUsed
}

type OperationResultPayload interface {
	AssetCreationResult | AssetSupplyResult | LogicDeployResult | LogicInvokeResult | LogicEnlistResult
}

// AssetCreationResult holds the result of asset creation operation.
type AssetCreationResult struct {
	AssetID identifiers.AssetID `json:"asset_id"`
}

// AssetSupplyResult holds the result of asset mint or burn operation.
type AssetSupplyResult struct {
	TotalSupply hexutil.Big `json:"total_supply"`
}

// LogicDeployResult holds the result of logic deploy operation.
type LogicDeployResult struct {
	LogicID identifiers.LogicID `json:"logic_id,omitempty"`
	Error   hexutil.Bytes       `json:"error"`
}

// LogicInvokeResult holds the result of logic invoke operation.
type LogicInvokeResult struct {
	Outputs hexutil.Bytes `json:"outputs"`
	Error   hexutil.Bytes `json:"error"`
}

// LogicEnlistResult holds the result of logic enlist operation.
type LogicEnlistResult struct {
	Outputs hexutil.Bytes `json:"outputs"`
	Error   hexutil.Bytes `json:"error"`
}

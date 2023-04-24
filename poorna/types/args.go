package types

import (
	"encoding/json"
	"sort"

	"github.com/sarvalabs/moichain/mudra/kramaid"

	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/types"
)

const (
	LatestTesseractHeight = -1
)

type TesseractNumberOrHash struct {
	TesseractNumber *int64      `json:"tesseract_number"`
	TesseractHash   *types.Hash `json:"tesseract_hash"`
}

func (t *TesseractNumberOrHash) Number() (int64, error) {
	if t.TesseractNumber == nil {
		return 0, types.ErrEmptyHeight
	}

	if *t.TesseractNumber < LatestTesseractHeight { // if tesseract number less than -1 then it is invalid
		return 0, types.ErrInvalidHeight
	}

	return *t.TesseractNumber, nil
}

func (t *TesseractNumberOrHash) Hash() (types.Hash, bool) {
	if t.TesseractHash == nil {
		return types.NilHash, false
	}

	return *t.TesseractHash, true
}

type RPCAssetDescriptor struct {
	Type   types.AssetKind `json:"type"`
	Symbol string          `json:"symbol"`
	Owner  types.Address   `json:"owner"`
	Supply hexutil.Big     `json:"supply"`

	Dimension hexutil.Uint8 `json:"dimension"`
	Decimals  hexutil.Uint8 `json:"decimals"`

	IsFungible     bool `json:"is_fungible"`
	IsMintable     bool `json:"is_mintable"`
	IsTransferable bool `json:"is_transferable"`

	LogicID types.LogicID `json:"logic_id"`
}

type RPCAccount struct {
	Nonce   hexutil.Uint64    `json:"nonce"`
	AccType types.AccountType `json:"acc_type"`

	Balance        types.Hash `json:"balance"`
	AssetApprovals types.Hash `json:"asset_approvals"`
	ContextHash    types.Hash `json:"context_hash"`
	StorageRoot    types.Hash `json:"storage_root"`
	LogicRoot      types.Hash `json:"logic_root"`
	FileRoot       types.Hash `json:"file_root"`
}

type RPCAccountMetaInfo struct {
	Type types.AccountType `json:"type"`

	Address types.Address  `json:"address"`
	Height  hexutil.Uint64 `json:"height"`

	TesseractHash types.Hash `json:"tesseract_hash"`
	LatticeExists bool       `json:"lattice_exists"`
	StateExists   bool       `json:"state_exists"`
}

type RPCStateHash struct {
	Address types.Address `json:"address"`
	Hash    types.Hash    `json:"hash"`
}

type RPCStateHashes []RPCStateHash

func (stateHashes RPCStateHashes) Sort() {
	sort.Slice(stateHashes, func(i, j int) bool {
		return stateHashes[i].Address.Hex() < stateHashes[j].Address.Hex()
	})
}

type RPCContextHash struct {
	Address types.Address `json:"address"`
	Hash    types.Hash    `json:"hash"`
}

type RPCContextHashes []RPCContextHash

func (contextHashes RPCContextHashes) Sort() {
	sort.Slice(contextHashes, func(i, j int) bool {
		return contextHashes[i].Address.Hex() < contextHashes[j].Address.Hex()
	})
}

type RPCReceipt struct {
	IxType        hexutil.Uint64   `json:"ix_type"`
	IxHash        types.Hash       `json:"ix_hash"`
	FuelUsed      hexutil.Uint64   `json:"fuel_used"`
	StateHashes   RPCStateHashes   `json:"state_hashes"`
	ContextHashes RPCContextHashes `json:"context_hashes"`
	ExtraData     json.RawMessage  `json:"extra_data"`
}

type RPCInteraction struct {
	Type  types.IxType   `json:"type"`
	Nonce hexutil.Uint64 `json:"nonce"`

	Sender   types.Address `json:"sender"`
	Receiver types.Address `json:"receiver"`
	Payer    types.Address `json:"payer"`

	TransferValues  map[types.AssetID]string `json:"transfer_values"`
	PerceivedValues map[types.AssetID]string `json:"perceived_values"`
	PerceivedProofs hexutil.Bytes            `json:"perceived_proofs"`

	FuelPrice *hexutil.Big `json:"fuel_price"`
	FuelLimit *hexutil.Big `json:"fuel_limit"`

	Payload json.RawMessage `json:"payload"`

	Mode         hexutil.Uint64    `json:"mode"`
	ComputeHash  hexutil.Bytes     `json:"compute_hash"`
	ComputeNodes []kramaid.KramaID `json:"compute_nodes"`

	MTQ        hexutil.Uint64    `json:"mtq"`
	TrustNodes []kramaid.KramaID `json:"trust_nodes"`

	Hash      types.Hash    `json:"hash"`
	Signature hexutil.Bytes `json:"signature"`
}

type RPCInteractions []*RPCInteraction

type RPCTesseractPart struct {
	Address types.Address  `json:"address"`
	Hash    types.Hash     `json:"hash"`
	Height  hexutil.Uint64 `json:"height"`
}

type RPCTesseractParts []RPCTesseractPart

func (parts RPCTesseractParts) Sort() {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Address.Hex() < parts[j].Address.Hex()
	})
}

type RPCContextLockInfo struct {
	Address       types.Address  `json:"address"`
	ContextHash   types.Hash     `json:"context_hash"`
	Height        hexutil.Uint64 `json:"height"`
	TesseractHash types.Hash     `json:"tesseract_hash"`
}

type RPCContextLockInfos []RPCContextLockInfo

func (infos RPCContextLockInfos) Sort() {
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Address.Hex() < infos[j].Address.Hex()
	})
}

type RPCDeltaGroup struct {
	Address          types.Address         `json:"address"`
	Role             types.ParticipantRole `json:"role"`
	BehaviouralNodes []kramaid.KramaID     `json:"behavioural_nodes"`
	RandomNodes      []kramaid.KramaID     `json:"random_nodes"`
	ReplacedNodes    []kramaid.KramaID     `json:"replaced_nodes"`
}

type RPCDeltaGroups []RPCDeltaGroup

func (groups RPCDeltaGroups) Sort() {
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Address.Hex() < groups[j].Address.Hex()
	})
}

type RPCTesseractGridID struct {
	Hash  types.Hash        `json:"hash"`
	Total hexutil.Uint64    `json:"total"`
	Parts RPCTesseractParts `json:"parts"`
}

type RPCCommitData struct {
	Round           hexutil.Uint64      `json:"round"`
	CommitSignature hexutil.Bytes       `json:"commit_signature"`
	VoteSet         string              `json:"vote_set"`
	EvidenceHash    types.Hash          `json:"evidence_hash"`
	GridID          *RPCTesseractGridID `json:"grid_id"`
}

type RPCHeader struct {
	Address     types.Address       `json:"address"`
	PrevHash    types.Hash          `json:"prev_hash"`
	Height      hexutil.Uint64      `json:"height"`
	FuelUsed    hexutil.Uint64      `json:"fuel_used"`
	FuelLimit   hexutil.Uint64      `json:"fuel_limit"`
	BodyHash    types.Hash          `json:"body_hash"`
	GridHash    types.Hash          `json:"grid_hash"`
	Operator    string              `json:"operator"`
	ClusterID   string              `json:"cluster_id"`
	Timestamp   hexutil.Uint64      `json:"timestamp"`
	ContextLock RPCContextLockInfos `json:"context_lock"`
	Extra       RPCCommitData       `json:"extra"`
}

type RPCBody struct {
	StateHash       types.Hash     `json:"state_hash"`
	ContextHash     types.Hash     `json:"context_hash"`
	InteractionHash types.Hash     `json:"interaction_hash"`
	ReceiptHash     types.Hash     `json:"receipt_hash"`
	ContextDelta    RPCDeltaGroups `json:"context_delta"` // Some Problem here
	ConsensusProof  types.PoXCData `json:"consensus_proof"`
}

type RPCTesseract struct {
	Header RPCHeader       `json:"header"`
	Body   RPCBody         `json:"body"`
	Ixns   RPCInteractions `json:"ixns"`
	Seal   hexutil.Bytes   `json:"seal"`
	Hash   types.Hash      `json:"hash"`
}

func (ts *RPCTesseract) Address() types.Address {
	return ts.Header.Address
}

// TesseractArgs is an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	Address          types.Address         `json:"address"` // Address for which to retrieve the latest Tesseract
	WithInteractions bool                  `json:"with_interactions"`
	Options          TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	Address types.Address         `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type AssetDescriptorArgs struct {
	AssetID string `json:"asset_id"`
}

type InteractionCountArgs struct {
	Address types.Address         `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type IxPoolArgs struct {
	Address types.Address `json:"address"`
}

type InspectArgs struct{}

type StatusArgs struct{}

type ContentArgs struct{}

type NetArgs struct{}

type AccountArgs struct{}

type DebugArgs struct {
	Key string `json:"storage_key"`
}

type GetStorageArgs struct {
	LogicID    string                `json:"logic_id"`
	StorageKey string                `json:"storage_key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type GetAccountArgs struct {
	Address types.Address         `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  string                `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	Address types.Address         `json:"address"`  // Address for which to retrieve the balance
	AssetID string                `json:"asset_id"` // Asset for which to retrieve balance
	Options TesseractNumberOrHash `json:"options"`
}

// SendIXArgs is an argument wrapper for sending Interactions to the pool
type SendIXArgs struct {
	Type  types.IxType   `json:"type"`
	Nonce hexutil.Uint64 `json:"nonce"`

	Sender   types.Address `json:"sender"`
	Receiver types.Address `json:"receiver"`
	Payer    types.Address `json:"payer"`

	TransferValues  map[types.AssetID]string `json:"transfer_values"`
	PerceivedValues map[types.AssetID]string `json:"perceived_values"`

	FuelPrice *hexutil.Big `json:"fuel_price"`
	FuelLimit *hexutil.Big `json:"fuel_limit"`

	Payload json.RawMessage `json:"payload"`
}

type RPCAssetCreation struct {
	Type types.AssetKind `json:"type"`

	Symbol string       `json:"symbol"`
	Supply *hexutil.Big `json:"supply"`

	Dimension hexutil.Uint8 `json:"dimension"`
	Decimals  hexutil.Uint8 `json:"decimals"`

	IsFungible     bool `json:"is_fungible"`
	IsMintable     bool `json:"is_mintable"`
	IsTransferable bool `json:"is_transferable"`

	LogicID string `json:"logic_id,omitempty"`
	// LogicCode []byte `json:"logic_code,omitempty"`
}

type RPCLogicPayload struct {
	Manifest hexutil.Bytes `json:"manifest"`
	LogicID  string        `json:"logic_id"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
}

// Response wrapper
type Response struct {
	Status string          `json:"status,omitempty"`
	Data   json.RawMessage `json:"data"`
	Error  *JSONError      `json:"error,omitempty"`
}

// ContextResponse is response object for fetching context info
type ContextResponse struct {
	BehaviourNodes []string `json:"behaviour_nodes"`
	RandomNodes    []string `json:"random_nodes"`
	StorageNodes   []string `json:"storage_nodes"`
}

// ReceiptArgs is an argument wrapper for retrieving the receipt of an interaction
type ReceiptArgs struct {
	Hash types.Hash `json:"hash"`
}

// InteractionArg is a struct that represents a single interaction
type InteractionArg struct {
	Nonce     hexutil.Uint64 `json:"nonce"`
	Type      hexutil.Uint64 `json:"type"`
	Sender    types.Address  `json:"sender"`
	Receiver  types.Address  `json:"receiver"`
	Cost      *hexutil.Big   `json:"cost"`
	FuelPrice *hexutil.Big   `json:"fuel_price"`
	FuelLimit *hexutil.Big   `json:"fuel_limit"`
	Input     string         `json:"input"`
	Hash      types.Hash     `json:"hash"`
}

// NewInteractionArg is a contructor function that generates and returns a new InteractionArg for a given Interaction
func NewInteractionArg(ix *types.Interaction) *InteractionArg {
	return &InteractionArg{
		Nonce:     hexutil.Uint64(ix.Nonce()),
		Type:      hexutil.Uint64(ix.Type()),
		Sender:    ix.Sender(),
		Receiver:  ix.Receiver(),
		Cost:      (*hexutil.Big)(ix.Cost()),
		FuelPrice: (*hexutil.Big)(ix.FuelPrice()),
		FuelLimit: (*hexutil.Big)(ix.FuelLimit()),
		Input:     types.BytesToHex(ix.Payload()),
		Hash:      ix.Hash(),
	}
}

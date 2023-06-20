package types

import (
	"encoding/json"
	"sort"

	"github.com/sarvalabs/moichain/mudra/kramaid"

	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/types"
)

var LatestTesseractHeight int64 = -1

// RPC args

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

// TesseractArgs is an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	Address          types.Address         `json:"address"` // Address for which to retrieve the latest Tesseract
	WithInteractions bool                  `json:"with_interactions"`
	Options          TesseractNumberOrHash `json:"options"`
}

type QueryArgs struct {
	Address types.Address         `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	Address types.Address         `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
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
	LogicID    types.LogicID         `json:"logic_id"`
	StorageKey hexutil.Bytes         `json:"storage_key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type GetAssetInfoArgs struct {
	AssetID types.AssetID         `json:"asset_id"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetAccountArgs struct {
	Address types.Address         `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetLogicIDArgs struct {
	Address types.Address         `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  types.LogicID         `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	Address types.Address         `json:"address"`  // Address for which to retrieve the balance
	AssetID types.AssetID         `json:"asset_id"` // Asset for which to retrieve balance
	Options TesseractNumberOrHash `json:"options"`
}

type SendIX struct {
	IXArgs    string `json:"ix_args"`
	Signature string `json:"signature"`
}

type RPCAssetCreation struct {
	Symbol string       `json:"symbol"`
	Supply *hexutil.Big `json:"supply"`

	Dimension *hexutil.Uint8  `json:"dimension"`
	Standard  *hexutil.Uint16 `json:"standard"`

	IsLogical  bool `json:"is_logical"`
	IsStateful bool `json:"is_stateful"`

	Logic *RPCLogicPayload `json:"logic_code,omitempty"`
}

type RPCAssetMintOrBurn struct {
	AssetID types.AssetID `json:"asset_id"`
	Amount  *hexutil.Big  `json:"amount"`
}

type RPCLogicPayload struct {
	Manifest hexutil.Bytes `json:"manifest"`
	LogicID  string        `json:"logic_id"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
}

func (l *RPCLogicPayload) LogicPayload() *types.LogicPayload {
	return &types.LogicPayload{
		Manifest: l.Manifest.Bytes(),
		Logic:    types.LogicID(l.LogicID),
		Calldata: l.Calldata,
		Callsite: l.Callsite,
	}
}

func RPClogicPayloadFromLogicPayload(payload *types.LogicPayload) *RPCLogicPayload {
	if payload == nil {
		return nil
	}

	return &RPCLogicPayload{
		Manifest: (hexutil.Bytes)(payload.Manifest),
		LogicID:  payload.Logic.String(),
		Callsite: payload.Callsite,
		Calldata: (hexutil.Bytes)(payload.Calldata),
	}
}

type InteractionByHashArgs struct {
	Hash types.Hash `json:"hash"`
}

type InteractionByTesseract struct {
	Address types.Address         `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
	IxIndex *hexutil.Uint64       `json:"ix_index"`
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

// RPC Responses

type RPCRegistry struct {
	AssetID   string             `json:"asset_id"`
	AssetInfo RPCAssetDescriptor `json:"asset_info"`
}

type RPCAssetDescriptor struct {
	Symbol   string        `json:"symbol"`
	Operator types.Address `json:"operator"`
	Supply   hexutil.Big   `json:"supply"`

	Dimension hexutil.Uint8  `json:"dimension"`
	Standard  hexutil.Uint16 `json:"standard"`

	IsLogical  bool `json:"is_logical"`
	IsStateFul bool `json:"is_stateful"`

	LogicID types.LogicID `json:"logic_id,omitempty"`
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

type Hashes struct {
	Address     types.Address `json:"address"`
	StateHash   types.Hash    `json:"state_hash"`
	ContextHash types.Hash    `json:"context_hash"`
}

type RPCHashes []Hashes

func (hashes RPCHashes) Sort() {
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].Address.Hex() < hashes[j].Address.Hex()
	})
}

type RPCReceipt struct {
	IxType    hexutil.Uint64      `json:"ix_type"`
	IxHash    types.Hash          `json:"ix_hash"`
	Status    types.ReceiptStatus `json:"status"`
	FuelUsed  hexutil.Big         `json:"fuel_used"`
	Hashes    RPCHashes           `json:"hashes"`
	ExtraData json.RawMessage     `json:"extra_data"`
	From      types.Address       `json:"from"`
	To        types.Address       `json:"to"`
	IXIndex   hexutil.Uint64      `json:"ix_index"`
	Parts     RPCTesseractParts   `json:"parts"`
}

type RPCInteraction struct {
	Type  types.IxType   `json:"type"`
	Nonce hexutil.Uint64 `json:"nonce"`

	Sender   types.Address `json:"sender"`
	Receiver types.Address `json:"receiver"`
	Payer    types.Address `json:"payer"`

	TransferValues  map[types.AssetID]*hexutil.Big `json:"transfer_values"`
	PerceivedValues map[types.AssetID]*hexutil.Big `json:"perceived_values"`
	PerceivedProofs hexutil.Bytes                  `json:"perceived_proofs"`

	FuelPrice *hexutil.Big `json:"fuel_price"`
	FuelLimit *hexutil.Big `json:"fuel_limit"`

	Payload json.RawMessage `json:"payload"`

	Mode         hexutil.Uint64    `json:"mode"`
	ComputeHash  types.Hash        `json:"compute_hash"`
	ComputeNodes []kramaid.KramaID `json:"compute_nodes"`

	MTQ        hexutil.Uint64    `json:"mtq"`
	TrustNodes []kramaid.KramaID `json:"trust_nodes"`

	Hash      types.Hash        `json:"hash"`
	Signature hexutil.Bytes     `json:"signature"`
	Parts     RPCTesseractParts `json:"parts"`
	IxIndex   hexutil.Uint64    `json:"ix_index"`
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
	FuelUsed    hexutil.Big         `json:"fuel_used"`
	FuelLimit   hexutil.Big         `json:"fuel_limit"`
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

// InteractionResponse is a struct that represents a single interaction
type InteractionResponse struct {
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

// NewInteractionResponse is a contructor function that generates
// and returns a new InteractionResponse for a given Interaction
func NewInteractionResponse(ix *types.Interaction) *InteractionResponse {
	return &InteractionResponse{
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

func GetRPCAssetDescriptor(ad *types.AssetDescriptor) RPCAssetDescriptor {
	return RPCAssetDescriptor{
		Symbol:     ad.Symbol,
		Operator:   ad.Operator,
		Dimension:  hexutil.Uint8(ad.Dimension),
		Standard:   hexutil.Uint16(ad.Standard),
		Supply:     (hexutil.Big)(*ad.Supply),
		IsLogical:  ad.IsLogical,
		IsStateFul: ad.IsStateFul,
		LogicID:    ad.LogicID,
	}
}

type TDU struct {
	AssetID types.AssetID `json:"asset_id"`
	Amount  *hexutil.Big  `json:"amount"`
}

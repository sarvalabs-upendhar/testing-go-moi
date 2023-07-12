package args

import (
	"encoding/json"
	"sort"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/hexutil"
	"github.com/sarvalabs/moichain/common/kramaid"
)

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
	Hash common.Hash `json:"hash"`
}

// RPC Responses

type RPCRegistry struct {
	AssetID   string             `json:"asset_id"`
	AssetInfo RPCAssetDescriptor `json:"asset_info"`
}

type RPCAssetDescriptor struct {
	Symbol   string         `json:"symbol"`
	Operator common.Address `json:"operator"`
	Supply   hexutil.Big    `json:"supply"`

	Dimension hexutil.Uint8  `json:"dimension"`
	Standard  hexutil.Uint16 `json:"standard"`

	IsLogical  bool `json:"is_logical"`
	IsStateFul bool `json:"is_stateful"`

	LogicID common.LogicID `json:"logic_id,omitempty"`
}

type RPCAccount struct {
	Nonce   hexutil.Uint64     `json:"nonce"`
	AccType common.AccountType `json:"acc_type"`

	Balance        common.Hash `json:"balance"`
	AssetApprovals common.Hash `json:"asset_approvals"`
	ContextHash    common.Hash `json:"context_hash"`
	StorageRoot    common.Hash `json:"storage_root"`
	LogicRoot      common.Hash `json:"logic_root"`
	FileRoot       common.Hash `json:"file_root"`
}

type RPCAccountMetaInfo struct {
	Type common.AccountType `json:"type"`

	Address common.Address `json:"address"`
	Height  hexutil.Uint64 `json:"height"`

	TesseractHash common.Hash `json:"tesseract_hash"`
	LatticeExists bool        `json:"lattice_exists"`
	StateExists   bool        `json:"state_exists"`
}

type Hashes struct {
	Address     common.Address `json:"address"`
	StateHash   common.Hash    `json:"state_hash"`
	ContextHash common.Hash    `json:"context_hash"`
}

type RPCHashes []Hashes

func (hashes RPCHashes) Sort() {
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].Address.Hex() < hashes[j].Address.Hex()
	})
}

type RPCReceipt struct {
	IxType    hexutil.Uint64       `json:"ix_type"`
	IxHash    common.Hash          `json:"ix_hash"`
	Status    common.ReceiptStatus `json:"status"`
	FuelUsed  hexutil.Big          `json:"fuel_used"`
	Hashes    RPCHashes            `json:"hashes"`
	ExtraData json.RawMessage      `json:"extra_data"`
	From      common.Address       `json:"from"`
	To        common.Address       `json:"to"`
	IXIndex   hexutil.Uint64       `json:"ix_index"`
	Parts     RPCTesseractParts    `json:"parts"`
}

type RPCInteraction struct {
	Type  common.IxType  `json:"type"`
	Nonce hexutil.Uint64 `json:"nonce"`

	Sender   common.Address `json:"sender"`
	Receiver common.Address `json:"receiver"`
	Payer    common.Address `json:"payer"`

	TransferValues  map[common.AssetID]*hexutil.Big `json:"transfer_values"`
	PerceivedValues map[common.AssetID]*hexutil.Big `json:"perceived_values"`
	PerceivedProofs hexutil.Bytes                   `json:"perceived_proofs"`

	FuelPrice *hexutil.Big `json:"fuel_price"`
	FuelLimit *hexutil.Big `json:"fuel_limit"`

	Payload json.RawMessage `json:"payload"`

	Mode         hexutil.Uint64    `json:"mode"`
	ComputeHash  common.Hash       `json:"compute_hash"`
	ComputeNodes []kramaid.KramaID `json:"compute_nodes"`

	MTQ        hexutil.Uint64    `json:"mtq"`
	TrustNodes []kramaid.KramaID `json:"trust_nodes"`

	Hash      common.Hash       `json:"hash"`
	Signature hexutil.Bytes     `json:"signature"`
	Parts     RPCTesseractParts `json:"parts"`
	IxIndex   hexutil.Uint64    `json:"ix_index"`
}

type RPCInteractions []*RPCInteraction

type RPCTesseractPart struct {
	Address common.Address `json:"address"`
	Hash    common.Hash    `json:"hash"`
	Height  hexutil.Uint64 `json:"height"`
}

type RPCTesseractParts []RPCTesseractPart

func (parts RPCTesseractParts) Sort() {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Address.Hex() < parts[j].Address.Hex()
	})
}

type RPCContextLockInfo struct {
	Address       common.Address `json:"address"`
	ContextHash   common.Hash    `json:"context_hash"`
	Height        hexutil.Uint64 `json:"height"`
	TesseractHash common.Hash    `json:"tesseract_hash"`
}

type RPCContextLockInfos []RPCContextLockInfo

func (infos RPCContextLockInfos) Sort() {
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Address.Hex() < infos[j].Address.Hex()
	})
}

type RPCDeltaGroup struct {
	Address          common.Address         `json:"address"`
	Role             common.ParticipantRole `json:"role"`
	BehaviouralNodes []kramaid.KramaID      `json:"behavioural_nodes"`
	RandomNodes      []kramaid.KramaID      `json:"random_nodes"`
	ReplacedNodes    []kramaid.KramaID      `json:"replaced_nodes"`
}

type RPCDeltaGroups []RPCDeltaGroup

func (groups RPCDeltaGroups) Sort() {
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Address.Hex() < groups[j].Address.Hex()
	})
}

type RPCTesseractGridID struct {
	Hash  common.Hash       `json:"hash"`
	Total hexutil.Uint64    `json:"total"`
	Parts RPCTesseractParts `json:"parts"`
}

type RPCCommitData struct {
	Round           hexutil.Uint64      `json:"round"`
	CommitSignature hexutil.Bytes       `json:"commit_signature"`
	VoteSet         string              `json:"vote_set"`
	EvidenceHash    common.Hash         `json:"evidence_hash"`
	GridID          *RPCTesseractGridID `json:"grid_id"`
}

type RPCHeader struct {
	Address     common.Address      `json:"address"`
	PrevHash    common.Hash         `json:"prev_hash"`
	Height      hexutil.Uint64      `json:"height"`
	FuelUsed    hexutil.Big         `json:"fuel_used"`
	FuelLimit   hexutil.Big         `json:"fuel_limit"`
	BodyHash    common.Hash         `json:"body_hash"`
	GridHash    common.Hash         `json:"grid_hash"`
	Operator    string              `json:"operator"`
	ClusterID   string              `json:"cluster_id"`
	Timestamp   hexutil.Uint64      `json:"timestamp"`
	ContextLock RPCContextLockInfos `json:"context_lock"`
	Extra       RPCCommitData       `json:"extra"`
}

type RPCBody struct {
	StateHash       common.Hash     `json:"state_hash"`
	ContextHash     common.Hash     `json:"context_hash"`
	InteractionHash common.Hash     `json:"interaction_hash"`
	ReceiptHash     common.Hash     `json:"receipt_hash"`
	ContextDelta    RPCDeltaGroups  `json:"context_delta"` // Some Problem here
	ConsensusProof  common.PoXtData `json:"consensus_proof"`
}

type RPCTesseract struct {
	Header RPCHeader       `json:"header"`
	Body   RPCBody         `json:"body"`
	Ixns   RPCInteractions `json:"ixns"`
	Seal   hexutil.Bytes   `json:"seal"`
	Hash   common.Hash     `json:"hash"`
}

func (ts *RPCTesseract) Address() common.Address {
	return ts.Header.Address
}

// InteractionResponse is a struct that represents a single interaction
type InteractionResponse struct {
	Nonce     hexutil.Uint64 `json:"nonce"`
	Type      hexutil.Uint64 `json:"type"`
	Sender    common.Address `json:"sender"`
	Receiver  common.Address `json:"receiver"`
	Cost      *hexutil.Big   `json:"cost"`
	FuelPrice *hexutil.Big   `json:"fuel_price"`
	FuelLimit *hexutil.Big   `json:"fuel_limit"`
	Input     string         `json:"input"`
	Hash      common.Hash    `json:"hash"`
}

// NewInteractionResponse is a contructor function that generates
// and returns a new InteractionResponse for a given Interaction
func NewInteractionResponse(ix *common.Interaction) *InteractionResponse {
	return &InteractionResponse{
		Nonce:     hexutil.Uint64(ix.Nonce()),
		Type:      hexutil.Uint64(ix.Type()),
		Sender:    ix.Sender(),
		Receiver:  ix.Receiver(),
		Cost:      (*hexutil.Big)(ix.Cost()),
		FuelPrice: (*hexutil.Big)(ix.FuelPrice()),
		FuelLimit: (*hexutil.Big)(ix.FuelLimit()),
		Input:     common.BytesToHex(ix.Payload()),
		Hash:      ix.Hash(),
	}
}

func GetRPCAssetDescriptor(ad *common.AssetDescriptor) RPCAssetDescriptor {
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
	AssetID common.AssetID `json:"asset_id"`
	Amount  *hexutil.Big   `json:"amount"`
}

type NodeInfoResponse struct {
	KramaID kramaid.KramaID `json:"krama_id"`
}

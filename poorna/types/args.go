package types

import (
	"encoding/json"
	"math/big"
	"sort"

	"github.com/sarvalabs/moichain/types"
)

const (
	LatestTesseractHeight = -1
)

type TesseractNumberOrHash struct {
	TesseractNumber *int64  `json:"tesseract_number"`
	TesseractHash   *string `json:"tesseract_hash"`
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

func (t *TesseractNumberOrHash) Hash() (string, bool) {
	if t.TesseractHash == nil {
		return "", false
	}

	return *t.TesseractHash, true
}

type RPCInteraction struct {
	Input     types.IxInput   `json:"input"`
	Compute   types.IxCompute `json:"compute"`
	Trust     types.IxTrust   `json:"trust"`
	Hash      types.Hash      `json:"hash"`
	Signature []byte          `json:"signature"`
}

type RPCInteractions []*RPCInteraction

type RPCTesseractPart struct {
	Address types.Address `json:"address"`
	Hash    types.Hash    `json:"hash"`
	Height  uint64        `json:"height"`
}

type RPCTesseractParts []RPCTesseractPart

func (parts RPCTesseractParts) Sort() {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Address.Hex() < parts[j].Address.Hex()
	})
}

type RPCTesseractGridID struct {
	Hash  types.Hash        `json:"hash"`
	Total int32             `json:"total"`
	Parts RPCTesseractParts `json:"parts"`
}

type RPCCommitData struct {
	Round           int32               `json:"round"`
	CommitSignature []byte              `json:"commit_signature"`
	VoteSet         *types.ArrayOfBits  `json:"vote_set"`
	EvidenceHash    types.Hash          `json:"evidence_hash"`
	GridID          *RPCTesseractGridID `json:"grid_id"`
}

type RPCHeader struct {
	Address     types.Address                           `json:"address"`
	PrevHash    types.Hash                              `json:"prev_hash"`
	Height      uint64                                  `json:"height"`
	FuelUsed    uint64                                  `json:"fuel_used"`
	FuelLimit   uint64                                  `json:"fuel_limit"`
	BodyHash    types.Hash                              `json:"body_hash"`
	GridHash    types.Hash                              `json:"grid_hash"`
	Operator    string                                  `json:"operator"`
	ClusterID   string                                  `json:"cluster_id"`
	Timestamp   int64                                   `json:"timestamp"`
	ContextLock map[types.Address]types.ContextLockInfo `json:"context_lock"`
	Extra       RPCCommitData                           `json:"extra"`
}

type RPCTesseract struct {
	Header   RPCHeader           `json:"header"`
	Body     types.TesseractBody `json:"body"`
	Ixns     RPCInteractions     `json:"ixns"`
	Receipts types.Receipts      `json:"receipts"`
	Seal     []byte              `json:"seal"`
	Hash     types.Hash          `json:"hash"`
}

func (ts *RPCTesseract) Address() types.Address {
	return ts.Header.Address
}

func (ts *RPCTesseract) Height() uint64 {
	return ts.Header.Height
}

// TesseractArgs is an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	Address          string                `json:"address"` // Address for which to retrieve the latest Tesseract
	WithInteractions bool                  `json:"with_interactions"`
	Options          TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	Address string                `json:"address"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type TesseractByHashArgs struct {
	Hash             string `json:"hash"`
	WithInteractions bool   `json:"with_interactions"`
}

type TesseractByHeightArgs struct {
	Address          string `json:"address"`
	Height           uint64 `json:"height"`
	WithInteractions bool   `json:"with_interactions"`
}

type AssetDescriptorArgs struct {
	AssetID string `json:"asset_id"`
}

type InteractionCountArgs struct {
	Address string                `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type IxPoolArgs struct {
	Address string `json:"address"`
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
	Address string                `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID  string                `json:"logic_id"`
	Encoding string                `json:"encoding"`
	Options  TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	Address string                `json:"address"`  // Address for which to retrieve the balance
	AssetID string                `json:"asset_id"` // Asset for which to retrieve balance
	Options TesseractNumberOrHash `json:"options"`
}

// SendIXArgs is an argument wrapper for sending Interactions to the pool
type SendIXArgs struct {
	Type  types.IxType `json:"type"`
	Nonce uint64       `json:"nonce"`

	Sender   string `json:"sender"`
	Receiver string `json:"receiver"`
	Payer    string `json:"payer"`

	TransferValues  map[types.AssetID]string `json:"transfer_values"`
	PerceivedValues map[types.AssetID]string `json:"perceived_values"`

	FuelPrice string `json:"fuel_price"`
	FuelLimit string `json:"fuel_limit"`

	Payload json.RawMessage `json:"payload"`
}

type AssetCreationArgs struct {
	Type types.AssetKind `json:"type"`

	Symbol string `json:"symbol"`
	Supply string `json:"supply"`

	Dimension uint8 `json:"dimension"`
	Decimals  uint8 `json:"decimals"`

	IsFungible     bool `json:"is_fungible"`
	IsMintable     bool `json:"is_mintable"`
	IsTransferable bool `json:"is_transferable"`

	LogicID string `json:"logic_id,omitempty"`
	// LogicCode []byte `json:"logic_code,omitempty"`
}

type LogicDeployArgs struct {
	Manifest string `json:"manifest"`
	Callsite string `json:"callsite"`
	Calldata string `json:"calldata"`
}

type LogicInvokeArgs struct {
	LogicID  string `json:"logic_id"`
	Callsite string `json:"callsite"`
	Calldata string `json:"calldata"`
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
	Hash string `json:"hash"`
}

// InteractionArg is a struct that represents a single interaction
type InteractionArg struct {
	Nonce     uint64   `json:"nonce"`
	Type      uint64   `json:"type"`
	Sender    string   `json:"sender"`
	Receiver  string   `json:"receiver"`
	Cost      *big.Int `json:"cost"`
	FuelPrice *big.Int `json:"fuel_price"`
	FuelLimit *big.Int `json:"fuel_limit"`
	Input     string   `json:"input"`
	Hash      string   `json:"hash"`
}

// NewInteractionArg is a contructor function that generates and returns a new InteractionArg for a given Interaction
func NewInteractionArg(ix *types.Interaction) *InteractionArg {
	return &InteractionArg{
		Nonce:     ix.Nonce(),
		Type:      uint64(ix.Type()),
		Sender:    ix.Sender().Hex(),
		Receiver:  ix.Receiver().Hex(),
		Cost:      ix.Cost(),
		FuelPrice: ix.FuelPrice(),
		FuelLimit: ix.FuelLimit(),
		Input:     types.BytesToHex(ix.Payload()),
		Hash:      ix.Hash().Hex(),
	}
}

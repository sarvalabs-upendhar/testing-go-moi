package types

import (
	"encoding/json"
	"math/big"

	"github.com/sarvalabs/moichain/types"
)

const (
	LatestTesseractHeight = -1
)

type TesseractNumberOrHash struct {
	TesseractNumber *int64  `json:"tesseract_number"`
	TesseractHash   *string `json:"tesseract_hash"`
}

type RPCInteraction struct {
	Input     types.IxInput
	Compute   types.IxCompute
	Trust     types.IxTrust
	Hash      types.Hash
	Signature []byte
}

type RPCInteractions []*RPCInteraction

type RPCTesseract struct {
	Header   types.TesseractHeader
	Body     types.TesseractBody
	Ixns     RPCInteractions
	Receipts types.Receipts
	Seal     []byte
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

// TesseractArgs is an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	From             string                `json:"from"` // Address for which to retrieve the latest Tesseract
	WithInteractions bool                  `json:"with_interactions"`
	Options          TesseractNumberOrHash `json:"options"`
}

type ContextInfoArgs struct {
	From    string                `json:"from"` // Address for which to retrieve the latest Tesseract
	Options TesseractNumberOrHash `json:"options"`
}

type TesseractByHashArgs struct {
	Hash             string `json:"hash"`
	WithInteractions bool   `json:"with_interactions"`
}

type TesseractByHeightArgs struct {
	From             string `json:"from"`
	Height           uint64 `json:"height"`
	WithInteractions bool   `json:"with_interactions"`
}

type AssetDescriptorArgs struct {
	AssetID string `json:"asset_id"`
}

type InteractionCountArgs struct {
	From    string                `json:"from"`
	Options TesseractNumberOrHash `json:"options"`
}

type IxPoolArgs struct {
	From string `json:"from"`
}

type NetArgs struct{}

type DebugArgs struct {
	Key string `json:"storage_key"`
}

type GetStorageArgs struct {
	LogicID    string                `json:"logic_id"`
	StorageKey string                `json:"storage-key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type GetAccountArgs struct {
	Address string                `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type LogicManifestArgs struct {
	LogicID string                `json:"logic_id"`
	Options TesseractNumberOrHash `json:"options"`
}

// BalArgs is an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	From    string                `json:"from"`    // Address for which to retrieve the balance
	AssetID string                `json:"assetid"` // Asset for which to retrieve balance
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
	Type          types.LogicKind `json:"type"`
	IsStateFul    bool            `json:"is_stateful"`
	IsInteractive bool            `json:"is_interactive"`
	Manifest      string          `json:"manifest"`
	CallData      string          `json:"calldata"`
}

type LogicExecuteArgs struct {
	LogicID  string `json:"logic_id"`
	CallSite string `json:"callsite"`
	CallData string `json:"calldata"`
}

// Response wrapper
type Response struct {
	Status string          `json:"status,omitempty"`
	Data   json.RawMessage `json:"data"`
	Error  *JSONError      `json:"error,omitempty"`
}

// ContextResponse is response object for fetching context info
type ContextResponse struct {
	BehaviourNodes []string
	RandomNodes    []string
	StorageNodes   []string
}

// ReceiptArgs is an argument wrapper for retrieving the receipt of an interaction
type ReceiptArgs struct {
	Hash string
}

// InteractionArg is a struct that represents a single interaction
type InteractionArg struct {
	Nonce     uint64
	Type      uint64
	Sender    string
	Receiver  string
	Cost      *big.Int
	FuelPrice *big.Int
	FuelLimit *big.Int
	Input     string
	Hash      string
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

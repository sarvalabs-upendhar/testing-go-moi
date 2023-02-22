package api

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

type GetStorageArgs struct {
	LogicID    string                `json:"logic_id"`
	StorageKey string                `json:"storage-key"`
	Options    TesseractNumberOrHash `json:"options"`
}

type GetAccountArgs struct {
	Address string                `json:"address"`
	Options TesseractNumberOrHash `json:"options"`
}

type GetLogicManifestArgs struct {
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
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data"`
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

// ReceiptResponse is response wrapper for receipts
type ReceiptResponse struct {
	Receipt types.Receipt
}

// CommitData is a struct that represents arbitrary commit data of a Tesseract
type CommitData struct {
	// Represents the validation round
	Round int32
	// Represents the commits of the Tesseract
	CommitSignature []byte
	// Represents the hash of the evidence collected by the observer
	EvidenceHash string
	// Represents the pre commit vote set
	VoteSet *types.ArrayOfBits
}

// PoXCData is a struct that represents Proof of Context data
type PoXCData struct {
	// Represents the hash of validators in the ICS
	ICSHash string
	// Represents the binary hash of the context
	BinaryHash string
	// Represents the identity hash of context participants
	IdentityHash string
}

// TesseractHeader is a struct that represents a Tesseract header
type TesseractHeader struct {
	// Represents the address of the the Tesseract
	Address string
	// Represents the hash of the previous Tesseract
	PrevHash string
	// Represents the context lock hashes
	ContextLock map[types.Address]types.ContextLockInfo
	// Represents the height of the Tesseract in the lattice
	Height uint64
	// Represents the timestamp of Tesseract generation
	Timestamp int64
	// Represents the amount of ANU used in the Tesseract
	AnuUsed uint64
	// Represents the ANU limit of the Tesseract
	AnuLimit uint64
	// Represents the hash of the Tesseract body
	BodyHash string
	// Represent the group hash of all Tesseracts in the ICS
	GroupHash string
	// Represents the address of the Tesseract operator
	Operator string
	// Represents the id of the Tesseract ICS
	IcsID string
	// Represents the Tesseract commit data
	Extra CommitData
}

// TesseractBody is a struct that represents a Tesseract body
type TesseractBody struct {
	// Represents the hash of the Tesseract state
	StateHash string
	// Represents the hash of the Tesseract context
	ContextHash string
	// Represents the hash of the Tesseract interactions
	InteractionHash string
	// Represents the hash of the Tesseract receipt
	ReceiptHash string
	// Represents the context delta of the Tesseract
	ContextDelta map[string]*types.DeltaGroup
	// Represents the consensus proof of the Tesseract
	ConsensusProof PoXCData
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

// TesseractArg is a struct that represents a Tesseract
type TesseractArg struct {
	// Represents the header of the Tesseract
	Header TesseractHeader
	// Represents the body of the Tesseract
	Body TesseractBody
	// Represents the Interactions in the Tesseract
	Interactions []InteractionArg
}

// NewTesseractArg is a constructor function that generates and returns a new TesseractArg for a given Tesseract
func NewTesseractArg(t *types.Tesseract, withInteractions bool) TesseractArg {
	var tesseract TesseractArg

	tesseract.Header = TesseractHeader{
		Address:     t.Header.Address.Hex(),
		PrevHash:    t.Header.PrevHash.Hex(),
		Height:      t.Header.Height,
		ContextLock: make(map[types.Address]types.ContextLockInfo),
		Timestamp:   t.Header.Timestamp,
		AnuUsed:     t.Header.AnuUsed,
		AnuLimit:    t.Header.AnuLimit,
		BodyHash:    t.Header.BodyHash.Hex(),
		GroupHash:   t.Header.GridHash.Hex(),
		Operator:    t.Header.Operator,
		IcsID:       t.Header.ClusterID,
		Extra: CommitData{
			Round:           t.Header.Extra.Round,
			CommitSignature: t.Header.Extra.CommitSignature,
			EvidenceHash:    t.Header.Extra.EvidenceHash.Hex(),
			VoteSet:         t.Header.Extra.VoteSet,
		},
	}

	tesseract.Body = TesseractBody{
		StateHash:       t.Body.StateHash.Hex(),
		ContextHash:     t.Body.ContextHash.Hex(),
		InteractionHash: t.Body.InteractionHash.Hex(),
		ReceiptHash:     t.Body.ReceiptHash.Hex(),
		ContextDelta:    make(map[string]*types.DeltaGroup),
		ConsensusProof: PoXCData{
			BinaryHash:   t.Body.ConsensusProof.BinaryHash.Hex(),
			IdentityHash: t.Body.ConsensusProof.IdentityHash.Hex(),
			ICSHash:      t.Body.ConsensusProof.ICSHash.Hex(),
		},
	}

	if withInteractions && len(t.Ixns) > 0 {
		tesseract.Interactions = make([]InteractionArg, 0)
		for _, ix := range t.Ixns {
			tesseract.Interactions = append(tesseract.Interactions, *NewInteractionArg(ix))
		}
	}

	for k, v := range t.Header.ContextLock {
		tesseract.Header.ContextLock[k] = v
	}
	// Accumulate the Tesseract context delta into the TesseractArg
	for k, v := range t.Body.ContextDelta {
		tesseract.Body.ContextDelta[k.Hex()] = v
	}

	return tesseract
}

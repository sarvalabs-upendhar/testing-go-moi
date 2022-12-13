package api

import (
	"encoding/json"

	"github.com/sarvalabs/moichain/types"
)

// TesseractArgs is a struct that represents an argument wrapper for retrieving the latest Tesseract
type TesseractArgs struct {
	// Represents the address for which to retrieve the latest Tesseract
	From             string `json:"from"`
	WithInteractions bool   `json:"with_interactions"`
}

type ContextInfoByHashArgs struct {
	// Represents the address for which to retrieve the latest Tesseract
	From string `json:"from"`
	Hash string `json:"hash"`
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
	From   string `json:"from"`
	Status bool   `json:"status"`
}

type GetStorageArgs struct {
	LogicID    string `json:"logic_id"`
	StorageKey string `json:"storage-key"`
}

// BalArgs is a struct that represents an argument wrapper for retrieving balance of an asset
type BalArgs struct {
	// Represents the address for which to retrieve the balance
	From string `json:"from"`

	// Represents the asset for which to retrieve balance
	AssetID string `json:"assetid"`
}

// SendIXArgs is a struct that represents an argument wrapper for sending Interactions to the pool
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
	Manifest      []byte          `json:"manifest"`
	CallData      []byte          `json:"calldata"`
}

type LogicExecuteArgs struct {
	LogicID  string `json:"logic_id"`
	CallSite string `json:"callsite"`
	CallData []byte `json:"calldata"`
}

// Response is a struct that represents a response wrapper
type Response struct {
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data"`
}

// ContextResponse is a response object for fetching context info
type ContextResponse struct {
	BehaviourNodes []string
	RandomNodes    []string
	StorageNodes   []string
}

// ReceiptArgs is a struct that represent an argument wrapper for retrieving the receipt of an interaction
type ReceiptArgs struct {
	Address string
	Hash    string
}

// ReceiptResponse is a response wrapper for receipts
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

// TesseractArg is a struct that represents a Tesseract
type TesseractArg struct {
	// Represents the header of the Tesseract
	Header TesseractHeader
	// Represents the body of the Tesseract
	Body TesseractBody
	// Represents the Interactions in the Tesseract
	Interactions types.Interactions
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

	if withInteractions {
		tesseract.Interactions = t.Ixns
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

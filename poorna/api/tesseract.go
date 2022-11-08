package api

import (
	"gitlab.com/sarvalabs/moichain/types"
)

// TesseractArg is a struct that represents a Tesseract
type TesseractArg struct {
	// Represents the header of the Tesseract
	Header TesseractHeader
	// Represents the body of the Tesseract
	Body TesseractBody
}

// TesseractHeader is a struct that represents a Tesseract header
type TesseractHeader struct {
	// Represents the address of the the Tesseract
	Address string
	// Represents the hash of the previous Tesseract
	PrevHash string
	// Represents the context lock hashes
	ContextLock map[types.Address]types.ContextLockInfo
	// Represents the height of the Tesseract in the chain
	Height uint64
	// Represents the timestamp of Tesseract generation
	Timestamp int64
	// Represents the amount of ANU used in the Tesseract
	AnuUsed uint64
	// Represents the ANU limit of the Tesseract
	AnuLimit uint64
	// Represents the hash of the Tesseract
	TesseractHash string
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
	// Represents the Interactions in the Tesseract
	Interactions types.Interactions
	// Represents the context delta of the Tesseract
	ContextDelta map[string]*types.DeltaGroup
	// Represents the consensus proof of the Tesseract
	ConsensusProof PoXCData
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

// NewTesseractArg is a constructor function that generates and returns a new TesseractArg for a given Tesseract
func NewTesseractArg(t *types.Tesseract) TesseractArg {
	header := TesseractHeader{
		Address:       t.Header.Address.Hex(),
		PrevHash:      t.Header.PrevHash.Hex(),
		Height:        t.Header.Height,
		ContextLock:   make(map[types.Address]types.ContextLockInfo),
		Timestamp:     t.Header.Timestamp,
		AnuUsed:       t.Header.AnuUsed,
		AnuLimit:      t.Header.AnuLimit,
		TesseractHash: t.Header.TesseractHash.Hex(),
		GroupHash:     t.Header.GridHash.Hex(),
		Operator:      t.Header.Operator,
		IcsID:         t.Header.ClusterID,
		Extra: CommitData{
			Round:           t.Header.Extra.Round,
			CommitSignature: t.Header.Extra.CommitSignature,
			EvidenceHash:    t.Header.Extra.EvidenceHash.Hex(),
			VoteSet:         t.Header.Extra.VoteSet,
		},
	}

	body := TesseractBody{
		StateHash:       t.Body.StateHash.Hex(),
		ContextHash:     t.Body.ContextHash.Hex(),
		InteractionHash: t.Body.InteractionHash.Hex(),
		ReceiptHash:     t.Body.ReceiptHash.Hex(),
		Interactions:    t.Body.Interactions,
		ContextDelta:    make(map[string]*types.DeltaGroup),
		ConsensusProof: PoXCData{
			BinaryHash:   t.Body.ConsensusProof.BinaryHash.Hex(),
			IdentityHash: t.Body.ConsensusProof.IdentityHash.Hex(),
			ICSHash:      t.Body.ConsensusProof.ICSHash.Hex(),
		},
	}

	// Create the TesseractArg from the header and body generated using the Tesseract object
	tesseract := TesseractArg{header, body}

	for k, v := range t.Header.ContextLock {
		tesseract.Header.ContextLock[k] = v
	}
	// Accumulate the Tesseract context delta into the TesseractArg
	for k, v := range t.Body.ContextDelta {
		tesseract.Body.ContextDelta[k.Hex()] = v
	}

	return tesseract
}

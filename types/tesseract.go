package types

import (
	"sync"

	"github.com/mr-tron/base58"

	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
)

const (
	Sender ParticipantRole = iota
	Receiver
	Genesis
)

type ContextDelta map[Address]*DeltaGroup

type DeltaGroup struct {
	Role             ParticipantRole
	BehaviouralNodes []id.KramaID
	RandomNodes      []id.KramaID
	ReplacedNodes    []id.KramaID
}

type ContextLockInfo struct {
	ContextHash   Hash
	Height        uint64
	TesseractHash Hash
}

type Tesseract struct {
	Header TesseractHeader
	Body   TesseractBody
	Ixns   Interactions
	Seal   []byte
}

type TesseractHeader struct {
	Address       Address
	PrevHash      Hash
	Height        uint64
	AnuUsed       uint64
	AnuLimit      uint64
	TesseractHash Hash
	GridHash      Hash
	Operator      string
	ClusterID     string
	Timestamp     int64
	ContextLock   map[Address]ContextLockInfo
	Extra         CommitData
}

type TesseractBody struct {
	StateHash       Hash
	ContextHash     Hash
	InteractionHash Hash
	ReceiptHash     Hash
	ContextDelta    ContextDelta // Some Problem here
	ConsensusProof  PoXCData
}

type PoXCData struct {
	BinaryHash   Hash
	IdentityHash Hash
	ICSHash      Hash
}

type CommitData struct {
	Round           int32
	CommitSignature []byte
	VoteSet         *ArrayOfBits
	EvidenceHash    Hash
	GridID          *TesseractGridID
}

func (t *Tesseract) GridLength() int32 {
	return t.Header.Extra.GridID.Parts.Total
}

func (t *Tesseract) Operator() string {
	return t.Header.Operator
}

func (t *Tesseract) GetICSHash() Hash {
	return t.Body.ConsensusProof.ICSHash
}

func (t *Tesseract) BodyHash() (Hash, error) {
	return PoloHash(t.Body)
}

func (t *Tesseract) Hash() (Hash, error) {
	protoHeader := new(TesseractHeader)
	protoHeader.ContextLock = t.Header.ContextLock
	protoHeader.Address = t.Header.Address
	protoHeader.PrevHash = t.Header.PrevHash
	protoHeader.Height = t.Header.Height
	protoHeader.AnuUsed = t.Header.AnuUsed
	protoHeader.AnuLimit = t.Header.AnuLimit
	protoHeader.TesseractHash = t.Header.TesseractHash
	protoHeader.GridHash = t.Header.GridHash
	protoHeader.Operator = t.Header.Operator
	protoHeader.ClusterID = t.Header.ClusterID
	protoHeader.Timestamp = t.Header.Timestamp

	data, err := polo.Polorize(protoHeader)
	if err != nil {
		return Hash{}, err
	}

	return GetHash(data), nil
}

func (t *Tesseract) Interactions() Interactions {
	return t.Ixns
}

func (t *Tesseract) ContextDelta() ContextDelta {
	return t.Body.ContextDelta
}

func (t *Tesseract) Address() Address {
	return t.Header.Address
}

func (t *Tesseract) ContextHash() Hash {
	return t.Body.ContextHash
}

func (t *Tesseract) PreviousHash() Hash {
	return t.Header.PrevHash
}

func (t *Tesseract) InteractionHash() Hash {
	return t.Body.InteractionHash
}

func (t *Tesseract) ReceiptHash() Hash {
	return t.Body.ReceiptHash
}

func (t *Tesseract) GridHash() Hash {
	return t.Header.GridHash
}

func (t *Tesseract) ClusterID() ClusterID {
	return ClusterID(t.Header.ClusterID)
}

func (t *Tesseract) StateHash() Hash {
	return t.Body.StateHash
}

func (t *Tesseract) Height() uint64 {
	return t.Header.Height
}

func (t *Tesseract) Bytes() ([]byte, error) {
	c := t.CanonicalWithoutSeal()

	return polo.Polorize(c)
}

// CanonicalWithoutSeal method returns a copy of the tesseract without seal and interactions
func (t *Tesseract) CanonicalWithoutSeal() *CanonicalTesseractWithoutSeal {
	return &CanonicalTesseractWithoutSeal{
		Header: t.Header,
		Body:   t.Body,
	}
}

// Canonical method returns a copy of the tesseract without interactions
func (t *Tesseract) Canonical() *CanonicalTesseract {
	return &CanonicalTesseract{
		Header: t.Header,
		Body:   t.Body,
		Seal:   t.Seal,
	}
}

type CanonicalTesseract struct {
	Header TesseractHeader
	Body   TesseractBody
	Seal   []byte
}

// Bytes method serializes and returns the canonical tesseract in bytes
func (c *CanonicalTesseract) Bytes() ([]byte, error) {
	return polo.Polorize(c)
}

type CanonicalTesseractWithoutSeal struct {
	Header TesseractHeader
	Body   TesseractBody
}

type Item struct {
	Tesseract *Tesseract
	Delta     map[Hash][]byte
	Sender    id.KramaID
}

type TesseractStack struct {
	Items []*Item
	lock  sync.Mutex
}

func (s *TesseractStack) Push(t *Item) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Items = append(s.Items, t)
}

func (s *TesseractStack) Pop() *Item {
	s.lock.Lock()
	defer s.lock.Unlock()

	if size := len(s.Items); size > 0 {
		item := s.Items[size-1]
		s.Items = s.Items[:size-1]

		return item
	}

	return nil
}

func (s *TesseractStack) Len() int32 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return int32(len(s.Items))
}

type TesseractParts struct {
	Total   int32
	Hashes  []Hash
	Heights []uint64
}

type TesseractGridID struct {
	Hash  Hash
	Parts *TesseractParts
}

func (tid *TesseractGridID) IsNil() bool {
	return tid.Hash.IsNil() && len(tid.Parts.Hashes) == 0
}

func (tid *TesseractGridID) String() string {
	if !tid.IsNil() {
		return tid.Hash.Hex()
	}

	return "Nil"
}

// ClusterID ...
type ClusterID string

func (c ClusterID) String() string {
	return string(c)
}

func (c ClusterID) Hash() Hash {
	rawHash, err := base58.Decode(c.String())
	if err != nil {
		return NilHash
	}

	return BytesToHash(rawHash)
}

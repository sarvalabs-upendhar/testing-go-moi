package types

import (
	"sync"

	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
)

type Tesseract struct {
	Header TesseractHeader
	Body   TesseractBody
	Seal   []byte
}

type ContextDelta map[Address]*DeltaGroup

type DeltaGroup struct {
	Role             ParticipantRole
	BehaviouralNodes []id.KramaID
	RandomNodes      []id.KramaID
	ReplacedNodes    []id.KramaID
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
	Interactions    Interactions
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

type CanonicalTesseract struct {
	Header TesseractHeader
	Body   TesseractBody
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

func (t *Tesseract) Canonical() *CanonicalTesseract {
	return &CanonicalTesseract{
		Header: t.Header,
		Body:   t.Body,
	}
}

func (t *Tesseract) BodyHash() Hash {
	return PoloHash(t.Body)
}

func (t *Tesseract) Hash() Hash {
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

	data := polo.Polorize(protoHeader)

	return GetHash(data)
}

func (t *Tesseract) Interactions() Interactions {
	return t.Body.Interactions
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

func (t *Tesseract) Bytes() []byte {
	c := t.Canonical()

	return polo.Polorize(c)
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

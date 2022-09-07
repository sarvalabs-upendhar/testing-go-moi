package ktypes

import (
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"
	"sync"
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
	GroupHash     Hash
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
	ContextDelta    ContextDelta //Some Problem here
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

/*
func TesseractFromProto(t *ktypes.Tesseract) *Tesseract {
	tnew := &Tesseract{
		Header: TesseractHeader{
			Address:       ktypes.BytesToAddress(t.Header.Address),
			PrevHash:      ktypes.BytesToHash(t.Header.PrevHash),
			Height:        t.Header.Height,
			AnuUsed:       t.Header.AnuUsed,
			AnuLimit:      t.Header.AnuLimit,
			Timestamp:     t.Header.Timestamp,
			TesseractHash: ktypes.BytesToHash(t.Header.TesseractHash),
			GroupHash:     ktypes.BytesToHash(t.Header.GroupHash),
			Operator:      t.Header.Operator,
			ClusterID:     t.Header.ClusterID,
			Extra: CommitData{
				Round:   t.Header.Extra.Round,
				Commits: t.Header.Extra.Commits,
				Seal:    t.Header.Extra.Seal,
				VoteSet: t.Header.Extra.Voteset,
				EvidenceHash:  ktypes.BytesToHash(t.Header.Extra.Evidence),
			},
		},
		Body: TesseractBody{
			StateHash:       ktypes.BytesToHash(t.Body.StateHash),
			ContextHash:     ktypes.BytesToHash(t.Body.ContextHash),
			InteractionHash: ktypes.BytesToHash(t.Body.InteractionHash),
			ReceiptHash:     ktypes.BytesToHash(t.Body.ReceiptHash),
			Interactions:    t.Body.Interactions,
			ContextDelta:    make(map[ktypes.Address][]id.KramaID),

			ConsensusProof: PoXCData{
				IdentityHash: ktypes.BytesToHash(t.Body.ConsensusProof.IdentityHash),
				BinaryHash:   ktypes.BytesToHash(t.Body.ConsensusProof.BinaryHash),
				ICSHash:      ktypes.BytesToHash(t.Body.ConsensusProof.IcsHash),
			},
		},
		Signature: t.Signature,
	}
	for k, v := range t.Body.ContextDelta {
		tnew.Body.ContextDelta[ktypes.HexToAddress(k)] = ktypes.ToKIPPeerID(v.List)
	}

	return tnew
}
*/

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
	protoHeader.GroupHash = t.Header.GroupHash
	protoHeader.Operator = t.Header.Operator
	protoHeader.ClusterID = t.Header.ClusterID
	protoHeader.Timestamp = t.Header.Timestamp

	//protoHeader.Extra = &ktypes.CommitData{
	//	Round:   t.Header.Extra.Round,
	//	Seal:    t.Header.Extra.Seal,
	//	Commits: nil,
	//}

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

func (t *Tesseract) StateHash() Hash {
	return t.Body.StateHash
}

func (t *Tesseract) Height() uint64 {
	return t.Header.Height
}

/*
func (t *Tesseract) ToProto() *ktypes.Tesseract {
	tproto := &ktypes.Tesseract{
		Header: &ktypes.TesseractHeader{
			Address:       t.Header.Address.Bytes(),
			PrevHash:      t.Header.PrevHash.Bytes(),
			Height:        t.Header.Height,
			AnuUsed:       t.Header.AnuUsed,
			AnuLimit:      t.Header.AnuLimit,
			TesseractHash: t.Header.TesseractHash.Bytes(),
			GroupHash:     t.Header.GroupHash.Bytes(),
			Operator:      t.Header.Operator,
			Timestamp:     t.Header.Timestamp,
			ClusterID:     t.Header.ClusterID,
			Extra: &ktypes.CommitData{
				Round:   t.Header.Extra.Round,
				Seal:    t.Header.Extra.Seal,
				Commits: t.Header.Extra.Commits,
				Voteset: t.Header.Extra.VoteSet,
				Evidence: t.Header.Extra.EvidenceHash.Bytes(),
			},
		},
		Body: &ktypes.TesseractBody{
			StateHash:       t.Body.StateHash.Bytes(),
			ContextHash:     t.Body.ContextHash.Bytes(),
			InteractionHash: t.Body.InteractionHash.Bytes(),
			ReceiptHash:     t.Body.ReceiptHash.Bytes(),
			Interactions: &ktypes.InteractionsData{
				Ixns: t.Body.Interactions,
			},
			ContextDelta: t.ContextDeltaToProto(),
			ConsensusProof: &ktypes.PoXCData{
				BinaryHash:   t.Body.ConsensusProof.BinaryHash.Bytes(),
				IdentityHash: t.Body.ConsensusProof.IdentityHash.Bytes(),
				IcsHash:      t.Body.ConsensusProof.ICSHash.Bytes(),
			},
		},
		Signature: t.Signature,
	}

	return tproto
}

*/
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

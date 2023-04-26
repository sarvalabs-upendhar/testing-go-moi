package types

import (
	"sync"
	"sync/atomic"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/mudra/kramaid"
)

type ParticipantRole int

const (
	Sender ParticipantRole = iota
	Receiver
	Genesis
)

type ContextDelta map[Address]*DeltaGroup

func (ctx ContextDelta) Copy() ContextDelta {
	if len(ctx) == 0 {
		return nil
	}

	contextDelta := make(ContextDelta)

	for key, value := range ctx {
		contextDelta[key] = value.Copy()
	}

	return contextDelta
}

type DeltaGroup struct {
	Role             ParticipantRole   `json:"role"`
	BehaviouralNodes []kramaid.KramaID `json:"behavioural_nodes"`
	RandomNodes      []kramaid.KramaID `json:"random_nodes"`
	ReplacedNodes    []kramaid.KramaID `json:"replaced_nodes"`
}

func (d DeltaGroup) Copy() *DeltaGroup {
	deltaGroup := &DeltaGroup{
		Role: d.Role,
	}

	if len(d.BehaviouralNodes) > 0 {
		deltaGroup.BehaviouralNodes = make([]kramaid.KramaID, len(d.BehaviouralNodes))
		copy(deltaGroup.BehaviouralNodes, d.BehaviouralNodes)
	}

	if len(d.RandomNodes) > 0 {
		deltaGroup.RandomNodes = make([]kramaid.KramaID, len(d.RandomNodes))
		copy(deltaGroup.RandomNodes, d.RandomNodes)
	}

	if len(d.ReplacedNodes) > 0 {
		deltaGroup.ReplacedNodes = make([]kramaid.KramaID, len(d.ReplacedNodes))
		copy(deltaGroup.ReplacedNodes, d.ReplacedNodes)
	}

	return deltaGroup
}

type ContextLockInfo struct {
	ContextHash   Hash   `json:"context_hash"`
	Height        uint64 `json:"height"`
	TesseractHash Hash   `json:"tesseract_hash"`
}

type Tesseract struct {
	header   TesseractHeader
	body     TesseractBody
	ixns     Interactions
	receipts Receipts
	seal     []byte
	hash     atomic.Value
}

type TesseractHeader struct {
	Address     Address
	PrevHash    Hash
	Height      uint64
	FuelUsed    uint64
	FuelLimit   uint64
	BodyHash    Hash
	GroupHash   Hash
	Operator    string
	ClusterID   string
	Timestamp   int64
	ContextLock map[Address]ContextLockInfo
	Extra       CommitData
}

func (h *TesseractHeader) Copy() TesseractHeader {
	header := *h

	if len(h.ContextLock) > 0 {
		header.ContextLock = make(map[Address]ContextLockInfo, len(h.ContextLock))

		for k, v := range h.ContextLock {
			header.ContextLock[k] = v
		}
	}

	header.Extra = h.Extra.Copy()

	return header
}

func (h *TesseractHeader) Hash() (Hash, error) {
	header := TesseractHeader{
		Address:     h.Address,
		PrevHash:    h.PrevHash,
		Height:      h.Height,
		FuelUsed:    h.FuelUsed,
		FuelLimit:   h.FuelLimit,
		BodyHash:    h.BodyHash,
		GroupHash:   h.GroupHash,
		Operator:    h.Operator,
		ClusterID:   h.ClusterID,
		Timestamp:   h.Timestamp,
		ContextLock: h.ContextLock,
	}

	data, err := polo.Polorize(header)
	if err != nil {
		return Hash{}, errors.Wrap(err, "failed to polorize tesseract header")
	}

	return GetHash(data), nil
}

type TesseractBody struct {
	StateHash       Hash         `json:"state_hash"`
	ContextHash     Hash         `json:"context_hash"`
	InteractionHash Hash         `json:"interaction_hash"`
	ReceiptHash     Hash         `json:"receipt_hash"`
	ContextDelta    ContextDelta `json:"context_delta"` // Some Problem here
	ConsensusProof  PoXCData     `json:"consensus_proof"`
}

func (b *TesseractBody) Copy() TesseractBody {
	body := *b
	body.ContextDelta = b.ContextDelta.Copy()

	return body
}

func (b *TesseractBody) Hash() (Hash, error) {
	hash, err := PoloHash(b)
	if err != nil {
		return NilHash, errors.Wrap(err, "failed to polorize tesseract body")
	}

	return hash, nil
}

type PoXCData struct {
	BinaryHash   Hash `json:"binary_hash"`
	IdentityHash Hash `json:"identity_hash"`
	ICSHash      Hash `json:"ics_hash"`
}

type CommitData struct {
	Round           int32
	CommitSignature []byte
	VoteSet         *ArrayOfBits
	EvidenceHash    Hash
	GridID          *TesseractGridID
}

func (commitData *CommitData) Copy() CommitData {
	data := *commitData

	if len(commitData.CommitSignature) > 0 {
		data.CommitSignature = make([]byte, len(data.CommitSignature))

		copy(data.CommitSignature, commitData.CommitSignature)
	}

	if commitData.VoteSet != nil {
		data.VoteSet = commitData.VoteSet.Copy()
	}

	if commitData.GridID != nil {
		data.GridID = commitData.GridID.Copy()
	}

	return data
}

func NewTesseract(
	header TesseractHeader,
	body TesseractBody,
	ixns Interactions,
	receipts Receipts,
	seal []byte,
) *Tesseract {
	bytes := make([]byte, len(seal))
	copy(bytes, seal)

	return &Tesseract{
		header:   header.Copy(),
		body:     body.Copy(),
		ixns:     ixns,
		receipts: receipts.Copy(),
		seal:     bytes,
	}
}

func (t *Tesseract) SetExtraData(data CommitData) {
	t.header.Extra = data
}

func (t *Tesseract) SetSeal(seal []byte) {
	t.seal = seal
}

func (t *Tesseract) SetReceipts(receipts Receipts) {
	t.receipts = receipts.Copy()
}

func (t *Tesseract) Header() TesseractHeader {
	return t.header.Copy()
}

func (t *Tesseract) Body() TesseractBody {
	return t.body.Copy()
}

func (t *Tesseract) Interactions() Interactions {
	return t.ixns
}

func (t *Tesseract) Receipts() Receipts {
	return t.receipts.Copy()
}

func (t *Tesseract) HasReceipts() bool {
	return t.receipts != nil
}

func (t *Tesseract) Seal() []byte {
	bytes := make([]byte, len(t.seal))

	copy(bytes, t.seal)

	return bytes
}

func (t *Tesseract) Address() Address {
	return t.header.Address
}

func (t *Tesseract) PrevHash() Hash {
	return t.header.PrevHash
}

func (t *Tesseract) Height() uint64 {
	return t.header.Height
}

func (t *Tesseract) BodyHash() Hash {
	return t.header.BodyHash
}

func (t *Tesseract) GroupHash() Hash {
	return t.header.GroupHash
}

func (t *Tesseract) GridHash() (Hash, error) {
	if t.header.Extra.GridID != nil {
		return t.header.Extra.GridID.Hash, nil
	}

	return NilHash, ErrGridIDNotFound
}

func (t *Tesseract) Parts() (*TesseractParts, error) {
	if t.header.Extra.GridID == nil {
		return nil, ErrGridIDNotFound
	}

	if t.header.Extra.GridID.Parts == nil {
		return nil, ErrTesseractPartsNotFound
	}

	return t.header.Extra.GridID.Parts.Copy(), nil
}

func (t *Tesseract) Operator() string {
	return t.header.Operator
}

func (t *Tesseract) ContextLock() map[Address]ContextLockInfo {
	contextLock := make(map[Address]ContextLockInfo, len(t.header.ContextLock))

	for k, v := range t.header.ContextLock {
		contextLock[k] = v
	}

	return contextLock
}

func (t *Tesseract) ContextLockByAddress(address Address) (ContextLockInfo, bool) {
	ctxLockInfo, ok := t.header.ContextLock[address]

	return ctxLockInfo, ok
}

func (t *Tesseract) GridLength() int32 {
	return t.header.Extra.GridID.Parts.Total
}

func (t *Tesseract) ClusterID() ClusterID {
	return ClusterID(t.header.ClusterID)
}

func (t *Tesseract) Timestamp() int64 {
	return t.header.Timestamp
}

func (t *Tesseract) Extra() CommitData {
	return t.header.Extra.Copy()
}

func (t *Tesseract) ContextDelta() ContextDelta {
	return t.body.ContextDelta.Copy()
}

func (t *Tesseract) GetContextDeltaByAddress(address Address) (DeltaGroup, bool) {
	delta, ok := t.body.ContextDelta[address]
	if !ok {
		return DeltaGroup{}, ok
	}

	return *delta.Copy(), ok
}

func (t *Tesseract) ContextHash() Hash {
	return t.body.ContextHash
}

func (t *Tesseract) InteractionHash() Hash {
	return t.body.InteractionHash
}

func (t *Tesseract) ReceiptHash() Hash {
	return t.body.ReceiptHash
}

func (t *Tesseract) StateHash() Hash {
	return t.body.StateHash
}

func (t *Tesseract) ConsensusProof() PoXCData {
	return t.body.ConsensusProof
}

func (t *Tesseract) ICSHash() Hash {
	return t.body.ConsensusProof.ICSHash
}

func (t *Tesseract) Hash() (Hash, error) {
	if hash := t.hash.Load(); hash != nil {
		actualHash, ok := hash.(Hash)
		if !ok {
			return NilHash, ErrInvalidHash
		}

		return actualHash, nil
	}

	header := t.Header()

	hash, err := header.Hash()
	if err != nil {
		return Hash{}, err
	}

	t.hash.Store(hash)

	return hash, nil
}

func (t *Tesseract) Bytes() ([]byte, error) {
	c := t.CanonicalWithoutSeal()

	rawData, err := polo.Polorize(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract")
	}

	return rawData, nil
}

func (t *Tesseract) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(t, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract")
	}

	return nil
}

// CanonicalWithoutSeal method returns a copy of the tesseract without seal and interactions
func (t *Tesseract) CanonicalWithoutSeal() *CanonicalTesseractWithoutSeal {
	return &CanonicalTesseractWithoutSeal{
		Header: t.header,
		Body:   t.body,
	}
}

// Canonical method returns a copy of the tesseract without interactions
func (t *Tesseract) Canonical() *CanonicalTesseract {
	return &CanonicalTesseract{
		Header: t.header,
		Body:   t.body,
		Seal:   t.seal,
	}
}

func (t *Tesseract) GetTesseractWithoutIxns() *Tesseract {
	return &Tesseract{
		header: t.header,
		body:   t.body,
		seal:   t.seal,
	}
}

type CanonicalTesseract struct {
	Header TesseractHeader
	Body   TesseractBody
	Seal   []byte
}

// Bytes method polorizes and returns the canonical tesseract in bytes
func (c *CanonicalTesseract) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize canonical tesseract")
	}

	return rawData, nil
}

func (c *CanonicalTesseract) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(c, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize canonical tesseract")
	}

	return nil
}

func (c *CanonicalTesseract) ToTesseract(ixns Interactions) *Tesseract {
	return &Tesseract{
		header: c.Header,
		body:   c.Body,
		ixns:   ixns,
		seal:   c.Seal,
	}
}

func (c *CanonicalTesseract) GridHash() (Hash, error) {
	if c.Header.Extra.GridID != nil {
		return c.Header.Extra.GridID.Hash, nil
	}

	return NilHash, errors.New("grid hash not found")
}

type CanonicalTesseractWithoutSeal struct {
	Header TesseractHeader
	Body   TesseractBody
}

type Item struct {
	Tesseract *Tesseract
	Delta     map[Hash][]byte
	Sender    kramaid.KramaID
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

type TesseractHeightAndHash struct {
	Height uint64
	Hash   Hash
}

type TesseractParts struct {
	Total int32
	Grid  map[Address]TesseractHeightAndHash
}

func (p *TesseractParts) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(p)
	if err != nil {
		return nil, errors.Wrap(err, "error polorizing tesseract parts")
	}

	return rawData, nil
}

func (p *TesseractParts) FromBytes(data []byte) error {
	if err := polo.Depolorize(p, data); err != nil {
		return errors.Wrap(err, "error depolorizing tesseract parts")
	}

	return nil
}

func (p *TesseractParts) Copy() *TesseractParts {
	parts := &TesseractParts{
		Total: p.Total,
	}

	if len(p.Grid) > 0 {
		parts.Grid = make(map[Address]TesseractHeightAndHash)

		for k, v := range p.Grid {
			parts.Grid[k] = v
		}
	}

	return parts
}

type TesseractGridID struct {
	Hash  Hash
	Parts *TesseractParts
}

func (gridId *TesseractGridID) IsNil() bool {
	return gridId.Hash.IsNil()
}

func (gridId *TesseractGridID) String() string {
	if !gridId.IsNil() {
		return gridId.Hash.Hex()
	}

	return "Nil"
}

func (gridId *TesseractGridID) Copy() *TesseractGridID {
	id := *gridId

	if gridId.Parts != nil {
		id.Parts = gridId.Parts.Copy()
	}

	return &id
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

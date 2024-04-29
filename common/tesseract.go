package common

import (
	"bytes"
	"math/big"
	"sync/atomic"

	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
)

type State struct {
	Height          uint64
	TransitiveLink  Hash // transitive link is made up of multiple accounts data in previous grid formation
	PreviousContext Hash
	LatestContext   Hash
	ContextDelta    DeltaGroup
	StateHash       Hash
}

func (s *State) Copy() State {
	state := *s

	state.ContextDelta = *(s.ContextDelta.Copy())

	return state
}

type Participants map[identifiers.Address]State

func (p Participants) Copy() Participants {
	if len(p) == 0 {
		return nil
	}

	participants := make(Participants)

	for key, value := range p {
		participants[key] = value.Copy()
	}

	return participants
}

type PoXtData struct {
	BinaryHash   Hash         `json:"binary_hash"`
	IdentityHash Hash         `json:"identity_hash"`
	ICSHash      Hash         `json:"ics_hash"`
	ClusterID    ClusterID    `json:"cluster_id"`
	ICSSignature []byte       `json:"ics_signature"`
	ICSVoteset   *ArrayOfBits `json:"ics_vote_set"`

	// non canonical fields
	EvidenceHash    Hash         `json:"evidence_hash"`
	Round           int32        `json:"round"`
	CommitSignature []byte       `json:"commit_signature"`
	BFTVoteSet      *ArrayOfBits `json:"bft_vote_set"`
}

func (p *PoXtData) Copy() PoXtData {
	poxt := *p

	if len(p.ICSSignature) > 0 {
		poxt.ICSSignature = make([]byte, len(p.ICSSignature))

		copy(poxt.ICSSignature, p.ICSSignature)
	}

	if len(p.CommitSignature) > 0 {
		poxt.CommitSignature = make([]byte, len(p.CommitSignature))

		copy(poxt.CommitSignature, p.CommitSignature)
	}

	if p.ICSVoteset != nil {
		poxt.ICSVoteset = p.ICSVoteset.copy()
	}

	if p.BFTVoteSet != nil {
		poxt.BFTVoteSet = p.BFTVoteSet.copy()
	}

	return poxt
}

type Tesseract struct {
	participants     Participants
	interactionsHash Hash
	receiptsHash     Hash
	epoch            *big.Int
	timestamp        uint64
	operator         string
	fuelUsed         uint64
	fuelLimit        uint64
	consensusInfo    PoXtData

	// non canonical fields
	seal   []byte
	sealBy kramaid.KramaID

	// derived fields
	hash     atomic.Value
	ixns     Interactions
	receipts Receipts
}

func NewTesseract(
	participants Participants,
	interactionsHash Hash,
	receiptHash Hash,
	epoch *big.Int,
	timestamp uint64,
	operator string,
	fuelUsed, fuelLimit uint64,
	consensusInfo PoXtData,
	seal []byte,
	sealBy kramaid.KramaID,
	ixns Interactions,
	receipts Receipts,
) *Tesseract {
	bytes := make([]byte, len(seal))
	copy(bytes, seal)

	t := &Tesseract{
		participants:     participants.Copy(),
		interactionsHash: interactionsHash,
		receiptsHash:     receiptHash,
		epoch:            new(big.Int).Set(epoch),
		timestamp:        timestamp,
		operator:         operator,
		fuelUsed:         fuelUsed,
		fuelLimit:        fuelLimit,
		consensusInfo:    consensusInfo.Copy(),

		seal:   bytes,
		sealBy: sealBy,

		ixns:     ixns,
		receipts: receipts,
	}

	return t
}

func (t *Tesseract) Copy() *Tesseract {
	return NewTesseract(
		t.Participants(),
		t.InteractionsHash(),
		t.ReceiptsHash(),
		t.Epoch(),
		t.Timestamp(),
		t.Operator(),
		t.FuelUsed(),
		t.FuelLimit(),
		t.ConsensusInfo(),
		t.Seal(),
		t.SealBy(),
		t.Interactions(),
		t.Receipts(),
	)
}

func (t *Tesseract) CompareHash(tsHash Hash) bool {
	if len(tsHash.Bytes()) == 0 {
		return false
	}

	if t == nil {
		return false
	}

	return bytes.Equal(t.Hash().Bytes(), tsHash.Bytes())
}

func (t *Tesseract) Hash() Hash {
	if hash := t.hash.Load(); hash != nil {
		actualHash, ok := hash.(Hash)
		if !ok {
			panic("hash type conversion failed")
		}

		return actualHash
	}

	ts := CanonicalTesseract{
		Participants:     t.participants,
		InteractionsHash: t.interactionsHash,
		ReceiptsHash:     t.receiptsHash,
		Epoch:            t.epoch,
		Timestamp:        t.timestamp,
		Operator:         t.operator,
		FuelUsed:         t.fuelUsed,
		FuelLimit:        t.fuelLimit,
		ConsensusInfo: PoXtData{
			BinaryHash:   t.consensusInfo.BinaryHash,
			IdentityHash: t.consensusInfo.IdentityHash,
			ICSHash:      t.consensusInfo.ICSHash,
			ClusterID:    t.consensusInfo.ClusterID,
			ICSSignature: t.consensusInfo.ICSSignature,
			ICSVoteset:   t.consensusInfo.ICSVoteset,
		},
	}

	hash, err := PoloHash(ts)
	if err != nil {
		panic("failed to polorize tesseract")
	}

	t.hash.Store(hash)

	return hash
}

func (t *Tesseract) HasParticipant(target identifiers.Address) bool {
	for addr := range t.participants {
		if addr == target {
			return true
		}
	}

	return false
}

func (t *Tesseract) Addresses() []identifiers.Address {
	addrs := make([]identifiers.Address, 0, t.ParticipantCount())

	for addr := range t.participants {
		addrs = append(addrs, addr)
	}

	return addrs
}

func (t *Tesseract) AnyAddress() identifiers.Address {
	for addr := range t.participants {
		return addr
	}

	return identifiers.NilAddress
}

func (t *Tesseract) Participants() Participants {
	return t.participants
}

func (t *Tesseract) ParticipantCount() int {
	return len(t.participants)
}

func (t *Tesseract) State(addr identifiers.Address) (State, bool) {
	state, ok := t.participants[addr]
	if !ok {
		return State{}, ok
	}

	return state, ok
}

func (t *Tesseract) InteractionsHash() Hash {
	return t.interactionsHash
}

func (t *Tesseract) ReceiptsHash() Hash {
	return t.receiptsHash
}

func (t *Tesseract) Epoch() *big.Int {
	return t.epoch
}

func (t *Tesseract) Timestamp() uint64 {
	return t.timestamp
}

func (t *Tesseract) Operator() string {
	return t.operator
}

func (t *Tesseract) FuelUsed() uint64 {
	return t.fuelUsed
}

func (t *Tesseract) FuelLimit() uint64 {
	return t.fuelLimit
}

func (t *Tesseract) ConsensusInfo() PoXtData {
	return t.consensusInfo
}

func (t *Tesseract) BFTVoteSet() *ArrayOfBits {
	return t.consensusInfo.BFTVoteSet
}

func (t *Tesseract) Seal() []byte {
	return t.seal
}

func (t *Tesseract) SealBy() kramaid.KramaID {
	return t.sealBy
}

func (t *Tesseract) ExecutionContext() *ExecutionContext {
	return &ExecutionContext{
		CtxDelta: t.ContextDelta(),
		Cluster:  t.ClusterID(),
		Time:     t.Timestamp(),
	}
}

func (t *Tesseract) Interactions() Interactions {
	return t.ixns
}

func (t *Tesseract) Receipts() Receipts {
	return t.receipts
}

func (t *Tesseract) SetReceipts(receipts Receipts) {
	t.receipts = receipts
}

func (t *Tesseract) HasReceipts() bool {
	return t.receipts != nil
}

func (t *Tesseract) Height(address identifiers.Address) uint64 {
	return t.participants[address].Height
}

func (t *Tesseract) TransitiveLink(address identifiers.Address) Hash {
	return t.participants[address].TransitiveLink
}

func (t *Tesseract) StateHash(address identifiers.Address) Hash {
	return t.participants[address].StateHash
}

func (t *Tesseract) LatestContextHash(address identifiers.Address) Hash {
	return t.participants[address].LatestContext
}

func (t *Tesseract) PreviousContextHash(address identifiers.Address) Hash {
	return t.participants[address].PreviousContext
}

func (t *Tesseract) PreviousContext() map[identifiers.Address]Hash {
	previousContext := make(map[identifiers.Address]Hash)

	for addr, p := range t.participants {
		previousContext[addr] = p.PreviousContext
	}

	return previousContext
}

func (t *Tesseract) ContextDelta() ContextDelta {
	ctxDelta := make(ContextDelta)

	for addr, participant := range t.participants {
		participant := participant
		ctxDelta[addr] = &(participant.ContextDelta)
	}

	return ctxDelta
}

func (t *Tesseract) GetContextDelta(address identifiers.Address) (DeltaGroup, bool) {
	state, ok := t.participants[address]
	if !ok {
		return DeltaGroup{}, ok
	}

	return state.ContextDelta, true
}

func (t *Tesseract) ClusterID() ClusterID {
	return t.consensusInfo.ClusterID
}

func (t *Tesseract) ICSHash() Hash {
	return t.consensusInfo.ICSHash
}

func (t *Tesseract) SetRound(round int32) {
	t.consensusInfo.Round = round
}

func (t *Tesseract) SetCommitSignature(sig []byte) {
	t.consensusInfo.CommitSignature = sig
}

func (t *Tesseract) SetEvidenceHash(hash Hash) {
	t.consensusInfo.EvidenceHash = hash
}

func (t *Tesseract) SetBFTVoteSet(v *ArrayOfBits) {
	t.consensusInfo.BFTVoteSet = v
}

func (t *Tesseract) SetSeal(seal []byte) {
	t.seal = seal
}

func (t *Tesseract) SetSealBy(sealBy kramaid.KramaID) {
	t.sealBy = sealBy
}

func (t *Tesseract) SetIxns(ixns Interactions) {
	t.ixns = ixns
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

// Canonical method returns a copy of the tesseract without interactions
func (t *Tesseract) Canonical() *CanonicalTesseract {
	return &CanonicalTesseract{
		Participants:     t.participants,
		InteractionsHash: t.interactionsHash,
		ReceiptsHash:     t.receiptsHash,
		Epoch:            t.epoch,
		Timestamp:        t.timestamp,
		Operator:         t.operator,
		FuelUsed:         t.fuelUsed,
		FuelLimit:        t.fuelLimit,
		ConsensusInfo:    t.consensusInfo,
		Seal:             t.seal,
		SealBy:           t.sealBy,
	}
}

// CanonicalWithoutSeal method returns a copy of the tesseract without seal and interactions
func (t *Tesseract) CanonicalWithoutSeal() *CanonicalTesseractWithoutSeal {
	return &CanonicalTesseractWithoutSeal{
		Participants:     t.participants,
		InteractionsHash: t.interactionsHash,
		ReceiptsHash:     t.receiptsHash,
		Epoch:            t.epoch,
		Timestamp:        t.timestamp,
		Operator:         t.operator,
		FuelUsed:         t.fuelUsed,
		FuelLimit:        t.fuelLimit,
		ConsensusInfo:    t.consensusInfo,
	}
}

func (t *Tesseract) GetTesseractWithoutIxns() *Tesseract {
	return &Tesseract{
		participants:     t.participants,
		interactionsHash: t.interactionsHash,
		receiptsHash:     t.receiptsHash,
		epoch:            t.epoch,
		timestamp:        t.timestamp,
		operator:         t.operator,
		fuelUsed:         t.fuelUsed,
		fuelLimit:        t.fuelLimit,
		consensusInfo:    t.consensusInfo,
		seal:             t.seal,
		sealBy:           t.sealBy,
	}
}

type CanonicalTesseractWithoutSeal struct {
	Participants     map[identifiers.Address]State
	InteractionsHash Hash
	ReceiptsHash     Hash
	Epoch            *big.Int
	Timestamp        uint64
	Operator         string
	FuelUsed         uint64
	FuelLimit        uint64
	ConsensusInfo    PoXtData
}

type CanonicalTesseract struct {
	Participants     Participants
	InteractionsHash Hash
	ReceiptsHash     Hash
	Epoch            *big.Int
	Timestamp        uint64
	Operator         string
	FuelUsed         uint64
	FuelLimit        uint64
	ConsensusInfo    PoXtData
	Seal             []byte
	SealBy           kramaid.KramaID
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

// TODO WRITE TEST
func (c *CanonicalTesseract) ToTesseract(ixns Interactions, receipts Receipts) *Tesseract {
	return &Tesseract{
		participants:     c.Participants,
		interactionsHash: c.InteractionsHash,
		receiptsHash:     c.ReceiptsHash,
		epoch:            c.Epoch,
		timestamp:        c.Timestamp,
		operator:         c.Operator,
		fuelUsed:         c.FuelUsed,
		fuelLimit:        c.FuelLimit,
		consensusInfo:    c.ConsensusInfo,

		// non canonical fields
		seal:   c.Seal,
		sealBy: c.SealBy,

		// derived fields
		ixns:     ixns,
		receipts: receipts,
	}
}

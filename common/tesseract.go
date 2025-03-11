package common

import (
	"bytes"
	"math/big"
	"sort"
	"sync/atomic"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"golang.org/x/crypto/blake2b"
)

type State struct {
	Height         uint64
	TransitiveLink Hash
	LockedContext  Hash
	ContextDelta   *DeltaGroup
	StateHash      Hash
}

func (s *State) Copy() State {
	state := *s

	if state.ContextDelta != nil {
		state.ContextDelta = s.ContextDelta.Copy()
	}

	return state
}

type ParticipantsState map[identifiers.Identifier]State

func (p ParticipantsState) Copy() ParticipantsState {
	if len(p) == 0 {
		return nil
	}

	participants := make(ParticipantsState)

	for key, value := range p {
		participants[key] = value.Copy()
	}

	return participants
}

func (p ParticipantsState) IsExcluded(id identifiers.Identifier) bool {
	state, ok := p[id]
	if !ok {
		return true
	}

	return state.StateHash == NilHash
}

type PoXtData struct {
	Proposer     identifiers.KramaID                 `json:"proposer"`
	BinaryHash   Hash                                `json:"binary_hash"`
	IdentityHash Hash                                `json:"identity_hash"`
	View         uint64                              `json:"view"`
	LastCommit   map[identifiers.Identifier]Hash     `json:"last_commit"`
	EvidenceHash map[identifiers.Identifier]Hash     `json:"evidence_hash"`
	AccountLocks map[identifiers.Identifier]LockType `json:"account_locks"`
	ICSSeed      [32]byte                            `json:"ics_seed"`
	ICSProof     []byte                              `json:"ics_proof"`
}

type CommitInfo struct {
	QC                        *Qc                   `json:"commit_qc"`
	Operator                  identifiers.KramaID   `json:"operator"`
	ClusterID                 ClusterID             `json:"cluster_id"`
	View                      uint64                `json:"commit_view"`
	RandomSet                 []identifiers.KramaID `json:"random_set"`
	RandomSetSizeWithoutDelta uint32                `json:"random_set_size"`
}

func (ci *CommitInfo) FromBytes(raw []byte) error {
	if err := polo.Depolorize(ci, raw); err != nil {
		return errors.Wrap(err, "failed to depolorize commit info")
	}

	return nil
}

func (ci *CommitInfo) Bytes() ([]byte, error) {
	return polo.Polorize(ci)
}

func (ci *CommitInfo) Hash() (Hash, error) {
	return PoloHash(ci)
}

type Tesseract struct {
	participants     ParticipantsState
	interactionsHash Hash
	receiptsHash     Hash
	epoch            *big.Int
	timestamp        uint64
	fuelUsed         uint64
	fuelLimit        uint64
	consensusInfo    PoXtData

	seal   []byte
	sealBy identifiers.KramaID

	// derived fields, these fields are not available in the encoded data
	hash     atomic.Value
	ixns     Interactions
	receipts Receipts
	// commitQc info associated with the current tesseract
	commitInfo *CommitInfo
}

func NewTesseract(
	participants ParticipantsState,
	interactionsHash Hash,
	receiptHash Hash,
	epoch *big.Int,
	timestamp uint64,
	fuelUsed, fuelLimit uint64,
	consensusInfo PoXtData,
	seal []byte,
	sealBy identifiers.KramaID,
	ixns Interactions,
	receipts Receipts,
	commitInfo *CommitInfo,
) *Tesseract {
	bytes := make([]byte, len(seal))
	copy(bytes, seal)

	t := &Tesseract{
		participants:     participants.Copy(),
		interactionsHash: interactionsHash,
		receiptsHash:     receiptHash,
		epoch:            new(big.Int).Set(epoch),
		timestamp:        timestamp,
		fuelUsed:         fuelUsed,
		fuelLimit:        fuelLimit,
		consensusInfo:    consensusInfo,

		seal:   bytes,
		sealBy: sealBy,

		ixns:       ixns,
		receipts:   receipts,
		commitInfo: commitInfo,
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
		t.FuelUsed(),
		t.FuelLimit(),
		t.ConsensusInfo(),
		t.Seal(),
		t.SealBy(),
		t.Interactions(),
		t.Receipts(),
		t.CommitInfo(),
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

	raw, err := t.SignBytes()
	if err != nil {
		panic(err)
	}

	hash := blake2b.Sum256(raw)

	t.hash.Store(BytesToHash(hash[:]))

	return hash
}

func (t *Tesseract) HasParticipant(target identifiers.Identifier) bool {
	for id := range t.participants {
		if id == target {
			return true
		}
	}

	return false
}

func (t *Tesseract) ExcludedAccounts() IdentifierList {
	ids := make(IdentifierList, 0)

	for id, ps := range t.participants {
		if ps.StateHash == NilHash {
			ids = append(ids, id)
		}
	}

	return ids
}

func (t *Tesseract) AccountIDs() IdentifierList {
	ids := make(IdentifierList, 0, t.ParticipantCount())

	for id := range t.participants {
		ids = append(ids, id)
	}

	sort.Sort(ids)

	return ids
}

func (t *Tesseract) Heights() map[identifiers.Identifier]uint64 {
	heights := make(map[identifiers.Identifier]uint64)
	for id, ps := range t.participants {
		heights[id] = ps.Height
	}

	return heights
}

func (t *Tesseract) AnyAccountID() identifiers.Identifier {
	for id := range t.participants {
		return id
	}

	return identifiers.Nil
}

func (t *Tesseract) Participants() ParticipantsState {
	return t.participants
}

func (t *Tesseract) ParticipantCount() int {
	return len(t.participants)
}

func (t *Tesseract) State(id identifiers.Identifier) (State, bool) {
	state, ok := t.participants[id]
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

func (t *Tesseract) Operator() identifiers.KramaID {
	return t.consensusInfo.Proposer
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

func (t *Tesseract) CommitInfo() *CommitInfo {
	return t.commitInfo
}

func (t *Tesseract) CommitHash() Hash {
	if t.commitInfo == nil {
		return NilHash
	}

	hash, _ := t.commitInfo.Hash()

	return hash
}

func (t *Tesseract) Seal() []byte {
	return t.seal
}

func (t *Tesseract) SealBy() identifiers.KramaID {
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

func (t *Tesseract) Height(id identifiers.Identifier) uint64 {
	ps, ok := t.participants[id]
	if !ok {
		return 0
	}

	return ps.Height
}

func (t *Tesseract) TransitiveLink(id identifiers.Identifier) Hash {
	return t.participants[id].TransitiveLink
}

func (t *Tesseract) StateHash(id identifiers.Identifier) Hash {
	return t.participants[id].StateHash
}

func (t *Tesseract) LockedContextHash(id identifiers.Identifier) Hash {
	return t.participants[id].LockedContext
}

func (t *Tesseract) LockedContext() map[identifiers.Identifier]Hash {
	previousContext := make(map[identifiers.Identifier]Hash)

	for id, p := range t.participants {
		previousContext[id] = p.LockedContext
	}

	return previousContext
}

func (t *Tesseract) ContextDelta() ContextDelta {
	ctxDelta := make(ContextDelta)

	for id, participant := range t.participants {
		if participant.ContextDelta != nil {
			ctxDelta[id] = participant.ContextDelta
		}
	}

	return ctxDelta
}

func (t *Tesseract) GetContextDelta(id identifiers.Identifier) (*DeltaGroup, bool) {
	state, ok := t.participants[id]
	if !ok {
		return nil, ok
	}

	return state.ContextDelta, true
}

func (t *Tesseract) ClusterID() ClusterID {
	return t.commitInfo.ClusterID
}

func (t *Tesseract) ICSSeed() [32]byte {
	return t.consensusInfo.ICSSeed
}

func (t *Tesseract) ICSProof() []byte {
	return t.consensusInfo.ICSProof
}

func (t *Tesseract) SetView(view uint64) {
	t.consensusInfo.View = view
}

func (t *Tesseract) SetCommitQc(qc *Qc) {
	t.commitInfo.QC = qc
}

func (t *Tesseract) SetEvidenceHash(id identifiers.Identifier, hash Hash) {
	t.consensusInfo.EvidenceHash[id] = hash
}

func (t *Tesseract) SetSeal(seal []byte) {
	t.seal = seal
}

func (t *Tesseract) SetSealBy(sealBy identifiers.KramaID) {
	t.sealBy = sealBy
}

func (t *Tesseract) SetIxns(ixns Interactions) {
	t.ixns = ixns
}

func (t *Tesseract) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(t.participants); err != nil {
		return nil, err
	}

	polorizer.PolorizeBytes(t.interactionsHash.Bytes())
	polorizer.PolorizeBytes(t.receiptsHash.Bytes())
	polorizer.PolorizeBigInt(t.epoch)
	polorizer.PolorizeUint(t.timestamp)
	polorizer.PolorizeUint(t.fuelUsed)
	polorizer.PolorizeUint(t.fuelLimit)

	if err := polorizer.Polorize(t.consensusInfo); err != nil {
		return nil, err
	}

	polorizer.PolorizeBytes(t.seal)
	polorizer.PolorizeString(string(t.SealBy()))

	return polorizer, nil
}

func (t *Tesseract) SignBytes() ([]byte, error) {
	polorizer := polo.NewPolorizer()
	if err := polorizer.Polorize(t.participants); err != nil {
		return nil, err
	}

	polorizer.PolorizeBytes(t.interactionsHash.Bytes())
	polorizer.PolorizeBytes(t.receiptsHash.Bytes())
	polorizer.PolorizeBigInt(t.epoch)
	polorizer.PolorizeUint(t.timestamp)
	polorizer.PolorizeUint(t.fuelUsed)
	polorizer.PolorizeUint(t.fuelLimit)

	if err := polorizer.Polorize(t.consensusInfo); err != nil {
		return nil, err
	}

	return polorizer.Bytes(), nil
}

func (t *Tesseract) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	if depolorizer.IsNull() {
		return nil
	}

	depolorizer, err = depolorizer.Unpacked()
	if err != nil {
		return err
	}

	ps := make(ParticipantsState)
	consensusInfo := new(PoXtData)

	if err = depolorizer.Depolorize(&ps); err != nil {
		return err
	}

	rawIxnHash, err := depolorizer.DepolorizeBytes()
	if err != nil {
		return err
	}

	t.interactionsHash = BytesToHash(rawIxnHash)

	rawReceiptsHash, err := depolorizer.DepolorizeBytes()
	if err != nil {
		return err
	}

	t.receiptsHash = BytesToHash(rawReceiptsHash)

	if t.epoch, err = depolorizer.DepolorizeBigInt(); err != nil {
		return err
	}

	if t.timestamp, err = depolorizer.DepolorizeUint64(); err != nil {
		return err
	}

	if t.fuelUsed, err = depolorizer.DepolorizeUint64(); err != nil {
		return err
	}

	if t.fuelLimit, err = depolorizer.DepolorizeUint64(); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(consensusInfo); err != nil {
		return err
	}

	if t.seal, err = depolorizer.DepolorizeBytes(); err != nil {
		return err
	}

	sealer, err := depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	t.sealBy = identifiers.KramaID(sealer)
	t.participants = ps
	t.consensusInfo = *consensusInfo

	return nil
}

func (t *Tesseract) Bytes() ([]byte, error) {
	data, err := polo.Polorize(t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize tesseract")
	}

	return data, nil
}

func (t *Tesseract) FromBytes(data []byte) error {
	if err := polo.Depolorize(t, data); err != nil {
		return errors.Wrap(err, "failed to depolorize tesseract")
	}

	return nil
}

func (t *Tesseract) ValidateAllParticipantsState() bool {
	for _, state := range t.participants {
		if state.StateHash == NilHash {
			return false
		}
	}

	return true
}

func (t *Tesseract) WithIxnAndReceipts(ixs Interactions, receipts Receipts, commitInfo *CommitInfo) {
	t.ixns = ixs
	t.receipts = receipts
	t.commitInfo = commitInfo
}

func (t *Tesseract) GetTesseractWithoutIxns() *Tesseract {
	return &Tesseract{
		participants:     t.participants,
		interactionsHash: t.interactionsHash,
		receiptsHash:     t.receiptsHash,
		epoch:            t.epoch,
		timestamp:        t.timestamp,
		fuelUsed:         t.fuelUsed,
		fuelLimit:        t.fuelLimit,
		consensusInfo:    t.consensusInfo,
		seal:             t.seal,
		sealBy:           t.sealBy,
		commitInfo:       t.commitInfo,
	}
}

func (t *Tesseract) GetTesseractWithoutCommitInfo() *Tesseract {
	return &Tesseract{
		participants:     t.participants,
		interactionsHash: t.interactionsHash,
		receiptsHash:     t.receiptsHash,
		epoch:            t.epoch,
		timestamp:        t.timestamp,
		fuelUsed:         t.fuelUsed,
		fuelLimit:        t.fuelLimit,
		consensusInfo:    t.consensusInfo,
		seal:             t.seal,
		sealBy:           t.sealBy,
		ixns:             t.ixns,
		receipts:         t.receipts,
	}
}

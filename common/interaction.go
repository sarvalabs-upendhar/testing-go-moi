package common

import (
	"math/big"
	"sort"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"
)

const MaxSlotsForIxBatch = 4

// IxFund represents an AssetID and Amount involved in the interaction.
type IxFund struct {
	AssetID identifiers.AssetID `json:"asset_id"`
	Amount  *big.Int            `json:"amount"`
}

// Copy creates and returns a deep copy of IxFund.
func (ixFund *IxFund) Copy() IxFund {
	fund := *ixFund

	if ixFund.Amount != nil {
		fund.Amount = new(big.Int).Set(ixFund.Amount)
	}

	return fund
}

// IxOpRaw hold the raw payload of IxOp.
type IxOpRaw struct {
	Type    IxOpType `json:"type"`
	Payload []byte   `json:"payload"`
}

// Copy creates and returns a deep copy of IxOpRaw.
func (ixRaw *IxOpRaw) Copy() IxOpRaw {
	op := *ixRaw

	if len(ixRaw.Payload) > 0 {
		op.Payload = make([]byte, len(ixRaw.Payload))

		copy(op.Payload, ixRaw.Payload)
	}

	return op
}

// IxParticipant represents a participant with an id and LockType.
type IxParticipant struct {
	ID       identifiers.Identifier `json:"id"`
	LockType LockType               `json:"lock_type"`
	Notary   bool                   `json:"notary"`
}

// IxConsensusPreference contains preferences related to consensus.
type IxConsensusPreference struct {
	MTQ        uint                  `json:"mtq"`
	TrustNodes []identifiers.KramaID `json:"trust_nodes"`
}

// Copy creates and returns a deep copy of IxConsensusPreference.
func (ixConsensusPreference *IxConsensusPreference) Copy() *IxConsensusPreference {
	trust := &IxConsensusPreference{
		MTQ: ixConsensusPreference.MTQ,
	}

	if len(ixConsensusPreference.TrustNodes) > 0 {
		trust.TrustNodes = make([]identifiers.KramaID, len(ixConsensusPreference.TrustNodes))
		copy(trust.TrustNodes, ixConsensusPreference.TrustNodes)
	}

	return trust
}

// IxPreferences includes compute and consensus preferences.
type IxPreferences struct {
	Compute   []byte                 `json:"compute"`
	Consensus *IxConsensusPreference `json:"consensus"`
}

// Copy creates and returns a deep copy of IxPreferences.
func (ixPreferences *IxPreferences) Copy() *IxPreferences {
	preferences := *ixPreferences

	if len(ixPreferences.Compute) > 0 {
		preferences.Compute = make([]byte, len(ixPreferences.Compute))

		copy(preferences.Compute, ixPreferences.Compute)
	}

	if ixPreferences.Consensus != nil {
		preferences.Consensus = ixPreferences.Consensus.Copy()
	}

	return &preferences
}

type Sender struct {
	ID         identifiers.Identifier `json:"id"`
	SequenceID uint64                 `json:"sequence_id"`
	KeyID      uint64                 `json:"key_id"`
}

// IxData represents interaction related information.
type IxData struct {
	Sender Sender                 `json:"sender"`
	Payer  identifiers.Identifier `json:"payer"`

	FuelPrice *big.Int `json:"fuel_price"`
	FuelLimit uint64   `json:"fuel_limit"`

	Funds        []IxFund        `json:"funds"`
	IxOps        []IxOpRaw       `json:"ix_operations"`
	Participants []IxParticipant `json:"participants"`

	Preferences *IxPreferences `json:"preferences"`
	Perception  []byte         `json:"perception"`
}

// Copy creates and returns a deep copy of IxData.
func (ixData *IxData) Copy() IxData {
	data := *ixData

	if ixData.FuelPrice != nil {
		data.FuelPrice = new(big.Int).Set(ixData.FuelPrice)
	}

	if len(ixData.Funds) > 0 {
		data.Funds = make([]IxFund, len(ixData.Funds))
		for i, fund := range ixData.Funds {
			data.Funds[i] = fund.Copy()
		}
	}

	if len(ixData.IxOps) > 0 {
		data.IxOps = make([]IxOpRaw, len(ixData.IxOps))
		for i, op := range ixData.IxOps {
			data.IxOps[i] = op.Copy()
		}
	}

	if len(ixData.Participants) > 0 {
		data.Participants = make([]IxParticipant, len(ixData.Participants))
		copy(data.Participants, ixData.Participants)
	}

	if ixData.Preferences != nil {
		data.Preferences = ixData.Preferences.Copy()
	}

	if len(ixData.Perception) > 0 {
		data.Perception = make([]byte, len(ixData.Perception))
		copy(data.Perception, ixData.Perception)
	}

	return data
}

// Bytes serializes IxData and returns the serialized data.
func (ixData *IxData) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ixData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize ix data")
	}

	return rawData, nil
}

// FromBytes deserializes and returns IxData.
func (ixData *IxData) FromBytes(bytes []byte) error {
	err := polo.Depolorize(ixData, bytes)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize ix data")
	}

	return nil
}

func (ixData *IxData) ParticipantsInfo() map[identifiers.Identifier]*ParticipantInfo {
	psInfo := make(map[identifiers.Identifier]*ParticipantInfo, len(ixData.Participants))
	for _, ps := range ixData.Participants {
		psInfo[ps.ID] = &ParticipantInfo{
			LockType: ps.LockType,
			ID:       ps.ID,
		}

		if ps.ID == ixData.Sender.ID {
			psInfo[ps.ID].IsSigner = true
		}
	}

	return psInfo
}

// IxOp represents an ix operation. It inherits fields and methods from Interaction.
type IxOp struct {
	*Interaction
	target  identifiers.Identifier
	OpType  IxOpType     `json:"type"`
	Payload *IxOpPayload `json:"payload"`
}

// Type returns the ixn type.
func (op *IxOp) Type() IxOpType {
	return op.OpType
}

// GetParticipantCreatePayload returns the participant payload if its present.
func (op *IxOp) GetParticipantCreatePayload() (*ParticipantCreatePayload, error) {
	// If payload has been decoded, return the asset form
	if op.Payload == nil || op.Payload.participant == nil {
		return nil, errors.New("payload not found")
	}

	return op.Payload.participant.Create, nil
}

// GetAccountConfigurePayload returns the account configure payload if its present.
func (op *IxOp) GetAccountConfigurePayload() (*AccountConfigurePayload, error) {
	if op.Payload == nil || op.Payload.participant == nil || op.Payload.participant.Configure == nil {
		return nil, errors.New("payload not found")
	}

	return op.Payload.participant.Configure, nil
}

// GetAccountInheritPayload returns the account inherit payload if its present.
func (op *IxOp) GetAccountInheritPayload() (*AccountInheritPayload, error) {
	if op.Payload == nil || op.Payload.participant == nil || op.Payload.participant.Inherit == nil {
		return nil, errors.New("payload not found")
	}

	return op.Payload.participant.Inherit, nil
}

// getAssetPayload returns the asset payload if its present.
func (op *IxOp) getAssetPayload() *AssetPayload {
	// If payload has been decoded, return the asset form
	if op.Payload != nil && op.Payload.asset != nil {
		return op.Payload.asset
	}

	return nil
}

// GetAssetCreatePayload returns the asset creation payload if present, or an error if not found.
func (op *IxOp) GetAssetCreatePayload() (*AssetCreatePayload, error) {
	payload := op.getAssetPayload()
	if payload == nil || payload.Create == nil {
		return nil, errors.New("payload not found")
	}

	return payload.Create, nil
}

// GetAssetActionPayload returns the asset action payload if present, or an error if not found.
func (op *IxOp) GetAssetActionPayload() (*AssetActionPayload, error) {
	payload := op.getAssetPayload()
	if payload == nil || payload.Action == nil {
		return nil, errors.New("payload not found")
	}

	return payload.Action, nil
}

// getGuardianPayload returns the guardian payload if present, or an error if not found.
func (op *IxOp) getGuardianPayload() *GuardianPayload {
	// If payload has been decoded, return the guardian form
	if op.Payload != nil && op.Payload.guardian != nil {
		return op.Payload.guardian
	}

	return nil
}

// GetGuardianRegisterPayload returns the guardian register payload if present,
// or an error if not found.
func (op *IxOp) GetGuardianRegisterPayload() (*GuardianRegisterPayload, error) {
	payload := op.getGuardianPayload()
	if payload == nil || payload.Register == nil {
		return nil, errors.New("payload not found")
	}

	return payload.Register, nil
}

// GetGuardianActionPayload returns the guardian action payload if present,
// or an error if not found.
func (op *IxOp) GetGuardianActionPayload() (*GuardianActionPayload, error) {
	payload := op.getGuardianPayload()
	if payload == nil || payload.Action == nil {
		return nil, errors.New("payload not found")
	}

	return payload.Action, nil
}

// GetLogicPayload returns the logic payload if present, or an error if not found.
func (op *IxOp) GetLogicPayload() (*LogicPayload, error) {
	// If payload has been decoded, return the logic form
	if op.Payload == nil || op.Payload.logic == nil {
		return nil, errors.New("payload not found")
	}

	return op.Payload.logic, nil
}

// Manifest returns the manifest from the logic payload.
func (op *IxOp) Manifest() []byte {
	payload, err := op.GetLogicPayload()
	if err != nil {
		return nil
	}

	return payload.Manifest
}

// Target returns the id of the op beneficiary.
func (op *IxOp) Target() identifiers.Identifier {
	// Based on the op type return the id
	if op.target != identifiers.Nil {
		return op.target
	}

	return op.target
}

// Following methods implement engineio.Action interface

func (op *IxOp) Callsite() string {
	callSite := ""

	switch op.Type() {
	case IxParticipantCreate:
		payload, err := op.GetParticipantCreatePayload()
		if err != nil {
			panic("failed to get participant payload")
		}

		callSite = payload.Value.Callsite
	case IxAccountInherit:
		payload, err := op.GetAccountInheritPayload()
		if err != nil {
			panic("failed to get account inheritance payload")
		}

		callSite = payload.Value.Callsite

	case IxAssetCreate:
		payload, err := op.GetAssetCreatePayload()
		if err != nil {
			panic("failed to get asset create payload")
		}

		if payload.Logic != nil {
			callSite = payload.Logic.Callsite
		}

	case IxAssetAction:
		payload, err := op.GetAssetActionPayload()
		if err != nil {
			panic("failed to get asset action payload")
		}

		callSite = payload.Callsite
	case IxLogicInvoke, IxLogicDeploy:
		payload, err := op.GetLogicPayload()
		if err != nil {
			panic("failed to get asset action payload")
		}

		callSite = payload.Callsite
	default:
		return callSite
	}

	return callSite
}

func (op *IxOp) Calldata() polo.Document {
	var callData []byte

	switch op.Type() {
	case IxParticipantCreate:
		payload, err := op.GetParticipantCreatePayload()
		if err != nil {
			panic("failed to get participant payload")
		}

		callData = payload.Value.Calldata
	case IxAccountInherit:
		payload, err := op.GetAccountInheritPayload()
		if err != nil {
			panic("failed to get account inheritance payload")
		}

		callData = payload.Value.Calldata

	case IxAssetCreate:
		payload, err := op.GetAssetCreatePayload()
		if err != nil {
			panic("failed to get asset create payload")
		}

		if payload.Logic != nil {
			callData = payload.Logic.Calldata
		}

	case IxAssetAction:
		payload, err := op.GetAssetActionPayload()
		if err != nil {
			panic("failed to get asset action payload")
		}

		callData = payload.Calldata
	case IxLogicInvoke, IxLogicDeploy:
		payload, err := op.GetLogicPayload()
		if err != nil {
			panic("failed to get asset action payload")
		}

		callData = payload.Calldata
	default:
	}

	doc := make(polo.Document)

	if len(callData) == 0 {
		return doc
	}

	if err := polo.Depolorize(&doc, callData); err != nil {
		return doc
	}

	return doc
}

func (op *IxOp) Timestamp() uint64 {
	return 0
}

func (op *IxOp) Identifier() [32]byte {
	return op.Hash()
}

func (op *IxOp) Origin() [32]byte {
	return op.Sender().ID
}

func (op *IxOp) Caller() [32]byte {
	return op.Sender().ID
}

func (op *IxOp) Access(id [32]byte) (int, error) {
	info, ok := op.Participants()[id]
	if !ok {
		return 0, errors.New("actor not found")
	}

	return int(info.LockType), nil
}

func (op *IxOp) AccessList() map[[32]byte]int {
	accessList := make(map[[32]byte]int, len(op.Participants()))

	for id, info := range op.Participants() {
		accessList[id] = int(info.LockType)
	}

	return accessList
}

func (op *IxOp) Parameters() map[string][]byte {
	// This method is not implemented for IxOp.
	return nil
}

// Interaction represents a batch of ops with associated data and metadata.
type Interaction struct {
	inner              IxData
	ops                []*IxOp
	ps                 map[identifiers.Identifier]*ParticipantInfo
	leaderCandidateAcc *ParticipantInfo
	hash               atomic.Value
	size               atomic.Value
	signatures         Signatures
	allottedView       atomic.Uint64
	shouldPropose      bool
}

// NewInteraction initializes and returns the Interaction.
func NewInteraction(ixData IxData, signatures Signatures) (*Interaction, error) {
	cpyIxData := ixData.Copy()
	ix := &Interaction{
		inner: cpyIxData,
		ops:   make([]*IxOp, len(cpyIxData.IxOps)),
		ps:    ixData.ParticipantsInfo(),
	}

	ix.signatures = signatures

	data, err := polo.Polorize(ixData)
	if err != nil {
		return nil, err
	}

	if _, ok := ix.ps[ix.SenderID()]; !ok {
		return nil, ErrMissingSender
	}

	if !ix.Payer().IsNil() {
		if _, ok := ix.ps[ix.Payer()]; !ok {
			return nil, ErrMissingPayer
		}
	}

	for idx, op := range ixData.IxOps {
		switch op.Type {
		case IxParticipantCreate:
			psCreatePayload := new(ParticipantCreatePayload)
			if err = psCreatePayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				target:      psCreatePayload.ID,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					participant: &ParticipantPayload{
						Create: psCreatePayload,
					},
				},
			}

			_, ok := ix.ps[SargaAccountID]
			if !ok {
				ix.ps[SargaAccountID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

			_, ok = ix.ps[psCreatePayload.ID]
			if !ok {
				ix.ps[psCreatePayload.ID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
					ID:        psCreatePayload.ID,
				}
			}

			if psCreatePayload.Value == nil {
				return nil, ErrMissingValuePayload
			}

			_, ok = ix.ps[psCreatePayload.Value.AssetID.AsIdentifier()]
			if !ok {
				return nil, ErrMissingAssetAccount
			}

		case IxAccountConfigure:
			accConfigurePayload := new(AccountConfigurePayload)
			if err := accConfigurePayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					participant: &ParticipantPayload{
						Configure: accConfigurePayload,
					},
				},
			}

		case IxAccountInherit:
			accInheritPayload := new(AccountInheritPayload)
			if err = accInheritPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			if accInheritPayload.Value == nil {
				return nil, ErrMissingValuePayload
			}

			subAccount, err := ix.SenderID().DeriveVariant(accInheritPayload.SubAccountIndex, nil, nil)
			if err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				target:      subAccount,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					participant: &ParticipantPayload{
						Inherit: accInheritPayload,
					},
				},
			}

			_, ok := ix.ps[subAccount]
			if !ok {
				ix.ps[subAccount] = &ParticipantInfo{
					ID:        subAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
				}
			}

			_, ok = ix.ps[SargaAccountID]
			if !ok {
				ix.ps[SargaAccountID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

			_, ok = ix.ps[accInheritPayload.Value.AssetID.AsIdentifier()]
			if !ok {
				return nil, ErrMissingAssetAccount
			}

		case IxAssetCreate:
			assetCreatePayload := new(AssetCreatePayload)
			if err = assetCreatePayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			assetID, err := identifiers.GenerateAssetIDv0(
				NewAccountID(ix.Sender()),
				0,
				uint16(assetCreatePayload.Standard),
				assetCreatePayload.Flags()...,
			)
			if err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				target:      assetID.AsIdentifier(),
				OpType:      op.Type,
				Payload: &IxOpPayload{
					asset: &AssetPayload{
						Create: assetCreatePayload,
					},
				},
			}

			_, ok := ix.ps[SargaAccountID]
			if !ok {
				ix.ps[SargaAccountID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

			_, ok = ix.ps[assetID.AsIdentifier()]
			if !ok {
				ix.ps[assetID.AsIdentifier()] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
					ID:        assetID.AsIdentifier(),
				}
			}

		case IxAssetAction:
			assetActionPayload := new(AssetActionPayload)
			if err = assetActionPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				target:      assetActionPayload.AssetID.AsIdentifier(),
				OpType:      op.Type,
				Payload: &IxOpPayload{
					asset: &AssetPayload{
						Action: assetActionPayload,
					},
				},
			}

			_, ok := ix.ps[assetActionPayload.AssetID.AsIdentifier()]
			if !ok {
				return nil, ErrMissingAssetAccount
			}

		case IxLogicDeploy, IxLogicInvoke: // IxLogicEnlist
			logicPayload := new(LogicPayload)
			if err = logicPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					logic: logicPayload,
				},
			}

			if IxLogicDeploy != op.Type {
				_, ok := ix.ps[logicPayload.LogicID.AsIdentifier()]
				if !ok {
					return nil, ErrMissingLogicAccount
				}

				for _, logic := range logicPayload.Interfaces {
					account, ok := ix.ps[logic]
					if !ok {
						return nil, ErrMissingForeignLogicAccount
					}

					account.IsSigner = false
				}

				ix.ops[idx].target = logicPayload.LogicID.AsIdentifier()

				continue
			}

			_, ok := ix.ps[SargaAccountID]
			if !ok {
				ix.ps[SargaAccountID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

			logicID, _ := identifiers.GenerateLogicIDv0(
				NewAccountID(ix.Sender()),
				0,
				logicPayload.Flags()...,
			)

			_, ok = ix.ps[logicID.AsIdentifier()]
			if !ok {
				ix.ps[logicID.AsIdentifier()] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
					ID:        logicID.AsIdentifier(),
				}
			}

			ix.ops[idx].target = logicID.AsIdentifier()

		case IxGuardianRegister:
			guardianRegisterPayload := new(GuardianRegisterPayload)
			if err = guardianRegisterPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					guardian: &GuardianPayload{
						Register: guardianRegisterPayload,
					},
				},
			}

			_, ok := ix.ps[SystemAccountID]
			if !ok {
				ix.ps[SystemAccountID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SystemAccountID,
				}
			}

		case IxGuardianStake, IxGuardianUnstake, IxGuardianWithdraw, IxGuardianClaim:
			guardianActionPayload := new(GuardianActionPayload)
			if err = guardianActionPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					guardian: &GuardianPayload{
						Action: guardianActionPayload,
					},
				},
			}

			_, ok := ix.ps[SystemAccountID]
			if !ok {
				ix.ps[SystemAccountID] = &ParticipantInfo{
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SystemAccountID,
				}
			}

		default:
			return nil, ErrInvalidInteractionType
		}
	}

	signaturesBytes, err := signatures.Bytes()
	if err != nil {
		return nil, err
	}

	ix.hash.Store(GetHash(data))
	ix.size.Store(uint64(len(data) + len(signaturesBytes)))

	return ix, ix.UpdateLeaderCandidateID()
}

func (ix *Interaction) Participants() map[identifiers.Identifier]*ParticipantInfo {
	return ix.ps
}

func (ix *Interaction) IDs() []identifiers.Identifier {
	ids := make([]identifiers.Identifier, 0, len(ix.ps))

	for id := range ix.ps {
		ids = append(ids, id)
	}

	return ids
}

func (ix *Interaction) AccountsWithoutNoLock() []identifiers.Identifier {
	ids := make([]identifiers.Identifier, 0)

	for id, info := range ix.ps {
		if info.LockType == NoLock {
			continue
		}

		ids = append(ids, id)
	}

	return ids
}

func (ix *Interaction) LeaderCandidateAcc() identifiers.Identifier {
	return ix.leaderCandidateAcc.ID
}

// IXData returns a copy of the interaction data.
func (ix *Interaction) IXData() IxData {
	return ix.inner.Copy()
}

// Signatures returns the interaction's signatures.
func (ix *Interaction) Signatures() Signatures {
	return ix.signatures
}

// SenderID returns the id of the Interaction sender
func (ix *Interaction) SenderID() identifiers.Identifier {
	return ix.inner.Sender.ID
}

func (ix *Interaction) SenderKeyID() uint64 {
	return ix.inner.Sender.KeyID
}

func (ix *Interaction) Sender() Sender {
	return ix.inner.Sender
}

func (ix *Interaction) SetSender(sender Sender) {
	ix.inner.Sender = sender
}

// Payer returns the id of the Interaction payer
func (ix *Interaction) Payer() identifiers.Identifier {
	return ix.inner.Payer
}

// SequenceID returns the account sequenceID of the Interaction sender
func (ix *Interaction) SequenceID() uint64 {
	return ix.inner.Sender.SequenceID
}

// KMOITokenValue aggregates and returns the KMOI token values in asset transfers.
func (ix *Interaction) KMOITokenValue() *big.Int {
	var tokens int64 = 0

	// FIXME: We need figure this logic

	return big.NewInt(tokens)
}

// FuelPrice returns the fuel price for the interaction.
func (ix *Interaction) FuelPrice() *big.Int {
	if ix.inner.FuelPrice == nil {
		return big.NewInt(0)
	}

	return new(big.Int).Set(ix.inner.FuelPrice)
}

// FuelLimit returns the fuel limit for the interaction.
func (ix *Interaction) FuelLimit() uint64 {
	return ix.inner.FuelLimit
}

// FuelPriceCmp compares the fuel price with another interaction.
func (ix *Interaction) FuelPriceCmp(other *Interaction) int {
	return ix.FuelPrice().Cmp(other.FuelPrice())
}

// FuelPriceIntCmp compares the fuel price with a big.Int value.
func (ix *Interaction) FuelPriceIntCmp(other *big.Int) int {
	return ix.FuelPrice().Cmp(other)
}

// Cost calculates and returns the total cost of the interaction.
func (ix *Interaction) Cost() *big.Int {
	total := new(big.Int).Mul(ix.FuelPrice(), new(big.Int).SetUint64(ix.FuelLimit()))
	total.Add(total, ix.KMOITokenValue())

	return total
}

// IsUnderpriced checks if the interaction's fuel price is below the price limit.
func (ix *Interaction) IsUnderpriced(priceLimit *big.Int) bool {
	return ix.FuelPrice().Cmp(priceLimit) != 0
}

// Hash returns the cached hash or computes it if not cached.
func (ix *Interaction) Hash() Hash {
	if hash := ix.hash.Load(); hash != nil {
		return hash.(Hash) //nolint:forcetypeassert
	}

	raw, err := ix.Bytes()
	if err != nil {
		panic(err)
	}

	hash := GetHash(raw)

	ix.hash.Store(hash)

	return hash
}

// Size returns the cached size or computes it if not cached.
func (ix *Interaction) Size() (uint64, error) {
	if size := ix.size.Load(); size != nil {
		return size.(uint64), nil //nolint:forcetypeassert
	}

	data, err := ix.Bytes()
	if err != nil {
		return 0, errors.Wrap(err, "failed to polorize interaction")
	}

	size := uint64(len(data))
	ix.size.Store(size)

	return size, err
}

// Polorize serializes the interaction.
func (ix *Interaction) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(ix.inner); err != nil {
		return nil, errors.Wrap(err, "failed to polorize interaction data")
	}

	rawSig, err := ix.signatures.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal signatures")
	}

	polorizer.PolorizeBytes(rawSig)

	return polorizer, nil
}

// Depolorize deserializes the interaction.
func (ix *Interaction) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	if depolorizer.IsNull() {
		return nil
	}

	depolorizer, err = depolorizer.Unpacked()
	if err != nil {
		return err
	}

	data := IxData{}
	if err = depolorizer.Depolorize(&data); err != nil {
		return errors.Wrap(err, "failed to depolorize interaction data")
	}

	sig, err := depolorizer.DepolorizeBytes()
	if err != nil {
		return errors.Wrap(err, "failed to depolorize interaction signature")
	}

	signatures := make(Signatures, 0)

	if err := signatures.FromBytes(sig); err != nil {
		return err
	}

	ixn, err := NewInteraction(data, signatures)
	if err != nil {
		return err
	}

	*ix = *ixn //nolint

	return nil
}

// Bytes returns the serialized interaction data.
func (ix *Interaction) Bytes() ([]byte, error) {
	polorizer, err := ix.Polorize()
	if err != nil {
		return nil, err
	}

	return polorizer.Bytes(), nil
}

// FromBytes deserializes interaction from bytes.
func (ix *Interaction) FromBytes(data []byte) error {
	depolorizer, err := polo.NewDepolorizer(data)
	if err != nil {
		return errors.Wrap(err, "failed to depolorize interaction")
	}

	if err = ix.Depolorize(depolorizer); err != nil {
		return err
	}

	return nil
}

// PayloadForSignature returns the serialized data used for signing.
func (ix *Interaction) PayloadForSignature() ([]byte, error) {
	return polo.Polorize(ix.inner)
}

// Funds returns the funds that are associated with the interaction.
func (ix *Interaction) Funds() []IxFund {
	return ix.inner.Funds
}

// IxParticipants returns the participants that are associated with the interaction.
func (ix *Interaction) IxParticipants() []IxParticipant {
	return ix.inner.Participants
}

// Ops returns a list of interaction ops.
func (ix *Interaction) Ops() []*IxOp {
	return ix.ops
}

// GetIxOp returns a specific op by its index.
func (ix *Interaction) GetIxOp(opID int) *IxOp {
	if opID < 0 || opID >= len(ix.ops) {
		panic(ErrIndexOutOfRange)
	}

	return ix.ops[opID]
}

// Perception returns the interaction perception.
func (ix *Interaction) Perception() []byte {
	return ix.inner.Perception
}

// Preferences return the interaction preferences.
func (ix *Interaction) Preferences() *IxPreferences {
	return ix.inner.Preferences
}

func (ix *Interaction) UpdateLeaderCandidateID() error {
	if len(ix.ps) == 0 {
		return errors.New("empty ix participants")
	}

	regularAccounts := make(IdentifierList, 0, len(ix.ps))
	nonRegularAccounts := make(IdentifierList, 0, len(ix.ps))

	for id, info := range ix.ps {
		if info.AccountType() == InvalidAccount {
			return ErrInvalidIdentifier
		}

		if id == SargaAccountID || id == SystemAccountID {
			ix.leaderCandidateAcc = ix.ps[id]

			return nil
		}
		// skip accounts which are created as part of the current interaction
		if info.IsGenesis {
			continue
		}

		if info.AccountType() == RegularAccount && info.LockType == MutateLock {
			regularAccounts = append(regularAccounts, id)

			continue
		}

		// Non-regular account with mutate lock
		if info.LockType < ReadLock {
			nonRegularAccounts = append(nonRegularAccounts, id)
		}
	}

	if len(nonRegularAccounts) > 0 {
		sort.Sort(nonRegularAccounts)

		ix.leaderCandidateAcc = ix.ps[nonRegularAccounts[0]]

		return nil
	}

	sort.Sort(regularAccounts)
	ix.leaderCandidateAcc = ix.ps[regularAccounts[0]]

	return nil
}

func (ix *Interaction) SetShouldPropose(val bool) {
	ix.shouldPropose = val
}

func (ix *Interaction) ShouldPropose() bool {
	return ix.shouldPropose
}

func (ix *Interaction) UpdateAllottedView(view uint64) {
	ix.allottedView.Store(view)
}

func (ix *Interaction) AllottedView() uint64 {
	return ix.allottedView.Load()
}

type Interactions struct {
	ixns              []*Interaction
	leaderCandidateID identifiers.Identifier
	ps                atomic.Value
}

func NewInteractions() *Interactions {
	return &Interactions{
		ixns: make([]*Interaction, 0),
	}
}

func NewInteractionsWithLeaderCheck(checkForLeader bool, l ...*Interaction) Interactions {
	ixns := Interactions{ixns: l}

	if !checkForLeader {
		return ixns
	}

	nonRegularAccounts := make(IdentifierList, 0)
	regularAccounts := make(IdentifierList, 0)

	for _, ixn := range ixns.ixns {
		if ixn.leaderCandidateAcc.AccountType() == SystemAccount {
			ixns.leaderCandidateID = ixn.leaderCandidateAcc.ID

			return ixns
		}

		if ixn.leaderCandidateAcc.AccountType() == RegularAccount {
			regularAccounts = append(regularAccounts, ixn.leaderCandidateAcc.ID)
		} else {
			nonRegularAccounts = append(nonRegularAccounts, ixn.leaderCandidateAcc.ID)
		}
	}

	// non-regular accounts have priority over regular accounts
	if len(nonRegularAccounts) > 0 {
		sort.Sort(nonRegularAccounts)

		ixns.leaderCandidateID = nonRegularAccounts[0]

		return ixns
	}

	if len(regularAccounts) > 0 {
		sort.Sort(regularAccounts)

		ixns.leaderCandidateID = regularAccounts[0]

		return ixns
	}

	return ixns
}

func (ixs *Interactions) Append(ix *Interaction) {
	ixs.ixns = append(ixs.ixns, ix)
}

func (ixs Interactions) LeaderCandidateID() identifiers.Identifier {
	return ixs.leaderCandidateID
}

func (ixs Interactions) Hashes() Hashes {
	hashes := make(Hashes, 0, len(ixs.ixns))

	for _, ixn := range ixs.ixns {
		hashes = append(hashes, ixn.Hash())
	}

	return hashes
}

func (ixs Interactions) UniqueIdsWithoutNoLocks() []identifiers.Identifier {
	ids := make(IdentifierList, 0)
	hasID := make(map[identifiers.Identifier]struct{})

	for _, ixn := range ixs.ixns {
		for id, info := range ixn.ps {
			if info.LockType == NoLock {
				continue
			}

			if _, ok := hasID[id]; ok {
				continue
			}

			ids = append(ids, id)
			hasID[id] = struct{}{}
		}
	}

	sort.Sort(ids)

	return ids
}

func (ixs Interactions) Participants() map[identifiers.Identifier]ParticipantInfo {
	v := ixs.ps.Load()
	if v != nil {
		ps := v.(map[identifiers.Identifier]ParticipantInfo) //nolint

		return ps
	}

	ps := make(map[identifiers.Identifier]ParticipantInfo)
	for _, ixn := range ixs.ixns {
		for id, info := range ixn.ps {
			oldInfo, ok := ps[id]
			if !ok {
				ps[id] = *info
			}

			if oldInfo.LockType < info.LockType {
				continue
			}

			ps[id] = *info
		}
	}

	ixs.ps.Store(ps)

	return ps
}

func (ixs Interactions) Locks() map[identifiers.Identifier]LockType {
	v := ixs.ps.Load()
	if v == nil {
		v = ixs.Participants()
	}

	locks := make(map[identifiers.Identifier]LockType)

	for id, info := range v.(map[identifiers.Identifier]ParticipantInfo) { //nolint
		locks[id] = info.LockType
	}

	return locks
}

func (ixs Interactions) IxList() []*Interaction {
	return ixs.ixns
}

func (ixs Interactions) Len() int {
	return len(ixs.ixns)
}

// Copy returns a shallow copy of interactions
func (ixs Interactions) Copy() Interactions {
	newIxs := ixs

	return newIxs
}

func (ixs Interactions) Size() (ixsSize uint64) {
	for _, ix := range ixs.ixns {
		size, err := ix.Size()
		if err != nil {
			continue
		}

		ixsSize += size
	}

	return ixsSize
}

func (ixs Interactions) Bytes() ([]byte, error) {
	return polo.Polorize(ixs.ixns)
}

func (ixs *Interactions) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	ixns := ixs.ixns

	if err := polorizer.Polorize(ixns); err != nil {
		return nil, err
	}

	return polorizer, nil
}

func (ixs *Interactions) Depolorize(depolorizer *polo.Depolorizer) error {
	ixns := make([]*Interaction, 0)

	if err := depolorizer.Depolorize(&ixns); err != nil {
		return errors.Wrap(err, "failed to depolorize interactions")
	}

	if len(ixns) == 0 {
		return nil
	}

	*ixs = NewInteractionsWithLeaderCheck(true, ixns...)

	return nil
}

func (ixs *Interactions) FromBytes(b []byte) error {
	ixns := make([]*Interaction, 0)

	if err := polo.Depolorize(&ixns, b); err != nil {
		return errors.Wrap(err, "failed to depolorize interactions")
	}

	if len(ixns) == 0 {
		return nil
	}

	*ixs = NewInteractionsWithLeaderCheck(true, ixns...)

	return nil
}

// Hash returns the hash of all Interactions
func (ixs Interactions) Hash() (Hash, error) {
	data, err := ixs.Bytes()
	if err != nil {
		return NilHash, err
	}

	return GetHash(data), nil
}

// FuelLimit aggregates and returns the fuel limits of all Interactions.
func (ixs Interactions) FuelLimit() (limit uint64) {
	for _, ix := range ixs.ixns {
		limit += ix.FuelLimit()
	}

	return limit
}

type IxBySequenceID Interactions

func (s IxBySequenceID) Len() int             { return len(s.ixns) }
func (s IxBySequenceID) Less(i, j int) bool   { return s.ixns[i].SequenceID() < s.ixns[j].SequenceID() }
func (s IxBySequenceID) Swap(i, j int)        { s.ixns[i], s.ixns[j] = s.ixns[j], s.ixns[i] }
func (s IxBySequenceID) List() []*Interaction { return s.ixns }

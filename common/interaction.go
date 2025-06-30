package common

import (
	"math/big"
	"sort"
	"sync/atomic"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
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
			AccType:  RegularAccount,
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

// GetAssetSupplyPayload returns the asset supply payload if present, or an error if not found.
func (op *IxOp) GetAssetSupplyPayload() (*AssetSupplyPayload, error) {
	payload := op.getAssetPayload()
	if payload == nil || payload.Supply == nil {
		return nil, errors.New("payload not found")
	}

	return payload.Supply, nil
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

// Callsite returns the callsite from the logic payload.
func (op *IxOp) Callsite() string {
	payload, err := op.GetLogicPayload()
	if err != nil {
		return ""
	}

	return payload.Callsite
}

// Calldata returns the calldata from the logic payload.
func (op *IxOp) Calldata() []byte {
	payload, err := op.GetLogicPayload()
	if err != nil {
		return nil
	}

	return payload.Calldata
}

// LogicID returns the logic identifier from the logic payload.
func (op *IxOp) LogicID() identifiers.LogicID {
	payload, err := op.GetLogicPayload()
	if err != nil {
		return identifiers.Nil
	}

	return payload.Logic
}

// Target returns the id of the op beneficiary.
func (op *IxOp) Target() identifiers.Identifier {
	// Based on the op type return the id
	if op.target != identifiers.Nil {
		return op.target
	}

	switch op.Type() {
	case IxParticipantCreate:
		payload, err := op.GetParticipantCreatePayload()
		if err != nil {
			panic(err)
		}

		op.target = payload.ID
	case IxAssetCreate:
		payload, err := op.GetAssetCreatePayload()
		if err != nil {
			panic(err)
		}

		assetID, _ := identifiers.GenerateAssetIDv0(
			NewAccountID(op.Sender()),
			0,
			uint16(payload.Standard),
			payload.Flags()...,
		)

		op.target = assetID.AsIdentifier()
	case IxAssetTransfer, IxAssetApprove, IxAssetRevoke, IxAssetLockup, IxAssetRelease:
		payload, err := op.GetAssetActionPayload()
		if err != nil {
			panic(err)
		}

		op.target = payload.Beneficiary
	case IXAccountInherit:
		payload, err := op.GetAccountInheritPayload()
		if err != nil {
			panic(err)
		}

		op.target, _ = op.SenderID().DeriveVariant(payload.SubAccountIndex, nil, nil)
	case IxAssetMint, IxAssetBurn:
		payload, err := op.GetAssetSupplyPayload()
		if err != nil {
			panic(err)
		}

		op.target = payload.AssetID.AsIdentifier()
	case IxLogicDeploy:
		payload, err := op.GetLogicPayload()
		if err != nil {
			panic(err)
		}

		logicID, _ := identifiers.GenerateLogicIDv0(
			NewAccountID(op.Sender()),
			0,
			payload.Flags()...,
		)

		op.target = logicID.AsIdentifier()
	case IxLogicInvoke, IxLogicEnlist:
		payload, err := op.GetLogicPayload()
		if err != nil {
			panic(err)
		}

		op.target = payload.Logic.AsIdentifier()

	default:
		panic(ErrInvalidInteractionType)
	}

	return op.target
}

type Signature struct {
	ID        identifiers.Identifier
	KeyID     uint64
	Signature []byte
}

type Signatures []Signature

// Bytes serializes signatures to bytes.
func (s Signatures) Bytes() ([]byte, error) {
	data, err := polo.Polorize(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize signatures payload")
	}

	return data, nil
}

// FromBytes deserializes signatures from bytes.
func (s *Signatures) FromBytes(data []byte) error {
	if err := polo.Depolorize(s, data); err != nil {
		return errors.Wrap(err, "failed to depolorize signatures payload")
	}

	return nil
}

// Benefactor returns the benefactor's id if applicable; otherwise, returns nil id.
func (op *IxOp) Benefactor() identifiers.Identifier {
	if op.Type() == IxAssetTransfer || op.Type() == IxAssetRelease {
		payload, err := op.GetAssetActionPayload()
		if err != nil {
			panic(err)
		}

		return payload.Benefactor
	}

	return identifiers.Nil
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
		ops: make([]*IxOp,
			len(cpyIxData.IxOps)),
		ps: ixData.ParticipantsInfo(),
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
					AccType:   SargaAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

			info, ok := ix.ps[psCreatePayload.ID]
			if !ok {
				return nil, ErrMissingBeneficiary
			}

			info.IsSigner = false
			info.AccType = RegularAccount
			info.IsGenesis = true

		case IXAccountConfigure:
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

		case IXAccountInherit:
			accInheritPayload := new(AccountInheritPayload)
			if err := accInheritPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					participant: &ParticipantPayload{
						Inherit: accInheritPayload,
					},
				},
			}

			subAccount, err := ix.SenderID().DeriveVariant(accInheritPayload.SubAccountIndex, nil, nil)
			if err != nil {
				return nil, err
			}

			_, ok := ix.ps[subAccount]
			if !ok {
				ix.ps[subAccount] = &ParticipantInfo{
					ID:        subAccount,
					AccType:   RegularAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
				}
			}

			_, ok = ix.ps[SargaAccountID]
			if !ok {
				ix.ps[SargaAccountID] = &ParticipantInfo{
					AccType:   SargaAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

		case IxAssetTransfer:
			assetActionPayload := new(AssetActionPayload)
			if err = assetActionPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					asset: &AssetPayload{
						Action: assetActionPayload,
					},
				},
			}

			info, ok := ix.ps[assetActionPayload.Beneficiary]
			if !ok {
				return nil, ErrMissingBeneficiary
			}

			info.AccType = RegularAccount
			info.IsSigner = false

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
					AccType:   SargaAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: false,
					ID:        SargaAccountID,
				}
			}

			_, ok = ix.ps[assetID.AsIdentifier()]
			if !ok {
				ix.ps[assetID.AsIdentifier()] = &ParticipantInfo{
					AccType:   AssetAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
					ID:        assetID.AsIdentifier(),
				}
			}

		case IxAssetApprove, IxAssetRevoke, IxAssetLockup, IxAssetRelease:
			assetActionPayload := new(AssetActionPayload)
			if err = assetActionPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				target:      assetActionPayload.Beneficiary,
				OpType:      op.Type,
				Payload: &IxOpPayload{
					asset: &AssetPayload{
						Action: assetActionPayload,
					},
				},
			}

			if IxAssetRelease == op.Type {
				beneficiary, ok := ix.ps[assetActionPayload.Beneficiary]
				if !ok {
					return nil, ErrMissingBeneficiary
				}

				beneficiary.AccType = RegularAccount
				beneficiary.IsSigner = false

				benefactor, ok := ix.ps[assetActionPayload.Benefactor]
				if !ok {
					return nil, ErrMissingBenefactor
				}

				benefactor.AccType = RegularAccount
				benefactor.IsSigner = false
			}

		case IxAssetMint, IxAssetBurn:
			assetSupplyPayload := new(AssetSupplyPayload)
			if err = assetSupplyPayload.FromBytes(op.Payload); err != nil {
				return nil, err
			}

			ix.ops[idx] = &IxOp{
				Interaction: ix,
				target:      assetSupplyPayload.AssetID.AsIdentifier(),
				OpType:      op.Type,
				Payload: &IxOpPayload{
					asset: &AssetPayload{
						Supply: assetSupplyPayload,
					},
				},
			}

			info, ok := ix.ps[assetSupplyPayload.AssetID.AsIdentifier()]
			if !ok {
				return nil, ErrMissingAssetAccount
			}

			info.AccType = AssetAccount

		case IxLogicDeploy, IxLogicInvoke, IxLogicEnlist:
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
				info, ok := ix.ps[logicPayload.Logic.AsIdentifier()]
				if !ok {
					return nil, ErrMissingLogicAccount
				}

				info.AccType = LogicAccount

				for _, logic := range logicPayload.Interfaces {
					account, ok := ix.ps[logic.AsIdentifier()]
					if !ok {
						return nil, ErrMissingForeignLogicAccount
					}

					account.AccType = LogicAccount
					account.IsSigner = false
				}

				ix.ops[idx].target = logicPayload.Logic.AsIdentifier()

				continue
			}

			_, ok := ix.ps[SargaAccountID]
			if !ok {
				ix.ps[SargaAccountID] = &ParticipantInfo{
					AccType:   SargaAccount,
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
					AccType:   LogicAccount,
					IsSigner:  false,
					LockType:  MutateLock,
					IsGenesis: true,
					ID:        logicID.AsIdentifier(),
				}
			}

			ix.ops[idx].target = logicID.AsIdentifier()

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

	for _, op := range ix.Ops() {
		switch op.Type() {
		case IxAssetTransfer:
			payload, err := op.GetAssetActionPayload()
			if err != nil || payload.AssetID != KMOITokenAssetID || payload.Amount == nil {
				continue
			}

			tokens += payload.Amount.Int64()
		case IxParticipantCreate:
			payload, err := op.GetParticipantCreatePayload()
			if err != nil || payload.Amount == nil {
				continue
			}

			tokens += payload.Amount.Int64()
		default:
		}
	}

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
		if id == SargaAccountID {
			ix.leaderCandidateAcc = ix.ps[id]

			return nil
		}
		// skip accounts which are created as part of the current interaction
		if info.IsGenesis {
			continue
		}

		if info.AccType == RegularAccount {
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
		if ixn.leaderCandidateAcc.AccType == SargaAccount {
			ixns.leaderCandidateID = ixn.leaderCandidateAcc.ID

			return ixns
		}

		if ixn.leaderCandidateAcc.AccType == RegularAccount {
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

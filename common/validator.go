package common

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"
)

type (
	Epoch          uint64
	KYCStatus      uint8
	ValidatorIndex uint64
	Validator      struct {
		ID                    ValidatorIndex
		ActiveStake           *big.Int
		InactiveStake         *big.Int
		SocialTokens          *big.Int
		BehaviourTokens       *big.Int
		PendingStakeAdditions *big.Int

		// TODO: Has to be removed once the treasury logic is implemented
		Rewards *big.Int

		KramaID              identifiers.KramaID
		WalletAddress        identifiers.Identifier
		ConsensusPubKey      []byte
		KYCProof             []byte
		PendingStakeRemovals map[Epoch]*big.Int

		KYCStatus KYCStatus
	}
)

func NewValidator(
	id ValidatorIndex, kramaID identifiers.KramaID, pendingStake, activeStake *big.Int,
	walletAddress identifiers.Identifier, consensusPubKey []byte,
	kycProof []byte, kycStatus KYCStatus,
) *Validator {
	return &Validator{
		ID:                    id,
		ActiveStake:           activeStake,
		InactiveStake:         big.NewInt(0),
		SocialTokens:          big.NewInt(0),
		BehaviourTokens:       big.NewInt(0),
		PendingStakeAdditions: pendingStake,
		Rewards:               big.NewInt(0),
		PendingStakeRemovals:  make(map[Epoch]*big.Int),
		KramaID:               kramaID,
		WalletAddress:         walletAddress,
		ConsensusPubKey:       consensusPubKey,
		// Todo: Check if any kind of verification is required for kyc proof and status
		KYCProof:  kycProof,
		KYCStatus: kycStatus,
	}
}

func (v *Validator) IsActive(minimumStake *big.Int) bool {
	return v.ActiveStake.Cmp(minimumStake) >= 0
}

func (v *Validator) VotingPower() uint64 {
	return 0
}

func (v *Validator) Copy() *Validator {
	copiedVal := *v // Create a shallow copy of the Validator

	if v.ActiveStake != nil {
		copiedVal.ActiveStake = new(big.Int).Set(v.ActiveStake)
	}

	if v.InactiveStake != nil {
		copiedVal.InactiveStake = new(big.Int).Set(v.InactiveStake)
	}

	if v.SocialTokens != nil {
		copiedVal.SocialTokens = new(big.Int).Set(v.SocialTokens)
	}

	if v.BehaviourTokens != nil {
		copiedVal.BehaviourTokens = new(big.Int).Set(v.BehaviourTokens)
	}

	if v.PendingStakeAdditions != nil {
		copiedVal.PendingStakeAdditions = new(big.Int).Set(v.PendingStakeAdditions)
	}

	if v.Rewards != nil {
		copiedVal.Rewards = new(big.Int).Set(v.Rewards)
	}

	copiedVal.ConsensusPubKey = make([]byte, len(v.ConsensusPubKey))
	copy(copiedVal.ConsensusPubKey, v.ConsensusPubKey)

	copiedVal.KYCProof = make([]byte, len(v.KYCProof))
	copy(copiedVal.KYCProof, v.KYCProof)

	copiedVal.PendingStakeRemovals = make(map[Epoch]*big.Int, len(v.PendingStakeRemovals))

	for k, val := range v.PendingStakeRemovals {
		if val != nil {
			copiedVal.PendingStakeRemovals[k] = new(big.Int).Set(val)
		}
	}

	return &copiedVal
}

func (v *Validator) Bytes() ([]byte, error) {
	data, err := polo.Polorize(v)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (v *Validator) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(v, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize guardian object")
	}

	return nil
}

type ReadOnlyValidator interface {
	// GetID returns the validator's ID
	GetID() uint64

	// GetKramaID returns the validator's KramaID
	GetKramaID() identifiers.KramaID

	// GetWalletAddress returns the validator's wallet address
	GetWalletAddress() identifiers.Identifier

	// GetConsensusPubKey returns the validator's consensus public key
	GetConsensusPubKey() []byte

	// GetActiveStake returns the validator's active stake amount
	GetActiveStake() uint64

	// GetInactiveStake returns the validator's inactive stake amount
	GetInactiveStake() uint64

	// GetSocialTokens returns the validator's social tokens amount
	GetSocialTokens() uint64

	// GetBehaviourTokens returns the validator's behaviour tokens amount
	GetBehaviourTokens() uint64

	// GetPendingStakeAdditions returns the validator's pending stake additions amount
	GetPendingStakeAdditions() uint64

	// GetPendingStakeRemovals returns the validator's pending stake removals map
	GetPendingStakeRemovals() map[Epoch]uint64

	GetKYCProof() []byte

	GetKYCStatus() KYCStatus
}

type ValidatorInfo struct {
	ID          ValidatorIndex
	KramaID     identifiers.KramaID
	PublicKey   []byte
	VotingPower uint64
	Msg         interface{}
}

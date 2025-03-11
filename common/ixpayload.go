package common

import (
	"math/big"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// IxOpPayload holds the data for asset, file, or logic ops.
type IxOpPayload struct {
	participant *ParticipantPayload
	asset       *AssetPayload
	file        *FilePayload  //nolint:unused
	logic       *LogicPayload //nolint:unused
}

// ParticipantPayload holds different types of participant operations data.
type ParticipantPayload struct {
	Create    *ParticipantCreatePayload
	Configure *AccountConfigurePayload
	Inherit   *AccountInheritPayload
}

// AssetPayload holds different types of asset operations data.
type AssetPayload struct {
	// Create contains the payload for IxAssetCreate
	Create *AssetCreatePayload
	// Action contains the payload for IxAssetTransfer, IxAssetApprove and IxAssetRevoke
	Action *AssetActionPayload
	// Supply contains the payload for IxAssetMint and IxAssetBurn
	Supply *AssetSupplyPayload
}

// Bytes serializes AssetPayload to bytes.
func (asset *AssetPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(asset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset payload")
	}

	return data, nil
}

// FromBytes deserializes AssetPayload from bytes.
func (asset *AssetPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(asset, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset payload")
	}

	return nil
}

// AssetCreatePayload holds data for creating an asset.
type AssetCreatePayload struct {
	Symbol string
	Supply *big.Int

	Standard  AssetStandard
	Dimension uint8

	IsStateFul bool
	IsLogical  bool

	LogicPayload *LogicPayload
}

// Bytes serializes AssetCreatePayload to bytes.
func (ac *AssetCreatePayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(ac)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset create payload")
	}

	return data, nil
}

// FromBytes deserializes AssetCreatePayload from bytes.
func (ac *AssetCreatePayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(ac, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset create payload")
	}

	return nil
}

func (ac *AssetCreatePayload) Flags() []identifiers.Flag {
	flags := make([]identifiers.Flag, 0)

	if ac.IsLogical {
		flags = append(flags, identifiers.AssetLogical)
	}

	if ac.IsStateFul {
		flags = append(flags, identifiers.AssetStateful)
	}

	return flags
}

// Validate checks if the AssetCreatePayload is valid.
func (ac *AssetCreatePayload) Validate() error {
	// asset standard should be mas1 or mas2
	if ac.Standard != MAS1 && ac.Standard != MAS0 {
		return ErrInvalidAssetStandard
	}

	// supply should be one if asset standard is mas1
	if ac.Standard == MAS1 {
		if ac.Supply == nil || ac.Supply.Uint64() != 1 {
			return ErrInvalidAssetSupply
		}
	}

	return nil
}

// AssetSupplyPayload holds data for minting or burning an asset.
type AssetSupplyPayload struct {
	// AssetID is used to specify the AssetID for which to mint/burn
	AssetID identifiers.AssetID
	// Amount is used for mint/burn
	Amount *big.Int
}

// Bytes serializes AssetSupplyPayload to bytes.
func (supply *AssetSupplyPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(supply)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset supply payload")
	}

	return data, nil
}

// FromBytes deserializes AssetSupplyPayload from bytes.
func (supply *AssetSupplyPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(supply, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset supply payload")
	}

	return nil
}

// Validate checks if the AssetSupplyPayload is valid.
func (supply *AssetSupplyPayload) Validate() error {
	if err := supply.AssetID.Validate(); err != nil {
		return ErrInvalidAssetID
	}

	// can not mint asset standard mas1
	if AssetStandard(supply.AssetID.Standard()) == MAS1 {
		return ErrMintOrBurnNonFungibleToken
	}

	if supply.Amount == nil || supply.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

type KeyAddPayload struct {
	PublicKey          []byte
	Weight             uint64
	SignatureAlgorithm uint64
}

type KeyRevokePayload struct {
	KeyID uint64
}

// ParticipantCreatePayload holds the data for creating a new participant account
type ParticipantCreatePayload struct {
	ID          identifiers.Identifier
	KeysPayload []KeyAddPayload
	Amount      *big.Int
}

func (register *ParticipantCreatePayload) Weight() uint64 {
	weight := uint64(0)

	for _, key := range register.KeysPayload {
		weight += key.Weight
	}

	return weight
}

func (register *ParticipantCreatePayload) VerifySignatureAlgorithms() bool {
	for _, add := range register.KeysPayload {
		if add.SignatureAlgorithm != 0 {
			return false
		}
	}

	return true
}

// Bytes serializes ParticipantCreatePayload to bytes.
func (register *ParticipantCreatePayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(register)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize participant register payload")
	}

	return data, nil
}

// FromBytes deserializes ParticipantCreatePayload from bytes.
func (register *ParticipantCreatePayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(register, data); err != nil {
		return errors.Wrap(err, "failed to depolorize participant register payload")
	}

	return nil
}

// Validate checks if the ParticipantCreatePayload is valid.
func (register *ParticipantCreatePayload) Validate(senderID identifiers.Identifier) error {
	if register.ID.IsNil() || !isValidParticipantID(register.ID) || senderID == register.ID {
		return ErrInvalidIdentifier
	}

	if register.Amount == nil || register.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	if register.Weight() < MinWeight {
		return ErrInvalidWeight
	}

	if !register.VerifySignatureAlgorithms() {
		return ErrInvalidSignatureAlgorithm
	}

	return nil
}

type AccountConfigurePayload struct {
	Add    []KeyAddPayload
	Revoke []KeyRevokePayload
}

// Bytes serializes ParticipantCreatePayload to bytes.
func (configure *AccountConfigurePayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(configure)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account configure payload")
	}

	return data, nil
}

// FromBytes deserializes account configure payload from bytes.
func (configure *AccountConfigurePayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(configure, data); err != nil {
		return errors.Wrap(err, "failed to depolorize account configure payload")
	}

	return nil
}

// Validate checks if the AccountConfigurePayload is valid.
func (configure *AccountConfigurePayload) Validate() error {
	payloadAddLen := len(configure.Add)
	payloadRevokeLen := len(configure.Revoke)

	if (payloadAddLen > 0 && payloadRevokeLen > 0) || (payloadAddLen == 0 && payloadRevokeLen == 0) {
		return ErrInvalidAccountConfigure
	}

	for _, key := range configure.Add {
		if key.SignatureAlgorithm > 0 {
			return ErrInvalidSignatureAlgorithm
		}
	}

	return nil
}

type AccountInheritPayload struct {
	TargetAccount   identifiers.Identifier
	Amount          *big.Int
	SubAccountIndex uint32
}

// Bytes serializes ParticipantCreatePayload to bytes.
func (inherit *AccountInheritPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(inherit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account inherit payload")
	}

	return data, nil
}

// FromBytes deserializes account configure payload from bytes.
func (inherit *AccountInheritPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(inherit, data); err != nil {
		return errors.Wrap(err, "failed to depolorize account inherit payload")
	}

	return nil
}

// Validate checks if the AccountInheritPayload is valid.
func (inherit *AccountInheritPayload) Validate(senderID identifiers.Identifier) error {
	if senderID.IsParticipantVariant() {
		return ErrSenderAccount
	}

	if inherit.TargetAccount.IsNil() {
		return ErrInvalidIdentifier
	}

	if inherit.TargetAccount.Tag().Kind() != identifiers.KindLogic {
		return ErrInvalidTargetAccount
	}

	if inherit.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

// AssetActionPayload holds data for transferring, approving, or revoking an asset.
type AssetActionPayload struct {
	// Benefactor is the id that authorized access to his asset funds.
	Benefactor identifiers.Identifier
	// Beneficiary is the recipient id for the transfer/approve/revoke operation
	Beneficiary identifiers.Identifier
	// AssetID is used to specify the AssetID for which to transfer/approve/revoke
	AssetID identifiers.AssetID
	// Amount is used to specify the Amount for transfer/approve/revoke
	Amount *big.Int
	// Timestamp is used to specify the validity of the mandate
	Timestamp uint64
}

// Bytes serializes AssetActionPayload to bytes.
func (action *AssetActionPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(action)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset action payload")
	}

	return data, nil
}

// FromBytes deserializes AssetActionPayload from bytes.
func (action *AssetActionPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(action, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset action payload")
	}

	return nil
}

// Validate checks if the AssetActionPayload has a valid beneficiary.
func (action *AssetActionPayload) Validate() error {
	if action.Beneficiary.IsNil() {
		return ErrBeneficiaryMissing
	}

	// Reject genesis account interaction
	if action.Beneficiary == SargaAccountID {
		return ErrGenesisAccount
	}

	return nil
}

// ValidateAssetApprove checks if the AssetActionPayload payload for approval is valid.
func (action *AssetActionPayload) ValidateAssetApprove(senderID identifiers.Identifier) error {
	if err := action.Validate(); err != nil {
		return err
	}

	if !isValidParticipantID(action.Beneficiary) && !isValidLogicID(action.Beneficiary) {
		return ErrInvalidBeneficiary
	}

	if senderID == action.Beneficiary {
		return ErrInvalidBeneficiary
	}

	if action.Amount == nil || action.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	if action.Timestamp < uint64(time.Now().Unix()) {
		return ErrInvalidTimestamp
	}

	return nil
}

// ValidateAssetRevoke checks if the AssetActionPayload payload for revoke is valid.
func (action *AssetActionPayload) ValidateAssetRevoke(senderID identifiers.Identifier) error {
	if err := action.Validate(); err != nil {
		return err
	}

	if !isValidParticipantID(action.Beneficiary) && !isValidLogicID(action.Beneficiary) {
		return ErrInvalidBeneficiary
	}

	if senderID == action.Beneficiary {
		return ErrInvalidBeneficiary
	}

	return nil
}

// ValidateAssetTransfer checks if the AssetActionPayload payload for transfer is valid.
func (action *AssetActionPayload) ValidateAssetTransfer(senderID identifiers.Identifier) error {
	if err := action.Validate(); err != nil {
		return err
	}

	if action.Benefactor.IsNil() {
		if !isValidParticipantID(action.Beneficiary) || senderID == action.Beneficiary {
			return ErrInvalidBeneficiary
		}
	} else {
		if !isValidParticipantID(action.Benefactor) || senderID == action.Benefactor {
			return ErrInvalidBenefactor
		}

		// Reject genesis account interaction
		if action.Benefactor == SargaAccountID {
			return ErrGenesisAccount
		}
	}

	if action.Amount == nil || action.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

// ValidateAssetLockup checks if the AssetActionPayload payload for lockup is valid.
func (action *AssetActionPayload) ValidateAssetLockup(senderID identifiers.Identifier) error {
	if err := action.Validate(); err != nil {
		return err
	}

	if !isValidParticipantID(action.Beneficiary) && !isValidLogicID(action.Beneficiary) {
		return ErrInvalidBeneficiary
	}

	if senderID == action.Beneficiary {
		return ErrInvalidBeneficiary
	}

	if action.Amount == nil || action.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

// ValidateAssetRelease checks if the AssetActionPayload payload for release is valid.
func (action *AssetActionPayload) ValidateAssetRelease(senderID identifiers.Identifier) error {
	if err := action.Validate(); err != nil {
		return err
	}

	if !isValidParticipantID(action.Beneficiary) {
		return ErrInvalidBeneficiary
	}

	if action.Benefactor.IsNil() {
		return ErrBenefactorMissing
	}

	if !isValidParticipantID(action.Benefactor) || senderID == action.Benefactor {
		return ErrInvalidBenefactor
	}

	// Reject genesis account interaction
	if action.Benefactor == SargaAccountID {
		return ErrGenesisAccount
	}

	if action.Amount == nil || action.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

// FilePayload holds file-related data.
type FilePayload struct {
	Name  string
	Hash  string
	File  []byte
	Nodes []identifiers.KramaID
}

// LogicPayload holds data for logic execution.
type LogicPayload struct {
	// Manifest specifies some Logic manifest artifact.
	// Required for IxLogicDeploy, TxLogicUpgrade
	Manifest []byte

	// Logic specifies the Logic ID to execute a method on.
	// Required for IxLogicInvoke, TxLogicInteract, IxLogicEnlist, TxLogicUpgrade
	Logic identifiers.LogicID

	// Callsite specifies the method name to deploy and invoke.
	Callsite string

	// Calldata specifies the input call data.
	Calldata []byte

	// Interfaces specifies the foreign logics
	Interfaces map[string]identifiers.LogicID
}

// Bytes serializes LogicPayload to bytes.
func (payload *LogicPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize logic payload")
	}

	return data, nil
}

// FromBytes deserializes LogicPayload from bytes.
func (payload *LogicPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(payload, data); err != nil {
		return errors.Wrap(err, "failed to depolorize logic payload")
	}

	return nil
}

func (payload *LogicPayload) Flags() []identifiers.Flag {
	flags := make([]identifiers.Flag, 0)

	// TODO: fix this
	// flags = append(flags, identifiers.LogicIntrinsic, identifiers.LogicExtrinsic)

	return flags
}

// ValidateLogicDeploy checks if the LogicPayload for logic deploy is valid.
func (payload *LogicPayload) ValidateLogicDeploy() error {
	// Manifest cannot be empty for logic deploy
	if len(payload.Manifest) == 0 {
		return ErrEmptyManifest
	}

	return nil
}

// ValidateLogicInteract checks if the LogicPayload for logic invoke and enlist is valid.
func (payload *LogicPayload) ValidateLogicInteract() error {
	// Callsite cannot be empty
	if len(payload.Callsite) == 0 {
		return ErrEmptyCallSite
	}

	// LogicID cannot be empty
	if payload.Logic.AsIdentifier().IsNil() {
		return ErrMissingLogicID
	}

	if err := payload.Logic.Validate(); err != nil {
		return ErrInvalidLogicID
	}

	return nil
}

// isValidParticipantID checks if the participant id is valid.
func isValidParticipantID(id identifiers.Identifier) bool {
	if _, err := id.AsParticipantID(); err != nil {
		return false
	}

	return true
}

// isValidParticipantID checks if the logic id is valid.
func isValidLogicID(id identifiers.Identifier) bool {
	if _, err := id.AsLogicID(); err != nil {
		return false
	}

	return true
}

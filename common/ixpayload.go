package common

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// IxOpPayload holds the data for asset, file, or logic ops.
type IxOpPayload struct {
	participant *ParticipantPayload
	guardian    *GuardianPayload
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
	Symbol       string
	Dimension    uint8
	Decimals     uint8
	Standard     AssetStandard
	EnableEvents bool
	Manager      identifiers.Identifier
	MaxSupply    *big.Int
	MetaData     map[string][]byte

	Logic *LogicPayload
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

	flags = append(flags, identifiers.AssetLogical)

	flags = append(flags, identifiers.AssetStateful)

	return flags
}

// Validate checks if the AssetCreatePayload is valid.
func (ac *AssetCreatePayload) Validate() error {
	if ac.Symbol == "" {
		return ErrInvalidAssetSymbol
	}

	if _, ok := ValidAssetStandards[ac.Standard]; !ok {
		return ErrInvalidAssetStandard
	}

	if ac.Standard == MASX {
		if ac.Logic == nil {
			return ErrInvalidLogicPayload
		}

		if len(ac.Logic.Manifest) == 0 {
			return ErrEmptyManifest
		}
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
	Value       *AssetActionPayload
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

	if register.Value == nil {
		return ErrInvalidValue
	}

	if register.Value.AssetID != KMOITokenAssetID {
		return ErrInvalidAssetID
	}

	if len(register.Value.Callsite) == 0 {
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
	Value           *AssetActionPayload
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

	if inherit.Value == nil {
		return ErrInvalidValue
	}

	if inherit.Value.AssetID != KMOITokenAssetID {
		return ErrInvalidAssetID
	}

	if len(inherit.Value.Callsite) == 0 {
		return ErrInvalidCallSite
	}

	return nil
}

// AssetActionPayload holds data for transferring, approving, or revoking an asset.
type AssetActionPayload struct {
	// AssetID
	AssetID identifiers.AssetID

	// Callsite specifies the method name to deploy and invoke.
	Callsite string

	// Calldata specifies the input call data.
	Calldata []byte

	Funds map[identifiers.AssetID]*big.Int
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
	if err := action.AssetID.Validate(); err != nil {
		return ErrInvalidAssetID
	}

	if action.Callsite == "" {
		return ErrInvalidCallSite
	}

	return nil
}

// GuardianRegisterPayload contains information required to register a new guardian node.
type GuardianRegisterPayload struct {
	KramaID      identifiers.KramaID
	WalletID     identifiers.Identifier
	ConsensusKey []byte
	KYCProof     []byte
	Amount       *big.Int
}

// Bytes serializes GuardianRegisterPayload to bytes.
func (register *GuardianRegisterPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(register)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize guardian register payload")
	}

	return data, nil
}

// FromBytes deserializes GuardianRegisterPayload from bytes.
func (register *GuardianRegisterPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(register, data); err != nil {
		return errors.Wrap(err, "failed to depolorize guardian register payload")
	}

	return nil
}

// Validate checks if the GuardianRegisterPayload is valid.
func (register *GuardianRegisterPayload) Validate() error {
	if err := register.KramaID.Validate(); err != nil {
		return ErrInvalidKramaID
	}

	if register.WalletID.IsNil() {
		return ErrInvalidIdentifier
	}

	if register.ConsensusKey == nil {
		return errors.New("invalid consensus key")
	}

	if register.KYCProof == nil {
		return errors.New("invalid kyc proof")
	}

	if register.Amount == nil || register.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

// GuardianActionPayload holds data for performing guardian actions like staking, unstaking, withdrawing,
// or claiming rewards.
type GuardianActionPayload struct {
	KramaID identifiers.KramaID
	Amount  *big.Int
}

// Bytes serializes GuardianActionPayload to bytes.
func (action *GuardianActionPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(action)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize guardian action payload")
	}

	return data, nil
}

// FromBytes deserializes GuardianActionPayload from bytes.
func (action *GuardianActionPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(action, data); err != nil {
		return errors.Wrap(err, "failed to depolorize guardian action payload")
	}

	return nil
}

// Validate checks if the GuardianActionPayload is valid.
func (action *GuardianActionPayload) Validate() error {
	if err := action.KramaID.Validate(); err != nil {
		return ErrInvalidKramaID
	}

	if action.Amount == nil || action.Amount.Sign() <= 0 {
		return ErrInvalidValue
	}

	return nil
}

// GuardianPayload represents data related to guardian actions such as registration, staking, unstaking,
// withdrawing, or claiming rewards.
type GuardianPayload struct {
	Register *GuardianRegisterPayload
	Action   *GuardianActionPayload
}

// Bytes serializes GuardianPayload to bytes.
func (gp *GuardianPayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(gp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize guardian payload")
	}

	return data, nil
}

// FromBytes deserializes GuardianPayload from bytes.
func (gp *GuardianPayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(gp, data); err != nil {
		return errors.Wrap(err, "failed to depolorize guardian payload")
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

	// LogicID specifies the Logic ID to execute a method on.
	// Required for IxLogicInvoke, TxLogicInteract, IxLogicEnlist, TxLogicUpgrade
	LogicID identifiers.LogicID

	// Callsite specifies the method name to deploy and invoke.
	Callsite string

	// Calldata specifies the input call data.
	Calldata []byte

	// Interfaces specifies the foreign logics
	Interfaces map[string]identifiers.Identifier
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
		return ErrInvalidCallSite
	}

	// LogicID cannot be empty
	if payload.LogicID.AsIdentifier().IsNil() {
		return ErrMissingLogicID
	}

	if err := payload.LogicID.Validate(); err != nil {
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

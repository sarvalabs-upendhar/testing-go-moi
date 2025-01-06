package common

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
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
func (asset *AssetCreatePayload) Bytes() ([]byte, error) {
	data, err := polo.Polorize(asset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize asset create payload")
	}

	return data, nil
}

// FromBytes deserializes AssetCreatePayload from bytes.
func (asset *AssetCreatePayload) FromBytes(data []byte) error {
	if err := polo.Depolorize(asset, data); err != nil {
		return errors.Wrap(err, "failed to depolorize asset create payload")
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
	Address     identifiers.Address
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

// AssetActionPayload holds data for transferring, approving, or revoking an asset.
type AssetActionPayload struct {
	// Benefactor is the address that authorized access to his asset funds.
	Benefactor identifiers.Address
	// Beneficiary is the recipient address for the transfer/approve/revoke operation
	Beneficiary identifiers.Address
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

// FilePayload holds file-related data.
type FilePayload struct {
	Name  string
	Hash  string
	File  []byte
	Nodes []kramaid.KramaID
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

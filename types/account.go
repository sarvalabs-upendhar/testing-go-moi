package types

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// AccountType ...
type AccountType int

const (
	SargaAccount AccountType = iota + 1
	RegularAccount
	LogicAccount
	AssetAccount
)

type Account struct {
	Nonce          uint64      `json:"nonce"`
	AccType        AccountType `json:"acc_type"`
	Balance        Hash        `json:"balance"`
	AssetApprovals Hash        `json:"asset_approvals"`
	AssetRegistry  Hash        `json:"asset_registry"`
	ContextHash    Hash        `json:"context_hash"`
	StorageRoot    Hash        `json:"storage_root"`
	LogicRoot      Hash        `json:"logic_root"`
	FileRoot       Hash        `json:"file_root"`
}

func (a *Account) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(a)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account")
	}

	return rawData, nil
}

func (a *Account) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(a, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account")
	}

	return nil
}

func (a *Account) Hash() (Hash, error) {
	return PoloHash(a)
}

// Accounts ...
type Accounts []*AccountMetaInfo

func (acc *Accounts) Bytes() ([]byte, error) {
	return polo.Polorize(acc)
}

type AccountMetaInfo struct {
	Type AccountType `json:"type"`

	Address Address `json:"address"`
	Height  uint64  `json:"height"`

	TesseractHash Hash `json:"tesseract_hash"`
	LatticeExists bool `json:"lattice_exists"`
	StateExists   bool `json:"state_exists"`
}

func (ami *AccountMetaInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(ami)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account meta info")
	}

	return rawData, nil
}

func (ami *AccountMetaInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(ami, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account meta info")
	}

	return nil
}

type AccountGenesisInfo struct {
	MoiID  string
	IxHash Hash
}

func (agi *AccountGenesisInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(agi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize genesis account info")
	}

	return rawData, nil
}

func (agi *AccountGenesisInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(agi, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize genesis account info")
	}

	return nil
}

func AccTypeFromIxType(ixType IxType) AccountType {
	switch ixType {
	case IxAssetCreate:
		return AssetAccount
	case IxLogicDeploy:
		return LogicAccount
	default:
		return RegularAccount
	}
}

package types

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

// AccountType ...
type AccountType int

const (
	RegularAccount AccountType = iota
	SargaAccount
	ContractAccount
)

type Account struct {
	Nonce   uint64
	AccType AccountType

	Balance        Hash
	AssetApprovals Hash
	ContextHash    Hash
	StorageRoot    Hash
	LogicRoot      Hash
	FileRoot       Hash
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
	Type AccountType
	Mode string

	Address Address
	Height  *big.Int

	TesseractHash Hash
	LatticeExists bool
	StateExists   bool
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
	case IxLogicDeploy:
		return ContractAccount
	default:
		return RegularAccount
	}
}

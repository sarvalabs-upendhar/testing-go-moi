package common

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"
)

// AccountType ...
type AccountType int

const (
	SargaAccount AccountType = iota + 1
	LogicAccount
	AssetAccount
	RegularAccount
)

const MinWeight = 1000

type AccountKey struct {
	ID                 uint64
	PublicKey          []byte
	Weight             uint64
	SignatureAlgorithm uint64
	Revoked            bool
	SequenceID         uint64
}

type AccountKeys []*AccountKey

func (a AccountKeys) Hash() (Hash, error) {
	return PoloHash(a)
}

func (a AccountKeys) Copy() AccountKeys {
	newAccountKeys := make(AccountKeys, len(a))

	copy(newAccountKeys, a)

	return newAccountKeys
}

func (a AccountKeys) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(a)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account keys object")
	}

	return rawData, nil
}

func (a *AccountKeys) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(a, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account keys object")
	}

	return nil
}

type Account struct {
	AccType     AccountType `json:"acc_type"`
	AssetDeeds  Hash        `json:"asset_deeds"`
	ContextHash Hash        `json:"context_hash"`
	StorageRoot Hash        `json:"storage_root"`
	AssetRoot   Hash        `json:"asset_root"`
	LogicRoot   Hash        `json:"logic_root"`
	FileRoot    Hash        `json:"file_root"`
	KeysHash    Hash        `json:"keys_hash"`
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
	Type                 AccountType            `json:"type"`
	ID                   identifiers.Identifier `json:"id"`
	Height               uint64                 `json:"height"`
	TesseractHash        Hash                   `json:"tesseract_hash"`
	StateHash            Hash                   `json:"state_hash"`
	ConsensusNodesHash   Hash                   `json:"consensus_nodes"`
	ContextHash          Hash                   `json:"context_hash"`
	CommitHash           Hash                   `json:"commit_hash"`
	PositionInContextSet int                    `json:"position"`
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

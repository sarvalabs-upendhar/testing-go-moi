package common

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-polo"
)

// AccountType ...
type AccountType int

const (
	SystemAccount AccountType = iota + 1
	LogicAccount
	AssetAccount
	RegularAccount
	InvalidAccount
)

func AccountTypeFromID(id identifiers.Identifier) AccountType {
	if id == SystemAccountID || id == SargaAccountID {
		return SystemAccount
	}

	switch id.Tag().Kind() {
	case identifiers.KindParticipant:
		return RegularAccount
	case identifiers.KindAsset:
		return AssetAccount
	case identifiers.KindLogic:
		return LogicAccount
	default:
		return InvalidAccount
	}
}

func (at AccountType) String() string {
	switch at {
	case SystemAccount:
		return "SystemAccount"
	case LogicAccount:
		return "LogicAccount"
	case AssetAccount:
		return "AssetAccount"
	case RegularAccount:
		return "RegularAccount"
	default:
		return "InvalidAccount"
	}
}

const MinWeight = 1000

type AccountKey struct {
	ID                 uint64
	PublicKey          []byte
	Weight             uint64
	SignatureAlgorithm uint64
	Revoked            bool
	SequenceID         uint64
}

func (a *AccountKey) copy() *AccountKey {
	pk := make([]byte, len(a.PublicKey))
	copy(pk, a.PublicKey)

	return &AccountKey{
		ID:                 a.ID,
		PublicKey:          pk,
		Weight:             a.Weight,
		SignatureAlgorithm: a.SignatureAlgorithm,
		Revoked:            a.Revoked,
		SequenceID:         a.SequenceID,
	}
}

type AccountKeys []*AccountKey

func (a AccountKeys) Hash() (Hash, error) {
	return PoloHash(a)
}

func (a AccountKeys) Copy() AccountKeys {
	newAccountKeys := make(AccountKeys, len(a))

	for i, accountKey := range a {
		newAccountKeys[i] = accountKey.copy()
	}

	return newAccountKeys
}

func (a AccountKeys) CopyForInheritAccount() AccountKeys {
	newAccountKeys := a.Copy()

	for i := 0; i < len(a); i++ {
		newAccountKeys[i].SequenceID = 0
	}

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
	InheritedAccount     identifiers.Identifier `json:"inherited_account"`
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

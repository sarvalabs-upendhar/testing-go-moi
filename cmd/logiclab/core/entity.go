package core

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
)

type AccountKind int

const (
	UnknownAccKind AccountKind = iota
	UserAccount
	LogicAccount
	AssetAccount
)

type Account struct {
	Kind AccountKind
	Name string
	Data []byte
}

func (acc *Account) Encode() ([]byte, error) {
	rawData, err := polo.Polorize(acc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize account")
	}

	return rawData, nil
}

func (acc *Account) Decode(bytes []byte) error {
	if err := polo.Depolorize(acc, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize account")
	}

	return nil
}

func (kind AccountKind) String() string {
	switch kind {
	case UserAccount:
		return "user"
	case LogicAccount:
		return "logic"
	case AssetAccount:
		return "asset"
	default:
		panic("unknown account type")
	}
}

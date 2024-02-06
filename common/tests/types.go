package tests

import (
	identifiers "github.com/sarvalabs/go-moi-identifiers"
)

type AccountWithMnemonic struct {
	Addr     identifiers.Address `json:"address"`
	Mnemonic string              `json:"mnemonic"`
}

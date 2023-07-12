package tests

import (
	"github.com/sarvalabs/go-moi/common"
)

type AccountWithMnemonic struct {
	Addr     common.Address `json:"address"`
	Mnemonic string         `json:"mnemonic"`
}

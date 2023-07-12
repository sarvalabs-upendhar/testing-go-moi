package tests

import (
	"github.com/sarvalabs/moichain/common"
)

type AccountWithMnemonic struct {
	Addr     common.Address `json:"address"`
	Mnemonic string         `json:"mnemonic"`
}

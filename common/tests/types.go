package tests

import "github.com/sarvalabs/moichain/types"

type AccountWithMnemonic struct {
	Addr     types.Address `json:"address"`
	Mnemonic string        `json:"mnemonic"`
}

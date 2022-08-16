package main

import (
	"fmt"
	"gitlab.com/sarvalabs/moichain/cmd"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
)

func main() {
	fmt.Println("Hello", ktypes.BytesToAddress(ktypes.GetHash([]byte("sargaAccount")).Bytes()).Hex())
	cmd.Execute()
}

package main

import (
	"fmt"

	"gitlab.com/sarvalabs/moichain/cmd"
	"gitlab.com/sarvalabs/moichain/types"
)

func main() {
	fmt.Println("Hello", types.BytesToAddress(types.GetHash([]byte("sargaAccount")).Bytes()).Hex())
	cmd.Execute()
}

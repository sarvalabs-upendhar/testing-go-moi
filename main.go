package main

import (
	"fmt"

	"github.com/sarvalabs/moichain/cmd"
	"github.com/sarvalabs/moichain/types"
)

func main() {
	fmt.Println("Hello", types.BytesToAddress(types.GetHash([]byte("sargaAccount")).Bytes()).Hex())
	cmd.Execute()
}

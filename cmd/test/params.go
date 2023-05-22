package test

import (
	"github.com/sarvalabs/moichain/cmd/server"
	"github.com/sarvalabs/moichain/types"
)

var (
	accAddresses          []string
	behaviouralNodesCount int
	randomNodesCount      int
	directoryIndex        int
	count                 int
	bootnode              string
	jaegerAddress         string
	password              string
	logFilePath           string
	peerListFilePath      string
	genesisFilePath       string
	instancesFilePath     string
	accountsFilePath      string
	port                  int
	peerList              *server.PeerList
)

type AccountWithMnemonic struct {
	Addr     types.Address `json:"address"`
	Mnemonic string        `json:"mnemonic"`
}

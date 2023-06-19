package internal

import (
	"github.com/sarvalabs/moichain/cmd/common"
)

var (
	premineAmount          uint64
	accAddresses           []string
	behaviouralNodesCount  int
	randomNodesCount       int
	directoryIndex         int
	accountCount           int
	count                  int
	bootnode               string
	jaegerAddress          string
	password               string
	logFilePath            string
	peerListFilePath       string
	genesisFilePath        string
	writeInstancesFilePath string
	readInstancesFilePath  string
	writeAccountsFilePath  string
	readAccountsFilePath   string
	GuardianLogicPath      string
	port                   int
	peerList               *common.PeerList
)

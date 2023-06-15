package test

import (
	"github.com/sarvalabs/moichain/cmd/server"
)

var (
	premineAmount         uint64
	accAddresses          []string
	behaviouralNodesCount int
	randomNodesCount      int
	directoryIndex        int
	accountCount          int
	count                 int
	bootnode              string
	jaegerAddress         string
	password              string
	logFilePath           string
	peerListFilePath      string
	genesisFilePath       string
	instancesFilePath     string
	accountsFilePath      string
	GuardianLogicPath     string
	port                  int
	peerList              *server.PeerList
)

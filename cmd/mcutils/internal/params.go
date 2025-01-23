package internal

import (
	"github.com/sarvalabs/go-moi/cmd/common"
)

var (
	premineAmount          uint64
	consensusNodesCount    int
	directoryIndex         int
	accountCount           int
	count                  int
	bootnode               string
	otlpAddress            string
	token                  string
	password               string
	peerListFilePath       string
	genesisFilePath        string
	writeInstancesFilePath string
	directoryPath          string
	readInstancesFilePath  string
	writeAccountsFilePath  string
	readAccountsFilePath   string
	GuardianLogicPath      string
	writeLogsToFile        bool
	shouldExecute          bool
	enableSortition        bool
	libp2pPort             int
	jsonrpcPort            int
	peerList               *common.PeerList
)

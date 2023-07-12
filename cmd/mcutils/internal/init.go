package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/hexutil"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/crypto/poi"
)

func GetInitCommand() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialises necessary config files.",
		Run:   runCommand,
	}

	parseFlags(initCmd)

	return initCmd
}

func runCommand(cmd *cobra.Command, args []string) {
	setupTestEnv()
}

func parseFlags(initcmd *cobra.Command) {
	initcmd.PersistentFlags().IntVar(
		&port,
		"port",
		0,
		"Provide the starting port number",
	)
	initcmd.PersistentFlags().IntVar(
		&count,
		"dir-count",
		10,
		"Number of test directories",
	)
	initcmd.PersistentFlags().IntVar(
		&directoryIndex,
		"directory-index",
		0,
		"Directory Index",
	)
	initcmd.PersistentFlags().StringVar(
		&bootnode,
		"bootnode",
		"",
		"Bootnode Multi-Address",
	)
	initcmd.PersistentFlags().StringVar(
		&jaegerAddress,
		"jaeger-address",
		"",
		"Jaeger Address",
	)
	initcmd.PersistentFlags().StringVar(
		&password,
		"password",
		"test123",
		"Password to unlock key store.",
	)
	initcmd.PersistentFlags().StringVar(
		&peerListFilePath,
		"peer-list",
		"",
		"Peer list file path.",
	)
	initcmd.PersistentFlags().StringVar(
		&writeInstancesFilePath,
		"instances-path",
		"instances.json",
		"Path to instances.json file.",
	)
	initcmd.PersistentFlags().BoolVar(
		&writeLogsToFile,
		"writeLogsToFile",
		false,
		"Enabling this flag will save logs to the logfile located in data-dir/log/.",
	)

	if err := cobra.MarkFlagRequired(initcmd.PersistentFlags(), "port"); err != nil {
		cmdCommon.Err(err)
	}

	if err := cobra.MarkFlagRequired(initcmd.PersistentFlags(), "bootnode"); err != nil {
		cmdCommon.Err(err)
	}
}

func CreateConfigFile(datadir string, index int) []byte {
	data := cmdCommon.Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Genesis:        "genesis.json",
		Network: cmdCommon.NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(port+index),
			},
			JSONRPCAddr: "0.0.0.0:" + strconv.Itoa(config.DefaultJSONRPCPort+index),
			BootStrapPeers: []string{
				bootnode,
			},
			TrustedPeers:       peerList.TrustedPeers,
			StaticPeers:        peerList.StaticPeers,
			InboundConnLimit:   config.DefaultInboundConnLimit,
			OutboundConnLimit:  config.DefaultOutboundConnLimit,
			CorsAllowedOrigins: []string{"*"},
		},
		Syncer: cmdCommon.SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(config.DefaultSyncMode),
			EnableSnapSync: true,
		},
		Consensus: cmdCommon.ConsensusConfig{
			TimeoutPropose:        30000,
			TimeoutProposeDelta:   50000,
			TimeoutPrevote:        10000,
			TimeoutPrevoteDelta:   50000,
			TimeoutPrecommit:      10000,
			TimeoutPrecommitDelta: 50000,
			TimeoutCommit:         10000,
			Precision:             1000,
			MessageDelay:          5500,
			AccountWaitTime:       1500,
			OperatorSlots:         -1,
			ValidatorSlots:        3,
		},
		DB: cmdCommon.DBConfig{
			CleanDB:     false,
			MaxSnapSize: config.DefaultSnapSize, // 1GB limit
		},
		Execution: cmdCommon.ExecutionConfig{
			FuelLimit: hexutil.Big(*config.DefaultFuelLimit),
		},
		Ixpool: cmdCommon.IxPoolConfig{
			Mode:       config.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*config.DefaultIxPriceLimit),
			MaxSlots:   config.DefaultMaxIXPoolSlots,
		},
		Telemetry: cmdCommon.Telemetry{
			PrometheusAddr: ":" + strconv.Itoa(config.DefaultPrometheusPort+index),
			JaegerAddr:     jaegerAddress,
		},
		Vault: cmdCommon.VaultConfig{
			DataDir:      datadir,
			NodePassword: password,
		},
	}

	if writeLogsToFile {
		data.LogFilePath = datadir + config.DefaultLogDirectory
	}

	file, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		cmdCommon.Err(err)
	}

	return file
}

func setupTestEnv() {
	instances := make([]cmdCommon.Instance, count)

	ip, err := cmdCommon.GetThisNodeIP()
	if err != nil {
		cmdCommon.Err(err)
	}

	for i := 0; i < count; i++ {
		if err = os.MkdirAll(filepath.Join(fmt.Sprintf("test_%d", directoryIndex+i), "libp2p"), os.ModePerm); err != nil {
			cmdCommon.Err(err)
		}

		if err = os.Mkdir(filepath.Join(fmt.Sprintf("test_%d", directoryIndex+i), "consensus"), os.ModePerm); err != nil {
			cmdCommon.Err(err)
		}

		publicKey, kramaID, err := poi.RandGenKeystore(fmt.Sprintf("test_%d", directoryIndex+i), password)
		if err != nil {
			cmdCommon.Err(err)
		}

		peerList, err = cmdCommon.ReadPeerList(peerListFilePath)
		if err != nil {
			cmdCommon.Err(err)
		}

		configData := CreateConfigFile(fmt.Sprintf("test_%d", directoryIndex+i), directoryIndex+i)

		if err := ioutil.WriteFile(fmt.Sprintf("test_%d/config.json", directoryIndex+i), configData, 0o600); err != nil {
			cmdCommon.Err(err)
		}

		instances[i].KramaID = string(kramaID)
		instances[i].RPCUrl = ip + ":" + strconv.Itoa(1600+directoryIndex+i)
		instances[i].ConsensusKey = common.BytesToHex(publicKey)
	}

	instancesFile, err := json.MarshalIndent(instances, "", "\t")
	if err != nil {
		cmdCommon.Err(err)
	}

	if err = ioutil.WriteFile(writeInstancesFilePath, instancesFile, os.ModePerm); err != nil {
		cmdCommon.Err(err)
	}
}

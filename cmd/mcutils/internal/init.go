package internal

import (
	"encoding/json"
	"fmt"
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
		&libp2pPort,
		"libp2pPort",
		0,
		"Provide the starting lib-p2p port number",
	)
	initcmd.PersistentFlags().IntVar(
		&jsonrpcPort,
		"jsonrpcPort",
		1600,
		"Provide the starting json rpc port number",
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
		&otlpAddress,
		"otlp-address",
		"",
		"OTLP Address",
	)
	initcmd.PersistentFlags().StringVar(
		&token,
		"token",
		"",
		"Token",
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

	if err := cobra.MarkFlagRequired(initcmd.PersistentFlags(), "libp2pPort"); err != nil {
		cmdCommon.Err(err)
	}

	if err := cobra.MarkFlagRequired(initcmd.PersistentFlags(), "jsonrpcPort"); err != nil {
		cmdCommon.Err(err)
	}

	if err := cobra.MarkFlagRequired(initcmd.PersistentFlags(), "bootnode"); err != nil {
		cmdCommon.Err(err)
	}
}

func CreateConfigFile(datadir string, index int, ipAddr string) []byte {
	data := cmdCommon.Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Genesis:        "genesis.json",
		Network: cmdCommon.NetworkConfig{
			Libp2pAddr: []string{
				fmt.Sprintf("/ip4/%s/tcp/%d", ipAddr, libp2pPort+index),
				fmt.Sprintf("/ip4/%s/udp/%d/%s", ipAddr, libp2pPort+index, "quic-v1"),
				"/ip6/::/tcp/" + strconv.Itoa(libp2pPort+index),
				"/ip6/::/udp/" + strconv.Itoa(libp2pPort+index) + "/quic-v1",
			},
			JSONRPCAddr: "0.0.0.0:" + strconv.Itoa(jsonrpcPort+index),
			BootStrapPeers: []string{
				bootnode,
			},
			TrustedPeers:       peerList.NetworkTrustedPeers,
			StaticPeers:        peerList.NetworkStaticPeers,
			InboundConnLimit:   config.DefaultInboundConnLimit,
			OutboundConnLimit:  config.DefaultOutboundConnLimit,
			MinimumConnections: config.DefaultMinimumConnections,
			MaximumConnections: config.DefaultMaximumConnections,
			DiscoveryInterval:  config.DefaultDiscoveryInterval,
			CorsAllowedOrigins: []string{"*"},
			RefreshSenatus:     true,
		},
		Syncer: cmdCommon.SyncerConfig{
			ShouldExecute:  true,
			SyncMode:       int(config.DefaultSyncMode),
			EnableSnapSync: true,
			TrustedPeers:   peerList.SyncerTrustedPeers,
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
			MaxGossipPeers:        5,
			MinGossipPeers:        3,
		},
		DB: cmdCommon.DBConfig{
			CleanDB:     false,
			MaxSnapSize: config.DefaultSnapSize, // 6GB limit
		},
		Execution: cmdCommon.ExecutionConfig{
			FuelLimit: hexutil.Uint64(config.DefaultFuelLimit),
		},
		IxPool: cmdCommon.IxPoolConfig{
			Mode:       config.DefaultIxPoolMode,
			PriceLimit: hexutil.Big(*config.DefaultIxPriceLimit),
			MaxSlots:   config.DefaultMaxIXPoolSlots,
		},
		Telemetry: cmdCommon.Telemetry{
			PrometheusAddr: ":" + strconv.Itoa(config.DefaultPrometheusPort+index),
			OtlpAddress:    otlpAddress,
			Token:          token,
		},
		Vault: cmdCommon.VaultConfig{
			DataDir:      datadir,
			NodePassword: password,
		},
		JSONRPC: cmdCommon.JSONRPCConfig{
			TesseractRangeLimit: config.DefaultTesseractRangeLimit,
			BatchLengthLimit:    config.DefaultBatchLengthLimit,
		},
		NetworkID: config.Local,
		State: cmdCommon.StateConfig{
			TreeCacheSize: config.DefaultTreeCacheSize,
		},
		GenesisTime: 0,
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
	instances := make([]common.Instance, count)

	ip, err := cmdCommon.GetIP()
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

		configData := CreateConfigFile(fmt.Sprintf("test_%d", directoryIndex+i), directoryIndex+i, ip)

		if err := os.WriteFile(fmt.Sprintf("test_%d/config.json", directoryIndex+i), configData, 0o600); err != nil {
			cmdCommon.Err(err)
		}

		instances[i].KramaID = string(kramaID)
		instances[i].RPCUrl = ip + ":" + strconv.Itoa(jsonrpcPort+directoryIndex+i)
		instances[i].ConsensusKey = common.BytesToHex(publicKey)
	}

	instancesFile, err := json.MarshalIndent(instances, "", "\t")
	if err != nil {
		cmdCommon.Err(err)
	}

	if err = os.WriteFile(writeInstancesFilePath, instancesFile, os.ModePerm); err != nil {
		cmdCommon.Err(err)
	}
}

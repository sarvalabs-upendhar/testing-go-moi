package test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	cmdCommon "github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/cmd/server"
	"github.com/sarvalabs/moichain/common"

	"github.com/sarvalabs/moichain/types"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/mudra/poi"
)

func GetInitCommand() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialised necessary config files",
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
		"count",
		10,
		"Number of test directories",
	)
	initcmd.PersistentFlags().IntVar(
		&directoryIndex,
		"directory-index",
		0,
		"Directory Index",
	)
	initcmd.PersistentFlags().StringVar(&bootnode, "bootnode",
		"/ip4/139.59.73.20/tcp/4001/p2p/16Uiu2HAmVFp1xtDsokTWuCTkThQSDetjqTx7W9EwcCSxrXqH33Dm",
		"Bootnode MultiAddr",
	)
	initcmd.PersistentFlags().StringVar(&jaegerAddress, "jaegerAddress",
		"",
		"Jeager Address",
	)
	initcmd.PersistentFlags().StringVar(&password,
		"password",
		"test123",
		"password to unlock key store",
	)
	initcmd.PersistentFlags().StringVar(
		&logFilePath,
		"logfile",
		"",
		"path at which you'd like to store the logs file",
	)
	initcmd.PersistentFlags().StringVar(
		&peerListFilePath,
		"peer-list",
		"",
		"peer list file path",
	)

	if err := cobra.MarkFlagRequired(initcmd.PersistentFlags(), "port"); err != nil {
		cmdCommon.Err(err)
	}
}

func CreateConfigFile(datadir string, index int) []byte {
	data := server.Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Genesis:        "genesis.json",
		Network: server.NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(port+index),
			},
			JSONRPCAddr: "0.0.0.0:" + strconv.Itoa(common.DefaultJSONRPCPort+index),
			BootStrapPeers: []string{
				bootnode,
			},
			TrustedPeers: peerList.TrustedPeers,
			StaticPeers:  peerList.StaticPeers,
		},
		Ixpool: server.IxPoolConfig{
			Mode:       common.DefaultIxPoolMode,
			PriceLimit: common.DefaultIxPriceLimit,
		},
		Telemetry: server.Telemetry{
			PrometheusAddr: ":" + strconv.Itoa(common.DefaultPrometheusPort+index),
			JaegerAddr:     jaegerAddress,
		},
		Vault: server.VaultConfig{
			DataDir:      datadir,
			NodePassword: password,
		},
		LogFilePath: logFilePath,
	}

	file, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		cmdCommon.Err(err)
	}

	return file
}

func setupTestEnv() {
	instances := make([]cmdCommon.Instance, count)

	ip, err := getThisNodeIP()
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

		if err = StoreKey(kramaID, publicKey); err != nil {
			cmdCommon.Err(err)
		}

		peerList, err = server.ReadPeerList(peerListFilePath)
		if err != nil {
			cmdCommon.Err(err)
		}

		configData := CreateConfigFile(fmt.Sprintf("test_%d", directoryIndex+i), directoryIndex+i)

		if err := ioutil.WriteFile(fmt.Sprintf("test_%d/config.json", directoryIndex+i), configData, 0o600); err != nil {
			cmdCommon.Err(err)
		}

		instances[i].KramaID = string(kramaID)
		instances[i].RPCUrl = ip + ":" + strconv.Itoa(1600+directoryIndex+i)
		instances[i].ConsensusKey = types.BytesToHex(publicKey)
	}

	instancesFile, err := json.MarshalIndent(instances, "", "\t")
	if err != nil {
		cmdCommon.Err(err)
	}

	if err = ioutil.WriteFile("instances.json", instancesFile, os.ModePerm); err != nil {
		cmdCommon.Err(err)
	}
}

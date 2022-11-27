package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"

	"github.com/spf13/cobra"
)

var (
	directoryIndex int
	count          int
	bootnode       string
	jaegerAddress  string
	password       string
	logFilePath    string
	port           int
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Test Directories created")

		for i := 0; i < count; i++ {
			if err := os.MkdirAll(fmt.Sprintf("test_%d/libp2p", directoryIndex+i), os.ModePerm); err != nil {
				Err(err)
			}

			if err := os.Mkdir(fmt.Sprintf("test_%d/consensus", directoryIndex+i), os.ModePerm); err != nil {
				Err(err)
			}

			publicKey, kramaID, err := poi.RandGenKeystore(fmt.Sprintf("test_%d", directoryIndex+i), password)
			if err != nil {
				Err(err)
			}

			if err := StoreKey(kramaID, publicKey); err != nil {
				Err(err)
			}

			configData := CreateConfigFile(fmt.Sprintf("test_%d", directoryIndex+i), directoryIndex+i)

			if err := ioutil.WriteFile(fmt.Sprintf("test_%d/config.json", directoryIndex+i), configData, 0o600); err != nil {
				Err(err)
			}
		}
	},
}

func init() {
	testCmd.AddCommand(initCmd)

	testCmd.PersistentFlags().IntVar(
		&port,
		"port",
		0,
		"Provide the starting port number",
	)
	testCmd.PersistentFlags().IntVar(
		&count,
		"count",
		10,
		"Number of test directories",
	)
	testCmd.PersistentFlags().IntVar(
		&directoryIndex,
		"directory-index",
		0,
		"Directory Index",
	)
	testCmd.PersistentFlags().StringVar(&bootnode, "bootnode",
		"/ip4/139.59.73.20/tcp/4001/p2p/16Uiu2HAmVFp1xtDsokTWuCTkThQSDetjqTx7W9EwcCSxrXqH33Dm",
		"Bootnode MultiAddr",
	)
	testCmd.PersistentFlags().StringVar(&jaegerAddress, "jaegerAddress",
		"",
		"Jeager Address",
	)
	testCmd.PersistentFlags().StringVar(&password,
		"password",
		"test123",
		"password to unlock key store",
	)
	testCmd.PersistentFlags().StringVar(
		&logFilePath,
		"logfile",
		"",
		"path at which you'd like to store the logs file",
	)

	if err := cobra.MarkFlagRequired(testCmd.PersistentFlags(), "port"); err != nil {
		Err(err)
	}
}

func CreateConfigFile(datadir string, index int) []byte {
	data := Config{
		NodeType:       7,
		KramaIDVersion: 1,
		Genesis:        "genesis.json",
		Network: NetworkConfig{
			Libp2pAddr: []string{
				"/ip4/0.0.0.0/tcp/" + strconv.Itoa(port+index),
			},
			ProtocolID:  "MOI",
			JSONRPCAddr: "0.0.0.0:" + strconv.Itoa(1600+index),
			BootStrapPeers: []string{
				bootnode,
			},
		},
		Telemetry: Telemetry{
			PrometheusAddr: ":" + strconv.Itoa(30000+index),
			JaegerAddr:     jaegerAddress,
		},
		Vault: VaultConfig{
			DataDir:      datadir,
			NodePassword: password,
		},
		LogFilePath: logFilePath,
	}

	file, err := json.MarshalIndent(data, "", "")
	Err(err)

	return file
}

type Request struct {
	ID  string `json:"kramaID"`
	Key string `json:"publicKey"`
}

func StoreKey(id id.KramaID, key []byte) error {
	data, err := json.Marshal(Request{ID: string(id), Key: hex.EncodeToString(key)})
	if err != nil {
		return err
	}

	res, err := http.Post("http://159.203.191.91/api/store", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return errors.New("error storing the public key")
	}

	return nil
}

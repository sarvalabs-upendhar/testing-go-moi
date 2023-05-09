package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"
)

var (
	directoryIndex   int
	count            int
	bootnode         string
	jaegerAddress    string
	password         string
	logFilePath      string
	peerListFilePath string
	port             int
	peerList         *PeerList
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialised necessary config files",
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Test Directories created")

		for i := 0; i < count; i++ {
			if err := os.MkdirAll(filepath.Join(fmt.Sprintf("test_%d", directoryIndex+i), "libp2p"), os.ModePerm); err != nil {
				Err(err)
			}

			if err := os.Mkdir(filepath.Join(fmt.Sprintf("test_%d", directoryIndex+i), "consensus"), os.ModePerm); err != nil {
				Err(err)
			}

			publicKey, kramaID, err := poi.RandGenKeystore(fmt.Sprintf("test_%d", directoryIndex+i), password)
			if err != nil {
				Err(err)
			}

			if err := StoreKey(kramaID, publicKey); err != nil {
				Err(err)
			}

			peerList, err = ReadPeerList(peerListFilePath)
			if err != nil {
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
	testnetCmd.AddCommand(initCmd)

	testnetCmd.PersistentFlags().IntVar(
		&port,
		"port",
		0,
		"Provide the starting port number",
	)
	testnetCmd.PersistentFlags().IntVar(
		&count,
		"count",
		10,
		"Number of test directories",
	)
	testnetCmd.PersistentFlags().IntVar(
		&directoryIndex,
		"directory-index",
		0,
		"Directory Index",
	)
	testnetCmd.PersistentFlags().StringVar(&bootnode, "bootnode",
		"/ip4/139.59.73.20/tcp/4001/p2p/16Uiu2HAmVFp1xtDsokTWuCTkThQSDetjqTx7W9EwcCSxrXqH33Dm",
		"Bootnode MultiAddr",
	)
	testnetCmd.PersistentFlags().StringVar(&jaegerAddress, "jaegerAddress",
		"",
		"Jeager Address",
	)
	testnetCmd.PersistentFlags().StringVar(&password,
		"password",
		"test123",
		"password to unlock key store",
	)
	testnetCmd.PersistentFlags().StringVar(
		&logFilePath,
		"logfile",
		"",
		"path at which you'd like to store the logs file",
	)
	testnetCmd.PersistentFlags().StringVar(
		&peerListFilePath,
		"peer-list",
		"",
		"peer list file path",
	)

	if err := cobra.MarkFlagRequired(testnetCmd.PersistentFlags(), "port"); err != nil {
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
			JSONRPCAddr: "0.0.0.0:" + strconv.Itoa(1600+index),
			BootStrapPeers: []string{
				bootnode,
			},
			TrustedPeers: peerList.TrustedPeers,
			StaticPeers:  peerList.StaticPeers,
		},
		Ixpool: IxPoolConfig{
			Mode:       0,
			PriceLimit: big.NewInt(10),
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
	if err != nil {
		Err(err)
	}

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

	res, err := http.Post("http://91.107.196.74/api/store", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return errors.New("error storing the public key")
	}

	return nil
}

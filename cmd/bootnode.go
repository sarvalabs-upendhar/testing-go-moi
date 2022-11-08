package cmd

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
)

var (
	portNumber int
	ipAddress  string
	keyFile    string
)

// bootnodeCmd represents the bootnode init command
var bootnodeCmd = &cobra.Command{
	Use:   "bootnode",
	Short: "A command to initialize bootnode",
	Long: `This command is used to initialize bootnode for kad-dht application using libp2p

	Usage: Run 'moichain bootnode --port [port number] --ip-addr [listener address] --key [key filename]'`,
	Run: func(cmd *cobra.Command, args []string) {
		var KadDHT *dht.IpfsDHT

		path := filepath.Join(keyFile)

		privateKey, err := getPrivateKey(path)
		if err != nil {
			log.Panic("Failed to get private keys : ", err)
		}

		ctx := context.Background()

		// 0.0.0.0 will listen on any interface device.
		sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipAddress, portNumber))

		selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			dhtOpts := []dht.Option{
				dht.Mode(dht.ModeServer),
				dht.ProtocolPrefix("MOI"),
			}
			Dht, errs := dht.New(ctx, h, dhtOpts...)
			if errs != nil {
				panic(err)
			}
			KadDHT = Dht

			return Dht, nil
		})
		// libp2p.New constructs a new libp2p Host.
		// Other options can be added here.
		host, err := libp2p.New(
			libp2p.ListenAddrs(sourceMultiAddr),
			libp2p.NATPortMap(),
			libp2p.ForceReachabilityPublic(),
			libp2p.EnableNATService(),
			libp2p.Identity(privateKey),
			selfRouting,
		)
		if err != nil {
			panic(err)
		}
		fmt.Println("")
		fmt.Printf("[*] Your Bootstrap ID Is: /ip4/%s/tcp/%v/p2p/%s\n", ipAddress, portNumber, host.ID().String())
		fmt.Println("")
		var input string
		for {
			fmt.Println("Please enter your input:")
			fmt.Println("1: DHT stats")
			fmt.Println("2: Exit")

			if _, err := fmt.Scanf("%s", &input); err != nil {
				log.Fatal(err)
			}

			if input == "1" {
				for _, v := range KadDHT.RoutingTable().ListPeers() {
					protocols, _ := host.Peerstore().GetProtocols(v)
					fmt.Println("Peer ID", v, "Supported-Protocols", protocols)
					fmt.Println("Peer Info", host.Peerstore().PeerInfo(v))
				}
			} else if input == "2" {
				return
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(bootnodeCmd)
	bootnodeCmd.PersistentFlags().IntVar(&portNumber, "port", 4001, "Provide the port number")
	bootnodeCmd.PersistentFlags().StringVar(&ipAddress, "ip-addr", "0.0.0.0", "Provide the listener ip address")
	bootnodeCmd.PersistentFlags().StringVar(&keyFile, "key", "file.key", "Provide the key file path")
}

func getPrivateKey(keyfile string) (crypto.PrivKey, error) {
	// Declare a variable for the private key
	var key crypto.PrivKey

	// Check if the key file exists at the given path
	if _, err := os.Stat(keyfile); err == nil {
		// Keyfile already exists
		log.Println(" Found an existing key")

		// Read the keyfile into bytes data and check for errors
		data, err := ioutil.ReadFile(keyfile)
		if err != nil {
			// Return the error
			return nil, err
		}

		// Unmarshal the private key data into the key and check for errors
		key, err = crypto.UnmarshalPrivateKey(data)
		if err != nil {
			// Return the error
			return nil, err
		}
	} else {
		// Keyfile does not exist
		log.Println("Generating new key")

		// Generate a new RSA keypair and check for errors
		key, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 256, crand.Reader)
		if err != nil {
			// Return the error
			return nil, err
		}

		// Marshal the private key and check for errors
		data, err := crypto.MarshalPrivateKey(key)
		if err != nil {
			// Return the error
			return nil, err
		}

		// Write the private key data to keyfile and check for errors
		if err := ioutil.WriteFile(keyfile, data, 0o600); err != nil {
			// Return the error
			return nil, err
		}
	}

	// Return the key and a nil error
	return key, nil
}

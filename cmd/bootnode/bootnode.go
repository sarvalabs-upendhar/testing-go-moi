package bootnode

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/multiformats/go-multiaddr"
	cmdcommon "github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/common"
	"github.com/spf13/cobra"
)

var (
	portNumber int
	ipAddress  string
	keyFile    string
)

func GetCommand() *cobra.Command {
	bootnodeCmd := &cobra.Command{
		Use:   "bootnode",
		Short: "A command to initialize moichain bootnode",
		Run:   runCommand,
	}

	parseFlags(bootnodeCmd)

	return bootnodeCmd
}

func parseFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().IntVar(&portNumber, "port", 4001, "Provide the port number")
	cmd.PersistentFlags().StringVar(&ipAddress, "ip-addr", "0.0.0.0", "Provide the listener ip address")
	cmd.PersistentFlags().StringVar(&keyFile, "key", "file.key", "Provide the key file path")
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
		err = ioutil.WriteFile(keyfile, data, 0o600)
		if err != nil {
			// Return the error
			return nil, err
		}
	}

	// Return the key and a nil error
	return key, nil
}

func runCommand(cmd *cobra.Command, args []string) {
	startBootNode()
}

func startBootNode() {
	var KadDHT *dht.IpfsDHT

	privateKey, err := getPrivateKey(keyFile)
	if err != nil {
		log.Panic("Failed to get private keys : ", err)
	}

	// 0.0.0.0 will listen on any interface device.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipAddress, portNumber))

	selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		dhtOpts := []dht.Option{
			dht.Mode(dht.ModeServer),
			dht.ProtocolPrefix(common.MOIProtocolStream),
		}
		Dht, err := dht.New(context.Background(), h, dhtOpts...)
		if err != nil {
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
			cmdcommon.Err(err)
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
}

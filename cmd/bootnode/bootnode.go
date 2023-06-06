package bootnode

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"

	badger "github.com/ipfs/go-ds-badger"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"

	cmdcommon "github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/common"
)

var (
	portNumber    int
	ipAddress     string
	keyFile       string
	minConnReq    int
	maxConnReq    int
	dataStorePath string
	peers         string
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
	cmd.PersistentFlags().IntVar(&minConnReq, "min-conn-req", 200, "max number of connections allowed")
	cmd.PersistentFlags().IntVar(&maxConnReq, "max-conn-req", 400, "max number of connections allowed")
	cmd.PersistentFlags().StringVar(&dataStorePath, "data-store-path", "", "path to store bootnode data")
	cmd.PersistentFlags().StringVar(&peers, "peers", "", "comma-separated list of peers to connect")
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
	sourceMultiAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipAddress, portNumber))
	if err != nil {
		panic(err)
	}

	dhtOptions := make([]dht.Option, 0)

	if dataStorePath != "" {
		dataStore, err := badger.NewDatastore(dataStorePath, nil)
		if err != nil {
			panic(err)
		}

		dhtOptions = append(dhtOptions, dht.Datastore(dataStore))
	}

	dhtOptions = append(dhtOptions, dht.Mode(dht.ModeServer))
	dhtOptions = append(dhtOptions, dht.ProtocolPrefix(common.MOIProtocolStream))

	selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		Dht, err := dht.New(context.Background(), h, dhtOptions...)
		if err != nil {
			panic(err)
		}
		KadDHT = Dht

		return Dht, nil
	})

	mgr, err := connmgr.NewConnManager(minConnReq, maxConnReq)
	if err != nil {
		panic(err)
	}

	resourceManager, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits))
	if err != nil {
		panic(err)
	}

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	p2pHost, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.NATPortMap(),
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableNATService(),
		libp2p.Identity(privateKey),
		libp2p.ConnectionManager(mgr),
		selfRouting,
		libp2p.ResourceManager(resourceManager),
	)
	if err != nil {
		panic(err)
	}

	peerAddresses := strings.Split(peers, ",")
	peerInfoList := make([]peer.AddrInfo, 0, len(peerAddresses))

	for _, addr := range peerAddresses {
		if addr == "" {
			continue
		}

		maddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			log.Printf("Invalid peer address: %s", err)

			continue
		}

		peerinfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Printf("Failed to parse peer address: %s", err)

			continue
		}

		err = p2pHost.Connect(context.Background(), *peerinfo)
		if err != nil {
			log.Printf("Failed to connect to peer %s: %s", peerinfo.ID, err)
		}

		peerInfoList = append(peerInfoList, *peerinfo)
	}

	// Start a background task to maintain connections
	go maintainConnections(context.Background(), p2pHost, peerInfoList)

	fmt.Println("")
	fmt.Printf("[*] Your Bootstrap ID Is: /ip4/%s/tcp/%v/p2p/%s\n", ipAddress, portNumber, p2pHost.ID().String())
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
				protocols, _ := p2pHost.Peerstore().GetProtocols(v)
				fmt.Println("Peer ID", v, "Supported-Protocols", protocols)
				fmt.Println("Peer Info", p2pHost.Peerstore().PeerInfo(v))
			}
		} else if input == "2" {
			return
		}
	}
}

func getPrivateKey(keyfile string) (crypto.PrivKey, error) {
	key, err := loadExistingKey(keyfile)
	if err != nil {
		if os.IsNotExist(err) {
			key, err = generateAndStoreNewKey(keyfile)
		}

		if err != nil {
			return nil, err
		}
	}

	return key, nil
}

func loadExistingKey(keyfile string) (crypto.PrivKey, error) {
	data, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return nil, err
	}

	key, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	log.Println("Found an existing key")

	return key, nil
}

func generateAndStoreNewKey(keyfile string) (crypto.PrivKey, error) {
	key, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 256, crand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new key: %w", err)
	}

	data, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := ioutil.WriteFile(keyfile, data, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write key to file: %w", err)
	}

	log.Println("Generated new key")

	return key, nil
}

func maintainConnections(ctx context.Context, host host.Host, peers []peer.AddrInfo) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, p := range peers {
				if host.Network().Connectedness(p.ID) != network.Connected {
					fmt.Printf("Reconnecting to peer %s", p.ID.String())

					err := host.Connect(ctx, p)
					if err != nil {
						fmt.Printf("Failed to connect to peer %s: %s\n", p.ID.String(), err)
					}
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

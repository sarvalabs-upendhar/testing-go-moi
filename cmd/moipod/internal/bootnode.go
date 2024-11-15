package internal

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/cmd/common"

	"github.com/libp2p/go-libp2p"
	"github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-moi/common/config"

	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"

	badger "github.com/ipfs/go-ds-badger"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/spf13/cobra"
)

var (
	portNumber    int
	ipv4Address   string
	ipv6Address   string
	keyFile       string
	minConnReq    int
	maxConnReq    int
	dataStorePath string
	peers         string
)

func GetBootNodeCommand() *cobra.Command {
	bootnodeCmd := &cobra.Command{
		Use:   "bootnode",
		Short: "A command to initialize MOI protocol bootnode.",
		Run:   runBootNodeCommand,
	}

	parseBootNodeFlags(bootnodeCmd)

	return bootnodeCmd
}

func parseBootNodeFlags(cmd *cobra.Command) {
	ipv4Addr, err := common.GetIP()
	if err != nil {
		common.Err(errors.Wrap(err, "failed to fetch IP addr"))
	}

	cmd.PersistentFlags().IntVar(&portNumber, "port", 4001, "Provide the port number.")
	cmd.PersistentFlags().StringVar(&ipv6Address, "ipv6-address", "::", "Provide the listener IPV6 address.")
	cmd.PersistentFlags().StringVar(&ipv4Address, "ipv4-address", ipv4Addr, "Provide the listener IPV4 address.")
	cmd.PersistentFlags().StringVar(&keyFile, "key-path", "file.key", "Path to keystore file.")
	cmd.PersistentFlags().IntVar(&minConnReq, "min-conn", 200, "Min number of connections allowed.")
	cmd.PersistentFlags().IntVar(&maxConnReq, "max-conn", 400, "Max number of connections allowed.")
	cmd.PersistentFlags().StringVar(&dataStorePath, "store-path", "", "Path to store bootnode data.")
	cmd.PersistentFlags().StringVar(
		&peers,
		"peer-list",
		"",
		"Comma-separated list of peers to connect. Format: <peerAddress1,peerAddress2,...>",
	)
}

func runBootNodeCommand(cmd *cobra.Command, args []string) {
	startBootNode()
}

func startBootNode() {
	privateKey, err := getPrivateKey(keyFile)
	if err != nil {
		log.Panic("failed to get private keys : ", err)
	}

	sourceMultiAddr4, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipv4Address, portNumber))
	if err != nil {
		panic(err)
	}

	sourceMultiAddr6, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/%d", ipv6Address, portNumber))
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
	dhtOptions = append(dhtOptions, dht.ProtocolPrefix(config.MOIProtocolStream))

	selfRouting := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		Dht, err := dht.New(context.Background(), h, dhtOptions...)
		if err != nil {
			panic(err)
		}

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

	addrsFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if ipv4Address != "0.0.0.0" {
			addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipv4Address, portNumber))
			if err != nil {
				panic(err)
			}

			addrs = append(addrs, addr)
		}

		if ipv6Address != "::" {
			addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/%d", ipv6Address, portNumber))
			if err != nil {
				panic(err)
			}

			addrs = append(addrs, addr)
		}

		return addrs
	}

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	p2pHost, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr4, sourceMultiAddr6),
		libp2p.ChainOptions(
			libp2p.Transport(tcp.NewTCPTransport),
			libp2p.Transport(quic.NewTransport),
		),
		libp2p.NATPortMap(),
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableNATService(),
		libp2p.Identity(privateKey),
		libp2p.ConnectionManager(mgr),
		selfRouting,
		libp2p.ResourceManager(resourceManager),
		libp2p.AddrsFactory(addrsFactory),
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
			log.Printf("failed to parse peer address: %s", err)

			continue
		}

		err = p2pHost.Connect(context.Background(), *peerinfo)
		if err != nil {
			log.Printf("failed to connect to peer %s: %s", peerinfo.ID, err)
		}

		peerInfoList = append(peerInfoList, *peerinfo)
	}

	// Start a background task to maintain connections
	go maintainConnections(context.Background(), p2pHost, peerInfoList)

	fmt.Println("")
	fmt.Printf("[*] Your Bootstrap ID Is: /ip4/%s/tcp/%v/p2p/%s\n", ipv4Address, portNumber, p2pHost.ID().String())
	fmt.Printf("[*] Your Bootstrap ID Is: /ip6/%s/tcp/%v/p2p/%s\n", ipv6Address, portNumber, p2pHost.ID().String())
	fmt.Println("")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit the bootnode")

	sig := <-sigChan

	fmt.Printf("Received signal: %v\n", sig)

	// Perform any necessary cleanup here
	fmt.Println("Exiting...")
	os.Exit(0)
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
	data, err := os.ReadFile(keyfile)
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

	if err := os.WriteFile(keyfile, data, 0o600); err != nil {
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
						fmt.Printf("failed to connect to peer %s: %s\n", p.ID.String(), err)
					}
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna/node"
	"github.com/sarvalabs/moichain/telemetry/tracing"
)

var ErrReadingConfig = errors.New("error reading config file")

var (
	AccountWaitTime   int
	OperatorSlots     int
	ValidatorSlots    int
	NetworkSize       uint64
	MTQ               float64
	EnableTracing     bool
	NoDiscovery       bool
	RefreshSenatus    bool
	Bootnode          string
	LogLevel          string
	JaegerAddress     string
	PeerListFilePath  string
	InboundConnLimit  int64
	OutboundConnLimit int64
	CleanDB           bool
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the moi-chain server",
	Run: func(cmd *cobra.Command, args []string) {
		cfgPath, err := cmd.Flags().GetString("config")
		Err(err)

		dataDir, err := cmd.Flags().GetString("data-dir")
		Err(err)

		SetupNode(dataDir, cfgPath)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.PersistentFlags().String("config", "config.json", "Config file name")
	serverCmd.PersistentFlags().IntVar(&AccountWaitTime, "wait-time", 0, "WaitTime per account")
	serverCmd.PersistentFlags().IntVar(&OperatorSlots, "operator-slots", -1, "Maximum number of operator slots")
	serverCmd.PersistentFlags().IntVar(&ValidatorSlots, "validator-slots", -1, "Maximum number of validator slots")
	serverCmd.PersistentFlags().Uint64Var(&NetworkSize, "network-size", 12, "Network Size")
	serverCmd.PersistentFlags().Float64Var(&MTQ, "mtq", 0.7, "Default MTQ")
	serverCmd.PersistentFlags().String("data-dir", "test-chain", "Data directory location")
	serverCmd.PersistentFlags().BoolVar(&CleanDB, "clean-db", false, "Deletes the data stored in database")
	serverCmd.PersistentFlags().BoolVar(&EnableTracing, "enable-tracing", false, "Enable Tracing")
	serverCmd.PersistentFlags().BoolVar(&NoDiscovery, "no-discovery", false, "Disable peer discovery")
	serverCmd.PersistentFlags().BoolVar(&RefreshSenatus, "refresh-senatus", false, "Update the senatus with new peers")
	serverCmd.PersistentFlags().StringVar(&JaegerAddress, "jaeger-address", "", "Jeager Collector Address")
	serverCmd.PersistentFlags().StringVar(&Bootnode, "bootnode", "", "Boot-node MultiAddr")
	serverCmd.PersistentFlags().StringVar(&PeerListFilePath, "peer-list", "", "Peer list file path")
	serverCmd.PersistentFlags().StringVar(&LogLevel, "log-level", "TRACE", "Logger level")
	serverCmd.PersistentFlags().Int64Var(
		&InboundConnLimit,
		"inbound-limit",
		common.DefaultInboundConnLimit,
		"Maximum inbound peer connection limit")
	serverCmd.PersistentFlags().Int64Var(
		&OutboundConnLimit,
		"outbound-limit",
		common.DefaultOutboundConnLimit,
		"Maximum outbound peer connection limit")

	if err := cobra.MarkFlagRequired(serverCmd.PersistentFlags(), "data-dir"); err != nil {
		log.Print("data-dir is required")
	}
}

func ReadConfig(path string) (*Config, error) {
	cfg := new(Config)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrReadingConfig
	}

	if err = json.Unmarshal(file, cfg); err != nil {
		log.Print(err)

		return nil, errors.Wrap(err, ErrReadingConfig.Error())
	}

	return cfg, nil
}

func BuildConfig(dataDir string, fileCfg *Config) (*common.Config, error) {
	var err error

	nodeCfg := common.DefaultConfig(dataDir)
	nodeCfg.LogFilePath = fileCfg.LogFilePath

	// TODO:Check node type and krama version

	buildChainConfig(nodeCfg, fileCfg)

	if err = buildNetworkConfig(nodeCfg, fileCfg); err != nil {
		return nil, err
	}

	buildConsensusConfig(nodeCfg, fileCfg)

	buildIxPoolConfig(nodeCfg, fileCfg)

	buildDBConfig(nodeCfg, fileCfg)

	if err = buildTelemetryConfig(nodeCfg, fileCfg); err != nil {
		return nil, err
	}

	buildVaultConfig(nodeCfg, fileCfg)

	return nodeCfg, nil
}

func buildChainConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.Genesis != "" {
		nodeCfg.Chain.GenesisFilePath = fileCfg.Genesis
	}
}

func buildNetworkConfig(nodeCfg *common.Config, fileCfg *Config) (err error) {
	assignNetworkSize(nodeCfg)

	assignNetworkMTQ(nodeCfg)

	assignNetworkNoDiscovery(nodeCfg)

	assignNetworkRefreshSenatus(nodeCfg)

	assignNetworkInboundLimit(nodeCfg, fileCfg)

	assignNetworkOutboundLimit(nodeCfg, fileCfg)

	if err = assignNetworkNodes(nodeCfg, fileCfg); err != nil {
		return err
	}

	if err = assignNetworkLibp2pListenAddress(nodeCfg, fileCfg); err != nil {
		return err
	}

	if err = assignNetworkJSONRPCAddr(nodeCfg, fileCfg); err != nil {
		return err
	}

	return nil
}

func buildConsensusConfig(nodeCfg *common.Config, fileCfg *Config) {
	if OperatorSlots != -1 {
		nodeCfg.Consensus.OperatorSlotCount = OperatorSlots
	} else if fileCfg.Consensus.OperatorSlots != 0 {
		nodeCfg.Consensus.OperatorSlotCount = fileCfg.Consensus.OperatorSlots
	}

	if ValidatorSlots != -1 {
		nodeCfg.Consensus.ValidatorSlotCount = ValidatorSlots
	} else if fileCfg.Consensus.ValidatorSlots != 0 {
		nodeCfg.Consensus.ValidatorSlotCount = fileCfg.Consensus.ValidatorSlots
	}

	if AccountWaitTime != 0 {
		nodeCfg.Consensus.AccountWaitTime = time.Duration(AccountWaitTime) * time.Millisecond
	} else if fileCfg.Consensus.AccountWaitTime != 0 {
		nodeCfg.Consensus.AccountWaitTime = time.Duration(fileCfg.Consensus.AccountWaitTime) * time.Millisecond
	}
}

func buildIxPoolConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.Ixpool.PriceLimit.Cmp(big.NewInt(0)) == 1 {
		nodeCfg.IxPool.PriceLimit = fileCfg.Ixpool.PriceLimit
	}

	if fileCfg.Ixpool.Mode != 0 {
		nodeCfg.IxPool.Mode = fileCfg.Ixpool.Mode
	}
}

func buildDBConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.DB.DBFolder != "" {
		nodeCfg.DB.DBFolderPath = fileCfg.DB.DBFolder
	}

	nodeCfg.DB.CleanDB = CleanDB
}

func buildTelemetryConfig(nodeCfg *common.Config, fileCfg *Config) (err error) {
	if fileCfg.Telemetry.PrometheusAddr != "" {
		nodeCfg.Metrics.PrometheusAddr, err = common.ResolveAddr(fileCfg.Telemetry.PrometheusAddr)
		if err != nil {
			return errors.New("invalid prometheus address")
		}
	}

	if EnableTracing {
		switch {
		case JaegerAddress != "":
			nodeCfg.Metrics.JaegerAddr = JaegerAddress
		case fileCfg.Telemetry.JaegerAddr != "":
			nodeCfg.Metrics.JaegerAddr = fileCfg.Telemetry.JaegerAddr
		default:
			return errors.New("tracing is enabled but a valid JaegerCollector address is not passed")
		}
	}

	return nil
}

func buildVaultConfig(nodeCfg *common.Config, fileCfg *Config) {
	if fileCfg.Vault.NodePassword != "" {
		nodeCfg.Vault.NodePassword = fileCfg.Vault.NodePassword
	}

	if fileCfg.Vault.DataDir != "" {
		nodeCfg.Vault.DataDir = fileCfg.Vault.DataDir
	}
}

func assignNetworkInboundLimit(nodeCfg *common.Config, fileCfg *Config) {
	if InboundConnLimit != common.DefaultInboundConnLimit {
		nodeCfg.Network.InboundConnLimit = InboundConnLimit
	} else if fileCfg.Network.InboundConnLimit != 0 {
		nodeCfg.Network.InboundConnLimit = fileCfg.Network.InboundConnLimit
	}
}

func assignNetworkOutboundLimit(nodeCfg *common.Config, fileCfg *Config) {
	if OutboundConnLimit != common.DefaultOutboundConnLimit {
		nodeCfg.Network.OutboundConnLimit = OutboundConnLimit
	} else if fileCfg.Network.OutboundConnLimit != 0 {
		nodeCfg.Network.OutboundConnLimit = fileCfg.Network.OutboundConnLimit
	}
}

func assignNetworkSize(nodeCfg *common.Config) {
	if NetworkSize != 0 {
		nodeCfg.Network.NetworkSize = NetworkSize
	}
}

func assignNetworkMTQ(nodeCfg *common.Config) {
	if MTQ != 0 {
		nodeCfg.Network.MTQ = MTQ
	}
}

func assignNetworkNoDiscovery(nodeCfg *common.Config) {
	nodeCfg.Network.NoDiscovery = NoDiscovery
}

func assignNetworkRefreshSenatus(nodeCfg *common.Config) {
	nodeCfg.Network.RefreshSenatus = RefreshSenatus
}

func assignNetworkBootStrapNodes(nodeCfg *common.Config, fileCfg *Config) error {
	if Bootnode != "" {
		addr, err := maddr.NewMultiaddr(Bootnode)
		if err != nil {
			return errors.New("invalid bootnode address")
		}

		nodeCfg.Network.BootstrapPeers = append(nodeCfg.Network.BootstrapPeers, addr)

		return nil
	}

	// validate bootnode address
	if len(fileCfg.Network.BootStrapPeers) == 0 {
		return errors.New("minimum one bootnode is required")
	}

	for _, v := range fileCfg.Network.BootStrapPeers {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid bootnode address")
		}

		nodeCfg.Network.BootstrapPeers = append(nodeCfg.Network.BootstrapPeers, addr)
	}

	return nil
}

func assignNetworkTrustedNodes(nodeCfg *common.Config, fileCfg *Config, trustedNodes []PeerInfo) error {
	if len(trustedNodes) == 0 && len(fileCfg.Network.TrustedPeers) > 0 {
		trustedNodes = fileCfg.Network.TrustedPeers
	}

	for _, trustedNode := range trustedNodes {
		addr, err := maddr.NewMultiaddr(trustedNode.Address)
		if err != nil {
			return errors.New("invalid trusted node address")
		}

		nodeCfg.Network.TrustedPeers = append(nodeCfg.Network.TrustedPeers, common.NodeInfo{
			ID:      kramaid.KramaID(trustedNode.ID),
			Address: addr,
		})
	}

	return nil
}

func assignNetworkStaticNodes(nodeCfg *common.Config, fileCfg *Config, staticNodes []PeerInfo) error {
	if len(staticNodes) == 0 && len(fileCfg.Network.StaticPeers) > 0 {
		staticNodes = fileCfg.Network.StaticPeers
	}

	for _, staticNode := range staticNodes {
		addr, err := maddr.NewMultiaddr(staticNode.Address)
		if err != nil {
			return errors.New("invalid static node address")
		}

		nodeCfg.Network.StaticPeers = append(nodeCfg.Network.StaticPeers, common.NodeInfo{
			ID:      kramaid.KramaID(staticNode.ID),
			Address: addr,
		})
	}

	return nil
}

func assignNetworkNodes(nodeCfg *common.Config, fileCfg *Config) error {
	peerList, err := ReadPeerList(PeerListFilePath)
	if err != nil {
		return err
	}

	if err = assignNetworkTrustedNodes(nodeCfg, fileCfg, peerList.TrustedPeers); err != nil {
		return err
	}

	if err = assignNetworkStaticNodes(nodeCfg, fileCfg, peerList.StaticPeers); err != nil {
		return err
	}

	if err = assignNetworkBootStrapNodes(nodeCfg, fileCfg); err != nil {
		return err
	}

	return nil
}

func assignNetworkLibp2pListenAddress(nodeCfg *common.Config, fileCfg *Config) error {
	if len(fileCfg.Network.Libp2pAddr) == 0 {
		return errors.New("lip2p address not specified")
	}

	for _, v := range fileCfg.Network.Libp2pAddr {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return errors.New("invalid libp2p address")
		}

		nodeCfg.Network.ListenAddresses = append(nodeCfg.Network.ListenAddresses, addr)
	}

	return nil
}

func assignNetworkJSONRPCAddr(nodeCfg *common.Config, fileCfg *Config) (err error) {
	// validate json-rpc address
	if fileCfg.Network.JSONRPCAddr == "" {
		return errors.New("empty json address")
	}

	nodeCfg.Network.JSONRPCAddr, err = common.ResolveAddr(fileCfg.Network.JSONRPCAddr)
	if err != nil {
		return errors.New("invalid json-rpc address")
	}

	return nil
}

func Err(err error) {
	if err != nil {
		log.Println("Error starting MOIPOD", err)
		os.Exit(1)
	}
}

func SetupNode(datadir string, cfgPath string) {
	profiling := profile.Start(profile.BlockProfile, profile.MutexProfile, profile.ProfilePath(datadir))
	closeCh := make(chan os.Signal, 1)

	defer profiling.Stop()

	fileCfg, err := ReadConfig(filepath.Join(datadir, cfgPath))
	if err != nil {
		Err(err)
	}

	cfg, err := BuildConfig(datadir, fileCfg)
	if err != nil {
		Err(err)
	}

	n, err := node.NewNode(LogLevel, cfg)
	if err != nil {
		Err(err)
	}

	err = n.Start()
	if err != nil {
		Err(err)
	}

	defer n.Stop()

	// init trace provider
	ctx := context.Background()

	tp, err := tracing.NewTracerProvider(ctx, EnableTracing, cfg.Metrics.JaegerAddr, n.GetKramaID())
	if err != nil {
		fmt.Println("error starting tp")
	}

	defer func() {
		fmt.Println("Shutting down trace provider")

		if err := tp.Shutdown(ctx); err != nil {
			fmt.Println("error shutting down trace provider")
		}
	}()

	otel.SetTracerProvider(tp)

	signal.Notify(closeCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-closeCh
}

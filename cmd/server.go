package cmd

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p-core/protocol"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/pkg/profile"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/poorna/node"

	//"os/signal"
	//"syscall"

	"github.com/spf13/cobra"
)

var (
	ErrReadingConfig = errors.New("error reading config file")
)

var OperatorSlots int
var ValidatorSlots int
var NetworkSize uint64
var MTQ float64
var SkipGenesis bool
var Bootnode string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the moi-chain server",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
	serverCmd.PersistentFlags().IntVar(&OperatorSlots, "operator-slots", 0, "Maximum number of operator slots")
	serverCmd.PersistentFlags().IntVar(&ValidatorSlots, "validator-slots", 0, "Maximum number of validator slots")
	serverCmd.PersistentFlags().Uint64Var(&NetworkSize, "network-size", 12, "Network Size")
	serverCmd.PersistentFlags().Float64Var(&MTQ, "mtq", 0.7, "Default MTQ")
	serverCmd.PersistentFlags().String("data-dir", "test-chain", "data directory location")
	serverCmd.PersistentFlags().BoolVar(&SkipGenesis, "skip-genesis", false, "Set the genesis")
	serverCmd.PersistentFlags().StringVar(&Bootnode, "bootnode", "", "Bootnode MultiAddr")

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

		return nil, ErrReadingConfig
	}

	return cfg, nil
}

func BuildConfig(dataDir string, cmdCfg *Config) (*common.Config, error) {
	var err error

	nodeCfg := common.DefaultConfig(dataDir)
	nodeCfg.LogFilePath = cmdCfg.LogFilePath
	//TODO:Check node type and krama version
	if cmdCfg.Genesis != "" {
		nodeCfg.Chain.Genesis = cmdCfg.Genesis
	}

	if SkipGenesis {
		nodeCfg.Chain.Genesis = "nil"
	}

	if NetworkSize != 0 {
		nodeCfg.Network.NetworkSize = NetworkSize
	}

	if MTQ != 0 {
		nodeCfg.Network.MTQ = MTQ
	}

	if OperatorSlots != 0 {
		nodeCfg.Consensus.OperatorSlotCount = OperatorSlots
	} else if cmdCfg.Consensus.OperatorSlots != 0 {
		nodeCfg.Consensus.OperatorSlotCount = cmdCfg.Consensus.OperatorSlots
	}

	if ValidatorSlots != 0 {
		nodeCfg.Consensus.ValidatorSlotCount = ValidatorSlots
	} else if cmdCfg.Consensus.ValidatorSlots != 0 {
		nodeCfg.Consensus.ValidatorSlotCount = cmdCfg.Consensus.ValidatorSlots
	}

	if Bootnode != "" {
		addr, err := maddr.NewMultiaddr(Bootnode)
		if err != nil {
			return nil, errors.New("invalid bootnode address")
		}

		nodeCfg.Network.BootstrapPeers = append(nodeCfg.Network.BootstrapPeers, addr)
	} else {
		// validate bootnode address
		if len(cmdCfg.Network.BootStrapPeers) == 0 {
			return nil, errors.New("minimum one bootnode is required")
		}

		for _, v := range cmdCfg.Network.BootStrapPeers {
			addr, err := maddr.NewMultiaddr(v)
			if err != nil {
				return nil, errors.New("invalid bootnode address")
			}

			nodeCfg.Network.BootstrapPeers = append(nodeCfg.Network.BootstrapPeers, addr)
		}
	}

	// validate listener address
	if len(cmdCfg.Network.Libp2pAddr) == 0 {
		return nil, errors.New("lip2p address not specified")
	}

	for _, v := range cmdCfg.Network.Libp2pAddr {
		addr, err := maddr.NewMultiaddr(v)
		if err != nil {
			return nil, errors.New("invalid libp2p address")
		}

		nodeCfg.Network.ListenAddresses = append(nodeCfg.Network.BootstrapPeers, addr)
	}

	// validate json-rpc address
	if cmdCfg.Network.JSONRPCAddr == "" {
		return nil, errors.New("empty json address")
	}

	nodeCfg.Network.JSONRPCAddr, err = common.ResolveAddr(cmdCfg.Network.JSONRPCAddr)
	if err != nil {
		return nil, errors.New("invalid json-rpc address")
	}

	if cmdCfg.Network.ProtocolID != "" {
		nodeCfg.Network.ProtocolID = protocol.ID(cmdCfg.Network.ProtocolID)
	}

	if cmdCfg.Ixpool.PriceLimit > 0 {
		nodeCfg.IxPool.PriceLimit = cmdCfg.Ixpool.PriceLimit
	}

	if cmdCfg.Ixpool.Mode != 0 {
		nodeCfg.IxPool.Mode = cmdCfg.Ixpool.Mode
	}

	if cmdCfg.DB.DBFolder != "" {
		nodeCfg.DB.DBFolderPath = cmdCfg.DB.DBFolder
	}

	if cmdCfg.Telemetry.PrometheusAddr != "" {
		nodeCfg.Metrics.PrometheusAddr, err = common.ResolveAddr(cmdCfg.Telemetry.PrometheusAddr)
		if err != nil {
			return nil, errors.New("invalid prometheus address")
		}
	}

	if cmdCfg.Vault.NodePassword != "" {
		nodeCfg.Vault.NodePassword = cmdCfg.Vault.NodePassword
	}

	if cmdCfg.Vault.DataDir != "" {
		nodeCfg.Vault.DataDir = cmdCfg.Vault.DataDir
	}

	return nodeCfg, nil
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

	fileCfg, err := ReadConfig(datadir + "/" + cfgPath)
	Err(err)

	cfg, err := BuildConfig(datadir, fileCfg)
	if err != nil {
		Err(err)
	}

	n, err := node.NewNode("TRACE", cfg)
	if err != nil {
		Err(err)
	}

	n.Start()

	signal.Notify(closeCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	for range closeCh {
		profiling.Stop()
		n.Stop()

		return
	}
}

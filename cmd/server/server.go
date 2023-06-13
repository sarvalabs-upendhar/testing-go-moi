package server

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	common2 "github.com/sarvalabs/moichain/cmd/common"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/poorna/node"
	"github.com/sarvalabs/moichain/telemetry/tracing"
)

var ErrReadingConfig = errors.New("error reading config file")

var (
	GenesisPath        string
	Directory          string
	ConfigPath         string
	AccountWaitTime    int
	OperatorSlots      int
	ValidatorSlots     int
	NetworkSize        uint64
	MTQ                float64
	EnableTracing      bool
	NoDiscovery        bool
	RefreshSenatus     bool
	Bootnode           string
	LogLevel           string
	JaegerAddress      string
	PeerListFilePath   string
	InboundConnLimit   int64
	OutboundConnLimit  int64
	CleanDB            bool
	CorsAllowedOrigins []string
)

func GetServerCommand() *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Starts the moi-chain server",
		Run:   runCommand,
	}

	parseFlags(serverCmd)

	return serverCmd
}

func runCommand(cmd *cobra.Command, args []string) {
	SetupNode()
}

func parseFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&GenesisPath, "genesis", "genesis.json", "Genesis file path")
	cmd.PersistentFlags().StringVar(&ConfigPath, "config", "config.json", "Config file path")
	cmd.PersistentFlags().IntVar(&AccountWaitTime, "wait-time", 0, "WaitTime per account")
	cmd.PersistentFlags().IntVar(&OperatorSlots, "operator-slots", -1, "Maximum number of operator slots")
	cmd.PersistentFlags().IntVar(&ValidatorSlots, "validator-slots", -1, "Maximum number of validator slots")
	cmd.PersistentFlags().Uint64Var(&NetworkSize, "network-size", 12, "Network Size")
	cmd.PersistentFlags().Float64Var(&MTQ, "mtq", 0.7, "Default MTQ")
	cmd.PersistentFlags().StringVar(&Directory, "data-dir", "test-chain", "Data directory location")
	cmd.PersistentFlags().BoolVar(&CleanDB, "clean-db", false, "Deletes the data stored in database")
	cmd.PersistentFlags().BoolVar(&EnableTracing, "enable-tracing", false, "Enable Tracing")
	cmd.PersistentFlags().BoolVar(&NoDiscovery, "no-discovery", false, "Disable peer discovery")
	cmd.PersistentFlags().BoolVar(&RefreshSenatus, "refresh-senatus", false, "Update the senatus with new peers")
	cmd.PersistentFlags().StringVar(&JaegerAddress, "jaeger-address", "", "Jeager Collector Address")
	cmd.PersistentFlags().StringVar(&Bootnode, "bootnode", "", "Boot-node MultiAddr")
	cmd.PersistentFlags().StringVar(&PeerListFilePath, "peer-list", "", "Peer list file path")
	cmd.PersistentFlags().StringVar(&LogLevel, "log-level", "DEBUG", "Logger level")
	cmd.PersistentFlags().StringSliceVar(
		&CorsAllowedOrigins,
		"allow-origins",
		[]string{},
		"The CORS header determines if the specified origin is allowed to receive any JSON-RPC response.",
	)
	cmd.PersistentFlags().Int64Var(
		&InboundConnLimit,
		"inbound-limit",
		common.DefaultInboundConnLimit,
		"Maximum inbound peer connection limit")
	cmd.PersistentFlags().Int64Var(
		&OutboundConnLimit,
		"outbound-limit",
		common.DefaultOutboundConnLimit,
		"Maximum outbound peer connection limit")

	if err := cobra.MarkFlagRequired(cmd.PersistentFlags(), "data-dir"); err != nil {
		log.Print("data-dir is required")
	}
}

func SetupNode() {
	profiling := profile.Start(profile.BlockProfile, profile.MutexProfile, profile.ProfilePath(Directory))
	closeCh := make(chan os.Signal, 1)

	defer profiling.Stop()

	fileCfg, err := ReadConfig(filepath.Join(Directory, ConfigPath))
	if err != nil {
		common2.Err(err)
	}

	cfg, err := BuildConfig(Directory, fileCfg)
	if err != nil {
		common2.Err(err)
	}

	n, err := node.NewNode(LogLevel, cfg)
	if err != nil {
		common2.Err(err)
	}

	err = n.Start()
	if err != nil {
		common2.Err(err)
	}

	defer n.Stop()

	// init trace provider
	ctx := context.Background()

	tp, err := tracing.NewTracerProvider(ctx, EnableTracing, cfg.Metrics.JaegerAddr, n.GetKramaID())
	if err != nil {
		log.Println("Error starting tp", "error", err)
	}

	defer func() {
		log.Println("Shutting down trace provider")

		if err := tp.Shutdown(ctx); err != nil {
			log.Println("Error shutting down trace provider", "error", err)
		}
	}()

	otel.SetTracerProvider(tp)

	signal.Notify(closeCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-closeCh
}

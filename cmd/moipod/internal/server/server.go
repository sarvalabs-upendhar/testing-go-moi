package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sarvalabs/go-moi/common/config"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/node"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/sarvalabs/go-moi/telemetry/tracing"
)

var ErrReadingConfig = errors.New("error reading config file")

var (
	GenesisPath        string
	Directory          string
	ConfigPath         string
	LogDirPath         string
	OperatorSlots      int
	ValidatorSlots     int
	EnableTracing      bool
	LogLevel           string
	CleanDB            bool
	CorsAllowedOrigins []string
	Babylon            bool
	Bootnodes          []string
	NodePassword       string
	P2pHostIP          string
	AllowIPv6Addresses bool
	DiscoveryInterval  time.Duration
	enableDebugMode    bool
)

const (
	genesisFlag            = "genesis-path"
	configFlag             = "config-path"
	LogDirPathFlag         = "log-dir"
	operatorSlotFlag       = "operator-slots"
	validatorSlotFlag      = "validator-slots"
	dataDirFlag            = "data-dir"
	cleanDBFlag            = "clean-db"
	enableTracingFlag      = "enable-tracing"
	logLevelFlag           = "log-level"
	allowOriginsFlag       = "allow-origins"
	babylonFlag            = "babylon"
	bootNodesFlag          = "bootnodes"
	nodePasswordFlag       = "node-password"
	p2pHostIPFlag          = "p2p-host-ip"
	allowIPv6AddressesFlag = "allow-ipv6-addresses"
	discoveryIntervalFlag  = "discovery-interval"
	enableDebugModeFlag    = "enable-debug-mode"
)

func GetServerCommand() *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Starts the MOI protocol server.",
		Run:   runCommand,
	}

	parseFlags(serverCmd)

	return serverCmd
}

func runCommand(cmd *cobra.Command, args []string) {
	if enableDebugMode {
		fmt.Println("WARNING: Debug mode is enabled. Do not use in production environment.")
	}

	SetupNode(cmd)
}

func parseFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&GenesisPath, genesisFlag, "genesis.json", "Path to genesis.json file.")
	cmd.PersistentFlags().StringVar(&P2pHostIP, p2pHostIPFlag, "0.0.0.0", "The ipv4 address for the p2p host.")
	cmd.PersistentFlags().StringVar(&ConfigPath, configFlag, "", "Path to config.json file.")
	cmd.PersistentFlags().StringVar(&LogDirPath, LogDirPathFlag, "", "Path to log directory.")
	cmd.PersistentFlags().IntVar(&OperatorSlots, operatorSlotFlag, -1, "Maximum number of operator slots.")
	cmd.PersistentFlags().IntVar(&ValidatorSlots, validatorSlotFlag, -1, "Maximum number of validator slots.")
	cmd.PersistentFlags().StringVar(&Directory, dataDirFlag, "", "Data directory location.")
	cmd.PersistentFlags().BoolVar(&CleanDB, cleanDBFlag, false, "Deletes the data stored in database.")
	cmd.PersistentFlags().BoolVar(&EnableTracing, enableTracingFlag, false, "Enables tracing.")
	cmd.PersistentFlags().StringVar(&LogLevel, logLevelFlag, "INFO", "Logger level.")
	cmd.PersistentFlags().BoolVar(
		&AllowIPv6Addresses,
		allowIPv6AddressesFlag,
		false,
		"Enable IPv6 communication for the p2p host.",
	)
	cmd.PersistentFlags().DurationVar(
		&DiscoveryInterval,
		discoveryIntervalFlag,
		config.DefaultDiscoveryInterval,
		"Time interval for discovering nodes.",
	)
	cmd.PersistentFlags().StringSliceVar(
		&CorsAllowedOrigins,
		allowOriginsFlag,
		[]string{},
		"The CORS header determines if the specified origin is allowed to receive any JSON-RPC response.",
	)
	cmd.PersistentFlags().BoolVar(
		&enableDebugMode,
		enableDebugModeFlag,
		false,
		"Enable debug mode for troubleshooting and debugging purposes. WARNING: Do not use in production environment.",
	)
	cmd.PersistentFlags().BoolVar(
		&Babylon,
		babylonFlag,
		false,
		"Connect to babylon network by downloading its genesis file.",
	)
	cmd.PersistentFlags().StringSliceVar(
		&Bootnodes,
		bootNodesFlag,
		[]string{},
		"List of bootnode multi-address.",
	)
	cmd.PersistentFlags().StringVar(
		&NodePassword,
		nodePasswordFlag,
		os.Getenv("NODE_PASSWORD"),
		"Node password which is used to decrypt keystore.",
	)

	if err := cobra.MarkFlagRequired(cmd.PersistentFlags(), "data-dir"); err != nil {
		cmdCommon.Err(err)
	}
}

func SetupNode(cmd *cobra.Command) {
	closeCh := make(chan os.Signal, 1)

	cfg, err := BuildNodeConfig(cmd, Directory)
	if err != nil {
		cmdCommon.Err(err)
	}

	n, err := node.NewNode(LogLevel, cfg)
	if err != nil {
		cmdCommon.Err(err)
	}

	err = n.Start()
	if err != nil {
		cmdCommon.Err(err)
	}

	defer n.Stop()

	// init trace provider
	ctx := context.Background()

	tp, err := tracing.NewTracerProvider(
		ctx, EnableTracing,
		cfg.Metrics.OtlpAddress,
		cfg.Metrics.Token,
		cfg.NetworkID,
		n.GetKramaID(),
	)
	if err != nil {
		log.Println("Error starting tp", "err", err)
	}

	defer func() {
		log.Println("Shutting down trace provider")

		if err := tp.Shutdown(ctx); err != nil {
			log.Println("Error shutting down trace provider", "err", err)
		}
	}()

	otel.SetTracerProvider(tp)

	signal.Notify(closeCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-closeCh
}

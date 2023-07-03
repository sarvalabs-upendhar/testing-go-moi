package server

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	cmdCommon "github.com/sarvalabs/moichain/cmd/common"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/sarvalabs/moichain/poorna/node"
	"github.com/sarvalabs/moichain/telemetry/tracing"
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
)

const (
	genesisFlag       = "genesis-path"
	configFlag        = "config-path"
	LogDirPathFlag    = "log-dir"
	operatorSlotFlag  = "operator-slots"
	validatorSlotFlag = "validator-slots"
	dataDirFlag       = "data-dir"
	cleanDBFlag       = "clean-db"
	enableTracingFlag = "enable-tracing"
	logLevelFlag      = "log-level"
	allowOriginsFlag  = "allow-origins"
	babylonFlag       = "babylon"
	bootNodesFlag     = "bootnodes"
	nodePasswordFlag  = "node-password"
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
	SetupNode(cmd)
}

func parseFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&GenesisPath, genesisFlag, "genesis.json", "Path to genesis.json file.")
	cmd.PersistentFlags().StringVar(&ConfigPath, configFlag, "", "Path to config.json file.")
	cmd.PersistentFlags().StringVar(&LogDirPath, LogDirPathFlag, "", "Path to log directory.")
	cmd.PersistentFlags().IntVar(&OperatorSlots, operatorSlotFlag, -1, "Maximum number of operator slots.")
	cmd.PersistentFlags().IntVar(&ValidatorSlots, validatorSlotFlag, -1, "Maximum number of validator slots.")
	cmd.PersistentFlags().StringVar(&Directory, dataDirFlag, "", "Data directory location.")
	cmd.PersistentFlags().BoolVar(&CleanDB, cleanDBFlag, false, "Deletes the data stored in database.")
	cmd.PersistentFlags().BoolVar(&EnableTracing, enableTracingFlag, false, "Enables tracing.")
	cmd.PersistentFlags().StringVar(&LogLevel, logLevelFlag, "INFO", "Logger level.")
	cmd.PersistentFlags().StringSliceVar(
		&CorsAllowedOrigins,
		allowOriginsFlag,
		[]string{},
		"The CORS header determines if the specified origin is allowed to receive any JSON-RPC response.",
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
		"list of bootnode multi-address.",
	)
	cmd.PersistentFlags().StringVar(
		&NodePassword,
		nodePasswordFlag,
		"",
		"Node password which is used to decrypt keystore.",
	)

	if err := cobra.MarkFlagRequired(cmd.PersistentFlags(), "data-dir"); err != nil {
		cmdCommon.Err(err)
	}
}

func SetupNode(cmd *cobra.Command) {
	profiling := profile.Start(profile.BlockProfile, profile.MutexProfile, profile.ProfilePath(Directory))
	closeCh := make(chan os.Signal, 1)

	defer profiling.Stop()

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

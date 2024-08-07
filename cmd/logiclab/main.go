package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/cmd/logiclab/api"
	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
)

func init() {
	engineio.RegisterEngine(pisa.NewEngine())
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	CliRoot().Execute()
}

type CliCommand struct {
	cmd *cobra.Command
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (cli *CliCommand) Execute() {
	if err := cli.cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func CliRoot() *CliCommand {
	root := &CliCommand{
		cmd: &cobra.Command{
			Use:   "logiclab",
			Short: "Start LogicLab sessions and manage LogicLab environments.",
			Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.`,
			Run: startLogicLab,
		},
	}

	parseflags(root.cmd)

	return root
}

func parseflags(cmd *cobra.Command) {
	// -d | --dir [string]
	cmd.PersistentFlags().StringP(
		"dir", "d", "",
		fmt.Sprintf("root directory path for logiclab. if the 'dir' flag is provided, its value is used. otherwise, "+
			"it defaults to the value of 'LABDIR' environment variable, if set, or to '%v' if "+
			"neither is specified.", core.DefaultDir),
	)

	// -s | --silent [bool]
	cmd.PersistentFlags().BoolP(
		"silent", "s", false,
		"run API silently without emitting any logs. OFF by default",
	)

	// -p | --port [int]
	cmd.PersistentFlags().IntP(
		"port", "p", core.DefaultPort,
		fmt.Sprintf("port to run logiclab. defaults to '%v'", core.DefaultPort),
	)

	// -v | --version [bool]
	cmd.PersistentFlags().BoolP(
		"version", "v", false,
		"print the LogicLab version",
	)
}

func startLogicLab(command *cobra.Command, args []string) {
	// Check if the version flag is set
	if version, _ := command.Flags().GetBool("version"); version {
		// Print logiclab version
		fmt.Printf("LogicLab %v\n", config.ProtocolVersion)

		return
	}

	// Get the lab root dirpath from the input flags (defaults if not provided)
	dir, _ := command.Flags().GetString("dir")
	if dir == "" {
		labdir, exists := os.LookupEnv("LABDIR")
		if exists {
			dir = labdir
		} else {
			dir = core.DefaultDir
		}
	}

	dir, _ = filepath.Abs(dir)

	// Create a logiclab instance
	lab, err := core.NewLab(dir)
	if err != nil {
		fmt.Println(err)

		return
	}

	// Start the interrupt handler
	handler := lab.HandleInterrupt()
	go handler()

	// Get the port number for the logiclab environment (API)
	port, _ := command.Flags().GetInt("port")

	// Validate port number
	if port < 0 || port > 65535 {
		fmt.Printf("invalid port number: %d\n", port)

		return
	}

	// Print logiclab launch text if not silent
	if silent, _ := command.Flags().GetBool("silent"); !silent {
		fmt.Println(fmt.Sprintf(core.LAUNCH, dir, core.DOCS, port))
	} else {
		fmt.Println(core.DIVIDE) // print just the divider if suppressed
		// Disable Gin logging
		gin.DefaultWriter = io.Discard
	}

	// Create a new API instance and start it
	api := api.NewAPI(lab)

	if err = api.Start(port); err != nil {
		fmt.Printf("Cannot start Logiclab API, can't listen to port %d: %v\n", port, err)
		os.Exit(1)
	}
}
